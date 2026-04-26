## ADDED Requirements

### Requirement: Wait Event Parsing

The debugger SHALL parse `wait` timeline events into structured `WaitDetails` using a strict parser in `eventSemantics.ts`.

The parser SHALL require `event_type === 'wait'`, a non-empty `wait_kind` string, and a non-empty `phase` string. It SHALL accept optional `wait_id` and `resolution` fields. Malformed events SHALL return `null`.

The parser SHALL NOT validate `wait_kind` or `phase` vocabulary beyond non-empty string checks.

#### Scenario: Well-formed wait event with all fields
- **WHEN** a timeline event has `event_type: 'wait'` and payload contains `wait_kind: 'model_response'`, `phase: 'entered'`, `wait_id: 'w-1'`
- **THEN** `getWaitDetails()` returns `{ waitKind: 'model_response', phase: 'entered', waitId: 'w-1', resolution: undefined }`

#### Scenario: Minimal well-formed wait event
- **WHEN** a timeline event has `event_type: 'wait'` and payload contains `wait_kind: 'tool_call'`, `phase: 'resolved'` with no `wait_id`
- **THEN** `getWaitDetails()` returns `{ waitKind: 'tool_call', phase: 'resolved', waitId: undefined, resolution: undefined }`

#### Scenario: Malformed wait event missing required field
- **WHEN** a timeline event has `event_type: 'wait'` but payload is missing `wait_kind`
- **THEN** `getWaitDetails()` returns `null`

#### Scenario: Non-wait event type
- **WHEN** a timeline event has `event_type: 'effect'`
- **THEN** `getWaitDetails()` returns `null`

---

### Requirement: Wait Pairing Lifecycle

The classifier SHALL compute open waits entirely client-side from the full currently loaded event set using `wait_id`-only matching. The `computeOpenWaits()` utility SHALL sort events internally using the existing timeline comparator before pairing, so callers are not required to pass a pre-sorted list.

Only exact `phase === 'entered'` SHALL open a wait. Only exact `phase === 'resolved'` SHALL close a wait. Entered/resolved pairing SHALL use `wait_id` as the sole matching key. When multiple `entered` waits share the same `wait_id`, a single `resolved` wait SHALL close the earliest unmatched `entered` wait with that `wait_id` in timeline order (FIFO). A resolved wait with no matching earlier unmatched `entered` `wait_id` SHALL close nothing. Waits without `wait_id` SHALL never be fuzzy-matched or close any other wait. Non-`entered` / non-`resolved` phases SHALL NOT affect pairing. The decisive open wait SHALL be the latest unmatched entered wait in timeline order.

#### Scenario: Single entered wait remains open
- **WHEN** the event set contains one `entered` wait with `wait_id: 'w-1'` and no corresponding `resolved` wait
- **THEN** the wait pairing reports one open wait with `wait_id: 'w-1'`

#### Scenario: Entered and resolved pair with same wait_id
- **WHEN** the event set contains an `entered` wait with `wait_id: 'w-1'` followed by a `resolved` wait with `wait_id: 'w-1'`
- **THEN** the wait pairing reports zero open waits

#### Scenario: Resolved before entered self-heals across recomputation
- **WHEN** the event set initially contains only a `resolved` wait with `wait_id: 'w-1'`, and a later poll adds an `entered` wait with `wait_id: 'w-1'` that precedes the resolved event in timeline order
- **THEN** recomputation from the full event set reports zero open waits

#### Scenario: Waits without wait_id never close other waits
- **WHEN** the event set contains an `entered` wait with `wait_id: 'w-1'` and a `resolved` wait without `wait_id`
- **THEN** the entered wait with `wait_id: 'w-1'` remains open

#### Scenario: Duplicate wait_id FIFO pairing
- **WHEN** the event set contains two `entered` waits with `wait_id: 'w-1'` (at t1 and t2, t1 before t2) followed by one `resolved` wait with `wait_id: 'w-1'`
- **THEN** the resolved wait closes the t1 entered wait, and the t2 entered wait remains open as the decisive open wait

#### Scenario: Two resolves close two enters with same wait_id
- **WHEN** the event set contains two `entered` waits with `wait_id: 'w-1'` (at t1 and t2) followed by two `resolved` waits with `wait_id: 'w-1'` (at t3 and t4)
- **THEN** the t3 resolved closes the t1 entered, the t4 resolved closes the t2 entered, and zero open waits remain

#### Scenario: Unsorted input produces correct pairing
- **WHEN** events are passed to `computeOpenWaits()` in arbitrary order (e.g., resolved at t2 before entered at t1)
- **THEN** the utility sorts internally and produces the same pairing as if events were pre-sorted in timeline order

#### Scenario: Unsupported phase does not affect pairing
- **WHEN** a wait event has `phase: 'suspended'` with `wait_id: 'w-1'`
- **THEN** it neither opens nor closes any wait

---

### Requirement: Running Trace Classification

The classifier SHALL produce a `WaitStallAssessment` for running traces after the initial timeline snapshot is loaded. It SHALL return `null` when the trace status is not `RUNNING` or the initial snapshot has not loaded.

The classifier SHALL use `span.status === 'STARTED'` as the definition of an open span. `SCHEDULED` spans SHALL NOT count as active execution.

The classifier SHALL apply a fixed precedence of six classification levels.

"Execution evidence beyond trace start" means at least one explicit timeline event exists, or at least one span exists with status `STARTED`, `COMPLETED`, or `FAILED`. A trace whose only activity evidence is the trace start timestamp itself does not have execution evidence beyond trace start.

When multiple spans of the same kind qualify as "latest-started," the classifier SHALL use array order (i.e., the last matching span in the input array) as the tie-break. This is a determinism convention, not a semantic guarantee.

#### Scenario: Declared wait classification
- **WHEN** the trace is `RUNNING` and there is at least one open declared wait (entered, not resolved)
- **THEN** the assessment is `classification: 'declared_wait'`, `basis: 'declared'`, `reason: 'open_declared_wait'` with `decisiveEventId` set to the latest unmatched entered wait event

#### Scenario: Waiting on model classification
- **WHEN** the trace is `RUNNING`, there is no open declared wait, and at least one open `LLM` span exists
- **THEN** the assessment is `classification: 'waiting_on_model'`, `basis: 'inferred'`, `reason: 'open_model_span'` with `decisiveSpanId` and `decisiveSpanName` set to the latest-started open `LLM` span

#### Scenario: Waiting on tool classification
- **WHEN** the trace is `RUNNING`, there is no open declared wait, no open `LLM` span, and at least one open `TOOL` span exists
- **THEN** the assessment is `classification: 'waiting_on_tool'`, `basis: 'inferred'`, `reason: 'open_tool_span'` with `decisiveSpanId` and `decisiveSpanName` set to the latest-started open `TOOL` span

#### Scenario: Actively executing with open generic span
- **WHEN** the trace is `RUNNING`, there is no higher-priority state, and at least one open `AGENT`, `CHAIN`, or `CUSTOM` span exists
- **THEN** the assessment is `classification: 'actively_executing'`, `basis: 'heuristic'`, `reason: 'open_generic_span'` with `decisiveSpanId` and `decisiveSpanName` set to the latest-started open generic span

#### Scenario: Actively executing between spans with recent activity
- **WHEN** the trace is `RUNNING`, there are no open spans, but execution evidence beyond trace start exists and activity is recent
- **THEN** the assessment is `classification: 'actively_executing'`, `basis: 'heuristic'`, `reason: 'recent_activity_without_open_span'` with no `decisiveSpanId`

#### Scenario: Possibly stalled classification
- **WHEN** the trace is `RUNNING`, there is no higher-priority state, and the existing stale trace heuristic returns `shouldDisplay: true`
- **THEN** the assessment is `classification: 'possibly_stalled'`, `basis: 'heuristic'`, `reason: 'stale_without_stronger_signal'`

#### Scenario: Unknown classification for zero-evidence trace
- **WHEN** the trace is `RUNNING`, there are zero spans, zero explicit events, and the stale heuristic returns `shouldDisplay: false`
- **THEN** the assessment is `classification: 'unknown'`, `basis: 'heuristic'`, `reason: 'insufficient_running_evidence'`

#### Scenario: Scheduled-only trace is unknown
- **WHEN** the trace is `RUNNING` and all spans have `status: 'SCHEDULED'` with no explicit events
- **THEN** the assessment is `classification: 'unknown'`

#### Scenario: Open generic span beats stale heuristic
- **WHEN** the trace is `RUNNING`, an `AGENT` span is `STARTED`, and the stale heuristic returns `shouldDisplay: true`
- **THEN** the assessment is `classification: 'actively_executing'`, not `possibly_stalled`

#### Scenario: Declared wait beats model and tool spans
- **WHEN** the trace is `RUNNING`, there is an open declared wait, an open `LLM` span, and an open `TOOL` span
- **THEN** the assessment is `classification: 'declared_wait'`

#### Scenario: Model span beats tool span
- **WHEN** the trace is `RUNNING`, there is no open declared wait, and both an open `LLM` span and an open `TOOL` span exist
- **THEN** the assessment is `classification: 'waiting_on_model'`

#### Scenario: Non-running trace returns null
- **WHEN** the trace status is `COMPLETED`
- **THEN** the hook returns `null`

---

### Requirement: Poll-Stable Analysis Hook

The `useWaitStallAnalysis` hook SHALL recompute timing fields (`runtimeMs`, `inactivityMs`, `latestActivityAt`) and the stale heuristic on every poll cycle, since these depend on the current time. However, the hook SHALL produce a stable classification when the underlying evidence (span states, wait events, explicit events, and trace status) has not changed and the stale heuristic result has not flipped. The hook MAY use any caching or memoization strategy to achieve this.

#### Scenario: Classification stable across time-only advance
- **WHEN** a poll cycle passes with no new events or span state changes, and the stale heuristic result has not flipped
- **THEN** the hook returns the same `classification` and `reason`

#### Scenario: Time advance flips stale heuristic
- **WHEN** enough time passes that `evaluateStaleTraceSignal` flips from `shouldDisplay: false` to `shouldDisplay: true`, with no other evidence changes
- **THEN** the hook transitions to `possibly_stalled` (assuming no higher-priority state)

#### Scenario: New explicit event updates evidence and timing
- **WHEN** a new `log` event arrives during a poll cycle, providing execution evidence beyond trace start and refreshing `latestActivityAt`
- **THEN** the hook recomputes and may change classification (e.g., `unknown` to `actively_executing`)

#### Scenario: New open wait changes result
- **WHEN** a new `entered` wait event appears in a poll cycle
- **THEN** the hook recomputes and returns an updated assessment

---

### Requirement: Running-Trace Span Freshness

The trace detail page SHALL periodically refresh spans while the trace status is `RUNNING`, so that span-based classifications (`waiting_on_model`, `waiting_on_tool`, `actively_executing` via `open_generic_span`) reflect current execution state rather than the initial load snapshot.

The refresh SHALL piggyback on the existing timeline poll cadence. When the trace reaches a terminal status, the existing terminal-refresh invalidation already covers the final span state.

#### Scenario: New span visible during live execution
- **WHEN** a trace is `RUNNING` and a new `LLM` span with status `STARTED` is created server-side after the initial page load
- **THEN** the span becomes visible to the classifier within one poll cycle, and the classification updates accordingly

#### Scenario: Span completion visible during live execution
- **WHEN** a trace is `RUNNING` and an open `TOOL` span transitions to `COMPLETED` server-side
- **THEN** the span status change becomes visible to the classifier within one poll cycle

#### Scenario: No span refetch after terminal status
- **WHEN** the trace has reached `COMPLETED` or `FAILED`
- **THEN** span refetching stops (the existing terminal-refresh invalidation handles the final state)

---

### Requirement: Running-State Panel

The trace detail workspace SHALL display a running-state panel for running traces after the initial timeline snapshot is loaded. This panel SHALL replace the previous stale-only `StaleTraceSignalPanel`.

The panel SHALL display the classification label, basis label, advisory copy, latest activity timestamp, and runtime/inactivity durations when available. When `decisiveSpanId` exists, the panel SHALL offer a jump-to-span action. For `declared_wait`, the panel SHALL resolve a human-readable wait label from `decisiveEventId` and the current event list.

The panel SHALL NOT appear on non-running traces. The running-state classification SHALL NOT be surfaced on tree badges, waterfall badges, span detail sections, or list-page surfaces.

#### Scenario: Declared wait panel shows wait-specific label
- **WHEN** the assessment is `declared_wait` with `decisiveEventId` pointing to a wait event with `wait_kind: 'model_response'`
- **THEN** the panel displays "Declared wait" label, the copy "Execution declared a wait and has not yet recorded a matching resolution.", and the resolved wait kind "model_response"

#### Scenario: Model wait panel shows jump action
- **WHEN** the assessment is `waiting_on_model` with `decisiveSpanId: 'span-1'` and `decisiveSpanName: 'gpt-4o call'`
- **THEN** the panel displays "Waiting on model" label, the copy "Execution appears to be waiting on an in-flight model span.", and a jump action targeting span `span-1`

#### Scenario: Possibly stalled panel shows timing
- **WHEN** the assessment is `possibly_stalled` with `latestActivityAt`, `runtimeMs`, and `inactivityMs` populated
- **THEN** the panel displays "Possibly stalled" label, the copy "Execution is still marked running, but recent activity is sparse.", the latest activity timestamp, and duration information

#### Scenario: Panel not shown for completed trace
- **WHEN** the trace status is `COMPLETED`
- **THEN** no running-state panel is rendered

---

### Requirement: Timeline Wait Row Rendering

The timeline SHALL render improved summaries for `wait` event rows, whether via the summary helper or direct component rendering.

When `getWaitDetails()` succeeds, the timeline SHALL render `Entered wait: <kind>` or `Resolved wait: <kind>`, appending `→ <resolution>` when resolution exists. When strict parsing fails but the raw payload contains a non-empty `phase` string, the timeline SHALL render `<Capitalized phase> wait`. Otherwise the timeline SHALL fall back to existing generic summary/message behavior.

Phase-only waits SHALL NOT count as declared waits for classification purposes.

#### Scenario: Well-formed entered wait rendering
- **WHEN** a timeline event has `event_type: 'wait'` with `wait_kind: 'tool_call'`, `phase: 'entered'`
- **THEN** the timeline row summary reads "Entered wait: tool_call"

#### Scenario: Well-formed resolved wait with resolution
- **WHEN** a timeline event has `event_type: 'wait'` with `wait_kind: 'model_response'`, `phase: 'resolved'`, `resolution: 'success'`
- **THEN** the timeline row summary reads "Resolved wait: model_response → success"

#### Scenario: Phase-only fallback rendering
- **WHEN** a timeline event has `event_type: 'wait'` and strict parsing fails but `payload.phase` is `'paused'`
- **THEN** the timeline row summary reads "Paused wait"

#### Scenario: Generic fallback for fully malformed wait
- **WHEN** a timeline event has `event_type: 'wait'` and strict parsing fails and `payload.phase` is missing or empty
- **THEN** the timeline falls back to existing `message` or `event_type` summary behavior

#### Scenario: Non-wait rows unchanged
- **WHEN** a timeline event has `event_type: 'log'`
- **THEN** the timeline row rendering is unchanged
