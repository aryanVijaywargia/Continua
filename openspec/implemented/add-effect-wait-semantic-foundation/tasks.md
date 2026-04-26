## Track A: Accepted and Preserved Effect / Wait Types

### 1. Contract and generation
- [x] 1.1 Add `effect` and `wait` to `IngestEventType` enum in `contracts/openapi/openapi.yaml`
- [x] 1.2 Add `effect` and `wait` to `TimelineEventType` enum in `contracts/openapi/openapi.yaml`
- [x] 1.3 Run `make generate` and verify Go, TypeScript, and Python artifacts include the new types
- [x] 1.4 Update `web/src/api/client.ts` to add `'effect' | 'wait'` to the manual `TimelineEvent.event_type` union

### 2. Write Track A tests first
- [x] 2.1 Write ingest tests: `effect` accepted, `wait` accepted
- [x] 2.2 Write ingest tests: caller-provided `effect_id` and `wait_id` preserved unchanged
- [x] 2.3 Write ingest tests: deterministic `effect_id` derived when absent; repeated identical inputs produce the same ID
- [x] 2.4 Write ingest tests: deterministic `wait_id` derived when absent
- [x] 2.5 Write ingest tests: nil payload and `{}` produce identical fallback derivation
- [x] 2.6 Write ingest tests: recursively sorted nested payload objects in fallback hashing
- [x] 2.7 Write ingest tests: semantic ID derived from pre-truncation payload and survives truncation (near-limit payload with semantic ID key guaranteed present in persisted payload)
- [x] 2.8 Write API/mapper tests: timeline returns `effect` as `effect`, `wait` as `wait`
- [x] 2.9 Write ingest test: orphan `wait` event stored successfully _(confirms existing behavior for new type)_
- [x] 2.10 Confirm all new tests fail (red phase)

### 3. Implement Track A
- [x] 3.1 Add `"effect"` and `"wait"` to `isValidIngestEventType` in `internal/ingest/processor.go`
- [x] 3.2 Add `"effect"` → `TimelineEventTypeEffect` and `"wait"` → `TimelineEventTypeWait` cases in `mapExplicitTimelineEventType` in `internal/api/mapper.go`
- [x] 3.3 Implement a pure, stateless `deriveSemanticID` helper with positional tuple, `\x1f` separator, SHA-256, and `effect_`/`wait_` prefix
- [x] 3.4 Implement recursive payload canonicalization for fallback content hashing (sort map keys at every level, preserve array order, handle nil/empty equivalence)
- [x] 3.5 Wire derivation into ingest processing: derive `effect_id`/`wait_id` when missing/empty/non-string, using pre-truncation payload as input
- [x] 3.6 Guarantee semantic ID key survives truncation: extract before truncation, re-inject after (see design.md D8)
- [x] 3.7 Ensure caller-provided non-empty string IDs are preserved unchanged

### 4. Verify Track A tests pass
- [x] 4.1 Run `go test ./internal/ingest/...` — all Track A tests pass
- [x] 4.2 Run `go test ./internal/api/...` — all Track A tests pass
- [x] 4.3 Run `pnpm --filter web test` — web type changes compile
- [x] 4.4 Run `make lint` — no lint issues

---

## Track B: Forward-Compatible Unknown Explicit-Event Fallback

_Depends on Track A being complete._

### 5. Write Track B tests first
- [x] 5.1 Write ingest tests: unknown explicit type strings (e.g. `"workflow_step"`) accepted and persisted
- [x] 5.2 Write HTTP handler-level test: POST raw JSON with `event_type: "workflow_step"` through the actual ingest endpoint with sync mode forced, verify sync success response and persisted type _(validates the public server boundary, not just internal service)_
- [x] 5.3 Write ingest tests: `span_started`, `span_completed`, `span_failed` explicitly rejected
- [x] 5.4 Write ingest tests: empty string event type rejected
- [x] 5.5 Write API/mapper tests: unknown stored type downgrades to `custom` with `__continua_original_event_type` _(note: `default → custom` fallback is existing; the metadata injection is the new behavior)_
- [x] 5.6 Write API/mapper tests: downgrade works for events with and without existing payload; original parsed map is not mutated
- [x] 5.7 Write API/mapper negative test: genuine stored `event_type = "custom"` round-trips as `custom` with NO `__continua_original_event_type` key in payload
- [x] 5.8 Write API/mapper tests: pagination and poll-cursor traversal remain duplicate-free with mixed `effect`, `wait`, and downgraded unknown events
- [x] 5.9 Confirm all new Track B tests fail (red phase)

### 6. Implement Track B
- [x] 6.1 Replace strict `isValidIngestEventType` with explicit blocklist + non-empty-string acceptance in `internal/ingest/processor.go`
- [x] 6.2 Create synthetic-type blocklist set (`span_started`, `span_completed`, `span_failed`) with coupling comment: `// keep in sync with synthetic timeline event types in contracts/openapi/openapi.yaml and internal/api/timeline.go`
- [x] 6.3 Reject empty string event types explicitly
- [x] 6.4 In the timeline mapper, detect unknown stored types and downgrade to `TimelineEventTypeCustom` with `__continua_original_event_type` metadata injection
- [x] 6.5 Clone payload if it exists; create new map if absent. Do not mutate the original parsed map
- [x] 6.6 Add TODO comment near clone: `// TODO: consider pooling or lower-allocation path if this becomes hot`

### 7. Verify Track B tests pass
- [x] 7.1 Run `go test ./internal/ingest/...` — all Track B tests pass
- [x] 7.2 Run `go test ./internal/api/...` — all Track B tests pass
- [x] 7.3 Run `pnpm --filter web test` — still passing

---

## Final Validation
- [x] 8.1 `make generate` passes
- [x] 8.2 `go test ./internal/ingest/...` passes (all tests)
- [x] 8.3 `go test ./internal/api/...` passes (all tests)
- [x] 8.4 `pnpm --filter web test` passes
- [x] 8.5 `make lint` passes
