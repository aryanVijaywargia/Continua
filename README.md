# Continua

A Temporal-inspired durable execution platform for AI agents.

## Features

- **Durable Execution**: Event-sourced agent workflows that survive failures
- **Deterministic Replay**: Debug any execution by replaying recorded events
- **Multi-Language SDKs**: Python and TypeScript SDKs for building agents
- **Time-Travel Debugging**: Step through executions forward and backward

## Quick Start

```bash
# Setup development environment
./scripts/setup-dev.sh

# Start all services
make dev

# Run tests
make test
```

## Documentation

- [Architecture](docs/architecture.md)
- [Getting Started](docs/getting-started.md)
- [API Design](docs/api-design.md)
- [Generated Code](docs/GENERATED_CODE.md)

## Project Structure

```
continua/
├── proto/                  # Protobuf definitions (source of truth)
├── packages/
│   ├── proto-go/           # Generated Go code
│   └── api-client/         # TypeScript API client
├── server/                 # Go server (Gateway, Execution, Dispatch, Recorder)
├── sdks/
│   ├── python/             # Python SDK
│   └── typescript/         # TypeScript SDK
├── apps/
│   └── web/                # Next.js debug UI
├── tests/integration/      # Integration tests
└── docker/                 # Docker configurations
```

## License

MIT
