## ADDED Requirements

### Requirement: Per-Span Monotonic Sequence Counter

The Python SDK `_record_event()` method SHALL assign a monotonically increasing `sequence` integer to every explicit event emitted through the method. The counter SHALL be per-span, starting at 1, and shared across all explicit event types (`log`, `error`, `exception`, `metric`, `state_change`, `decision`, `effect`, `wait`, `message`, `custom`).

The `sequence` value SHALL be serialized as a top-level integer sibling of `trace_id`, `span_id`, `event_type`, `level`, `message`, and `payload` in the event dict passed to `client.add_event()`.

#### Scenario: Sequential events receive increasing sequence values
- **WHEN** a span emits three explicit events via `log()`, `metric()`, and `state_change()` in order
- **THEN** the events SHALL have `sequence` values 1, 2, and 3 respectively

#### Scenario: Sequence counter is per-span
- **WHEN** two spans each emit two events
- **THEN** each span's events SHALL have sequence values 1 and 2 independently

#### Scenario: All explicit event types share one counter
- **WHEN** a span emits a `log` event (sequence 1) then an `effect` event
- **THEN** the `effect` event SHALL have sequence 2, not sequence 1

---

### Requirement: Int32 Overflow Guard

The per-span sequence counter SHALL raise `OverflowError` if the next sequence value would exceed 2,147,483,647 (int32 maximum).

This is intentional fail-loud behavior. Silent wrap or saturation would corrupt ordering semantics and semantic ID derivation inputs.

#### Scenario: OverflowError at int32 boundary
- **WHEN** the per-span sequence counter is at 2,147,483,647 and another event is emitted
- **THEN** `_record_event()` SHALL raise `OverflowError`

#### Scenario: Normal operation below boundary
- **WHEN** the per-span sequence counter is at 2,147,483,646 and another event is emitted
- **THEN** the event SHALL receive sequence 2,147,483,647 without error

---

### Requirement: RFC 3339 UTC Timestamp on Explicit Events

The Python SDK `_record_event()` method SHALL include a top-level `event_ts` string on every explicit event. The timestamp SHALL be formatted as RFC 3339 UTC with `Z` suffix and microsecond precision (e.g., `2026-03-23T12:00:00.000000Z`).

The `event_ts` SHALL be captured inside `_event_lock` alongside the sequence increment to ensure monotonic timestamp-sequence pairs under concurrent emission.

#### Scenario: Event includes event_ts
- **WHEN** any explicit event is emitted via `_record_event()`
- **THEN** the event dict SHALL contain an `event_ts` key matching the pattern `\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{6}Z`

#### Scenario: Timestamp is UTC
- **WHEN** an event is emitted
- **THEN** `event_ts` SHALL end with `Z` (not `+00:00`)

---

### Requirement: Thread-Safe Sequence Assignment

The Python SDK SHALL use a per-span `_event_lock` (threading.Lock) to protect sequence counter increment, int32 bound enforcement, and timestamp capture. The lock SHALL be released before building the event dict and before calling `client.add_event()`.

#### Scenario: Concurrent event emission produces duplicate-free contiguous sequences
- **WHEN** N threads each emit one event on the same span concurrently
- **THEN** the N assigned sequence values SHALL form a duplicate-free contiguous set (e.g., {1, 2, ..., N})
- **AND** ordering correctness SHALL be determined by sorting on `sequence` and `event_ts` after collection, not by observation order in the queue or test harness

#### Scenario: Lock does not serialize enqueue
- **WHEN** multiple threads emit events concurrently on the same span
- **THEN** the events MAY arrive at the batch queue out of sequence order
- **AND** this is acceptable because the `sequence` field carries ordering truth independent of enqueue order

---

### Requirement: Early Returns Do Not Consume Sequence Numbers

The `_record_event()` method SHALL perform its existing early-return checks (`trace_id is None`, client not initialized) before acquiring `_event_lock`. Disabled tracing SHALL NOT increment the sequence counter or capture a timestamp.

#### Scenario: No trace_id does not consume sequence
- **WHEN** `_record_event()` is called on a span with `trace_id = None`
- **THEN** the sequence counter SHALL NOT be incremented
- **AND** no timestamp SHALL be captured

#### Scenario: No client does not consume sequence
- **WHEN** `_record_event()` is called and no client is initialized
- **THEN** the sequence counter SHALL NOT be incremented
