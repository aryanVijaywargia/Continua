> **Status: Historical**
> This document is preserved for historical context and does not define the current repo state. Use [docs/README.md](../README.md) and [DEBUGGER_PLATFORM_BASELINE.md](../DEBUGGER_PLATFORM_BASELINE.md) for current guidance.

# Phase 4 Spec 4: Python SDK Event Helpers

## Implemented Surface

Files:

- `sdks/python/src/continua/span.py`
- `sdks/python/tests/test_errors.py`
- `sdks/python/examples/e2e_demo.py`

## SDK Changes

Added span helper methods:

- `span.error(message, payload=None)`
- `span.exception(exc, payload=None)`
- `span.metric(name, value, unit=None, payload=None)`

Refactor:

- extracted `_record_event(...)` so `log`, `error`, `exception`, and `metric` share one event batching path

## Payload Behavior

Implemented payload shapes:

- `error`: preserves caller payload and emits `event_type="error"` with `level="error"`
- `exception`: merges caller payload with:
  - `exception_type`
  - `exception_message`
  - `traceback`
- `metric`: merges caller payload with:
  - `metric_name`
  - `metric_value`
  - optional `metric_unit`

## Example Update

Updated `sdks/python/examples/e2e_demo.py` to demonstrate:

- `log()`
- `metric()`
- `error()`
- `exception()`

## Verification

Commands run:

- `uv run pytest -q tests/test_errors.py tests/test_span.py`
- `uv run pytest -q`

Observed result:

- focused SDK helper tests passed
- full Python SDK suite passed: `39 passed, 12 skipped`
