## ADDED Requirements

### Requirement: Span LLM Metadata Fields

The `Span` API schema SHALL include optional `model` and `provider` fields for LLM spans.

#### Scenario: Model mapped from database
- **WHEN** a span has `Model = "gpt-4o"` in the database
- **THEN** `model` in the API response is `"gpt-4o"`

#### Scenario: Provider mapped from database
- **WHEN** a span has `Provider = "openai"` in the database
- **THEN** `provider` in the API response is `"openai"`

#### Scenario: Model and provider absent for non-LLM spans
- **WHEN** a span has nil `Model` and nil `Provider` in the database
- **THEN** `model` and `provider` are absent from the API response

### Requirement: Span Truncation Metadata Fields

The `Span` API schema SHALL include truncation fields to indicate payload truncation status. The boolean flags (`input_truncated`, `output_truncated`) SHALL always be present when the ingest processor has written them, including `false`. Size and reason fields are optional and present only when truncation occurred.

#### Scenario: Input truncation fields mapped when truncated
- **WHEN** a span has `InputTruncated = true`, `InputOriginalSizeBytes = 524288`, `InputTruncationReason = "size_limit"` in the database
- **THEN** the API response includes `input_truncated: true`, `input_original_size_bytes: 524288`, `input_truncation_reason: "size_limit"`

#### Scenario: Output truncation fields mapped when truncated
- **WHEN** a span has `OutputTruncated = true`, `OutputOriginalSizeBytes = 1048576`, `OutputTruncationReason = "size_limit"` in the database
- **THEN** the API response includes `output_truncated: true`, `output_original_size_bytes: 1048576`, `output_truncation_reason: "size_limit"`

#### Scenario: Truncation booleans present when not truncated
- **WHEN** a span has `InputTruncated = false` and `OutputTruncated = false` in the database
- **THEN** `input_truncated: false` and `output_truncated: false` are present in the API response
- **AND** `input_original_size_bytes`, `input_truncation_reason`, `output_original_size_bytes`, `output_truncation_reason` are absent

#### Scenario: Truncation fields absent for legacy rows
- **WHEN** a span has nil `InputTruncated` and nil `OutputTruncated` in the database (legacy row)
- **THEN** all six truncation fields are absent from the API response

#### Scenario: Truncation fields in spans-by-trace response
- **WHEN** `GET /api/traces/{id}/spans` is called
- **THEN** each span includes truncation fields according to the rules above
