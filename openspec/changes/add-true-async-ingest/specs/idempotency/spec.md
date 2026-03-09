## ADDED Requirements

### Requirement: Batch Idempotency Semantics

The system SHALL treat `batch_key` as the project-scoped idempotency key with semantics that distinguish pre-acceptance failures from durably accepted async batches.

#### Scenario: Duplicate batch key returns duplicate
- **WHEN** a batch with an existing `batch_key` is submitted for the same project
- **THEN** the ingest response returns status `duplicate`
- **AND** no new traces, spans, or events are inserted

#### Scenario: Acceptance failure before commit is retryable
- **WHEN** sync processing fails before commit, or true-async acceptance fails before the acceptance transaction commits
- **THEN** the batch key is not reserved
- **AND** a subsequent retry with the same `batch_key` is processed normally

#### Scenario: Durably accepted async batch reserves the key
- **WHEN** a true-async request returns `202` with a `batch_id`
- **THEN** the `(project_id, batch_key)` pair remains reserved through `queued`, `processing`, `completed`, and `failed`
- **AND** later submissions with the same key return `duplicate` and the existing `batch_id`

#### Scenario: Cleanup does not release the key
- **WHEN** payload cleanup runs after batch completion or failure
- **THEN** the idempotency record remains
- **AND** the same `batch_key` cannot be reused
