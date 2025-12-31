# API Design

## Transport Protocols

Continua uses different transports for different clients:

| Client | Protocol | Port | Status |
|--------|----------|------|--------|
| **Web UI** | Connect over HTTP | 8243 | Implemented |
| **SDKs** | gRPC | 8233 | Future |
| **Internal** | gRPC | 8233 | Future |

## Current Implementation

The server currently exposes HTTP/Connect on port 8243. gRPC on port 8233 will be added in a future phase.

## When to Use Which

- **Building a web app?** Use `@continua/api-client` (Connect/HTTP on 8243)
- **Building an agent in Python/TS?** Use the SDK (HTTP for now, gRPC later)
- **Quick testing/debugging?** Use Connect (works with curl)

```bash
# Health check
curl http://localhost:8243/health

# Connect works with curl for all endpoints
curl -X POST http://localhost:8243/continua.api.v1.ExecutionService/ListAgentExecutions \
  -H "Content-Type: application/json" \
  -d '{}'
```
