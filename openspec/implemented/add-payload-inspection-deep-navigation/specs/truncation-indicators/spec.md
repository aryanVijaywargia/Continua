# Truncation Indicators

Banners for truncated span payloads using existing API metadata fields.

## ADDED Requirements

### Requirement: Truncation Banner for Span Payloads

The system MUST render a truncation banner above span input and output PayloadInspector instances when the span's truncation metadata indicates the payload was truncated.

#### Scenario: Full truncation metadata

Given a span with `input_truncated: true`, `input_original_size_bytes: 524288`, and `input_truncation_reason: "size_limit"`
When the span input PayloadInspector renders
Then a banner is shown indicating truncation, the original size (formatted), and the reason

#### Scenario: Partial truncation metadata

Given a span with `output_truncated: true` and `output_original_size_bytes: 1048576` but no `output_truncation_reason`
When the span output PayloadInspector renders
Then a banner is shown indicating truncation and the original size, without a reason

#### Scenario: Truncated flag only

Given a span with `input_truncated: true` but no `input_original_size_bytes` and no `input_truncation_reason`
When the span input PayloadInspector renders
Then a banner is shown indicating the payload was truncated

#### Scenario: Not truncated

Given a span with `input_truncated: false` (or field absent)
When the span input PayloadInspector renders
Then no truncation banner is shown

#### Scenario: Trace-level payloads excluded

Given the trace context section with trace input and output
When the trace-level PayloadInspectors render
Then no truncation banners are shown (the API does not expose truncation metadata for trace-level payloads)

#### Scenario: Timeline payloads excluded

Given a timeline event with an expanded payload
When the timeline PayloadInspector renders
Then no truncation banner is shown

### Requirement: Truncation Formatting

The truncation banner MUST format metadata for readability.

#### Scenario: Size formatting

Given original size of 1048576 bytes
When the truncation banner renders
Then the size is displayed as "1.0 MB" (human-readable)

#### Scenario: Small size formatting

Given original size of 2048 bytes
When the truncation banner renders
Then the size is displayed as "2.0 KB"
