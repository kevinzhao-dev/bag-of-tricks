package pv

import (
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"photo-viewer/internal/web"
)

// NewServeMux creates the HTTP handler for the app.
func NewServeMux(app *App) http.Handler {
	mux := http.NewServeMux()

	// Serve embedded static files
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			data, err := web.Static.ReadFile("static/index.html")
			if err != nil {
				http.Error(w, "not found", 404)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(data)
			return
		}
		// Serve other static files
		name := strings.TrimPrefix(r.URL.Path, "/")
		data, err := web.Static.ReadFile(name)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		ext := filepath.Ext(name)
		ct := mime.TypeByExtension(ext)
		if ct == "" {
			ct = "application/octet-stream"
		}
		w.Header().Set("Content-Type", ct)
		w.Write(data)
	})

	// Serve images by index
	mux.HandleFunc("/image", func(w http.ResponseWriter, r *http.Request) {
		idxStr := r.URL.Query().Get("idx")
		idx, err := strconv.Atoi(idxStr)
		if err != nil {
			http.Error(w, "bad idx", 400)
			return
		}
		path, err := app.ImagePath(idx)
		if err != nil {
			http.Error(w, err.Error(), 404)
			return
		}
		ext := strings.ToLower(filepath.Ext(path))
		ct := mime.TypeByExtension(ext)
		if ct == "" {
			ct = "application/octet-stream"
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Cache-Control", "public, max-age=86400, immutable")

		f, err := os.Open(path)
		if err != nil {
			http.Error(w, "cannot open image", 500)
			return
		}
		defer f.Close()
		io.Copy(w, f)
	})

	// SSE endpoint for state updates
	mux.HandleFunc("/events", func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming not supported", 500)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		ch := app.Subscribe()
		defer app.Unsubscribe(ch)

		// Send initial state immediately
		snap := app.State.Snapshot()
		data, _ := json.Marshal(snap)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		for {
			select {
			case snap, ok := <-ch:
				if !ok {
					return
				}
				data, _ := json.Marshal(snap)
				fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})

	// Command endpoint
	mux.HandleFunc("/cmd", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", 405)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad body", 400)
			return
		}
		app.HandleCommand(body)
		w.WriteHeader(http.StatusNoContent)
	})

	return mux
}
