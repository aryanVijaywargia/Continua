> **Status: Historical**
> This document is preserved for historical context and does not define the current repo state. Use [docs/README.md](../README.md) and [DEBUGGER_PLATFORM_BASELINE.md](../DEBUGGER_PLATFORM_BASELINE.md) for current guidance.

# Phase 3 - Existing Tests Review (Pre-existing Tests)

Scope (files reviewed)
- pkg/truncation/truncate_test.go
- pkg/truncation/wrapper_test.go
- internal/ingest/ingest_test.go
- sdks/typescript/tests/client.test.ts
- sdks/python/tests/test_client.py
- sdks/python/tests/test_trace.py
- sdks/python/tests/test_span.py
- sdks/python/tests/test_batch.py
- sdks/python/tests/test_integration.py

Critical / logical issues
P1 - `internal/ingest/ingest_test.go` contains placeholder tests that do not exercise any production validation or ingest logic.
  - Why it matters: these tests can pass even if ingest validation is broken or missing entirely, which creates false confidence.
  - Fix: replace with service-layer tests that call the ingest service + store (with a real test DB), or remove until true integration tests are in place.

P1 - `internal/ingest/ingest_test.go` includes `TestBatchKeyGeneration` and `TestIntegrationPlaceholder`, which only test UUID formatting or log a message.
  - Why it matters: they are not tied to any production code and do not verify behavior.
  - Fix: delete these tests or rewrite them to validate real behavior (e.g., batch key handling in ingest service, end-to-end ingest + verify DB).

Notable gaps (non-blocking but worth addressing)
- `sdks/python/tests/test_integration.py` validates status codes but does not verify that traces/spans were actually persisted (no follow-up query by trace_id).
  - Suggested fix: after ingest, query `/api/traces` and assert the expected trace ID is present, or add a GET-by-ID check once that endpoint exists.
- `sdks/python/tests/test_batch.py` does not cover background flush interval behavior (`BatchQueue.start()` and timer-based flush).
  - Suggested fix: add a timed flush test with a short interval and a deterministic callback.

Redundancy
- No direct redundancies identified in these pre-existing tests.
