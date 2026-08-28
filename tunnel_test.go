package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScanForTunnelURLFindsURL(t *testing.T) {
	out := strings.Join([]string{
		"2026-08-29T00:00:00Z INF Starting tunnel",
		"2026-08-29T00:00:01Z INF +--------------------------------------------------------------------------------------------+",
		"2026-08-29T00:00:01Z INF |  Your quick Tunnel has been created! Visit it at (it may take some time to be reachable):     |",
		"2026-08-29T00:00:01Z INF |  https://random-two-words.trycloudflare.com                                                  |",
		"2026-08-29T00:00:01Z INF +--------------------------------------------------------------------------------------------+",
	}, "\n")
	url, err := scanForTunnelURL(strings.NewReader(out))
	if err != nil {
		t.Fatalf("scanForTunnelURL: %v", err)
	}
	if url != "https://random-two-words.trycloudflare.com" {
		t.Fatalf("got %q", url)
	}
}

func TestScanForTunnelURLIgnoresUnrelatedURLs(t *testing.T) {
	out := strings.Join([]string{
		"INF see https://developers.cloudflare.com/cloudflared for docs",
		"INF connecting to edge",
		"INF registered tunnel at https://sunny-clouds-77.trycloudflare.com",
	}, "\n")
	url, err := scanForTunnelURL(strings.NewReader(out))
	if err != nil {
		t.Fatalf("scanForTunnelURL: %v", err)
	}
	if url != "https://sunny-clouds-77.trycloudflare.com" {
		t.Fatalf("got %q", url)
	}
}

func TestScanForTunnelURLNoURLReturnsError(t *testing.T) {
	out := "INF starting\nINF connecting\nERR failed to connect\n"
	if _, err := scanForTunnelURL(strings.NewReader(out)); err == nil {
		t.Fatal("want error when no URL is ever printed")
	}
}

// TestStartTunnelParsesURLAndStops exercises the real process-spawning path
// with a fake "cloudflared" script instead of the network binary: it prints
// a quick-tunnel URL line and then idles, mimicking a real tunnel that stays
// up until killed.
func TestStartTunnelParsesURLAndStops(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-cloudflared.sh")
	body := "#!/bin/sh\n" +
		"echo 'INF Starting tunnel'\n" +
		"echo 'INF https://fake-name.trycloudflare.com is online'\n" +
		"sleep 30\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}

	tun, err := startTunnel(context.Background(), script, 4999)
	if err != nil {
		t.Fatalf("startTunnel: %v", err)
	}
	defer tun.Stop()

	if tun.URL != "https://fake-name.trycloudflare.com" {
		t.Fatalf("URL = %q", tun.URL)
	}
	if tun.Hostname != "fake-name.trycloudflare.com" {
		t.Fatalf("Hostname = %q", tun.Hostname)
	}

	tun.Stop()
	done := make(chan error, 1)
	go func() { done <- tun.cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("process still running after Stop")
	}
}

func TestStartTunnelErrorsWhenNoURLPrinted(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-cloudflared-silent.sh")
	body := "#!/bin/sh\n" +
		"echo 'INF nothing useful here'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := startTunnel(context.Background(), script, 4999); err == nil {
		t.Fatal("want error when cloudflared exits without a tunnel URL")
	}
}
