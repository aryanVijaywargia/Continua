package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/continua-ai/continua/internal/api/middleware"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/web"
)

// NewRouter creates the main HTTP router with all handlers mounted.
// Health endpoint is public (no auth), all OpenAPI endpoints are protected.
// The SPA is served at "/" for production builds.
func NewRouter(server *Server, s *store.Store) http.Handler {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)

	// Public: health endpoint (NOT in OpenAPI - routed directly)
	r.Get("/api/health", server.HealthCheck)

	// Protected: all OpenAPI routes
	r.Group(func(r chi.Router) {
		r.Use(engineRouteAvailabilityMiddleware(server))
		r.Use(middleware.APIKeyAuth(s))
		r.Use(enginePreviewHeaderMiddleware())

		// Mount OpenAPI handlers
		HandlerWithOptions(server, ChiServerOptions{
			BaseRouter: r,
		})
	})

	// SPA: serve embedded web UI at root for production
	// In development, Vite serves the UI directly
	r.Handle("/*", web.Handler())

	return r
}
