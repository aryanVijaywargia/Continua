# Claude command inventory translated for Codex

Claude's `/project:*` commands do not exist in Codex, but the underlying workflows are still useful. Ask for these directly in natural language.

## Workflow management
- `add-to-todos`: capture follow-up work with enough context to resume later
- `check-todos`: review and pick from outstanding tasks
- `whats-next`: create a handoff note before context resets
- `dev-docs-update`: refresh docs or implementation notes before stopping

## Development workflows
- `dev`: start or stop the local database, server, or web app
- `build`: build specific components or the full repo
- `generate`: regenerate contracts, SQLC outputs, and SDK types
- `migrate`: create or run database migrations
- `openapi-sync`: update the OpenAPI contract and regenerate downstream code

## Quality workflows
- `pr-check`: run the local pre-PR pipeline
- `security-scan`: audit auth, secrets handling, and dependency or code risks
- `optimize`: inspect hot paths, query performance, or high-latency flows
- `5-whys`: run root-cause analysis when debugging recurring issues

## Review and testing workflows
- `review`: perform a code review focused on bugs, regressions, and missing tests
- `review-pr`: inspect a pull request or diff with review findings first
- `test`: plan and implement the smallest relevant test coverage for the change

## Suggested Codex phrasing
- "Review this diff for regressions and missing tests."
- "Run the pre-PR checks relevant to these files."
- "Create a migration for X and wire the generated code."
- "Update the OpenAPI contract, regenerate, and implement the handler."
