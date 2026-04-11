## ADDED Requirements

### Requirement: ContinueAsNew Sentinel
The engine workflow package SHALL expose `ContinueAsNew(input any) error` that returns a sentinel error recognized by the replay engine as a continuation request.

The sentinel SHALL be detectable via `errors.Is(err, ErrContinueAsNew)` and the continuation input SHALL be extractable from the error.

Returning the sentinel from the workflow `Run` function, or returning an error that wraps it, triggers continuation. Swallowing the sentinel, or otherwise failing to return it, is a programming error that produces a replay mismatch.

#### Scenario: ContinueAsNew returned from workflow
- **WHEN** a workflow returns `workflow.ContinueAsNew(newInput)`
- **THEN** the replay engine detects the sentinel and produces a `decisionContinuedAsNew`
- **AND** the continuation input is available in the decision

#### Scenario: ContinueAsNew sentinel detection
- **WHEN** an error wraps the `ErrContinueAsNew` sentinel
- **THEN** `errors.Is(err, workflow.ErrContinueAsNew)` returns `true`

### Requirement: ContinueAsNew Replay Handling
On replay, if the history contains a `workflow.continued_as_new` event, the replay engine SHALL verify that the workflow returns the `ContinueAsNew` sentinel with matching input.

Matching input SHALL use the same JSON-equivalence rule as existing replay payload comparisons: trimmed raw bytes match first; otherwise both payloads are decoded as JSON and compared for semantic equality.

A mismatch (workflow completes normally or fails instead of continuing) SHALL produce a `workflow.replay_mismatch` event.

#### Scenario: Replay matches continuation
- **WHEN** history contains `workflow.continued_as_new` with input X
- **AND** the workflow returns `ContinueAsNew(X)` on replay
- **THEN** the replay cursor advances past the event and the decision is `continuedAsNew`

#### Scenario: Replay matches semantically equivalent JSON
- **WHEN** history contains `workflow.continued_as_new` with JSON input `{"a":1,"b":2}`
- **AND** the workflow returns `ContinueAsNew({"b":2,"a":1})` on replay
- **THEN** the replay treats the continuation input as matching

#### Scenario: Replay mismatch on continuation
- **WHEN** history contains `workflow.continued_as_new`
- **AND** the workflow returns `nil` (completes) on replay
- **THEN** a `workflow.replay_mismatch` event is produced
