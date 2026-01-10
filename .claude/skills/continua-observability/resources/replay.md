# Replay System

## Core Concept

Replay allows debugging agent executions **without re-incurring LLM costs**. The captured payloads (requests + responses) enable deterministic replay.

## How It Works

```
Original Execution:
Agent → LLM API → Response stored in Continua

Replay:
Agent → Continua Proxy → Stored Response (no LLM call)
```

## Deterministic Replay Requirements

### 1. Complete Payload Capture

Every LLM span must have both request and response payloads:

```sql
-- Verify complete capture
SELECT s.id, s.name,
    EXISTS(SELECT 1 FROM payloads WHERE span_id = s.id AND direction = 'request') as has_request,
    EXISTS(SELECT 1 FROM payloads WHERE span_id = s.id AND direction = 'response') as has_response
FROM spans s
WHERE s.trace_id = $1 AND s.kind = 'LLM';
```

### 2. Request Matching

During replay, match incoming requests to stored responses:

```go
// Matching strategy
func matchRequest(incoming Request, stored []Payload) *Payload {
    // 1. Exact match on model + messages
    // 2. Fuzzy match with threshold
    // 3. Sequential fallback (use order)
}
```

### 3. Timing Preservation

Optionally replay with original timing:

```go
type ReplayOptions struct {
    PreserveTiming bool  // Wait for original latency
    SpeedMultiplier float64  // 2.0 = 2x speed
}

func replaySpan(span Span, opts ReplayOptions) {
    if opts.PreserveTiming {
        time.Sleep(time.Duration(span.LatencyMs) * time.Millisecond / opts.SpeedMultiplier)
    }
    // Return stored response
}
```

## Step-Through Debugging

UI allows stepping through spans:

```typescript
interface DebugSession {
    traceId: string;
    currentSpanIndex: number;
    spans: Span[];
    state: 'paused' | 'playing' | 'stepping';
}

// Step to next span
function stepForward(session: DebugSession): DebugSession {
    return {
        ...session,
        currentSpanIndex: Math.min(
            session.currentSpanIndex + 1,
            session.spans.length - 1
        )
    };
}
```

## Time-Travel

Jump to any point in execution:

```go
// Get state at specific span
func GetStateAtSpan(traceID uuid.UUID, spanID uuid.UUID) (*TraceState, error) {
    // Get all spans up to and including target
    spans, err := queries.GetSpansUpTo(ctx, traceID, spanID)
    if err != nil {
        return nil, err
    }

    // Reconstruct state
    state := &TraceState{
        CompletedSpans: filterCompleted(spans),
        CurrentSpan:    findByID(spans, spanID),
        PendingSpans:   filterPending(spans),
    }

    return state, nil
}
```

## Payload Storage

### Truncation for Large Payloads

```go
const MaxPayloadSize = 1 * 1024 * 1024  // 1MB

func storePayload(body []byte) *Payload {
    if len(body) > MaxPayloadSize {
        return &Payload{
            Body:         body[:MaxPayloadSize],
            Truncated:    true,
            OriginalSize: len(body),
        }
    }
    return &Payload{Body: body}
}
```

### PII Redaction

Before storing, redact sensitive data:

```go
// pkg/redaction/redact.go

var patterns = []regexp.Regexp{
    regexp.MustCompile(`sk-[a-zA-Z0-9]{48}`),  // OpenAI keys
    regexp.MustCompile(`\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b`),  // Emails
}

func Redact(input string) string {
    result := input
    for _, pattern := range patterns {
        result = pattern.ReplaceAllString(result, "[REDACTED]")
    }
    return result
}
```

## Replay API

```yaml
# Proposed endpoint
POST /api/traces/{id}/replay
Content-Type: application/json

{
    "preserveTiming": false,
    "speedMultiplier": 1.0,
    "startFromSpanId": "optional-span-id"
}
```

Response: WebSocket URL for replay events

## Engine Module

The `engine/` module (separate Go module) handles:
- Replay orchestration
- Request matching
- Timing simulation
- State reconstruction

```
engine/
├── cmd/continua-engine/main.go
├── replay/
│   ├── matcher.go      # Request → Response matching
│   ├── orchestrator.go # Replay execution
│   └── timing.go       # Timing simulation
└── db/migrations/      # Engine-specific schema
```
