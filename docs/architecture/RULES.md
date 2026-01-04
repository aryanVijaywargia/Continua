# Continua Architecture Rules

## 10 Rules That Prevent Drift and Coupling

### 1. ONE generation command
```bash
make generate  # CI runs this and fails on any diff.
```

### 2. Contracts are source of truth
- `contracts/openapi/openapi.yaml` → REST API
- `contracts/websocket/events.ts` → WebSocket events

### 3. Generated Go files use `_gen.go` suffix (except sqlc)

### 4. Track generated code, not build artifacts

### 5. Module boundaries enforced by Go

### 6. Platform and Engine have separate schemas

### 7. Web UI is static-only

### 8. One lockfile at root

### 9. CI drift check is the gatekeeper

### 10. Domain types never leak into API responses
