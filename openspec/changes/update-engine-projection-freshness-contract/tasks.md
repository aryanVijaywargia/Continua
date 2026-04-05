## 1. Spec Reconciliation
- [x] 1.1 Update `engine-trace-projection` to clarify that stored checkpoint transitions remain authoritative for start and non-terminal activation progress, while `decisionCancelled` and root-side terminate stay engine-only and may leave stored projection metadata behind until projector catch-up.
- [x] 1.2 Update `engine-debugger-integration` to define effective projection-state reporting and live fallback behavior for stale stored `up_to_date` shells, while preserving `journal_expired` as summary-shell-only.

## 2. Validation
- [x] 2.1 Run `openspec validate update-engine-projection-freshness-contract --strict`
