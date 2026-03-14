# Change: Add Debugger Data Surface Foundation

## Why

The API exposes only summary trace data (name, status, tokens, cost). Seven fields already stored in the database — `trace_id`, `user_id`, `tags`, `environment`, `release`, `input`, `output` — are invisible to debugger users. Similarly, span responses omit `model`, `provider`, and all truncation metadata. Surfacing these fields is a prerequisite for trace context, LLM-specific debugging, and the future replay engine.

## What Changes

### Capabilities

| Capability | Type | Description |
|------------|------|-------------|
| **trace-detail-api** | NEW | `TraceDetail` schema via `allOf` composition on `Trace`, returned by `GET /api/traces/{id}` only. Adds `trace_id`, `user_id`, `tags`, `environment`, `release`, `input`, `output`. |
| **span-llm-context** | NEW | Extend `Span` schema with optional `model`, `provider`, and six truncation fields. Map from existing DB columns. |
| **web-debugger-ui** | NEW | Trace Context section on detail page, LLM Context block in span detail, `TraceDetail` client type, `JsonValue` type for arbitrary JSON. |

### Key Design Decisions

1. **Summary/Detail split**: `Trace` stays lightweight for list endpoints. `TraceDetail` extends it via `allOf` for `GET /api/traces/{id}` only.
2. **allOf composition**: `TraceDetail` uses OpenAPI `allOf` referencing `Trace` so oapi-codegen embeds the base struct — flat JSON, not nested.
3. **Untyped input/output**: Trace and span `input`/`output` remain schema-less in OpenAPI 3.1 (any valid JSON). Go maps to `interface{}`, TypeScript to `JsonValue`.
4. **Tags empty-slice safety**: Map `tags` only when `len(t.Tags) > 0` because Postgres returns `[]string{}` for empty arrays, not `nil`.
5. **No new SQL**: All needed columns are already selected by existing queries.
6. **Truncation data available, not displayed**: Truncation fields are in API responses and client types but not rendered in UI yet.
7. **Truncation booleans always present**: `input_truncated` and `output_truncated` are always serialized (including `false`) because the ingest processor always writes explicit values. Only size and reason are optional.

### Breaking Changes

None. All changes are additive:
- `GET /api/traces/{id}` returns a superset of the current `Trace`.
- `Span` gains optional fields that were previously omitted.

## Impact

### Affected Specs

- `trace-detail-api` (new)
- `span-llm-context` (new)
- `web-debugger-ui` (new)

### Affected Code

| Path | Change |
|------|--------|
| `contracts/openapi/openapi.yaml` | ADD: `TraceDetail` schema with `allOf`, extend `Span` with 8 optional fields, update `GET /api/traces/{id}` response ref |
| `internal/api/mapper.go` | ADD: `traceDetailToAPI()` mapper; MODIFY: `spanToAPI()` to include model/provider/truncation |
| `internal/api/traces_handlers.go` | MODIFY: `GetTrace` handler to use detail mapper and return `TraceDetail` |
| `web/src/api/client.ts` | ADD: `TraceDetail`, `JsonValue` types; MODIFY: `fetchTrace` return type |
| `web/src/pages/TraceDetailPage.tsx` | ADD: Trace Context section |
| `web/src/components/SpanDetail.tsx` | ADD: LLM Context block |

### Database Changes

None. No migrations, no new queries.

### OpenAPI Changes

- ADD: `TraceDetail` schema (`allOf` on `Trace` + detail fields)
- MODIFY: `GET /api/traces/{id}` response → `TraceDetail`
- MODIFY: `Span` schema → add `model`, `provider`, 6 truncation fields

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| oapi-codegen `allOf` produces nested struct instead of flat | Add test asserting JSON response has no nested `trace` key |
| Falsy JSON values (`false`, `0`, `""`, `null`) dropped by mapper | Use `interface{}` unmarshal + explicit presence checks in Go and TypeScript |
| Tags serialized as `[]` instead of omitted | Guard with `len(t.Tags) > 0` before assignment |

## Dependencies

None. No new Go or JS dependencies.

## Related Documents

- Design: [design.md](./design.md)
- Tasks: [tasks.md](./tasks.md)
- Spec Deltas: [specs/](./specs/)
