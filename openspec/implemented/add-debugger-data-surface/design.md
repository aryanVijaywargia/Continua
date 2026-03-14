## Context

Phase 5 extends the debugger data surface — the fields returned by trace and span API endpoints — without changing ingestion, migrations, or SQL queries. The goal is to expose data already stored in the database so the web UI can render richer trace context and LLM-specific span information.

### Current State

- `GET /api/traces/{id}` returns `Trace` (summary): id, session_id, name, status, started_at, ended_at, total_tokens_in/out, total_cost_usd, error_count, metadata
- `GET /api/traces/{id}/spans` returns `Span`: id, trace_id, span_id, parent_span_id, name, kind, status, started_at, ended_at, tokens_in/out, cost_usd, latency_ms, error_message, input, output, metadata
- Database `traces` table has: trace_id, user_id, tags, environment, release, input, output — all unmapped
- Database `spans` table has: model, provider, input_truncated, input_original_size_bytes, input_truncation_reason, output_truncated, output_original_size_bytes, output_truncation_reason — all unmapped
- Web UI has no Trace Context section and no LLM Context block

### Constraints

- Contract-first: OpenAPI changes before implementation
- No new SQL queries, migrations, or store methods
- Generated code via `make generate`
- `Trace` summary schema must not grow (used by list endpoints)
- Web client types are handwritten (not generated) for this phase

## Goals / Non-Goals

**Goals**
- Expose trace identity/context fields via a `TraceDetail` schema on `GET /api/traces/{id}`
- Expose span LLM metadata (model, provider) and truncation fields on `GET /api/traces/{id}/spans`
- Add Trace Context section to the trace detail page
- Add LLM Context block to span detail panel
- Introduce `JsonValue` type for arbitrary JSON in frontend

**Non-Goals**
- Trace search or filter UI changes
- Truncation badges or indicators in the UI
- `duration_ms` or `total_spans` on traces (deferred)
- Session external_id resolution or display
- `thinking` field exposure
- Replay or export fields

## Decisions

### Decision 1: allOf Composition for TraceDetail

**What:** Define `TraceDetail` using OpenAPI `allOf` referencing `Trace` as base, plus an inline object with the additional properties.

**Why:** oapi-codegen generates an embedded `Trace` struct inside `TraceDetail`, producing flat JSON serialization. This avoids duplicating summary fields and keeps the `Trace` schema as the single source of truth for summary data.

**Alternative considered:** Separate `TraceDetail` schema duplicating all `Trace` fields. Rejected because it creates drift risk and violates DRY.

### Decision 2: Untyped Input/Output

**What:** Trace and span `input`/`output` fields have no `type` constraint in OpenAPI 3.1. Go maps to `interface{}`, TypeScript to a `JsonValue` union.

**Why:** These fields can be any valid JSON — arrays, scalars, objects, `null`, `false`, `0`, empty strings. Adding `type: object` would reject valid payloads.

**Go mapping:** `json.Unmarshal(bytes, &interface{})` preserves the original JSON structure. Presence is determined by `len(bytes) > 0`, not truthiness of the unmarshalled value.

**TypeScript mapping:** `JsonValue = string | number | boolean | null | JsonValue[] | { [key: string]: JsonValue }`. Presence checks use `!== undefined`, not `!!value`.

### Decision 3: Tags Empty-Slice Guard

**What:** Only assign `tags` to `TraceDetail` when `len(t.Tags) > 0`.

**Why:** PostgreSQL returns empty text arrays as `[]string{}` (non-nil empty slice), not `nil`. Without the guard, the API would serialize `"tags": []` for traces with no tags. The API contract says tags is optional — omission is cleaner than an empty array.

### Decision 4: Trace Detail Mapper Composition

**What:** `traceDetailToAPI()` calls `traceToAPI()` for summary fields, then constructs the generated `TraceDetail` type embedding the summary and setting detail fields.

**Why:** Reuses existing mapper logic. If `traceToAPI()` gains new summary fields, `traceDetailToAPI()` inherits them automatically.

### Decision 5: Span Truncation — API-Only, No UI

**What:** Six truncation fields are added to the `Span` schema and mapped in Go, but not rendered in the web UI.

**Truncation boolean contract:** `input_truncated` and `output_truncated` are always present in the API response (including `false`) because the ingest processor always writes explicit boolean values to the database (`processor.go:322-323`). This lets clients distinguish "payload confirmed complete" (`false`) from "field unavailable / legacy row" (`absent`). Only `*_original_size_bytes` and `*_truncation_reason` remain optional (nil when not truncated).

**Why:** Truncation badges and indicators are a separate UX concern. Making the data available now unblocks future UI work without coupling it to this phase.

## Risks / Trade-offs

| Risk | Impact | Mitigation |
|------|--------|------------|
| oapi-codegen allOf produces unexpected struct shape | Mapper won't compile or JSON nests incorrectly | Test asserting flat JSON structure |
| `interface{}` unmarshal loses type fidelity for large numbers | Extremely large integers may become float64 | Acceptable for observability payloads; not a precision-critical path |
| Tags empty-array leak | `"tags": []` in response for no-tag traces | `len(t.Tags) > 0` guard |

## Open Questions

None.
