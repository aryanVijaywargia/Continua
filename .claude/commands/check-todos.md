<!--
  Source: taches-cc-resources
  Original Path: commands/check-todos.md
  Adaptations: Go-specific workflow detection (internal/, cmd/, db/), CLAUDE.md integration
-->
---
description: List outstanding todos and select one to work on
allowed-tools:
  - Read
  - Edit
  - Glob
---

# Check Todos

## Instructions

1. Read TO-DOS.md in the working directory (if doesn't exist, say "No outstanding todos" and exit)

2. Parse and display todos:
   - Extract all list items starting with `- **` (active todos)
   - If none exist, say "No outstanding todos" and exit
   - Display compact numbered list showing:
     - Number (for selection)
     - Bold title only (part between `**` markers)
     - Date from h2 heading above it
   - Prompt: "Reply with the number of the todo you'd like to work on."
   - Wait for user to reply with a number

3. Load full context for selected todo:
   - Display complete line with all fields (Problem, Files, Solution)
   - Display h2 heading (topic + date) for additional context
   - Read and briefly summarize relevant files mentioned

4. Check for established workflows:
   - Read CLAUDE.md (if exists) to understand project-specific workflows and rules
   - Look for `.claude/skills/` directory
   - Match file paths in todo to domain patterns:
     - `internal/api/` or `contracts/openapi/` -> API development workflow
     - `db/platform/migrations/` -> Database migration workflow
     - `internal/store/` -> Data access layer workflow
     - `cmd/` -> Entry point / CLI workflow
     - `web/` -> Frontend workflow
   - Check CLAUDE.md for explicit workflow requirements for this type of work

5. Present action options to user:
   - **If matching skill/workflow found**: "This looks like [domain] work. Would you like to:\n\n1. Invoke [skill-name] skill and start\n2. Work on it directly\n3. Brainstorm approach first\n4. Put it back and browse other todos\n\nReply with the number of your choice."
   - **If no workflow match**: "Would you like to:\n\n1. Start working on it\n2. Brainstorm approach first\n3. Put it back and browse other todos\n\nReply with the number of your choice."
   - Wait for user response

6. Handle user choice:
   - **Option "Invoke skill" or "Start working"**: Remove todo from TO-DOS.md (and h2 heading if section becomes empty), then begin work (invoke skill if applicable, or proceed directly)
   - **Option "Brainstorm approach"**: Keep todo in file, brainstorm the approach with user
   - **Option "Put it back"**: Keep todo in file, return to step 2 to display the full list again

## Display Format

```
Outstanding Todos:

1. Fix migration rollback logic (2025-01-10 14:23)
2. Update OpenAPI spec validation (2025-01-10 14:23)
3. Optimize trace ingestion query (2025-01-09 09:15)

Reply with the number of the todo you'd like to work on.
```
