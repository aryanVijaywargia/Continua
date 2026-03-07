package api

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

const (
	defaultTimelineLimit = int32(100)
	maxTimelineLimit     = int32(500)

	timelinePhaseStarted = iota
	timelinePhaseTerminal
)

var errInvalidTimelineCursor = errors.New("invalid timeline cursor")

type timelineEntry struct {
	event            TimelineEvent
	cursorTimestamp  time.Time
	explicitSequence *int32
	syntheticPhase   int
	spanID           string
}

type timelineCursor struct {
	CursorTimestamp string `json:"cts"`
	Source          string `json:"src"`
	ID              string `json:"id"`
}

type paginatedTimelineEntries struct {
	page       []timelineEntry
	hasMore    bool
	nextCursor *string
	pollCursor *string
}

func buildTimelineEntries(explicitEvents []platform.SpanEvent, spans []platform.Span) []timelineEntry {
	spanNames := make(map[string]string, len(spans))
	entries := make([]timelineEntry, 0, len(explicitEvents)+(len(spans)*2))

	for i := range spans {
		spanNames[spans[i].SpanID] = spans[i].Name
	}

	for i := range explicitEvents {
		var spanName *string
		if name, ok := spanNames[explicitEvents[i].SpanID]; ok {
			nameCopy := name
			spanName = &nameCopy
		}

		entries = append(entries, timelineEntry{
			event:            explicitTimelineEventToAPI(&explicitEvents[i], spanName),
			cursorTimestamp:  explicitEvents[i].CreatedAt,
			explicitSequence: explicitEvents[i].Sequence,
			spanID:           explicitEvents[i].SpanID,
		})
	}

	for i := range spans {
		entries = append(entries, syntheticTimelineEntriesFromSpan(&spans[i])...)
	}

	return entries
}

func syntheticTimelineEntriesFromSpan(sp *platform.Span) []timelineEntry {
	entries := make([]timelineEntry, 0, 2)

	if !sp.StartTime.IsZero() {
		entries = append(entries, timelineEntry{
			event:           syntheticTimelineEventToAPI(sp, TimelineEventTypeSpanStarted, sp.StartTime),
			cursorTimestamp: sp.CreatedAt,
			syntheticPhase:  timelinePhaseStarted,
			spanID:          sp.SpanID,
		})
	}

	if !sp.EndTime.Valid {
		return entries
	}

	switch sp.Status {
	case "completed":
		entries = append(entries, timelineEntry{
			event:           syntheticTimelineEventToAPI(sp, TimelineEventTypeSpanCompleted, sp.EndTime.Time),
			cursorTimestamp: sp.CreatedAt,
			syntheticPhase:  timelinePhaseTerminal,
			spanID:          sp.SpanID,
		})
	case "failed", "error":
		entries = append(entries, timelineEntry{
			event:           syntheticTimelineEventToAPI(sp, TimelineEventTypeSpanFailed, sp.EndTime.Time),
			cursorTimestamp: sp.CreatedAt,
			syntheticPhase:  timelinePhaseTerminal,
			spanID:          sp.SpanID,
		})
	}

	return entries
}

func paginateTimelineEntries(entries []timelineEntry, after *string, limit int) (paginatedTimelineEntries, error) {
	cursorOrdered := append([]timelineEntry(nil), entries...)
	sortTimelineEntriesByCursor(cursorOrdered)

	startIndex := 0
	if after != nil {
		cursor, err := decodeTimelineCursor(*after)
		if err != nil {
			return paginatedTimelineEntries{}, errInvalidTimelineCursor
		}

		cursorIndex := resolveTimelineCursorIndex(cursorOrdered, cursor)
		if cursorIndex < 0 {
			return paginatedTimelineEntries{}, errInvalidTimelineCursor
		}

		startIndex = cursorIndex + 1
	}

	if startIndex > len(cursorOrdered) {
		startIndex = len(cursorOrdered)
	}

	endIndex := startIndex + limit
	if endIndex > len(cursorOrdered) {
		endIndex = len(cursorOrdered)
	}

	pageCursorOrdered := append([]timelineEntry(nil), cursorOrdered[startIndex:endIndex]...)
	pageDisplayOrdered := append([]timelineEntry(nil), pageCursorOrdered...)
	sortTimelineEntriesByDisplay(pageDisplayOrdered)

	hasMore := endIndex < len(cursorOrdered)
	var nextCursor *string
	if hasMore && len(pageCursorOrdered) > 0 {
		cursor := encodeTimelineCursor(pageCursorOrdered[len(pageCursorOrdered)-1])
		nextCursor = &cursor
	}

	return paginatedTimelineEntries{
		page:       pageDisplayOrdered,
		hasMore:    hasMore,
		nextCursor: nextCursor,
		pollCursor: timelinePollCursor(pageCursorOrdered, after),
	}, nil
}

func sortTimelineEntriesByDisplay(entries []timelineEntry) {
	sortTimelineEntries(entries, compareTimelineEntriesByDisplay)
}

func sortTimelineEntriesByCursor(entries []timelineEntry) {
	sortTimelineEntries(entries, compareTimelineEntriesByCursor)
}

func sortTimelineEntries(entries []timelineEntry, compare func(a, b timelineEntry) int) {
	slices.SortFunc(entries, compare)
}

func compareTimelineEntriesByDisplay(a, b timelineEntry) int {
	if cmp := compareTimes(a.event.Timestamp, b.event.Timestamp); cmp != 0 {
		return cmp
	}
	if cmp := compareTimelineSources(a.event.Source, b.event.Source); cmp != 0 {
		return cmp
	}

	switch {
	case a.event.Source == Explicit && b.event.Source == Explicit:
		if cmp := compareNullableInt32(a.explicitSequence, b.explicitSequence); cmp != 0 {
			return cmp
		}
	case a.event.Source == Synthetic && b.event.Source == Synthetic:
		if cmp := compareInts(a.syntheticPhase, b.syntheticPhase); cmp != 0 {
			return cmp
		}
	}

	return compareStrings(a.event.Id, b.event.Id)
}

func compareTimelineEntriesByCursor(a, b timelineEntry) int {
	if cmp := compareTimes(a.cursorTimestamp, b.cursorTimestamp); cmp != 0 {
		return cmp
	}
	if cmp := compareTimelineSources(a.event.Source, b.event.Source); cmp != 0 {
		return cmp
	}

	switch {
	case a.event.Source == Explicit && b.event.Source == Explicit:
		if cmp := compareNullableInt32(a.explicitSequence, b.explicitSequence); cmp != 0 {
			return cmp
		}
		return compareStrings(a.event.Id, b.event.Id)
	case a.event.Source == Synthetic && b.event.Source == Synthetic:
		if cmp := compareInts(a.syntheticPhase, b.syntheticPhase); cmp != 0 {
			return cmp
		}
		return compareStrings(a.spanID, b.spanID)
	default:
		return compareStrings(a.event.Id, b.event.Id)
	}
}

func compareTimelineSources(a, b TimelineEventSource) int {
	return compareInts(timelineSourceRank(a), timelineSourceRank(b))
}

func timelineSourceRank(source TimelineEventSource) int {
	if source == Explicit {
		return 0
	}
	return 1
}

func compareNullableInt32(a, b *int32) int {
	switch {
	case a == nil && b == nil:
		return 0
	case a == nil:
		return 1
	case b == nil:
		return -1
	default:
		return compareInts(int(*a), int(*b))
	}
}

func compareTimes(a, b time.Time) int {
	switch {
	case a.Before(b):
		return -1
	case a.After(b):
		return 1
	default:
		return 0
	}
}

func compareInts(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}

func timelinePollCursor(pageCursorOrdered []timelineEntry, after *string) *string {
	if len(pageCursorOrdered) > 0 {
		cursor := encodeTimelineCursor(pageCursorOrdered[len(pageCursorOrdered)-1])
		return &cursor
	}
	if after == nil || *after == "" {
		return nil
	}

	cursor := *after
	return &cursor
}

func encodeTimelineCursor(entry timelineEntry) string {
	payload, _ := json.Marshal(timelineCursor{
		CursorTimestamp: entry.cursorTimestamp.UTC().Format(time.RFC3339Nano),
		Source:          string(entry.event.Source),
		ID:              entry.event.Id,
	})

	return base64.RawURLEncoding.EncodeToString(payload)
}

func decodeTimelineCursor(raw string) (timelineCursor, error) {
	if raw == "" {
		return timelineCursor{}, errInvalidTimelineCursor
	}

	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return timelineCursor{}, errInvalidTimelineCursor
	}

	var cursor timelineCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return timelineCursor{}, errInvalidTimelineCursor
	}

	if cursor.ID == "" || cursor.CursorTimestamp == "" {
		return timelineCursor{}, errInvalidTimelineCursor
	}
	if cursor.Source != string(Explicit) && cursor.Source != string(Synthetic) {
		return timelineCursor{}, errInvalidTimelineCursor
	}
	if _, err := time.Parse(time.RFC3339Nano, cursor.CursorTimestamp); err != nil {
		return timelineCursor{}, errInvalidTimelineCursor
	}

	return cursor, nil
}

func resolveTimelineCursorIndex(entries []timelineEntry, cursor timelineCursor) int {
	for i := range entries {
		if entries[i].event.Id != cursor.ID {
			continue
		}
		if string(entries[i].event.Source) != cursor.Source {
			continue
		}
		if entries[i].cursorTimestamp.UTC().Format(time.RFC3339Nano) != cursor.CursorTimestamp {
			continue
		}
		return i
	}

	return -1
}
