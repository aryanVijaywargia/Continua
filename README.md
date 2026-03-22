# Continua

AI Agent Observability Debugger

## Quick Start

```bash
# Setup
./scripts/setup.sh

# Start development
make dev           # Start database
make dev-server    # Start Go backend (in new terminal)
make dev-web       # Start web UI (in new terminal)

# Open http://localhost:3000
```

## Architecture

- **Backend**: Go 1.22+ with Chi router
- **Frontend**: Vite + React + TypeScript
- **Database**: PostgreSQL / SQLite

Current repo-state baseline: [docs/DEBUGGER_PLATFORM_BASELINE.md](./docs/DEBUGGER_PLATFORM_BASELINE.md)

## Commands

```bash
make generate      # Generate all code
make build         # Build everything
make test          # Run tests
make lint          # Run linters
make help          # Show all commands
```

## Project Structure

```
continua/
├── cmd/continua/           # Server binary
├── contracts/              # API contracts (source of truth)
├── db/platform/            # Platform database
├── deploy/                 # Docker, K8s, Helm
├── docs/                   # Documentation
├── engine/                 # Isolated engine module
├── internal/               # Server internals
├── pkg/                    # Shared packages
├── scripts/                # Build scripts
├── sdks/                   # Python & TypeScript SDKs
├── web/                    # Vite React SPA
├── go.mod                  # Root module
├── go.work                 # Workspace (links engine)
├── Makefile                # Build system
└── pnpm-workspace.yaml     # JS workspace
```

## Architecture Rules

See [docs/architecture/RULES.md](./docs/architecture/RULES.md) for the 10 rules that prevent drift.

## License

MIT
