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

		// An API path reaches this fallback only when no route matched its path and
		// method, so it must fail as an API error, never as the SPA page.
		if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/v1/") {
			http.NotFound(w, r)
			return
		}

		// The SPA only answers page reads.
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
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
