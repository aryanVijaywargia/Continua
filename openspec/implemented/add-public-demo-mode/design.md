## Context
Continua already has a protected operator console backed by Auth0 runtime bootstrap. The new requirement is a separate portfolio-style deployment where visitors can browse curated traces without creating accounts, while real user data continues to live in local or private self-hosted environments.

## Goals / Non-Goals
- Goals:
  - provide a public read-only debugger demo
  - force all demo reads into a single seeded project
  - keep ingest and control routes protected
  - expose clear run-locally guidance
- Non-Goals:
  - hosted end-user accounts
  - browser-visible project API keys
  - multi-tenant hosted data storage
  - changing private non-demo auth behavior

## Decisions
- Decision: demo mode is server-enforced through request context, not a browser API key.
  - Why: a browser-visible project API key could write to ingest routes and would not actually create a safe read-only environment.
- Decision: demo mode is controlled entirely by env vars.
  - Why: the hosted portfolio deployment should be switchable without rebuilding the frontend.
- Decision: the runtime auth config endpoint carries demo metadata.
  - Why: the SPA needs to know when to skip Auth0 bootstrapping and hide operator-only chrome.
- Decision: the demo seed flow recreates a dedicated project, then repopulates it through the existing ingest path.
  - Why: demo data should exercise the real ingest pipeline while remaining easy to refresh without manual DB surgery.

## Risks / Trade-offs
- Public demo reads widen anonymous access to a small set of endpoints.
  - Mitigation: admit only GET debugger read routes and force the project scope in middleware.
- Demo data can drift from the actual product if it is hand-maintained.
  - Mitigation: generate it through the SDK demo example and keep the seed command rerunnable.
- Local self-host flows still need a private-console path that does not depend on Auth0.
  - Mitigation: keep the browser API-key flow available only for non-demo local/private use, while public demo mode still sends no browser-visible project key.

## Migration Plan
1. Add config, contract, middleware, and runtime auth config fields for demo mode.
2. Update the SPA to treat demo mode as a public read-only shell.
3. Add the demo seed script and run-locally documentation.
4. Validate demo and non-demo routes with backend, frontend, and smoke coverage.
