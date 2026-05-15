const sessionId = '123e4567-e89b-12d3-a456-426614174000';
const otherSessionId = '123e4567-e89b-12d3-a456-426614174001';
const sessionExternalId = 'conv-checkout-123';
const otherSessionExternalId = 'conv-latency-456';

const sessions = [
  {
    id: sessionId,
    external_id: sessionExternalId,
    name: 'Checkout Session',
    user_id: 'user-123',
    trace_count: 2,
    created_at: '2026-03-14T09:00:00.000Z',
  },
  {
    id: otherSessionId,
    external_id: otherSessionExternalId,
    name: 'Latency Session',
    user_id: 'user-456',
    trace_count: 1,
    created_at: '2026-03-14T10:00:00.000Z',
  },
];

const traces = [
  {
    id: 'trace-checkout',
    session_id: sessionId,
    session_external_id: sessionExternalId,
    name: 'Checkout Trace',
    status: 'FAILED',
    started_at: '2026-03-14T10:00:00.000Z',
    ended_at: '2026-03-14T10:00:02.000Z',
    total_tokens_in: 120,
    total_tokens_out: 80,
    total_cost_usd: 0.12,
    error_count: 2,
  },
  {
    id: 'trace-latency',
    session_id: otherSessionId,
    session_external_id: otherSessionExternalId,
    name: 'Latency Trace',
    status: 'RUNNING',
    started_at: '2026-03-14T11:00:00.000Z',
    total_tokens_in: 50,
    total_tokens_out: 25,
    total_cost_usd: 0.03,
    error_count: 0,
  },
  {
    id: 'trace-alpha',
    session_id: sessionId,
    session_external_id: sessionExternalId,
    name: 'Alpha Trace',
    status: 'COMPLETED',
    started_at: '2026-03-14T09:00:00.000Z',
    ended_at: '2026-03-14T09:00:03.000Z',
    total_tokens_in: 20,
    total_tokens_out: 10,
    total_cost_usd: 0.01,
    error_count: 0,
  },
];

const traceDetails = new Map([
  [
    'trace-checkout',
    {
      ...traces[0],
      trace_id: 'external-trace-checkout',
      user_id: 'user-123',
      tags: ['checkout'],
      environment: 'prod',
      release: '2026.03.14',
      input: { prompt: 'hello' },
      output: { answer: 'world' },
    },
  ],
  [
    'trace-latency',
    {
      ...traces[1],
      trace_id: 'external-trace-latency',
      user_id: 'user-456',
      tags: ['latency'],
      input: { prompt: 'latency check' },
      output: { answer: 'running' },
    },
  ],
  [
    'trace-alpha',
    {
      ...traces[2],
      trace_id: 'external-trace-alpha',
      user_id: 'user-123',
      tags: ['alpha'],
      input: { prompt: 'alpha' },
      output: { answer: 'complete' },
    },
  ],
]);

const traceSpans = {
  spans: [
    {
      id: 'span-root',
      trace_id: 'trace-checkout',
      span_id: 'root',
      name: 'Checkout root',
      kind: 'CHAIN',
      status: 'FAILED',
      started_at: traces[0].started_at,
      ended_at: traces[0].ended_at,
      latency_ms: 2000,
      tokens_in: 120,
      tokens_out: 80,
      cost_usd: 0.12,
      metadata: { phase: 'checkout' },
    },
    {
      id: 'span-tool',
      trace_id: 'trace-checkout',
      span_id: 'tool-call',
      parent_span_id: 'root',
      name: 'Checkout tool',
      kind: 'TOOL',
      status: 'FAILED',
      started_at: '2026-03-14T10:00:01.000Z',
      ended_at: '2026-03-14T10:00:02.000Z',
      latency_ms: 1000,
      error_message: 'Gateway timeout',
      metadata: { tool: 'charge-card' },
    },
  ],
};

const traceTimeline = {
  events: [
    {
      id: 'timeline-error',
      trace_id: 'trace-checkout',
      span_id: 'tool-call',
      span_name: 'Checkout tool',
      event_type: 'error',
      timestamp: '2026-03-14T10:00:02.000Z',
      source: 'explicit',
      level: 'error',
      message: 'Gateway timeout',
      payload: { code: 'gateway_timeout' },
    },
  ],
  trace_status: 'FAILED',
  has_more: false,
};

const sessionNarrative = {
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
    {
      id: 'trace-alpha',
      trace_id: 'external-trace-alpha',
      name: 'Alpha Narrative',
      status: 'COMPLETED',
      started_at: '2026-03-14T09:00:00.000Z',
      ended_at: '2026-03-14T09:00:03.000Z',
      duration_ms: 3000,
      error_count: 0,
      total_cost_usd: 0.01,
      total_tokens_in: 20,
      total_tokens_out: 10,
      latest_activity_at: '2026-03-14T09:00:03.000Z',
      semantic_events: [],
      lineage: { type: 'unlinked' },
    },
    {
      id: 'trace-checkout',
      trace_id: 'external-trace-checkout',
      name: 'Checkout Narrative',
      status: 'FAILED',
      started_at: '2026-03-14T10:00:00.000Z',
      ended_at: '2026-03-14T10:00:02.000Z',
      duration_ms: 2000,
      error_count: 2,
      total_cost_usd: 0.12,
      total_tokens_in: 120,
      total_tokens_out: 80,
      latest_activity_at: '2026-03-14T10:00:02.000Z',
      semantic_events: [],
      lineage: { type: 'inferred', parent_trace_id: 'external-trace-alpha' },
    },
  ],
};

const sessionCompare = {
  session: {
    id: sessionId,
    external_id: sessionExternalId,
    name: 'Checkout Session',
  },
  baseline: {
    id: 'trace-alpha',
    trace_id: 'external-trace-alpha',
    name: 'Alpha Narrative',
    status: 'COMPLETED',
    started_at: '2026-03-14T09:00:00.000Z',
    ended_at: '2026-03-14T09:00:03.000Z',
    duration_ms: 3000,
    error_count: 0,
  },
  candidate: {
    id: 'trace-checkout',
    trace_id: 'external-trace-checkout',
    name: 'Checkout Narrative',
    status: 'FAILED',
    started_at: '2026-03-14T10:00:00.000Z',
    ended_at: '2026-03-14T10:00:02.000Z',
    duration_ms: 2000,
    error_count: 2,
  },
  summary: {
    compared_span_count: 2,
    changed_span_count: 1,
    baseline_only_count: 0,
    candidate_only_count: 1,
    semantic_event_delta: 1,
  },
  span_diffs: [
    {
      key: 'root',
      name: 'Checkout root',
      status: 'changed',
      baseline_span: null,
      candidate_span: {
        span_id: 'root',
        name: 'Checkout root',
        kind: 'CHAIN',
        status: 'FAILED',
        latency_ms: 2000,
        tokens_in: 120,
        tokens_out: 80,
        cost_usd: 0.12,
      },
      changed_fields: ['status', 'latency_ms'],
      semantic_groups: [],
      depth: 0,
    },
  ],
};

function json(data, status = 200) {
  return new Response(JSON.stringify(data), {
    status,
    headers: {
      'Content-Type': 'application/json; charset=utf-8',
      'Cache-Control': 'public, max-age=30',
    },
  });
}

function notFound() {
  return json({ code: 'not_found', message: 'Resource not found' }, 404);
}

function filterTraces(url) {
  let filtered = [...traces];
  const session = url.searchParams.get('session_id');
  const status = url.searchParams.get('status');
  const q = url.searchParams.get('q')?.toLowerCase();
  const userId = url.searchParams.get('user_id');
  const hasErrors = url.searchParams.get('has_errors') === 'true';

  if (session) {
    filtered = filtered.filter((trace) => trace.session_id === session);
  }
  if (status) {
    filtered = filtered.filter((trace) => trace.status.toLowerCase() === status);
  }
  if (q) {
    filtered = filtered.filter((trace) => {
      const detail = traceDetails.get(trace.id);
      return (
        trace.name.toLowerCase().includes(q) ||
        detail?.user_id?.toLowerCase().includes(q) ||
        trace.session_external_id?.toLowerCase().includes(q)
      );
    });
  }
  if (userId) {
    filtered = filtered.filter((trace) => traceDetails.get(trace.id)?.user_id === userId);
  }
  if (hasErrors) {
    filtered = filtered.filter((trace) => (trace.error_count ?? 0) > 0);
  }

  const offset = Number(url.searchParams.get('offset') ?? '0');
  const limit = Number(url.searchParams.get('limit') ?? '20');
  return {
    total: filtered.length,
    traces: filtered.slice(offset, offset + limit),
  };
}

function filterSessions(url) {
  let filtered = [...sessions];
  const q = url.searchParams.get('q')?.toLowerCase();
  const userId = url.searchParams.get('user_id');

  if (q) {
    filtered = filtered.filter(
      (session) =>
        session.external_id.toLowerCase().includes(q) ||
        session.name?.toLowerCase().includes(q)
    );
  }
  if (userId) {
    filtered = filtered.filter((session) => session.user_id === userId);
  }

  const offset = Number(url.searchParams.get('offset') ?? '0');
  const limit = Number(url.searchParams.get('limit') ?? '20');
  return {
    total: filtered.length,
    sessions: filtered.slice(offset, offset + limit),
  };
}

function handleApi(url) {
  if (url.pathname === '/api/auth/config') {
    return json({
      enabled: false,
      public_demo_enabled: true,
      public_demo_label: 'Sample data',
    });
  }

  if (url.pathname === '/api/projects') {
    return notFound();
  }

  if (url.pathname === '/api/traces') {
    return json(filterTraces(url));
  }

  if (/^\/api\/traces\/[^/]+\/spans$/.test(url.pathname)) {
    return json(traceSpans);
  }

  if (/^\/api\/traces\/[^/]+\/events$/.test(url.pathname)) {
    return json(traceTimeline);
  }

  if (/^\/api\/traces\/[^/]+$/.test(url.pathname)) {
    const traceId = url.pathname.split('/').at(-1);
    return json(traceDetails.get(traceId) ?? { ...traceDetails.get('trace-checkout'), id: traceId });
  }

  if (url.pathname === '/api/sessions') {
    return json(filterSessions(url));
  }

  if (/^\/api\/sessions\/[^/]+\/narrative$/.test(url.pathname)) {
    return json(sessionNarrative);
  }

  if (/^\/api\/sessions\/[^/]+\/compare$/.test(url.pathname)) {
    return json(sessionCompare);
  }

  if (/^\/api\/sessions\/[^/]+$/.test(url.pathname)) {
    const id = url.pathname.split('/').at(-1);
    const session = sessions.find((item) => item.id === id);
    return session ? json(session) : notFound();
  }

  return notFound();
}

export default {
  async fetch(request, env) {
    const url = new URL(request.url);

    if (url.pathname.startsWith('/api/')) {
      return handleApi(url);
    }

    const response = await env.ASSETS.fetch(request);
    if (response.status !== 404 || !request.headers.get('Accept')?.includes('text/html')) {
      return response;
    }

    return env.ASSETS.fetch(new URL('/index.html', request.url));
  },
};
