## ADDED Requirements

### Requirement: Continuation Following in wait_for_terminal
The `EngineControlClient.wait_for_terminal` method SHALL accept `follow_continuations` (default `False`) and `max_continuations` (default `32`) parameters.

When `follow_continuations=True` and the result status is `CONTINUED_AS_NEW`, the SDK SHALL extract `continued_to_run_id` from the result and poll the next run, repeating until a non-continuation terminal status is observed or the depth limit is reached.

When `follow_continuations=False` (default), the method SHALL return the result of the specific run regardless of status, preserving backward compatibility.

#### Scenario: Follow single continuation
- **WHEN** a client calls `wait_for_terminal(run_id, follow_continuations=True)`
- **AND** the run terminates with `CONTINUED_AS_NEW` pointing to run2
- **AND** run2 terminates with `completed`
- **THEN** the method returns run2's result

#### Scenario: No follow by default
- **WHEN** a client calls `wait_for_terminal(run_id)` (default follow_continuations=False)
- **AND** the run terminates with `CONTINUED_AS_NEW`
- **THEN** the method returns the CONTINUED_AS_NEW result directly

#### Scenario: Multi-hop continuation
- **WHEN** a client calls `wait_for_terminal(run_id, follow_continuations=True)`
- **AND** the chain is run1 → run2 → run3 (completed)
- **THEN** the method follows the chain and returns run3's result

### Requirement: Continuation Depth Guard
When `follow_continuations=True` and the continuation chain exceeds `max_continuations`, the SDK SHALL raise `EngineRunContinuationDepthError`.

#### Scenario: Depth limit exceeded
- **WHEN** a client calls `wait_for_terminal(run_id, follow_continuations=True, max_continuations=1)`
- **AND** the chain has 2 or more continuations
- **THEN** `EngineRunContinuationDepthError` is raised

### Requirement: CONTINUED_AS_NEW Status Enum
The `EngineRunStatus` enum in the Python SDK types SHALL include `CONTINUED_AS_NEW`.

#### Scenario: Status enum includes continued_as_new
- **WHEN** a client receives a run response with status `CONTINUED_AS_NEW`
- **THEN** the SDK decodes it as `EngineRunStatus.CONTINUED_AS_NEW`

### Requirement: Continuation Chain Fields in Response Types
The `EngineRunResultResponse` type SHALL include `continued_from_run_id`, `continued_to_run_id`, `continued_from_trace_id`, `continued_to_trace_id` as optional string fields.

#### Scenario: Response includes continuation fields
- **WHEN** a client calls `get_result(run_id)` on a continued run
- **THEN** the response object has `continued_to_run_id` and `continued_to_trace_id` populated

### Requirement: EngineRunContinuationDepthError Exception
The Python SDK SHALL expose `EngineRunContinuationDepthError` for cases where the continuation chain exceeds the depth limit during `wait_for_terminal`.

#### Scenario: Depth error raised
- **WHEN** continuation following exceeds `max_continuations`
- **THEN** `EngineRunContinuationDepthError` is raised with the run_id and depth limit
