<!--
  Source: taches-cc-resources
  Original Path: commands/whats-next.md
  Adaptations: Go/PostgreSQL context fields, Makefile commands, internal package references
-->
---
name: whats-next
description: Analyze the current conversation and create a handoff document for continuing this work in a fresh context
allowed-tools:
  - Read
  - Write
  - Bash
---

Create a comprehensive, detailed handoff document that captures all context from the current conversation. This allows continuing the work in a fresh context with complete precision.

## Instructions

**PRIORITY: Comprehensive detail and precision over brevity.** The goal is to enable someone (or a fresh Claude instance) to pick up exactly where you left off with zero information loss.

Adapt the level of detail to the task type (Go development, database migrations, API changes, debugging) but maintain comprehensive coverage:

1. **Original Task**: Identify what was initially requested (not new scope or side tasks)

2. **Work Completed**: Document everything accomplished in detail
   - All artifacts created, modified, or analyzed (Go files, migrations, OpenAPI specs, etc.)
   - Specific changes made (code with line numbers, schema changes, API modifications)
   - Actions taken (commands run like `make test-go`, `make migrate`, API calls, etc.)
   - Findings discovered (insights, patterns, answers, performance data, etc.)
   - Decisions made and the reasoning behind them

3. **Work Remaining**: Specify exactly what still needs to be done
   - Break down remaining work into specific, actionable steps
   - Include precise locations (file paths like `internal/api/handler.go:45-89`)
   - Note dependencies, prerequisites, or ordering requirements
   - Specify validation or verification steps needed (`make lint-go`, `make test-go`)

4. **Attempted Approaches**: Capture everything tried, including failures
   - Approaches that didn't work and why they failed
   - Errors encountered, blockers hit, or limitations discovered
   - Dead ends to avoid repeating
   - Alternative approaches considered but not pursued

5. **Critical Context**: Preserve all essential knowledge
   - Key decisions and trade-offs considered
   - Constraints, requirements, or boundaries
   - Important discoveries, gotchas, edge cases, or non-obvious behaviors
   - Relevant environment, configuration, or setup details
   - Assumptions made that need validation
   - References to documentation, sources, or resources consulted

6. **Current State**: Document the exact current state
   - Status of deliverables (complete, in-progress, not started)
   - What's committed, saved, or finalized vs. what's temporary or draft
   - Any temporary changes, workarounds, or open questions
   - Current position in the workflow or process

Write to `whats-next.md` in the current working directory using the format below.

## Output Format

```xml
<original_task>
[The specific task that was initially requested - be precise about scope]
</original_task>

<work_completed>
[Comprehensive detail of everything accomplished:
- Go files created/modified (with specific references like internal/api/handler.go:45-89)
- Database migrations created or run
- OpenAPI spec changes made
- Commands executed (make generate, make test-go, make lint-go)
- Key discoveries or insights
- Decisions made and reasoning]
</work_completed>

<work_remaining>
[Detailed breakdown of what needs to be done:
- Specific tasks with precise file paths
- Exact targets to create, modify, or analyze
- Dependencies and ordering (e.g., run migration before testing)
- Validation steps (make test-go, make lint-go, make ci)]
</work_remaining>

<attempted_approaches>
[Everything tried, including failures:
- Approaches that didn't work and why
- Errors, blockers, or limitations encountered
- Dead ends to avoid
- Alternative approaches considered but not pursued]
</attempted_approaches>

<critical_context>
[All essential knowledge for continuing:
- Key decisions and trade-offs
- Constraints, requirements, or boundaries
- Important discoveries, gotchas, or edge cases
- Environment, configuration, or setup details
- Assumptions requiring validation
- References to CLAUDE.md, docs/, or external resources]
</critical_context>

<current_state>
[Exact state of the work:
- Status of deliverables (complete/in-progress/not started)
- What's committed vs. what's temporary or draft
- Temporary changes or workarounds in place
- Current position in workflow or process
- Any open questions or pending decisions]
</current_state>
```
