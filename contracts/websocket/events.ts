import { z } from 'zod';

export const SpanKind = z.enum(['LLM', 'TOOL', 'CHAIN', 'AGENT', 'CUSTOM']);
export type SpanKind = z.infer<typeof SpanKind>;

export const SpanStatus = z.enum(['SCHEDULED', 'STARTED', 'COMPLETED', 'FAILED']);
export type SpanStatus = z.infer<typeof SpanStatus>;

export const TraceStatus = z.enum(['RUNNING', 'COMPLETED', 'FAILED']);
export type TraceStatus = z.infer<typeof TraceStatus>;

export const SpanCreatedEvent = z.object({
  type: z.literal('span.created'),
  traceId: z.string().uuid(),
  spanId: z.string().uuid(),
  parentSpanId: z.string().uuid().nullable(),
  name: z.string(),
  kind: SpanKind,
  status: SpanStatus,
  startedAt: z.string().datetime(),
});
export type SpanCreatedEvent = z.infer<typeof SpanCreatedEvent>;

export const SpanUpdatedEvent = z.object({
  type: z.literal('span.updated'),
  spanId: z.string().uuid(),
  status: SpanStatus,
  endedAt: z.string().datetime().nullable(),
  tokensIn: z.number().int().nullable(),
  tokensOut: z.number().int().nullable(),
  costUsd: z.number().nullable(),
  latencyMs: z.number().int().nullable(),
});
export type SpanUpdatedEvent = z.infer<typeof SpanUpdatedEvent>;

export const StreamChunkEvent = z.object({
  type: z.literal('stream.chunk'),
  spanId: z.string().uuid(),
  chunkIndex: z.number().int(),
  content: z.string(),
});
export type StreamChunkEvent = z.infer<typeof StreamChunkEvent>;

export const TraceCompletedEvent = z.object({
  type: z.literal('trace.completed'),
  traceId: z.string().uuid(),
  status: TraceStatus,
  totalCostUsd: z.number(),
  totalDurationMs: z.number().int(),
  spanCount: z.number().int(),
  errorCount: z.number().int(),
});
export type TraceCompletedEvent = z.infer<typeof TraceCompletedEvent>;

export const ErrorEvent = z.object({
  type: z.literal('error'),
  code: z.string(),
  message: z.string(),
  traceId: z.string().uuid().optional(),
  spanId: z.string().uuid().optional(),
});
export type ErrorEvent = z.infer<typeof ErrorEvent>;

export const WebSocketEvent = z.discriminatedUnion('type', [
  SpanCreatedEvent,
  SpanUpdatedEvent,
  StreamChunkEvent,
  TraceCompletedEvent,
  ErrorEvent,
]);
export type WebSocketEvent = z.infer<typeof WebSocketEvent>;

export const SubscribeMessage = z.object({
  type: z.literal('subscribe'),
  traceIds: z.array(z.string().uuid()).optional(),
  sessionIds: z.array(z.string().uuid()).optional(),
});
export type SubscribeMessage = z.infer<typeof SubscribeMessage>;

export const UnsubscribeMessage = z.object({
  type: z.literal('unsubscribe'),
  traceIds: z.array(z.string().uuid()).optional(),
  sessionIds: z.array(z.string().uuid()).optional(),
});
export type UnsubscribeMessage = z.infer<typeof UnsubscribeMessage>;

export const ClientMessage = z.discriminatedUnion('type', [
  SubscribeMessage,
  UnsubscribeMessage,
]);
export type ClientMessage = z.infer<typeof ClientMessage>;
