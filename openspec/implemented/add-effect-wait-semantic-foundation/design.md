## Context

Continua's event taxonomy has 8 ingest types and 11 timeline types (adding 3 synthetic span lifecycle markers). Two of the ingest types—`state_change` and `decision`—are already first-class semantic types with SDK helpers (`span.state_change()`, `span.decision()`), documentation (`docs/event-conventions.md`), and frontend summarization (`web/src/utils/timeline.ts`). This change extends the semantic taxonomy with causal relationship types—`effect` and `wait`—and introduces a forward-compatible fallback so the server can accept future event types without requiring contract changes for every new type.

**Scope**: This is a wire-level foundation phase. It adds contract enums, backend plumbing, and generated type support. SDK convenience helpers, documentation conventions, and frontend summarization/rendering for `effect`/`wait` are deferred to a follow-up phase.

The change spans the OpenAPI contract, Go ingest validation, Go timeline mapping, and the web client's manual type union. It does not touch the database schema, River jobs, or the engine module.

### Stakeholders
- Backend: ingest validation, timeline mapping
- Frontend: timeline event type rendering
- SDK consumers: Python SDK (auto-generated types), TypeScript SDK (stub)

## Goals / Non-Goals

### Goals
- Add `effect` and `wait` as accepted and preserved event types across the ingest → store → timeline read path (wire-level foundation, not product-level parity with `state_change`/`decision`)
- Deterministic `effect_id` / `wait_id` derivation so causal links are stable without caller coordination
- Accept unknown explicit event type strings server-side only for forward compatibility (generated SDK enums remain strict)
- Downgrade unknown types to `custom` on timeline read with preserved original type in payload metadata

### Non-Goals
- No `engine/` runtime work
- No richer `effect`/`wait` payload validation beyond reserved ID fields
- No causal graph construction or linking logic (future phase)
- No WebSocket or replay changes
- No database migration
- No SDK convenience helpers (like `span.effect()` / `span.wait()`) — deferred
- No `docs/event-conventions.md` entries — deferred until SDK helpers exist
- No frontend summarization or rendering for `effect`/`wait` — deferred

## Decisions

### D1: Platform-path only, no engine/ runtime work
The `engine/` module is scaffolded and isolated. This change stays on the live platform path: contracts, ingest, API, and web.

### D2: Track A and Track B are separate deliverables
Track A (first-class `effect`/`wait`) and Track B (forward-compatible unknown fallback) are independently testable and reviewable. Track A can land without Track B, but Track B depends on Track A's enum additions being present.

### D3: Server-side forward compatibility is intentional server-only permissiveness
The OpenAPI enum remains strict (`effect`, `wait`, plus existing types). The server-side ingest validation is deliberately relaxed beyond the enum to accept unknown strings. This creates an intentional contract/runtime divergence:
- **Generated SDK clients** (Python `types.py`, TS generated types) will only expose enum-defined types. Sending an unknown type via the SDK requires bypassing the typed API.
- **Raw HTTP clients** can send any non-empty, non-synthetic event type string, and the server will persist it.
- This divergence is acceptable because Track B targets forward compatibility for raw integrations and internal tooling, not SDK-driven workflows. The SDK enum is updated when a type graduates to first-class (as `effect`/`wait` do in Track A).

### D4: Reserved metadata key is `__continua_original_event_type`
When an unknown stored event type is downgraded to `custom` on timeline read, the original raw type is preserved in `payload.__continua_original_event_type`. The `__continua_` prefix is reserved for platform metadata. No new top-level response field is added.

### D5: Deterministic semantic ID derivation is v1, internal, pure, and stateless

#### Derivation trigger
- Only when the event type is `effect` and `payload.effect_id` is missing/empty/non-string, or `wait` and `payload.wait_id` is missing/empty/non-string.
- A non-empty caller-provided string ID is preserved unchanged.

#### Tuple structure
Positional-first to avoid JSON serialization-order ambiguity. Fields joined with `\x1f` (ASCII unit separator):

| Position | Field | Value |
|----------|-------|-------|
| 0 | namespace | `continua-semantic-id` (literal) |
| 1 | version | `v1` (literal) |
| 2 | kind | `effect` or `wait` |
| 3 | trace_id | external `trace_id` string |
| 4 | span_id | external `span_id` string |
| 5 | sequence | decimal string, or `""` if nil |
| 6 | event_ts | RFC3339Nano UTC, or `""` if nil |
| 7 | fallback hash | only present when both sequence and event_ts are absent |

#### Fallback content hash
When both `sequence` and `event_ts` are absent, a content hash is appended as field 7:
- Remove `effect_id`, `wait_id`, and all `__continua_*` keys from payload
- Treat nil and empty payload identically as `{}`
- Canonicalize with a custom recursive normalizer:
  - Maps/objects: sort keys at every nesting level
  - Arrays: preserve original order
  - Scalars: preserve value type
  - Nil object values remain nil
- Canonical fallback content: `level` (string or empty) + `message` (string or empty) + recursively normalized payload
- SHA-256 the canonical content and take the first 32 lowercase hex chars

#### Final digest
- SHA-256 the `\x1f`-joined tuple bytes
- Take the first 32 lowercase hex chars
- Prefix with `effect_` or `wait_`
- Result: `effect_<32hex>` or `wait_<32hex>`

#### Properties
- `sequence` is nullable `int32`, serialized as decimal string in the tuple
- `TimelineEvent.id` remains the persisted row UUID; derived semantic IDs live only in payload
- Stateless: identical inputs always produce identical outputs across instances

### D6: Synthetic type blocklist coupling
The blocklist of synthetic-only types (`span_started`, `span_completed`, `span_failed`) that are rejected at ingest is maintained as a single set/helper with a code comment noting its coupling to `contracts/openapi/openapi.yaml` and `internal/api/timeline.go`.

### D7: Unknown-type downgrade clone strategy
When downgrading an unknown type to `custom`:
- If payload is absent, create a new map with only `__continua_original_event_type`
- If payload exists, clone once and append the metadata key (do not mutate the parsed original)
- Accept the per-event clone allocation cost in Phase 1; leave a TODO noting pooling can be considered later

Note: the `default → TimelineEventTypeCustom` fallback already exists in `mapExplicitTimelineEventType` today. The new behavior in Track B is specifically the `__continua_original_event_type` metadata injection and the intentional server-side acceptance of unknown types at ingest.

### D8: Semantic IDs are guaranteed to survive payload truncation
Event payloads are truncated on write via `pkg/truncation.TruncateJSON` (default 64KB limit). The `truncateObject` function iterates Go maps with non-deterministic order, so simply injecting the semantic ID key before truncation does NOT guarantee survival — near-limit payloads could randomly drop it.

The derivation and persistence steps must follow this order:

1. **Derive** `effect_id` / `wait_id` from the pre-truncation payload (so the derivation input is deterministic and not affected by truncation heuristics)
2. **Extract** the semantic ID key from the payload map (hold it aside)
3. **Serialize** the payload (without the semantic ID key) to JSON bytes
4. **Truncate** the serialized payload if it exceeds the size limit
5. **Re-inject** the semantic ID key into the (possibly truncated) payload bytes

Implementation: truncate the payload to `maxBytes - reservedKeySize` (where `reservedKeySize` accounts for the semantic ID key-value pair, ~50 bytes for `"effect_id":"effect_<32hex>"`), then re-inject the reserved key into the result. This avoids overshooting the intended size budget — a naive "truncate to full limit, then re-inject" approach would exceed `maxBytes`.

This guarantees the spec's `SHALL be stored in payload` requirement is met regardless of payload size or truncation behavior.

## Risks / Trade-offs

| Risk | Mitigation |
|------|------------|
| Forward-compatible acceptance could let typos through silently | Only affects server-side; SDK enums remain strict. Monitoring/logging can flag unexpected types. |
| Payload clone on downgrade adds allocation per unknown event | Acceptable in Phase 1; leave TODO for optimization if it becomes hot path |
| Deterministic ID collisions if tuple fields are all empty | Fallback content hash ensures differentiation; truly identical events correctly share an ID |
| Truncated payload reduces effective max size by ~50 bytes for semantic events | Negligible vs 64KB limit. Reserved headroom approach keeps truncation budget honest. |

## Open Questions

None — all decisions are locked for Phase 1.
