package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "init" {
		runInit(os.Args[2:])
		return
	}

	port := flag.Int("port", 0, "listen port (default: a random free port)")
	noOpen := flag.Bool("no-open", false, "do not auto-open the browser")
	useTunnel := flag.Bool("tunnel", false, "expose the review on a public https URL via a Cloudflare quick tunnel, for review from another device (implies --no-open)")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: mdreview [flags] <file.md>")
		fmt.Fprintln(os.Stderr, "       mdreview init [--project]")
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	path := flag.Arg(0)
	source, err := os.ReadFile(path)
	if err != nil {
		fatal(err)
	}
	doc, err := Render(source)
	if err != nil {
		fatal(err)
	}
	srv, err := NewServer(filepath.Base(path), doc, source)
	if err != nil {
		fatal(err)
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		fatal(err)
	}
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		fatal(fmt.Errorf("unexpected listener address type %T", ln.Addr()))
	}
	localURL := fmt.Sprintf("http://%s/", ln.Addr())

	var tun *tunnel
	if *useTunnel {
		bin, err := findOrFetchCloudflared()
		if err != nil {
			fatal(err)
		}
		tun, err = startTunnel(context.Background(), bin, addr.Port)
		if err != nil {
			fatal(err)
		}
		defer tun.Stop()

		token, err := srv.EnableTunnel(tun.Hostname)
		if err != nil {
			fatal(err)
		}
		localURL = fmt.Sprintf("http://%s/?t=%s", ln.Addr(), token)
		fmt.Fprintf(os.Stderr, "mdreview: local:  %s\n", localURL)
		fmt.Fprintf(os.Stderr, "mdreview: remote: %s/?t=%s\n", tun.URL, token)
	} else {
		fmt.Fprintf(os.Stderr, "mdreview: reviewing %s at %s\n", path, localURL)
	}

	go func() {
		if err := http.Serve(ln, srv.Handler()); err != nil {
			fatal(err)
		}
	}()
	// Tunnel mode is for reviewing from another device; auto-opening a
	// local browser defeats that point, and both URLs are already on
	// stderr for the reviewer to pick from.
	if !*noOpen && !*useTunnel {
		openBrowser(localURL)
	}

	// Block until the reviewer submits; stdout carries only the feedback.
	fmt.Print(FormatFeedback(srv.Wait()))
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "mdreview: could not open browser (%v); open the URL above manually\n", err)
	}
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "mdreview: %v\n", err)
	os.Exit(1)
}
