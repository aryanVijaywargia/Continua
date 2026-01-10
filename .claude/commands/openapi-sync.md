<!--
  Source: codex_claude_setup
  Original Path: commands/openapi-sync/COMMAND.md
  Adaptations: Minor formatting for consistency
-->
---
description: Update OpenAPI contract and regenerate types
argument-hint: [endpoint or schema description]
---

# OpenAPI Sync

Keep `contracts/openapi/openapi.yaml` aligned with the intended API contract and regenerate derived types.

## Context
- OpenAPI is the source of truth for API contracts
- Bundle file: `contracts/openapi/openapi.bundle.yaml`
- **Generated outputs:**
  - `contracts/generated/go/server_gen.go`
  - `internal/api/server_gen.go`
  - `contracts/generated/typescript/api.ts`

## Process

1. **Read current spec**:
   - Examine `contracts/openapi/openapi.yaml`
   - Identify affected paths/schemas based on $ARGUMENTS

2. **Update OpenAPI spec**:
   - Add/modify paths, parameters, request/response schemas
   - Keep tags and operationIds consistent
   - Ensure required fields and formats match intended types
   - Add appropriate error responses

3. **Update WebSocket contracts** (if applicable):
   - Modify `contracts/websocket/events.ts`

4. **Regenerate**:
   ```bash
   make generate
   ```
   Or invoke `/project:generate`

5. **Summarize changes**:
   - List spec changes
   - List generated file updates

## Examples
- `/project:openapi-sync add traces export endpoint`
- `/project:openapi-sync update Span schema with new attributes`
- `/project:openapi-sync add pagination to list endpoints`

## Anti-patterns
- Editing `contracts/generated/*` or `internal/api/server_gen.go` directly
- Updating OpenAPI without running `/project:generate`
- Introducing breaking changes without documenting them
- Inconsistent operationIds or tags
