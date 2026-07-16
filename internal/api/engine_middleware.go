package api

import (
	"log/slog"
	"net/http"
	"strings"
)

const enginePreviewHeader = "X-Continua-Engine-Preview"

func engineRouteAvailabilityMiddleware(server *Server) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/v1/engine") {
				next.ServeHTTP(w, r)
				return
			}

			if server == nil || !server.enginePublicAPIEnabled {
				http.NotFound(w, r)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func enginePreviewHeaderMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/v1/engine") {
				next.ServeHTTP(w, r)
				return
			}

			if r.Method == http.MethodPost && len(r.Header.Values(enginePreviewHeader)) == 0 {
				slog.Warn(enginePreviewHeader+" requirement is deprecated and removed during the sunset window", "path", r.URL.Path)
			}

			next.ServeHTTP(w, r)
		})
	}
}
