package api

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/continua-ai/continua/db/gen/go/platform"
)

func TestExplicitTimelineEventToAPI_PreservesSemanticEventTypes(t *testing.T) {
	now := time.Now().UTC()

	testCases := []struct {
		name      string
		eventType string
		expected  TimelineEventType
	}{
		{
			name:      "state_change",
			eventType: "state_change",
			expected:  TimelineEventTypeStateChange,
		},
		{
			name:      "decision",
			eventType: "decision",
			expected:  TimelineEventTypeDecision,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			event := platform.SpanEvent{
				ID:               uuid.New(),
				TraceID:          uuid.New(),
				SpanID:           "span-1",
				EventType:        tc.eventType,
				ServerIngestedAt: now,
			}

			apiEvent := explicitTimelineEventToAPI(&event, nil)

			assert.Equal(t, tc.expected, apiEvent.EventType)
		})
	}
}
