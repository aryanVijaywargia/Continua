module github.com/aryanVijaywargia/Continua/server

go 1.22

require go.uber.org/zap v1.27.0

require (
	github.com/stretchr/testify v1.9.0 // indirect
	go.uber.org/multierr v1.11.0 // indirect
)

replace github.com/aryanVijaywargia/Continua/packages/proto-go => ../packages/proto-go
