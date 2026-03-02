## ADDED Requirements

### Requirement: Batch Idempotency Semantics

The system SHALL treat batch_key as the idempotency key with a clear state transition for ingest_batches.

#### Scenario: Duplicate batch key returns duplicate
- **WHEN** a batch with an existing batch_key is submitted
- **THEN** the ingest response returns status "duplicate"
- **AND** no new traces, spans, or events are inserted

#### Scenario: Failed ingest does not mark accepted
- **WHEN** ingest fails before commit
- **THEN** the batch is not marked "accepted"
- **AND** a subsequent retry with the same batch_key is processed normally

## MODIFIED Requirements

### Requirement: Span Upsert Time Handling

The system SHALL preserve correct timestamps when span updates arrive out of order.

#### Scenario: End time arrives before start time
- **WHEN** a span update with `end_time=T2` is ingested
- **AND** a subsequent update with `start_time=T1` is ingested (where T1 < T2)
- **THEN** the span record has `start_time=T1` and `end_time=T2`
- **AND** both timestamps are preserved

#### Scenario: Start time arrives before end time
- **WHEN** a span update with `start_time=T1` is ingested
- **AND** a subsequent update with `end_time=T2` is ingested (where T2 > T1)
- **THEN** the span record has `start_time=T1` and `end_time=T2`
- **AND** both timestamps are preserved

#### Scenario: Earlier start time replaces later
- **WHEN** a span has `start_time=T2`
- **AND** an update with `start_time=T1` arrives (where T1 < T2)
- **THEN** the span record has `start_time=T1`
- **AND** the earlier timestamp is preserved

#### Scenario: Later end time replaces earlier
- **WHEN** a span has `end_time=T1`
- **AND** an update with `end_time=T2` arrives (where T2 > T1)
- **THEN** the span record has `end_time=T2`
- **AND** the later timestamp is preserved

#### Scenario: NULL timestamps handled correctly
- **WHEN** a span update has `start_time=NULL`
- **AND** the existing span has `start_time=T1`
- **THEN** the span record retains `start_time=T1`
- **AND** the NULL does not overwrite the existing value
