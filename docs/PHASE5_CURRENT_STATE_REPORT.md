# Continua Current State Report

Status date: 2026-03-12

Purpose: provide a code-verified architecture and implementation baseline for Phase 5 planning.

Scope of this report:
- current product scope
- current runtime architecture
- major modules and responsibilities
- implemented workflows end to end
- database and API surfaces
- completed, partial, and missing work
- planning implications for Phase 5

Method used:
- audited the live repository, not just historical docs
- treated checked-in code as the primary source of truth
- used existing phase reports and OpenSpec changes only as supporting context
- explicitly call out places where docs/specs drift from the current code

---

## 1. Executive Summary

Continua is currently an AI agent observability and debugging platform centered on one strong path:

1. SDKs emit traces, spans, and events
2. the Go backend authenticates and accepts ingest
3. PostgreSQL stores traces, spans, events, sessions, and ingest batch state
4. River processes background jobs for rollups and async ingest
5. the web UI lets a user inspect traces, sessions, span trees, and merged timelines

The system is no longer just a Phase 2 or Phase 3 prototype. The codebase now includes:
- authenticated ingest with project scoping
- durable idempotent batch handling
- true async ingest infrastructure
- async rollups
- trace search and filtering support in the backend
- sessions list and detail experiences
- merged trace timeline API and UI
- Python SDK helpers for traces, spans, events, retry behavior, and async ingest polling

The project is not yet a full durable workflow runtime. The separate `engine/` module remains pre-scaffolded. Real-time WebSocket infrastructure, proxy capture, score management, and TypeScript SDK parity are not implemented.

The biggest planning takeaway for Phase 5 is this:

Phase 5 should not be planned as "build sessions from scratch." Sessions already exist as a read path and an ingest-linked grouping concept. The actual missing work is:
- formal session lifecycle and richer session semantics
- score or annotation domain design and implementation
- surfacing existing backend capabilities in the UI
- closing operational and documentation gaps around true async ingest
- cleaning up source-of-truth drift so future planning starts from a stable baseline

---

## 2. Current Product Goal and Scope Boundary

### 2.1 What Continua is today

Continua is currently optimized for:
- capturing agent execution data
- grouping that data into traces and sessions
- reconstructing span hierarchies
- attaching explicit events such as logs, errors, exceptions, and metrics
- computing trace-level aggregates
- providing a debuggable trace detail view with timeline ordering

### 2.2 What Continua is not yet

The current codebase does not yet provide:
- a production durable execution engine
- a live WebSocket-based realtime pipeline
- first-class score CRUD APIs
- explicit session creation or mutation APIs
- a mature TypeScript ingestion SDK
- a fully implemented proxy capture subsystem

### 2.3 Product boundary by implementation status

Implemented core:
- ingest
- storage
- rollups
- trace browsing
- session browsing
- timeline debugging
- Python instrumentation SDK

Adjacent but incomplete:
- true async ingest operational hardening
- search/filter UX
- richer session semantics

Mostly scaffolded:
- engine
- WebSocket runtime
- proxy/runtime capture
- alerts/export/replay/state subsystems
- TypeScript SDK

---

## 3. Source of Truth and Current Planning Risk

The repository currently has a source-of-truth split that matters for planning:

- contracts are the source of truth for REST and WebSocket schemas
- database truth lives in the Postgres migrations and SQLC queries
- runtime truth lives in the Go, web, and Python code
- historical planning docs still contain future-state or outdated assumptions

Important current planning risk:
- `openspec/specs/` is empty, so there is no archived "current state" spec set
- active change folders still show incomplete task counts even where code is already implemented
- `config.example.yaml` describes features the runtime config loader does not currently support
- older architecture docs still mention scores, explicit session CRUD, and broader subsystems that are not implemented in the shipping code

Phase 5 planning should therefore use this report plus live code as the baseline, not the older architecture plan alone.

---

## 4. Repository Structure

### 4.1 Top-level layout

```text
cmd/                  Server CLI entrypoint
contracts/            OpenAPI and WebSocket contracts
db/platform/          Postgres and SQLite schema + SQLC inputs
docs/                 Phase reports, design docs, architecture notes
engine/               Separate future runtime module
internal/             Main application implementation
pkg/                  Shared packages (only truncation is active today)
sdks/python/          Main usable SDK
sdks/typescript/      Early scaffold SDK
web/                  React SPA
```

### 4.2 Current implementation density by area

High density and active:
- `internal/api`
- `internal/ingest`
- `internal/jobs`
- `internal/store`
- `db/platform`
- `web/src`
- `sdks/python`

Low density or placeholders:
- `internal/proxy`
- `internal/ws`
- `internal/replay`
- `internal/alerts`
- `internal/export`
- `internal/state`
- `internal/telemetry`
- `pkg/idempotency`
- `pkg/redaction`
- `pkg/matcher`
- `engine/`

This split is important for Phase 5 planning because it shows which areas are real platforms to extend and which are still naming placeholders.

---

## 5. Runtime Architecture

### 5.1 Container view

```mermaid
flowchart LR
    U["Developer / Operator"]
    PY["Python SDK"]
    TS["TypeScript SDK (stub)"]
    WEB["React Web UI"]
    API["Go Platform Server"]
    DB["PostgreSQL"]
    RQ["River Queue (in Postgres)"]
    ENG["Engine Module (future)"]

    U --> PY
    U --> WEB
    U --> TS

    PY -->|"POST /v1/ingest"| API
    PY -->|"GET /v1/ingest/batches/{id}"| API
    WEB -->|"GET /api/*"| API
    TS -->|"not implemented for real ingest"| API

    API --> DB
    API --> RQ
    RQ --> API

    ENG -. "separate module, not implemented" .- API
```

### 5.2 Internal server module view

```mermaid
flowchart TD
    MAIN["cmd/continua/main.go"]
    CFG["internal/config"]
    STORE["internal/store"]
    JOBS["internal/jobs"]
    INGEST["internal/ingest"]
    API["internal/api"]
    EMBED["internal/web"]

    MAIN --> CFG
    MAIN --> STORE
    MAIN --> JOBS
    MAIN --> INGEST
    MAIN --> API
    API --> EMBED

    API --> STORE
    API --> INGEST
    INGEST --> STORE
    INGEST --> JOBS
    JOBS --> STORE
```

### 5.3 Architectural style

The active platform follows a layered service architecture:

- HTTP layer in `internal/api`
- ingest orchestration and processing in `internal/ingest`
- background jobs in `internal/jobs`
- persistence access in `internal/store`
- generated SQL and contract surfaces below those layers

This is not a heavy domain-driven architecture, but it is clean enough for Phase 5 work because:
- handlers do not directly manipulate SQL
- ingest orchestration is separated from the write path
- background work is not embedded in handlers
- DB types are mapped to API types before leaving the service

---

## 6. Component Responsibilities

### 6.1 Server entrypoint

Primary file:
- `cmd/continua/main.go`

Responsibilities:
- provide Cobra commands: `serve`, `migrate`, `version`
- initialize Fx modules
- run the HTTP server
- run embedded migrations against Postgres

Notable current behavior:
- server startup is straightforward and functional
- migrations are Postgres-oriented in the CLI path
- `serve` relies on env-only config, not the YAML example

### 6.2 API layer

Primary files:
- `internal/api/router.go`
- `internal/api/server.go`
- `internal/api/ingest_handlers.go`
- `internal/api/traces_handlers.go`
- `internal/api/sessions_handlers.go`
- `internal/api/timeline.go`
- `internal/api/mapper.go`

Responsibilities:
- route public and protected endpoints
- read authenticated `project_id` from middleware context
- validate request shape and auth context
- call store or ingest services
- map DB rows to API responses
- build and paginate timeline responses

Current API capabilities:
- ingest endpoint
- batch status endpoint
- traces list/detail
- spans by trace
- timeline events by trace
- sessions list/detail
- health endpoint

### 6.3 Auth middleware

Primary file:
- `internal/api/middleware/auth.go`

Responsibilities:
- extract API key from `X-API-Key` or `Authorization: Bearer`
- SHA-256 hash lookup against `projects.api_key_hash`
- inject `project_id` into request context

Design rationale:
- simple multi-tenant enforcement
- no user auth or RBAC yet
- project scoping is the central tenant boundary

### 6.4 Ingest service

Primary files:
- `internal/ingest/service.go`
- `internal/ingest/processor.go`
- `internal/ingest/dto.go`

Responsibilities of `Service`:
- choose sync vs async behavior
- manage batch idempotency records
- store async payloads
- enqueue batch and rollup jobs
- expose batch status mapping

Responsibilities of `Processor`:
- validate ingest payloads
- resolve trace references
- upsert traces
- upsert spans
- insert events
- handle session resolution by external session key
- perform payload truncation and JSON normalization

This split is one of the strongest patterns in the codebase. It should be preserved in Phase 5. New ingest write-path behavior should continue to land in `Processor`, not be folded back into handler or service code.

### 6.5 Jobs layer

Primary files:
- `internal/jobs/module.go`
- `internal/jobs/trace_rollup.go`
- `internal/jobs/ingest_batch.go`
- `internal/jobs/cleanup.go`

Responsibilities:
- configure River workers
- run queue consumers on app startup
- process trace rollups asynchronously
- process accepted ingest batches asynchronously
- clean up expired async ingest payloads

Queue topology in code:
- `ingest`
- `rollup`
- `maintenance`
- temporary `default` queue consumer retained for rollout compatibility

### 6.6 Store layer

Primary files:
- `internal/store/store.go`
- `internal/store/traces.go`
- `internal/store/spans.go`
- `internal/store/span_events.go`
- `internal/store/sessions.go`
- `internal/store/batches.go`
- `internal/store/rollups.go`
- `internal/store/search.go`

Responsibilities:
- own pgx pool and transactions
- wrap SQLC queries
- normalize not-found behavior
- provide the search query path not easily modeled in static SQLC

Store layer style:
- mostly thin wrappers
- minimal business logic except dynamic search query construction and numeric conversion helpers
- transaction-aware helper methods on `Tx`

### 6.7 Embedded web serving

Primary files:
- `internal/web/embed.go`
- `internal/web/handler.go`

Responsibilities:
- embed Vite build output into the Go binary
- serve SPA routes in production
- allow API routes to bypass SPA fallback

### 6.8 Frontend

Primary files:
- `web/src/App.tsx`
- `web/src/api/client.ts`
- `web/src/pages/*`
- `web/src/components/*`

Responsibilities:
- prompt for API key
- browse traces and sessions
- inspect spans
- render merged timeline
- poll running traces

### 6.9 SDKs

Python SDK:
- the primary usable external instrumentation surface
- supports trace, span, session contexts
- batches writes on a timer and size threshold
- retries transient errors
- supports `sync`, `async_v2`, and `server_default` ingest modes
- supports `wait_for_batch()`

TypeScript SDK:
- currently only exposes a trivial client shell and version constant
- not a real ingestion or instrumentation surface yet

### 6.10 Separate engine module

`engine/` is present to reserve module boundaries for a future durable execution engine, but it is not part of the current product runtime. The current `continua-engine` binary only prints a placeholder message.

---

## 7. Database Design and Schema

### 7.1 Current live platform tables

Core platform tables:
- `projects`
- `sessions`
- `traces`
- `spans`
- `span_events`
- `ingest_batches`
- `ingest_batch_payloads`

Operational queue tables:
- `river_job`
- `river_queue`
- `river_client`
- `river_client_queue`
- River support tables and enums

Legacy or compatibility table:
- `payloads`

### 7.2 Core entity relationships

```mermaid
erDiagram
    PROJECTS ||--o{ SESSIONS : owns
    PROJECTS ||--o{ TRACES : owns
    PROJECTS ||--o{ SPANS : owns
    PROJECTS ||--o{ SPAN_EVENTS : owns
    PROJECTS ||--o{ INGEST_BATCHES : owns

    SESSIONS ||--o{ TRACES : groups
    TRACES ||--o{ SPANS : contains
    TRACES ||--o{ SPAN_EVENTS : contains
    INGEST_BATCHES ||--|| INGEST_BATCH_PAYLOADS : stores
```

Important non-FK design choice:
- `spans.parent_span_id` is an external text reference, not a foreign key
- `span_events.span_id` is external text and has no FK to `spans`

This is deliberate to allow out-of-order ingest and orphan events.

### 7.3 Table-level semantics

#### `projects`

Purpose:
- tenant boundary
- API key ownership

Important fields:
- `id`
- `name`
- `api_key_hash`

Current auth model:
- API key maps directly to one project
- no user-level auth or org model exists

#### `sessions`

Purpose:
- group related traces

Important fields:
- `id` internal UUID
- `project_id`
- `external_id` human-meaningful external session key
- `name`
- `user_id`
- `metadata`
- `created_at`, `updated_at`

Current usage:
- sessions are created or resolved implicitly from ingest via `external_id`
- current ingest path does not deeply populate `name` or metadata
- session API and UI are read-only

#### `traces`

Purpose:
- represent one agent execution

Important fields:
- internal `id`
- external `trace_id`
- optional `session_id` FK to `sessions.id`
- identity and classification fields: `name`, `user_id`, `tags`, `environment`, `release`
- state fields: `status`, `start_time`, `end_time`, `server_received_at`
- payload fields: `input`, `output`, `metadata`
- rollup fields: `total_spans`, `total_tokens_in`, `total_tokens_out`, `total_cost`, `error_count`, `duration_ms`
- concurrency field: `version`

Current behavior:
- upsert by `(project_id, trace_id)`
- failed or error states cannot be downgraded
- start/end times are merged to keep earliest start and latest end

#### `spans`

Purpose:
- represent operations inside a trace

Important fields:
- internal `id`
- `trace_id` internal UUID FK
- external `span_id`
- external `parent_span_id`
- `name`, `type`, `status`, `status_message`, `level`
- `start_time`, `end_time`, `server_received_at`, `duration_ms`
- structured payloads and truncation metadata for input/output
- model/provider/tokens/cost fields
- `sequence`, `depth`, `version`

Current behavior:
- upsert by `(trace_id, span_id)`
- time merge preserves earliest start and latest end
- status downgrade protection matches trace semantics

#### `span_events`

Purpose:
- append-only event stream attached to traces and spans

Important fields:
- `id`
- `project_id`
- `trace_id`
- external `span_id`
- `event_type`
- `level`
- `event_ts`
- `server_ingested_at`
- `sequence`
- `message`
- `payload`
- truncation metadata
- `idempotency_key`

Current event types accepted by app validation:
- `log`
- `error`
- `exception`
- `message`
- `metric`
- `custom`

Important semantic choice:
- orphans are allowed
- event ordering falls back to server ingest timestamps if client timestamps are absent

#### `ingest_batches`

Purpose:
- durable batch idempotency record
- processing status record
- async ingest control plane

Important fields:
- `id`
- `project_id`
- `batch_key`
- `status`
- `server_received_at`
- `processing_started_at`
- `processing_completed_at`
- `attempt_count`
- item count fields
- `last_error_code`
- `last_error_message`
- `last_error_at`

Public-facing mapped statuses:
- `accepted`
- `processing`
- `completed`
- `failed`

#### `ingest_batch_payloads`

Purpose:
- hold gzipped request payloads for true async ingest until processing completes or cleanup removes failed payloads later

Important fields:
- `batch_id`
- `payload_bytes`
- `compression`
- `content_type`
- `byte_size`
- `created_at`

### 7.4 Migration history and what it tells us

Current Postgres migrations show the real maturity of the platform:

- `000001_initial_schema`: core multi-tenant observability schema
- `000002_fix_default_api_key_hash`: auth fix for default seeded project
- `000003_add_river_tables`: queue infrastructure
- `000004_add_search`: initial search path
- `000005_fix_index_scoping`: project-scoped performance fixes
- `000006_search_generated_columns`: finalized search vector approach
- `000007_trace_version_on_span_change`: rollup correctness fix
- `000008_split_trace_tokens`: token split in trace rollups
- `000009_session_external_id`: external session key support
- `000010_add_true_async_ingest`: async ingest state and payload storage
- `000011_async_ingest_cleanup_index`: cleanup query tuning

This progression matters for Phase 5 because it shows the backend has already gone through several rounds of hardening and is no longer on its first-cut schema.

---

## 8. API Surface and Contract State

### 8.1 Current implemented REST endpoints

Public:
- `GET /api/health`

Protected ingest:
- `POST /v1/ingest`
- `GET /v1/ingest/batches/{id}`

Protected query APIs:
- `GET /api/traces`
- `GET /api/traces/{id}`
- `GET /api/traces/{id}/spans`
- `GET /api/traces/{id}/events`
- `GET /api/sessions`
- `GET /api/sessions/{id}`

### 8.2 Current query capabilities

`GET /api/traces` supports:
- `limit`
- `offset`
- `session_id`
- `q`
- `status`
- `start_time_from`
- `start_time_to`
- `user_id`
- `has_errors`
- `min_duration_ms`

Important nuance:
- backend search and filtering is implemented
- frontend traces page currently does not expose these filters

### 8.3 Ingest contract state

`POST /v1/ingest` currently supports:
- `sync=true` for inline processing
- `X-Continua-Async-Version: 2` for true async opt-in
- server default routing via config

Request contents:
- `batch_key`
- `traces[]`
- `spans[]`
- `events[]`

Current contract alignment details:
- trace `session_id` in ingest is an external session key string, not an internal UUID
- `total_tokens` on span ingest is deprecated
- total-only token payloads are rejected unless directional token fields are present

### 8.4 Response mapping model

The API follows an explicit mapping layer:
- DB types do not leak directly into responses
- statuses are normalized into API enums
- split token totals are mapped from DB columns
- JSON payloads are unmarshaled for API responses

This is aligned with the repository architecture rule that domain and DB types should not leak directly to API consumers.

### 8.5 WebSocket contract status

The repository contains a typed WebSocket contract in `contracts/websocket/events.ts`, but there is no implemented ws backend in the main runtime. The Vite config proxies `/ws`, but the server does not currently expose a completed ws pipeline.

For planning purposes:
- WebSocket schema exists
- WebSocket product behavior does not

---

## 9. End-to-End Workflows

### 9.1 Sync ingest workflow

```mermaid
sequenceDiagram
    participant SDK as Python SDK
    participant API as Go API
    participant ING as Ingest Service
    participant TX as Store Tx
    participant DB as PostgreSQL
    participant RQ as River

    SDK->>API: POST /v1/ingest?sync=true
    API->>ING: Ingest(project_id, request)
    ING->>ING: Validate request
    ING->>TX: Begin transaction
    TX->>DB: Claim batch or get existing
    ING->>TX: Upsert traces
    ING->>TX: Upsert spans
    ING->>TX: Insert events
    ING->>TX: Enqueue rollup jobs in same transaction
    TX->>DB: Commit
    API-->>SDK: 200 OK with counts
```

Current characteristics:
- request is fully processed before response
- duplicate `batch_key` returns a duplicate response without reprocessing
- rollups are deferred to River even in sync ingest mode

### 9.2 True async ingest workflow

```mermaid
sequenceDiagram
    participant SDK as Python SDK
    participant API as Go API
    participant ING as Ingest Service
    participant DB as PostgreSQL
    participant RQ as River
    participant W as IngestBatchWorker

    SDK->>API: POST /v1/ingest + async_v2
    API->>ING: AcceptAsync(project_id, request, raw_payload)
    ING->>ING: Validate request shape
    ING->>DB: Create ingest_batches row
    ING->>DB: Store gzipped payload in ingest_batch_payloads
    ING->>RQ: Insert ingest batch job in same transaction
    API-->>SDK: 202 Accepted + batch_id

    SDK->>API: GET /v1/ingest/batches/{id}
    API-->>SDK: accepted / processing / completed / failed

    RQ->>W: Run ingest batch worker
    W->>DB: Claim queued batch as processing
    W->>DB: Load stored payload
    W->>DB: Run shared processor inside tx
    W->>RQ: Enqueue rollup jobs
    W->>DB: Mark completed and delete payload
```

Current characteristics:
- acceptance-time validation is real
- processing is decoupled from HTTP response
- payload storage is durable
- dependency-not-ready batches retry for a bounded window
- failed payloads are retained temporarily for cleanup

### 9.3 Trace detail query workflow

```mermaid
sequenceDiagram
    participant UI as React UI
    participant API as Go API
    participant DB as PostgreSQL

    UI->>API: GET /api/traces/{id}
    API->>DB: Load scoped trace
    API-->>UI: Trace

    UI->>API: GET /api/traces/{id}/spans
    API->>DB: Load spans ordered by time/sequence
    API-->>UI: SpanList

    UI->>API: GET /api/traces/{id}/events
    API->>DB: Load explicit span_events
    API->>DB: Load spans
    API->>API: Build synthetic lifecycle markers
    API->>API: Merge and paginate timeline
    API-->>UI: TimelineResponse
```

### 9.4 Timeline polling workflow

Current UI behavior:
- fetches the full timeline snapshot page by page
- stores the latest poll cursor
- only polls while trace status is `RUNNING`
- merges new events by event ID and sort order
- does one final full refresh when a trace reaches terminal state

This is a clean intermediate solution between no live updates and full realtime infrastructure.

---

## 10. Current Frontend Architecture

### 10.1 Routing

Current routes:
- `/`
- `/traces`
- `/traces/:id`
- `/sessions`
- `/sessions/:id`

### 10.2 State model

The web app uses:
- React Router for navigation
- TanStack Query for server state
- a simple local API key persistence model via `localStorage`

There is no client-side global app state layer beyond query cache and component state.

### 10.3 Current page responsibilities

`TracesPage`
- paginated trace list
- displays name, status, duration, tokens, cost, start time
- does not yet expose search/filter controls even though the backend supports them

`TraceDetailPage`
- loads one trace
- loads spans for that trace
- renders a span tree on the left
- renders span detail on the right
- renders the merged timeline below

`SessionsPage`
- paginated session list
- displays session ID, name, user ID, trace count, created time

`SessionDetailPage`
- session header
- paginated trace list filtered by session ID

### 10.4 Current UX strengths

- usable trace-to-span inspection flow
- timeline is now genuinely useful for debugging
- session navigation exists and is coherent
- auth gate is simple and effective for local/dev usage

### 10.5 Current frontend weaknesses

- handwritten API types can drift from contract changes
- no UI for backend search/filter functionality
- no frontend test coverage
- sessions are still displayed mostly through internal IDs instead of user-facing external IDs
- no admin or operational views for ingest batches, queue state, or failures

---

## 11. Current SDK State

### 11.1 Python SDK

Primary files:
- `sdks/python/src/continua/client.py`
- `sdks/python/src/continua/trace.py`
- `sdks/python/src/continua/span.py`
- `sdks/python/src/continua/session.py`
- `sdks/python/src/continua/batch.py`

What is implemented:
- batch queue with background flush
- client singleton
- retry with backoff
- trace context and decorator
- span context with model, tokens, cost helpers
- explicit event helpers:
  - `log()`
  - `error()`
  - `exception()`
  - `metric()`
- session context that injects `session_id`, `user_id`, and metadata into traces
- ingest mode control:
  - `sync`
  - `async_v2`
  - `server_default`
- `wait_for_batch()` polling helper

Why it matters:
- this is the strongest external integration surface in the codebase
- it already encodes many of the intended product semantics for sessions and events

### 11.2 TypeScript SDK

Current state:
- version constant
- `ContinuaClient` shell
- trivial tests around base URL

Missing:
- batching
- instrumentation helpers
- ingest support
- auth behavior
- async ingest support
- event helpers

For Phase 5 planning, the TypeScript SDK should be treated as not yet started in product terms.

---

## 12. Testing and Verification State

### 12.1 Current automated coverage shape

Go test coverage exists for:
- API handlers
- auth middleware
- config
- ingest validation and behavior
- jobs
- rollups
- search
- spans
- performance/index plan checks
- truncation helpers

Python test coverage exists for:
- batching
- client behavior
- spans
- traces
- errors
- integration behavior

TypeScript SDK tests exist but are minimal.

Frontend tests:
- effectively none
- validation has relied on type-checking and manual verification

### 12.2 Current test infrastructure assumptions

Integration-heavy Go tests expect a Postgres test database, defaulting to:
- `postgres://continua:continua@localhost:5432/continua_test?sslmode=disable`

This is useful for Phase 5 because any session lifecycle or score feature that touches DB semantics should continue to use real Postgres-backed tests rather than trying to mock the store layer heavily.

### 12.3 Verification drift

The codebase contains phase reports and task lists that indicate tests were run during prior implementation work, but several OpenSpec checklists still show incomplete final validation items. That means:
- the implementation is ahead of the process bookkeeping
- planning should not interpret incomplete task counts as "feature missing" without checking code

---

## 13. Completed, Partial, and Missing Features

### 13.1 Completed or effectively completed

Completed:
- server bootstrap via Fx
- API key authentication and project scoping
- contract-first REST generation flow
- Postgres schema for core observability entities
- idempotent ingest batches
- trace/span/event persistence
- out-of-order tolerant event model
- payload truncation utilities
- async trace rollups
- search and filter capability in backend trace listing
- session list/detail APIs
- session list/detail UI
- merged trace timeline API
- timeline UI with long polling
- Python SDK event helpers
- true async ingest path, batch status API, cleanup worker, and rollout runbook

### 13.2 Partial implementation

Partial:
- search as a product experience
  - backend implemented
  - frontend controls missing

- sessions as a rich domain
  - grouping and read APIs exist
  - lifecycle APIs and strong metadata semantics do not

- true async ingest operational readiness
  - feature mostly implemented
  - metrics and some final validation tasks remain open

- source-of-truth management
  - code is current
  - archived specs and docs are not fully synchronized

### 13.3 Missing

Missing:
- score model and score APIs
- score UI
- explicit session CRUD
- WebSocket runtime
- proxy capture implementation
- replay/export/alerts/state subsystems
- engine implementation
- TypeScript SDK parity
- frontend test suite
- production rate limiting and redaction subsystem

---

## 14. Important Design Decisions Already Made

### 14.1 Contract-first API development

Decision:
- OpenAPI is edited first
- `make generate` propagates generated surfaces

Why it matters:
- keeps REST shapes stable across backend and SDK consumers
- supports deliberate API evolution

### 14.2 Project-scoped multi-tenancy

Decision:
- API keys map to projects
- every request is scoped to a `project_id`

Why it matters:
- simple tenancy model
- avoids premature user/org complexity

### 14.3 Out-of-order tolerance over strong relational strictness

Decision:
- `span_events` has no FK to `spans`
- parent span references are external text IDs

Why it matters:
- ingest can accept realistic distributed event ordering
- debugging data is preserved instead of rejected

### 14.4 Async rollups and async ingest via River on Postgres

Decision:
- background jobs use River instead of introducing Redis or another queue

Why it matters:
- operational simplicity
- one persistence substrate
- fits the current scale and architecture well

### 14.5 Timeline as a computed merged view

Decision:
- timeline is not another table
- it is assembled from `spans` plus `span_events`

Why it matters:
- avoids storage duplication
- keeps schema simpler
- leverages already indexed sources

### 14.6 Session resolution by external key

Decision:
- ingest trace `session_id` is a human or SDK-supplied external string
- the server resolves it to internal session UUIDs

Why it matters:
- better SDK ergonomics
- sessions can map to user conversations or workflow runs without leaking internal IDs to clients

### 14.7 Thin store layer, explicit mapping layer

Decision:
- store wraps DB access
- mapper converts DB rows to API responses

Why it matters:
- keeps transport contracts independent from persistence details
- makes Phase 5 contract changes safer

---

## 15. Current Gaps, Limitations, and Technical Debt

### 15.1 Documentation and spec drift

Examples:
- `config.example.yaml` is broader than the actual env-driven runtime config
- old docs still discuss scores as if they are part of the v1 implementation
- OpenSpec active changes are not fully archived into current-state specs

Impact:
- future planning can easily overestimate what exists

### 15.2 Sessions are structurally present but semantically thin

Current limitation:
- sessions are mostly implicit grouping artifacts created from ingest
- current main ingest path only guarantees `external_id` resolution
- `name`, `user_id`, and session metadata are not deeply modeled across the product

Impact:
- Phase 5 cannot safely build on "sessions" without first deciding what a session means in product terms

### 15.3 No score domain in live code

Current limitation:
- no migrations
- no tables
- no store methods
- no contracts
- no UI

Impact:
- scores are a true new capability, not a thin extension

### 15.4 Frontend/backend feature mismatch

Current mismatch:
- backend supports search and filters
- traces UI does not expose them

Impact:
- product surface understates actual backend capability

### 15.5 Frontend contract drift risk

Current limitation:
- web client uses handwritten response types rather than generated ones

Impact:
- Phase 5 additions could silently drift if contract changes are not mirrored carefully

### 15.6 Weak frontend test coverage

Current limitation:
- no meaningful component or integration tests for the SPA

Impact:
- UI regressions are more likely as Phase 5 adds complexity

### 15.7 Operational visibility gap for async ingest

Current limitation:
- structured logs exist
- metrics called out in OpenSpec are not yet implemented

Impact:
- production troubleshooting of ingest backlog and latency will be weaker than it should be

### 15.8 Placeholder subsystems create false surface area

Current limitation:
- directories such as `internal/proxy`, `internal/ws`, `internal/replay`, and `engine/` suggest broader coverage than exists

Impact:
- an agent planning from folder names alone could incorrectly assume those capabilities are partly done

---

## 16. Phase 5 Planning Implications

### 16.1 What Phase 5 should assume is stable

Safe to build on:
- project-scoped auth model
- trace/span/event data model
- River-based background job model
- session external ID resolution
- current REST API patterns
- timeline merge approach
- Python SDK as primary instrumentation path

### 16.2 What Phase 5 should not assume

Do not assume:
- sessions are already a fully designed product domain
- scores exist in any runtime form
- WebSockets are close to done
- TypeScript SDK is close to parity
- config example reflects runtime reality
- OpenSpec task completion percentages equal implementation status

### 16.3 Most likely Phase 5 workstreams

Recommended Phase 5 workstreams:

1. Session domain formalization
   - define session lifecycle
   - decide whether sessions remain ingest-derived only or gain explicit create/update APIs
   - decide whether UI should pivot to external session identity in more places

2. Score or annotation domain
   - define schema and ownership model
   - define whether scores target traces, spans, sessions, or all three
   - define write path source types: human, evaluator, SDK, automation
   - define query and UI placement

3. Existing capability surfacing
   - expose search/filter controls in traces UI
   - consider operator-facing batch status views if async ingest is now strategic

4. Async ingest hardening closure
   - add metrics
   - finish rollout cleanup
   - finalize validation and smoke-test story

5. Source-of-truth cleanup
   - archive completed changes
   - create current-state OpenSpec specs
   - align config and docs with actual runtime

### 16.4 Suggested order

Recommended order:

1. re-baseline documentation and current-state specs
2. define session semantics
3. define score domain against that session model
4. implement score storage and APIs
5. expose score and richer session behavior in UI
6. close async ingest operational gaps

Reason:
- scores depend on choosing the right target model
- if session semantics change later, score design may need rework

### 16.5 Key decisions Phase 5 must resolve early

Mandatory early decisions:
- Is a session only a grouping key from ingest, or a user-managed object?
- Is session identity user-facing via `external_id`, internal `id`, or both?
- Are scores immutable events, mutable annotations, or both?
- Should scores be attached to traces only, or also spans and sessions?
- Do scores enter through ingest, dedicated CRUD endpoints, or evaluator jobs?
- Does Phase 5 include search/filter UI, or is that intentionally deferred despite backend support?

---

## 17. Recommended Phase 5 Baseline Statement

If a future agent needs a short planning baseline, use this:

"Continua already has a working observability core with authenticated ingest, Postgres persistence, River-backed background jobs, session grouping, trace browsing, and timeline debugging. The next phase should treat sessions as an existing but semantically thin domain, treat scores as a genuinely new capability, and avoid assuming that engine, WebSockets, proxy capture, or TypeScript SDK work is already underway beyond scaffolding."

---

## 18. Appendix: Current Phase Status Readout

### 18.1 Phase-by-phase practical interpretation

Phase 2:
- foundational platform usability is implemented

Phase 3:
- reliability, async rollups, search backend, sessions UI, and Python SDK polish are largely implemented

Phase 4:
- timeline debugging and long polling are implemented

Phase 4.5:
- true async ingest is mostly implemented, with some operational closure still open

Phase 5:
- still open in substance
- should be reframed around session semantics, score/annotation capabilities, and source-of-truth cleanup

### 18.2 Active OpenSpec changes and what they mean

Observed active changes:
- `add-reliability-search-sessions`
- `add-timeline-debugging`
- `add-true-async-ingest`

Interpretation:
- code for these changes is substantially ahead of some checklist statuses
- the repo needs archival and current-state spec cleanup
- planning should use code and this report first, then repair the OpenSpec state as part of early Phase 5 work

