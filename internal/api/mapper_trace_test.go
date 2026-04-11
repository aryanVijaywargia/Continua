package api

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/continua-ai/continua/db/gen/go/platform"
	enginedb "github.com/continua-ai/continua/engine/db/gen/go"
	publicprojection "github.com/continua-ai/continua/engine/pkg/projection"
	"github.com/continua-ai/continua/internal/store"
	"github.com/continua-ai/continua/internal/testutil"
)

func TestTraceDetailToAPI_MapsSummaryAndDetailFields(t *testing.T) {
	start := time.Date(2026, 3, 12, 9, 30, 0, 0, time.UTC)
	end := start.Add(45 * time.Second)
	sessionID := uuid.New()
	name := "Debugger Trace"
	sessionExternalID := "conv-123"
	userID := "user@example.com"
	environment := "production"
	release := "v1.2.3"
	errorCount := int32(2)

	trace := platform.Trace{
		ID:               uuid.New(),
		SessionID:        pgtype.UUID{Bytes: sessionID, Valid: true},
		TraceID:          "trace-ext-123",
		Name:             &name,
		UserID:           &userID,
		Environment:      &environment,
		Release:          &release,
		Metadata:         []byte(`{"source":"sdk"}`),
		Status:           "completed",
		StartTime:        testutil.PgtypeTimestamptz(start),
		EndTime:          testutil.PgtypeTimestamptz(end),
		ErrorCount:       &errorCount,
		TotalTokensIn:    12,
		TotalTokensOut:   34,
		ServerReceivedAt: start.Add(-time.Second),
	}

	detail := traceDetailToAPI(&store.TraceRead{
		Trace:             trace,
		SessionExternalID: &sessionExternalID,
	})

	assert.Equal(t, trace.ID, detail.Id)
	assert.Equal(t, name, detail.Name)
	assert.Equal(t, TraceDetailStatusCOMPLETED, detail.Status)
	assert.Equal(t, start, detail.StartedAt)
	require.NotNil(t, detail.EndedAt)
	assert.Equal(t, end, *detail.EndedAt)
	require.NotNil(t, detail.SessionId)
	assert.Equal(t, sessionID, *detail.SessionId)
	require.NotNil(t, detail.SessionExternalId)
	assert.Equal(t, sessionExternalID, *detail.SessionExternalId)
	require.NotNil(t, detail.TotalTokensIn)
	assert.Equal(t, 12, *detail.TotalTokensIn)
	require.NotNil(t, detail.TotalTokensOut)
	assert.Equal(t, 34, *detail.TotalTokensOut)
	require.NotNil(t, detail.ErrorCount)
	assert.Equal(t, 2, *detail.ErrorCount)
	require.NotNil(t, detail.Metadata)
	assert.Equal(t, "sdk", (*detail.Metadata)["source"])
	require.NotNil(t, detail.TraceId)
	assert.Equal(t, "trace-ext-123", *detail.TraceId)
	require.NotNil(t, detail.UserId)
	assert.Equal(t, userID, *detail.UserId)
	require.NotNil(t, detail.Environment)
	assert.Equal(t, environment, *detail.Environment)
	require.NotNil(t, detail.Release)
	assert.Equal(t, release, *detail.Release)
}

func TestTraceToAPI_MapsEngineMetadata(t *testing.T) {
	runID := uuid.New()
	trace := platform.Trace{
		ID:                      uuid.New(),
		TraceID:                 "engine:" + runID.String(),
		Name:                    testutil.StrPtr("Engine Trace"),
		Status:                  "running",
		StartTime:               testutil.PgtypeTimestamptz(time.Now().UTC()),
		EngineRunID:             pgtype.UUID{Bytes: runID, Valid: true},
		EngineDefinitionName:    testutil.StrPtr("checkout"),
		EngineDefinitionVersion: testutil.StrPtr("v1"),
		EngineProjectionState:   testutil.StrPtr(publicprojection.StateUpToDate.String()),
	}

	apiTrace := traceToAPI(&store.TraceRead{Trace: trace})

	require.NotNil(t, apiTrace.Engine)
	assert.Equal(t, runID, apiTrace.Engine.RunId)
	assert.Equal(t, "checkout", apiTrace.Engine.DefinitionName)
	assert.Equal(t, "v1", apiTrace.Engine.DefinitionVersion)
	assert.Equal(t, UpToDate, apiTrace.Engine.ProjectionState)
}

func TestTraceDetailToAPIWithProjectedEngineSummary_UsesProjectedInstanceKey(t *testing.T) {
	runID := uuid.New()
	trace := platform.Trace{
		ID:                         uuid.New(),
		TraceID:                    "engine:" + runID.String(),
		Name:                       testutil.StrPtr("Engine Trace"),
		Status:                     "running",
		StartTime:                  testutil.PgtypeTimestamptz(time.Now().UTC()),
		EngineRunID:                pgtype.UUID{Bytes: runID, Valid: true},
		EngineInstanceKey:          testutil.StrPtr("instance-1"),
		EngineRunStatus:            testutil.StrPtr(string(enginedb.EngineRunLifecycleStatusWaiting)),
		EngineCustomStatus:         []byte(`{"step":"approval"}`),
		EngineWaitState:            []byte(`{"kind":"signal","signal_name":"approval"}`),
		EnginePendingActivityTasks: testutil.Int64Ptr(2),
		EnginePendingInboxItems:    testutil.Int64Ptr(1),
		EngineDefinitionName:       testutil.StrPtr("checkout"),
		EngineDefinitionVersion:    testutil.StrPtr("v1"),
		EngineProjectionState:      testutil.StrPtr(publicprojection.StateUpToDate.String()),
	}
	sessionExternalID := "session-1"

	detail := traceDetailToAPIWithEngine(&store.TraceRead{
		Trace:             trace,
		SessionExternalID: &sessionExternalID,
	}, nil)

	require.NotNil(t, detail.Engine)
	assert.Equal(t, "instance-1", detail.Engine.InstanceKey)
	assert.Equal(t, EngineRunStatusWAITING, detail.Engine.Status)
	require.NotNil(t, detail.Engine.CustomStatus)
	assert.Equal(t, "approval", (*detail.Engine.CustomStatus)["step"])
	require.NotNil(t, detail.Engine.WaitState)
	require.NotNil(t, detail.Engine.WaitState.SignalName)
	assert.Equal(t, "approval", *detail.Engine.WaitState.SignalName)
	assert.EqualValues(t, 2, detail.Engine.PendingWork.PendingActivityTasks)
	assert.EqualValues(t, 1, detail.Engine.PendingWork.PendingInboxItems)
}

func TestTraceDetailToAPIWithEngine_PrefersLiveEngineSummary(t *testing.T) {
	runID := uuid.New()
	previousRunID := uuid.New()
	nextRunID := uuid.New()
	previousTraceID := "engine:" + previousRunID.String()
	nextTraceID := "engine:" + nextRunID.String()
	trace := platform.Trace{
		ID:                      uuid.New(),
		TraceID:                 "engine:" + runID.String(),
		Name:                    testutil.StrPtr("Engine Trace"),
		Status:                  "running",
		StartTime:               testutil.PgtypeTimestamptz(time.Now().UTC()),
		EngineRunID:             pgtype.UUID{Bytes: runID, Valid: true},
		EngineInstanceKey:       testutil.StrPtr("instance-1"),
		EngineDefinitionName:    testutil.StrPtr("checkout"),
		EngineDefinitionVersion: testutil.StrPtr("v1"),
		EngineProjectionState:   testutil.StrPtr(publicprojection.StateCatchingUp.String()),
	}

	live := engineRunSummaryToAPI(&engineRunSummary{
		RunID:                runID,
		InstanceKey:          "instance-1",
		ContinuedFromRunID:   &previousRunID,
		ContinuedToRunID:     &nextRunID,
		ContinuedFromTraceID: &previousTraceID,
		ContinuedToTraceID:   &nextTraceID,
		DefinitionName:       "checkout",
		DefinitionVersion:    "v1",
		ProjectionState:      publicprojection.StateCatchingUp.String(),
		Status:               enginedb.EngineRunLifecycleStatusWaiting,
		CreatedAt:            time.Now().Add(-time.Minute).UTC(),
		UpdatedAt:            time.Now().UTC(),
		WaitState:            json.RawMessage(`{"kind":"signal","signal_name":"approval"}`),
	})

	detail := traceDetailToAPIWithEngine(&store.TraceRead{Trace: trace}, &live)

	require.NotNil(t, detail.Engine)
	assert.Equal(t, "instance-1", detail.Engine.InstanceKey)
	assert.Equal(t, EngineRunStatusWAITING, detail.Engine.Status)
	require.NotNil(t, detail.Engine.ContinuedFromRunId)
	assert.Equal(t, previousRunID, *detail.Engine.ContinuedFromRunId)
	require.NotNil(t, detail.Engine.ContinuedToRunId)
	assert.Equal(t, nextRunID, *detail.Engine.ContinuedToRunId)
	require.NotNil(t, detail.Engine.ContinuedFromTraceId)
	assert.Equal(t, previousTraceID, *detail.Engine.ContinuedFromTraceId)
	require.NotNil(t, detail.Engine.ContinuedToTraceId)
	assert.Equal(t, nextTraceID, *detail.Engine.ContinuedToTraceId)
	require.NotNil(t, detail.Engine.WaitState)
	require.NotNil(t, detail.Engine.WaitState.SignalName)
	assert.Equal(t, "approval", *detail.Engine.WaitState.SignalName)
}

func TestTraceDetailToAPI_TagsMapping(t *testing.T) {
	t.Run("omits empty tags", func(t *testing.T) {
		trace := platform.Trace{
			ID:        uuid.New(),
			TraceID:   "trace-ext-empty-tags",
			Name:      testutil.StrPtr("Trace"),
			Status:    "running",
			StartTime: testutil.PgtypeTimestamptz(time.Now().UTC()),
			Tags:      []string{},
		}

		detail := traceDetailToAPI(&store.TraceRead{Trace: trace})
		assert.Nil(t, detail.Tags)
	})

	t.Run("maps non-empty tags", func(t *testing.T) {
		trace := platform.Trace{
			ID:        uuid.New(),
			TraceID:   "trace-ext-tags",
			Name:      testutil.StrPtr("Trace"),
			Status:    "running",
			StartTime: testutil.PgtypeTimestamptz(time.Now().UTC()),
			Tags:      []string{"prod", "v2"},
		}

		detail := traceDetailToAPI(&store.TraceRead{Trace: trace})
		require.NotNil(t, detail.Tags)
		assert.Equal(t, []string{"prod", "v2"}, *detail.Tags)
	})
}

func TestTraceDetailToAPI_PreservesArbitraryJSON(t *testing.T) {
	testCases := []struct {
		name string
		json string
	}{
		{name: "false", json: `false`},
		{name: "zero", json: `0`},
		{name: "empty string", json: `""`},
		{name: "null", json: `null`},
		{name: "array", json: `[]`},
		{name: "object", json: `{}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			trace := platform.Trace{
				ID:        uuid.New(),
				TraceID:   "trace-ext-json",
				Name:      testutil.StrPtr("Trace"),
				Status:    "completed",
				StartTime: testutil.PgtypeTimestamptz(time.Now().UTC()),
				Input:     []byte(tc.json),
				Output:    []byte(tc.json),
			}

			detail := traceDetailToAPI(&store.TraceRead{Trace: trace})

			payload, err := json.Marshal(detail)
			require.NoError(t, err)

			var body map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(payload, &body))

			rawInput, ok := body["input"]
			require.True(t, ok, "input should be present for %s", tc.name)
			rawOutput, ok := body["output"]
			require.True(t, ok, "output should be present for %s", tc.name)

			assertJSONValue(t, rawInput, tc.json)
			assertJSONValue(t, rawOutput, tc.json)
		})
	}
}

func TestSpanToAPI_MapsLLMContextAndTruncationFields(t *testing.T) {
	t.Run("maps truncated payload metadata", func(t *testing.T) {
		inputReason := "size_limit"
		outputReason := "size_limit"
		model := "gpt-4o"
		provider := "openai"
		inputTruncated := true
		outputTruncated := true
		inputSize := int64(524288)
		outputSize := int64(1048576)

		span := platform.Span{
			ID:                      uuid.New(),
			TraceID:                 uuid.New(),
			SpanID:                  "span-1",
			Name:                    "LLM Span",
			Type:                    "llm",
			Status:                  "completed",
			StartTime:               time.Now().UTC(),
			Model:                   &model,
			Provider:                &provider,
			InputTruncated:          &inputTruncated,
			InputOriginalSizeBytes:  &inputSize,
			InputTruncationReason:   &inputReason,
			OutputTruncated:         &outputTruncated,
			OutputOriginalSizeBytes: &outputSize,
			OutputTruncationReason:  &outputReason,
		}

		apiSpan := spanToAPI(&span)

		require.NotNil(t, apiSpan.Model)
		assert.Equal(t, model, *apiSpan.Model)
		require.NotNil(t, apiSpan.Provider)
		assert.Equal(t, provider, *apiSpan.Provider)
		require.NotNil(t, apiSpan.InputTruncated)
		assert.True(t, *apiSpan.InputTruncated)
		require.NotNil(t, apiSpan.InputOriginalSizeBytes)
		assert.Equal(t, inputSize, *apiSpan.InputOriginalSizeBytes)
		require.NotNil(t, apiSpan.InputTruncationReason)
		assert.Equal(t, inputReason, *apiSpan.InputTruncationReason)
		require.NotNil(t, apiSpan.OutputTruncated)
		assert.True(t, *apiSpan.OutputTruncated)
		require.NotNil(t, apiSpan.OutputOriginalSizeBytes)
		assert.Equal(t, outputSize, *apiSpan.OutputOriginalSizeBytes)
		require.NotNil(t, apiSpan.OutputTruncationReason)
		assert.Equal(t, outputReason, *apiSpan.OutputTruncationReason)
	})

	t.Run("preserves false truncation booleans", func(t *testing.T) {
		inputTruncated := false
		outputTruncated := false

		span := platform.Span{
			ID:              uuid.New(),
			TraceID:         uuid.New(),
			SpanID:          "span-2",
			Name:            "LLM Span",
			Type:            "llm",
			Status:          "completed",
			StartTime:       time.Now().UTC(),
			InputTruncated:  &inputTruncated,
			OutputTruncated: &outputTruncated,
		}

		apiSpan := spanToAPI(&span)

		require.NotNil(t, apiSpan.InputTruncated)
		assert.False(t, *apiSpan.InputTruncated)
		require.NotNil(t, apiSpan.OutputTruncated)
		assert.False(t, *apiSpan.OutputTruncated)
		assert.Nil(t, apiSpan.InputOriginalSizeBytes)
		assert.Nil(t, apiSpan.InputTruncationReason)
		assert.Nil(t, apiSpan.OutputOriginalSizeBytes)
		assert.Nil(t, apiSpan.OutputTruncationReason)
	})
}

func assertJSONValue(t *testing.T, actual interface{}, expectedJSON string) {
	t.Helper()

	actualBytes, err := json.Marshal(actual)
	require.NoError(t, err)

	var actualValue interface{}
	require.NoError(t, json.Unmarshal(actualBytes, &actualValue))

	var expectedValue interface{}
	require.NoError(t, json.Unmarshal([]byte(expectedJSON), &expectedValue))

	assert.Equal(t, expectedValue, actualValue)
}
