## 1. Contract and Code Generation

- [x] 1.1 Add `snapshot_marker` to `IngestEventType` enum in `contracts/openapi/openapi.yaml`
- [x] 1.2 Add `snapshot_marker` to `TimelineEventType` enum in `contracts/openapi/openapi.yaml`
- [x] 1.3 Add payload convention descriptions for `snapshot_marker` (`marker_kind`, `label`) in OpenAPI schema
- [x] 1.4 Run `make generate` and verify generated Go, TypeScript, and Python artifacts include `snapshot_marker`

## 2. Backend Mapper

- [x] 2.1 Add `snapshot_marker` case to `mapExplicitTimelineEventType` in `internal/api/mapper.go`
- [x] 2.2 Add mapper unit test proving `snapshot_marker` maps through as `snapshot_marker`
- [x] 2.3 Add API-level ingest-to-timeline round-trip test for `snapshot_marker`

## 3. Python SDK Helper

- [x] 3.1 Implement `snapshot_marker()` method on `Span` class in `sdks/python/src/continua/span.py`
- [x] 3.2 Add docstring with parameter descriptions and usage example
- [x] 3.3 Add unit tests for validation (empty label, empty marker_kind)
- [x] 3.4 Add unit tests for payload/message default behavior and key override
- [x] 3.5 Add integration round-trip test for emitted `snapshot_marker`

## 4. Frontend: Snapshot Marker Semantics

- [x] 4.1 Add `snapshot_marker` to the TypeScript event type union in `web/src/api/client.ts`
- [x] 4.2 Add `getSnapshotMarkerDetails()` in `web/src/utils/eventSemantics.ts`
- [x] 4.3 Add `snapshot_marker` branch to `summarizeTimelineEvent()` in `web/src/utils/timeline.ts`
- [x] 4.4 Add unit tests for `getSnapshotMarkerDetails()` (well-formed, malformed, wrong type)
- [x] 4.5 Add unit tests for `summarizeTimelineEvent()` snapshot_marker summaries

## 5. Frontend: Snapshot Marker Timeline Rendering

- [x] 5.1 Add `SnapshotMarkerPreview` component in `Timeline.tsx` with label text and marker_kind pill
- [x] 5.2 Wire `SnapshotMarkerPreview` into `TimelineRow` rendering chain with malformed fallback
- [x] 5.3 Add `snapshot_marker` to `SEMANTIC_EVENT_TYPES` array in `Timeline.tsx`
- [x] 5.4 Add rendering tests for marker preview and semantic-filter inclusion
- [x] 5.5 Add component test proving `Timeline.tsx` falls back to generic row when `getSnapshotMarkerDetails()` returns `null` (malformed marker)

## 6. Frontend: Unresolved Wait Rows

- [x] 6.1 Derive `openWaits` via `useMemo` + `computeOpenWaits()` in `TraceDetailPage.tsx`
- [x] 6.2 Pass `openWaits` into `RunningStatePanel` component
- [x] 6.3 Render unresolved-wait rows beneath existing summary content (newest-first); existing summary (classification label, basis, explanatory copy, timing, current-wait, jump action) stays intact — this is additive only
- [x] 6.4 Implement gate title logic (`Approval gate` for `human_approval`, `Wait gate` otherwise)
- [x] 6.5 Show wait kind, entered timestamp, open duration, span jump action, optional wait_id, optional message per row
- [x] 6.6 Add page tests: single wait, multiple waits newest-first, human_approval title, resolved wait disappears on poll, anonymous wait standalone, duration refresh on poll only

## 7. Documentation

- [x] 7.1 Add `snapshot_marker` entry to `docs/event-conventions.md` with payload conventions and SDK example
- [x] 7.2 Add scope guard clarifying markers are debugger milestones, not checkpoint/resume primitives

## 8. Verification

- [x] 8.1 Run `go test ./internal/api/...`
- [x] 8.2 Run `pnpm --filter web test`
- [x] 8.3 Run `cd sdks/python && uv run pytest`
- [x] 8.4 Run `openspec validate add-execution-state-enrichment --strict`
