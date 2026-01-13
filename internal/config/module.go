package config

import "go.uber.org/fx"

// Module provides configuration loading for the application.
var Module = fx.Provide(Load)
