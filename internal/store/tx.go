package store

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

// Tx wraps a database transaction with sqlc queries.
type Tx struct {
	tx pgx.Tx
	q  *platform.Queries
}

// Queries returns the sqlc queries instance scoped to this transaction.
func (t *Tx) Queries() *platform.Queries {
	return t.q
}

// Commit commits the transaction.
func (t *Tx) Commit(ctx context.Context) error {
	return t.tx.Commit(ctx)
}

// Rollback rolls back the transaction.
func (t *Tx) Rollback(ctx context.Context) error {
	return t.tx.Rollback(ctx)
}

// Tx returns the underlying pgx.Tx.
func (t *Tx) Tx() pgx.Tx {
	return t.tx
}
