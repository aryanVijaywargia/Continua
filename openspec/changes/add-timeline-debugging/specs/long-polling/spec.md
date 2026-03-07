## ADDED Requirements

### Requirement: Incremental Timeline Polling

The timeline UI SHALL poll for new events while a trace is running, using cursor-based incremental fetches.

#### Scenario: Initial full load
- **WHEN** the timeline view is first opened
- **THEN** the full first page of events is fetched without a cursor

#### Scenario: Polling while running
- **WHEN** the trace status is `RUNNING`
- **THEN** the UI polls the timeline endpoint at a regular interval using the `poll_cursor` from the last response as the next `after` cursor

#### Scenario: Full refresh on terminal state
- **WHEN** the response `trace_status` becomes `COMPLETED` or `FAILED`
- **THEN** the client performs one final cursorless fetch to get the complete timeline
- **AND** polling stops after the full refresh completes

#### Scenario: No duplicate events and correct display order
- **WHEN** incremental polling returns new events
- **THEN** new events are merge-sorted into the existing timeline by display timestamp, deduplicating by event ID, so the displayed list remains in strict chronological order

### Requirement: Live Polling Indicator

The timeline UI SHALL show a visual indicator while live polling is active.

#### Scenario: Running trace indicator
- **WHEN** the trace is `RUNNING` and polling is active
- **THEN** a live/updating indicator is visible in the timeline view

#### Scenario: Terminal state indicator
- **WHEN** polling stops due to terminal trace status
- **THEN** the indicator updates to show the final status (completed or failed)
