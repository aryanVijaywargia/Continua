package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

func TestBackfillEngineTraceLineage_PopulatesRootAndChildColumns(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	engineQueries := enginedb.New(pool)

	projectID := testutil.CreateTestProject(t, ctx, s.Queries())

	rootInstance, err := engineQueries.CreateInstance(ctx, enginedb.CreateInstanceParams{
		ProjectID:      projectID,
		InstanceKey:    "instance-root-lineage",
		DefinitionName: "checkout",
	})
	require.NoError(t, err)
	rootRun, err := engineQueries.CreateRun(ctx, enginedb.CreateRunParams{
		ProjectID:         projectID,
		InstanceID:        rootInstance.ID,
		RunNumber:         1,
		DefinitionVersion: "v1",
		ReadyAt:           time.Now().UTC(),
	})
	require.NoError(t, err)

	childInstance, err := engineQueries.CreateInstance(ctx, enginedb.CreateInstanceParams{
		ProjectID:      projectID,
		InstanceKey:    "instance-child-lineage",
		DefinitionName: "charge-card",
	})
	require.NoError(t, err)
	childKey := "charge-card"
	childRun, err := engineQueries.CreateChildRun(ctx, enginedb.CreateChildRunParams{
		ProjectID:         projectID,
		InstanceID:        childInstance.ID,
		RunNumber:         1,
		DefinitionVersion: "v2",
		ReadyAt:           time.Now().UTC(),
		ParentRunID:       pgtype.UUID{Bytes: rootRun.ID, Valid: true},
		RootRunID:         rootRun.ID,
		ChildKey:          &childKey,
		ChildDepth:        1,
	})
	require.NoError(t, err)
	_, err = engineQueries.CreateChildWorkflow(ctx, enginedb.CreateChildWorkflowParams{
		ProjectID:                  projectID,
		ParentInstanceID:           rootInstance.ID,
		ParentRunID:                rootRun.ID,
		ChildKey:                   childKey,
		RequestedDefinitionName:    "charge-card",
		RequestedDefinitionVersion: "v2",
		ChildInstanceID:            childInstance.ID,
		ChildInstanceKey:           childInstance.InstanceKey,
		CurrentChildRunID:          childRun.ID,
		RootRunID:                  rootRun.ID,
		ChildDepth:                 1,
	})
	require.NoError(t, err)

	rootTrace, err := s.Queries().UpsertTrace(ctx, platformdb.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "engine:" + rootRun.ID.String(),
		Name:      testutil.StrPtr("root trace"),
		Status:    "running",
		StartTime: testutil.PgtypeTimestamptz(time.Now().UTC()),
	})
	require.NoError(t, err)
	childTrace, err := s.Queries().UpsertTrace(ctx, platformdb.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   "engine:" + childRun.ID.String(),
		Name:      testutil.StrPtr("child trace"),
		Status:    "running",
		StartTime: testutil.PgtypeTimestamptz(time.Now().UTC()),
	})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		UPDATE traces
		SET engine_run_id = $2
		WHERE id = $1
	`, rootTrace.ID, rootRun.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		UPDATE traces
		SET engine_run_id = $2
		WHERE id = $1
	`, childTrace.ID, childRun.ID)
	require.NoError(t, err)

	tx, err := s.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	defer tx.Rollback(ctx)

	require.NoError(t, tx.BackfillEngineTraceLineage(ctx, rootTrace.ID, &rootRun))
	require.NoError(t, tx.BackfillEngineTraceLineage(ctx, childTrace.ID, &childRun))
	require.NoError(t, tx.Commit(ctx))

	var (
		rootParent pgtype.UUID
		rootRoot   pgtype.UUID
		rootKey    *string
		rootDepth  *int32
	)
	err = pool.QueryRow(ctx, `
		SELECT engine_parent_run_id, engine_root_run_id, engine_child_key, engine_child_depth
		FROM traces
		WHERE id = $1
	`, rootTrace.ID).Scan(&rootParent, &rootRoot, &rootKey, &rootDepth)
	require.NoError(t, err)
	require.False(t, rootParent.Valid)
	require.True(t, rootRoot.Valid)
	require.Equal(t, rootRun.ID, uuid.UUID(rootRoot.Bytes))
	require.Nil(t, rootKey)
	require.NotNil(t, rootDepth)
	require.EqualValues(t, 0, *rootDepth)

	var (
		childParent  pgtype.UUID
		childRoot    pgtype.UUID
		childKeyDB   *string
		childDepthDB *int32
	)
	err = pool.QueryRow(ctx, `
		SELECT engine_parent_run_id, engine_root_run_id, engine_child_key, engine_child_depth
		FROM traces
		WHERE id = $1
	`, childTrace.ID).Scan(&childParent, &childRoot, &childKeyDB, &childDepthDB)
	require.NoError(t, err)
	require.True(t, childParent.Valid)
	require.Equal(t, rootRun.ID, uuid.UUID(childParent.Bytes))
	require.True(t, childRoot.Valid)
	require.Equal(t, rootRun.ID, uuid.UUID(childRoot.Bytes))
	require.NotNil(t, childKeyDB)
	require.Equal(t, childKey, *childKeyDB)
	require.NotNil(t, childDepthDB)
	require.EqualValues(t, 1, *childDepthDB)

	_, err = pool.Exec(ctx, `
		UPDATE engine.runs
		SET parent_run_id = NULL,
		    root_run_id = id,
		    child_key = NULL,
		    child_depth = 0
		WHERE id = $1
	`, childRun.ID)
	require.NoError(t, err)
	driftedChildRun, err := engineQueries.GetRun(ctx, childRun.ID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `
		UPDATE traces
		SET engine_parent_run_id = NULL,
		    engine_root_run_id = $2,
		    engine_child_key = NULL,
		    engine_child_depth = 0
		WHERE id = $1
	`, childTrace.ID, childRun.ID)
	require.NoError(t, err)

	tx, err = s.BeginTx(ctx, pgx.TxOptions{})
	require.NoError(t, err)
	defer tx.Rollback(ctx)

	require.NoError(t, tx.BackfillEngineTraceLineage(ctx, childTrace.ID, &driftedChildRun))
	require.NoError(t, tx.Commit(ctx))

	var (
		repairedParent  pgtype.UUID
		repairedRoot    pgtype.UUID
		repairedKey     *string
		repairedDepthDB *int32
	)
	err = pool.QueryRow(ctx, `
		SELECT engine_parent_run_id, engine_root_run_id, engine_child_key, engine_child_depth
		FROM traces
		WHERE id = $1
	`, childTrace.ID).Scan(&repairedParent, &repairedRoot, &repairedKey, &repairedDepthDB)
	require.NoError(t, err)
	require.True(t, repairedParent.Valid)
	require.Equal(t, rootRun.ID, uuid.UUID(repairedParent.Bytes))
	require.True(t, repairedRoot.Valid)
	require.Equal(t, rootRun.ID, uuid.UUID(repairedRoot.Bytes))
	require.NotNil(t, repairedKey)
	require.Equal(t, childKey, *repairedKey)
	require.NotNil(t, repairedDepthDB)
	require.EqualValues(t, 1, *repairedDepthDB)
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
