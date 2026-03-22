# Change: Add Effect/Wait Semantic Event Contract Foundation

## Why

Continua's event taxonomy already includes semantic types (`state_change`, `decision`) with SDK helpers, documentation, and frontend rendering. This change **extends the existing semantic taxonomy** with causal relationship types—`effect` and `wait`—that are central to agent debugging. Adding them as accepted ingest and timeline event types, with deterministic ID derivation, lays the wire-level foundation for future causal linking. A forward-compatible unknown-type fallback ensures the server-side ingest path can accept future event types without requiring contract changes for each new type.

This is a **wire-level foundation** phase: it adds contract, backend plumbing, and generated type support. SDK convenience helpers (like `span.state_change()`), documentation conventions (like `docs/event-conventions.md`), and frontend summarization/rendering for `effect`/`wait` are deferred to a follow-up phase once there are consumers of the causal data.

## What Changes

### Track A: Accepted and preserved `effect` / `wait`
- Add `effect` and `wait` to `IngestEventType` enum in OpenAPI
- Add `effect` and `wait` to `TimelineEventType` enum in OpenAPI
- Run `make generate` to propagate to Go, TypeScript, and Python artifacts
- Update `web/src/api/client.ts` manual type union
- Extend ingest validation to accept `effect` and `wait`
- Extend timeline mapper to return stored `effect`/`wait` as recognized types (not downgraded to `custom`)
- Implement deterministic `effect_id`/`wait_id` derivation when caller omits them

### Track B: Forward-compatible unknown explicit-event fallback (server-only permissiveness)
- Relax **server-side** ingest validation to accept unknown non-empty event type strings (generated SDK enums remain strict; this is intentional divergence for raw HTTP clients)
- Explicitly reject synthetic-only types (`span_started`, `span_completed`, `span_failed`) at ingest
- Preserve raw event type in `span_events.event_type` column (already TEXT, no migration needed)
- On timeline read, downgrade unknown stored types to `custom` with `payload.__continua_original_event_type` metadata (note: the `default → custom` fallback already exists in `mapExplicitTimelineEventType`; the new behavior is injecting the `__continua_original_event_type` metadata key)

## Impact
- Affected specs: `event-taxonomy` (new capability spec)
- Affected code:
  - `contracts/openapi/openapi.yaml` — enum additions
  - `internal/ingest/processor.go` — validation changes
  - `internal/api/mapper.go` — timeline type mapping and downgrade logic
  - `internal/api/timeline.go` — no structural change, but coupled via synthetic type blocklist
  - `web/src/api/client.ts` — manual type union update
- No database migration required (`event_type` is already `TEXT NOT NULL`)
- No breaking changes to existing clients
