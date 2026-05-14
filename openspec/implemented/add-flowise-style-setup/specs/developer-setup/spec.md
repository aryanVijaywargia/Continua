## ADDED Requirements

### Requirement: Docker-first demo setup

The repository SHALL provide a single documented command that builds and starts a local demo environment with Postgres, applied migrations, the embedded web UI, and seeded sample traces.

#### Scenario: New user starts the demo

- **WHEN** a user runs `make demo` from a clean checkout with Docker available
- **THEN** Continua starts on `http://localhost:8080`
- **AND** the public demo workspace contains seeded trace and session data

### Requirement: Agent-readable setup guide

The repository SHALL provide a canonical setup guide with exact commands, prerequisites, expected success states, health checks, reset commands, and troubleshooting.

#### Scenario: Coding agent bootstraps the repo

- **WHEN** a coding agent is pointed at `docs/setup.md`
- **THEN** it can choose the recommended Docker demo path without asking follow-up questions
- **AND** it can fall back to the native development path when repository edits require local toolchains
