package ingest

import (
	"go.uber.org/fx"

	"github.com/continua-ai/continua/internal/store"
)

// Module provides the ingest service for the application.
var Module = fx.Provide(func(s *store.Store) *Service {
	return NewService(s)
})
