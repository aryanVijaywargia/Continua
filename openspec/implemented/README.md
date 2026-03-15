# Implemented OpenSpec Changes

This folder holds OpenSpec changes that are implemented in the codebase and no longer need to stay in `openspec/changes/` for active planning visibility.

It is intentionally separate from `openspec/changes/archive/`:
- `openspec/implemented/` means implemented and moved out of the active queue for repository navigation.
- `openspec/changes/archive/` remains the place for formal OpenSpec archival when you want to follow the full archive workflow.

| change_id | status_basis | evidence_doc | why_not_left_active |
| --- | --- | --- | --- |
| `enable-e2e-usability` | `code_verified_implemented` | `docs/guides/phase2_e2e_usability_codebase_guide.md` | Guide explicitly marks the proposal/spec set as implemented; active folder is stale bookkeeping. |
| `add-ingestion-pipeline` | `code_verified_implemented` | `docs/guides/ingestion_codebase_guide.md` | Foundational ingest pipeline exists in live code and docs; it should not be mistaken for pending work. |
| `add-reliability-search-sessions` | `code_verified_implemented` | `docs/phase3/report_phase3.md` | Phase 3 capabilities are implemented, but the original checklist was never fully kept in sync. |
| `fix-token-rollup-sessions` | `formal_complete` | `docs/PHASE5_CURRENT_STATE_REPORT.md` | This change is formally complete and should not remain mixed into active planning folders. |
| `add-timeline-debugging` | `code_verified_implemented` | `docs/phase4/REPORT.md` | Timeline API/UI and polling are implemented; remaining checklist drift is verification bookkeeping. |
| `add-debugger-data-surface` | `code_verified_implemented` | `docs/PHASE5_CURRENT_STATE_REPORT.md` | Trace detail/context surface is implemented; unchecked tasks are final validation items rather than missing feature work. |

Remaining active work stays in `openspec/changes/`.
