package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		{
			name:      "effect",
			eventType: "effect",
			expected:  TimelineEventTypeEffect,
		},
		{
			name:      "wait",
			eventType: "wait",
			expected:  TimelineEventTypeWait,
		},
		{
			name:      "snapshot_marker",
			eventType: "snapshot_marker",
			expected:  TimelineEventTypeSnapshotMarker,
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

func TestExplicitTimelineEventToAPI_DowngradesUnknownTypeWithOriginalTypeMetadata(t *testing.T) {
	now := time.Now().UTC()
	payloadBytes, err := json.Marshal(map[string]any{"key": "value"})
	require.NoError(t, err)

	event := platform.SpanEvent{
		ID:               uuid.New(),
		TraceID:          uuid.New(),
		SpanID:           "span-1",
		EventType:        "workflow_step",
		Payload:          payloadBytes,
		ServerIngestedAt: now,
	}

	apiEvent := explicitTimelineEventToAPI(&event, nil)

	assert.Equal(t, TimelineEventTypeCustom, apiEvent.EventType)
	require.NotNil(t, apiEvent.Payload)
	assert.Equal(t, "value", (*apiEvent.Payload)["key"])
	assert.Equal(t, "workflow_step", (*apiEvent.Payload)[originalEventTypePayloadKey])
}

func TestCloneTimelinePayloadWithOriginalType_DoesNotMutateInput(t *testing.T) {
	original := map[string]any{"key": "value"}

	cloned := cloneTimelinePayloadWithOriginalType(original, "workflow_step")

	assert.Equal(t, map[string]any{"key": "value"}, original)
	assert.Equal(t, "value", cloned["key"])
	assert.Equal(t, "workflow_step", cloned[originalEventTypePayloadKey])
}

func TestCloneTimelinePayloadWithOriginalType_CreatesPayloadWhenAbsent(t *testing.T) {
	cloned := cloneTimelinePayloadWithOriginalType(nil, "workflow_step")

	assert.Equal(
		t,
		map[string]any{originalEventTypePayloadKey: "workflow_step"},
		cloned,
	)
}

func TestExplicitTimelineEventToAPI_DoesNotTagRecognizedCustomEvents(t *testing.T) {
	now := time.Now().UTC()
	payloadBytes, err := json.Marshal(map[string]any{"kind": "manual"})
	require.NoError(t, err)

	event := platform.SpanEvent{
		ID:               uuid.New(),
		TraceID:          uuid.New(),
		SpanID:           "span-1",
		EventType:        "custom",
		Payload:          payloadBytes,
		ServerIngestedAt: now,
	}

	apiEvent := explicitTimelineEventToAPI(&event, nil)

	assert.Equal(t, TimelineEventTypeCustom, apiEvent.EventType)
	require.NotNil(t, apiEvent.Payload)
	assert.Equal(t, "manual", (*apiEvent.Payload)["kind"])
	_, hasOriginalType := (*apiEvent.Payload)[originalEventTypePayloadKey]
	assert.False(t, hasOriginalType)
}
