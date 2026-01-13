## MODIFIED Requirements

### Requirement: Trace Rollup Computation

The system SHALL compute and store aggregate metrics on traces after span ingestion. These metrics include total spans, total tokens, total cost, error count, and duration.

#### Scenario: Rollups computed after ingest
- **WHEN** batch containing spans is ingested
- **THEN** for each affected trace, rollups are computed
- **AND** trace record is updated with aggregated values

#### Scenario: Token aggregation
- **WHEN** trace has spans with `total_tokens` values
- **THEN** trace `total_tokens` equals sum of span values

#### Scenario: Cost aggregation
- **WHEN** trace has spans with `total_cost` values
- **THEN** trace `total_cost` equals sum of span values

#### Scenario: Span count
- **WHEN** trace has N spans
- **THEN** trace `total_spans` equals N

#### Scenario: Error counting
- **WHEN** trace has spans with status "error" or "failed"
- **THEN** trace `error_count` equals count of failed spans

#### Scenario: Duration computation
- **WHEN** trace has `start_time` and `end_time` set
- **THEN** trace `duration_ms` is computed from timestamps

#### Scenario: Rollup failure non-blocking
- **WHEN** rollup computation fails
- **THEN** warning is logged
- **AND** ingest transaction is NOT aborted
- **AND** trace data is still persisted
