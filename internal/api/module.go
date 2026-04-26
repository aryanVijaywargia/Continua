package api

import (
	"go.uber.org/fx"

	"github.com/continua-ai/continua/internal/api/middleware"
	"github.com/continua-ai/continua/internal/config"
	"github.com/continua-ai/continua/internal/store"
)

// Module provides API handlers for the application.
var Module = fx.Module("api",
	fx.Provide(newConfiguredServer),
	fx.Provide(newConfiguredAuthenticator),
	fx.Provide(NewRouter),
)

func newConfiguredAuthenticator(s *store.Store, cfg *config.Config) (*middleware.Authenticator, error) {
	return middleware.NewAuthenticator(s, cfg)
}
