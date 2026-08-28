package main

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// tunnelURLPattern matches the public quick-tunnel URL cloudflared prints to
// its own stdout/stderr once the tunnel is up, e.g.
// "https://random-two-words.trycloudflare.com".
var tunnelURLPattern = regexp.MustCompile(`https://[a-z0-9-]+\.trycloudflare\.com`)

// tunnel is a running cloudflared quick tunnel.
type tunnel struct {
	URL      string // e.g. "https://random-two-words.trycloudflare.com"
	Hostname string // e.g. "random-two-words.trycloudflare.com"
	cmd      *exec.Cmd
}

// Stop terminates the cloudflared process. Safe to call on a nil tunnel.
func (t *tunnel) Stop() {
	if t == nil || t.cmd == nil || t.cmd.Process == nil {
		return
	}
	t.cmd.Process.Kill()
	t.cmd.Wait()
}

// findOrFetchCloudflared locates a usable cloudflared binary: one already on
// PATH, or a copy previously downloaded to the user cache directory, or a
// freshly downloaded one (cached for subsequent runs).
func findOrFetchCloudflared() (string, error) {
	if p, err := exec.LookPath("cloudflared"); err == nil {
		return p, nil
	}

	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", fmt.Errorf("locate cache directory: %w", err)
	}
	dir := filepath.Join(cacheDir, "mdreview")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create cache directory: %w", err)
	}
	bin := filepath.Join(dir, "cloudflared")
	if _, err := os.Stat(bin); err == nil {
		return bin, nil
	}

	fmt.Fprintln(os.Stderr, "mdreview: downloading cloudflared (~40MB, first run only)…")
	if err := downloadCloudflared(bin); err != nil {
		return "", err
	}
	return bin, nil
}

// downloadCloudflared fetches the official cloudflared release binary for
// the current OS/arch and writes it to dest with executable permissions.
func downloadCloudflared(dest string) error {
	arch := runtime.GOARCH
	if arch != "amd64" && arch != "arm64" {
		return fmt.Errorf("unsupported architecture %q for automatic cloudflared download; install cloudflared manually and put it on PATH", arch)
	}

	switch runtime.GOOS {
	case "linux":
		url := fmt.Sprintf("https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-linux-%s", arch)
		return downloadRawBinary(url, dest)
	case "darwin":
		url := fmt.Sprintf("https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-darwin-%s.tgz", arch)
		return downloadTarballBinary(url, dest)
	default:
		return fmt.Errorf("automatic cloudflared download is not supported on %s; install cloudflared manually and put it on PATH", runtime.GOOS)
	}
}

// downloadRawBinary downloads the bare cloudflared binary served at url
// directly to dest (the linux release asset).
func downloadRawBinary(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download cloudflared: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download cloudflared: unexpected status %s", resp.Status)
	}
	return writeExecutable(dest, resp.Body)
}

// downloadTarballBinary downloads the .tgz served at url and extracts the
// single "cloudflared" binary from it to dest (the darwin release asset).
func downloadTarballBinary(url, dest string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download cloudflared: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download cloudflared: unexpected status %s", resp.Status)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("open cloudflared archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("cloudflared binary not found in downloaded archive")
		}
		if err != nil {
			return fmt.Errorf("read cloudflared archive: %w", err)
		}
		if filepath.Base(hdr.Name) != "cloudflared" {
			continue
		}
		return writeExecutable(dest, tr)
	}
}

// writeExecutable copies r to dest and marks it executable.
func writeExecutable(dest string, r io.Reader) error {
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("create cloudflared binary: %w", err)
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("write cloudflared binary: %w", err)
	}
	return nil
}

// startTunnel spawns cloudflared as a quick tunnel for the local server at
// 127.0.0.1:port, and waits up to 30 seconds for it to announce its public
// trycloudflare.com URL. cloudflared's own output is scanned for that URL
// and then discarded; it never reaches mdreview's stdout/stderr.
func startTunnel(ctx context.Context, bin string, port int) (*tunnel, error) {
	// --protocol http2: cloudflared defaults to QUIC (UDP) and only falls
	// back to HTTP/2 after a slow, unreliable auto-detection. Networks
	// that block outbound UDP (common in sandboxes and corporate
	// networks) are common enough that forcing HTTP/2 up front is more
	// robust, at no cost when QUIC would have worked anyway.
	cmd := exec.CommandContext(ctx, bin, "tunnel", "--url", fmt.Sprintf("http://127.0.0.1:%d", port), "--protocol", "http2")

	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("start cloudflared: %w", err)
	}
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		pr.Close()
		pw.Close()
		return nil, fmt.Errorf("start cloudflared: %w", err)
	}
	pw.Close() // our copy; the child keeps its own until it exits

	type scanResult struct {
		url string
		err error
	}
	found := make(chan scanResult, 1)
	go func() {
		url, err := scanForTunnelURL(pr)
		found <- scanResult{url, err}
	}()

	select {
	case res := <-found:
		if res.err != nil {
			cmd.Process.Kill()
			cmd.Wait()
			return nil, fmt.Errorf("cloudflared: %w", res.err)
		}
		// Keep draining so cloudflared never blocks on a full pipe once we
		// stop reading for the URL ourselves.
		go io.Copy(io.Discard, pr)
		return &tunnel{
			URL:      res.url,
			Hostname: strings.TrimPrefix(res.url, "https://"),
			cmd:      cmd,
		}, nil
	case <-time.After(30 * time.Second):
		cmd.Process.Kill()
		cmd.Wait()
		return nil, fmt.Errorf("timed out waiting for cloudflared to print a tunnel URL")
	}
}

// scanForTunnelURL reads lines from r until one contains a trycloudflare.com
// URL, returning it. Returns an error if r is exhausted first.
func scanForTunnelURL(r io.Reader) (string, error) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		if m := tunnelURLPattern.FindString(sc.Text()); m != "" {
			return m, nil
		}
	}
	return "", fmt.Errorf("cloudflared exited without printing a tunnel URL")
}
