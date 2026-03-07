## ADDED Requirements

### Requirement: Span Error Helper

The Python SDK SHALL provide a `span.error(message, payload)` method for emitting error events.

#### Scenario: Error event emitted
- **WHEN** `span.error("something went wrong", {"code": 500})` is called
- **THEN** an event is queued with `event_type="error"`, `level="error"`, the given message, and payload

#### Scenario: Error event includes span ID
- **WHEN** an error event is emitted
- **THEN** the event includes the current span's `trace_id` and `span_id`

#### Scenario: Error event batched
- **WHEN** an error event is emitted
- **THEN** it is added to the client's event batch for the next flush

### Requirement: Span Exception Helper

The Python SDK SHALL provide a `span.exception(exc, payload)` method for capturing exception details as events.

#### Scenario: Exception event emitted
- **WHEN** `span.exception(exc)` is called with a caught exception
- **THEN** an event is queued with `event_type="exception"`, `level="error"`, the exception message, and exception type/traceback in payload

#### Scenario: Exception event with extra payload
- **WHEN** `span.exception(exc, {"context": "during retry"})` is called
- **THEN** the extra payload is merged with the exception details

### Requirement: Span Metric Helper

The Python SDK SHALL provide a `span.metric(name, value, unit, payload)` method for emitting metric events.

#### Scenario: Metric event emitted
- **WHEN** `span.metric("latency_ms", 42.5, "ms")` is called
- **THEN** an event is queued with `event_type="metric"`, `level="info"`, and payload containing `{"metric_name": "latency_ms", "metric_value": 42.5, "metric_unit": "ms"}`

#### Scenario: Metric event without unit
- **WHEN** `span.metric("retry_count", 3)` is called
- **THEN** the event payload includes metric_name and metric_value without metric_unit

#### Scenario: Metric event with extra payload
- **WHEN** `span.metric("tokens", 150, payload={"model": "gpt-4"})` is called
- **THEN** the extra payload is merged with the metric fields

## MODIFIED Requirements

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

#### Scenario: Error event
- **WHEN** `span.error(message, payload)` is called
- **THEN** an error event is recorded on the span with `event_type="error"` and `level="error"`

#### Scenario: Exception capture
- **WHEN** `span.exception(exc, payload)` is called
- **THEN** an exception event is recorded on the span with exception details in payload

#### Scenario: Metric recording
- **WHEN** `span.metric(name, value, unit, payload)` is called
- **THEN** a metric event is recorded on the span with metric data in payload
