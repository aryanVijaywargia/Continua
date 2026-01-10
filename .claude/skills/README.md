# Continua Skills

Domain-focused skills for Claude Code, following the pattern from `claude-code-infrastructure-showcase`.

## Design Principles

1. **Domain-level, not niche** - "continua-backend-dev" not "go-service" or "sqlc-queries"
2. **Concise SKILL.md** - ~150-200 lines, quick reference only
3. **Progressive disclosure** - Detailed content in `resources/` folder
4. **Project-specific decisions** - Things Claude doesn't know about Continua

## Skills (5 total)

### continua-backend-dev
**Purpose:** Backend development patterns for Continua's Go monorepo

**Covers:**
- Contract-first development (OpenAPI → Go)
- SQLC queries and migrations
- Chi routing and handler patterns
- The 10 Architecture Rules

**When to use:** Working on `cmd/`, `internal/`, `db/`, `contracts/`

### continua-observability
**Purpose:** Domain-specific guide for Continua's trace/span data model

**Covers:**
- Trace/span tree structure
- WebSocket real-time events
- Token counting and cost calculation
- Replay system

**When to use:** Working on trace ingestion, WebSocket handlers, replay functionality

### continua-integrations
**Purpose:** SDK and proxy development patterns

**Covers:**
- LLM proxy implementation
- Python SDK patterns
- TypeScript SDK patterns
- Framework adapters (LangChain, etc.)

**When to use:** Working on `sdks/`, `internal/proxy/`

### continua-testing
**Purpose:** Testing strategy and commands

**Covers:**
- Unit vs integration test scope
- Make targets (`make test`, `make test-go`, etc.)
- SDK testing commands
- Coverage expectations

**When to use:** Adding tests, fixing bugs, changing API/DB behavior

### skill-developer
**Purpose:** Meta-skill for creating and managing Claude Code skills

**Covers:**
- Skill structure and YAML frontmatter
- Trigger types (keywords, intent patterns, file paths, content patterns)
- Hook mechanisms (UserPromptSubmit, PreToolUse)
- Enforcement levels (block, suggest, warn)
- The 500-line rule and progressive disclosure

**When to use:** Creating new skills, debugging skill activation, extending Claude Code setup

## Shared References

### references/decisions.md
Critical shared knowledge about Continua:
- Source of truth locations (contracts, data model docs)
- Generated files list (do not edit)
- Codegen commands (`make generate`)
- Database requirements (Postgres + SQLite parity)

## Structure

```
skills/
├── README.md
├── references/
│   └── decisions.md          # Shared decisions across all skills
├── continua-backend-dev/
│   ├── SKILL.md
│   └── resources/
│       ├── architecture.md
│       ├── api-patterns.md
│       ├── database.md
│       └── testing.md
├── continua-observability/
│   ├── SKILL.md
│   └── resources/
│       ├── trace-lifecycle.md
│       ├── websocket-events.md
│       └── replay.md
├── continua-integrations/
│   ├── SKILL.md
│   └── resources/
│       ├── proxy.md
│       ├── python-sdk.md
│       └── typescript-sdk.md
└── continua-testing/
    ├── SKILL.md
    └── references/
        ├── strategy.md
        └── commands.md
└── skill-developer/
    ├── SKILL.md
    ├── TRIGGER_TYPES.md
    ├── SKILL_RULES_REFERENCE.md
    ├── HOOK_MECHANISMS.md
    ├── PATTERNS_LIBRARY.md
    ├── TROUBLESHOOTING.md
    └── ADVANCED.md
```

## Token Efficiency

| Skill | SKILL.md Lines | Resources |
|-------|----------------|-----------|
| continua-backend-dev | ~150 | 4 files |
| continua-observability | ~140 | 3 files |
| continua-integrations | ~100 | 3 files |
| continua-testing | ~15 | 2 files |
| skill-developer | ~420 | 6 files |

Claude loads only SKILL.md initially. Resources are loaded on-demand when Claude needs deeper information.

## Why Not More Skills?

The codex_claude_setup has additional skills like:
- `continua-contracts-openapi`
- `continua-db-sqlc`
- `continua-sdk-generation`
- `continua-tracing-model`

These are **not included** because:
1. `continua-backend-dev` already covers contracts, DB, and general patterns
2. `continua-observability` covers tracing model
3. Adding more would create overlap and bloat

The 4 Continua skills + skill-developer + shared references provide complete coverage without redundancy.
