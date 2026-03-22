# TypeScript SDK status

The TypeScript SDK is currently a stub, not a tracing client.

## What exists

```
sdks/typescript/
├── package.json
├── src/index.ts
└── tests/client.test.ts
```

Current exported behavior:
- `VERSION`
- `ContinuaClient` with a configurable `baseUrl`

## What does not exist yet
- trace/span helpers
- batching
- ingest client
- provider integrations
- context propagation
- generated runtime client wrappers

## How to approach TS SDK work
- treat anything beyond the current stub as new product work
- check whether the change should start with OpenSpec
- do not document or implement against imaginary existing files like `client.ts`, `context.ts`, or adapter modules unless you are creating them in the task

## Current verification
- `pnpm --filter @continua/sdk test`
- `pnpm --filter @continua/sdk type-check`
