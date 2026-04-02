> **Status: Historical**
> This document is preserved for historical context and does not define the current repo state. Use [docs/README.md](../README.md) and [DEBUGGER_PLATFORM_BASELINE.md](../DEBUGGER_PLATFORM_BASELINE.md) for current guidance.

# Spec 2: Idempotency Hardening

## Summary

Fixed time handling for out-of-order span updates using LEAST/GREATEST pattern.

## Changes Made

### 1. Updated UpsertSpan Query

**File**: `db/platform/queries/spans.sql`

**Before** (COALESCE-only approach):
```sql
start_time = COALESCE(EXCLUDED.start_time, spans.start_time),
end_time = COALESCE(EXCLUDED.end_time, spans.end_time),
```

**After** (LEAST/GREATEST with triple COALESCE):
```sql
-- Use LEAST/GREATEST for time merging to handle out-of-order updates correctly
start_time = COALESCE(
    LEAST(spans.start_time, EXCLUDED.start_time),
    spans.start_time,
    EXCLUDED.start_time
),
end_time = COALESCE(
    GREATEST(spans.end_time, EXCLUDED.end_time),
    spans.end_time,
    EXCLUDED.end_time
),
```

### 2. Regenerated SQLC

Ran `make generate` to regenerate `db/gen/go/platform/spans.sql.go` with the updated query.

## Behavior

### Before (Problem)
- If an update with `end_time=T2` arrived first, then an update with `start_time=T1` arrived
- The COALESCE would keep the first non-NULL value, potentially losing data

### After (Fixed)
- **LEAST** ensures the earliest `start_time` is always preserved
- **GREATEST** ensures the latest `end_time` is always preserved
- Triple COALESCE handles all NULL combinations:
  1. Both values exist → use LEAST/GREATEST result
  2. Only existing value → keep it
  3. Only new value → use it

## Test Coverage

Tests exist in `internal/store/spans_test.go`:

| Test | Scenario |
|------|----------|
| `TestUpsertSpan_EndTimeArriveBeforeStartTime` | End time arrives first, then start time |
| `TestUpsertSpan_StartTimeArriveBeforeEndTime` | Start time arrives first, then end time |
| `TestUpsertSpan_EarlierStartTimeReplacesLater` | Earlier start_time replaces later one (LEAST) |
| `TestUpsertSpan_LaterEndTimeReplacesEarlier` | Later end_time replaces earlier one (GREATEST) |
| `TestUpsertSpan_NullTimestampDoesNotOverwrite` | NULL values don't overwrite existing timestamps |

## Verification

```bash
# Regenerate code
make generate

# Run Go vet (note: some unrelated test files have issues pending Spec 1)
go vet ./internal/store/...

# Run tests (requires PostgreSQL)
go test -v ./internal/store/... -run TestUpsertSpan
```

---

=== SPEC 2 COMPLETE: READY FOR REVIEW ===
