# API Patterns

## Contract-First Workflow

1. **Edit OpenAPI spec** (source of truth):
   ```yaml
   # contracts/openapi/openapi.yaml
   paths:
     /api/spans/{id}/payload:
       get:
         operationId: getSpanPayload
         parameters:
           - name: id
             in: path
             required: true
             schema:
               type: string
               format: uuid
         responses:
           '200':
             content:
               application/json:
                 schema:
                   $ref: '#/components/schemas/Payload'
   ```

2. **Regenerate**:
   ```bash
   make generate
   ```

3. **Implement handler** (compiler tells you what's missing):
   ```go
   // internal/api/handlers.go
   func (s *Server) GetSpanPayload(w http.ResponseWriter, r *http.Request, id string) {
       // Implementation
   }
   ```

## Handler Structure

```go
type Server struct {
    queries *platform.Queries
}

func NewServer(queries *platform.Queries) *Server {
    return &Server{queries: queries}
}

func (s *Server) GetTrace(w http.ResponseWriter, r *http.Request, id string) {
    ctx := r.Context()

    // 1. Parse/validate input
    traceID, err := uuid.Parse(id)
    if err != nil {
        writeError(w, http.StatusBadRequest, "INVALID_ID", "Invalid UUID format")
        return
    }

    // 2. Call database via SQLC
    trace, err := s.queries.GetTrace(ctx, traceID)
    if err != nil {
        if errors.Is(err, pgx.ErrNoRows) {
            writeError(w, http.StatusNotFound, "NOT_FOUND", "Trace not found")
            return
        }
        slog.ErrorContext(ctx, "failed to get trace", "error", err)
        writeError(w, http.StatusInternalServerError, "INTERNAL", "Internal error")
        return
    }

    // 3. Map to API type and respond
    writeJSON(w, http.StatusOK, mapTraceToAPI(trace))
}
```

## Response Helpers

```go
func writeJSON(w http.ResponseWriter, status int, v any) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(Error{Code: code, Message: message})
}
```

## Type Mapping

**Rule #10: Domain types never leak into API responses**

```go
// internal/api/mapper.go

// mapTraceToAPI converts database row to API response
func mapTraceToAPI(t platform.Trace) Trace {
    return Trace{
        Id:             t.ID.String(),
        SessionId:      ptrString(t.SessionID),
        Name:           t.Name,
        Status:         TraceStatus(t.Status),
        StartedAt:      t.StartedAt.Time,
        EndedAt:        ptrTime(t.EndedAt),
        TotalTokensIn:  int(t.TotalTokensIn.Int32),
        TotalTokensOut: int(t.TotalTokensOut.Int32),
        TotalCostUsd:   t.TotalCostUsd.Float64,
        Metadata:       t.Metadata,
    }
}

// Helper for nullable UUIDs
func ptrString(id pgtype.UUID) *string {
    if !id.Valid {
        return nil
    }
    s := id.Bytes.String()
    return &s
}

// Helper for nullable timestamps
func ptrTime(t pgtype.Timestamptz) *time.Time {
    if !t.Valid {
        return nil
    }
    return &t.Time
}
```

## HTTP Status Code Usage

| Code | When |
|------|------|
| 200 | Success with body |
| 201 | Created new resource |
| 204 | Success, no body |
| 400 | Invalid input (bad UUID, missing field) |
| 401 | Missing/invalid auth |
| 404 | Resource not found |
| 500 | Internal error (log it!) |

## Pagination Pattern

```yaml
# OpenAPI
parameters:
  - name: limit
    in: query
    schema:
      type: integer
      default: 50
      maximum: 100
  - name: offset
    in: query
    schema:
      type: integer
      default: 0
```

```go
// Handler
limit := 50
if params.Limit != nil && *params.Limit <= 100 {
    limit = *params.Limit
}
```
