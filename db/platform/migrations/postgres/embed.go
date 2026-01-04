package postgres

import "embed"

//go:embed *.sql
var Migrations embed.FS
