package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

const (
	SessionCompareMaxSpans          = 500
	SessionCompareMaxSemanticEvents = 1000

	sessionCompareRootScope = "__root__"
)

type SessionCompareDiffStatus string

const (
	SessionCompareDiffStatusUnchanged     SessionCompareDiffStatus = "unchanged"
	SessionCompareDiffStatusChanged       SessionCompareDiffStatus = "changed"
	SessionCompareDiffStatusBaselineOnly  SessionCompareDiffStatus = "baseline_only"
	SessionCompareDiffStatusCandidateOnly SessionCompareDiffStatus = "candidate_only"
)

type SessionCompareMatchSource string

const (
	SessionCompareMatchSourceStableID  SessionCompareMatchSource = "stable_id"
	SessionCompareMatchSourceHeuristic SessionCompareMatchSource = "heuristic"
)

type SessionCompareSemanticEventType string

const (
	SessionCompareSemanticEventTypeDecision SessionCompareSemanticEventType = "decision"
	SessionCompareSemanticEventTypeEffect   SessionCompareSemanticEventType = "effect"
	SessionCompareSemanticEventTypeWait     SessionCompareSemanticEventType = "wait"
)

type SessionComparison struct {
	Session   SessionCompareSessionHeader
	Baseline  SessionCompareTraceHeader
	Candidate SessionCompareTraceHeader
	Summary   SessionCompareSummary
	SpanDiffs []SessionCompareSpanDiffRow
}

type SessionCompareSessionHeader struct {
	ID         uuid.UUID
	ExternalID string
	Name       *string
}

type SessionCompareTraceHeader struct {
	ID             uuid.UUID
	TraceID        string
	Name           string
	Status         string
	UserID         *string
	StartedAt      time.Time
	EndedAt        *time.Time
	DurationMs     *int64
	ErrorCount     *int
	TotalCostUsd   *float32
	TotalTokensIn  *int64
	TotalTokensOut *int64
}

type SessionCompareSummary struct {
	TotalSpansBaseline      int
	TotalSpansCandidate     int
	MatchedSpans            int
	UnmatchedBaselineSpans  int
	UnmatchedCandidateSpans int
	HeuristicMatches        int
	DurationDeltaMs         int64
	TokensInDelta           int64
	TokensOutDelta          int64
	CostDeltaUsd            float32
	TotalSemanticBaseline   int
	TotalSemanticCandidate  int
}

type SessionCompareSpanDiffRow struct {
	DiffStatus     SessionCompareDiffStatus
	MatchSource    *SessionCompareMatchSource
	MatchReason    *string
	ChangedFields  []string
	BaselineSpan   *SessionCompareSpanSummary
	CandidateSpan  *SessionCompareSpanSummary
	SemanticGroups []SessionCompareSemanticDiffGroup
	Depth          int
}

type SessionCompareSemanticDiffGroup struct {
	EventType      SessionCompareSemanticEventType
	DiffStatus     SessionCompareDiffStatus
	MatchSource    *SessionCompareMatchSource
	MatchReason    *string
	ChangedFields  []string
	BaselineEvent  *SessionCompareSemanticSummary
	CandidateEvent *SessionCompareSemanticSummary
}

type SessionCompareSpanSummary struct {
	ID           uuid.UUID
	SpanID       string
	ParentSpanID *string
	Name         string
	Kind         string
	Status       string
	StartedAt    time.Time
	EndedAt      *time.Time
	LatencyMs    *int64
	TokensIn     *int64
	TokensOut    *int64
	CostUsd      *float32
	ErrorMessage *string
	Model        *string
}

type SessionCompareSemanticSummary struct {
	ID        string
	SpanID    *string
	SpanName  *string
	EventType SessionCompareSemanticEventType
	Timestamp time.Time
	Message   *string
	Payload   map[string]interface{}
}

type SessionCompareCountDetail struct {
	BaselineSpanCount      int
	CandidateSpanCount     int
	BaselineSemanticCount  int
	CandidateSemanticCount int
	MaxSpans               int
	MaxSemanticEvents      int
}

type SessionCompareValidationResult struct {
	Session   SessionCompareSessionHeader
	Baseline  SessionCompareTraceHeader
	Candidate SessionCompareTraceHeader
	Counts    SessionCompareCountDetail
}

type SessionCompareValidationError struct {
	Code    string
	Message string
}

func (e *SessionCompareValidationError) Error() string {
	return e.Message
}

type SessionCompareTooLargeError struct {
	Message string
	Detail  SessionCompareCountDetail
}

func (e *SessionCompareTooLargeError) Error() string {
	return e.Message
}

// ValidateCompareEligibility loads the scoped compare headers and rejects invalid comparisons
// before any span or event rows are fetched.
func (s *Store) ValidateCompareEligibility(
	ctx context.Context,
	projectID uuid.UUID,
	sessionID uuid.UUID,
	baselineTraceID uuid.UUID,
	candidateTraceID uuid.UUID,
) (SessionCompareValidationResult, error) {
	if baselineTraceID == candidateTraceID {
		return SessionCompareValidationResult{}, &SessionCompareValidationError{
			Code:    "invalid_compare_request",
			Message: "Baseline and candidate traces must be different",
		}
	}

	row, err := s.q.GetCompareValidation(ctx, platform.GetCompareValidationParams{
		ID:        sessionID,
		ProjectID: projectID,
		ID_2:      baselineTraceID,
		ID_3:      candidateTraceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return SessionCompareValidationResult{}, ErrNotFound
	}
	if err != nil {
		return SessionCompareValidationResult{}, fmt.Errorf("get compare validation: %w", err)
	}

	baselineStatusBucket := NormalizeTraceStatus(row.BaselineStatus)
	candidateStatusBucket := NormalizeTraceStatus(row.CandidateStatus)
	if baselineStatusBucket == TraceStatusBucketRunning || candidateStatusBucket == TraceStatusBucketRunning {
		return SessionCompareValidationResult{}, &SessionCompareValidationError{
			Code:    "invalid_compare_request",
			Message: "Both traces must be terminal to compare",
		}
	}

	traceIDs := []uuid.UUID{baselineTraceID, candidateTraceID}

	spanCounts, err := s.q.GetCompareSpanCounts(ctx, traceIDs)
	if err != nil {
		return SessionCompareValidationResult{}, fmt.Errorf("get compare span counts: %w", err)
	}

	semanticCounts, err := s.q.GetCompareSemanticCounts(ctx, traceIDs)
	if err != nil {
		return SessionCompareValidationResult{}, fmt.Errorf("get compare semantic counts: %w", err)
	}

	countDetail := SessionCompareCountDetail{
		BaselineSpanCount:      countCompareRows(spanCounts, baselineTraceID),
		CandidateSpanCount:     countCompareRows(spanCounts, candidateTraceID),
		BaselineSemanticCount:  countCompareSemanticRows(semanticCounts, baselineTraceID),
		CandidateSemanticCount: countCompareSemanticRows(semanticCounts, candidateTraceID),
		MaxSpans:               SessionCompareMaxSpans,
		MaxSemanticEvents:      SessionCompareMaxSemanticEvents,
	}

	if countDetail.BaselineSpanCount > SessionCompareMaxSpans ||
		countDetail.CandidateSpanCount > SessionCompareMaxSpans ||
		countDetail.BaselineSemanticCount > SessionCompareMaxSemanticEvents ||
		countDetail.CandidateSemanticCount > SessionCompareMaxSemanticEvents {
		return SessionCompareValidationResult{}, &SessionCompareTooLargeError{
			Message: "Comparison exceeds the supported size limits",
			Detail:  countDetail,
		}
	}

	return SessionCompareValidationResult{
		Session: SessionCompareSessionHeader{
			ID:         row.SessionID,
			ExternalID: row.SessionExternalID,
			Name:       row.SessionName,
		},
		Baseline: compareTraceHeaderFromValidationRow(
			row.BaselineID,
			row.BaselineTraceID,
			row.BaselineName,
			row.BaselineStatus,
			row.BaselineUserID,
			row.BaselineStartedAt,
			row.BaselineEndedAt,
			row.BaselineDurationMs,
			row.BaselineErrorCount,
			row.BaselineTotalCostUsd,
			row.BaselineTotalTokensIn,
			row.BaselineTotalTokensOut,
		),
		Candidate: compareTraceHeaderFromValidationRow(
			row.CandidateID,
			row.CandidateTraceID,
			row.CandidateName,
			row.CandidateStatus,
			row.CandidateUserID,
			row.CandidateStartedAt,
			row.CandidateEndedAt,
			row.CandidateDurationMs,
			row.CandidateErrorCount,
			row.CandidateTotalCostUsd,
			row.CandidateTotalTokensIn,
			row.CandidateTotalTokensOut,
		),
		Counts: countDetail,
	}, nil
}

// BuildSessionComparison assembles the full deterministic trace comparison payload.
func (s *Store) BuildSessionComparison(
	ctx context.Context,
	projectID uuid.UUID,
	sessionID uuid.UUID,
	baselineTraceID uuid.UUID,
	candidateTraceID uuid.UUID,
) (SessionComparison, error) {
	validation, err := s.ValidateCompareEligibility(ctx, projectID, sessionID, baselineTraceID, candidateTraceID)
	if err != nil {
		return SessionComparison{}, err
	}

	traceIDs := []uuid.UUID{baselineTraceID, candidateTraceID}

	spanRows, err := s.q.ListCompareSpans(ctx, traceIDs)
	if err != nil {
		return SessionComparison{}, fmt.Errorf("list compare spans: %w", err)
	}

	eventRows, err := s.q.ListCompareSemanticEvents(ctx, traceIDs)
	if err != nil {
		return SessionComparison{}, fmt.Errorf("list compare semantic events: %w", err)
	}

	builder := newSessionComparisonBuilder(&validation, baselineTraceID, candidateTraceID, spanRows, eventRows)
	return builder.build(), nil
}

type sessionCompareHeuristicKey struct {
	Scope   string
	Name    string
	Kind    string
	Ordinal int
}

type sessionCompareSpanRecord struct {
	summary        SessionCompareSpanSummary
	traceID        uuid.UUID
	rawKind        string
	normalizedName string
	parentSpanID   *string
	pairID         string
	matchSource    *SessionCompareMatchSource
	matchReason    *string
	matched        *sessionCompareSpanRecord
}

type sessionCompareSemanticRecord struct {
	summary          SessionCompareSemanticSummary
	traceID          uuid.UUID
	spanID           string
	payload          map[string]interface{}
	matchSource      *SessionCompareMatchSource
	matchReason      *string
	paired           *sessionCompareSemanticRecord
	candidateEmitted bool
}

type sessionComparisonBuilder struct {
	validation            *SessionCompareValidationResult
	baselineTraceID       uuid.UUID
	candidateTraceID      uuid.UUID
	baselineSpans         []*sessionCompareSpanRecord
	candidateSpans        []*sessionCompareSpanRecord
	baselineBySpanID      map[string]*sessionCompareSpanRecord
	candidateBySpanID     map[string]*sessionCompareSpanRecord
	baselineChildren      map[string][]*sessionCompareSpanRecord
	candidateChildren     map[string][]*sessionCompareSpanRecord
	baselineEventsBySpan  map[string][]*sessionCompareSemanticRecord
	candidateEventsBySpan map[string][]*sessionCompareSemanticRecord
	heuristicMatches      int
	emittedCandidate      map[string]bool
}

func newSessionComparisonBuilder(
	validation *SessionCompareValidationResult,
	baselineTraceID uuid.UUID,
	candidateTraceID uuid.UUID,
	spanRows []platform.ListCompareSpansRow,
	eventRows []platform.ListCompareSemanticEventsRow,
) *sessionComparisonBuilder {
	builder := &sessionComparisonBuilder{
		validation:            validation,
		baselineTraceID:       baselineTraceID,
		candidateTraceID:      candidateTraceID,
		baselineBySpanID:      make(map[string]*sessionCompareSpanRecord),
		candidateBySpanID:     make(map[string]*sessionCompareSpanRecord),
		baselineChildren:      make(map[string][]*sessionCompareSpanRecord),
		candidateChildren:     make(map[string][]*sessionCompareSpanRecord),
		baselineEventsBySpan:  make(map[string][]*sessionCompareSemanticRecord),
		candidateEventsBySpan: make(map[string][]*sessionCompareSemanticRecord),
		emittedCandidate:      make(map[string]bool),
	}

	for i := range spanRows {
		row := &spanRows[i]
		record := newSessionCompareSpanRecord(row)
		switch row.TraceID {
		case baselineTraceID:
			builder.baselineSpans = append(builder.baselineSpans, record)
			builder.baselineBySpanID[record.summary.SpanID] = record
		case candidateTraceID:
			builder.candidateSpans = append(builder.candidateSpans, record)
			builder.candidateBySpanID[record.summary.SpanID] = record
		}
	}

	builder.baselineChildren = groupSessionCompareChildren(builder.baselineSpans, builder.baselineBySpanID)
	builder.candidateChildren = groupSessionCompareChildren(builder.candidateSpans, builder.candidateBySpanID)

	for i := range eventRows {
		row := &eventRows[i]
		record := newSessionCompareSemanticRecord(row)
		switch row.TraceID {
		case baselineTraceID:
			builder.baselineEventsBySpan[record.spanID] = append(builder.baselineEventsBySpan[record.spanID], record)
		case candidateTraceID:
			builder.candidateEventsBySpan[record.spanID] = append(builder.candidateEventsBySpan[record.spanID], record)
		}
	}

	return builder
}

func (b *sessionComparisonBuilder) build() SessionComparison {
	b.matchStableSpans()
	b.heuristicMatches = b.matchHeuristicSpans()

	rows := make([]SessionCompareSpanDiffRow, 0, len(b.baselineSpans)+len(b.candidateSpans))
	for _, root := range b.baselineChildren[sessionCompareRootScope] {
		rows = b.appendBaselineBranch(rows, root, 0)
	}

	for _, root := range b.candidateChildren[sessionCompareRootScope] {
		if root.matched != nil || b.emittedCandidate[root.summary.SpanID] {
			continue
		}
		rows = b.appendCandidateOnlyBranch(rows, root, 0)
	}

	return SessionComparison{
		Session:   b.validation.Session,
		Baseline:  b.validation.Baseline,
		Candidate: b.validation.Candidate,
		Summary:   b.buildSummary(),
		SpanDiffs: rows,
	}
}

func (b *sessionComparisonBuilder) buildSummary() SessionCompareSummary {
	durationDelta := compareInt64Value(b.validation.Candidate.DurationMs) - compareInt64Value(b.validation.Baseline.DurationMs)
	tokensInDelta := compareInt64Value(b.validation.Candidate.TotalTokensIn) - compareInt64Value(b.validation.Baseline.TotalTokensIn)
	tokensOutDelta := compareInt64Value(b.validation.Candidate.TotalTokensOut) - compareInt64Value(b.validation.Baseline.TotalTokensOut)
	costDelta := compareFloat32Value(b.validation.Candidate.TotalCostUsd) - compareFloat32Value(b.validation.Baseline.TotalCostUsd)

	return SessionCompareSummary{
		TotalSpansBaseline:      b.validation.Counts.BaselineSpanCount,
		TotalSpansCandidate:     b.validation.Counts.CandidateSpanCount,
		MatchedSpans:            countMatchedCompareSpans(b.baselineSpans),
		UnmatchedBaselineSpans:  countUnmatchedCompareSpans(b.baselineSpans),
		UnmatchedCandidateSpans: countUnmatchedCompareSpans(b.candidateSpans),
		HeuristicMatches:        b.heuristicMatches,
		DurationDeltaMs:         durationDelta,
		TokensInDelta:           tokensInDelta,
		TokensOutDelta:          tokensOutDelta,
		CostDeltaUsd:            costDelta,
		TotalSemanticBaseline:   b.validation.Counts.BaselineSemanticCount,
		TotalSemanticCandidate:  b.validation.Counts.CandidateSemanticCount,
	}
}

func (b *sessionComparisonBuilder) matchStableSpans() {
	for spanID, baseline := range b.baselineBySpanID {
		candidate := b.candidateBySpanID[spanID]
		if candidate == nil {
			continue
		}
		b.assignSpanPair(baseline, candidate, SessionCompareMatchSourceStableID, "exact_span_id")
	}
}

func (b *sessionComparisonBuilder) matchHeuristicSpans() int {
	matches := 0
	for {
		baselineGroups := b.groupHeuristicCandidates(b.baselineSpans, b.baselineBySpanID)
		candidateGroups := b.groupHeuristicCandidates(b.candidateSpans, b.candidateBySpanID)

		matchedThisRound := 0
		for key, baselineGroup := range baselineGroups {
			candidateGroup := candidateGroups[key]
			if len(baselineGroup) != 1 || len(candidateGroup) != 1 {
				continue
			}
			if baselineGroup[0].matched != nil || candidateGroup[0].matched != nil {
				continue
			}
			b.assignSpanPair(baselineGroup[0], candidateGroup[0], SessionCompareMatchSourceHeuristic, "name_kind_ordinal")
			matchedThisRound++
			matches++
		}

		if matchedThisRound == 0 {
			break
		}
	}
	return matches
}

func (b *sessionComparisonBuilder) groupHeuristicCandidates(
	spans []*sessionCompareSpanRecord,
	bySpanID map[string]*sessionCompareSpanRecord,
) map[sessionCompareHeuristicKey][]*sessionCompareSpanRecord {
	groups := make(map[sessionCompareHeuristicKey][]*sessionCompareSpanRecord)
	scopeOrdinals := make(map[string]int)

	for _, span := range spans {
		scope, eligible := span.heuristicScope(bySpanID)
		if !eligible {
			continue
		}

		ordinal := scopeOrdinals[scope]
		scopeOrdinals[scope] = ordinal + 1

		if span.matched != nil {
			continue
		}

		key := sessionCompareHeuristicKey{
			Scope:   scope,
			Name:    span.normalizedName,
			Kind:    span.rawKind,
			Ordinal: ordinal,
		}
		groups[key] = append(groups[key], span)
	}

	return groups
}

func (b *sessionComparisonBuilder) appendBaselineBranch(
	rows []SessionCompareSpanDiffRow,
	span *sessionCompareSpanRecord,
	depth int,
) []SessionCompareSpanDiffRow {
	rows = append(rows, b.buildSpanDiffRow(span, span.matched, depth))

	for _, child := range b.baselineChildren[span.summary.SpanID] {
		rows = b.appendBaselineBranch(rows, child, depth+1)
	}

	if span.matched == nil {
		return rows
	}

	for _, child := range b.candidateChildren[span.matched.summary.SpanID] {
		if child.matched != nil || b.emittedCandidate[child.summary.SpanID] {
			continue
		}
		rows = b.appendCandidateOnlyBranch(rows, child, depth+1)
	}

	return rows
}

func (b *sessionComparisonBuilder) appendCandidateOnlyBranch(
	rows []SessionCompareSpanDiffRow,
	span *sessionCompareSpanRecord,
	depth int,
) []SessionCompareSpanDiffRow {
	b.emittedCandidate[span.summary.SpanID] = true
	rows = append(rows, b.buildSpanDiffRow(nil, span, depth))

	for _, child := range b.candidateChildren[span.summary.SpanID] {
		if child.matched != nil || b.emittedCandidate[child.summary.SpanID] {
			continue
		}
		rows = b.appendCandidateOnlyBranch(rows, child, depth+1)
	}

	return rows
}

func (b *sessionComparisonBuilder) buildSpanDiffRow(
	baseline *sessionCompareSpanRecord,
	candidate *sessionCompareSpanRecord,
	depth int,
) SessionCompareSpanDiffRow {
	switch {
	case baseline == nil && candidate != nil:
		return SessionCompareSpanDiffRow{
			DiffStatus:     SessionCompareDiffStatusCandidateOnly,
			ChangedFields:  []string{},
			BaselineSpan:   nil,
			CandidateSpan:  &candidate.summary,
			SemanticGroups: buildUnpairedSemanticGroups(b.candidateEventsBySpan[candidate.summary.SpanID], SessionCompareDiffStatusCandidateOnly),
			Depth:          depth,
		}
	case baseline != nil && candidate == nil:
		return SessionCompareSpanDiffRow{
			DiffStatus:     SessionCompareDiffStatusBaselineOnly,
			ChangedFields:  []string{},
			BaselineSpan:   &baseline.summary,
			CandidateSpan:  nil,
			SemanticGroups: buildUnpairedSemanticGroups(b.baselineEventsBySpan[baseline.summary.SpanID], SessionCompareDiffStatusBaselineOnly),
			Depth:          depth,
		}
	default:
		semanticGroups := buildMatchedSemanticGroups(
			b.baselineEventsBySpan[baseline.summary.SpanID],
			b.candidateEventsBySpan[candidate.summary.SpanID],
		)
		changedFields := compareSpanChangedFields(&baseline.summary, &candidate.summary, len(b.baselineEventsBySpan[baseline.summary.SpanID]), len(b.candidateEventsBySpan[candidate.summary.SpanID]))
		diffStatus := SessionCompareDiffStatusUnchanged
		if len(changedFields) > 0 || semanticGroupsHaveChanges(semanticGroups) {
			diffStatus = SessionCompareDiffStatusChanged
		}
		return SessionCompareSpanDiffRow{
			DiffStatus:     diffStatus,
			MatchSource:    baseline.matchSource,
			MatchReason:    baseline.matchReason,
			ChangedFields:  changedFields,
			BaselineSpan:   &baseline.summary,
			CandidateSpan:  &candidate.summary,
			SemanticGroups: semanticGroups,
			Depth:          depth,
		}
	}
}

func buildMatchedSemanticGroups(
	baselineEvents []*sessionCompareSemanticRecord,
	candidateEvents []*sessionCompareSemanticRecord,
) []SessionCompareSemanticDiffGroup {
	if len(baselineEvents) == 0 && len(candidateEvents) == 0 {
		return nil
	}

	resetCompareSemanticPairs(baselineEvents)
	resetCompareSemanticPairs(candidateEvents)

	pairSemanticEvents(baselineEvents, candidateEvents, SessionCompareSemanticEventTypeDecision, semanticDecisionKey, SessionCompareMatchSourceHeuristic, "unique_normalized_question")
	pairSemanticEvents(baselineEvents, candidateEvents, SessionCompareSemanticEventTypeEffect, semanticEffectKey, SessionCompareMatchSourceStableID, "exact_effect_id")
	pairSemanticEvents(baselineEvents, candidateEvents, SessionCompareSemanticEventTypeWait, semanticWaitKey, SessionCompareMatchSourceStableID, "exact_wait_id")

	groups := make([]SessionCompareSemanticDiffGroup, 0, len(baselineEvents)+len(candidateEvents))
	emittedCandidate := make(map[string]bool, len(candidateEvents))

	for _, baseline := range baselineEvents {
		if baseline.paired == nil {
			groups = append(groups, SessionCompareSemanticDiffGroup{
				EventType:      baseline.summary.EventType,
				DiffStatus:     SessionCompareDiffStatusBaselineOnly,
				ChangedFields:  []string{},
				BaselineEvent:  &baseline.summary,
				CandidateEvent: nil,
			})
			continue
		}

		emittedCandidate[baseline.paired.summary.ID] = true
		changedFields := compareSemanticChangedFields(baseline, baseline.paired)
		diffStatus := SessionCompareDiffStatusUnchanged
		if len(changedFields) > 0 {
			diffStatus = SessionCompareDiffStatusChanged
		}

		groups = append(groups, SessionCompareSemanticDiffGroup{
			EventType:      baseline.summary.EventType,
			DiffStatus:     diffStatus,
			MatchSource:    baseline.matchSource,
			MatchReason:    baseline.matchReason,
			ChangedFields:  changedFields,
			BaselineEvent:  &baseline.summary,
			CandidateEvent: &baseline.paired.summary,
		})
	}

	for _, candidate := range candidateEvents {
		if emittedCandidate[candidate.summary.ID] {
			continue
		}
		groups = append(groups, SessionCompareSemanticDiffGroup{
			EventType:      candidate.summary.EventType,
			DiffStatus:     SessionCompareDiffStatusCandidateOnly,
			ChangedFields:  []string{},
			BaselineEvent:  nil,
			CandidateEvent: &candidate.summary,
		})
	}

	return groups
}

func buildUnpairedSemanticGroups(
	events []*sessionCompareSemanticRecord,
	diffStatus SessionCompareDiffStatus,
) []SessionCompareSemanticDiffGroup {
	if len(events) == 0 {
		return nil
	}

	groups := make([]SessionCompareSemanticDiffGroup, 0, len(events))
	for _, event := range events {
		group := SessionCompareSemanticDiffGroup{
			EventType:     event.summary.EventType,
			DiffStatus:    diffStatus,
			ChangedFields: []string{},
		}
		if diffStatus == SessionCompareDiffStatusBaselineOnly {
			group.BaselineEvent = &event.summary
		} else {
			group.CandidateEvent = &event.summary
		}
		groups = append(groups, group)
	}
	return groups
}

func pairSemanticEvents(
	baselineEvents []*sessionCompareSemanticRecord,
	candidateEvents []*sessionCompareSemanticRecord,
	eventType SessionCompareSemanticEventType,
	keyFn func(*sessionCompareSemanticRecord) (string, bool),
	matchSource SessionCompareMatchSource,
	matchReason string,
) {
	baselineGroups := make(map[string][]*sessionCompareSemanticRecord)
	candidateGroups := make(map[string][]*sessionCompareSemanticRecord)

	for _, event := range baselineEvents {
		if event.summary.EventType != eventType || event.paired != nil {
			continue
		}
		key, ok := keyFn(event)
		if !ok {
			continue
		}
		baselineGroups[key] = append(baselineGroups[key], event)
	}

	for _, event := range candidateEvents {
		if event.summary.EventType != eventType || event.paired != nil {
			continue
		}
		key, ok := keyFn(event)
		if !ok {
			continue
		}
		candidateGroups[key] = append(candidateGroups[key], event)
	}

	for key, baselineGroup := range baselineGroups {
		candidateGroup := candidateGroups[key]
		if len(baselineGroup) != 1 || len(candidateGroup) != 1 {
			continue
		}

		source := matchSource
		reason := matchReason
		baselineGroup[0].paired = candidateGroup[0]
		baselineGroup[0].matchSource = &source
		baselineGroup[0].matchReason = &reason
		candidateGroup[0].paired = baselineGroup[0]
		candidateGroup[0].matchSource = &source
		candidateGroup[0].matchReason = &reason
	}
}

func compareSpanChangedFields(
	baseline *SessionCompareSpanSummary,
	candidate *SessionCompareSpanSummary,
	baselineSemanticCount int,
	candidateSemanticCount int,
) []string {
	changedFields := make([]string, 0, 6)

	if baseline.Status != candidate.Status {
		changedFields = append(changedFields, "status")
	}
	if compareInt64Value(baseline.LatencyMs) != compareInt64Value(candidate.LatencyMs) {
		changedFields = append(changedFields, "latency_ms")
	}
	if compareInt64Value(baseline.TokensIn) != compareInt64Value(candidate.TokensIn) {
		changedFields = append(changedFields, "tokens_in")
	}
	if compareInt64Value(baseline.TokensOut) != compareInt64Value(candidate.TokensOut) {
		changedFields = append(changedFields, "tokens_out")
	}
	if compareFloat32Value(baseline.CostUsd) != compareFloat32Value(candidate.CostUsd) {
		changedFields = append(changedFields, "cost_usd")
	}
	if baselineSemanticCount != candidateSemanticCount {
		changedFields = append(changedFields, "semantic_count")
	}

	return changedFields
}

func compareSemanticChangedFields(
	baseline *sessionCompareSemanticRecord,
	candidate *sessionCompareSemanticRecord,
) []string {
	switch baseline.summary.EventType {
	case SessionCompareSemanticEventTypeDecision:
		return compareDecisionChangedFields(baseline, candidate)
	case SessionCompareSemanticEventTypeEffect:
		return compareEffectChangedFields(baseline, candidate)
	case SessionCompareSemanticEventTypeWait:
		return compareWaitChangedFields(baseline, candidate)
	default:
		return nil
	}
}

func compareDecisionChangedFields(
	baseline *sessionCompareSemanticRecord,
	candidate *sessionCompareSemanticRecord,
) []string {
	changedFields := make([]string, 0, 4)
	if !reflect.DeepEqual(payloadFieldValue(baseline.payload, "chosen"), payloadFieldValue(candidate.payload, "chosen")) {
		changedFields = append(changedFields, "chosen")
	}
	if !reflect.DeepEqual(payloadFieldValue(baseline.payload, "reasoning"), payloadFieldValue(candidate.payload, "reasoning")) {
		changedFields = append(changedFields, "reasoning")
	}
	if !reflect.DeepEqual(payloadFieldValue(baseline.payload, "alternatives"), payloadFieldValue(candidate.payload, "alternatives")) {
		changedFields = append(changedFields, "alternatives")
	}
	if !reflect.DeepEqual(baseline.summary.Message, candidate.summary.Message) {
		changedFields = append(changedFields, "message")
	}
	return changedFields
}

func compareEffectChangedFields(
	baseline *sessionCompareSemanticRecord,
	candidate *sessionCompareSemanticRecord,
) []string {
	changedFields := make([]string, 0, 6)
	if !reflect.DeepEqual(payloadFieldValue(baseline.payload, "effect_kind"), payloadFieldValue(candidate.payload, "effect_kind")) {
		changedFields = append(changedFields, "effect_kind")
	}
	if !reflect.DeepEqual(payloadFieldValue(baseline.payload, "has_external_side_effect"), payloadFieldValue(candidate.payload, "has_external_side_effect")) {
		changedFields = append(changedFields, "has_external_side_effect")
	}
	if !reflect.DeepEqual(payloadFieldValue(baseline.payload, "idempotent"), payloadFieldValue(candidate.payload, "idempotent")) {
		changedFields = append(changedFields, "idempotent")
	}
	if !reflect.DeepEqual(payloadFieldValue(baseline.payload, "idempotency_key"), payloadFieldValue(candidate.payload, "idempotency_key")) {
		changedFields = append(changedFields, "idempotency_key")
	}
	if !reflect.DeepEqual(baseline.summary.Message, candidate.summary.Message) {
		changedFields = append(changedFields, "message")
	}
	if !reflect.DeepEqual(baseline.payload, candidate.payload) {
		changedFields = append(changedFields, "payload")
	}
	return changedFields
}

func compareWaitChangedFields(
	baseline *sessionCompareSemanticRecord,
	candidate *sessionCompareSemanticRecord,
) []string {
	changedFields := make([]string, 0, 5)
	if !reflect.DeepEqual(payloadFieldValue(baseline.payload, "wait_kind"), payloadFieldValue(candidate.payload, "wait_kind")) {
		changedFields = append(changedFields, "wait_kind")
	}
	if !reflect.DeepEqual(payloadFieldValue(baseline.payload, "phase"), payloadFieldValue(candidate.payload, "phase")) {
		changedFields = append(changedFields, "phase")
	}
	if !reflect.DeepEqual(payloadFieldValue(baseline.payload, "resolution"), payloadFieldValue(candidate.payload, "resolution")) {
		changedFields = append(changedFields, "resolution")
	}
	if !reflect.DeepEqual(baseline.summary.Message, candidate.summary.Message) {
		changedFields = append(changedFields, "message")
	}
	if !reflect.DeepEqual(baseline.payload, candidate.payload) {
		changedFields = append(changedFields, "payload")
	}
	return changedFields
}

func semanticGroupsHaveChanges(groups []SessionCompareSemanticDiffGroup) bool {
	for _, group := range groups {
		if group.DiffStatus != SessionCompareDiffStatusUnchanged {
			return true
		}
	}
	return false
}

func resetCompareSemanticPairs(events []*sessionCompareSemanticRecord) {
	for _, event := range events {
		event.paired = nil
		event.matchSource = nil
		event.matchReason = nil
		event.candidateEmitted = false
	}
}

func (b *sessionComparisonBuilder) assignSpanPair(
	baseline *sessionCompareSpanRecord,
	candidate *sessionCompareSpanRecord,
	matchSource SessionCompareMatchSource,
	matchReason string,
) {
	source := matchSource
	reason := matchReason
	pairID := fmt.Sprintf("%s|%s", baseline.summary.SpanID, candidate.summary.SpanID)
	baseline.matched = candidate
	baseline.matchSource = &source
	baseline.matchReason = &reason
	baseline.pairID = pairID
	candidate.matched = baseline
	candidate.matchSource = &source
	candidate.matchReason = &reason
	candidate.pairID = pairID
}

func (s *sessionCompareSpanRecord) heuristicScope(bySpanID map[string]*sessionCompareSpanRecord) (string, bool) {
	if s.parentSpanID == nil {
		return sessionCompareRootScope, true
	}

	parent := bySpanID[*s.parentSpanID]
	if parent == nil {
		return sessionCompareRootScope, true
	}
	if parent.pairID != "" {
		return parent.pairID, true
	}

	// Descendants under unmatched branches stay out of pass-2 matching until their
	// direct parent is matched, which prevents cross-branch guesses.
	return "", false
}

func newSessionCompareSpanRecord(row *platform.ListCompareSpansRow) *sessionCompareSpanRecord {
	latencyMs := row.DurationMs
	if latencyMs == nil && row.EndTime.Valid {
		calculated := int64(row.EndTime.Time.Sub(row.StartTime) / time.Millisecond)
		latencyMs = &calculated
	}

	summary := SessionCompareSpanSummary{
		ID:           row.ID,
		SpanID:       row.SpanID,
		ParentSpanID: row.ParentSpanID,
		Name:         row.Name,
		Kind:         normalizeCompareSpanKind(row.Type),
		Status:       normalizeCompareSpanStatus(row.Status),
		StartedAt:    row.StartTime,
		TokensIn:     row.PromptTokens,
		TokensOut:    row.CompletionTokens,
		LatencyMs:    latencyMs,
		ErrorMessage: row.StatusMessage,
		Model:        row.Model,
	}

	if row.EndTime.Valid {
		endedAt := row.EndTime.Time
		summary.EndedAt = &endedAt
	}
	if row.TotalCost.Valid {
		if costUsd, err := compareNumericToFloat32(row.TotalCost); err == nil {
			summary.CostUsd = &costUsd
		}
	}

	return &sessionCompareSpanRecord{
		summary:        summary,
		traceID:        row.TraceID,
		rawKind:        strings.ToLower(strings.TrimSpace(row.Type)),
		normalizedName: normalizeSessionCompareText(row.Name),
		parentSpanID:   row.ParentSpanID,
	}
}

func newSessionCompareSemanticRecord(row *platform.ListCompareSemanticEventsRow) *sessionCompareSemanticRecord {
	eventType := SessionCompareSemanticEventType(strings.ToLower(strings.TrimSpace(row.EventType)))
	spanID := row.SpanID

	record := &sessionCompareSemanticRecord{
		traceID: row.TraceID,
		spanID:  row.SpanID,
		payload: parseComparePayload(row.Payload),
		summary: SessionCompareSemanticSummary{
			ID:        row.ID.String(),
			SpanID:    &spanID,
			SpanName:  row.SpanName,
			EventType: eventType,
			Timestamp: compareEventTimestamp(row.EventTs, row.ServerIngestedAt),
			Message:   row.Message,
		},
	}

	if len(record.payload) > 0 {
		record.summary.Payload = record.payload
	}

	return record
}

func groupSessionCompareChildren(
	spans []*sessionCompareSpanRecord,
	bySpanID map[string]*sessionCompareSpanRecord,
) map[string][]*sessionCompareSpanRecord {
	children := make(map[string][]*sessionCompareSpanRecord)
	for _, span := range spans {
		parentKey := sessionCompareRootScope
		if span.parentSpanID != nil {
			if _, ok := bySpanID[*span.parentSpanID]; ok {
				parentKey = *span.parentSpanID
			}
		}
		children[parentKey] = append(children[parentKey], span)
	}
	return children
}

func compareTraceHeaderFromValidationRow(
	id uuid.UUID,
	traceID string,
	name *string,
	status string,
	userID *string,
	startedAt time.Time,
	endedAt pgtype.Timestamptz,
	durationMs *int64,
	errorCount *int32,
	totalCost pgtype.Numeric,
	totalTokensIn int64,
	totalTokensOut int64,
) SessionCompareTraceHeader {
	header := SessionCompareTraceHeader{
		ID:         id,
		TraceID:    traceID,
		Name:       derefString(name),
		Status:     normalizeCompareTraceStatus(status),
		UserID:     userID,
		StartedAt:  startedAt,
		DurationMs: durationMs,
	}

	if endedAt.Valid {
		value := endedAt.Time
		header.EndedAt = &value
	}
	if errorCount != nil {
		value := int(*errorCount)
		header.ErrorCount = &value
	}
	if totalCost.Valid {
		if costUsd, err := compareNumericToFloat32(totalCost); err == nil {
			header.TotalCostUsd = &costUsd
		}
	}

	totalIn := totalTokensIn
	header.TotalTokensIn = &totalIn
	totalOut := totalTokensOut
	header.TotalTokensOut = &totalOut

	return header
}

func countCompareRows(rows []platform.GetCompareSpanCountsRow, traceID uuid.UUID) int {
	for _, row := range rows {
		if row.TraceID == traceID {
			return int(row.SpanCount)
		}
	}
	return 0
}

func countCompareSemanticRows(rows []platform.GetCompareSemanticCountsRow, traceID uuid.UUID) int {
	for _, row := range rows {
		if row.TraceID == traceID {
			return int(row.SemanticCount)
		}
	}
	return 0
}

func countMatchedCompareSpans(spans []*sessionCompareSpanRecord) int {
	count := 0
	for _, span := range spans {
		if span.matched != nil {
			count++
		}
	}
	return count
}

func countUnmatchedCompareSpans(spans []*sessionCompareSpanRecord) int {
	count := 0
	for _, span := range spans {
		if span.matched == nil {
			count++
		}
	}
	return count
}

func semanticDecisionKey(event *sessionCompareSemanticRecord) (string, bool) {
	return normalizeSessionCompareStringField(event.payload, "question")
}

func semanticEffectKey(event *sessionCompareSemanticRecord) (string, bool) {
	return sessionCompareStableIDField(event.payload, "effect_id")
}

func semanticWaitKey(event *sessionCompareSemanticRecord) (string, bool) {
	return sessionCompareStableIDField(event.payload, "wait_id")
}

func payloadFieldValue(payload map[string]interface{}, key string) interface{} {
	if payload == nil {
		return nil
	}
	return payload[key]
}

func normalizeSessionCompareStringField(payload map[string]interface{}, key string) (string, bool) {
	value, ok := sessionCompareStringField(payload, key)
	if !ok {
		return "", false
	}
	value = normalizeSessionCompareText(value)
	if value == "" {
		return "", false
	}
	return value, true
}

func sessionCompareStableIDField(payload map[string]interface{}, key string) (string, bool) {
	if payload == nil {
		return "", false
	}
	raw, ok := payload[key]
	if !ok {
		return "", false
	}
	value, ok := raw.(string)
	if !ok {
		return "", false
	}
	if strings.TrimSpace(value) == "" {
		return "", false
	}
	return value, true
}

func sessionCompareStringField(payload map[string]interface{}, key string) (string, bool) {
	if payload == nil {
		return "", false
	}
	raw, ok := payload[key]
	if !ok {
		return "", false
	}
	value, ok := raw.(string)
	if !ok {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	return value, true
}

func normalizeSessionCompareText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
}

func normalizeCompareTraceStatus(status string) string {
	switch NormalizeTraceStatus(status) {
	case TraceStatusBucketCompleted:
		return "COMPLETED"
	case TraceStatusBucketFailed:
		return "FAILED"
	default:
		return "RUNNING"
	}
}

func normalizeCompareSpanKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "llm":
		return "LLM"
	case "tool":
		return "TOOL"
	case "chain":
		return "CHAIN"
	case "agent":
		return "AGENT"
	default:
		return "CUSTOM"
	}
}

func normalizeCompareSpanStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running":
		return "STARTED"
	case "completed":
		return "COMPLETED"
	case "failed", "error":
		return "FAILED"
	default:
		return "SCHEDULED"
	}
}

func compareEventTimestamp(eventTS pgtype.Timestamptz, serverIngestedAt time.Time) time.Time {
	if eventTS.Valid {
		return eventTS.Time
	}
	return serverIngestedAt
}

func parseComparePayload(data []byte) map[string]interface{} {
	if len(data) == 0 {
		return nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	return payload
}

func compareNumericToFloat32(value pgtype.Numeric) (float32, error) {
	floatValue, err := value.Float64Value()
	if err != nil {
		return 0, err
	}
	if !floatValue.Valid {
		return 0, nil
	}
	return float32(floatValue.Float64), nil
}

func compareInt64Value(value *int64) int64 {
	if value == nil {
		return 0
	}
	return *value
}

func compareFloat32Value(value *float32) float32 {
	if value == nil {
		return 0
	}
	return *value
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
