# Change: Add public demo mode

## Why
Continua needs a hosted portfolio deployment that people can use immediately without creating accounts or handing over data. The current operator-auth flow is correct for private console deployments, but the public showcase should be a read-only sample environment instead of a hosted multi-tenant product.

## What Changes
- Add an env-controlled public demo mode that makes debugger read routes available without sign-in.
- Force all public demo reads to a single seeded project on the server.
- Hide operator-only shell controls and repurpose landing/debugger copy around a read-only sample experience.
- Add a rerunnable demo seed flow and a first-class local run guide.

## Impact
- Affected specs: `public-demo`
- Affected code: `internal/config`, `internal/api`, `contracts/openapi/openapi.yaml`, `web/src`, `scripts`, `docs`
