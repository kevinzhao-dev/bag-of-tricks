package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"

	"photo-viewer/internal/pv"
)

func main() {
	var (
		port   = flag.Int("port", 0, "HTTP port (0 = random)")
		noOpen = flag.Bool("no-open", false, "don't auto-open browser")
		latest = flag.Bool("latest", false, "sort by modification time (most recent first)")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "pv - keyboard-first photo viewer\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n  %s [flags] [path]\n\n", filepath.Base(os.Args[0]))
		fmt.Fprintf(os.Stderr, "Path may be an image file or a directory (default: .).\n\nFlags:\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	path := "."
	if flag.NArg() > 0 {
		path = flag.Arg(0)
	}

	images, err := pv.BuildGallery(path, *latest)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Found %d images\n", len(images))

	app := pv.NewApp(images)
	mux := pv.NewServeMux(app)

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to listen: %v\n", err)
		os.Exit(1)
	}
	addr := ln.Addr().String()
	url := "http://" + addr

	fmt.Fprintf(os.Stderr, "Serving at %s\n", url)

	if !*noOpen {
		openBrowser(url)
	}

	// Graceful shutdown on Ctrl-C or quit command
	srv := &http.Server{Handler: mux}
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt)
		select {
		case <-sigCh:
		case <-app.Quit():
		}
		fmt.Fprintln(os.Stderr, "\nShutting down...")
		srv.Shutdown(context.Background())
	}()

	if err := srv.Serve(ln); err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

func openBrowser(url string) {
	var cmd string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "linux":
		cmd = "xdg-open"
	default:
		return
	}
	exec.Command(cmd, url).Start()
}
