package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

// Sentinel errors for store operations.
var (
	ErrNotFound = errors.New("store: record not found")
)

// Store provides database access for the platform.
type Store struct {
	pool *pgxpool.Pool
	q    *platform.Queries
}

// New creates a new Store with the given connection pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{
		pool: pool,
		q:    platform.New(pool),
	}
}

// Pool returns the underlying connection pool.
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// Queries returns the sqlc queries instance.
func (s *Store) Queries() *platform.Queries {
	return s.q
}

// BeginTx starts a new transaction with the given options.
//
//nolint:gocritic // pgx.TxOptions is a small config struct used idiomatically by value in pgx APIs.
func (s *Store) BeginTx(ctx context.Context, opts pgx.TxOptions) (*Tx, error) {
	tx, err := s.pool.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &Tx{
		tx: tx,
		q:  s.q.WithTx(tx),
	}, nil
}

// IsNotFound returns true if the error is ErrNotFound or pgx.ErrNoRows.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, pgx.ErrNoRows)
}
