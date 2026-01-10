<!--
  Source: taches-cc-resources
  Original Path: commands/add-to-todos.md
  Adaptations: Go file extensions, internal/ and cmd/ paths, PostgreSQL/OpenAPI context
-->
---
description: Add todo item to TO-DOS.md with context from conversation
argument-hint: <todo-description> (optional - infers from conversation if omitted)
allowed-tools:
  - Read
  - Edit
  - Write
---

# Add Todo Item

## Context

- Current timestamp: !`date "+%Y-%m-%d %H:%M"`

## Instructions

1. Read TO-DOS.md in the working directory (create with Write tool if it doesn't exist)

2. Check for duplicates:
   - Extract key concept/action from the new todo
   - Search existing todos for similar titles or overlapping scope
   - If found, ask user: "A similar todo already exists: [title]. Would you like to:\n\n1. Skip adding (keep existing)\n2. Replace existing with new version\n3. Add anyway as separate item\n\nReply with the number of your choice."
   - Wait for user response before proceeding

3. Extract todo content:
   - **With $ARGUMENTS**: Use as the focus/title for the todo and context heading
   - **Without $ARGUMENTS**: Analyze recent conversation to extract:
     - Specific problem or task discussed
     - Relevant file paths that need attention
     - Technical details (line numbers, error messages, conflicting specifications)
     - Root cause if identified

4. Append new section to bottom of file:
   - **Heading**: `## Brief Context Title - YYYY-MM-DD HH:MM` (3-8 word title, current timestamp)
   - **Todo format**: `- **[Action verb] [Component]** - [Brief description]. **Problem:** [What's wrong/why needed]. **Files:** [Comma-separated paths with line numbers]. **Solution:** [Approach hints or constraints, if applicable].`
   - **Required fields**: Problem and Files (with line numbers like `internal/api/handler.go:123-145`)
   - **Optional field**: Solution
   - Make each section self-contained for future Claude to understand weeks later
   - Use simple list items (not checkboxes) - todos are removed when work begins

5. Confirm and offer to continue with original work:
   - Identify what the user was working on before `/add-to-todos` was called
   - Confirm the todo was saved: "Saved to todos."
   - Ask if they want to continue with the original work: "Would you like to continue with [original task]?"
   - Wait for user response

## Format Example

```markdown
## Fix Database Migration Rollback - 2025-01-10 14:23

- **Fix migration rollback logic** - Handle partial migration failures gracefully. **Problem:** When migration 003 fails mid-execution, rollback doesn't properly restore previous state. **Files:** `db/platform/migrations/postgres/003_add_sessions.sql:15-28`, `internal/migrate/runner.go:89-120`. **Solution:** Wrap migration in transaction, ensure atomic rollback on failure.

- **Update OpenAPI spec validation** - Add stricter schema validation for trace endpoints. **Problem:** Invalid payloads accepted by /v1/traces endpoint causing database constraint violations. **Files:** `contracts/openapi/openapi.bundle.yaml:245-280`, `internal/api/validator.go:34-67`. **Solution:** Add required field validation and type coercion in generated server code.
```
