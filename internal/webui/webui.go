// Package webui embeds the AssistClaw web chat interface as static assets.
// The embedded HTML/JS/CSS is served directly from the binary with no external build step.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed assets/*
var embeddedAssets embed.FS

// Assets returns the embedded asset filesystem (rooted at the assets/ subdirectory).
func Assets() (fs.FS, error) {
	return fs.Sub(embeddedAssets, "assets")
}

// Handler returns an http.Handler that serves the embedded web UI.
// It mounts the assets at /assets/ and serves index.html for the root path.
func Handler() (http.Handler, error) {
	assets, err := Assets()
	if err != nil {
		return nil, err
	}
	fsHandler := http.FileServer(http.FS(assets))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Serve index.html for root
		if r.URL.Path == "/" || r.URL.Path == "" {
			r.URL.Path = "/index.html"
		}
		// Add security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		fsHandler.ServeHTTP(w, r)
	}), nil
}
