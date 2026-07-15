package api

import (
	"context"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
)

func notifyEngineChannel(ctx context.Context, db enginedb.DBTX, channel string) error {
	_, err := db.Exec(ctx, "SELECT pg_notify($1, '')", channel)
	return err
}
