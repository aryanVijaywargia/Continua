# Tasks: Reliability, Search & Sessions

## Spec 0: Repo Discovery

### Goal
Verify current state before implementing changes.

### 0.1 River / Job Queue Status
- [ ] Confirm no existing job queue in codebase
- [ ] Check `go.mod` for riverqueue deps (should be absent)
- [ ] Document current rollup trigger location

### 0.2 Idempotency Implementation
- [ ] Read `db/platform/queries/spans.sql` for UpsertSpan
- [ ] Verify current COALESCE-only approach
- [ ] Document exact SQL to modify

### 0.3 Search Capabilities
- [ ] Check for existing tsvector columns (should be none)
- [ ] Check for existing GIN indexes (should be none)
- [ ] Verify no search params in OpenAPI

### 0.4 Index Audit
- [ ] List all indexes on `traces` table
- [ ] List all indexes on `spans` table
- [ ] Identify indexes missing `project_id`

### 0.5 Sessions API Completeness
- [ ] Check `GET /api/sessions` in OpenAPI
- [ ] Check `GET /api/sessions/{id}` in OpenAPI
- [ ] Verify store methods exist for sessions

### 0.6 Python SDK Structure
- [ ] Review existing client.py
- [ ] Identify missing error handling
- [ ] Document current retry behavior (likely none)

### 0.7 Exit Criteria
- [ ] Written: `docs/phase3/spec-0-discovery.md`
- [ ] Print: `=== SPEC 0 COMPLETE ===`

---

## Spec 2: Idempotency Hardening

### Goal
Fix time handling for out-of-order span updates.

### 2.1 Update UpsertSpan Query
- [ ] Edit `db/platform/queries/spans.sql`
- [ ] Replace COALESCE-only with LEAST/GREATEST pattern:
  ```sql
  start_time = COALESCE(
      LEAST(spans.start_time, EXCLUDED.start_time),
      spans.start_time,
      EXCLUDED.start_time
  ),
  end_time = COALESCE(
      GREATEST(spans.end_time, EXCLUDED.end_time),
      spans.end_time,
      EXCLUDED.end_time
  )
  ```

### 2.2 Regenerate SQLC
- [ ] Run `make generate`
- [ ] Verify no drift in `db/gen/go/platform/`

### 2.3 Write Integration Test
- [ ] Create test in `internal/store/spans_test.go`
- [ ] Test: Send end_time first, then start_time
- [ ] Verify: Both times preserved correctly
- [ ] Test: Send start_time first, then end_time
- [ ] Verify: Both times preserved correctly

### 2.4 Verification
- [ ] Run `make test-go`
- [ ] Run `make lint`
- [ ] Written: `docs/phase3/spec-2-idempotency.md`
- [ ] Print: `=== SPEC 2 COMPLETE: READY FOR REVIEW ===`
- [ ] **STOP**: Wait for approval

---

## Spec 1: Async Rollups via River

### Goal
Move trace rollups off the ingest critical path.

### 1.1 Add River Dependencies
- [ ] Run `go get github.com/riverqueue/river`
- [ ] Run `go get github.com/riverqueue/river/riverdriver/riverpgxv5`
- [ ] Verify deps in `go.mod`

### 1.2 Vendor River Migrations
- [ ] Create `db/platform/migrations/postgres/XXXX_add_river_tables.up.sql`
- [ ] Copy required River tables from documentation
- [ ] Create corresponding `.down.sql`
- [ ] Run `make migrate`

### 1.3 Create Jobs Module
- [ ] Create `internal/jobs/module.go`
  - Export `var Module = fx.Module("jobs", ...)`
- [ ] Create `internal/jobs/river.go`
  - `NewClient(pool *pgxpool.Pool) (*river.Client, error)`
  - Configure workers and periodic jobs
- [ ] Create `internal/jobs/trace_rollup.go`
  - Define `TraceRollupArgs` struct
  - Implement `TraceRollupWorker`

### 1.4 Configure Job Uniqueness
- [x] Use default ByState (fast insertion path) with re-enqueue pattern for coalescing
- [x] Allow completed jobs to re-enqueue for new spans via version tracking:
  ```go
  // Use default ByState for fast path (includes Running)
  UniqueOpts: river.UniqueOpts{
      ByArgs: true,
      // Default ByState: Available, Completed, Pending, Running, Retryable, Scheduled
  }

  // After rollup completes, check if trace.version changed and re-enqueue if needed
  // This handles the case where new data arrived while job was running
  ```

### 1.4b Enqueue Transaction Boundary
- [x] Enqueue rollup jobs inside the ingest transaction (job row inserted in the same tx as spans)
- [x] Ensure rollback leaves no job rows
- [x] Verify job only becomes visible after ingest transaction commits

### 1.4c Coalescing Without Lost Updates
- [x] Use trace.version tracking to detect changes during rollup processing
- [x] Add trigger to bump trace.version on span insert/update (migration 000007)
- [x] Re-enqueue rollup job if version changed during processing

### 1.4d Configure Job Retention
- [x] Set completed job retention period (7 days)
- [x] Configure River retention (CancelledJobRetentionPeriod, CompletedJobRetentionPeriod, DiscardedJobRetentionPeriod)

### 1.5 Modify Ingest Service
- [ ] Update `internal/ingest/service.go`
- [ ] Remove inline rollup computation
- [ ] Collect affected trace IDs during span upsert
- [ ] Enqueue `TraceRollupJob` for each trace
- [ ] Log warning if enqueue fails (don't fail ingest)

### 1.6 Wire Into Fx App
- [ ] Update `cmd/continua/main.go`
- [ ] Add `jobs.Module` to fx.New()
- [ ] Ensure worker starts with server

### 1.7 Write Integration Test
- [ ] Test: Ingest spans → verify job in river_job table
- [ ] Test: Wait for job completion → verify trace rollups

### 1.8 Verification
- [ ] Run `make generate`
- [ ] Run `make test-go`
- [ ] Run `make lint`
- [ ] Start server, ingest data, check river_job table
- [ ] Written: `docs/phase3/spec-1-async-rollups.md`
- [ ] Print: `=== SPEC 1 COMPLETE: READY FOR REVIEW ===`
- [ ] **STOP**: Wait for approval

---

## Spec 3: Search & Filtering

### Goal
Enable finding traces at scale with full-text search.

### 3.1 Update OpenAPI Spec
- [ ] Edit `contracts/openapi/openapi.yaml`
- [ ] Add query params to `GET /api/traces`:
  - `q` (string): Full-text search query
  - `status` (string): Status filter
  - `start_time_from` (datetime): Time range start
  - `start_time_to` (datetime): Time range end
  - `user_id` (string): User filter
  - `session_id` (string): Session filter
  - `has_errors` (boolean): Error filter
  - `min_duration_ms` (integer): Duration filter

### 3.2 Regenerate Code
- [ ] Run `make generate`
- [ ] Verify handler signature updated

### 3.3 Create Search Migration
- [ ] Create `db/platform/migrations/postgres/XXXX_add_search.up.sql`
  ```sql
  ALTER TABLE traces ADD COLUMN search_vector tsvector
      GENERATED ALWAYS AS (
          setweight(to_tsvector('english', COALESCE(name, '')), 'A') ||
          setweight(to_tsvector('english', COALESCE(user_id, '')), 'B')
      ) STORED;

  CREATE INDEX idx_traces_search ON traces USING GIN(search_vector);
  ```
- [ ] Create corresponding `.down.sql`
- [ ] Run `make migrate`

### 3.3b Add Span-Name Search Index
- [ ] Add spans search_vector column (generated):
  ```sql
  ALTER TABLE spans ADD COLUMN search_vector tsvector
      GENERATED ALWAYS AS (
          setweight(to_tsvector('english', COALESCE(name, '')), 'A')
      ) STORED;

  CREATE INDEX idx_spans_search ON spans USING GIN(search_vector);
  ```
- [ ] Verify via EXPLAIN ANALYZE that the span-name index is used

### 3.4 Implement Store Method
- [ ] Create `TraceFilter` struct in `internal/store/traces.go`
- [ ] Implement `ListTracesFiltered(ctx, filter) ([]Trace, error)`
- [ ] Build dynamic WHERE clause from filter
- [ ] Use parameterized queries (prevent SQL injection)
- [ ] Handle duration filter with COALESCE for running traces and NULL start_time:
  ```sql
  EXTRACT(EPOCH FROM (COALESCE(end_time, now()) - COALESCE(start_time, server_received_at))) * 1000 >= $min_duration_ms
  ```

### 3.4b Define Search Semantics
- [ ] Parse q with `plainto_tsquery('english', q)`; ignore empty q (AND semantics - all terms must match)
- [ ] Use DISTINCT traces and COUNT(DISTINCT traces.id) for total when joining spans
- [ ] Map status param to stored values (running/completed/failed/error, case-insensitive)
- [ ] Treat stored status "cancelled" as FAILED unless a new API status is added
- [ ] Use COALESCE(start_time, server_received_at) for time-range filter

### 3.5 Update API Handler
- [ ] Update `ListTraces` in `internal/api/server.go`
- [ ] Parse query params into `TraceFilter`
- [ ] Call `ListTracesFiltered` instead of `ListTraces`

### 3.6 Update Web UI
- [ ] Add search bar to `web/src/pages/TracesPage.tsx`
- [ ] Add debounced search (300ms)
- [ ] Add filter controls (status, date range, has_errors)
- [ ] Add clear filters button
- [ ] Update API call to include filter params

### 3.7 Write Tests
- [ ] Store test: Search returns matching traces
- [ ] Store test: Filters work correctly
- [ ] API test: Query params parsed correctly

### 3.8 Performance Verification
- [ ] Run EXPLAIN ANALYZE on search query
- [ ] Verify GIN index used
- [ ] Target: <200ms for search on 100k traces

### 3.9 Verification
- [ ] Run `make generate`
- [ ] Run `make test`
- [ ] Run `make lint`
- [ ] Written: `docs/phase3/spec-3-search.md`
- [ ] Print: `=== SPEC 3 COMPLETE: READY FOR REVIEW ===`
- [ ] **STOP**: Wait for approval

---

## Spec 4: Query Performance

### Goal
Add project-scoped indexes for multi-tenant performance.

### 4.1 Audit Current Indexes
- [ ] Document all indexes on traces, spans, payloads tables
- [ ] Identify which are missing `project_id`

### 4.2 Create Index Migration
- [ ] Create `db/platform/migrations/postgres/XXXX_fix_index_scoping.up.sql`
- [ ] Replace only these non-project-scoped indexes (where queries already include project_id):
  - `idx_traces_started_at` → `idx_traces_project_started_at`
  - `idx_traces_server_received` → `idx_traces_project_server_received`
- [ ] Preserve trace_id-leading indexes and other indexes unless queries are updated to include project_id
- [ ] Create new indexes with `project_id` first:
  ```sql
  CREATE INDEX idx_traces_project_started_at
      ON traces(project_id, start_time DESC NULLS LAST);
  CREATE INDEX idx_traces_project_server_received
      ON traces(project_id, server_received_at DESC);
  ```
- [ ] Run `ANALYZE traces;`
- [ ] Create corresponding `.down.sql`

### 4.3 Run Migration
- [ ] Run `make migrate`

### 4.4 Verify Index Usage
- [ ] Run EXPLAIN ANALYZE on common queries
- [ ] Verify new indexes used
- [ ] Document query plan improvements

### 4.5 Verification
- [ ] Run `make test-go`
- [ ] Run `make lint`
- [ ] Written: `docs/phase3/spec-4-performance.md`
- [ ] Print: `=== SPEC 4 COMPLETE: READY FOR REVIEW ===`
- [ ] **STOP**: Wait for approval

---

## Spec 5: Sessions UI

### Goal
Complete the Sessions data model in the web UI.

### 5.1 Verify Sessions API
- [ ] Check `GET /api/sessions` exists (from Spec 0)
- [ ] Check `GET /api/sessions/{id}` exists
- [ ] If missing, add to OpenAPI and implement

### 5.1b Sessions API Additions
- [ ] Add `GET /api/sessions/{id}` to OpenAPI if missing:
  ```yaml
  /api/sessions/{id}:
    get:
      operationId: getSession
      summary: Get a session by ID
      tags: [Sessions]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
            format: uuid
      responses:
        '200':
          description: Session details
          content:
            application/json:
              schema:
                $ref: '#/components/schemas/Session'
        '404':
          description: Session not found
  ```
- [ ] Extend Session schema with user_id and trace_count
- [ ] Add store query to return trace_count per session (join to traces)
- [ ] Run `make generate`

### 5.2 Create Sessions Page
- [ ] Create `web/src/pages/SessionsPage.tsx`
- [ ] Display table: session_id, user, trace_count, created_at
- [ ] Add pagination (limit/offset like TracesPage)
- [ ] Link rows to `/sessions/:id`

### 5.3 Create Session Detail Page
- [ ] Create `web/src/pages/SessionDetailPage.tsx`
- [ ] Display session metadata header
- [ ] List traces filtered by session_id
- [ ] Reuse TracesPage components where possible
- [ ] Link back to sessions list

### 5.4 Update Router
- [ ] Update `web/src/App.tsx`
- [ ] Add routes:
  ```tsx
  <Route path="/sessions" element={<SessionsPage />} />
  <Route path="/sessions/:id" element={<SessionDetailPage />} />
  ```

### 5.5 Update Navigation
- [ ] Add "Sessions" link to navigation
- [ ] Ensure bidirectional navigation works

### 5.6 Verification
- [ ] Run `cd web && pnpm dev`
- [ ] Navigate to /sessions
- [ ] Verify pagination works
- [ ] Navigate to session detail
- [ ] Verify traces listed correctly
- [ ] Run `make lint`
- [ ] Run `cd web && pnpm test` (if applicable)
- [ ] Written: `docs/phase3/spec-5-sessions.md`
- [ ] Print: `=== SPEC 5 COMPLETE: READY FOR REVIEW ===`
- [ ] **STOP**: Wait for approval

---

## Spec 6: Python SDK Polish

### Goal
Improve error handling, add retries, and add helper methods.

### 6.1 Custom Exceptions
- [ ] Add to `sdks/python/src/continua/client.py`:
  ```python
  class ContinuaError(Exception): pass
  class AuthenticationError(ContinuaError): pass
  class RateLimitError(ContinuaError): pass
  class ValidationError(ContinuaError): pass
  class NetworkError(ContinuaError): pass
  ```
- [ ] Raise appropriate exceptions from client methods

### 6.2 Manual Retry with Backoff (sync)
- [ ] Implement retry logic in sync client (no asyncio):
  ```python
  def _send_with_retry(self, batch, max_attempts=3):
      for attempt in range(max_attempts):
          try:
              return self._send_batch(batch)
          except (httpx.ConnectError, httpx.TimeoutException) as e:
              if attempt == max_attempts - 1:
                  raise NetworkError(f"Failed after {max_attempts} attempts") from e
              delay = (2 ** attempt) + random.uniform(0, 1)
              time.sleep(delay)
  ```

### 6.3 Session Context Manager
- [ ] Add to `sdks/python/src/continua/trace.py`:
  ```python
  @contextmanager
  def session(session_id: str = None):
      """Context manager for session scoping"""
      sid = session_id or str(uuid.uuid4())
      token = _current_session.set(sid)
      try:
          yield sid
      finally:
          _current_session.reset(token)
  ```

### 6.4 Span Helpers
- [ ] Add to `sdks/python/src/continua/span.py`:
  - `set_llm_response(model, messages, response, tokens_in, tokens_out)`
  - `set_tool_call(tool_name, arguments, result)`
  - `log(message, level, payload)`

### 6.5 Update Public API
- [ ] Update `sdks/python/src/continua/__init__.py`
- [ ] Export new exceptions and helpers

### 6.6 Write Tests
- [ ] Create `sdks/python/tests/test_errors.py`
  - Test: AuthenticationError raised on 401
  - Test: RateLimitError raised on 429
  - Test: NetworkError raised after retries exhausted
- [ ] Update `sdks/python/tests/test_client.py`
  - Test: Retry succeeds after transient failure
  - Test: Retry gives up after max attempts

### 6.7 Verification
- [ ] Run `cd sdks/python && pip install -e .`
- [ ] Run `python -m pytest tests/`
- [ ] Written: `docs/phase3/spec-6-python-sdk.md`
- [ ] Print: `=== SPEC 6 COMPLETE: READY FOR REVIEW ===`
- [ ] **STOP**: Wait for approval

---

## Final Verification

### End-to-End Test
- [ ] Start server: `make dev-server`
- [ ] Run E2E: `make e2e` (or manual verification)
- [ ] Verify:
  - Ingest spans via Python SDK
  - Check rollup job in river_job table
  - Wait for job completion
  - Search for traces in UI
  - Navigate sessions
  - Verify pagination works

### CI Check
- [ ] Run `make ci`
- [ ] All tests pass
- [ ] No lint errors
- [ ] No drift in generated code

### Documentation
- [ ] Create `docs/phase3/REPORT.md` summarizing all changes
- [ ] Update CHANGELOG.md

---

## Dependencies

```
Spec 0 (Discovery)
    |
    v
Spec 2 (Idempotency)
    |
    v
Spec 1 (Async Rollups)
    |
    v
Spec 3 (Search)
    |
    v
Spec 4 (Performance)
    |
    v
Spec 5 (Sessions UI)

Spec 6 (Python SDK) - Can run after Spec 2
```

**Execution**: Sequential (as per user request)

---

## Constraints

- No new Go dependencies except River
- No new Python dependencies
- PostgreSQL 14+ minimum (SQLite migrations NOT updated for Phase 3)
- Project-scoped indexes: add only for traces table queries that filter by project_id; preserve all other indexes
- Vendor River migrations into `db/platform/migrations/`
- STOP after each spec for review
