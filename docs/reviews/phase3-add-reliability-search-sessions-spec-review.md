# Phase 3 OpenSpec Review: add-reliability-search-sessions (Post-Update)

## Repo Findings
- Ingest is a single transaction today, and async mode still processes inline (internal/ingest/service.go).
- No search vectors or GIN indexes exist yet (db/platform/migrations/postgres/0001_initial_schema.up.sql).
- Sessions exist in DB but current OpenAPI lacks a session detail endpoint (db/platform/migrations/postgres/0001_initial_schema.up.sql, contracts/openapi/openapi.yaml).

## OpenSpec Issues (Critical/Logical Only)
1) P1 - Search semantics conflict: the search scenario says "checkout flow" matches traces containing "checkout" OR "flow", but the spec mandates `plainto_tsquery` AND semantics (openspec/changes/add-reliability-search-sessions/specs/search/spec.md). This creates contradictory behavior expectations for the same query.

## Recommended Changes
1) Align the trace-name search scenario with AND semantics, or change the parser to `websearch_to_tsquery` if OR semantics are required.

## Paste-Ready Patch Blocks

### Fix trace-name search semantics

`openspec/changes/add-reliability-search-sessions/specs/search/spec.md` (replace the trace-name scenario):

```markdown
#### Scenario: Search by trace name
- **WHEN** a user searches for "checkout flow"
- **THEN** traces with names containing both "checkout" and "flow" are returned
- **AND** results are ranked by relevance (name matches weighted higher)
```

## Risk Register
- If OR semantics are desired but AND semantics are implemented, users will see incomplete search results for multi-term queries.

## Next Steps
1) Apply the patch block above (or switch to `websearch_to_tsquery` and update the spec accordingly).
2) Re-run `openspec validate add-reliability-search-sessions --strict`.
