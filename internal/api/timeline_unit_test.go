package api

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	openapi_types "github.com/oapi-codegen/runtime/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

func TestBuildTimelineEntries_EmptyInputs(t *testing.T) {
	assert.Empty(t, buildTimelineEntries(nil, nil))
}

func TestBuildTimelineEntries_MixedSources(t *testing.T) {
	traceID := uuid.New()
	base := time.Date(2026, 3, 7, 10, 0, 0, 0, time.UTC)
	sequence := int32(2)
	message := "alpha log"

	entries := buildTimelineEntries(
		[]platform.SpanEvent{
			{
				ID:               uuid.New(),
				TraceID:          traceID,
				SpanID:           "alpha",
				EventType:        "log",
				Level:            "info",
				EventTs:          pgtype.Timestamptz{Time: base, Valid: true},
				ServerIngestedAt: base.Add(10 * time.Millisecond),
				Sequence:         &sequence,
				Message:          &message,
				CreatedAt:        base.Add(20 * time.Millisecond),
			},
		},
		[]platform.Span{
			{
				ID:        uuid.New(),
				TraceID:   traceID,
				SpanID:    "alpha",
				Name:      "Alpha Span",
				Status:    "completed",
				StartTime: base,
				EndTime:   pgtype.Timestamptz{Time: base.Add(time.Second), Valid: true},
				CreatedAt: base.Add(30 * time.Millisecond),
			},
		},
	)

	require.Len(t, entries, 3)
	assert.Equal(t, Explicit, entries[0].event.Source)
	assert.Equal(t, TimelineEventTypeLog, entries[0].event.EventType)
	require.NotNil(t, entries[0].event.Sequence)
	assert.Equal(t, sequence, *entries[0].event.Sequence)
	require.NotNil(t, entries[0].event.SpanName)
	assert.Equal(t, "Alpha Span", *entries[0].event.SpanName)

	assert.Equal(t, Synthetic, entries[1].event.Source)
	assert.Equal(t, TimelineEventTypeSpanStarted, entries[1].event.EventType)
	assert.Equal(t, Synthetic, entries[2].event.Source)
	assert.Equal(t, TimelineEventTypeSpanCompleted, entries[2].event.EventType)
}

func TestPaginateTimelineEntries_EmptyAndCursorAtEnd(t *testing.T) {
	emptyPage, err := paginateTimelineEntries(nil, nil, 10)
	require.NoError(t, err)
	assert.Empty(t, emptyPage.page)
	assert.False(t, emptyPage.hasMore)
	assert.Nil(t, emptyPage.nextCursor)
	assert.Nil(t, emptyPage.pollCursor)

	base := time.Date(2026, 3, 7, 11, 0, 0, 0, time.UTC)
	entries := []timelineEntry{
		unitTimelineEntry("event-a", Explicit, TimelineEventTypeLog, base, base.Add(time.Millisecond), int32Ptr(1)),
		unitTimelineEntry("event-b", Explicit, TimelineEventTypeMessage, base.Add(time.Second), base.Add(2*time.Millisecond), int32Ptr(2)),
	}

	fullPage, err := paginateTimelineEntries(entries, nil, 10)
	require.NoError(t, err)
	require.Len(t, fullPage.page, 2)
	assert.False(t, fullPage.hasMore)
	assert.Nil(t, fullPage.nextCursor)
	require.NotNil(t, fullPage.pollCursor)

	endPage, err := paginateTimelineEntries(entries, fullPage.pollCursor, 10)
	require.NoError(t, err)
	assert.Empty(t, endPage.page)
	assert.False(t, endPage.hasMore)
	assert.Nil(t, endPage.nextCursor)
	require.NotNil(t, endPage.pollCursor)
	assert.Equal(t, *fullPage.pollCursor, *endPage.pollCursor)
}

func TestEncodeDecodeTimelineCursor_RoundTrip(t *testing.T) {
	base := time.Date(2026, 3, 7, 12, 0, 0, 0, time.UTC)
	entry := unitTimelineEntry("event-a", Explicit, TimelineEventTypeLog, base, base.Add(time.Millisecond), int32Ptr(3))

	encoded := encodeTimelineCursor(entry)
	decoded, err := decodeTimelineCursor(encoded)
	require.NoError(t, err)

	assert.Equal(t, entry.event.Id, decoded.ID)
	assert.Equal(t, string(entry.event.Source), decoded.Source)
	assert.Equal(t, entry.cursorTimestamp.UTC().Format(time.RFC3339Nano), decoded.CursorTimestamp)
}

func TestSortTimelineEntriesByDisplay_TieBreaksBySourceSequencePhaseAndID(t *testing.T) {
	base := time.Date(2026, 3, 7, 13, 0, 0, 0, time.UTC)
	entries := []timelineEntry{
		unitTimelineEntry("explicit-seq-5", Explicit, TimelineEventTypeLog, base, base.Add(5*time.Millisecond), int32Ptr(5)),
		unitTimelineEntry("synthetic-start", Synthetic, TimelineEventTypeSpanStarted, base, base.Add(6*time.Millisecond), nil),
		unitTimelineEntry("explicit-seq-1", Explicit, TimelineEventTypeLog, base, base.Add(4*time.Millisecond), int32Ptr(1)),
		unitTimelineEntry("explicit-nil-seq", Explicit, TimelineEventTypeMessage, base, base.Add(7*time.Millisecond), nil),
	}

	sortTimelineEntriesByDisplay(entries)

	gotIDs := []string{
		entries[0].event.Id,
		entries[1].event.Id,
		entries[2].event.Id,
		entries[3].event.Id,
	}
	assert.Equal(t, []string{
		"explicit-seq-1",
		"explicit-seq-5",
		"explicit-nil-seq",
		"synthetic-start",
	}, gotIDs)
}

func unitTimelineEntry(
	id string,
	source TimelineEventSource,
	eventType TimelineEventType,
	displayTimestamp time.Time,
	cursorTimestamp time.Time,
	sequence *int32,
) timelineEntry {
	traceID := uuid.New()
	event := TimelineEvent{
		Id:        id,
		TraceId:   openapi_types.UUID(traceID),
		EventType: eventType,
		Source:    source,
		Timestamp: displayTimestamp,
		Sequence:  sequence,
	}

	entry := timelineEntry{
		event:           event,
		cursorTimestamp: cursorTimestamp,
		spanID:          id,
	}
	if source == Explicit {
		entry.explicitSequence = sequence
	}
	if source == Synthetic {
		entry.syntheticPhase = timelinePhaseForEventType(eventType)
	}

	return entry
}

func timelinePhaseForEventType(eventType TimelineEventType) int {
	if eventType == TimelineEventTypeSpanStarted {
		return timelinePhaseStarted
	}
	return timelinePhaseTerminal
}

func int32Ptr(value int32) *int32 {
	return &value
}
