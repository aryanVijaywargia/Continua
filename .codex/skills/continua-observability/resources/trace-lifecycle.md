# Trace Lifecycle

## Ingestion Flow

```
SDK/Proxy → HTTP POST → Validation → Database → WebSocket Broadcast
```

### 1. Trace Creation

```go
// Trace starts when agent begins execution
trace, err := queries.CreateTrace(ctx, platform.CreateTraceParams{
    ID:        uuid.New(),
    SessionID: sessionID,  // Optional
    Name:      "chat_completion",
    Status:    "RUNNING",
    Metadata:  metadata,
})
```

### 2. Span Creation

```go
// Each operation creates a span
span, err := queries.CreateSpan(ctx, platform.CreateSpanParams{
    ID:           uuid.New(),
    TraceID:      traceID,
    ParentSpanID: parentSpanID,  // NULL for root
    Name:         "gpt-4-call",
    Kind:         "LLM",
    Status:       "STARTED",
})

// Broadcast to WebSocket subscribers
broadcast(SpanCreatedEvent{
    Type:         "span.created",
    TraceID:      traceID,
    SpanID:       span.ID,
    ParentSpanID: parentSpanID,
    Name:         span.Name,
    Kind:         span.Kind,
    Status:       span.Status,
    StartedAt:    span.StartedAt,
})
```

### 3. Payload Capture

```go
// Store request payload
queries.CreatePayload(ctx, platform.CreatePayloadParams{
    SpanID:      spanID,
    Direction:   "request",
    ContentType: "application/json",
    Body:        requestBody,
})

// Store response payload (after LLM responds)
queries.CreatePayload(ctx, platform.CreatePayloadParams{
    SpanID:      spanID,
    Direction:   "response",
    ContentType: "application/json",
    Body:        responseBody,
})
```

### 4. Span Completion

```go
// Update span with final metrics
queries.UpdateSpan(ctx, platform.UpdateSpanParams{
    ID:        spanID,
    Status:    "COMPLETED",
    EndedAt:   time.Now(),
    TokensIn:  promptTokens,
    TokensOut: completionTokens,
    CostUSD:   calculatedCost,
    LatencyMs: duration.Milliseconds(),
})

// Broadcast update
broadcast(SpanUpdatedEvent{
    Type:      "span.updated",
    SpanID:    spanID,
    Status:    "COMPLETED",
    EndedAt:   endedAt,
    TokensIn:  tokensIn,
    TokensOut: tokensOut,
    CostUSD:   costUSD,
    LatencyMs: latencyMs,
})
```

### 5. Trace Completion

```go
// When all spans complete
queries.UpdateTraceStatus(ctx, platform.UpdateTraceStatusParams{
    ID:      traceID,
    Status:  "COMPLETED",  // or "FAILED"
    EndedAt: time.Now(),
})

// Aggregate metrics
stats, _ := queries.GetTraceStats(ctx, traceID)
queries.UpdateTraceTokens(ctx, platform.UpdateTraceTokensParams{
    ID:             traceID,
    TotalTokensIn:  stats.TotalTokensIn,
    TotalTokensOut: stats.TotalTokensOut,
    TotalCostUSD:   stats.TotalCost,
})

// Final broadcast
broadcast(TraceCompletedEvent{
    Type:            "trace.completed",
    TraceID:         traceID,
    Status:          "COMPLETED",
    TotalCostUSD:    totalCost,
    TotalDurationMs: duration,
    SpanCount:       spanCount,
    ErrorCount:      errorCount,
})
```

## Error Handling

### Span Failure

```go
queries.UpdateSpan(ctx, platform.UpdateSpanParams{
    ID:           spanID,
    Status:       "FAILED",
    EndedAt:      time.Now(),
    ErrorMessage: err.Error(),
})
```

### Trace Failure

If any span fails with `kind=LLM` or `kind=AGENT`, the trace should be marked `FAILED`:

```go
// Check for critical failures
if hasFailedCriticalSpan(traceID) {
    queries.UpdateTraceStatus(ctx, traceID, "FAILED", time.Now())
}
```

## Status State Machine

```
Trace:
  RUNNING ─┬─> COMPLETED (all spans done successfully)
           └─> FAILED (critical span failed)

Span:
  SCHEDULED ─> STARTED ─┬─> COMPLETED
                        └─> FAILED
```

## Idempotency

Use span ID for idempotency on ingestion:

```go
// Client provides span ID
span, err := queries.UpsertSpan(ctx, clientProvidedSpanID, ...)
if err != nil && isConflict(err) {
    // Already exists, return existing
    return queries.GetSpan(ctx, clientProvidedSpanID)
}
```
