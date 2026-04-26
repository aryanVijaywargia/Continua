# Change: Add operator auth

## Why
Continua's debugger currently trusts a locally pasted project API key in the browser. That works for early operator use, but it is not an appropriate human-login flow and it prevents a shared operator console with real sign-in, sign-out, and project switching.

## What Changes
- Add Auth0-backed operator authentication for the web debugger using hosted login.
- Keep API-key authentication for ingest and as a backward-compatible fallback for debugger API access.
- Add a public runtime auth config endpoint for the embedded SPA.
- Add an operator-facing projects endpoint and a project switcher model for the debugger UI.
- Add Auth0 token validation plus backend email allowlist enforcement.

## Impact
- Affected specs: `authentication`, `project-selection`
- Affected code: `contracts/openapi/openapi.yaml`, `internal/api`, `internal/config`, `internal/store`, `web/src`
