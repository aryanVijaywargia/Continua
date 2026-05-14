## ADDED Requirements

### Requirement: Separate pending-work query
The trace detail page SHALL issue a separate TanStack Query keyed as `['enginePendingWork', runId]` that calls `GET /v1/engine/runs/{run_id}/pending-work`. This query SHALL NOT piggyback on existing timeline or span fetches.

#### Scenario: Pending-work query is separate from timeline
- **WHEN** the trace detail page loads for an engine-backed trace with `trace.engine.run_id`
- **THEN** a separate pending-work query is issued independently of timeline and span queries

### Requirement: Pending-work polling interval
The pending-work query SHALL use `TIMELINE_POLL_INTERVAL_MS` as its `refetchInterval`. It SHALL NOT share, modify, or depend on the timeline polling interval or poll cursor.

#### Scenario: Pending-work polls at TIMELINE_POLL_INTERVAL_MS
- **WHEN** pending-work polling is active
- **THEN** it refetches every `TIMELINE_POLL_INTERVAL_MS` milliseconds (3000ms in production, 25ms in test)

### Requirement: Pending-work polling enablement
Pending-work polling SHALL be enabled only while `trace.engine.status` is `QUEUED`, `RUNNING`, `WAITING`, or `SUSPENDED`. It SHALL be disabled for terminal statuses (`COMPLETED`, `FAILED`, `CANCELLED`, `TERMINATED`, `CONTINUED_AS_NEW`). Existing raw timeline/span polling SHALL NOT be widened; those remain governed by the raw trace status `RUNNING`.

#### Scenario: Pending-work polling active for WAITING run
- **WHEN** the engine run status is `WAITING`
- **THEN** pending-work polling is enabled

#### Scenario: Pending-work polling disabled for COMPLETED run
- **WHEN** the engine run status is `COMPLETED`
- **THEN** pending-work polling is disabled

#### Scenario: Timeline polling not widened for SUSPENDED runs
- **WHEN** the engine run status is `SUSPENDED` and the raw trace status is not `RUNNING`
- **THEN** pending-work polling is enabled but raw timeline/span polling remains disabled

### Requirement: Pending-work detail rendering
The trace detail page SHALL render pending-work detail including: current wait description, activities list, timers list, and signals list. Empty lists SHALL show an appropriate empty-state message. Query errors or unavailable data SHALL show a degraded-state fallback.

#### Scenario: Pending-work displays activities and timers
- **WHEN** the pending-work response includes 2 activities and 1 timer
- **THEN** the activities section shows 2 items and the timers section shows 1 item

#### Scenario: Empty pending-work shows empty state
- **WHEN** the pending-work response has empty activities, timers, and signals arrays
- **THEN** each section shows an appropriate empty-state message

#### Scenario: Pending-work query error shows degraded state
- **WHEN** the pending-work query fails
- **THEN** the pending-work section shows a fallback message instead of empty lists
