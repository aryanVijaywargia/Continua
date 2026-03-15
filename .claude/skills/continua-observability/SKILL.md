---
name: continua-observability
description: Domain-specific guide for Continua's current observability core. Use when working on trace/span/session/event semantics, async ingest lifecycle, rollups, timeline behavior, payload truncation, or debugger data surfaces.
---

# Continua Observability Domain

## Read first
- [../../references/decisions.md](../../references/decisions.md)

## Use this skill when
- changing ingest semantics or batch lifecycle
- changing trace/span/session/event data contracts
- changing rollup behavior
- changing timeline ordering or debugger data surfaces
- changing truncation or failure-analysis semantics

## Current domain reality
- Continua's active observability core is REST + Postgres + River + debugger UI.
- The important persisted entities are `projects`, `ingest_batches`, `ingest_batch_payloads`, `sessions`, `traces`, `spans`, and `span_events`.
- The debugger timeline is polling-based and merges explicit events with synthetic span lifecycle events.
- Replay and WebSocket runtime are not implemented product features today.

## Key domain rules
- ingest trace/span IDs are external strings; internal DB identity is UUID-based
- session grouping uses `sessions.external_id`
- span trees use external `parent_span_id`
- rollups compute trace totals from spans asynchronously
- explicit events are separate from synthetic timeline events derived from spans
- payload truncation metadata is part of the active trace/span model

## Useful references
- [trace-lifecycle.md](resources/trace-lifecycle.md)
- [websocket-events.md](resources/websocket-events.md)
- [replay.md](resources/replay.md)
