# WebSocket Events

## Event Schema (Source of Truth)

All events are defined in `contracts/websocket/events.ts` using Zod schemas.

```typescript
// contracts/websocket/events.ts

// Discriminated union - all events have 'type' field
export const WebSocketEvent = z.discriminatedUnion('type', [
    SpanCreatedEvent,
    SpanUpdatedEvent,
    StreamChunkEvent,
    TraceCompletedEvent,
    ErrorEvent,
]);
```

## Server → Client Events

### SpanCreatedEvent

Sent when a new span is created.

```typescript
{
    type: 'span.created',
    traceId: '550e8400-e29b-41d4-a716-446655440000',
    spanId: '550e8400-e29b-41d4-a716-446655440001',
    parentSpanId: null,  // or parent UUID
    name: 'gpt-4-turbo',
    kind: 'LLM',
    status: 'STARTED',
    startedAt: '2024-01-15T10:30:00Z'
}
```

### SpanUpdatedEvent

Sent when span status or metrics change.

```typescript
{
    type: 'span.updated',
    spanId: '550e8400-e29b-41d4-a716-446655440001',
    status: 'COMPLETED',
    endedAt: '2024-01-15T10:30:05Z',
    tokensIn: 150,
    tokensOut: 200,
    costUsd: 0.0035,
    latencyMs: 5000
}
```

### StreamChunkEvent

Sent during LLM streaming (real-time token output).

```typescript
{
    type: 'stream.chunk',
    spanId: '550e8400-e29b-41d4-a716-446655440001',
    chunkIndex: 0,
    content: 'Hello'
}
```

### TraceCompletedEvent

Sent when entire trace finishes.

```typescript
{
    type: 'trace.completed',
    traceId: '550e8400-e29b-41d4-a716-446655440000',
    status: 'COMPLETED',
    totalCostUsd: 0.015,
    totalDurationMs: 12000,
    spanCount: 5,
    errorCount: 0
}
```

### ErrorEvent

Sent when something goes wrong.

```typescript
{
    type: 'error',
    code: 'TRACE_NOT_FOUND',
    message: 'Trace does not exist',
    traceId: '550e8400-e29b-41d4-a716-446655440000'
}
```

## Client → Server Messages

### SubscribeMessage

```typescript
{
    type: 'subscribe',
    traceIds: ['550e8400-e29b-41d4-a716-446655440000'],
    sessionIds: []  // Optional: subscribe to all traces in session
}
```

### UnsubscribeMessage

```typescript
{
    type: 'unsubscribe',
    traceIds: ['550e8400-e29b-41d4-a716-446655440000']
}
```

## Go Implementation Pattern

```go
// internal/ws/hub.go

type Hub struct {
    clients    map[*Client]bool
    subscriptions map[string]map[*Client]bool  // traceID → clients
    broadcast  chan Event
    register   chan *Client
    unregister chan *Client
}

func (h *Hub) Run() {
    for {
        select {
        case client := <-h.register:
            h.clients[client] = true

        case client := <-h.unregister:
            delete(h.clients, client)
            h.removeFromSubscriptions(client)

        case event := <-h.broadcast:
            // Send to subscribed clients
            for client := range h.subscriptions[event.TraceID] {
                client.send <- event
            }
        }
    }
}

// internal/ws/client.go

func (c *Client) HandleMessage(msg []byte) {
    var message ClientMessage
    if err := json.Unmarshal(msg, &message); err != nil {
        c.sendError("INVALID_MESSAGE", err.Error())
        return
    }

    switch message.Type {
    case "subscribe":
        for _, traceID := range message.TraceIds {
            c.hub.Subscribe(c, traceID)
        }
    case "unsubscribe":
        for _, traceID := range message.TraceIds {
            c.hub.Unsubscribe(c, traceID)
        }
    }
}
```

## Broadcasting Events

```go
// When span is created, broadcast to subscribers
func (s *Server) CreateSpan(ctx context.Context, req CreateSpanRequest) error {
    span, err := s.queries.CreateSpan(ctx, ...)
    if err != nil {
        return err
    }

    // Broadcast to WebSocket subscribers
    s.hub.Broadcast(SpanCreatedEvent{
        Type:         "span.created",
        TraceID:      span.TraceID.String(),
        SpanID:       span.ID.String(),
        ParentSpanID: ptrString(span.ParentSpanID),
        Name:         span.Name,
        Kind:         span.Kind,
        Status:       span.Status,
        StartedAt:    span.StartedAt.Format(time.RFC3339),
    })

    return nil
}
```

## Frontend Consumption (React)

```typescript
// web/src/hooks/useTraceSubscription.ts

function useTraceSubscription(traceId: string) {
    const [spans, setSpans] = useState<Span[]>([]);

    useEffect(() => {
        const ws = new WebSocket(`ws://localhost:8080/ws`);

        ws.onopen = () => {
            ws.send(JSON.stringify({ type: 'subscribe', traceIds: [traceId] }));
        };

        ws.onmessage = (event) => {
            const data = JSON.parse(event.data);

            switch (data.type) {
                case 'span.created':
                    setSpans(prev => [...prev, data]);
                    break;
                case 'span.updated':
                    setSpans(prev => prev.map(s =>
                        s.spanId === data.spanId ? { ...s, ...data } : s
                    ));
                    break;
            }
        };

        return () => ws.close();
    }, [traceId]);

    return spans;
}
```
