## Context

Continua's trace rollup stores a single `total_tokens` value but the API contract exposes `total_tokens_in` and `total_tokens_out`. The mapper halves the total as a workaround. Additionally, sessions exist in the DB but cannot be created through SDK ingest — the SDK sends a string session key but the server expects a UUID.

There is also an input-model ambiguity: ingest span payloads can include `total_tokens`, but rollup logic should be source-of-truth from directional fields (`prompt_tokens`, `completion_tokens`) only. Since this project is pre-production, we can make a clean breaking change now.

These two fixes are prerequisites for the TypeScript SDK and production deployment.

## Goals / Non-Goals

- Goals:
  - Proper per-direction token tracking on traces (aligned with spans which already have `prompt_tokens` / `completion_tokens`)
  - SDK-friendly session creation via human-readable external keys
  - Clear and unambiguous ingest token contract (no legacy `total_tokens` compatibility behavior)
  - Session API responses that expose both internal UUID and user-facing external key
  - Clean schema (no legacy compatibility concerns — project is pre-production)
- Non-Goals:
  - Changing span-level token fields (already correct)
  - Adding session CRUD API endpoints (only auto-create on ingest)
  - Adding session metadata updates through ingest

## Decisions

### Token Rollup: Two columns instead of one

- Decision: Replace `total_tokens` with `total_tokens_in` (from `prompt_tokens`) and `total_tokens_out` (from `completion_tokens`) on the traces table
- Alternatives considered:
  - Keep `total_tokens` and add two new columns → rejected: creates ambiguity about which to use, old column becomes dead weight
  - Store tokens in a separate rollup table → rejected: over-engineering, current denormalized approach works fine

### Span ingest token policy: strict split fields

- Decision: Treat directional fields (`prompt_tokens`, `completion_tokens`) as the only rollup source. `total_tokens` in span ingest is deprecated and removed from the supported ingest contract for this change.
- Behavioral rule: payloads that provide only `total_tokens` are unsupported. We prefer explicit validation/rejection over silently accepting partial data.
- Rationale: avoids hidden undercount behavior and prevents confusion for SDK and API consumers.

### Session External IDs: Server-side mapping

- Decision: Add `external_id TEXT NOT NULL` column to sessions. All session_id strings from SDKs are treated as external keys — the server always resolves via `GetOrCreateSessionByExternalID`, even for UUID-looking strings. No special UUID bypass path.
- Alternatives considered:
  - Client-side UUID generation → rejected: UUIDs are unfriendly for users and make session grouping opaque
  - Separate session creation API call → rejected: adds latency and complexity; lazy creation during ingest is simpler
  - Hash-based ID derivation → rejected: loses the human-readable identifier

### Session API contract: external ID is first-class

- Decision: Session responses include `external_id` on both list and detail endpoints.
- Rationale: users instrument with external keys (e.g. `"checkout-flow-42"`), so API/UI must return that key for correlation. Returning only internal UUID makes session debugging and lookup harder.

### Migration sequencing

- Decision: Token fix = migration 000008, session fix = migration 000009. Token fix is independent and lower risk; session fix adds a column and index.
- Both migrations are breaking one-shot changes (acceptable since project is pre-production with no live data). Token migration adds columns, backfills, and drops old column in one step. Session migration adds a NOT NULL column.

## Risks / Trade-offs

- **Backfill performance**: The token backfill UPDATE scans all traces joined to spans. For large datasets this could be slow. Mitigation: The UPDATE uses a correlated subquery which PostgreSQL handles efficiently with the existing `idx_spans_trace` index.
- **Dropping `total_tokens`**: Code that references the old column will break at compile time (sqlc). Mitigation: This is the desired behavior — forces all references to be updated.
- **Breaking ingest token contract**: clients still sending only `total_tokens` will fail validation after this change. Mitigation: pre-production cleanup; update SDK/docs together in the same release.
- **Concurrent session creation**: Two ingest requests with the same `(project_id, external_id)` could race. Mitigation: The `ON CONFLICT` upsert is atomic and safe.

## Migration Plan

1. Apply migration 000008 (token columns) — adds new columns, backfills, drops old column
2. Run `make generate` to update sqlc types
3. Update Go code (store, domain, mapper, rollup worker)
4. Apply migration 000009 (session external_id) — adds NOT NULL column and unique index
5. Run `make generate` again
6. Update Go code (ingest service, store, mapper)
7. Update OpenAPI spec, regenerate
8. Update Python SDK types
9. Run full test suite

Rollback: Each migration has a down migration. Token rollback re-creates `total_tokens` as `total_tokens_in + total_tokens_out`. Session rollback drops the column and index.

## Scope Notes

- This change set assumes pre-production state with no historical production data to preserve.
- No legacy data backfill guarantees beyond migration correctness are required.

## Open Questions

- None — requirements are clear from the task description.
