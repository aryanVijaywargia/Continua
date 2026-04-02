> **Status: Historical**
> This document is preserved for historical context and does not define the current repo state. Use [docs/README.md](../README.md) and [DEBUGGER_PLATFORM_BASELINE.md](../DEBUGGER_PLATFORM_BASELINE.md) for current guidance.

# Spec 5: Sessions UI

## Summary

Added complete sessions UI to the web application, enabling users to view and navigate sessions with their associated traces.

## Changes Made

### 1. API Client Updates

**File**: `web/src/api/client.ts`

Added session types and API functions:

```typescript
export interface Session {
  id: string;
  name?: string;
  user_id?: string;
  trace_count?: number;
  metadata?: Record<string, unknown>;
  created_at: string;
}

export interface SessionList {
  sessions: Session[];
  total: number;
}

export async function fetchSessions(limit = 20, offset = 0): Promise<SessionList>;
export async function fetchSession(id: string): Promise<Session>;
export async function fetchTracesBySession(sessionId: string, limit = 20, offset = 0): Promise<TraceList>;
```

### 2. Sessions List Page

**File**: `web/src/pages/SessionsPage.tsx`

Features:
- Paginated table of sessions
- Columns: Session ID (truncated), Name, User ID, Trace Count, Created
- Click row to navigate to session detail
- API key prompt if not configured
- Previous/Next pagination controls

### 3. Session Detail Page

**File**: `web/src/pages/SessionDetailPage.tsx`

Features:
- Session metadata header (ID, User ID, Trace Count, Created)
- Paginated list of traces belonging to this session
- Back link to sessions list
- Click trace to navigate to trace detail

### 4. Navigation Component

**File**: `web/src/components/Navigation.tsx`

Features:
- Persistent navigation bar on all pages (except home)
- Links to Traces and Sessions
- Active state highlighting

### 5. App Router Updates

**File**: `web/src/App.tsx`

- Added `/sessions` and `/sessions/:id` routes
- Wrapped pages with `PageWithNav` for consistent navigation
- Updated home page with links to both Traces and Sessions

### 6. Backend: Session Queries with Trace Count

**File**: `db/platform/queries/sessions.sql`

Added new queries:
```sql
-- name: GetSessionWithTraceCount :one
SELECT s.*,
    (SELECT COUNT(*) FROM traces t WHERE t.session_id = s.id AND t.project_id = s.project_id) as trace_count
FROM sessions s
WHERE s.id = $1;

-- name: ListSessionsWithTraceCount :many
SELECT s.*,
    (SELECT COUNT(*) FROM traces t WHERE t.session_id = s.id AND t.project_id = s.project_id) as trace_count
FROM sessions s
WHERE s.project_id = $1
ORDER BY s.created_at DESC
LIMIT $2 OFFSET $3;
```

### 7. Store Updates

**File**: `internal/store/sessions.go`

Added:
- `SessionWithCount` struct embedding `platform.Session` with `TraceCount`
- `GetSessionWithTraceCount(ctx, id)` method
- `ListSessionsWithTraceCount(ctx, projectID, limit, offset)` method

### 8. API Handler Updates

**File**: `internal/api/server.go`

- `ListSessions` now uses `ListSessionsWithTraceCount`
- `GetSession` now uses `GetSessionWithTraceCount`

### 9. Mapper Updates

**File**: `internal/api/mapper.go`

Added:
```go
func sessionWithCountToAPI(s *platform.Session, traceCount int64) Session {
    session := sessionToAPI(s)
    tc := int(traceCount)
    session.TraceCount = &tc
    return session
}
```

## Page Structure

```
/                    → Home (links to Traces and Sessions)
/traces              → TracesPage (paginated traces list)
/traces/:id          → TraceDetailPage (trace with spans)
/sessions            → SessionsPage (paginated sessions list)
/sessions/:id        → SessionDetailPage (session with traces)
```

## Verification

```bash
# Build backend
go build ./...

# Type check frontend
pnpm --filter web type-check

# Start dev servers
make dev-server    # Backend on :8080
make dev-web       # Frontend on :5173

# Navigate to http://localhost:5173/sessions
```

## Requirements Met

Per spec:
- [x] Sessions page loads with table (session_id, user_id, trace_count, created_at, name)
- [x] Sessions pagination with Previous/Next
- [x] Session row click navigates to /sessions/:id
- [x] Session detail shows metadata in header
- [x] Session detail lists related traces with pagination
- [x] Trace click navigates to /traces/:id
- [x] Back link to /sessions available
- [x] Sessions link in navigation on all pages
- [x] trace_count computed per session
- [x] Session with zero traces appears with trace_count=0

---

=== SPEC 5 COMPLETE: READY FOR REVIEW ===
