# Continua Custom Commands

Commands adapted from reference repositories for use with Continua's Go/PostgreSQL/OpenAPI codebase.

## Quick Reference

| Command | Invoke As | Purpose | Source |
|---------|-----------|---------|--------|
| add-to-todos | `/project:add-to-todos [description]` | Capture todo with context for later | taches-cc-resources |
| check-todos | `/project:check-todos` | List and select todos to work on | taches-cc-resources |
| whats-next | `/project:whats-next` | Create handoff document for context reset | taches-cc-resources |
| dev-plan | `/project:dev-plan [task]` | Create strategic plan with task breakdown | claude-code-infrastructure-showcase |
| dev-docs-update | `/project:dev-docs-update` | Update docs before context compaction | claude-code-infrastructure-showcase |
| debug | `/project:debug [issue]` | Systematic debugging with hypothesis testing | myclaude |
| review | `/project:review [scope]` | Comprehensive Go code review | myclaude |
| review-pr | `/project:review-pr [PR#]` | Review GitHub pull request | claude-codex-settings |
| test | `/project:test [component]` | Create test strategy and implementation | myclaude |
| optimize | `/project:optimize [target]` | Performance optimization analysis | myclaude |
| security-scan | `/project:security-scan` | Go security scanning (gosec, gitleaks) | claude-code-skill-factory |
| 5-whys | `/project:5-whys [problem]` | Root cause analysis | taches-cc-resources |
| migrate | `/project:migrate [up\|down\|create <name>]` | Database migration management | codex_claude_setup |
| generate | `/project:generate` | Regenerate code from contracts/SQLC | codex_claude_setup |
| openapi-sync | `/project:openapi-sync [description]` | Update OpenAPI contract and regenerate | codex_claude_setup |
| dev | `/project:dev [db\|server\|web\|stop]` | Start/stop development services | codex_claude_setup |
| build | `/project:build [component]` | Build Continua components | codex_claude_setup |
| pr-check | `/project:pr-check` | Run full local CI pipeline | codex_claude_setup |

## Commands by Category

### Workflow Management
- **add-to-todos** - Capture work items mid-conversation with full context
- **check-todos** - Resume work from outstanding todos
- **whats-next** - Create comprehensive handoff for fresh context
- **dev-plan** - Strategic planning with persistent task structure
- **dev-docs-update** - Update documentation before context limits

### Code Quality
- **review** - Multi-dimensional Go code review (quality, security, performance, architecture)
- **review-pr** - GitHub PR review with structured feedback
- **security-scan** - Security scanning with Go-specific tools (gosec, nancy, gitleaks)
- **pr-check** - Full CI pipeline check before opening PR

### Development
- **debug** - Systematic debugging with hypothesis generation and validation
- **test** - Test strategy and implementation for Go (table-driven, testify)
- **optimize** - Performance analysis with Go profiling and PostgreSQL optimization
- **dev** - Start/stop development services (db, server, web)
- **build** - Build server, engine, web, SDK, or contracts

### Database & Code Generation
- **migrate** - Create and run database migrations
- **generate** - Regenerate all code from contracts and SQLC
- **openapi-sync** - Update OpenAPI contract and regenerate types

### Problem Solving
- **5-whys** - Root cause analysis to move past symptoms

## Installation

Copy these commands to your Continua project:

```bash
cp -r commands/ /path/to/continua/.claude/commands/
```

Or copy individual commands as needed.

## Usage Notes

1. **All commands are project-scoped** - Invoke with `/project:command-name`
2. **Arguments use $ARGUMENTS** - Pass arguments after the command name
3. **File references** - Commands reference Go paths like `internal/`, `cmd/`, `db/`
4. **Make commands** - Commands reference Makefile targets (`make test-go`, `make lint-go`)

## Gaps Report

| Category | Status | Notes |
|----------|--------|-------|
| Database/Migrations | ✅ Filled | `migrate` command added |
| OpenAPI/Code Gen | ✅ Filled | `generate`, `openapi-sync` commands added |
| Git Workflow | Partial | Only PR review, missing commit, branch management |
| Scaffolding | Gap | No code generation/scaffolding commands |
| Documentation | Partial | Dev docs only, no API doc generation |

## Attribution

All commands are adapted from open-source reference repositories. See `_sources/sources.md` for complete attribution and original content.
