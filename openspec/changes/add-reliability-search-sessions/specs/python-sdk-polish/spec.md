## MODIFIED Requirements

### Requirement: Error Handling

The Python SDK SHALL raise specific exceptions for different error conditions.

#### Scenario: Authentication error
- **WHEN** the API returns 401 Unauthorized
- **THEN** `AuthenticationError` is raised
- **AND** the error message indicates invalid API key

#### Scenario: Rate limit error
- **WHEN** the API returns 429 Too Many Requests
- **THEN** `RateLimitError` is raised
- **AND** the error includes retry-after information if available

#### Scenario: Validation error
- **WHEN** the API returns 400 Bad Request
- **THEN** `ValidationError` is raised
- **AND** the error message includes validation details

#### Scenario: Network error
- **WHEN** the network request fails after retries
- **THEN** `NetworkError` is raised
- **AND** the error includes the number of retry attempts

### Requirement: Retry with Backoff

The Python SDK SHALL retry transient network errors with exponential backoff.

#### Scenario: Retry on connection error
- **WHEN** a connection error occurs
- **THEN** the request is retried up to 3 times
- **AND** each retry waits (2^attempt + random jitter) seconds

#### Scenario: Retry on timeout
- **WHEN** a request timeout occurs
- **THEN** the request is retried up to 3 times
- **AND** each retry waits (2^attempt + random jitter) seconds

#### Scenario: No retry on authentication error
- **WHEN** a 401 error occurs
- **THEN** no retry is attempted
- **AND** `AuthenticationError` is raised immediately

#### Scenario: Retry exhaustion
- **WHEN** all retry attempts fail
- **THEN** `NetworkError` is raised
- **AND** the original error is preserved as cause

### Requirement: Session Context Manager

The Python SDK SHALL provide a context manager for session scoping.

#### Scenario: Session context sets session_id
- **WHEN** code runs inside `with continua.session("sess_123"):`
- **THEN** all traces created inherit session_id="sess_123"

#### Scenario: Session context generates ID
- **WHEN** code runs inside `with continua.session():` (no ID)
- **THEN** a UUID is generated for the session
- **AND** all traces created inherit that session_id

#### Scenario: Session context cleanup
- **WHEN** the session context exits
- **THEN** the session_id is cleared from context
- **AND** subsequent traces do not inherit the session

### Requirement: Span Helper Methods

The Python SDK SHALL provide helper methods for common span operations.

#### Scenario: Set LLM response
- **WHEN** `span.set_llm_response(model, messages, response, tokens_in, tokens_out)` is called
- **THEN** the span is updated with model, input (messages), output (response), and token counts

#### Scenario: Set tool call
- **WHEN** `span.set_tool_call(tool_name, arguments, result)` is called
- **THEN** the span is updated with tool name, input (arguments), and output (result)

#### Scenario: Log message
- **WHEN** `span.log(message, level, payload)` is called
- **THEN** an event is recorded on the span with the message, level, and optional payload
