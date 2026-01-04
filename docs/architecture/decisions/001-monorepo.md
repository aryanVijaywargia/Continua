# ADR 001: Monorepo Structure

## Status
Accepted

## Context
We need to manage multiple components (server, web UI, SDKs) in a way that enables:
- Shared contracts/types
- Atomic deployments
- Consistent tooling

## Decision
Use a monorepo with:
- Go workspace for server modules
- pnpm workspace for TypeScript packages
- Shared contracts package

## Consequences
- Simpler dependency management
- Atomic commits across components
- Single CI pipeline
