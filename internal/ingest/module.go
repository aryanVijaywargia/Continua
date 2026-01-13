package ingest

import (
	"go.uber.org/fx"
)

// Module provides the ingest service for the application.
var Module = fx.Provide(NewService)
