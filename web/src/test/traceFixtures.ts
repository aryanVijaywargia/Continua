import type { Span, TimelineEvent, Trace, TraceDetail } from '../api/client';

export const SESSION_ID = '123e4567-e89b-12d3-a456-426614174000';
export const OTHER_SESSION_ID = '123e4567-e89b-12d3-a456-426614174001';

export const TRACE_ONE: Trace = {
  id: 'trace-checkout',
  session_id: SESSION_ID,
  name: 'Checkout Trace',
  status: 'FAILED',
  started_at: '2026-03-14T10:00:00.000Z',
  ended_at: '2026-03-14T10:00:02.000Z',
  total_tokens_in: 120,
  total_tokens_out: 80,
  total_cost_usd: 0.12,
  error_count: 2,
};

export const TRACE_TWO: Trace = {
  id: 'trace-latency',
  session_id: OTHER_SESSION_ID,
  name: 'Latency Trace',
  status: 'RUNNING',
  started_at: '2026-03-14T11:00:00.000Z',
  total_tokens_in: 50,
  total_tokens_out: 25,
  total_cost_usd: 0.03,
  error_count: 0,
};

export const TRACE_THREE: Trace = {
  id: 'trace-alpha',
  session_id: SESSION_ID,
  name: 'Alpha Trace',
  status: 'COMPLETED',
  started_at: '2026-03-14T09:00:00.000Z',
  ended_at: '2026-03-14T09:00:03.000Z',
  total_tokens_in: 20,
  total_tokens_out: 10,
  total_cost_usd: 0.01,
  error_count: 0,
};

export const TRACE_ZETA: Trace = {
  ...TRACE_ONE,
  id: 'trace-zeta',
  name: 'Zeta Trace',
  session_id: OTHER_SESSION_ID,
  error_count: 1,
};

export const TRACE_DETAIL: TraceDetail = {
  ...TRACE_ONE,
  trace_id: 'external-trace-checkout',
  user_id: 'user-123',
  tags: ['checkout'],
  environment: 'prod',
  release: '2026.03.14',
  input: { prompt: 'hello' },
  output: { answer: 'world' },
};

let testEntityCounter = 0;

export function resetTestEntityCounter() {
  testEntityCounter = 0;
}

export function createSpan(overrides: Partial<Span> = {}): Span {
  testEntityCounter += 1;
  const spanId = overrides.span_id ?? `span-${testEntityCounter}`;

  return {
    id: overrides.id ?? `uuid-${spanId}`,
    trace_id: overrides.trace_id ?? TRACE_ONE.id,
    span_id: spanId,
    parent_span_id: overrides.parent_span_id,
    name: overrides.name ?? `Span ${testEntityCounter}`,
    kind: overrides.kind ?? 'CHAIN',
    status: overrides.status ?? 'COMPLETED',
    started_at: overrides.started_at ?? '2026-03-14T10:00:00.000Z',
    ended_at: overrides.ended_at,
    tokens_in: overrides.tokens_in,
    tokens_out: overrides.tokens_out,
    cost_usd: overrides.cost_usd,
    latency_ms: overrides.latency_ms ?? 1000,
    error_message: overrides.error_message,
    model: overrides.model,
    provider: overrides.provider,
    input: overrides.input,
    input_truncated: overrides.input_truncated,
    input_original_size_bytes: overrides.input_original_size_bytes,
    input_truncation_reason: overrides.input_truncation_reason,
    output: overrides.output,
    output_truncated: overrides.output_truncated,
    output_original_size_bytes: overrides.output_original_size_bytes,
    output_truncation_reason: overrides.output_truncation_reason,
    metadata: overrides.metadata,
  };
}

export function createTimelineEvent(
  overrides: Partial<TimelineEvent> = {}
): TimelineEvent {
  testEntityCounter += 1;

  return {
    id: overrides.id ?? `event-${testEntityCounter}`,
    trace_id: overrides.trace_id ?? TRACE_ONE.id,
    span_id: overrides.span_id,
    span_name: overrides.span_name,
    event_type: overrides.event_type ?? 'message',
    timestamp: overrides.timestamp ?? '2026-03-14T10:00:00.000Z',
    source: overrides.source ?? 'explicit',
    level: overrides.level,
    sequence: overrides.sequence,
    message: overrides.message,
    payload: overrides.payload,
  };
}
