## ADDED Requirements

### Requirement: Stale Trace Signal

The system SHALL display a subtle, informational trust signal on the trace detail page when all of the following conditions are met:

1. The effective trace status is `RUNNING`
2. Total runtime (now minus `started_at`) is at least 15 minutes
3. Latest observed activity is at least 5 minutes old

Latest activity SHALL be derived from the following sources in priority order:

1. Maximum timeline event timestamp
2. Latest span `ended_at`
3. Latest span `started_at`
4. Trace `started_at`

The signal copy SHALL be subtle and non-authoritative, for example: "Still marked running. Recent activity is sparse, so this trace may be stale or incomplete."

The thresholds (15 minutes runtime, 5 minutes stale) SHALL be implemented as local constants for easy tuning.

This feature is explicitly marked as experimental and heuristic.

#### Scenario: Running trace exceeds both thresholds

- **WHEN** a trace has been running for 20 minutes and the last activity was 7 minutes ago
- **THEN** the stale trace signal is displayed

#### Scenario: Running trace below runtime threshold

- **WHEN** a trace has been running for 10 minutes (below 15-minute threshold)
- **THEN** no stale trace signal is displayed

#### Scenario: Running trace with recent activity

- **WHEN** a trace has been running for 20 minutes but the last activity was 2 minutes ago (below 5-minute threshold)
- **THEN** no stale trace signal is displayed

#### Scenario: Completed or failed trace

- **WHEN** a trace has status `COMPLETED` or `FAILED`
- **THEN** no stale trace signal is displayed regardless of runtime

### Requirement: Stale Signal Non-Intrusiveness

The stale trace signal SHALL be rendered as static informational text, not an alert or live region. It SHALL NOT interrupt the page or announce repeatedly during polling refreshes.

#### Scenario: Signal stability during polling

- **WHEN** polling refreshes the trace data and the signal conditions remain met
- **THEN** the signal text remains visible without re-announcing or flashing
