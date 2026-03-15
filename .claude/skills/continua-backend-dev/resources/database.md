# Database patterns

## Runtime reality
- Postgres is the real platform DB.
- SQLite under `db/platform/migrations/sqlite/` is only a bootstrap scaffold.
- SQLC inputs live in `db/platform/queries/`.
- Generated Go lives in `db/gen/go/platform/`.

## Important tables today
- `projects`: API-key-scoped tenancy
- `ingest_batches`: durable idempotency and async status
- `ingest_batch_payloads`: compressed request payloads for true async ingest
- `sessions`: internal UUID plus `external_id`
- `traces`: internal UUID plus external `trace_id`
- `spans`: internal UUID plus external `span_id`, model/provider/truncation fields
- `span_events`: explicit logs/errors/exceptions/messages/metrics/custom events

There is no separate `payloads` table in the active schema. Trace/span payloads live on `traces`, `spans`, and `ingest_batch_payloads`.

## SQLC workflow
1. Edit or add SQL in `db/platform/queries/*.sql`
2. Run `make generate`
3. Wrap generated methods in `internal/store`

Use sqlc for fixed queries first. The notable exception is `internal/store/search.go`, which builds dynamic SQL for trace filtering and ranking.

## Current query domains
- `batches.sql`: batch claim/get/status/payload cleanup
- `traces.sql`: trace lookup, upsert, counts, rollup update
- `spans.sql`: span upsert, list-by-trace, token/status updates
- `events.sql`: explicit event insert and timeline reads
- `sessions.sql`: session counts and `GetOrCreateSessionByExternalID`
- `rollups.sql`: trace aggregate computation

## Migration rules
- Never edit existing migrations.
- Add new Postgres migrations with `make migrate-create name=<description>`.
- Keep both up and down migrations.
- If a migration changes contract-visible or sqlc-visible behavior, run `make generate`.

## Current modeling details that matter
- `session_id` in ingest is an external session key; the server resolves or creates the session row.
- `trace_id` and `span_id` in ingest are external strings, not DB UUIDs.
- `parent_span_id` is also an external span ID string.
- Rollups aggregate from spans into traces asynchronously via River.
- Explicit events tolerate out-of-order span arrival because `span_events.span_id` is not a hard FK to the spans table.
