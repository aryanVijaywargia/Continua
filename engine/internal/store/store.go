package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	"github.com/continua-ai/continua/engine/internal/config"
)

const uniqueViolationSQLState = "23505"

var (
	ErrNotFound        = errors.New("engine store: record not found")
	ErrAlreadyExists   = errors.New("engine store: record already exists")
	ErrStaleClaim      = errors.New("engine store: stale claim")
	ErrStaleCheckpoint = errors.New("engine store: stale checkpoint")
	ErrInvariant       = errors.New("engine store: invariant violation")
)

type storeOps struct {
	q             *enginedb.Queries
	projectFilter *uuid.UUID
}

// Store provides engine database access backed by a dedicated pgx pool.
type Store struct {
	pool *pgxpool.Pool
	*storeOps
}

// Tx wraps an engine transaction and exposes the same business-operation methods
// as Store.
type Tx struct {
	tx pgx.Tx
	*storeOps
}

// New constructs a store around an existing pool.
func New(pool *pgxpool.Pool) *Store {
	return &Store{
		pool:     pool,
		storeOps: &storeOps{q: enginedb.New(pool)},
	}
}

func (s *Store) WithProjectFilter(projectID uuid.UUID) *Store {
	if s == nil {
		return nil
	}
	filter := projectID
	return &Store{
		pool: s.pool,
		storeOps: &storeOps{
			q:             s.q,
			projectFilter: &filter,
		},
	}
}

func (o *storeOps) ProjectFilter() *uuid.UUID {
	if o == nil || o.projectFilter == nil {
		return nil
	}
	filter := *o.projectFilter
	return &filter
}

// NewPool constructs a pgx pool using the engine defaults from config.
func NewPool(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.Database.URL)
	if err != nil {
		return nil, fmt.Errorf("parse engine database url: %w", err)
	}

	poolConfig.MaxConns = cfg.Database.MaxConns
	poolConfig.MinConns = cfg.Database.MinConns
	poolConfig.MaxConnLifetime = cfg.Database.MaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.Database.MaxConnIdleTime
	poolConfig.HealthCheckPeriod = cfg.Database.HealthCheckPeriod

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create engine database pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping engine database: %w", err)
	}

	return pool, nil
}

// Close closes the underlying pool.
func (s *Store) Close() {
	if s == nil || s.pool == nil {
		return
	}
	s.pool.Close()
}

// Pool exposes the underlying pgx pool.
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// Queries exposes the generated sqlc query handle.
func (o *storeOps) Queries() *enginedb.Queries {
	return o.q
}

// BeginTx begins a new transaction with the provided options.
//
//nolint:gocritic // pgx.TxOptions is an idiomatic small config struct.
func (s *Store) BeginTx(ctx context.Context, opts pgx.TxOptions) (*Tx, error) {
	tx, err := s.pool.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}

	return &Tx{
		tx: tx,
		storeOps: &storeOps{
			q:             s.q.WithTx(tx),
			projectFilter: s.projectFilter,
		},
	}, nil
}

// Commit commits the transaction.
func (tx *Tx) Commit(ctx context.Context) error {
	return tx.tx.Commit(ctx)
}

// Rollback rolls the transaction back.
func (tx *Tx) Rollback(ctx context.Context) error {
	return tx.tx.Rollback(ctx)
}

// Tx exposes the underlying pgx transaction.
func (tx *Tx) Tx() pgx.Tx {
	return tx.tx
}

func normalizeError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationSQLState {
		return ErrAlreadyExists
	}

	return err
}

func normalizeStaleCheckpointError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrStaleCheckpoint
	}
	return normalizeError(err)
}

func mapResult[T any](value T, err error) (T, error) {
	if err != nil {
		var zero T
		return zero, normalizeError(err)
	}
	return value, nil
}

func mapStaleCheckpointResult[T any](value T, err error) (T, error) {
	if err != nil {
		var zero T
		return zero, normalizeStaleCheckpointError(err)
	}
	return value, nil
}

func leaseDurationMicros(duration time.Duration) int64 {
	return duration.Microseconds()
}

func nullableWorkerID(workerID string) *string {
	if workerID == "" {
		return nil
	}
	return &workerID
}
