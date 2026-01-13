# Continua Implementation Checklist v1.0

> **Detailed task breakdown for implementing the Continua observability platform**

---

## Overview

This checklist provides a task-level breakdown for implementing Continua v1. Each task includes:
- **Priority**: P0 (critical path), P1 (important), P2 (nice-to-have)
- **Estimate**: Time estimate in hours
- **Dependencies**: Tasks that must be completed first
- **Acceptance Criteria**: Definition of done

---

## Phase 1: Foundation (Weeks 1-3)

### Sprint 1.1: Database Foundation (Week 1)

#### Task 1.1.1: Project Structure Setup
**Priority:** P0 | **Estimate:** 4h | **Dependencies:** None

**Work Items:**
- [ ] Initialize Go module (`go mod init github.com/continua/continua`)
- [ ] Create directory structure:
  ```text
  continua/
  ├── cmd/
  │   └── server/
  │       └── main.go
  ├── internal/
  │   ├── config/
  │   ├── server/
  │   ├── store/
  │   ├── ingest/
  │   └── api/
  ├── migrations/
  ├── pkg/
  │   └── models/
  ├── ui/
  ├── Makefile
  ├── Dockerfile
  └── docker-compose.yml
  ```
- [ ] Add Makefile with targets: `build`, `test`, `migrate`, `dev`
- [ ] Add .gitignore, .editorconfig

**Acceptance Criteria:**
- `go build ./...` succeeds
- `make build` produces binary
- Directory structure follows Go conventions

---

#### Task 1.1.2: Configuration System
**Priority:** P0 | **Estimate:** 3h | **Dependencies:** 1.1.1

**Work Items:**
- [ ] Add Viper dependency (`go get github.com/spf13/viper`)
- [ ] Create `internal/config/config.go`:
  ```go
  type Config struct {
      Server   ServerConfig
      Database DatabaseConfig
      Auth     AuthConfig
  }
  
  type ServerConfig struct {
      Host string `mapstructure:"host"`
      Port int    `mapstructure:"port"`
  }
  
  type DatabaseConfig struct {
      Host         string `mapstructure:"host"`
      Port         int    `mapstructure:"port"`
      Database     string `mapstructure:"database"`
      User         string `mapstructure:"user"`
      Password     string `mapstructure:"password"`
      MaxConns     int    `mapstructure:"max_conns"`
      MinConns     int    `mapstructure:"min_conns"`
  }
  ```
- [ ] Support config from: file, env vars, flags
- [ ] Create example config file `config.example.yaml`
- [ ] Add validation for required fields

**Acceptance Criteria:**
- Config loads from YAML file
- Environment variables override file values
- Missing required values produce clear errors

---

#### Task 1.1.3: Database Connection Pool
**Priority:** P0 | **Estimate:** 4h | **Dependencies:** 1.1.2

**Work Items:**
- [ ] Add pgx/v5 dependency (`go get github.com/jackc/pgx/v5`)
- [ ] Create `internal/store/postgres.go`:
  ```go
  type Store struct {
      pool *pgxpool.Pool
  }
  
  func NewStore(cfg config.DatabaseConfig) (*Store, error) {
      config, err := pgxpool.ParseConfig(cfg.DSN())
      if err != nil {
          return nil, err
      }
      config.MaxConns = int32(cfg.MaxConns)
      config.MinConns = int32(cfg.MinConns)
      
      pool, err := pgxpool.NewWithConfig(ctx, config)
      if err != nil {
          return nil, err
      }
      
      return &Store{pool: pool}, nil
  }
  ```
- [ ] Add connection health check method
- [ ] Add graceful shutdown
- [ ] Add connection metrics (optional P1)

**Acceptance Criteria:**
- Pool connects to Postgres
- Health check verifies connection
- Graceful shutdown closes pool
- Connection errors produce clear messages

---

#### Task 1.1.4: Migration System
**Priority:** P0 | **Estimate:** 3h | **Dependencies:** 1.1.3

**Work Items:**
- [ ] Add goose dependency (`go get github.com/pressly/goose/v3`)
- [ ] Create `internal/store/migrate.go`:
  ```go
  func (s *Store) Migrate(ctx context.Context, dir string) error {
      goose.SetBaseFS(os.DirFS(dir))
      return goose.Up(s.pool, ".")
  }
  ```
- [ ] Add migration CLI command (`cmd/migrate/main.go`)
- [ ] Create migration directory structure
- [ ] Add Makefile targets: `migrate-up`, `migrate-down`, `migrate-create`

**Acceptance Criteria:**
- `make migrate-up` runs all migrations
- `make migrate-down` rolls back one migration
- `make migrate-create name=xxx` creates new migration file

---

#### Task 1.1.5: Core Schema Migrations
**Priority:** P0 | **Estimate:** 6h | **Dependencies:** 1.1.4

**Work Items:**
- [ ] Create migration `001_initial_schema.sql` with:
  - [ ] `projects` table
  - [ ] `api_keys` table
  - [ ] `sessions` table
  - [ ] `traces` table
  - [ ] `spans` table (with all payload and truncation columns)
  - [ ] `span_events` table (with level, no FK to spans)
  - [ ] `scores` table
  - [ ] `external_trace_ids` table
  - [ ] `payload_blobs` table
  - [ ] `ingest_batches` table
  - [ ] `redaction_rules` table
  - [ ] All indexes per schema spec
  - [ ] Triggers for `updated_at`
  - [ ] Helper functions (rollup updates)
- [ ] Verify migration runs without errors
- [ ] Verify rollback works

**Acceptance Criteria:**
- Migration creates all tables
- All indexes created
- Triggers functional
- Rollback drops all objects cleanly

---

### Sprint 1.2: API Foundation (Week 2)

#### Task 1.2.1: HTTP Server Setup
**Priority:** P0 | **Estimate:** 4h | **Dependencies:** 1.1.5

**Work Items:**
- [ ] Add chi router (`go get github.com/go-chi/chi/v5`)
- [ ] Create `internal/server/server.go`:
  ```go
  type Server struct {
      router *chi.Mux
      store  *store.Store
      config *config.Config
  }
  
  func NewServer(cfg *config.Config, store *store.Store) *Server {
      s := &Server{
          router: chi.NewRouter(),
          store:  store,
          config: cfg,
      }
      s.routes()
      return s
  }
  
  func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
      s.router.ServeHTTP(w, r)
  }
  ```
- [ ] Add middleware: logging, recovery, request ID, CORS
- [ ] Create `cmd/server/main.go` entry point
- [ ] Add graceful shutdown handling

**Acceptance Criteria:**
- Server starts and listens on configured port
- Request logging works
- Panics don't crash server
- SIGTERM triggers graceful shutdown

---

#### Task 1.2.2: Health Endpoints
**Priority:** P0 | **Estimate:** 2h | **Dependencies:** 1.2.1

**Work Items:**
- [ ] Create `internal/api/health.go`:
  ```go
  // GET /healthz - liveness (always 200 if server running)
  func (h *Handler) Healthz(w http.ResponseWriter, r *http.Request) {
      w.WriteHeader(http.StatusOK)
      json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
  }
  
  // GET /readyz - readiness (checks DB connection)
  func (h *Handler) Readyz(w http.ResponseWriter, r *http.Request) {
      if err := h.store.Ping(r.Context()); err != nil {
          w.WriteHeader(http.StatusServiceUnavailable)
          return
      }
      w.WriteHeader(http.StatusOK)
  }
  ```
- [ ] Register routes: `/healthz`, `/readyz`
- [ ] Add tests

**Acceptance Criteria:**
- `/healthz` returns 200 when server running
- `/readyz` returns 200 when DB connected
- `/readyz` returns 503 when DB disconnected

---

#### Task 1.2.3: Store Layer - Projects
**Priority:** P0 | **Estimate:** 3h | **Dependencies:** 1.1.5

**Work Items:**
- [ ] Create `internal/store/projects.go`:
  ```go
  func (s *Store) CreateProject(ctx context.Context, p *models.Project) error
  func (s *Store) GetProject(ctx context.Context, id uuid.UUID) (*models.Project, error)
  func (s *Store) GetProjectByName(ctx context.Context, name string) (*models.Project, error)
  func (s *Store) ListProjects(ctx context.Context) ([]*models.Project, error)
  func (s *Store) UpdateProject(ctx context.Context, p *models.Project) error
  func (s *Store) DeleteProject(ctx context.Context, id uuid.UUID) error
  ```
- [ ] Create `pkg/models/project.go`:
  ```go
  type Project struct {
      ID          uuid.UUID
      Name        string
      Description *string
      Settings    map[string]any
      CreatedAt   time.Time
      UpdatedAt   time.Time
  }
  ```
- [ ] Add unit tests with test database

**Acceptance Criteria:**
- CRUD operations work
- Name uniqueness enforced
- Tests pass

---

#### Task 1.2.4: Store Layer - API Keys
**Priority:** P0 | **Estimate:** 4h | **Dependencies:** 1.2.3

**Work Items:**
- [ ] Create `internal/store/api_keys.go`:
  ```go
  func (s *Store) CreateAPIKey(ctx context.Context, key *models.APIKey) error
  func (s *Store) GetAPIKeyByPublicKey(ctx context.Context, publicKey string) (*models.APIKey, error)
  func (s *Store) ListAPIKeys(ctx context.Context, projectID uuid.UUID) ([]*models.APIKey, error)
  func (s *Store) RevokeAPIKey(ctx context.Context, id uuid.UUID) error
  func (s *Store) UpdateLastUsed(ctx context.Context, id uuid.UUID) error
  ```
- [ ] Create `pkg/models/api_key.go`:
  ```go
  type APIKey struct {
      ID           uuid.UUID
      ProjectID    uuid.UUID
      PublicKey    string    // pk_xxx
      HashedSecret string    // bcrypt hash of sk_xxx
      Name         *string
      Scopes       []string  // ingest, query, admin
      ExpiresAt    *time.Time
      LastUsedAt   *time.Time
      RevokedAt    *time.Time
      CreatedAt    time.Time
  }
  ```
- [ ] Add key generation helpers (pk_/sk_ prefixes, secure random)
- [ ] Add bcrypt hashing for secret

**Acceptance Criteria:**
- Keys generated with proper prefixes
- Secret hashed before storage
- Lookup by public key works
- Revocation sets timestamp

---

#### Task 1.2.5: Auth Middleware
**Priority:** P0 | **Estimate:** 4h | **Dependencies:** 1.2.4

**Work Items:**
- [ ] Create `internal/api/middleware/auth.go`:
  ```go
  func APIKeyAuth(store *store.Store) func(next http.Handler) http.Handler {
      return func(next http.Handler) http.Handler {
          return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
              // Extract from Authorization: Bearer pk_xxx:sk_xxx
              auth := r.Header.Get("Authorization")
              publicKey, secret, err := parseAuthHeader(auth)
              if err != nil {
                  http.Error(w, "Invalid authorization", 401)
                  return
              }
              
              // Lookup and verify
              key, err := store.GetAPIKeyByPublicKey(r.Context(), publicKey)
              if err != nil || key.RevokedAt != nil {
                  http.Error(w, "Invalid API key", 401)
                  return
              }
              
              if !bcrypt.CompareHashAndPassword(key.HashedSecret, secret) {
                  http.Error(w, "Invalid API key", 401)
                  return
              }
              
              // Add to context
              ctx := context.WithValue(r.Context(), "api_key", key)
              ctx = context.WithValue(ctx, "project_id", key.ProjectID)
              
              // Update last used (async)
              go store.UpdateLastUsed(context.Background(), key.ID)
              
              next.ServeHTTP(w, r.WithContext(ctx))
          })
      }
  }
  ```
- [ ] Add scope checking helper
- [ ] Add rate limiting hooks (placeholder for Phase 7)
- [ ] Add tests

**Acceptance Criteria:**
- Valid key allows request
- Invalid/revoked key returns 401
- Project ID available in context
- Missing scope returns 403

---

#### Task 1.2.6: Projects API Endpoints
**Priority:** P0 | **Estimate:** 3h | **Dependencies:** 1.2.5

**Work Items:**
- [ ] Create `internal/api/projects.go`:
  ```go
  // POST /v1/projects
  func (h *Handler) CreateProject(w http.ResponseWriter, r *http.Request)
  
  // GET /v1/projects
  func (h *Handler) ListProjects(w http.ResponseWriter, r *http.Request)
  
  // GET /v1/projects/{project_id}
  func (h *Handler) GetProject(w http.ResponseWriter, r *http.Request)
  ```
- [ ] Add request/response DTOs
- [ ] Register routes
- [ ] Add integration tests

**Acceptance Criteria:**
- Can create project
- Can list projects
- Can get single project
- Validation errors return 400

---

#### Task 1.2.7: API Keys Endpoints
**Priority:** P0 | **Estimate:** 3h | **Dependencies:** 1.2.6

**Work Items:**
- [ ] Create `internal/api/api_keys.go`:
  ```go
  // POST /v1/api-keys
  func (h *Handler) CreateAPIKey(w http.ResponseWriter, r *http.Request)
  
  // GET /v1/api-keys
  func (h *Handler) ListAPIKeys(w http.ResponseWriter, r *http.Request)
  
  // DELETE /v1/api-keys/{key_id}
  func (h *Handler) RevokeAPIKey(w http.ResponseWriter, r *http.Request)
  ```
- [ ] Return secret only on create (never again)
- [ ] Mask secret in list responses
- [ ] Add integration tests

**Acceptance Criteria:**
- Can create API key, secret returned once
- Can list keys (secret masked)
- Can revoke key
- Revoked key stops working

---

### Sprint 1.3: Ingestion & Query (Week 3)

#### Task 1.3.1: Store Layer - Traces
**Priority:** P0 | **Estimate:** 4h | **Dependencies:** 1.2.3

**Work Items:**
- [ ] Create `internal/store/traces.go`:
  ```go
  func (s *Store) UpsertTrace(ctx context.Context, t *models.Trace) error
  func (s *Store) GetTrace(ctx context.Context, projectID, traceID uuid.UUID) (*models.Trace, error)
  func (s *Store) GetTraceByExternalID(ctx context.Context, projectID uuid.UUID, externalTraceID string) (*models.Trace, error)
  func (s *Store) ListTraces(ctx context.Context, filter TraceFilter) (*TracePage, error)
  func (s *Store) UpdateTraceRollups(ctx context.Context, traceID uuid.UUID) error
  ```
- [ ] Create `pkg/models/trace.go` with all fields
- [ ] Implement upsert with patch semantics (COALESCE)
- [ ] Implement list with filters: name, status, tags, environment, time range
- [ ] Implement cursor-based pagination

**Acceptance Criteria:**
- Upsert creates or updates trace
- Patch semantics don't overwrite with NULL
- Filters work correctly
- Pagination works

---

#### Task 1.3.2: Store Layer - Spans
**Priority:** P0 | **Estimate:** 5h | **Dependencies:** 1.3.1

**Work Items:**
- [ ] Create `internal/store/spans.go`:
  ```go
  func (s *Store) UpsertSpan(ctx context.Context, span *models.Span) error
  func (s *Store) GetSpan(ctx context.Context, id uuid.UUID) (*models.Span, error)
  func (s *Store) GetSpanByExternalID(ctx context.Context, traceID uuid.UUID, spanID string) (*models.Span, error)
  func (s *Store) ListSpans(ctx context.Context, traceID uuid.UUID) ([]*models.Span, error)
  func (s *Store) ListSpansSummary(ctx context.Context, traceID uuid.UUID) ([]*models.SpanSummary, error)
  ```
- [ ] Create `pkg/models/span.go` with all fields including truncation metadata
- [ ] Create `pkg/models/span_summary.go` (without full payloads)
- [ ] Implement upsert with patch semantics
- [ ] Handle status precedence (error wins)

**Acceptance Criteria:**
- Upsert creates or updates span
- Patch semantics work correctly
- Error status preserved on updates
- Summary query excludes large payloads

---

#### Task 1.3.3: Store Layer - Span Events
**Priority:** P0 | **Estimate:** 4h | **Dependencies:** 1.3.2

**Work Items:**
- [ ] Create `internal/store/span_events.go`:
  ```go
  func (s *Store) InsertSpanEvents(ctx context.Context, events []*models.SpanEvent) error
  func (s *Store) ListSpanEvents(ctx context.Context, filter SpanEventFilter) ([]*models.SpanEvent, error)
  func (s *Store) ListOrphanEvents(ctx context.Context, traceID uuid.UUID) ([]*models.SpanEvent, error)
  ```
- [ ] Create `pkg/models/span_event.go`:
  ```go
  type SpanEvent struct {
      ID               uuid.UUID
      ProjectID        uuid.UUID
      TraceID          uuid.UUID
      SpanID           string   // TEXT, not UUID
      EventType        string
      Level            string   // debug, info, warn, error
      EventTS          *time.Time
      ServerIngestedAt time.Time
      Sequence         *int
      Message          *string
      Payload          map[string]any
      Truncated        bool
      OriginalSizeBytes *int64
      TruncationReason *string
      IdempotencyKey   *string
  }
  ```
- [ ] Implement batch insert with idempotency (ON CONFLICT DO NOTHING)
- [ ] Implement orphan events query

**Acceptance Criteria:**
- Batch insert succeeds
- Duplicate idempotency keys ignored (not error)
- Orphan events query returns events without matching spans
- Events ordered by server_ingested_at, sequence

---

#### Task 1.3.4: JSON Wrapper Utility
**Priority:** P0 | **Estimate:** 2h | **Dependencies:** None

**Work Items:**
- [ ] Create `pkg/jsonutil/wrapper.go`:
  ```go
  // WrapPayload ensures valid JSONB by wrapping invalid JSON
  func WrapPayload(data []byte) ([]byte, error) {
      var js json.RawMessage
      if err := json.Unmarshal(data, &js); err != nil {
          wrapped := map[string]any{
              "__continua_raw":   string(data),
              "__parse_error":    err.Error(),
              "__content_type":   "text/plain",
          }
          return json.Marshal(wrapped)
      }
      return data, nil
  }
  
  // IsWrapped checks if payload was wrapped due to invalid JSON
  func IsWrapped(data map[string]any) bool {
      _, hasRaw := data["__continua_raw"]
      return hasRaw
  }
  ```
- [ ] Add tests for various invalid JSON scenarios

**Acceptance Criteria:**
- Valid JSON passed through unchanged
- Invalid JSON wrapped with error details
- Large strings handled without panic

---

#### Task 1.3.5: Ingest Request DTOs
**Priority:** P0 | **Estimate:** 3h | **Dependencies:** 1.3.4

**Work Items:**
- [ ] Create `internal/api/dto/ingest.go`:
  ```go
  type IngestRequest struct {
      BatchKey  string         `json:"batch_key"`
      SDKInfo   *SDKInfo       `json:"sdk_info"`
      Traces    []TraceInput   `json:"traces"`
      Spans     []SpanInput    `json:"spans"`
      Events    []EventInput   `json:"events"`
      Scores    []ScoreInput   `json:"scores"`
  }
  
  type TraceInput struct {
      TraceID     string         `json:"trace_id" validate:"required"`
      Name        *string        `json:"name"`
      StartTime   *time.Time     `json:"start_time"`
      EndTime     *time.Time     `json:"end_time"`
      Tags        []string       `json:"tags"`
      Environment *string        `json:"environment"`
      Metadata    map[string]any `json:"metadata"`
      Input       json.RawMessage `json:"input"`
      Output      json.RawMessage `json:"output"`
  }
  
  type SpanInput struct {
      TraceID      string          `json:"trace_id" validate:"required"`
      SpanID       string          `json:"span_id" validate:"required"`
      ParentSpanID *string         `json:"parent_span_id"`
      Name         string          `json:"name" validate:"required"`
      Type         string          `json:"type"`
      Status       *string         `json:"status"`
      StartTime    time.Time       `json:"start_time" validate:"required"`
      EndTime      *time.Time      `json:"end_time"`
      Model        *string         `json:"model"`
      Input        json.RawMessage `json:"input"`
      Output       json.RawMessage `json:"output"`
      Usage        *UsageInput     `json:"usage"`
      // ... etc
  }
  ```
- [ ] Add validation using go-playground/validator
- [ ] Create response DTOs

**Acceptance Criteria:**
- All fields mapped correctly
- Required fields validated
- JSON payloads kept as raw bytes until processing

---

#### Task 1.3.6: Ingest Endpoint (Sync Mode)
**Priority:** P0 | **Estimate:** 8h | **Dependencies:** 1.3.5

**Work Items:**
- [ ] Create `internal/api/ingest.go`:
  ```go
  // POST /v1/ingest
  func (h *Handler) Ingest(w http.ResponseWriter, r *http.Request) {
      projectID := r.Context().Value("project_id").(uuid.UUID)
      
      // Enforce 5MB batch size limit
      r.Body = http.MaxBytesReader(w, r.Body, 5*1024*1024)
      
      var req dto.IngestRequest
      if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
          if err.Error() == "http: request body too large" {
              http.Error(w, `{"error":"batch exceeds 5MB limit"}`, 413)
              return
          }
          // return 400
      }
      
      // Validate
      if err := h.validator.Struct(req); err != nil {
          // return 400 with validation errors
      }
      
      // Process synchronously for now
      result, err := h.ingestService.Process(r.Context(), projectID, &req)
      if err != nil {
          // return 500
      }
      
      // Return 200 with item statuses (not 202 for sync mode)
      w.WriteHeader(http.StatusOK)
      json.NewEncoder(w).Encode(result)
  }
  ```
- [ ] Create `internal/ingest/service.go` with correct transaction order:
  ```go
  func (s *IngestService) Process(ctx context.Context, projectID uuid.UUID, req *dto.IngestRequest) (*IngestResult, error) {
      tx, err := s.store.BeginTx(ctx)
      if err != nil {
          return nil, err
      }
      defer tx.Rollback(ctx)
      
      // STEP 1: Claim batch idempotency (FIRST!)
      batchID, err := s.claimBatch(ctx, tx, projectID, req.BatchKey)
      if err == ErrDuplicateBatch {
          // Already processed - return success
          return &IngestResult{Status: "duplicate", BatchKey: req.BatchKey}, nil
      }
      if err != nil {
          return nil, err
      }
      
      // STEP 2: Upsert traces and build ID map
      // CRITICAL: spans.trace_id FK → traces.id (UUID)
      // SDK sends trace_id as TEXT, we need internal UUID
      traceMap := make(map[string]uuid.UUID) // external trace_id → internal UUID
      for _, t := range req.Traces {
          wrapped := wrapPayloads(t)
          truncated := checkAndTruncate(wrapped)
          internalID, err := s.upsertTrace(ctx, tx, projectID, truncated)
          if err != nil {
              return nil, err
          }
          traceMap[t.TraceID] = internalID
      }
      
      // STEP 3: Upsert spans using trace UUID map
      for _, span := range req.Spans {
          traceUUID, ok := traceMap[span.TraceID]
          if !ok {
              // Trace not in this batch - lookup from DB
              traceUUID, err = s.getTraceUUID(ctx, tx, projectID, span.TraceID)
              if err != nil {
                  return nil, fmt.Errorf("trace %s not found", span.TraceID)
              }
              traceMap[span.TraceID] = traceUUID
          }
          wrapped := wrapPayloads(span)
          truncated := checkAndTruncate(wrapped)
          err := s.upsertSpan(ctx, tx, traceUUID, truncated)
          if err != nil {
              return nil, err
          }
      }
      
      // STEP 4: Insert events (append-only)
      for _, event := range req.Events {
          traceUUID, ok := traceMap[event.TraceID]
          if !ok {
              traceUUID, err = s.getTraceUUID(ctx, tx, projectID, event.TraceID)
              if err != nil {
                  continue // Skip orphan events to unknown traces
              }
              traceMap[event.TraceID] = traceUUID
          }
          wrapped := wrapPayload(event.Payload)
          err := s.insertEvent(ctx, tx, projectID, traceUUID, wrapped)
          // ON CONFLICT DO NOTHING - duplicates silently ignored
      }
      
      // STEP 5: Update batch status
      err = s.updateBatchStatus(ctx, tx, batchID, "accepted", counts)
      if err != nil {
          return nil, err
      }
      
      // COMMIT
      if err := tx.Commit(ctx); err != nil {
          return nil, err
      }
      
      // STEP 6: Trigger rollup updates (async)
      for _, traceUUID := range traceMap {
          s.queue.Enqueue(RollupJob{TraceID: traceUUID})
      }
      
      return &IngestResult{Status: "ok", ...}, nil
  }
  ```
- [ ] Implement size checking and truncation
- [ ] Implement JSON wrapping for invalid payloads
- [ ] Return per-item status (created/updated/duplicate/invalid)

**Acceptance Criteria:**
- Batch ingest works with traces, spans, events
- Batch idempotency claimed first (duplicates return success)
- Trace ID mapping works correctly (external TEXT → internal UUID)
- Invalid JSON wrapped correctly
- Truncation metadata set for large payloads
- Response includes per-item status
- 5MB limit enforced (413 error)
- Duplicate batches return 200 with status "duplicate"

---

#### Task 1.3.7: Trace List Endpoint
**Priority:** P0 | **Estimate:** 4h | **Dependencies:** 1.3.1

**Work Items:**
- [ ] Create `internal/api/traces.go`:
  ```go
  // GET /v1/traces
  func (h *Handler) ListTraces(w http.ResponseWriter, r *http.Request) {
      filter := TraceFilter{
          ProjectID:   getProjectID(r.Context()),
          Name:        r.URL.Query().Get("name"),
          Status:      r.URL.Query().Get("status"),
          Environment: r.URL.Query().Get("environment"),
          Tags:        r.URL.Query()["tag"],
          After:       parseTime(r.URL.Query().Get("after")),
          Before:      parseTime(r.URL.Query().Get("before")),
          Limit:       parseLimit(r.URL.Query().Get("limit"), 50),
          Cursor:      r.URL.Query().Get("cursor"),
      }
      
      page, err := h.store.ListTraces(r.Context(), filter)
      // ...
  }
  ```
- [ ] Implement cursor-based pagination
- [ ] Add sorting by server_received_at DESC

**Acceptance Criteria:**
- List returns traces for project
- Filters work correctly
- Pagination works with cursor
- Response includes next_cursor

---

#### Task 1.3.8: Trace Detail Endpoint
**Priority:** P0 | **Estimate:** 3h | **Dependencies:** 1.3.7

**Work Items:**
- [ ] Add to `internal/api/traces.go`:
  ```go
  // GET /v1/traces/{trace_id}
  func (h *Handler) GetTrace(w http.ResponseWriter, r *http.Request) {
      traceID := chi.URLParam(r, "trace_id")
      
      trace, err := h.store.GetTraceByExternalID(r.Context(), projectID, traceID)
      if err != nil {
          // 404 if not found
      }
      
      // Get spans (summary - no full payloads)
      spans, err := h.store.ListSpansSummary(r.Context(), trace.ID)
      
      // Get orphan events count
      orphanCount, err := h.store.CountOrphanEvents(r.Context(), trace.ID)
      
      response := TraceDetailResponse{
          Trace:            trace,
          Spans:            spans,
          OrphanEventCount: orphanCount,
      }
      
      json.NewEncoder(w).Encode(response)
  }
  ```

**Acceptance Criteria:**
- Returns trace with spans
- Spans don't include full payloads
- Orphan event count included

---

#### Task 1.3.9: Span Detail & Events Endpoints
**Priority:** P0 | **Estimate:** 4h | **Dependencies:** 1.3.8

**Work Items:**
- [ ] Create `internal/api/spans.go`:
  ```go
  // GET /v1/traces/{trace_id}/spans/{span_id}
  // Returns full span with all payloads
  func (h *Handler) GetSpan(w http.ResponseWriter, r *http.Request) {
      traceID := chi.URLParam(r, "trace_id")
      spanID := chi.URLParam(r, "span_id")
      projectID := getProjectID(r.Context())
      
      // First get trace to verify access and get internal UUID
      trace, err := h.store.GetTraceByExternalID(r.Context(), projectID, traceID)
      if err != nil {
          http.Error(w, "trace not found", 404)
          return
      }
      
      // Get span by external span_id within this trace
      span, err := h.store.GetSpanByExternalID(r.Context(), trace.ID, spanID)
      if err != nil {
          http.Error(w, "span not found", 404)
          return
      }
      
      json.NewEncoder(w).Encode(span)
  }
  
  // GET /v1/spans/{span_uuid}
  // Alternative: lookup by internal UUID (useful when UI has span.id)
  func (h *Handler) GetSpanByUUID(w http.ResponseWriter, r *http.Request) {
      spanUUID := chi.URLParam(r, "span_uuid")
      projectID := getProjectID(r.Context())
      
      span, err := h.store.GetSpan(r.Context(), uuid.MustParse(spanUUID))
      if err != nil || span.ProjectID != projectID {
          http.Error(w, "span not found", 404)
          return
      }
      
      json.NewEncoder(w).Encode(span)
  }
  
  // GET /v1/traces/{trace_id}/spans/{span_id}/events
  func (h *Handler) ListSpanEvents(w http.ResponseWriter, r *http.Request) {
      traceID := chi.URLParam(r, "trace_id")
      spanID := chi.URLParam(r, "span_id")
      
      // ... get trace UUID, then query events
      events, err := h.store.ListSpanEvents(r.Context(), SpanEventFilter{
          TraceID: traceUUID,
          SpanID:  spanID,
      })
      
      json.NewEncoder(w).Encode(events)
  }
  
  // GET /v1/traces/{trace_id}/orphan-events
  func (h *Handler) ListOrphanEvents(w http.ResponseWriter, r *http.Request) {
      // ...
  }
  ```
- [ ] Register routes with proper nesting:
  ```go
  r.Route("/v1/traces/{trace_id}", func(r chi.Router) {
      r.Get("/", h.GetTrace)
      r.Get("/spans", h.ListSpans)
      r.Get("/spans/{span_id}", h.GetSpan)
      r.Get("/spans/{span_id}/events", h.ListSpanEvents)
      r.Get("/events", h.ListTraceEvents)
      r.Get("/orphan-events", h.ListOrphanEvents)
  })
  r.Get("/v1/spans/{span_uuid}", h.GetSpanByUUID)  // Alternative by UUID
  ```
- [ ] Full span includes all payloads (input, output, thinking)
- [ ] Events ordered by: `ORDER BY COALESCE(event_ts, server_ingested_at), sequence`

**Acceptance Criteria:**
- Span detail includes full payloads
- Events endpoint returns events for span
- Events ordered correctly (client time preferred, server time fallback)
- Nested routes work: `/v1/traces/{trace_id}/spans/{span_id}`
- Alternative UUID route works: `/v1/spans/{span_uuid}`

---

#### Task 1.3.10: Docker Compose Setup
**Priority:** P0 | **Estimate:** 2h | **Dependencies:** 1.3.9

**Work Items:**
- [ ] Create `docker-compose.yml`:
  ```yaml
  version: '3.8'
  services:
    postgres:
      image: postgres:15
      environment:
        POSTGRES_DB: continua
        POSTGRES_USER: continua
        POSTGRES_PASSWORD: continua
      ports:
        - "5432:5432"
      volumes:
        - postgres_data:/var/lib/postgresql/data
    
    server:
      build: .
      ports:
        - "8080:8080"
      environment:
        DATABASE_HOST: postgres
        DATABASE_PORT: 5432
        DATABASE_USER: continua
        DATABASE_PASSWORD: continua
        DATABASE_NAME: continua
      depends_on:
        - postgres
  
  volumes:
    postgres_data:
  ```
- [ ] Create `Dockerfile` with multi-stage build
- [ ] Add `make docker-up`, `make docker-down` targets

**Acceptance Criteria:**
- `make docker-up` starts Postgres and server
- Server connects to Postgres
- Health endpoints accessible

---

#### Task 1.3.11: Integration Tests
**Priority:** P0 | **Estimate:** 4h | **Dependencies:** 1.3.10

**Work Items:**
- [ ] Create test database setup/teardown
- [ ] Create `internal/api/ingest_test.go`:
  - Test batch ingest with traces, spans, events
  - Test invalid JSON wrapping
  - Test idempotency
  - Test payload truncation
- [ ] Create `internal/api/traces_test.go`:
  - Test list with filters
  - Test pagination
  - Test detail with spans

**Acceptance Criteria:**
- All integration tests pass
- Tests use real Postgres (via test container or docker-compose)
- Tests cover happy path and error cases

---

## Phase 1 Exit Criteria

Before moving to Phase 2, verify:

- [ ] Can create project via API
- [ ] Can create API key, receive secret once
- [ ] Can ingest batch with traces, spans, events
- [ ] **Batch idempotency works (duplicate batch returns success, not error)**
- [ ] **Trace ID mapping works (external TEXT → internal UUID)**
- [ ] Invalid JSON wrapped correctly (not rejected)
- [ ] Large payloads truncated with metadata
- [ ] **Batch size limit enforced (413 for >5MB)**
- [ ] Idempotent requests handled correctly
- [ ] Can list traces with filters
- [ ] Can get trace detail with spans (includes span_uuid)
- [ ] **Can get span via nested route: `/v1/traces/{trace_id}/spans/{span_id}`**
- [ ] **Can get span via UUID route: `/v1/spans/{span_uuid}`**
- [ ] Can get events for span
- [ ] Orphan events queryable
- [ ] Migrations run cleanly
- [ ] Tests pass
- [ ] Docker compose works
- [ ] **HTTP status codes correct (200/202, no 409 for duplicates)**

---

## Phase 2: SDK & Batching (Weeks 4-5)

### Sprint 2.1: Queue & Async Ingest (Week 4)

#### Task 2.1.1: River Queue Setup
**Priority:** P0 | **Estimate:** 4h | **Dependencies:** Phase 1

**Work Items:**
- [ ] Add River dependency (`go get github.com/riverqueue/river`)
- [ ] Create `internal/queue/river.go`:
  ```go
  type RiverQueue struct {
      client *river.Client[pgx.Tx]
  }
  
  func NewRiverQueue(pool *pgxpool.Pool) (*RiverQueue, error)
  ```
- [ ] Run River migrations
- [ ] Add queue health check

---

#### Task 2.1.2: Ingest Job Definition
**Priority:** P0 | **Estimate:** 3h | **Dependencies:** 2.1.1

**Work Items:**
- [ ] Create `internal/queue/jobs/ingest.go`:
  ```go
  type IngestJobArgs struct {
      ProjectID uuid.UUID
      BatchKey  string
      Payload   []byte
  }
  
  func (IngestJobArgs) Kind() string { return "ingest" }
  
  type IngestWorker struct {
      service *ingest.Service
  }
  
  func (w *IngestWorker) Work(ctx context.Context, job *river.Job[IngestJobArgs]) error {
      var req dto.IngestRequest
      json.Unmarshal(job.Args.Payload, &req)
      _, err := w.service.Process(ctx, job.Args.ProjectID, &req)
      return err
  }
  ```

---

#### Task 2.1.3: Async Ingest Mode
**Priority:** P0 | **Estimate:** 3h | **Dependencies:** 2.1.2

**Work Items:**
- [ ] Update ingest endpoint:
  ```go
  func (h *Handler) Ingest(w http.ResponseWriter, r *http.Request) {
      sync := r.URL.Query().Get("sync") == "true"
      
      if sync {
          // Process inline (existing code)
      } else {
          // Enqueue to River
          job := IngestJobArgs{
              ProjectID: projectID,
              BatchKey:  req.BatchKey,
              Payload:   bodyBytes,
          }
          h.queue.Enqueue(r.Context(), job)
          
          w.WriteHeader(http.StatusAccepted)
          json.NewEncoder(w).Encode(map[string]string{
              "status": "accepted",
              "batch_key": req.BatchKey,
          })
      }
  }
  ```
- [ ] Add batch audit record

---

### Sprint 2.2: Python SDK (Week 5)

#### Task 2.2.1: SDK Project Setup
**Priority:** P0 | **Estimate:** 3h | **Dependencies:** None

**Work Items:**
- [ ] Create `sdk/python/` directory
- [ ] Initialize with pyproject.toml
- [ ] Set up pytest, black, mypy
- [ ] Create package structure

---

#### Task 2.2.2: SDK Client Core
**Priority:** P0 | **Estimate:** 4h | **Dependencies:** 2.2.1

**Work Items:**
- [ ] Create `continua/client.py`:
  ```python
  class ContinuaClient:
      def __init__(
          self,
          api_key: str,
          base_url: str = "https://api.continua.dev",
          environment: str | None = None,
      ):
          self._api_key = api_key
          self._base_url = base_url
          self._environment = environment
          self._batch_queue = BatchQueue()
          self._worker = BackgroundWorker(self._batch_queue, self._send_batch)
          
      def trace(self, name: str, **kwargs) -> TraceContext:
          return TraceContext(self, name, **kwargs)
          
      def span(self, name: str, **kwargs) -> SpanContext:
          return SpanContext(self, name, **kwargs)
  ```

---

#### Task 2.2.3: SDK Context Managers
**Priority:** P0 | **Estimate:** 4h | **Dependencies:** 2.2.2

**Work Items:**
- [ ] Create `continua/context.py`:
  ```python
  class TraceContext:
      def __enter__(self) -> "TraceContext":
          self._start_time = datetime.utcnow()
          return self
          
      def __exit__(self, exc_type, exc_val, exc_tb):
          self._end_time = datetime.utcnow()
          self._status = "error" if exc_type else "ok"
          self._client._enqueue_trace(self._to_dict())
          
      def span(self, name: str, **kwargs) -> "SpanContext":
          return SpanContext(self._client, self, name, **kwargs)
  ```

---

#### Task 2.2.4: SDK Batching
**Priority:** P0 | **Estimate:** 5h | **Dependencies:** 2.2.3

**Work Items:**
- [ ] Create `continua/batch/queue.py`:
  ```python
  class BatchQueue:
      """
      Batching with start/end merge.
      
      CRITICAL: Never create "end-only" spans. If span end arrives
      without corresponding start, log warning and drop. The DB
      requires start_time and name which only come from span start.
      """
      def __init__(
          self,
          max_size: int = 100,
          max_bytes: int = 1_000_000,
          flush_interval: float = 1.0,
      ):
          self._traces: dict[str, TraceData] = {}
          self._spans: dict[str, SpanData] = {}
          self._events: list[EventData] = []
          self._lock = threading.Lock()
          
      def enqueue_span_start(self, span: SpanData):
          """Record span start - creates the span entry."""
          with self._lock:
              key = f"{span.trace_id}:{span.span_id}"
              self._spans[key] = span
              
      def enqueue_span_end(self, trace_id: str, span_id: str, **updates):
          """
          Record span end - merges into existing span.
          
          If no start exists, logs warning and DROPS the data.
          We do NOT create partial spans without required fields.
          """
          with self._lock:
              key = f"{trace_id}:{span_id}"
              if key in self._spans:
                  # Merge end data into existing start
                  self._spans[key].merge_end(**updates)
              else:
                  # No start received - cannot create valid span
                  logger.warning(
                      f"Span end without start: {span_id}. "
                      f"Data dropped. Check instrumentation."
                  )
                  # DO NOT create partial span - it would fail DB constraints
  ```
- [ ] Implement background flush worker
- [ ] Add graceful shutdown (flush remaining on exit)
- [ ] Add tests for merge behavior and orphan end handling

**Acceptance Criteria:**
- Start/end correctly merged for same span_id
- Orphan ends logged and dropped (not sent to server)
- Flush triggers on size, bytes, or interval
- Graceful shutdown flushes remaining data

---

#### Task 2.2.5: SDK Retry Logic
**Priority:** P0 | **Estimate:** 3h | **Dependencies:** 2.2.4

**Work Items:**
- [ ] Create `continua/transport/retry.py`:
  ```python
  class RetryTransport:
      def send(self, payload: bytes) -> Response:
          for attempt in range(self.max_retries + 1):
              try:
                  response = self._http.post(...)
                  if response.status_code == 429:
                      time.sleep(self._get_retry_after(response))
                      continue
                  if response.status_code >= 500:
                      time.sleep(self._backoff(attempt))
                      continue
                  return response
              except (ConnectionError, Timeout):
                  if attempt == self.max_retries:
                      raise
                  time.sleep(self._backoff(attempt))
  ```

---

#### Task 2.2.6: SDK Decorators
**Priority:** P1 | **Estimate:** 3h | **Dependencies:** 2.2.3

**Work Items:**
- [ ] Create `continua/decorators.py`:
  ```python
  def trace(name: str = None, **kwargs):
      def decorator(func):
          @functools.wraps(func)
          def wrapper(*args, **call_kwargs):
              trace_name = name or func.__name__
              with get_client().trace(trace_name, **kwargs):
                  return func(*args, **call_kwargs)
          return wrapper
      return decorator
  ```

---

#### Task 2.2.7: SDK Events API
**Priority:** P0 | **Estimate:** 3h | **Dependencies:** 2.2.3

**Work Items:**
- [ ] Add event methods to SpanContext:
  ```python
  class SpanContext:
      def add_event(
          self,
          event_type: str,
          level: str = "info",
          message: str | None = None,
          payload: dict | None = None,
      ):
          event = SpanEvent(
              trace_id=self._trace_id,
              span_id=self._span_id,
              event_type=event_type,
              level=level,
              event_ts=datetime.utcnow(),
              message=message,
              payload=payload,
          )
          self._client._enqueue_event(event)
  ```

---

#### Task 2.2.8: SDK Tests
**Priority:** P0 | **Estimate:** 4h | **Dependencies:** 2.2.7

**Work Items:**
- [ ] Unit tests for batching logic
- [ ] Unit tests for retry logic
- [ ] Integration tests against test server
- [ ] Test context manager cleanup on exception

---

## Phase 2 Exit Criteria

- [ ] River queue processes ingest jobs
- [ ] Async ingest returns 202
- [ ] Sync mode still works (?sync=true)
- [ ] Python SDK traces code with context managers
- [ ] SDK batching merges start/end
- [ ] SDK retries on failure
- [ ] SDK flushes on shutdown

---

## Phase 3-7 Summary

### Phase 3: Web UI (Weeks 6-8)
- UI scaffolding (Vite + React + Tailwind)
- Trace list page with filters
- Trace detail with span tree
- Span inspector panel
- Events timeline view
- JSON viewer
- Embed UI in Go binary

### Phase 4: Payload Management (Weeks 9-10)
- Blob storage implementation
- Size threshold detection (64KB)
- Automatic blob upload
- Truncation metadata
- Lazy loading in UI
- Blob cleanup worker

### Phase 5: Sessions & Scores (Weeks 11-12)
- Session CRUD endpoints
- Session-trace linking
- Session list/detail UI
- Score CRUD endpoints
- Score display in UI

### Phase 6: OTel & Integrations (Weeks 13-14)
- OTLP HTTP endpoint
- OTel → Continua mapping
- External trace ID table population
- LangChain callback handler
- OpenAI/Anthropic wrappers

### Phase 7: Production Polish (Weeks 15-16)
- Redaction rules implementation
- Rate limiting
- Prometheus metrics
- Performance optimization
- Documentation
- Helm chart

---

## Appendix: Definition of Done

A task is complete when:

1. **Code complete**: Implementation matches specification
2. **Tests pass**: Unit and integration tests written and passing
3. **Documented**: Code comments, API docs updated
4. **Reviewed**: Code reviewed and approved
5. **Merged**: PR merged to main branch

---

## Appendix: Risk Register

| Risk | Impact | Mitigation |
|------|--------|------------|
| Payload size explosion | DB bloat, slow queries | Truncation, blob storage |
| Clock skew | Incorrect ordering | Server-trust timestamps |
| SDK crashes | Data loss | Batch persistence, retry |
| Out-of-order ingestion | Orphan events | Soft integrity, orphan queries |
| Invalid JSON | Ingest failures | Wrapper approach |

---

*End of Implementation Checklist*
