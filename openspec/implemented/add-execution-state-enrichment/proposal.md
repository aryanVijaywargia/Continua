# Change: Add Execution State Enrichment

## Why

The debugger's running-state panel currently shows a single classification summary (e.g. "declared wait", "waiting on model") but hides the full set of unresolved waits behind that summary. Operators debugging a running trace with multiple concurrent wait gates have no way to see which waits are still open, when each was entered, or how long each has been pending. Separately, there is no first-class milestone event type: teams that want to mark notable progress points in a long-running trace must use `custom` events, which lose semantic meaning in the timeline.

This phase adds two focused enrichments to the existing debugger surfaces:
1. Unresolved-wait rows inside the existing `RunningStatePanel`, derived from the already-implemented `computeOpenWaits()` logic
2. A new `snapshot_marker` event type that flows from contract through SDK to timeline UI, giving teams a dedicated milestone primitive

## What Changes

### 1. Contract: `snapshot_marker` event type
- Add `snapshot_marker` to `IngestEventType` and `TimelineEventType` enums in `contracts/openapi/openapi.yaml`
- Document payload conventions (`marker_kind`, `label`) without making them schema-required
- State explicitly that `snapshot_marker` is a debugger milestone event, not a checkpoint or resumability primitive

### 2. Backend: mapper passthrough
- Update `mapExplicitTimelineEventType` in `internal/api/mapper.go` to recognize `snapshot_marker` so it passes through as-is rather than degrading to `custom`
- No schema, migration, store, or ingest-warning changes

### 3. Python SDK: `snapshot_marker()` helper
- Add `span.snapshot_marker(label, *, marker_kind="milestone", payload=None, message=None)` in `sdks/python/src/continua/span.py`
- Reject empty `label` and empty `marker_kind`
- Default `message` to `label` when omitted
- Helper-owned payload keys written last to override conflicting caller keys

### 4. Frontend: `snapshot_marker` timeline rendering
- Add `getSnapshotMarkerDetails()` in `web/src/utils/eventSemantics.ts`
- Add `snapshot_marker` branch to `summarizeTimelineEvent()` in `web/src/utils/timeline.ts`
- Add dedicated collapsed-row preview in `Timeline.tsx` with `label` as primary text and `marker_kind` pill
- Include `snapshot_marker` in the `Semantic` filter mode
- Add `snapshot_marker` to the manual TypeScript union in `web/src/api/client.ts`
- Malformed markers (missing `marker_kind` or `label`) fall back to generic timeline rendering

### 5. Frontend: unresolved-wait rows in `RunningStatePanel`
- Derive `openWaits` once via `useMemo` from the existing `events` array in `TraceDetailPage.tsx`
- Pass `openWaits` into `RunningStatePanel`
- Render unresolved-wait rows beneath existing summary content, newest-first
- Each row shows: gate title (`Approval gate` for `human_approval`, `Wait gate` otherwise), wait kind, entered timestamp, open duration, span jump action, optional `wait_id`, optional message
- Resolved waits disappear on next poll cycle; anonymous waits without `wait_id` remain as standalone rows
- Open durations refresh only on existing poll cadence, no separate timer

## Impact
- Affected specs: `event-taxonomy` (modified), `timeline-ui` (modified), `event-conventions` (modified), `trace-running-state-display` (new), `python-sdk-events` (modified)
- Affected code:
  - `contracts/openapi/openapi.yaml` — enum extensions
  - `internal/api/mapper.go` — snapshot_marker case
  - `sdks/python/src/continua/span.py` — new helper
  - `web/src/api/client.ts` — union extension
  - `web/src/utils/eventSemantics.ts` — new extractor
  - `web/src/utils/timeline.ts` — new summary branch
  - `web/src/components/Timeline.tsx` — new preview component, filter update
  - `web/src/pages/TraceDetailPage.tsx` — openWaits derivation, RunningStatePanel enrichment
  - `docs/event-conventions.md` — snapshot_marker documentation
- No DB migrations, engine work, session-level changes, or compare-time logic
- Turn Lens is deferred entirely
