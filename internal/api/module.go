package api

import (
	"go.uber.org/fx"
)

// Module provides API handlers for the application.
var Module = fx.Module("api",
	fx.Provide(newConfiguredServer),
	fx.Provide(NewRouter),
)
