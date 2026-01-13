package api

import (
	"net/http"

	"go.uber.org/fx"

	"github.com/continua-ai/continua/internal/store"
)

// Module provides API handlers for the application.
var Module = fx.Module("api",
	fx.Provide(NewServer),
	fx.Provide(func(server *Server, s *store.Store) http.Handler {
		return NewRouter(server, s)
	}),
)
