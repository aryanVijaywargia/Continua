package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	platformdb "github.com/continua-ai/continua/db/gen/go/platform"
	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

func TestListProjectionRetentionCandidates_IncludesContinuedAsNewRuns(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	engineQueries := enginedb.New(pool)

	projectID := testutil.CreateTestProject(t, ctx, s.Queries())
	instance, err := engineQueries.CreateInstance(ctx, enginedb.CreateInstanceParams{
		ProjectID:      projectID,
		InstanceKey:    "instance-retention-continued",
		DefinitionName: "checkout",
	})
	require.NoError(t, err)

	completedAt := time.Now().UTC().Add(-2 * time.Hour).Round(time.Microsecond)
	run, err := engineQueries.CreateRun(ctx, enginedb.CreateRunParams{
		ProjectID:          projectID,
		InstanceID:         instance.ID,
		RunNumber:          1,
		DefinitionVersion:  "v1",
		ReadyAt:            completedAt.Add(-time.Minute),
		ContinuedFromRunID: pgtype.UUID{},
	})
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		UPDATE engine.runs
		SET status = 'continued_as_new',
		    completed_at = $2,
		    updated_at = $2
		WHERE id = $1
	`, run.ID, completedAt)
	require.NoError(t, err)

	trace, err := s.Queries().UpsertTrace(ctx, platformdb.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "engine:" + run.ID.String(),
		Name:      testutil.StrPtr("retention trace"),
		Status:    "completed",
		StartTime: testutil.PgtypeTimestamptz(completedAt.Add(-time.Minute)),
		EndTime:   testutil.PgtypeTimestamptz(completedAt),
	})
	require.NoError(t, err)

	_, err = pool.Exec(ctx, `
		UPDATE traces
		SET engine_run_id = $2,
		    engine_instance_key = 'instance-retention-continued',
		    engine_definition_name = 'checkout',
		    engine_definition_version = 'v1',
		    engine_run_status = 'continued_as_new',
		    engine_projection_state = 'up_to_date',
		    engine_projection_updated_at = $3,
		    updated_at = $3
		WHERE id = $1
	`, trace.ID, run.ID, completedAt)
	require.NoError(t, err)

	projectionCandidates, err := s.ListProjectionRetentionCandidates(ctx, time.Now().UTC(), 10)
	require.NoError(t, err)
	require.True(t, containsEngineRetentionCandidate(projectionCandidates, projectID, run.ID, trace.ID))

	historyCandidates, err := s.ListHistoryRetentionCandidates(ctx, time.Now().UTC(), 10)
	require.NoError(t, err)
	require.True(t, containsEngineRetentionCandidate(historyCandidates, projectID, run.ID, trace.ID))
}

func containsEngineRetentionCandidate(
	candidates []store.EngineRetentionCandidate,
	projectID uuid.UUID,
	runID uuid.UUID,
	traceID uuid.UUID,
) bool {
	for i := range candidates {
		candidate := candidates[i]
		if candidate.ProjectID == projectID && candidate.RunID == runID && candidate.TraceID == traceID {
			return true
		}
	}
	return false
}
