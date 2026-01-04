# Architecture Overview

## System Components

### Platform Server
The main Go server that handles:
- REST API for traces, spans, and sessions
- WebSocket connections for real-time updates
- LLM Proxy for intercepting API calls

### Engine (Future)
A separate, isolated Go module for durable workflow execution.

### Web UI
A Vite + React SPA that provides:
- Trace viewer
- Session management
- Real-time updates

### SDKs
Client libraries for Python and TypeScript to integrate with Continua.
