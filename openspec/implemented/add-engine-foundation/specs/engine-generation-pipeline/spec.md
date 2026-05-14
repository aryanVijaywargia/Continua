# Capability: engine-generation-pipeline

Updates to `Makefile`, `scripts/generate.sh`, `scripts/check-generated.sh`, and CI so engine sqlc generation always runs.

Related capabilities: [engine-schema-foundation](../engine-schema-foundation/spec.md), [engine-store-foundation](../engine-store-foundation/spec.md)

## ADDED Requirements

### Requirement: Unconditional engine sqlc generation

The generation pipeline MUST run engine sqlc unconditionally once engine query files exist, including when `scripts/generate.sh` executes.

#### Scenario: generate.sh runs engine sqlc
- **WHEN** `scripts/generate.sh` executes
- **THEN** it runs `sqlc generate` in `engine/db/` unconditionally (not gated on non-empty queries directory)

#### Scenario: make generate includes engine
- **WHEN** `make generate` is run
- **THEN** engine sqlc output in `engine/db/gen/go/` is regenerated alongside platform sqlc output

#### Scenario: CI drift check includes engine
- **WHEN** CI runs `scripts/check-generated.sh`
- **THEN** any drift in `engine/db/gen/go/` causes a failure, same as platform drift

---

### Requirement: Engine sqlc fully-qualified queries

Engine sqlc query files MUST use fully-qualified table names to match the schema-qualified DDL.

#### Scenario: Query file table references
- **WHEN** an engine sqlc query file references a table
- **THEN** it uses the fully-qualified form (e.g., `engine.instances`, `engine.runs`, not bare `instances`)
