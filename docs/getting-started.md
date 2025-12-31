# Getting Started with Continua

## Prerequisites

- Go 1.22+
- Node.js 20+
- Python 3.12+
- Docker
- pnpm (via corepack)
- uv (Python package manager)
- buf (Protocol Buffer tooling)

## Quick Setup

```bash
# Clone the repository
git clone https://github.com/continua-ai/continua
cd continua

# Run setup script (installs all dependencies)
./scripts/setup-dev.sh

# Or manually check your environment
make doctor

# Start development servers
make dev
```

## Project Structure

```
continua/
├── proto/                  # Protocol Buffer definitions
├── packages/
│   ├── proto-go/           # Generated Go types
│   └── api-client/         # TypeScript API client
├── server/                 # Go server
├── sdks/
│   ├── python/             # Python SDK
│   └── typescript/         # TypeScript SDK
├── apps/web/               # Debug UI
└── docker/                 # Docker configs
```

## Common Commands

```bash
make dev           # Start all services
make test          # Run unit tests
make test-integration  # Run integration tests
make proto         # Regenerate proto code
make lint          # Run all linters
make clean         # Clean build artifacts
```

## Next Steps

1. Read the [Architecture](architecture.md) document
2. Explore the [API Design](api-design.md)
3. Build your first agent with the [SDK documentation](../sdks/python/README.md)
