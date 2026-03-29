import type {
  Session,
  SessionNarrative,
  SessionNarrativeTrace,
  Span,
  TimelineEvent,
  Trace,
  TraceDetail,
} from '../api/client';

export const SESSION_ID = '123e4567-e89b-12d3-a456-426614174000';
export const OTHER_SESSION_ID = '123e4567-e89b-12d3-a456-426614174001';
export const SESSION_EXTERNAL_ID = 'conv-checkout-123';
export const OTHER_SESSION_EXTERNAL_ID = 'conv-latency-456';

export const SESSION_ONE: Session = {
  id: SESSION_ID,
  external_id: SESSION_EXTERNAL_ID,
  name: 'Checkout Session',
  user_id: 'user-123',
  trace_count: 2,
  created_at: '2026-03-14T09:00:00.000Z',
};

export const SESSION_TWO: Session = {
  id: OTHER_SESSION_ID,
  external_id: OTHER_SESSION_EXTERNAL_ID,
  name: 'Latency Session',
  user_id: 'user-456',
  trace_count: 1,
  created_at: '2026-03-14T10:00:00.000Z',
};

export const TRACE_ONE: Trace = {
  id: 'trace-checkout',
  session_id: SESSION_ID,
  session_external_id: SESSION_EXTERNAL_ID,
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
  session_external_id: OTHER_SESSION_EXTERNAL_ID,
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
  session_external_id: SESSION_EXTERNAL_ID,
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
  session_external_id: OTHER_SESSION_EXTERNAL_ID,
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

export function createSessionNarrativeTrace(
  overrides: Partial<SessionNarrativeTrace> = {}
): SessionNarrativeTrace {
  testEntityCounter += 1;
  const id = overrides.id ?? `narrative-trace-${testEntityCounter}`;

  return {
    id,
    trace_id: overrides.trace_id ?? `external-${id}`,
    name: overrides.name ?? `Narrative Trace ${testEntityCounter}`,
    status: overrides.status ?? 'COMPLETED',
    user_id: overrides.user_id,
    started_at: overrides.started_at ?? '2026-03-14T09:00:00.000Z',
    ended_at: overrides.ended_at,
    duration_ms: overrides.duration_ms ?? 1000,
    error_count: overrides.error_count ?? 0,
    total_cost_usd: overrides.total_cost_usd ?? 0.01,
    total_tokens_in: overrides.total_tokens_in ?? 10,
    total_tokens_out: overrides.total_tokens_out ?? 5,
    latest_activity_at: overrides.latest_activity_at ?? overrides.ended_at ?? '2026-03-14T09:00:01.000Z',
    semantic_events: overrides.semantic_events ?? [],
    lineage: overrides.lineage ?? { type: 'unlinked' },
  };
}

export const SESSION_NARRATIVE: SessionNarrative = {
  summary: {
    total_trace_count: 2,
    returned_trace_count: 2,
    truncated: false,
    running_trace_count: 0,
    completed_trace_count: 1,
    failed_trace_count: 1,
    total_cost_usd: 0.13,
    total_tokens_in: 140,
    total_tokens_out: 90,
    started_at: '2026-03-14T09:00:00.000Z',
    last_activity_at: '2026-03-14T10:00:02.000Z',
    explicit_link_count: 0,
    inferred_link_count: 1,
    unlinked_trace_count: 1,
  },
  traces: [
    createSessionNarrativeTrace({
      id: TRACE_THREE.id,
      trace_id: 'external-trace-alpha',
      name: 'Alpha Narrative',
      status: TRACE_THREE.status,
      started_at: TRACE_THREE.started_at,
      ended_at: TRACE_THREE.ended_at,
      duration_ms: 3000,
      error_count: TRACE_THREE.error_count,
      total_cost_usd: TRACE_THREE.total_cost_usd,
      total_tokens_in: TRACE_THREE.total_tokens_in,
      total_tokens_out: TRACE_THREE.total_tokens_out,
      latest_activity_at: TRACE_THREE.ended_at ?? TRACE_THREE.started_at,
      semantic_events: [
        createTimelineEvent({
          id: 'narrative-event-alpha',
          trace_id: TRACE_THREE.id,
          event_type: 'decision',
          message: 'Pick alpha path',
          timestamp: '2026-03-14T09:00:02.000Z',
        }),
      ],
      lineage: { type: 'unlinked' },
    }),
    createSessionNarrativeTrace({
      id: TRACE_ONE.id,
      trace_id: 'external-trace-checkout',
      name: 'Checkout Narrative',
      status: TRACE_ONE.status,
      started_at: TRACE_ONE.started_at,
      ended_at: TRACE_ONE.ended_at,
      duration_ms: 2000,
      error_count: TRACE_ONE.error_count,
      total_cost_usd: TRACE_ONE.total_cost_usd,
      total_tokens_in: TRACE_ONE.total_tokens_in,
      total_tokens_out: TRACE_ONE.total_tokens_out,
      latest_activity_at: TRACE_ONE.ended_at ?? TRACE_ONE.started_at,
      semantic_events: [
        createTimelineEvent({
          id: 'narrative-event-checkout',
          trace_id: TRACE_ONE.id,
          event_type: 'effect',
          message: 'Called checkout tool',
          timestamp: '2026-03-14T10:00:01.000Z',
        }),
      ],
      lineage: { type: 'inferred', parent_trace_id: 'external-trace-alpha' },
    }),
  ],
};

export const EMPTY_SESSION_NARRATIVE: SessionNarrative = {
  summary: {
    total_trace_count: 0,
    returned_trace_count: 0,
    truncated: false,
    running_trace_count: 0,
    completed_trace_count: 0,
    failed_trace_count: 0,
    total_cost_usd: 0,
    total_tokens_in: 0,
    total_tokens_out: 0,
    started_at: null,
    last_activity_at: null,
    explicit_link_count: 0,
    inferred_link_count: 0,
    unlinked_trace_count: 0,
  },
  traces: [],
};

export const RUNNING_SESSION_NARRATIVE: SessionNarrative = {
  summary: {
    ...SESSION_NARRATIVE.summary,
    running_trace_count: 1,
    completed_trace_count: 0,
    failed_trace_count: 1,
  },
  traces: [
    createSessionNarrativeTrace({
      ...SESSION_NARRATIVE.traces[0],
      status: 'RUNNING',
      ended_at: undefined,
      duration_ms: undefined,
      latest_activity_at: '2026-03-14T09:05:00.000Z',
    }),
    SESSION_NARRATIVE.traces[1],
  ],
};

export const TRUNCATED_SESSION_NARRATIVE: SessionNarrative = {
  summary: {
    ...SESSION_NARRATIVE.summary,
    total_trace_count: 150,
    returned_trace_count: 100,
    truncated: true,
  },
  traces: SESSION_NARRATIVE.traces,
};

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
