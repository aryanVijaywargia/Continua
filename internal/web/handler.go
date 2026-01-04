package web

import (
	"io/fs"
	"net/http"
	"strings"
)

// Handler returns an http.Handler that serves the embedded SPA.
func Handler() http.Handler {
	// Get the static subdirectory
	staticFS, err := fs.Sub(StaticFS, "static")
	if err != nil {
		panic(err)
	}

	fileServer := http.FileServer(http.FS(staticFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Serve API routes separately (this handler shouldn't receive them)
		if strings.HasPrefix(path, "/api/") {
			http.NotFound(w, r)
			return
		}

		// Try to serve the file directly
		if path != "/" && !strings.HasPrefix(path, "/assets/") {
			// For SPA routing, serve index.html for non-asset paths
			r.URL.Path = "/"
		}

		fileServer.ServeHTTP(w, r)
	})
}
