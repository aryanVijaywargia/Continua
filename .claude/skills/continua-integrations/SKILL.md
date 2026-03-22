---
name: continua-integrations
description: Guide for Continua's integration surfaces as they exist today. Use when working on the Python SDK, contract-driven SDK generation, the TypeScript SDK stub, or planning new proxy/adapter work from the current scaffolded baseline.
---

# Continua Integrations

## Read first
- [../references/decisions.md](../references/decisions.md)

## Use this skill when
- changing `sdks/python/`
- changing `sdks/typescript/`
- changing contract generation that affects SDK types
- evaluating work in `internal/proxy/`

## Current integration reality
- The Python SDK is real and actively usable.
- The TypeScript SDK is only a stub package.
- `internal/proxy/` is a placeholder directory, not a live proxy implementation.
- Framework adapters are not implemented in this repo today.

## Practical guidance

### Python SDK
- treat `sdks/python/` as the main integration surface
- keep batching/retry behavior in `client.py` and `batch.py`
- keep context logic in `trace.py`, `span.py`, and `session.py`
- preserve the current behavior where tracing quietly skips if the global client is not initialized

### TypeScript SDK
- do not assume a tracing client already exists
- current package exposes only a version constant and a minimal `ContinuaClient`
- substantial TS SDK expansion should usually start with OpenSpec because it changes product scope, not just docs

### Proxy or framework adapters
- treat proxy/adapter work as new capability work
- do not extend from imagined handler/provider files that are not present
- verify contract, product, and runtime implications before writing code

## Contract generation rule
- if an SDK change is driven by OpenAPI, update `contracts/openapi/openapi.yaml` first and run `make generate`
- Python types are regenerated from the OpenAPI bundle
- the TS SDK is not currently generated from a full runtime client implementation

## Useful references
- [python-sdk.md](resources/python-sdk.md)
- [typescript-sdk.md](resources/typescript-sdk.md)
- [proxy.md](resources/proxy.md)
