# Capability: engine-cli-foundation

Cobra-based `continua-engine` binary with `version`, `migrate up`, and `migrate down [steps]` commands.

Related capabilities: [engine-schema-foundation](../engine-schema-foundation/spec.md), [engine-store-foundation](../engine-store-foundation/spec.md)

## ADDED Requirements

### Requirement: Cobra root command

The `continua-engine` binary SHALL replace the placeholder `main.go` with a Cobra root command that groups engine subcommands.

#### Scenario: Root command with no args
- **WHEN** `continua-engine` is invoked with no arguments
- **THEN** it prints usage/help text listing available subcommands

---

### Requirement: Version command

The `continua-engine` binary SHALL provide a `version` subcommand that prints the engine build version.

#### Scenario: Version output
- **WHEN** `continua-engine version` is invoked
- **THEN** it prints the engine version string to stdout

---

### Requirement: Migrate up command

The `continua-engine` binary SHALL provide a `migrate up` subcommand that applies all pending engine migrations.

#### Scenario: Migrate up with pending migrations
- **WHEN** `continua-engine migrate up` is invoked against a database with no engine schema
- **THEN** all engine migrations are applied and the `engine` schema with all tables exists

#### Scenario: Migrate up idempotent
- **WHEN** `continua-engine migrate up` is invoked against a database that is already fully migrated
- **THEN** the command succeeds with no error (no-op)

---

### Requirement: Migrate down command

The `continua-engine` binary SHALL provide a `migrate down [steps]` subcommand that rolls back the specified number of migration steps.

#### Scenario: Migrate down one step
- **WHEN** `continua-engine migrate down 1` is invoked after a successful `migrate up`
- **THEN** the most recent migration is rolled back

#### Scenario: Migrate down without steps
- **WHEN** `continua-engine migrate down` is invoked without a step count
- **THEN** the command prints an error requiring a step count (safety: prevent accidental full rollback)

---

### Requirement: Engine config

Engine CLI commands MUST load configuration from environment variables.

#### Scenario: ENGINE_DATABASE_URL override
- **WHEN** `ENGINE_DATABASE_URL` is set
- **THEN** the engine connects to that database URL

#### Scenario: DATABASE_URL fallback
- **WHEN** `ENGINE_DATABASE_URL` is not set but `DATABASE_URL` is
- **THEN** the engine falls back to `DATABASE_URL`

#### Scenario: No database URL
- **WHEN** neither `ENGINE_DATABASE_URL` nor `DATABASE_URL` is set
- **THEN** the command exits with a clear error message
