## Context
Continua serves an embedded React debugger from the Go server and currently protects both ingest and debugger APIs with project API keys. The new operator-auth flow must add human login without breaking ingest clients or the existing project-scoped data model.

## Goals / Non-Goals
- Goals:
  - add hosted operator login for the debugger
  - keep SDK and custom-client ingest on API keys
  - preserve project-scoped reads with an active-project switcher
  - avoid adding user or membership tables in v1
- Non-Goals:
  - replacing ingest auth
  - adding per-user project memberships
  - adding multi-project merged debugger views

## Decisions
- Decision: Auth0 is the hosted identity provider.
  - Why: the requested implementation needs a popular prebuilt product with a no-branding path and official React SPA plus Go API guidance.
- Decision: debugger APIs accept either Auth0 bearer tokens or existing API keys.
  - Why: the operator console moves to bearer auth, while existing scripts and any direct API-key debugger usage remain functional during the transition.
- Decision: ingest endpoints remain API-key only.
  - Why: ingest already relies on project API keys and does not need user identity.
- Decision: operator-to-project authorization is deployment-global in v1.
  - Why: all signed-in operators can see all projects, so the backend only needs an email allowlist and the UI only needs an active project selector.
- Decision: the SPA fetches runtime Auth0 config from the server.
  - Why: Continua embeds the production web bundle into the Go binary, so auth configuration must be supplied at server runtime instead of Vite build time.
- Decision: backend allowlist enforcement resolves operator email from the validated Auth0 token via `/userinfo`.
  - Why: custom API access tokens are the correct API credential shape, but they do not reliably contain email claims by default. Using `/userinfo` avoids coupling the app to a required Auth0 Action.

## Risks / Trade-offs
- Calling `/userinfo` adds upstream latency on the first request per token.
  - Mitigation: cache resolved userinfo by token until token expiry.
- The debugger UI now depends on Auth0 runtime configuration.
  - Mitigation: fail fast in server config when Auth0 env is partially configured and show a clear client error if runtime config cannot be loaded.
- API-key fallback increases auth-branch complexity.
  - Mitigation: centralize request auth state in middleware and keep handlers on a single project-resolution helper.

## Migration Plan
1. Add OpenSpec change and contract updates.
2. Implement backend config, token validation, runtime config endpoint, and project endpoint.
3. Switch the SPA to Auth0 bootstrap and bearer-token requests while preserving `returnTo` and URL state.
4. Keep API-key fallback enabled for debugger endpoints during rollout.

## Open Questions
- None for v1. Manual Auth0 tenant setup remains an operator task documented in the final implementation notes.
