# Change: Reconcile engine projection freshness contract

## Why
The older `add-engine-public-surface` OpenSpec text defines engine projection freshness as a fully stored state machine on `public.traces`. The later `add-engine-operational-hardening-core` change explicitly forbids `decisionCancelled` and root-side terminate from writing `public.*` projection tables. Those two requirements conflict for terminal cancel/terminate paths and have caused review churn even though the runtime already implements a deliberate reconciliation.

## What Changes
- Modify `engine-trace-projection` to distinguish:
  - stored freshness transitions for start and non-terminal activation progress
  - the terminal cancel/terminate exception where the transaction stays engine-only and the stored projection shell may lag until projector catch-up
- Modify `engine-debugger-integration` to define read-side freshness in terms of an effective per-trace state:
  - stale stored `up_to_date` shells may be exposed as `catching_up`
  - `journal_expired` remains summary-shell-only with no live supplementation for freshness normalization
- Clarify that the read path may normalize freshness without rewriting `public.traces`
- No code, schema, API, or migration behavior changes are introduced by this change

## Impact
- Affected specs:
  - `engine-trace-projection`
  - `engine-debugger-integration`
- Affected code: none; this change documents the existing reconciliation already implemented in:
  - `engine/internal/workflow/activation.go`
  - `internal/api/engine_control.go`
  - `internal/api/traces_handlers.go`
  - `internal/api/sessions_handlers.go`
