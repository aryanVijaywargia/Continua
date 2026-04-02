> **Status: Historical**
> This document is preserved for historical context and does not define the current repo state. Use [docs/README.md](../README.md) and [DEBUGGER_PLATFORM_BASELINE.md](../DEBUGGER_PLATFORM_BASELINE.md) for current guidance.

# Continua Phase 4 Design Doc

## Document Metadata

- **Version**: 1.0
- **Date**: March 2026
- **Status**: Implemented
- **Purpose**: Define what Phase 4 will build, starting from the verified implementation state at the end of Phase 3

---

## 1. Executive Summary

Phase 4 focuses on making Continua meaningfully better for trace debugging and day-to-day product usability.

The main product outcome for this phase is:

> A user should be able to open any trace, understand what happened in order, and see new events appear for active traces without needing full realtime infrastructure.

Phase 4 is intentionally **not** the phase for major ingest pipeline redesign. We are explicitly deferring true async ingest, WebSockets, SSE, and an explicit session creation API. Instead, this phase will concentrate on:

1. A first-class **events timeline**
2. **Long polling** for active traces
3. **Python SDK** improvements
4. Contract and product **alignment/hardening**

---

## 2. Verified Baseline Entering Phase 4

The following reflects the verified implementation state after Phase 3.

### 2.1 What Exists Today

#### Backend and Data

- Go backend with Fx-based module wiring
- API key authentication
- Trace ingest pipeline with validation and idempotency
- Trace, span, and event persistence in PostgreSQL
- Async rollup jobs using River for analytics aggregation
- Search and filtering for traces
- Sessions list/detail APIs and UI

#### Product Surface

- Traces list and trace detail
- Span tree/detail inspection
- Sessions list/detail flows
- Read-only trace/session exploration

#### SDKs

- Python SDK is the primary production-ready SDK
- TypeScript SDK exists only as scaffold and is not feature-complete

### 2.2 Important Current Constraints

#### Ingest Semantics

- `sync=false` currently returns `202 Accepted`
- However, ingest processing still happens inline before the request finishes
- River is currently used for **rollups**, not for ingest execution

#### Events and Timeline State

- Event records already exist in storage
- Timeline is not yet exposed as a complete first-class product/API surface
- Trace detail is currently more span-centric than event-centric

#### Realtime State

- WebSocket contracts/placeholders exist in parts of the codebase
- There is no complete backend realtime pipeline
- No shipping live-update product behavior exists yet

---

## 3. Phase 4 Product Direction

### 3.1 Core Principle

Phase 4 is a **debuggability and polish** phase, not an infrastructure ambition phase.

That means the phase should optimize for:

1. Clarity of trace inspection
2. Useful product experience
3. Strong Python SDK ergonomics
4. Lower implementation risk

### 3.2 Decisions Locked for Phase 4

The following decisions were made during planning and should be treated as fixed scope unless requirements change materially.

#### Included

1. Events timeline as the primary new feature
2. Long polling as the only live-update mechanism
3. Python SDK priority over TypeScript SDK

#### Explicitly Deferred

1. True async ingest
2. TypeScript SDK parity
3. Larger ingest pipeline redesign

#### Removed from Roadmap

1. `POST /api/sessions`
2. WebSockets
3. SSE

Rationale:

- Sessions are already auto-created from ingest using external session IDs, which matches the current conversation-style product model.
- Long polling is sufficient for "live enough" trace inspection without connection-management complexity.
- WebSocket/SSE infrastructure would add more moving parts than value at the current stage.

---

## 4. Phase 4 Goals

### 4.1 Primary Goals

1. Make trace debugging substantially better through an event timeline
2. Allow active traces to update without manual refresh
3. Improve Python SDK usability and reliability
4. Reduce drift between docs, API contracts, backend behavior, and UI types

### 4.2 Non-Goals

1. Re-architect ingest into true background processing
2. Build a bidirectional realtime system
3. Add manual session lifecycle APIs
4. Bring TypeScript SDK to parity

---

## 5. Scope of Work

## 5.1 Workstream A: Events Timeline API

### Goal

Expose trace events as a first-class, ordered debugging surface.

### Planned Deliverables

1. A dedicated endpoint for trace events
2. Stable response schema for event timeline consumption
3. Support for incremental fetches to power long polling

### Recommended API Shape

```http
GET /api/traces/{id}/events
```

Recommended query parameters:

- `limit`: maximum number of events to return
- `after`: fetch only events after a known cursor or timestamp

Recommended response properties:

- event identifier
- trace identifier
- optional span identifier
- event type
- timestamp
- level or severity if applicable
- payload/details
- ordering cursor

### Behavioral Expectations

1. Events must be returned in deterministic chronological order
2. Incremental fetches must not duplicate or skip events
3. Error events must be identifiable without deep payload inspection

---

## 5.2 Workstream B: Timeline UI

### Goal

Make the trace detail page useful for answering: "What happened, in what order, and where did it fail?"

### Planned Deliverables

1. Timeline section on trace detail page
2. Event cards/rows with clear ordering and visual status
3. Connection between timeline events and related spans where available

### UX Requirements

1. Show strict chronological ordering
2. Highlight failures and warnings clearly
3. Surface key details without forcing users to inspect raw JSON immediately
4. Allow drill-down into event payload when needed

### Recommended Initial Event Presentation

Each timeline item should show:

- timestamp
- event name/type
- related span or trace context
- short summary text
- error/failure marker when relevant
- expandable detail payload

---

## 5.3 Workstream C: Long Polling for Active Traces

### Goal

Provide live-enough updates for active traces without using WebSockets or SSE.

### Planned Behavior

1. When a trace page loads, fetch the full existing timeline
2. If the trace is still active, poll periodically for new events
3. Append only newly returned events to the existing timeline
4. Stop polling when the trace reaches a terminal state

### Recommended Polling Strategy

1. Poll only for active/running traces
2. Use incremental fetches via `after`
3. Poll every 2 to 5 seconds
4. Pause polling when the page is unfocused if this is easy to support

### Why This Approach

- It is simple to host
- It is easy to reason about
- It aligns with the current product need without committing to a realtime platform

---

## 5.4 Workstream D: Python SDK Improvements

### Goal

Strengthen the SDK most likely to drive product adoption in the near term.

### Planned Deliverables

1. Better ingest ergonomics
2. Cleaner trace/span/event helper APIs
3. Clearer flush and shutdown behavior
4. Better error handling and documentation
5. End-to-end examples aligned with the timeline UX

### Priority Areas

1. Ensure emitted payloads align tightly with server expectations
2. Improve usability for common LLM/tool tracing patterns
3. Improve examples around sessions, traces, and events
4. Add or expand tests for edge behavior and reliability

---

## 5.5 Workstream E: Contract Alignment and Hardening

### Goal

Remove friction caused by drift between implementation and documentation.

### Planned Deliverables

1. Align OpenAPI, backend responses, and frontend types for timeline/event fields
2. Reconcile Phase 3 report assumptions with actual Phase 4 product direction
3. Tighten product-visible status/lifecycle semantics where needed

### Specific Alignment Areas

1. Event response schema
2. Trace status and terminal state semantics
3. Session fields used by UI and SDKs
4. Documentation around current ingest behavior and deferred async redesign

---

## 6. Proposed Delivery Order

The recommended order for Phase 4 work is:

1. Define and stabilize event timeline API contract
2. Implement timeline API endpoint(s)
3. Build timeline UI in trace detail page
4. Add long polling for active traces
5. Improve trace status visibility where needed for polling and UI
6. Improve Python SDK to match the new product surface
7. Finish docs and tests

This order ensures that product behavior is built on top of a stable event model rather than inventing UI behavior first.

---

## 7. Acceptance Criteria

Phase 4 should be considered successful when the following are true:

1. A user can open a trace and see an ordered timeline of what happened
2. A user can identify failures from the timeline without depending only on span tree inspection
3. A running trace updates without manual page refresh using long polling
4. Polling stops once the trace is completed or failed
5. Python SDK examples produce traces/events that are easy to inspect in the UI
6. API docs, backend behavior, and UI assumptions match

---

## 8. Risks and Mitigations

### Risk 1: Timeline duplicates span-detail functionality without adding clarity

**Mitigation**:
Design the timeline around ordered debugging events, not around re-rendering the span tree in another format.

### Risk 2: Polling introduces duplicate events or ordering bugs

**Mitigation**:
Define a deterministic ordering cursor and test incremental fetch behavior carefully.

### Risk 3: Python SDK work expands into full multi-SDK parity work

**Mitigation**:
Keep TypeScript SDK explicitly out of scope for this phase.

### Risk 4: Phase 4 grows into backend redesign through async ingest pressure

**Mitigation**:
Treat true async ingest as a separate future phase and document current ingest semantics honestly.

---

## 9. Deferred Items After Phase 4

The following are intentionally not part of this phase and should not be added casually:

1. True async ingest semantics
2. Queue-driven ingest workers
3. WebSocket infrastructure
4. SSE infrastructure
5. Manual session creation API
6. TypeScript SDK parity

---

## 10. Summary

Phase 4 is the phase where Continua should become much easier to use as an observability/debugging product.

The strategic shift is:

- from "ingest and store traces reliably"
- to "help users understand what happened quickly"

If Phase 3 established the backend foundation, Phase 4 should turn that foundation into a significantly stronger debugging experience through:

1. event timelines
2. long polling for active traces
3. Python SDK improvements
4. contract and UX cleanup
