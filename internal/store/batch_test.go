package store_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

// =============================================================================
// Batch Idempotency Tests
// =============================================================================
// Tests for add-ingestion-pipeline/specs/idempotency/spec.md

func TestBatch_ClaimNewBatch(t *testing.T) {
	// Scenario: Claim new batch
	// GIVEN no batch with batch_key exists for the project
	// WHEN the ingest transaction begins
	// THEN a new ingest_batches record is created with status: "processing"
	// AND the batch ID is returned for subsequent operations

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	batchKey := "batch-new-" + uuid.New().String()[:8]

	batchID, err := q.ClaimBatch(ctx, platform.ClaimBatchParams{
		ProjectID: projectID,
		BatchKey:  batchKey,
	})
	require.NoError(t, err)

	assert.NotEqual(t, uuid.Nil, batchID, "should have batch ID")

	// Verify batch was created with correct status
	batch, err := q.GetBatchByKey(ctx, platform.GetBatchByKeyParams{
		ProjectID: projectID,
		BatchKey:  batchKey,
	})
	require.NoError(t, err)
	assert.Equal(t, "processing", batch.Status)
}

func TestBatch_ClaimDuplicateBatch(t *testing.T) {
	// Scenario: Claim duplicate batch
	// GIVEN a batch with batch_key already exists
	// WHEN the ingest transaction attempts to claim
	// THEN the INSERT returns no rows (ON CONFLICT DO NOTHING)

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	batchKey := "batch-dup-" + uuid.New().String()[:8]

	// First claim - should succeed
	batch1ID, err := q.ClaimBatch(ctx, platform.ClaimBatchParams{
		ProjectID: projectID,
		BatchKey:  batchKey,
	})
	require.NoError(t, err)
	assert.NotEqual(t, uuid.Nil, batch1ID)

	// Second claim - should fail with "no rows in result set"
	_, err = q.ClaimBatch(ctx, platform.ClaimBatchParams{
		ProjectID: projectID,
		BatchKey:  batchKey,
	})

	// ON CONFLICT DO NOTHING returns no rows for duplicates
	assert.Error(t, err, "duplicate claim should error (no rows)")
}

func TestBatch_DuplicateReturnsSuccess(t *testing.T) {
	// Scenario: Duplicate returns success (not error)
	// GIVEN batch was already processed
	// WHEN the same batch is submitted
	// THEN we can check for its existence

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	batchKey := "batch-success-" + uuid.New().String()[:8]

	// Claim the batch
	batchID, err := q.ClaimBatch(ctx, platform.ClaimBatchParams{
		ProjectID: projectID,
		BatchKey:  batchKey,
	})
	require.NoError(t, err)

	// Update status to accepted
	err = q.UpdateBatchStatus(ctx, platform.UpdateBatchStatusParams{
		ID:     batchID,
		Status: "accepted",
	})
	require.NoError(t, err)

	// Check if duplicate exists by looking up the batch
	existing, err := q.GetBatchByKey(ctx, platform.GetBatchByKeyParams{
		ProjectID: projectID,
		BatchKey:  batchKey,
	})
	require.NoError(t, err)

	// Should find the existing batch - this is what enables "duplicate" response
	assert.Equal(t, batchKey, existing.BatchKey)
	assert.Equal(t, "accepted", existing.Status)
}

func TestBatch_StatusUpdatedOnSuccess(t *testing.T) {
	// Scenario: Batch status updated on success
	// GIVEN a batch is being processed
	// WHEN all traces, spans, events are successfully upserted
	// THEN batch status is updated to "accepted"
	// AND processing_completed_at is set
	// AND trace_count, span_count, event_count reflect actual counts

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	batchKey := "batch-complete-" + uuid.New().String()[:8]

	// Claim batch
	batchID, err := q.ClaimBatch(ctx, platform.ClaimBatchParams{
		ProjectID: projectID,
		BatchKey:  batchKey,
	})
	require.NoError(t, err)

	// Update batch status with counts
	traceCount := int32(5)
	spanCount := int32(20)
	eventCount := int32(10)
	err = q.UpdateBatchStatus(ctx, platform.UpdateBatchStatusParams{
		ID:         batchID,
		Status:     "accepted",
		TraceCount: &traceCount,
		SpanCount:  &spanCount,
		EventCount: &eventCount,
	})
	require.NoError(t, err)

	// Verify update
	updated, err := q.GetBatchByKey(ctx, platform.GetBatchByKeyParams{
		ProjectID: projectID,
		BatchKey:  batchKey,
	})
	require.NoError(t, err)

	assert.Equal(t, "accepted", updated.Status)
	assert.Equal(t, int32(5), *updated.TraceCount)
	assert.Equal(t, int32(20), *updated.SpanCount)
	assert.Equal(t, int32(10), *updated.EventCount)
	assert.NotNil(t, updated.ProcessingCompletedAt)
}

func TestBatch_ProjectScopedUniqueness(t *testing.T) {
	// Scenario: Same batch_key different projects
	// GIVEN Project A has batch "batch-001"
	// AND Project B does not have batch "batch-001"
	// WHEN Project B submits batch "batch-001"
	// THEN the batch is accepted for Project B
	// AND both projects have independent records

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectAID := testutil.CreateTestProject(t, ctx, q)
	projectBID := testutil.CreateTestProject(t, ctx, q)
	batchKey := "batch-shared-key"

	// Claim batch for project A
	batchAID, err := q.ClaimBatch(ctx, platform.ClaimBatchParams{
		ProjectID: projectAID,
		BatchKey:  batchKey,
	})
	require.NoError(t, err)

	// Claim batch with SAME key for project B - should succeed
	batchBID, err := q.ClaimBatch(ctx, platform.ClaimBatchParams{
		ProjectID: projectBID,
		BatchKey:  batchKey,
	})
	require.NoError(t, err)

	// Both should exist independently
	assert.NotEqual(t, batchAID, batchBID)

	// Verify both batches exist
	batchA, err := q.GetBatchByKey(ctx, platform.GetBatchByKeyParams{
		ProjectID: projectAID,
		BatchKey:  batchKey,
	})
	require.NoError(t, err)
	assert.Equal(t, projectAID, batchA.ProjectID)

	batchB, err := q.GetBatchByKey(ctx, platform.GetBatchByKeyParams{
		ProjectID: projectBID,
		BatchKey:  batchKey,
	})
	require.NoError(t, err)
	assert.Equal(t, projectBID, batchB.ProjectID)
}

func TestBatch_SameKeyDetectsDuplicate(t *testing.T) {
	// Scenario: Same batch_key same project
	// GIVEN Project A has batch "batch-001"
	// WHEN Project A submits "batch-001" again
	// THEN duplicate is detected via GetBatchByKey

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	batchKey := "batch-detect-dup"

	// First submission
	_, err := q.ClaimBatch(ctx, platform.ClaimBatchParams{
		ProjectID: projectID,
		BatchKey:  batchKey,
	})
	require.NoError(t, err)

	// Check for existing batch before processing
	existing, err := q.GetBatchByKey(ctx, platform.GetBatchByKeyParams{
		ProjectID: projectID,
		BatchKey:  batchKey,
	})
	require.NoError(t, err)

	// Existing batch found - this enables duplicate detection
	assert.Equal(t, batchKey, existing.BatchKey)
	// At this point, service would return "duplicate" status
}

// =============================================================================
// Data Model Schema Tests
// =============================================================================
// Tests for add-ingestion-pipeline/specs/data-model/spec.md

func TestDataModel_TracesDualIDSystem(t *testing.T) {
	// Scenario: Dual ID system
	// GIVEN a trace is created via ingestion
	// WHEN the SDK provides trace_id: "my-trace-123"
	// THEN the trace has id (internal UUID) and trace_id (external TEXT)

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	externalID := "external-trace-id-123"

	trace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   externalID,
		Name:      testutil.StrPtr("Dual ID Test"),
	})
	require.NoError(t, err)

	// Internal UUID
	assert.NotEqual(t, uuid.Nil, trace.ID)

	// External ID
	assert.Equal(t, externalID, trace.TraceID)

	// Lookup by external ID
	found, err := q.GetTraceByExternalID(ctx, platform.GetTraceByExternalIDParams{
		ProjectID: projectID,
		TraceID:   externalID,
	})
	require.NoError(t, err)
	assert.Equal(t, trace.ID, found.ID)
}

func TestDataModel_UniqueConstraintProjectTraceID(t *testing.T) {
	// UNIQUE(project_id, trace_id) constraint prevents duplicates

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	traceID := "unique-constraint-test"

	// First insert
	trace1, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   traceID,
		Name:      testutil.StrPtr("First"),
	})
	require.NoError(t, err)

	// Second insert with same IDs - should update, not create new
	trace2, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   traceID,
		Name:      testutil.StrPtr("Updated"),
	})
	require.NoError(t, err)

	// Should be same internal ID (upsert, not duplicate)
	assert.Equal(t, trace1.ID, trace2.ID)
	assert.Equal(t, "Updated", *trace2.Name)
}

func TestDataModel_SpanParentSpanIDIsText(t *testing.T) {
	// parent_span_id is TEXT (not UUID FK)

	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	traceID := "trace-parent-test"

	trace, err := q.UpsertTrace(ctx, platform.UpsertTraceParams{
		ProjectID: projectID,
		TraceID:   traceID,
		Name:      testutil.StrPtr("Parent Test"),
	})
	require.NoError(t, err)

	// Parent span
	_, err = q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID: projectID,
		TraceID:   trace.ID,
		SpanID:    "parent-span",
		Name:      "Parent",
	})
	require.NoError(t, err)

	// Child span with parent_span_id as TEXT
	parentID := "parent-span"
	child, err := q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID:    projectID,
		TraceID:      trace.ID,
		SpanID:       "child-span",
		Name:         "Child",
		ParentSpanID: &parentID,
	})
	require.NoError(t, err)

	// parent_span_id should be the external TEXT ID
	require.NotNil(t, child.ParentSpanID)
	assert.Equal(t, "parent-span", *child.ParentSpanID)
}
