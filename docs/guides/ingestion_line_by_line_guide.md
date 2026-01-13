# Ingestion Line-By-Line Guide

This guide walks the ingestion pipeline in the exact order the code executes, with line-level references for every step. It focuses on the synchronous ingest path implemented in Go.

## Scope And Entry Points

The ingest flow starts in the HTTP handler and moves through the ingest service, store layer, and SQL queries. The core entry point is `Server.Ingest` in `internal/api/server.go:41`, which calls `Service.Ingest` in `internal/ingest/service.go:31`.

## Request Types Used By The Service

The service-layer DTOs are defined in `internal/ingest/dto.go`, and the handler converts OpenAPI-generated types into these DTOs.

### IngestRequest

| Field | Meaning | Reference |
| --- | --- | --- |
| `batch_key` | Idempotency key for the batch. Required by the service. | `internal/ingest/dto.go:9` |
| `traces` | Trace inputs (optional list). | `internal/ingest/dto.go:10` |
| `spans` | Span inputs (optional list). | `internal/ingest/dto.go:11` |
| `events` | Event inputs (optional list). | `internal/ingest/dto.go:12` |

### TraceInput

| Field | Meaning | Reference |
| --- | --- | --- |
| `trace_id` | External trace identifier. | `internal/ingest/dto.go:17` |
| `session_id` | Optional external session UUID string. | `internal/ingest/dto.go:18` |
| `name` | Optional trace name. | `internal/ingest/dto.go:19` |
| `user_id` | Optional user ID. | `internal/ingest/dto.go:20` |
| `tags` | Optional tag list. | `internal/ingest/dto.go:21` |
| `environment` | Optional environment string. | `internal/ingest/dto.go:22` |
| `release` | Optional release string. | `internal/ingest/dto.go:23` |
| `metadata` | Optional metadata map (JSON object). | `internal/ingest/dto.go:24` |
| `input` | Optional input payload (any JSON value). | `internal/ingest/dto.go:25` |
| `output` | Optional output payload (any JSON value). | `internal/ingest/dto.go:26` |
| `status` | Optional status string. | `internal/ingest/dto.go:27` |
| `start_time` | Optional trace start time. | `internal/ingest/dto.go:28` |
| `end_time` | Optional trace end time. | `internal/ingest/dto.go:29` |

### SpanInput

| Field | Meaning | Reference |
| --- | --- | --- |
| `trace_id` | External trace identifier to attach span to. | `internal/ingest/dto.go:34` |
| `span_id` | External span identifier. | `internal/ingest/dto.go:35` |
| `parent_span_id` | Optional parent span external ID. | `internal/ingest/dto.go:36` |
| `name` | Span name. | `internal/ingest/dto.go:37` |
| `type` | Optional span type string. | `internal/ingest/dto.go:38` |
| `status` | Optional span status string. | `internal/ingest/dto.go:39` |
| `status_message` | Optional status message. | `internal/ingest/dto.go:40` |
| `level` | Optional span level. | `internal/ingest/dto.go:41` |
| `start_time` | Span start time (required). | `internal/ingest/dto.go:42` |
| `end_time` | Optional span end time. | `internal/ingest/dto.go:43` |
| `input` | Optional input payload (any JSON value). | `internal/ingest/dto.go:44` |
| `output` | Optional output payload (any JSON value). | `internal/ingest/dto.go:45` |
| `model` | Optional model name. | `internal/ingest/dto.go:46` |
| `provider` | Optional provider name. | `internal/ingest/dto.go:47` |
| `prompt_tokens` | Optional token count. | `internal/ingest/dto.go:48` |
| `completion_tokens` | Optional token count. | `internal/ingest/dto.go:49` |
| `total_tokens` | Optional token count. | `internal/ingest/dto.go:50` |
| `total_cost` | Optional cost. | `internal/ingest/dto.go:51` |
| `metadata` | Optional metadata map (JSON object). | `internal/ingest/dto.go:52` |
| `sequence` | Optional sequence number. | `internal/ingest/dto.go:53` |
| `depth` | Optional depth number. | `internal/ingest/dto.go:54` |

### EventInput

| Field | Meaning | Reference |
| --- | --- | --- |
| `trace_id` | External trace identifier. | `internal/ingest/dto.go:59` |
| `span_id` | External span identifier. | `internal/ingest/dto.go:60` |
| `event_type` | Optional event type string. | `internal/ingest/dto.go:61` |
| `level` | Optional level string. | `internal/ingest/dto.go:62` |
| `event_ts` | Optional event timestamp. | `internal/ingest/dto.go:63` |
| `sequence` | Optional sequence number. | `internal/ingest/dto.go:64` |
| `message` | Optional message string. | `internal/ingest/dto.go:65` |
| `payload` | Optional payload map (JSON object). | `internal/ingest/dto.go:66` |
| `idempotency_key` | Optional idempotency key for events. | `internal/ingest/dto.go:67` |

## Step-By-Step: HTTP Handler To Service Call

This is the exact order inside `Server.Ingest`.

1. `MaxBodySize` defines the 5MB limit enforced on request bodies. `internal/api/server.go:14`
2. The handler checks `r.ContentLength` against the limit and returns `413` if too large. `internal/api/server.go:43`
3. The request body is wrapped with `http.MaxBytesReader` to enforce the limit during decode. `internal/api/server.go:49`
4. JSON decoding happens into the OpenAPI request type, failing fast with a `400` if parsing fails. `internal/api/server.go:52`
5. The default project is loaded for v1 single-tenant flow. `internal/api/server.go:59`
6. OpenAPI request types are converted into `ingest.IngestRequest`. `internal/api/server.go:66`
7. The service call `Ingest` executes the full ingest pipeline. `internal/api/server.go:69`
8. Errors from the service are returned as `500`. `internal/api/server.go:71`
9. The service response is mapped to the API response shape and sent as `200`. `internal/api/server.go:76`

## Step-By-Step: OpenAPI Types To Service DTOs

The conversion functions map OpenAPI types to service-layer DTOs.

1. `convertToServiceRequest` initializes `ingest.IngestRequest` and copies `batch_key`. `internal/api/server.go:228`
2. Trace inputs are appended if the request contains a `traces` list. `internal/api/server.go:233`
3. Span inputs are appended if the request contains a `spans` list. `internal/api/server.go:239`
4. Event inputs are appended if the request contains an `events` list. `internal/api/server.go:245`
5. `convertTraceInput` copies `trace_id` and tags. `internal/api/server.go:255`
6. Session ID is converted to string if present. `internal/api/server.go:260`
7. Optional trace fields are copied when non-nil. `internal/api/server.go:264`
8. Status enum is converted to string. `internal/api/server.go:285`
9. Timestamps are forwarded as-is. `internal/api/server.go:289`
10. `convertSpanInput` copies required fields and timestamps. `internal/api/server.go:300`
11. Optional span fields are copied when non-nil. `internal/api/server.go:307`
12. `convertEventInput` maps optional event fields, payload, and idempotency key. `internal/api/server.go:371`
13. `derefSlice` safely expands pointer slices to empty or nil. `internal/api/server.go:398`

## Step-By-Step: Service Ingest Flow

This is the core transaction that writes to the database.

1. `Ingest` rejects missing `batch_key`. `internal/ingest/service.go:32`
2. A DB transaction is opened via the store. `internal/ingest/service.go:37`
3. The transaction is deferred for rollback on any early exit. `internal/ingest/service.go:41`
4. The batch is claimed first for idempotency using `ClaimBatch`. `internal/ingest/service.go:44`
5. Duplicate batches short-circuit with `duplicate` status. `internal/ingest/service.go:47`
6. A map tracks external `trace_id` to internal UUID for span/event linking. `internal/ingest/service.go:57`
7. The trace loop validates `trace_id` and records errors per trace. `internal/ingest/service.go:61`
8. Each valid trace is persisted via `processTrace`. `internal/ingest/service.go:68`
9. The span loop validates `trace_id` and `span_id` presence. `internal/ingest/service.go:81`
10. Missing trace UUIDs are looked up from the DB with `GetTraceUUID`. `internal/ingest/service.go:91`
11. Each span is persisted via `processSpan`. `internal/ingest/service.go:104`
12. The event loop validates `trace_id` and `span_id` presence. `internal/ingest/service.go:115`
13. Missing trace UUIDs are looked up again for events. `internal/ingest/service.go:124`
14. Each event is persisted via `processEvent`. `internal/ingest/service.go:137`
15. Counts and status are computed after processing all items. `internal/ingest/service.go:147`
16. Batch status is updated in the DB as the final in-transaction write. `internal/ingest/service.go:156`
17. The transaction commits after all writes complete. `internal/ingest/service.go:170`
18. The response status is derived from error counts. `internal/ingest/service.go:174`
19. The final response includes counts and errors. `internal/ingest/service.go:181`

## Step-By-Step: Trace Insert Logic

1. `processTrace` marshals metadata to JSON. `internal/ingest/service.go:195`
2. `input` and `output` payloads are normalized and truncated. `internal/ingest/service.go:201`
3. `start_time` and `end_time` are converted to `pgtype.Timestamptz`. `internal/ingest/service.go:205`
4. Session ID is parsed into UUID when valid. `internal/ingest/service.go:214`
5. Status defaults to `"running"` if not provided. `internal/ingest/service.go:222`
6. `CreateTrace` inserts the trace row. `internal/ingest/service.go:224`
7. The internal UUID returned by DB is used for mapping. `internal/ingest/service.go:244`

## Step-By-Step: Span Insert Logic

1. `processSpan` marshals metadata to JSON. `internal/ingest/service.go:248`
2. Input and output payloads are normalized and truncated. `internal/ingest/service.go:255`
3. `end_time` is converted to `pgtype.Timestamptz`. `internal/ingest/service.go:259`
4. `total_cost` is converted to `pgtype.Numeric`. `internal/ingest/service.go:265`
5. `type`, `status`, and `level` default to strings if missing. `internal/ingest/service.go:272`
6. `start_time` defaults to `time.Now()` when missing. `internal/ingest/service.go:277`
7. `CreateSpan` inserts the span row with truncation metadata. `internal/ingest/service.go:282`

## Step-By-Step: Event Insert Logic

1. `processEvent` normalizes and truncates payload if present. `internal/ingest/service.go:323`
2. `event_ts` is converted to `pgtype.Timestamptz`. `internal/ingest/service.go:327`
3. `event_type` defaults to `"log"` and `level` defaults to `"info"`. `internal/ingest/service.go:333`
4. `InsertSpanEvent` inserts the event with idempotency key. `internal/ingest/service.go:336`

## Step-By-Step: Payload Normalization And Truncation

The payload normalization happens in `processPayload` and the truncation logic lives in `pkg/truncation`.

1. `processPayload` returns nils when input is nil. `internal/ingest/processor.go:12`
2. Payloads are JSON-marshaled to bytes. `internal/ingest/processor.go:17`
3. Marshal errors are wrapped into a synthetic JSON object. `internal/ingest/processor.go:20`
4. `EnsureJSON` wraps invalid JSON into a JSON object. `internal/ingest/processor.go:25`
5. If `maxBytes` is not provided, the default is used. `internal/ingest/processor.go:28`
6. Truncation executes through `TruncateWithLimit`. `internal/ingest/processor.go:32`
7. The truncation result and reason are returned to the caller. `internal/ingest/processor.go:38`
8. `EnsureJSON` validates JSON and wraps using `WrapInvalidJSON`. `pkg/truncation/wrapper.go:41`
9. `WrapInvalidJSON` preserves valid JSON unchanged. `pkg/truncation/wrapper.go:24`
10. Invalid JSON is wrapped in `{"raw":"..."}` or `{"raw_hex":"..."}`. `pkg/truncation/wrapper.go:29`
11. The default truncation size is 64KB. `pkg/truncation/truncate.go:8`
12. `TruncateJSON` returns the original payload if it fits. `pkg/truncation/truncate.go:37`
13. JSON objects and arrays are recursively truncated. `pkg/truncation/truncate.go:46`
14. Raw fallback truncation emits a JSON-safe marker. `pkg/truncation/truncate.go:61`

## Store Layer Calls In The Ingest Flow

These are the store methods that the ingest service uses, and where they bind to SQL.

1. `BeginTx` creates a transaction and binds sqlc queries to it. `internal/store/store.go:44`
2. `Tx.ClaimBatch` uses the `ClaimBatch` query. `internal/store/batches.go:27`
3. `Tx.GetTraceUUID` resolves external trace IDs to internal UUIDs. `internal/store/traces.go:69`
4. `Tx.CreateTrace` inserts a trace row. `internal/store/traces.go:19`
5. `Tx.CreateSpan` inserts a span row. `internal/store/spans.go:19`
6. `Tx.InsertSpanEvent` inserts an event row with idempotency. `internal/store/span_events.go:25`
7. `Tx.UpdateBatchStatus` writes batch counts and status. `internal/store/batches.go:65`
8. `Commit` finalizes the transaction. `internal/store/tx.go:23`
9. `Rollback` is deferred and runs on early exit. `internal/store/tx.go:27`

## SQL The Ingest Flow Executes

These are the SQL statements used by the store during ingest.

1. `ClaimBatch` inserts the batch row and enforces `(project_id, batch_key)` idempotency. `db/platform/queries/batches.sql:4`
2. `UpdateBatchStatus` writes counts and marks completion time. `db/platform/queries/batches.sql:16`
3. `CreateTrace` inserts trace data. `db/platform/queries/traces.sql:24`
4. `CreateSpan` inserts span data, including truncation metadata. `db/platform/queries/spans.sql:23`
5. `InsertSpanEvent` inserts events with optional idempotency. `db/platform/queries/events.sql:2`
6. Event idempotency uses a partial unique index on `(project_id, idempotency_key)`. `db/platform/queries/events.sql:9`

## Schema Rules That Affect Ingestion

These constraints explain why the ingest service behaves the way it does.

1. Projects are the multi-tenant boundary for all ingestion writes. `db/platform/migrations/postgres/0001_initial_schema.up.sql:9`
2. External `trace_id` is unique per project. `db/platform/migrations/postgres/0001_initial_schema.up.sql:60`
3. Spans are keyed by `(trace_id, span_id)` and `trace_id` is a UUID FK. `db/platform/migrations/postgres/0001_initial_schema.up.sql:68`
4. `parent_span_id` is stored as text to allow out-of-order spans. `db/platform/migrations/postgres/0001_initial_schema.up.sql:73`
5. Events do not enforce a FK to spans for out-of-order tolerance. `db/platform/migrations/postgres/0001_initial_schema.up.sql:116`
6. Event idempotency is enforced with a partial unique index. `db/platform/migrations/postgres/0001_initial_schema.up.sql:136`
7. Batches are unique by `(project_id, batch_key)` for idempotency. `db/platform/migrations/postgres/0001_initial_schema.up.sql:156`
8. The default project is inserted for v1 single-tenant mode. `db/platform/migrations/postgres/0001_initial_schema.up.sql:215`

## Full Call Sequence (Happy Path)

This is the exact call sequence for a valid ingest request with traces, spans, and events.

1. `Server.Ingest` parses and validates request size. `internal/api/server.go:41`
2. `convertToServiceRequest` maps OpenAPI to service DTOs. `internal/api/server.go:228`
3. `Service.Ingest` opens the transaction. `internal/ingest/service.go:37`
4. `Tx.ClaimBatch` creates the batch row. `internal/store/batches.go:27`
5. `processTrace` inserts each trace. `internal/ingest/service.go:193`
6. `Tx.CreateTrace` executes the SQL insert. `internal/store/traces.go:19`
7. `processSpan` inserts each span. `internal/ingest/service.go:247`
8. `Tx.CreateSpan` executes the SQL insert. `internal/store/spans.go:19`
9. `processEvent` inserts each event. `internal/ingest/service.go:316`
10. `Tx.InsertSpanEvent` executes the SQL insert. `internal/store/span_events.go:25`
11. `Tx.UpdateBatchStatus` writes completion counts. `internal/store/batches.go:65`
12. `Commit` completes the transaction. `internal/store/tx.go:23`
13. `Server.Ingest` returns the response. `internal/api/server.go:76`
