package ingest

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRetryOnConflict(t *testing.T) {
	t.Parallel()

	deadlock := fmt.Errorf("failed to upsert trace a: %w", &pgconn.PgError{Code: sqlStateDeadlock})
	serialization := &pgconn.PgError{Code: sqlStateSerializationFailure}
	other := errors.New("connection reset")

	tests := []struct {
		name      string
		errs      []error // returned by successive attempts; later attempts succeed
		canceled  bool
		wantCalls int
		check     func(t *testing.T, err error)
	}{
		{
			name: "retries a wrapped deadlock until the attempt succeeds", errs: []error{deadlock}, wantCalls: 2,
			check: func(t *testing.T, err error) { require.NoError(t, err) },
		},
		{
			name: "returns any other error at once", errs: []error{other}, wantCalls: 1,
			check: func(t *testing.T, err error) {
				require.ErrorIs(t, err, other)
				assert.NotErrorIs(t, err, ErrWriteConflict)
			},
		},
		{
			name: "gives up after the last attempt", errs: []error{serialization, serialization, serialization},
			wantCalls: conflictAttempts,
			check: func(t *testing.T, err error) {
				require.ErrorIs(t, err, ErrWriteConflict)
				var pgErr *pgconn.PgError
				require.ErrorAs(t, err, &pgErr, "the last database error stays in the chain")
				assert.Equal(t, sqlStateSerializationFailure, pgErr.Code)
			},
		},
		{
			name: "stops backing off when the context ends", errs: []error{deadlock}, canceled: true, wantCalls: 1,
			check: func(t *testing.T, err error) {
				require.ErrorIs(t, err, context.Canceled)
				assert.NotErrorIs(t, err, ErrWriteConflict)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.canceled {
				cancel()
			}

			calls := 0
			err := retryOnConflict(ctx, uuid.New(), "batch", func() error {
				calls++
				if calls <= len(tt.errs) {
					return tt.errs[calls-1]
				}
				return nil
			})

			assert.Equal(t, tt.wantCalls, calls)
			tt.check(t, err)
		})
	}
}
