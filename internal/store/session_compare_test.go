package store_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/db/gen/go/platform"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

func TestBuildSessionComparison_ZeroSpanAndSemanticEventTraces(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session := createNarrativeSession(ctx, t, q, projectID, "compare-empty")
	base := time.Date(2026, 3, 26, 9, 0, 0, 0, time.UTC)

	baseline := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-baseline", "Baseline", "completed", base, timePtr(base.Add(time.Minute)), nil, 10, 5, 0.01, 0)
	candidate := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-candidate", "Candidate", "completed", base.Add(2*time.Minute), timePtr(base.Add(4*time.Minute)), nil, 12, 6, 0.02, 0)

	comparison, err := s.BuildSessionComparison(ctx, projectID, session.ID, baseline.ID, candidate.ID)
	require.NoError(t, err)

	assert.Equal(t, 0, comparison.Summary.TotalSpansBaseline)
	assert.Equal(t, 0, comparison.Summary.TotalSpansCandidate)
	assert.Equal(t, 0, comparison.Summary.TotalSemanticBaseline)
	assert.Equal(t, 0, comparison.Summary.TotalSemanticCandidate)
	assert.Equal(t, int64(60_000), comparison.Summary.DurationDeltaMs)
	assert.Equal(t, int64(2), comparison.Summary.TokensInDelta)
	assert.Equal(t, int64(1), comparison.Summary.TokensOutDelta)
	assert.InDelta(t, 0.01, comparison.Summary.CostDeltaUsd, 0.0001)
	assert.Empty(t, comparison.SpanDiffs)
}

func TestBuildSessionComparison_MatchesExactSpanIDsAndComputesChangedFields(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session := createNarrativeSession(ctx, t, q, projectID, "compare-stable")
	base := time.Date(2026, 3, 26, 10, 0, 0, 0, time.UTC)

	baseline := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-baseline", "Baseline", "completed", base, timePtr(base.Add(2*time.Minute)), nil, 100, 40, 0.11, 0)
	candidate := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-candidate", "Candidate", "completed", base.Add(3*time.Minute), timePtr(base.Add(6*time.Minute)), nil, 120, 55, 0.13, 0)

	createCompareSpan(ctx, t, q, projectID, baseline.ID, "root", nil, "Plan", "agent", base, timePtr(base.Add(20*time.Second)), int32Ptr(1), 10, 5, 0.01)
	createCompareSpan(ctx, t, q, projectID, candidate.ID, "root", nil, "Plan", "agent", base.Add(3*time.Minute), timePtr(base.Add(3*time.Minute+30*time.Second)), int32Ptr(1), 12, 8, 0.02)

	comparison, err := s.BuildSessionComparison(ctx, projectID, session.ID, baseline.ID, candidate.ID)
	require.NoError(t, err)
	require.Len(t, comparison.SpanDiffs, 1)

	row := comparison.SpanDiffs[0]
	assert.Equal(t, store.SessionCompareDiffStatusChanged, row.DiffStatus)
	require.NotNil(t, row.MatchSource)
	assert.Equal(t, store.SessionCompareMatchSourceStableID, *row.MatchSource)
	assert.Equal(t, []string{"latency_ms", "tokens_in", "tokens_out", "cost_usd"}, row.ChangedFields)
	require.NotNil(t, row.BaselineSpan)
	require.NotNil(t, row.CandidateSpan)
	assert.Equal(t, "root", row.BaselineSpan.SpanID)
	assert.Equal(t, "root", row.CandidateSpan.SpanID)
	assert.Equal(t, 1, comparison.Summary.MatchedSpans)
	assert.Equal(t, 0, comparison.Summary.HeuristicMatches)
}

func TestBuildSessionComparison_HeuristicMatchesRepeatedSiblingsByOrdinal(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session := createNarrativeSession(ctx, t, q, projectID, "compare-heuristic")
	base := time.Date(2026, 3, 26, 11, 0, 0, 0, time.UTC)

	baseline := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-baseline", "Baseline", "completed", base, timePtr(base.Add(time.Minute)), nil, 0, 0, 0, 0)
	candidate := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-candidate", "Candidate", "completed", base.Add(2*time.Minute), timePtr(base.Add(3*time.Minute)), nil, 0, 0, 0, 0)

	createCompareSpan(ctx, t, q, projectID, baseline.ID, "root-shared", nil, "Root", "agent", base, timePtr(base.Add(10*time.Second)), int32Ptr(1), 0, 0, 0)
	createCompareSpan(ctx, t, q, projectID, candidate.ID, "root-shared", nil, "Root", "agent", base.Add(2*time.Minute), timePtr(base.Add(2*time.Minute+10*time.Second)), int32Ptr(1), 0, 0, 0)

	createCompareSpan(ctx, t, q, projectID, baseline.ID, "baseline-child", strPtr("root-shared"), "Model Call", "llm", base.Add(11*time.Second), timePtr(base.Add(20*time.Second)), int32Ptr(2), 0, 0, 0)
	createCompareSpan(ctx, t, q, projectID, candidate.ID, "candidate-child", strPtr("root-shared"), "model   call", "llm", base.Add(2*time.Minute+11*time.Second), timePtr(base.Add(2*time.Minute+20*time.Second)), int32Ptr(2), 0, 0, 0)

	createCompareSpan(ctx, t, q, projectID, baseline.ID, "baseline-ambiguous", strPtr("root-shared"), "Lookup", "tool", base.Add(21*time.Second), timePtr(base.Add(30*time.Second)), int32Ptr(3), 0, 0, 0)
	createCompareSpan(ctx, t, q, projectID, candidate.ID, "candidate-ambiguous-a", strPtr("root-shared"), "Lookup", "tool", base.Add(2*time.Minute+21*time.Second), timePtr(base.Add(2*time.Minute+30*time.Second)), int32Ptr(3), 0, 0, 0)
	createCompareSpan(ctx, t, q, projectID, candidate.ID, "candidate-ambiguous-b", strPtr("root-shared"), "Lookup", "tool", base.Add(2*time.Minute+31*time.Second), timePtr(base.Add(2*time.Minute+40*time.Second)), int32Ptr(4), 0, 0, 0)

	comparison, err := s.BuildSessionComparison(ctx, projectID, session.ID, baseline.ID, candidate.ID)
	require.NoError(t, err)
	require.Len(t, comparison.SpanDiffs, 4)

	assert.Equal(t, 3, comparison.Summary.MatchedSpans)
	assert.Equal(t, 2, comparison.Summary.HeuristicMatches)
	assert.Equal(t, 0, comparison.Summary.UnmatchedBaselineSpans)
	assert.Equal(t, 1, comparison.Summary.UnmatchedCandidateSpans)

	assert.Equal(t, store.SessionCompareDiffStatusUnchanged, comparison.SpanDiffs[0].DiffStatus)
	require.NotNil(t, comparison.SpanDiffs[1].MatchSource)
	assert.Equal(t, store.SessionCompareMatchSourceHeuristic, *comparison.SpanDiffs[1].MatchSource)
	assert.Equal(t, "baseline-child", comparison.SpanDiffs[1].BaselineSpan.SpanID)
	assert.Equal(t, "candidate-child", comparison.SpanDiffs[1].CandidateSpan.SpanID)
	require.NotNil(t, comparison.SpanDiffs[2].MatchSource)
	assert.Equal(t, store.SessionCompareMatchSourceHeuristic, *comparison.SpanDiffs[2].MatchSource)
	assert.Equal(t, "baseline-ambiguous", comparison.SpanDiffs[2].BaselineSpan.SpanID)
	assert.Equal(t, "candidate-ambiguous-a", comparison.SpanDiffs[2].CandidateSpan.SpanID)
	assert.Equal(t, store.SessionCompareDiffStatusCandidateOnly, comparison.SpanDiffs[3].DiffStatus)
	assert.Equal(t, "candidate-ambiguous-b", comparison.SpanDiffs[3].CandidateSpan.SpanID)
}

func TestBuildSessionComparison_DoesNotHeuristicallyMatchAcrossUnmatchedParentBranches(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session := createNarrativeSession(ctx, t, q, projectID, "compare-unmatched-parent-scope")
	base := time.Date(2026, 3, 26, 11, 30, 0, 0, time.UTC)

	baseline := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-baseline", "Baseline", "completed", base, timePtr(base.Add(time.Minute)), nil, 0, 0, 0, 0)
	candidate := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-candidate", "Candidate", "completed", base.Add(2*time.Minute), timePtr(base.Add(3*time.Minute)), nil, 0, 0, 0, 0)

	createCompareSpan(ctx, t, q, projectID, baseline.ID, "root-shared", nil, "Root", "agent", base, timePtr(base.Add(10*time.Second)), int32Ptr(1), 0, 0, 0)
	createCompareSpan(ctx, t, q, projectID, candidate.ID, "root-shared", nil, "Root", "agent", base.Add(2*time.Minute), timePtr(base.Add(2*time.Minute+10*time.Second)), int32Ptr(1), 0, 0, 0)

	createCompareSpan(ctx, t, q, projectID, baseline.ID, "baseline-branch", strPtr("root-shared"), "Tools A", "chain", base.Add(11*time.Second), timePtr(base.Add(20*time.Second)), int32Ptr(2), 0, 0, 0)
	createCompareSpan(ctx, t, q, projectID, candidate.ID, "candidate-branch", strPtr("root-shared"), "Tools B", "chain", base.Add(2*time.Minute+11*time.Second), timePtr(base.Add(2*time.Minute+20*time.Second)), int32Ptr(2), 0, 0, 0)

	createCompareSpan(ctx, t, q, projectID, baseline.ID, "baseline-grandchild", strPtr("baseline-branch"), "Lookup", "tool", base.Add(21*time.Second), timePtr(base.Add(30*time.Second)), int32Ptr(3), 0, 0, 0)
	createCompareSpan(ctx, t, q, projectID, candidate.ID, "candidate-grandchild", strPtr("candidate-branch"), "Lookup", "tool", base.Add(2*time.Minute+21*time.Second), timePtr(base.Add(2*time.Minute+30*time.Second)), int32Ptr(3), 0, 0, 0)

	comparison, err := s.BuildSessionComparison(ctx, projectID, session.ID, baseline.ID, candidate.ID)
	require.NoError(t, err)
	require.Len(t, comparison.SpanDiffs, 5)

	assert.Equal(t, 1, comparison.Summary.MatchedSpans)
	assert.Equal(t, 0, comparison.Summary.HeuristicMatches)
	assert.Equal(t, 2, comparison.Summary.UnmatchedBaselineSpans)
	assert.Equal(t, 2, comparison.Summary.UnmatchedCandidateSpans)

	assert.Equal(t, store.SessionCompareDiffStatusUnchanged, comparison.SpanDiffs[0].DiffStatus)
	assert.Equal(t, "root-shared", comparison.SpanDiffs[0].BaselineSpan.SpanID)

	assert.Equal(t, store.SessionCompareDiffStatusBaselineOnly, comparison.SpanDiffs[1].DiffStatus)
	assert.Equal(t, "baseline-branch", comparison.SpanDiffs[1].BaselineSpan.SpanID)
	assert.Equal(t, store.SessionCompareDiffStatusBaselineOnly, comparison.SpanDiffs[2].DiffStatus)
	assert.Equal(t, "baseline-grandchild", comparison.SpanDiffs[2].BaselineSpan.SpanID)

	assert.Equal(t, store.SessionCompareDiffStatusCandidateOnly, comparison.SpanDiffs[3].DiffStatus)
	assert.Equal(t, "candidate-branch", comparison.SpanDiffs[3].CandidateSpan.SpanID)
	assert.Equal(t, store.SessionCompareDiffStatusCandidateOnly, comparison.SpanDiffs[4].DiffStatus)
	assert.Equal(t, "candidate-grandchild", comparison.SpanDiffs[4].CandidateSpan.SpanID)
}

func TestBuildSessionComparison_DecisionPairingLeavesChangedAndInvalidQuestionsUnpaired(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session := createNarrativeSession(ctx, t, q, projectID, "compare-decision-edges")
	base := time.Date(2026, 3, 26, 12, 30, 0, 0, time.UTC)

	baseline := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-baseline", "Baseline", "completed", base, timePtr(base.Add(time.Minute)), nil, 0, 0, 0, 0)
	candidate := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-candidate", "Candidate", "completed", base.Add(2*time.Minute), timePtr(base.Add(3*time.Minute)), nil, 0, 0, 0, 0)

	createCompareSpan(ctx, t, q, projectID, baseline.ID, "shared", nil, "Shared", "tool", base, timePtr(base.Add(20*time.Second)), int32Ptr(1), 0, 0, 0)
	createCompareSpan(ctx, t, q, projectID, candidate.ID, "shared", nil, "Shared", "tool", base.Add(2*time.Minute), timePtr(base.Add(2*time.Minute+20*time.Second)), int32Ptr(1), 0, 0, 0)

	createCompareEvent(ctx, t, pool, q, projectID, baseline.ID, "decision", base.Add(time.Second), int32Ptr(1), "unique baseline", map[string]any{"question": "Choose route?", "chosen": "A"})
	createCompareEvent(ctx, t, pool, q, projectID, candidate.ID, "decision", base.Add(2*time.Minute+time.Second), int32Ptr(1), "unique candidate", map[string]any{"question": " choose route? ", "chosen": "B"})
	createCompareEvent(ctx, t, pool, q, projectID, baseline.ID, "decision", base.Add(2*time.Second), int32Ptr(2), "changed question baseline", map[string]any{"question": "Old route?"})
	createCompareEvent(ctx, t, pool, q, projectID, candidate.ID, "decision", base.Add(2*time.Minute+2*time.Second), int32Ptr(2), "changed question candidate", map[string]any{"question": "New route?"})
	createCompareEvent(ctx, t, pool, q, projectID, baseline.ID, "decision", base.Add(3*time.Second), int32Ptr(3), "blank question", map[string]any{"question": "   "})
	createCompareEvent(ctx, t, pool, q, projectID, candidate.ID, "decision", base.Add(2*time.Minute+3*time.Second), int32Ptr(3), "non-string question", map[string]any{"question": 7})
	createCompareEvent(ctx, t, pool, q, projectID, baseline.ID, "decision", base.Add(4*time.Second), int32Ptr(4), "baseline duplicate one", map[string]any{"question": "Duplicate"})
	createCompareEvent(ctx, t, pool, q, projectID, baseline.ID, "decision", base.Add(5*time.Second), int32Ptr(5), "baseline duplicate two", map[string]any{"question": "duplicate"})
	createCompareEvent(ctx, t, pool, q, projectID, candidate.ID, "decision", base.Add(2*time.Minute+4*time.Second), int32Ptr(4), "candidate duplicate one", map[string]any{"question": "duplicate"})
	createCompareEvent(ctx, t, pool, q, projectID, candidate.ID, "decision", base.Add(2*time.Minute+5*time.Second), int32Ptr(5), "candidate duplicate two", map[string]any{"question": "duplicate"})

	comparison, err := s.BuildSessionComparison(ctx, projectID, session.ID, baseline.ID, candidate.ID)
	require.NoError(t, err)
	require.Len(t, comparison.SpanDiffs, 1)

	groups := comparison.SpanDiffs[0].SemanticGroups

	unique := findSemanticGroupByMessage(t, groups, "unique baseline")
	require.NotNil(t, unique.MatchSource)
	assert.Equal(t, store.SessionCompareMatchSourceHeuristic, *unique.MatchSource)
	assert.Equal(t, store.SessionCompareDiffStatusChanged, unique.DiffStatus)

	assert.Equal(t, store.SessionCompareDiffStatusBaselineOnly, findSemanticGroupByMessage(t, groups, "changed question baseline").DiffStatus)
	assert.Equal(t, store.SessionCompareDiffStatusCandidateOnly, findSemanticGroupByMessage(t, groups, "changed question candidate").DiffStatus)
	assert.Equal(t, store.SessionCompareDiffStatusBaselineOnly, findSemanticGroupByMessage(t, groups, "blank question").DiffStatus)
	assert.Equal(t, store.SessionCompareDiffStatusCandidateOnly, findSemanticGroupByMessage(t, groups, "non-string question").DiffStatus)
	assert.Equal(t, store.SessionCompareDiffStatusBaselineOnly, findSemanticGroupByMessage(t, groups, "baseline duplicate one").DiffStatus)
	assert.Equal(t, store.SessionCompareDiffStatusBaselineOnly, findSemanticGroupByMessage(t, groups, "baseline duplicate two").DiffStatus)
	assert.Equal(t, store.SessionCompareDiffStatusCandidateOnly, findSemanticGroupByMessage(t, groups, "candidate duplicate one").DiffStatus)
	assert.Equal(t, store.SessionCompareDiffStatusCandidateOnly, findSemanticGroupByMessage(t, groups, "candidate duplicate two").DiffStatus)
}

func TestBuildSessionComparison_EffectPairingLeavesDuplicateAndMissingIDsUnpaired(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session := createNarrativeSession(ctx, t, q, projectID, "compare-effect-edges")
	base := time.Date(2026, 3, 26, 12, 45, 0, 0, time.UTC)

	baseline := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-baseline", "Baseline", "completed", base, timePtr(base.Add(time.Minute)), nil, 0, 0, 0, 0)
	candidate := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-candidate", "Candidate", "completed", base.Add(2*time.Minute), timePtr(base.Add(3*time.Minute)), nil, 0, 0, 0, 0)

	createCompareSpan(ctx, t, q, projectID, baseline.ID, "shared", nil, "Shared", "tool", base, timePtr(base.Add(20*time.Second)), int32Ptr(1), 0, 0, 0)
	createCompareSpan(ctx, t, q, projectID, candidate.ID, "shared", nil, "Shared", "tool", base.Add(2*time.Minute), timePtr(base.Add(2*time.Minute+20*time.Second)), int32Ptr(1), 0, 0, 0)

	createCompareEvent(ctx, t, pool, q, projectID, baseline.ID, "effect", base.Add(time.Second), int32Ptr(1), "unique effect baseline", map[string]any{"effect_id": "effect-1", "effect_kind": "http"})
	createCompareEvent(ctx, t, pool, q, projectID, candidate.ID, "effect", base.Add(2*time.Minute+time.Second), int32Ptr(1), "unique effect candidate", map[string]any{"effect_id": "effect-1", "effect_kind": "http", "idempotent": true})
	createCompareEvent(ctx, t, pool, q, projectID, baseline.ID, "effect", base.Add(2*time.Second), int32Ptr(2), "missing effect id", map[string]any{"effect_kind": "side-effect"})
	createCompareEvent(ctx, t, pool, q, projectID, baseline.ID, "effect", base.Add(3*time.Second), int32Ptr(3), "baseline duplicate effect one", map[string]any{"effect_id": "effect-dup"})
	createCompareEvent(ctx, t, pool, q, projectID, baseline.ID, "effect", base.Add(4*time.Second), int32Ptr(4), "baseline duplicate effect two", map[string]any{"effect_id": "effect-dup"})
	createCompareEvent(ctx, t, pool, q, projectID, candidate.ID, "effect", base.Add(2*time.Minute+3*time.Second), int32Ptr(3), "candidate duplicate effect one", map[string]any{"effect_id": "effect-dup"})
	createCompareEvent(ctx, t, pool, q, projectID, candidate.ID, "effect", base.Add(2*time.Minute+4*time.Second), int32Ptr(4), "candidate duplicate effect two", map[string]any{"effect_id": "effect-dup"})

	comparison, err := s.BuildSessionComparison(ctx, projectID, session.ID, baseline.ID, candidate.ID)
	require.NoError(t, err)

	groups := comparison.SpanDiffs[0].SemanticGroups
	unique := findSemanticGroupByMessage(t, groups, "unique effect baseline")
	require.NotNil(t, unique.MatchSource)
	assert.Equal(t, store.SessionCompareMatchSourceStableID, *unique.MatchSource)
	assert.Equal(t, store.SessionCompareDiffStatusChanged, unique.DiffStatus)
	assert.Equal(t, store.SessionCompareDiffStatusBaselineOnly, findSemanticGroupByMessage(t, groups, "missing effect id").DiffStatus)
	assert.Equal(t, store.SessionCompareDiffStatusBaselineOnly, findSemanticGroupByMessage(t, groups, "baseline duplicate effect one").DiffStatus)
	assert.Equal(t, store.SessionCompareDiffStatusBaselineOnly, findSemanticGroupByMessage(t, groups, "baseline duplicate effect two").DiffStatus)
	assert.Equal(t, store.SessionCompareDiffStatusCandidateOnly, findSemanticGroupByMessage(t, groups, "candidate duplicate effect one").DiffStatus)
	assert.Equal(t, store.SessionCompareDiffStatusCandidateOnly, findSemanticGroupByMessage(t, groups, "candidate duplicate effect two").DiffStatus)
}

func TestBuildSessionComparison_WaitPairingLeavesDuplicateAndMissingIDsUnpaired(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session := createNarrativeSession(ctx, t, q, projectID, "compare-wait-edges")
	base := time.Date(2026, 3, 26, 13, 15, 0, 0, time.UTC)

	baseline := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-baseline", "Baseline", "completed", base, timePtr(base.Add(time.Minute)), nil, 0, 0, 0, 0)
	candidate := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-candidate", "Candidate", "completed", base.Add(2*time.Minute), timePtr(base.Add(3*time.Minute)), nil, 0, 0, 0, 0)

	createCompareSpan(ctx, t, q, projectID, baseline.ID, "shared", nil, "Shared", "tool", base, timePtr(base.Add(20*time.Second)), int32Ptr(1), 0, 0, 0)
	createCompareSpan(ctx, t, q, projectID, candidate.ID, "shared", nil, "Shared", "tool", base.Add(2*time.Minute), timePtr(base.Add(2*time.Minute+20*time.Second)), int32Ptr(1), 0, 0, 0)

	createCompareEvent(ctx, t, pool, q, projectID, baseline.ID, "wait", base.Add(time.Second), int32Ptr(1), "unique wait baseline", map[string]any{"wait_id": "wait-1", "resolution": "timeout"})
	createCompareEvent(ctx, t, pool, q, projectID, candidate.ID, "wait", base.Add(2*time.Minute+time.Second), int32Ptr(1), "unique wait candidate", map[string]any{"wait_id": "wait-1", "resolution": "completed"})
	createCompareEvent(ctx, t, pool, q, projectID, candidate.ID, "wait", base.Add(2*time.Minute+2*time.Second), int32Ptr(2), "missing wait id", map[string]any{"resolution": "timeout"})
	createCompareEvent(ctx, t, pool, q, projectID, baseline.ID, "wait", base.Add(3*time.Second), int32Ptr(3), "baseline duplicate wait one", map[string]any{"wait_id": "wait-dup"})
	createCompareEvent(ctx, t, pool, q, projectID, baseline.ID, "wait", base.Add(4*time.Second), int32Ptr(4), "baseline duplicate wait two", map[string]any{"wait_id": "wait-dup"})
	createCompareEvent(ctx, t, pool, q, projectID, candidate.ID, "wait", base.Add(2*time.Minute+3*time.Second), int32Ptr(3), "candidate duplicate wait one", map[string]any{"wait_id": "wait-dup"})
	createCompareEvent(ctx, t, pool, q, projectID, candidate.ID, "wait", base.Add(2*time.Minute+4*time.Second), int32Ptr(4), "candidate duplicate wait two", map[string]any{"wait_id": "wait-dup"})

	comparison, err := s.BuildSessionComparison(ctx, projectID, session.ID, baseline.ID, candidate.ID)
	require.NoError(t, err)

	groups := comparison.SpanDiffs[0].SemanticGroups
	unique := findSemanticGroupByMessage(t, groups, "unique wait baseline")
	require.NotNil(t, unique.MatchSource)
	assert.Equal(t, store.SessionCompareMatchSourceStableID, *unique.MatchSource)
	assert.Equal(t, store.SessionCompareDiffStatusChanged, unique.DiffStatus)
	assert.Equal(t, store.SessionCompareDiffStatusCandidateOnly, findSemanticGroupByMessage(t, groups, "missing wait id").DiffStatus)
	assert.Equal(t, store.SessionCompareDiffStatusBaselineOnly, findSemanticGroupByMessage(t, groups, "baseline duplicate wait one").DiffStatus)
	assert.Equal(t, store.SessionCompareDiffStatusBaselineOnly, findSemanticGroupByMessage(t, groups, "baseline duplicate wait two").DiffStatus)
	assert.Equal(t, store.SessionCompareDiffStatusCandidateOnly, findSemanticGroupByMessage(t, groups, "candidate duplicate wait one").DiffStatus)
	assert.Equal(t, store.SessionCompareDiffStatusCandidateOnly, findSemanticGroupByMessage(t, groups, "candidate duplicate wait two").DiffStatus)
}

func TestBuildSessionComparison_EffectAndWaitPairingRequireExactVerbatimIDs(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session := createNarrativeSession(ctx, t, q, projectID, "compare-exact-stable-ids")
	base := time.Date(2026, 3, 26, 13, 30, 0, 0, time.UTC)

	baseline := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-baseline", "Baseline", "completed", base, timePtr(base.Add(time.Minute)), nil, 0, 0, 0, 0)
	candidate := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-candidate", "Candidate", "completed", base.Add(2*time.Minute), timePtr(base.Add(3*time.Minute)), nil, 0, 0, 0, 0)

	createCompareSpan(ctx, t, q, projectID, baseline.ID, "shared", nil, "Shared", "tool", base, timePtr(base.Add(20*time.Second)), int32Ptr(1), 0, 0, 0)
	createCompareSpan(ctx, t, q, projectID, candidate.ID, "shared", nil, "Shared", "tool", base.Add(2*time.Minute), timePtr(base.Add(2*time.Minute+20*time.Second)), int32Ptr(1), 0, 0, 0)

	createCompareEvent(ctx, t, pool, q, projectID, baseline.ID, "effect", base.Add(time.Second), int32Ptr(1), "baseline exact effect", map[string]any{"effect_id": "effect-1", "effect_kind": "http"})
	createCompareEvent(ctx, t, pool, q, projectID, candidate.ID, "effect", base.Add(2*time.Minute+time.Second), int32Ptr(1), "candidate padded effect", map[string]any{"effect_id": " effect-1 ", "effect_kind": "http"})
	createCompareEvent(ctx, t, pool, q, projectID, baseline.ID, "wait", base.Add(2*time.Second), int32Ptr(2), "baseline exact wait", map[string]any{"wait_id": "wait-1", "resolution": "pending"})
	createCompareEvent(ctx, t, pool, q, projectID, candidate.ID, "wait", base.Add(2*time.Minute+2*time.Second), int32Ptr(2), "candidate padded wait", map[string]any{"wait_id": " wait-1 ", "resolution": "pending"})

	comparison, err := s.BuildSessionComparison(ctx, projectID, session.ID, baseline.ID, candidate.ID)
	require.NoError(t, err)

	groups := comparison.SpanDiffs[0].SemanticGroups
	assert.Equal(t, store.SessionCompareDiffStatusBaselineOnly, findSemanticGroupByMessage(t, groups, "baseline exact effect").DiffStatus)
	assert.Equal(t, store.SessionCompareDiffStatusCandidateOnly, findSemanticGroupByMessage(t, groups, "candidate padded effect").DiffStatus)
	assert.Equal(t, store.SessionCompareDiffStatusBaselineOnly, findSemanticGroupByMessage(t, groups, "baseline exact wait").DiffStatus)
	assert.Equal(t, store.SessionCompareDiffStatusCandidateOnly, findSemanticGroupByMessage(t, groups, "candidate padded wait").DiffStatus)
}

func TestBuildSessionComparison_PairsDecisionsEffectsAndWaits(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session := createNarrativeSession(ctx, t, q, projectID, "compare-semantics")
	base := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)

	baseline := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-baseline", "Baseline", "completed", base, timePtr(base.Add(time.Minute)), nil, 0, 0, 0, 0)
	candidate := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-candidate", "Candidate", "completed", base.Add(2*time.Minute), timePtr(base.Add(3*time.Minute)), nil, 0, 0, 0, 0)

	createCompareSpan(ctx, t, q, projectID, baseline.ID, "shared", nil, "Shared", "tool", base, timePtr(base.Add(20*time.Second)), int32Ptr(1), 0, 0, 0)
	createCompareSpan(ctx, t, q, projectID, candidate.ID, "shared", nil, "Shared", "tool", base.Add(2*time.Minute), timePtr(base.Add(2*time.Minute+20*time.Second)), int32Ptr(1), 0, 0, 0)

	createCompareEvent(ctx, t, pool, q, projectID, baseline.ID, "decision", base.Add(time.Second), int32Ptr(1), "baseline decision", map[string]any{"question": "Choose route?", "chosen": "A"})
	createCompareEvent(ctx, t, pool, q, projectID, candidate.ID, "decision", base.Add(2*time.Minute+time.Second), int32Ptr(1), "candidate decision", map[string]any{"question": " choose   route? ", "chosen": "B"})
	createCompareEvent(ctx, t, pool, q, projectID, baseline.ID, "decision", base.Add(2*time.Second), int32Ptr(2), "missing question", map[string]any{"chosen": "x"})
	createCompareEvent(ctx, t, pool, q, projectID, candidate.ID, "decision", base.Add(2*time.Minute+2*time.Second), int32Ptr(2), "candidate duplicate one", map[string]any{"question": "duplicate", "chosen": "x"})
	createCompareEvent(ctx, t, pool, q, projectID, candidate.ID, "decision", base.Add(2*time.Minute+3*time.Second), int32Ptr(3), "candidate duplicate two", map[string]any{"question": "duplicate", "chosen": "y"})

	createCompareEvent(ctx, t, pool, q, projectID, baseline.ID, "effect", base.Add(4*time.Second), int32Ptr(4), "effect baseline", map[string]any{"effect_id": "effect-1", "effect_kind": "http", "idempotent": true})
	createCompareEvent(ctx, t, pool, q, projectID, candidate.ID, "effect", base.Add(2*time.Minute+4*time.Second), int32Ptr(4), "effect baseline", map[string]any{"effect_id": "effect-1", "effect_kind": "http", "idempotent": false})
	createCompareEvent(ctx, t, pool, q, projectID, baseline.ID, "effect", base.Add(5*time.Second), int32Ptr(5), "effect no id", map[string]any{"effect_kind": "side-effect"})

	createCompareEvent(ctx, t, pool, q, projectID, baseline.ID, "wait", base.Add(6*time.Second), int32Ptr(6), "wait baseline", map[string]any{"wait_id": "wait-1", "wait_kind": "poll", "resolution": "timeout"})
	createCompareEvent(ctx, t, pool, q, projectID, candidate.ID, "wait", base.Add(2*time.Minute+6*time.Second), int32Ptr(5), "wait candidate", map[string]any{"wait_id": "wait-1", "wait_kind": "poll", "resolution": "completed"})

	comparison, err := s.BuildSessionComparison(ctx, projectID, session.ID, baseline.ID, candidate.ID)
	require.NoError(t, err)
	require.Len(t, comparison.SpanDiffs, 1)
	require.Len(t, comparison.SpanDiffs[0].SemanticGroups, 7)

	groups := comparison.SpanDiffs[0].SemanticGroups
	assert.Equal(t, store.SessionCompareDiffStatusChanged, groups[0].DiffStatus)
	require.NotNil(t, groups[0].MatchSource)
	assert.Equal(t, store.SessionCompareMatchSourceHeuristic, *groups[0].MatchSource)
	assert.Equal(t, []string{"chosen", "message"}, groups[0].ChangedFields)

	assert.Equal(t, store.SessionCompareDiffStatusBaselineOnly, groups[1].DiffStatus)
	assert.Equal(t, "missing question", *groups[1].BaselineEvent.Message)

	assert.Equal(t, store.SessionCompareDiffStatusChanged, groups[2].DiffStatus)
	require.NotNil(t, groups[2].MatchSource)
	assert.Equal(t, store.SessionCompareMatchSourceStableID, *groups[2].MatchSource)
	assert.Equal(t, []string{"idempotent", "payload"}, groups[2].ChangedFields)

	assert.Equal(t, store.SessionCompareDiffStatusBaselineOnly, groups[3].DiffStatus)
	assert.Equal(t, "effect no id", *groups[3].BaselineEvent.Message)

	assert.Equal(t, store.SessionCompareDiffStatusChanged, groups[4].DiffStatus)
	assert.Equal(t, []string{"resolution", "message", "payload"}, groups[4].ChangedFields)

	assert.Equal(t, store.SessionCompareDiffStatusCandidateOnly, groups[5].DiffStatus)
	assert.Equal(t, "candidate duplicate one", *groups[5].CandidateEvent.Message)
	assert.Equal(t, store.SessionCompareDiffStatusCandidateOnly, groups[6].DiffStatus)
	assert.Equal(t, "candidate duplicate two", *groups[6].CandidateEvent.Message)
}

func TestBuildSessionComparison_AppendsCandidateOnlyRootBranchesAfterBaselineTree(t *testing.T) {
	pool := testutil.TestDB(t)
	ctx := context.Background()
	s := store.New(pool)
	q := s.Queries()

	projectID := testutil.CreateTestProject(t, ctx, q)
	session := createNarrativeSession(ctx, t, q, projectID, "compare-order")
	base := time.Date(2026, 3, 26, 13, 0, 0, 0, time.UTC)

	baseline := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-baseline", "Baseline", "completed", base, timePtr(base.Add(time.Minute)), nil, 0, 0, 0, 0)
	candidate := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-candidate", "Candidate", "completed", base.Add(2*time.Minute), timePtr(base.Add(3*time.Minute)), nil, 0, 0, 0, 0)

	createCompareSpan(ctx, t, q, projectID, baseline.ID, "shared-root", nil, "Shared Root", "agent", base, timePtr(base.Add(10*time.Second)), int32Ptr(1), 0, 0, 0)
	createCompareSpan(ctx, t, q, projectID, candidate.ID, "shared-root", nil, "Shared Root", "agent", base.Add(2*time.Minute), timePtr(base.Add(2*time.Minute+10*time.Second)), int32Ptr(1), 0, 0, 0)
	createCompareSpan(ctx, t, q, projectID, baseline.ID, "baseline-child", strPtr("shared-root"), "Baseline Child", "tool", base.Add(11*time.Second), timePtr(base.Add(20*time.Second)), int32Ptr(2), 0, 0, 0)
	createCompareSpan(ctx, t, q, projectID, candidate.ID, "candidate-root", nil, "Candidate Root", "tool", base.Add(2*time.Minute+11*time.Second), timePtr(base.Add(2*time.Minute+20*time.Second)), int32Ptr(2), 0, 0, 0)
	createCompareSpan(ctx, t, q, projectID, candidate.ID, "candidate-child", strPtr("candidate-root"), "Candidate Child", "tool", base.Add(2*time.Minute+21*time.Second), timePtr(base.Add(2*time.Minute+30*time.Second)), int32Ptr(3), 0, 0, 0)

	comparison, err := s.BuildSessionComparison(ctx, projectID, session.ID, baseline.ID, candidate.ID)
	require.NoError(t, err)
	require.Len(t, comparison.SpanDiffs, 4)

	assert.Equal(t, "shared-root", comparison.SpanDiffs[0].BaselineSpan.SpanID)
	assert.Equal(t, "baseline-child", comparison.SpanDiffs[1].BaselineSpan.SpanID)
	assert.Equal(t, store.SessionCompareDiffStatusCandidateOnly, comparison.SpanDiffs[2].DiffStatus)
	assert.Equal(t, "candidate-root", comparison.SpanDiffs[2].CandidateSpan.SpanID)
	assert.Equal(t, 0, comparison.SpanDiffs[2].Depth)
	assert.Equal(t, "candidate-child", comparison.SpanDiffs[3].CandidateSpan.SpanID)
	assert.Equal(t, 1, comparison.SpanDiffs[3].Depth)
}

type sessionCompareGoldenFixture struct {
	ctx              context.Context
	t                *testing.T
	pool             *pgxpool.Pool
	q                *platform.Queries
	projectID        uuid.UUID
	sessionID        uuid.UUID
	baselineTraceID  uuid.UUID
	candidateTraceID uuid.UUID
	base             time.Time
}

func TestBuildSessionComparison_GoldenSnapshots(t *testing.T) {
	testCases := []struct {
		name  string
		setup func(sessionCompareGoldenFixture)
	}{
		{
			name: "nearly-identical-rerun",
			setup: func(f sessionCompareGoldenFixture) {
				createCompareSpan(f.ctx, f.t, f.q, f.projectID, f.baselineTraceID, "shared-root", nil, "Root", "agent", f.base, timePtr(f.base.Add(15*time.Second)), int32Ptr(1), 10, 4, 0.01)
				createCompareSpan(f.ctx, f.t, f.q, f.projectID, f.candidateTraceID, "shared-root", nil, "Root", "agent", f.base.Add(2*time.Minute), timePtr(f.base.Add(2*time.Minute+15*time.Second)), int32Ptr(1), 10, 4, 0.01)
				createCompareEventForSpan(f.ctx, f.t, f.pool, f.q, f.projectID, f.baselineTraceID, "shared-root", "decision", f.base.Add(time.Second), int32Ptr(1), "Choose alpha path", map[string]any{"question": "Pick a path?", "chosen": "alpha"})
				createCompareEventForSpan(f.ctx, f.t, f.pool, f.q, f.projectID, f.candidateTraceID, "shared-root", "decision", f.base.Add(2*time.Minute+time.Second), int32Ptr(1), "Choose alpha path", map[string]any{"question": " pick   a path? ", "chosen": "alpha"})
			},
		},
		{
			name: "changed-span-structure",
			setup: func(f sessionCompareGoldenFixture) {
				createCompareSpan(f.ctx, f.t, f.q, f.projectID, f.baselineTraceID, "shared-root", nil, "Root", "agent", f.base, timePtr(f.base.Add(30*time.Second)), int32Ptr(1), 0, 0, 0)
				createCompareSpan(f.ctx, f.t, f.q, f.projectID, f.candidateTraceID, "shared-root", nil, "Root", "agent", f.base.Add(2*time.Minute), timePtr(f.base.Add(2*time.Minute+30*time.Second)), int32Ptr(1), 0, 0, 0)
				createCompareSpan(f.ctx, f.t, f.q, f.projectID, f.baselineTraceID, "inspect", strPtr("shared-root"), "Inspect", "tool", f.base.Add(time.Second), timePtr(f.base.Add(10*time.Second)), int32Ptr(2), 3, 1, 0.001)
				createCompareSpan(f.ctx, f.t, f.q, f.projectID, f.candidateTraceID, "inspect", strPtr("shared-root"), "Inspect", "tool", f.base.Add(2*time.Minute+time.Second), timePtr(f.base.Add(2*time.Minute+10*time.Second)), int32Ptr(2), 3, 1, 0.001)
				createCompareSpan(f.ctx, f.t, f.q, f.projectID, f.baselineTraceID, "baseline-only", strPtr("shared-root"), "Fetch Inventory", "tool", f.base.Add(11*time.Second), timePtr(f.base.Add(20*time.Second)), int32Ptr(3), 5, 2, 0.002)
				createCompareSpan(f.ctx, f.t, f.q, f.projectID, f.candidateTraceID, "candidate-only", strPtr("shared-root"), "Retry Inventory", "tool", f.base.Add(2*time.Minute+11*time.Second), timePtr(f.base.Add(2*time.Minute+22*time.Second)), int32Ptr(3), 6, 2, 0.003)
			},
		},
		{
			name: "changed-decision-path",
			setup: func(f sessionCompareGoldenFixture) {
				createCompareSpan(f.ctx, f.t, f.q, f.projectID, f.baselineTraceID, "shared-root", nil, "Plan", "agent", f.base, timePtr(f.base.Add(20*time.Second)), int32Ptr(1), 8, 3, 0.01)
				createCompareSpan(f.ctx, f.t, f.q, f.projectID, f.candidateTraceID, "shared-root", nil, "Plan", "agent", f.base.Add(2*time.Minute), timePtr(f.base.Add(2*time.Minute+20*time.Second)), int32Ptr(1), 8, 3, 0.01)
				createCompareEventForSpan(f.ctx, f.t, f.pool, f.q, f.projectID, f.baselineTraceID, "shared-root", "decision", f.base.Add(time.Second), int32Ptr(1), "Picked checkout branch", map[string]any{"question": "What path should I use?", "chosen": "checkout", "alternatives": []string{"checkout", "catalog"}})
				createCompareEventForSpan(f.ctx, f.t, f.pool, f.q, f.projectID, f.candidateTraceID, "shared-root", "decision", f.base.Add(2*time.Minute+time.Second), int32Ptr(1), "Picked catalog branch", map[string]any{"question": "what path should I use?", "chosen": "catalog", "alternatives": []string{"checkout", "catalog"}})
			},
		},
		{
			name: "effect-added-removed",
			setup: func(f sessionCompareGoldenFixture) {
				createCompareSpan(f.ctx, f.t, f.q, f.projectID, f.baselineTraceID, "shared-root", nil, "Effects", "tool", f.base, timePtr(f.base.Add(20*time.Second)), int32Ptr(1), 0, 0, 0)
				createCompareSpan(f.ctx, f.t, f.q, f.projectID, f.candidateTraceID, "shared-root", nil, "Effects", "tool", f.base.Add(2*time.Minute), timePtr(f.base.Add(2*time.Minute+20*time.Second)), int32Ptr(1), 0, 0, 0)
				createCompareEventForSpan(f.ctx, f.t, f.pool, f.q, f.projectID, f.baselineTraceID, "shared-root", "effect", f.base.Add(time.Second), int32Ptr(1), "Checkout request", map[string]any{"effect_id": "effect-1", "effect_kind": "http", "idempotent": true})
				createCompareEventForSpan(f.ctx, f.t, f.pool, f.q, f.projectID, f.candidateTraceID, "shared-root", "effect", f.base.Add(2*time.Minute+time.Second), int32Ptr(1), "Checkout request", map[string]any{"effect_id": "effect-1", "effect_kind": "http", "idempotent": false})
				createCompareEventForSpan(f.ctx, f.t, f.pool, f.q, f.projectID, f.baselineTraceID, "shared-root", "effect", f.base.Add(2*time.Second), int32Ptr(2), "Legacy side effect", map[string]any{"effect_kind": "side-effect"})
				createCompareEventForSpan(f.ctx, f.t, f.pool, f.q, f.projectID, f.candidateTraceID, "shared-root", "effect", f.base.Add(2*time.Minute+2*time.Second), int32Ptr(2), "New idempotent side effect", map[string]any{"effect_id": "effect-2", "effect_kind": "cache_write", "idempotent": true})
			},
		},
		{
			name: "wait-added-removed",
			setup: func(f sessionCompareGoldenFixture) {
				createCompareSpan(f.ctx, f.t, f.q, f.projectID, f.baselineTraceID, "shared-root", nil, "Waits", "tool", f.base, timePtr(f.base.Add(20*time.Second)), int32Ptr(1), 0, 0, 0)
				createCompareSpan(f.ctx, f.t, f.q, f.projectID, f.candidateTraceID, "shared-root", nil, "Waits", "tool", f.base.Add(2*time.Minute), timePtr(f.base.Add(2*time.Minute+20*time.Second)), int32Ptr(1), 0, 0, 0)
				createCompareEventForSpan(f.ctx, f.t, f.pool, f.q, f.projectID, f.baselineTraceID, "shared-root", "wait", f.base.Add(time.Second), int32Ptr(1), "Waiting for webhook", map[string]any{"wait_id": "wait-1", "wait_kind": "webhook", "phase": "pending", "resolution": "timeout"})
				createCompareEventForSpan(f.ctx, f.t, f.pool, f.q, f.projectID, f.candidateTraceID, "shared-root", "wait", f.base.Add(2*time.Minute+time.Second), int32Ptr(1), "Waiting for webhook", map[string]any{"wait_id": "wait-1", "wait_kind": "webhook", "phase": "pending", "resolution": "completed"})
				createCompareEventForSpan(f.ctx, f.t, f.pool, f.q, f.projectID, f.baselineTraceID, "shared-root", "wait", f.base.Add(2*time.Second), int32Ptr(2), "Legacy wait", map[string]any{"phase": "blocked", "resolution": "timeout"})
				createCompareEventForSpan(f.ctx, f.t, f.pool, f.q, f.projectID, f.candidateTraceID, "shared-root", "wait", f.base.Add(2*time.Minute+2*time.Second), int32Ptr(2), "Retry wait", map[string]any{"wait_id": "wait-2", "wait_kind": "poll", "phase": "retry", "resolution": "completed"})
			},
		},
		{
			name: "ambiguous-heuristic-case",
			setup: func(f sessionCompareGoldenFixture) {
				createCompareSpan(f.ctx, f.t, f.q, f.projectID, f.baselineTraceID, "shared-root", nil, "Root", "agent", f.base, timePtr(f.base.Add(20*time.Second)), int32Ptr(1), 0, 0, 0)
				createCompareSpan(f.ctx, f.t, f.q, f.projectID, f.candidateTraceID, "shared-root", nil, "Root", "agent", f.base.Add(2*time.Minute), timePtr(f.base.Add(2*time.Minute+20*time.Second)), int32Ptr(1), 0, 0, 0)
				createCompareSpan(f.ctx, f.t, f.q, f.projectID, f.baselineTraceID, "baseline-child", strPtr("shared-root"), "Lookup", "tool", f.base.Add(21*time.Second), timePtr(f.base.Add(30*time.Second)), int32Ptr(2), 0, 0, 0)
				createCompareSpan(f.ctx, f.t, f.q, f.projectID, f.candidateTraceID, "candidate-child-a", strPtr("shared-root"), "Lookup", "tool", f.base.Add(2*time.Minute+21*time.Second), timePtr(f.base.Add(2*time.Minute+30*time.Second)), int32Ptr(2), 0, 0, 0)
				createCompareSpan(f.ctx, f.t, f.q, f.projectID, f.candidateTraceID, "candidate-child-b", strPtr("shared-root"), "Lookup", "tool", f.base.Add(2*time.Minute+31*time.Second), timePtr(f.base.Add(2*time.Minute+40*time.Second)), int32Ptr(3), 0, 0, 0)
			},
		},
		{
			name: "candidate-only-branch",
			setup: func(f sessionCompareGoldenFixture) {
				createCompareSpan(f.ctx, f.t, f.q, f.projectID, f.baselineTraceID, "shared-root", nil, "Root", "agent", f.base, timePtr(f.base.Add(20*time.Second)), int32Ptr(1), 0, 0, 0)
				createCompareSpan(f.ctx, f.t, f.q, f.projectID, f.candidateTraceID, "shared-root", nil, "Root", "agent", f.base.Add(2*time.Minute), timePtr(f.base.Add(2*time.Minute+20*time.Second)), int32Ptr(1), 0, 0, 0)
				createCompareSpan(f.ctx, f.t, f.q, f.projectID, f.candidateTraceID, "candidate-root", nil, "Candidate Root", "chain", f.base.Add(2*time.Minute+21*time.Second), timePtr(f.base.Add(2*time.Minute+35*time.Second)), int32Ptr(2), 0, 0, 0)
				createCompareSpan(f.ctx, f.t, f.q, f.projectID, f.candidateTraceID, "candidate-child", strPtr("candidate-root"), "Candidate Child", "tool", f.base.Add(2*time.Minute+36*time.Second), timePtr(f.base.Add(2*time.Minute+45*time.Second)), int32Ptr(3), 0, 0, 0)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pool := testutil.TestDB(t)
			ctx := context.Background()
			s := store.New(pool)
			q := s.Queries()

			projectID := testutil.CreateTestProject(t, ctx, q)
			session := createNarrativeSession(ctx, t, q, projectID, "compare-golden-"+tc.name)
			base := time.Date(2026, 3, 27, 9, 0, 0, 0, time.UTC)
			baseline := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-baseline-"+tc.name, "Baseline "+tc.name, "completed", base, timePtr(base.Add(time.Minute)), nil, 40, 20, 0.11, 0)
			candidate := createNarrativeTrace(t, ctx, pool, q, projectID, session.ID, "trace-candidate-"+tc.name, "Candidate "+tc.name, "completed", base.Add(2*time.Minute), timePtr(base.Add(3*time.Minute)), nil, 44, 22, 0.13, 0)

			tc.setup(sessionCompareGoldenFixture{
				ctx:              ctx,
				t:                t,
				pool:             pool,
				q:                q,
				projectID:        projectID,
				sessionID:        session.ID,
				baselineTraceID:  baseline.ID,
				candidateTraceID: candidate.ID,
				base:             base,
			})

			comparison, err := s.BuildSessionComparison(ctx, projectID, session.ID, baseline.ID, candidate.ID)
			require.NoError(t, err)
			assertCompareGoldenSnapshot(t, tc.name, comparison)
		})
	}
}

func createCompareSpan(
	ctx context.Context,
	t *testing.T,
	q *platform.Queries,
	projectID uuid.UUID,
	traceID uuid.UUID,
	spanID string,
	parentSpanID *string,
	name string,
	spanType string,
	startedAt time.Time,
	endedAt *time.Time,
	sequence *int32,
	promptTokens int64,
	completionTokens int64,
	totalCost float64,
) platform.Span {
	t.Helper()

	span, err := q.UpsertSpan(ctx, platform.UpsertSpanParams{
		ProjectID:        projectID,
		TraceID:          traceID,
		SpanID:           spanID,
		ParentSpanID:     parentSpanID,
		Name:             name,
		Type:             spanType,
		Status:           "completed",
		StatusMessage:    nil,
		Level:            "default",
		StartTime:        startedAt,
		EndTime:          testutil.PgtypeTimestamptzPtr(endedAt),
		Model:            nil,
		PromptTokens:     testutil.Int64Ptr(promptTokens),
		CompletionTokens: testutil.Int64Ptr(completionTokens),
		TotalCost:        testutil.PgtypeNumericFromFloat64(totalCost),
		Sequence:         sequence,
	})
	require.NoError(t, err)

	return span
}

func createCompareEvent(
	ctx context.Context,
	t *testing.T,
	pool *pgxpool.Pool,
	q *platform.Queries,
	projectID uuid.UUID,
	traceID uuid.UUID,
	eventType string,
	eventAt time.Time,
	sequence *int32,
	message string,
	payload map[string]any,
) {
	t.Helper()

	createCompareEventForSpan(ctx, t, pool, q, projectID, traceID, "shared", eventType, eventAt, sequence, message, payload)
}

func createCompareEventForSpan(
	ctx context.Context,
	t *testing.T,
	pool *pgxpool.Pool,
	q *platform.Queries,
	projectID uuid.UUID,
	traceID uuid.UUID,
	spanID string,
	eventType string,
	eventAt time.Time,
	sequence *int32,
	message string,
	payload map[string]any,
) {
	t.Helper()

	eventID, err := q.InsertSpanEvent(ctx, platform.InsertSpanEventParams{
		ProjectID: projectID,
		TraceID:   traceID,
		SpanID:    spanID,
		EventType: eventType,
		Level:     "info",
		EventTs:   testutil.PgtypeTimestamptz(eventAt),
		Sequence:  sequence,
		Message:   testutil.StrPtr(message),
		Payload:   narrativeJSON(t, payload),
	})
	require.NoError(t, err)

	_, err = pool.Exec(ctx, "UPDATE span_events SET server_ingested_at = $2 WHERE id = $1", eventID, eventAt)
	require.NoError(t, err)
}

func assertCompareGoldenSnapshot(t *testing.T, name string, comparison store.SessionComparison) {
	t.Helper()

	actual := renderCompareGoldenSnapshot(comparison)
	expectedPath := filepath.Join("testdata", "session_compare", name+".golden.txt")
	expected, err := os.ReadFile(expectedPath)
	if err != nil {
		t.Fatalf("read golden snapshot %s: %v\nactual snapshot:\n%s", expectedPath, err, actual)
	}

	assert.Equal(t, strings.TrimSpace(string(expected)), strings.TrimSpace(actual))
}

func renderCompareGoldenSnapshot(comparison store.SessionComparison) string {
	var builder strings.Builder
	fmt.Fprintf(
		&builder,
		"summary spans=%d/%d matched=%d unmatched=%d/%d heuristic=%d semantic=%d/%d delta(duration=%d tokens_in=%d tokens_out=%d cost=%.4f)\n",
		comparison.Summary.TotalSpansBaseline,
		comparison.Summary.TotalSpansCandidate,
		comparison.Summary.MatchedSpans,
		comparison.Summary.UnmatchedBaselineSpans,
		comparison.Summary.UnmatchedCandidateSpans,
		comparison.Summary.HeuristicMatches,
		comparison.Summary.TotalSemanticBaseline,
		comparison.Summary.TotalSemanticCandidate,
		comparison.Summary.DurationDeltaMs,
		comparison.Summary.TokensInDelta,
		comparison.Summary.TokensOutDelta,
		comparison.Summary.CostDeltaUsd,
	)

	for index, row := range comparison.SpanDiffs {
		fmt.Fprintf(
			&builder,
			"row %d depth=%d diff=%s match=%s fields=%s baseline=%s candidate=%s\n",
			index,
			row.Depth,
			row.DiffStatus,
			formatCompareMatch(row.MatchSource, row.MatchReason),
			formatCompareChangedFields(row.ChangedFields),
			formatCompareSpanSnapshot(row.BaselineSpan),
			formatCompareSpanSnapshot(row.CandidateSpan),
		)

		for semanticIndex, group := range row.SemanticGroups {
			fmt.Fprintf(
				&builder,
				"  semantic %d type=%s diff=%s match=%s fields=%s baseline=%s candidate=%s\n",
				semanticIndex,
				group.EventType,
				group.DiffStatus,
				formatCompareMatch(group.MatchSource, group.MatchReason),
				formatCompareChangedFields(group.ChangedFields),
				formatCompareSemanticSnapshot(group.BaselineEvent),
				formatCompareSemanticSnapshot(group.CandidateEvent),
			)
		}
	}

	return builder.String()
}

func formatCompareMatch(source *store.SessionCompareMatchSource, reason *string) string {
	if source == nil {
		return "-"
	}
	if reason == nil {
		return string(*source)
	}
	return fmt.Sprintf("%s:%s", *source, *reason)
}

func formatCompareChangedFields(fields []string) string {
	if len(fields) == 0 {
		return "-"
	}
	return strings.Join(fields, ",")
}

func formatCompareSpanSnapshot(span *store.SessionCompareSpanSummary) string {
	if span == nil {
		return "-"
	}

	return fmt.Sprintf(
		"%s(parent=%s name=%q kind=%s status=%s latency=%s tokens=%s/%s cost=%s error=%s)",
		span.SpanID,
		formatCompareOptionalString(span.ParentSpanID),
		span.Name,
		span.Kind,
		span.Status,
		formatCompareOptionalInt64(span.LatencyMs),
		formatCompareOptionalInt64(span.TokensIn),
		formatCompareOptionalInt64(span.TokensOut),
		formatCompareOptionalFloat32(span.CostUsd),
		formatCompareOptionalString(span.ErrorMessage),
	)
}

func formatCompareSemanticSnapshot(event *store.SessionCompareSemanticSummary) string {
	if event == nil {
		return "-"
	}

	parts := []string{
		fmt.Sprintf("span=%s", formatCompareOptionalString(event.SpanID)),
		fmt.Sprintf("msg=%s", formatCompareOptionalString(event.Message)),
	}

	switch event.EventType {
	case store.SessionCompareSemanticEventTypeDecision:
		parts = append(parts,
			fmt.Sprintf("question=%s", formatComparePayloadValue(event.Payload, "question")),
			fmt.Sprintf("chosen=%s", formatComparePayloadValue(event.Payload, "chosen")),
			fmt.Sprintf("alternatives=%s", formatComparePayloadValue(event.Payload, "alternatives")),
			fmt.Sprintf("reasoning=%s", formatComparePayloadValue(event.Payload, "reasoning")),
		)
	case store.SessionCompareSemanticEventTypeEffect:
		parts = append(parts,
			fmt.Sprintf("effect_id=%s", formatComparePayloadValue(event.Payload, "effect_id")),
			fmt.Sprintf("effect_kind=%s", formatComparePayloadValue(event.Payload, "effect_kind")),
			fmt.Sprintf("idempotent=%s", formatComparePayloadValue(event.Payload, "idempotent")),
			fmt.Sprintf("idempotency_key=%s", formatComparePayloadValue(event.Payload, "idempotency_key")),
		)
	case store.SessionCompareSemanticEventTypeWait:
		parts = append(parts,
			fmt.Sprintf("wait_id=%s", formatComparePayloadValue(event.Payload, "wait_id")),
			fmt.Sprintf("wait_kind=%s", formatComparePayloadValue(event.Payload, "wait_kind")),
			fmt.Sprintf("phase=%s", formatComparePayloadValue(event.Payload, "phase")),
			fmt.Sprintf("resolution=%s", formatComparePayloadValue(event.Payload, "resolution")),
		)
	}

	return fmt.Sprintf("%s(%s)", event.EventType, strings.Join(parts, " "))
}

func formatComparePayloadValue(payload map[string]interface{}, key string) string {
	if payload == nil {
		return "-"
	}

	value, ok := payload[key]
	if !ok {
		return "-"
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}

	return string(encoded)
}

func formatCompareOptionalString(value *string) string {
	if value == nil {
		return "-"
	}
	return fmt.Sprintf("%q", *value)
}

func formatCompareOptionalInt64(value *int64) string {
	if value == nil {
		return "-"
	}
	return fmt.Sprintf("%d", *value)
}

func formatCompareOptionalFloat32(value *float32) string {
	if value == nil {
		return "-"
	}
	return fmt.Sprintf("%.4f", *value)
}

func int32Ptr(value int32) *int32 {
	return &value
}

func strPtr(value string) *string {
	return &value
}

func findSemanticGroupByMessage(
	t *testing.T,
	groups []store.SessionCompareSemanticDiffGroup,
	message string,
) store.SessionCompareSemanticDiffGroup {
	t.Helper()

	for _, group := range groups {
		if group.BaselineEvent != nil && group.BaselineEvent.Message != nil && *group.BaselineEvent.Message == message {
			return group
		}
		if group.CandidateEvent != nil && group.CandidateEvent.Message != nil && *group.CandidateEvent.Message == message {
			return group
		}
	}

	t.Fatalf("expected semantic group with message %q", message)
	return store.SessionCompareSemanticDiffGroup{}
}
