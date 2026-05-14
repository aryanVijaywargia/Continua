# Change: Add Flowise-style local setup

## Why

Continua's first-run path was split across README, local-run docs, Make targets, Docker Compose, setup scripts, and SDK examples. New users and coding agents need one deterministic clone-to-running-demo path.

## What Changes

- Add a Docker-first demo command that builds the app, starts Postgres, runs migrations, starts the embedded UI, and seeds sample traces.
- Add a dedicated setup guide for humans and coding agents.
- Refresh the README around quick local success and the active product surface.
- Clarify native development setup and public-demo versus private-console behavior.

## Impact

- Affected specs: developer-setup
- Affected code/docs: `Makefile`, `scripts/demo.sh`, `scripts/setup.sh`, `deploy/docker/Dockerfile`, `deploy/docker-compose/docker-compose.yml`, `README.md`, `docs/setup.md`, `docs/guides/run-locally.md`
