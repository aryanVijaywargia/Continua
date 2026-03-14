## 1. Contract Changes

- [x] 1.1 Add `TraceDetail` schema to `contracts/openapi/openapi.yaml` using `allOf` on `Trace` with additional properties: `trace_id` (string), `user_id` (string), `tags` (array of string), `environment` (string), `release` (string), `input` (any JSON), `output` (any JSON)
- [x] 1.2 Update `GET /api/traces/{id}` response to reference `TraceDetail` instead of `Trace`
- [x] 1.3 Extend `Span` schema with optional `model` (string), `provider` (string), `input_truncated` (boolean — always present when known), `input_original_size_bytes` (integer), `input_truncation_reason` (string), `output_truncated` (boolean — always present when known), `output_original_size_bytes` (integer), `output_truncation_reason` (string)
- [ ] 1.4 Run `make generate` and verify generated Go types compile; confirm `TraceDetail` embeds `Trace`

Note: `make generate` and `go build ./...` succeeded, but the current `oapi-codegen` output flattened `TraceDetail` instead of embedding `Trace`. Flat JSON is still preserved and covered by handler tests.

**Validation:** `make generate` succeeds, `go build ./...` passes

## 2. Backend Mapper Updates

- [x] 2.1 Add `traceDetailToAPI()` in `internal/api/mapper.go`: compose `traceToAPI()` output into generated `TraceDetail`, then set `trace_id` (from `t.TraceID`), `user_id`, `tags` (only when `len > 0`), `environment`, `release`, `input`, `output`
- [x] 2.2 Map trace `input` and `output` via `json.Unmarshal` into `interface{}`, guarded by `len(bytes) > 0`
- [x] 2.3 Extend `spanToAPI()` to map `model`, `provider`, and all six truncation fields from `platform.Span`. Map `input_truncated` and `output_truncated` unconditionally (always present, including `false`). Map size and reason only when non-nil.
- [x] 2.4 Update `GetTrace` handler in `internal/api/traces_handlers.go` to call `traceDetailToAPI()` and return the `TraceDetail` response

**Validation:** `go build ./...` passes, `go vet ./...` clean

## 3. Backend Tests

- [x] 3.1 Add mapper test for `traceDetailToAPI()`: verify `trace_id`, `user_id`, `environment`, `release` mapping
- [x] 3.2 Add mapper test: tags omitted when DB returns empty slice; tags present when DB returns non-empty slice
- [x] 3.3 Add mapper test: trace `input`/`output` with arbitrary JSON including falsy values (`false`, `0`, `""`, `null`, `[]`, `{}`)
- [x] 3.4 Add mapper test for span: `model`, `provider`, and all truncation fields mapped correctly; assert `input_truncated: false` is present (not absent) when payload is not truncated
- [x] 3.5 Add handler test for `GET /api/traces/{id}`: insert trace with detail fields, assert response includes them
- [x] 3.6 Add handler test: assert `GET /api/traces/{id}` JSON is flat (no nested `trace` object from allOf)
- [x] 3.7 Add handler test for `GET /api/traces/{id}/spans`: insert LLM span with model/provider/truncation, assert fields returned
- [x] 3.8 Add handler test for `GET /api/traces`: assert detail-only fields (`trace_id`, `user_id`, `tags`, `environment`, `release`, `input`, `output`) are absent from list response
- [x] 3.9 Add project-scoping 404 tests for `GetTrace` and `ListSpansByTrace` (these do not exist yet in `traces_handlers_test.go`)

**Validation:** `go test ./internal/api/...` passes

## 4. Web Client Types

- [x] 4.1 Add `JsonValue` type to `web/src/api/client.ts`: `string | number | boolean | null | JsonValue[] | { [key: string]: JsonValue }`
- [x] 4.2 Add `TraceDetail` interface extending `Trace` with `trace_id`, `user_id`, `tags`, `environment`, `release`, `input` (`JsonValue`), `output` (`JsonValue`)
- [x] 4.3 Update `Span` interface to include optional `model`, `provider`, `input_truncated`, `input_original_size_bytes`, `input_truncation_reason`, `output_truncated`, `output_original_size_bytes`, `output_truncation_reason`
- [x] 4.4 Update `fetchTrace` return type from `Trace` to `TraceDetail`; leave `fetchTraces` and `fetchTracesBySession` on `Trace`
- [x] 4.5 Update `Span.input` and `Span.output` typing to `JsonValue` (from current implicit any)

**Validation:** `make type-check` passes

## 5. Trace Detail Page — Trace Context Section

- [x] 5.1 Add full-width Trace Context section to `TraceDetailPage.tsx` above the spans/detail split
- [x] 5.2 Render rows: internal trace UUID (labeled "ID"), external `trace_id` (labeled "External Trace ID"), session UUID (link to `/sessions/{id}` when present), user ID, environment, release. Show `-` for any missing value (do not hide the row).
- [x] 5.3 Render tags as chips when present, `-` when absent
- [x] 5.4 Render trace `input` and `output` in separate `JsonViewer` panels using presence checks (`!== undefined`), not truthiness

**Validation:** `make type-check` passes; visual review in browser

## 6. Span Detail — LLM Context Block

- [x] 6.1 Add LLM Context block to `SpanDetail.tsx` above the Input section
- [x] 6.2 Render only when `span.kind === 'LLM'` and at least one of `model` or `provider` is present
- [x] 6.3 Display model and provider as labeled rows; show `-` for whichever is missing

**Validation:** `make type-check` passes; visual review in browser

## 7. Final Validation

- [x] 7.1 `make generate` — no drift
- [x] 7.2 `go test ./internal/api/...` — mapper and handler tests pass
- [x] 7.3 `make type-check` — no TypeScript errors
- [ ] 7.4 `make lint` — no new warnings
- [x] 7.5 `make test` — full test suite (if local Postgres available)

Note: `make lint` is blocked in this environment because `golangci-lint` is not installed (`make: golangci-lint: No such file or directory`).
