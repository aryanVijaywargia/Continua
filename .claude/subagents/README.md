# Claude Code Subagent Patterns

Subagent patterns extracted from reference repositories and adapted for Go/PostgreSQL/OpenAPI stack.

## Active Subagents

| Subagent | Description | When to Use |
|----------|-------------|-------------|
| **bugfix** | Root-cause analysis and minimal targeted fixes | Production bugs, panics, regressions, failing tests |
| **debug** | Structured diagnosis with hypothesis validation | Complex/intermittent issues, multi-package problems |

## Expert Templates

| Template | Description | When to Use |
|----------|-------------|-------------|
| **code-reviewer** | Go code quality and best practices | PR review, before merge |
| **security-reviewer** | OWASP/security audit | Auth code, data handling, API security |
| **plan-reviewer** | Migration/API plan validation | Before implementing schema or API changes |
| **performance-reviewer** | Hot-path and query performance | Latency issues, query changes, batch processing |
| **test-reviewer** | Test coverage assessment | New features, missing tests, high-risk changes |
| **repo-mapper** | Fast codebase exploration | Onboarding, quick overview |
| **feature-explorer** | End-to-end feature tracing | Understanding feature implementation |
| **trace-engineer** | Execution path analysis | Debugging complex flows |

## Directory Structure

```
subagents/
├── README.md
├── bugfix/SUBAGENT.md        # Production bugfix agent
├── debug/SUBAGENT.md         # Systematic debugging agent
├── templates/                 # Expert agent templates
│   ├── code-reviewer.md
│   ├── security-reviewer.md
│   ├── plan-reviewer.md
│   ├── performance-reviewer.md
│   ├── test-reviewer.md
│   ├── repo-mapper.md
│   ├── feature-explorer.md
│   └── trace-engineer.md
├── patterns/                  # Orchestration patterns
│   ├── pipeline-spec-driven.md
│   ├── pipeline-segmented.md
│   ├── fan-out-parallel.md
│   ├── aggregation-hierarchical.md
│   └── deviation-rules.md
├── examples/                  # Usage examples
│   ├── api-endpoint-generation.md
│   ├── database-migration.md
│   └── code-review-workflow.md
└── _sources/                  # Attribution
    ├── sources.md
    └── originals/
```

## Quick Start

### 1. Bugfix Agent
```javascript
Task(
  subagent_type: "bugfix",
  prompt: "Fix panic in internal/api/handlers/ingest.go when body is empty",
  description: "Fix ingest panic"
)
```

### 2. Parallel Review (Fan-Out)
```javascript
// Launch in single message for parallel execution:
Task(subagent_type: "security-reviewer", prompt: "Review auth.go")
Task(subagent_type: "performance-reviewer", prompt: "Review auth.go")
Task(subagent_type: "code-reviewer", prompt: "Review auth.go")
```

### 3. Systematic Debug
```javascript
Task(
  subagent_type: "debug",
  prompt: "Debug intermittent 500s from ingest endpoint under load",
  description: "Debug ingest 500s"
)
```

## Pattern Sources

| Repository | Primary Contribution |
|------------|---------------------|
| langfuse-python | Exploration agents (repo-mapper, feature-explorer, trace-engineer) |
| Claude-Code-Workflow | Pipeline and fan-out patterns |
| taches-cc-resources | Segmented execution, checkpoints |
| agentic-ai-systems | Code review patterns |
| myclaude | Bugfix/debug agents |

## Gaps (Not Covered)

These use cases had no applicable patterns in reference repos:
- PostgreSQL-specific agent
- OpenAPI validation agent
- Go code generation agent
- Schema introspection pipeline
