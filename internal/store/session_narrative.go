package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

// SessionNarrative is the store read model for the session narrative endpoint.
type SessionNarrative struct {
	Summary SessionNarrativeSummary
	Traces  []SessionNarrativeTrace
}

// SessionNarrativeSummary contains the aggregate narrative summary for a session.
type SessionNarrativeSummary struct {
	TotalTraceCount     int64
	ReturnedTraceCount  int64
	Truncated           bool
	RunningTraceCount   int64
	CompletedTraceCount int64
	FailedTraceCount    int64
	TotalTokensIn       int64
	TotalTokensOut      int64
	TotalCostUsd        pgtype.Numeric
	StartedAt           *time.Time
	LastActivityAt      *time.Time
	ExplicitLinkCount   int64
	InferredLinkCount   int64
	UnlinkedTraceCount  int64
}

// SessionNarrativeTrace is the store read model for one narrative trace card.
type SessionNarrativeTrace struct {
	ID               uuid.UUID
	TraceID          string
	Name             *string
	Status           string
	UserID           *string
	StartedAt        time.Time
	EndedAt          *time.Time
	DurationMs       *int64
	ErrorCount       int32
	TotalCostUsd     pgtype.Numeric
	TotalTokensIn    int64
	TotalTokensOut   int64
	LatestActivityAt time.Time
	SemanticEvents   []SessionNarrativeSemanticEvent
	Lineage          SessionNarrativeLineage
	Metadata         json.RawMessage
}

// SessionNarrativeSemanticEvent is an explicit semantic timeline event with optional span name context.
type SessionNarrativeSemanticEvent struct {
	platform.SpanEvent
	SpanName *string
}

// SessionNarrativeLineageType identifies how the narrative parent relationship was derived.
type SessionNarrativeLineageType string

const (
	SessionNarrativeLineageTypeExplicit SessionNarrativeLineageType = "explicit"
	SessionNarrativeLineageTypeInferred SessionNarrativeLineageType = "inferred"
	SessionNarrativeLineageTypeUnlinked SessionNarrativeLineageType = "unlinked"
)

// SessionNarrativeLineage captures the resolved lineage metadata for a trace in the shown narrative.
type SessionNarrativeLineage struct {
	Type          SessionNarrativeLineageType
	ParentTraceID *string
	TriggerSpanID *string
	LinkKind      *string
}

type sessionNarrativeExplicitLink struct {
	ParentTraceID string
	TriggerSpanID *string
	LinkKind      *string
}

// BuildSessionNarrative returns the session narrative read model for a scoped session.
func (s *Store) BuildSessionNarrative(ctx context.Context, projectID, sessionID uuid.UUID, limit int32) (SessionNarrative, error) {
	summaryRow, err := s.q.GetSessionNarrativeSummary(ctx, platform.GetSessionNarrativeSummaryParams{
		ID:        sessionID,
		ProjectID: projectID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return SessionNarrative{}, ErrNotFound
	}
	if err != nil {
		return SessionNarrative{}, fmt.Errorf("get session narrative summary: %w", err)
	}

	summary, err := sessionNarrativeSummaryFromRow(&summaryRow)
	if err != nil {
		return SessionNarrative{}, err
	}

	traceRows, err := s.q.ListSessionNarrativeTraces(ctx, platform.ListSessionNarrativeTracesParams{
		SessionID: pgtype.UUID{Bytes: sessionID, Valid: true},
		ProjectID: projectID,
		Limit:     limit,
	})
	if err != nil {
		return SessionNarrative{}, fmt.Errorf("list session narrative traces: %w", err)
	}

	narrative := SessionNarrative{
		Summary: summary,
		Traces:  make([]SessionNarrativeTrace, len(traceRows)),
	}
	narrative.Summary.ReturnedTraceCount = int64(len(traceRows))
	narrative.Summary.Truncated = narrative.Summary.TotalTraceCount > narrative.Summary.ReturnedTraceCount

	traceIDs := make([]uuid.UUID, 0, len(traceRows))
	tracesByID := make(map[uuid.UUID]*SessionNarrativeTrace, len(traceRows))

	for i := range traceRows {
		narrativeTrace := sessionNarrativeTraceFromRow(&traceRows[i])
		narrative.Traces[i] = narrativeTrace
		traceIDs = append(traceIDs, narrativeTrace.ID)
		tracesByID[narrativeTrace.ID] = &narrative.Traces[i]
	}

	if len(traceIDs) > 0 {
		eventRows, err := s.q.ListSessionNarrativeSemanticEvents(ctx, traceIDs)
		if err != nil {
			return SessionNarrative{}, fmt.Errorf("list session narrative semantic events: %w", err)
		}

		for i := range eventRows {
			trace := tracesByID[eventRows[i].SpanEvent.TraceID]
			if trace == nil {
				continue
			}
			trace.SemanticEvents = append(trace.SemanticEvents, SessionNarrativeSemanticEvent{
				SpanEvent: eventRows[i].SpanEvent,
				SpanName:  eventRows[i].SpanName,
			})
		}
	}

	resolveSessionNarrativeLineage(&narrative)
	return narrative, nil
}

func sessionNarrativeSummaryFromRow(row *platform.GetSessionNarrativeSummaryRow) (SessionNarrativeSummary, error) {
	totalTokensIn, err := int64FromAny(row.TotalTokensIn)
	if err != nil {
		return SessionNarrativeSummary{}, fmt.Errorf("decode narrative total_tokens_in: %w", err)
	}

	totalTokensOut, err := int64FromAny(row.TotalTokensOut)
	if err != nil {
		return SessionNarrativeSummary{}, fmt.Errorf("decode narrative total_tokens_out: %w", err)
	}

	return SessionNarrativeSummary{
		TotalTraceCount:     row.TotalTraceCount,
		RunningTraceCount:   row.RunningTraceCount,
		CompletedTraceCount: row.CompletedTraceCount,
		FailedTraceCount:    row.FailedTraceCount,
		TotalTokensIn:       totalTokensIn,
		TotalTokensOut:      totalTokensOut,
		TotalCostUsd:        row.TotalCostUsd,
		StartedAt:           nullableNarrativeTimeFromAny(row.StartedAt),
		LastActivityAt:      nullableNarrativeTimeFromAny(row.LastActivityAt),
	}, nil
}

func sessionNarrativeTraceFromRow(row *platform.ListSessionNarrativeTracesRow) SessionNarrativeTrace {
	trace := SessionNarrativeTrace{
		ID:               row.ID,
		TraceID:          row.TraceID,
		Name:             row.Name,
		Status:           row.Status,
		UserID:           row.UserID,
		StartedAt:        row.StartedAt,
		ErrorCount:       derefInt32(row.ErrorCount),
		TotalCostUsd:     row.TotalCostUsd,
		TotalTokensIn:    row.TotalTokensIn,
		TotalTokensOut:   row.TotalTokensOut,
		LatestActivityAt: row.LatestActivityAt,
		Metadata:         append(json.RawMessage(nil), row.Metadata...),
		Lineage: SessionNarrativeLineage{
			Type: SessionNarrativeLineageTypeUnlinked,
		},
	}

	if row.EndedAt.Valid {
		endedAt := row.EndedAt.Time
		trace.EndedAt = &endedAt
	}
	if row.DurationMs >= 0 {
		durationMs := row.DurationMs
		trace.DurationMs = &durationMs
	}

	return trace
}

func resolveSessionNarrativeLineage(narrative *SessionNarrative) {
	if narrative == nil || len(narrative.Traces) == 0 {
		return
	}

	indexByExternalTraceID := make(map[string]int, len(narrative.Traces))
	for i := range narrative.Traces {
		indexByExternalTraceID[narrative.Traces[i].TraceID] = i
	}

	// Explicit metadata is the highest-precedence slice for non-root traces.
	for i := 1; i < len(narrative.Traces); i++ {
		explicit := parseSessionNarrativeExplicitLink(narrative.Traces[i].Metadata)
		if explicit == nil {
			continue
		}

		parentIndex, ok := indexByExternalTraceID[explicit.ParentTraceID]
		if !ok || parentIndex >= i {
			continue
		}

		parentTraceID := narrative.Traces[parentIndex].TraceID
		narrative.Traces[i].Lineage = SessionNarrativeLineage{
			Type:          SessionNarrativeLineageTypeExplicit,
			ParentTraceID: &parentTraceID,
			TriggerSpanID: explicit.TriggerSpanID,
			LinkKind:      explicit.LinkKind,
		}
	}

	for i := 1; i < len(narrative.Traces); i++ {
		if narrative.Traces[i].Lineage.Type == SessionNarrativeLineageTypeExplicit {
			continue
		}

		parentTraceID := inferSessionNarrativeParentTraceID(narrative.Traces, i)
		if parentTraceID == nil {
			continue
		}

		narrative.Traces[i].Lineage = SessionNarrativeLineage{
			Type:          SessionNarrativeLineageTypeInferred,
			ParentTraceID: parentTraceID,
		}
	}

	for i := range narrative.Traces {
		switch narrative.Traces[i].Lineage.Type {
		case SessionNarrativeLineageTypeExplicit:
			narrative.Summary.ExplicitLinkCount++
		case SessionNarrativeLineageTypeInferred:
			narrative.Summary.InferredLinkCount++
		default:
			narrative.Summary.UnlinkedTraceCount++
		}
	}
}

func inferSessionNarrativeParentTraceID(traces []SessionNarrativeTrace, childIndex int) *string {
	if childIndex == 0 {
		return nil
	}

	child := traces[childIndex]
	predecessor := traces[childIndex-1]

	if child.StartedAt.IsZero() || predecessor.LatestActivityAt.IsZero() {
		return nil
	}
	if !child.StartedAt.After(predecessor.LatestActivityAt) {
		return nil
	}

	for i := 0; i < childIndex-1; i++ {
		other := traces[i]
		if other.StartedAt.IsZero() || other.LatestActivityAt.IsZero() {
			continue
		}
		if !other.StartedAt.Before(child.StartedAt) {
			continue
		}
		if !other.LatestActivityAt.Before(child.StartedAt) {
			return nil
		}
	}

	parentTraceID := predecessor.TraceID
	return &parentTraceID
}

func parseSessionNarrativeExplicitLink(metadata json.RawMessage) *sessionNarrativeExplicitLink {
	if len(metadata) == 0 {
		return nil
	}

	var payload map[string]any
	if err := json.Unmarshal(metadata, &payload); err != nil {
		return nil
	}

	lineageValue, ok := payload["__continua_lineage"]
	if !ok {
		return nil
	}

	lineageMap, ok := lineageValue.(map[string]any)
	if !ok {
		return nil
	}

	parentTraceID, ok := lineageMap["parent_trace_id"].(string)
	parentTraceID = strings.TrimSpace(parentTraceID)
	if !ok || parentTraceID == "" {
		return nil
	}

	link := &sessionNarrativeExplicitLink{ParentTraceID: parentTraceID}
	if triggerSpanID := optionalTrimmedString(lineageMap["trigger_span_id"]); triggerSpanID != nil {
		link.TriggerSpanID = triggerSpanID
	}
	if linkKind := optionalTrimmedString(lineageMap["link_kind"]); linkKind != nil {
		link.LinkKind = linkKind
	}

	return link
}

func optionalTrimmedString(value any) *string {
	raw, ok := value.(string)
	if !ok {
		return nil
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func nullableNarrativeTime(ts time.Time) *time.Time {
	if ts.IsZero() {
		return nil
	}
	value := ts
	return &value
}

func nullableNarrativeTimeFromAny(value any) *time.Time {
	switch v := value.(type) {
	case nil:
		return nil
	case time.Time:
		return nullableNarrativeTime(v)
	case *time.Time:
		if v == nil {
			return nil
		}
		return nullableNarrativeTime(*v)
	case pgtype.Timestamptz:
		if !v.Valid {
			return nil
		}
		return nullableNarrativeTime(v.Time)
	case *pgtype.Timestamptz:
		if v == nil || !v.Valid {
			return nil
		}
		return nullableNarrativeTime(v.Time)
	default:
		return nil
	}
}

func derefInt32(value *int32) int32 {
	if value == nil {
		return 0
	}
	return *value
}

func int64FromAny(value any) (int64, error) {
	switch v := value.(type) {
	case nil:
		return 0, nil
	case int64:
		return v, nil
	case int32:
		return int64(v), nil
	case int:
		return int64(v), nil
	case uint64:
		if v > math.MaxInt64 {
			return 0, fmt.Errorf("uint64 %d exceeds int64", v)
		}
		return int64(v), nil
	case float64:
		return int64(v), nil
	case string:
		return strconv.ParseInt(v, 10, 64)
	case []byte:
		return strconv.ParseInt(string(v), 10, 64)
	case pgtype.Int8:
		if !v.Valid {
			return 0, nil
		}
		return v.Int64, nil
	default:
		return 0, fmt.Errorf("unsupported int64 source type %T", value)
	}
}
