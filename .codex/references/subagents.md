# Claude subagent patterns adapted for Codex

Claude's named subagents are not a Codex feature, but the roles are still useful as review or execution modes.

## Active problem-solving roles
- `bugfix`: root-cause analysis with the smallest viable code change
- `debug`: structured diagnosis with hypotheses, evidence, and validation

## Expert review modes
- `code-reviewer`: correctness, style drift, and maintainability
- `security-reviewer`: auth, secrets, input handling, and unsafe defaults
- `plan-reviewer`: migration, API, or rollout plan validation before coding
- `performance-reviewer`: latency, allocations, batch behavior, and query shape
- `test-reviewer`: missing coverage, weak assertions, and risk concentration

## Exploration modes
- `repo-mapper`: quick codebase map for unfamiliar areas
- `feature-explorer`: trace a feature end to end across layers
- `trace-engineer`: inspect execution paths in ingestion, replay, or event flows

## How to apply this in Codex
- Use these labels as internal working modes when a task benefits from a narrower lens.
- When reviewing changes, fan out mentally across security, performance, and testing before writing conclusions.
- When exploring unfamiliar code, start in `repo-mapper` mode, then switch to the domain skill that matches the files.
