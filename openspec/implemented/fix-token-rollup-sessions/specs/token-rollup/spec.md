## ADDED Requirements

### Requirement: Split Token Tracking on Traces

The system SHALL store input and output token counts separately on trace records as `total_tokens_in` and `total_tokens_out`, computed from span-level `prompt_tokens` and `completion_tokens` respectively.

#### Scenario: Rollup computes split tokens from spans

- **WHEN** a trace rollup job runs for a trace with spans containing `prompt_tokens` and `completion_tokens`
- **THEN** the trace's `total_tokens_in` is set to `SUM(prompt_tokens)` from all spans
- **AND** the trace's `total_tokens_out` is set to `SUM(completion_tokens)` from all spans

#### Scenario: API returns split token counts directly

- **WHEN** a client requests a trace via the API
- **THEN** the response includes `total_tokens_in` and `total_tokens_out` mapped directly from database columns
- **AND** no halving or synthetic splitting logic is applied

#### Scenario: Migration backfills existing traces

- **WHEN** migration 000008 runs on a database with existing traces
- **THEN** all traces are backfilled with `total_tokens_in = SUM(prompt_tokens)` and `total_tokens_out = SUM(completion_tokens)` from their spans
- **AND** the old `total_tokens` column is dropped

#### Scenario: Traces with no spans have zero tokens

- **WHEN** a trace has no spans
- **THEN** `total_tokens_in` and `total_tokens_out` are both 0

### Requirement: Directional Token Fields Are the Supported Ingest Contract

The system SHALL treat `prompt_tokens` and `completion_tokens` as the supported token inputs for rollup computation. A span payload that provides only `total_tokens` is unsupported.

#### Scenario: total_tokens-only payload is rejected

- **WHEN** a client submits a span with `total_tokens` but without directional token fields
- **THEN** ingest returns a validation error for unsupported token format
- **AND** no rollup values are updated from that payload

#### Scenario: directional fields are accepted for rollup

- **WHEN** a client submits spans with `prompt_tokens` and/or `completion_tokens`
- **THEN** ingest accepts the payload
- **AND** trace rollups are computed from these directional fields only
