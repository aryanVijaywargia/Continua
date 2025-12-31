# Generated Code in Continua

## Overview

This repository contains generated code from Protocol Buffer definitions.
This document explains what is generated, how to regenerate it, and what
must be committed.

## What is Generated

| Source | Output Location | Language |
|--------|-----------------|----------|
| `proto/continua/**/*.proto` | `packages/proto-go/continua/` | Go |
| `proto/continua/**/*.proto` | `packages/api-client/src/generated/` | TypeScript |
| `proto/continua/**/*.proto` | `sdks/python/src/continua/_generated/` | Python |

## How to Regenerate

```bash
# Regenerate all generated code
make proto

# Check if generated code is up-to-date
make proto-check
```

## Policy: Generated Code IS Committed

We commit generated code for these reasons:

1. **Lower contributor friction** - Clone and go, no extra tools needed
2. **Faster CI** - No generation step in most jobs
3. **Easier code review** - See exactly what proto changes produce

### Trade-offs

- Proto changes produce larger diffs
- Must remember to run `make proto` after editing `.proto` files
- CI runs `proto-check` to catch stale generated code

## CI Verification

The `proto-check` CI job verifies generated code matches proto definitions.
If this job fails, run `make proto` locally and commit the changes.

## Linter Exclusions

Generated code is excluded from linters:

- **Go**: `.golangci.yml` excludes `packages/proto-go`
- **TypeScript**: `eslint.config.js` excludes `src/generated/**`
- **Python**: `pyproject.toml` excludes `_generated/**`
