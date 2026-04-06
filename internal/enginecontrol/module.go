package enginecontrol

import "go.uber.org/fx"

var Module = fx.Module("enginecontrol",
	fx.Provide(NewService),
)
