<!--
  Source: codex_claude_setup
  Original Path: commands/pr-check/COMMAND.md
  Adaptations: Minor formatting for consistency
-->
---
description: Run full local CI pipeline before PR
argument-hint: (no arguments)
---

# Pre-PR Check

Run the same checks CI expects by invoking the Makefile pipeline.

## Context
- `make ci` runs: `make generate` → `make lint` → `make test`
- Use before opening a PR to catch drift or test failures early
- Catches common issues like:
  - Generated code out of sync
  - Lint errors
  - Failing tests

## Process

1. Confirm repo root
2. Run full CI pipeline:
   ```bash
   make ci
   ```
3. Report any failures with relevant targets

## What Gets Checked
- Code generation is up to date
- Go and JS linting passes
- All tests pass (unit tests only, not integration)

## Examples
- `/project:pr-check`

## If Checks Fail

**Generated code drift:**
```bash
make generate
```

**Lint errors:**
```bash
make lint-fix  # Auto-fix what's possible
```

**Test failures:**
```bash
make test-go   # Run Go tests with verbose output
```

## Anti-patterns
- Running `/project:pr-check` with uncommitted generated changes you don't intend to keep
- Skipping this before opening a PR
- Ignoring failures and pushing anyway
