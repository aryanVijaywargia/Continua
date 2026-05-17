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

const traceSpansByTrace = new Map([
  [
    'trace-checkout',
    {
      spans: [
        {
          id: 'span-checkout-root',
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
          input: { cart_id: 'cart-789', items: 3, total_usd: 42.5 },
          output: { ok: false, error: 'Gateway timeout' },
          metadata: { phase: 'checkout' },
        },
        {
          id: 'span-checkout-tool',
          trace_id: 'trace-checkout',
          span_id: 'tool-call',
          parent_span_id: 'root',
          name: 'Checkout tool',
          kind: 'TOOL',
          status: 'FAILED',
          started_at: '2026-03-14T10:00:01.000Z',
          ended_at: '2026-03-14T10:00:02.000Z',
          latency_ms: 1000,
          input: { card_last4: '4242', amount_usd: 42.5 },
          output: { ok: false, code: 'gateway_timeout' },
          error_message: 'Gateway timeout',
          metadata: { tool: 'charge-card' },
        },
      ],
    },
  ],
  [
    'trace-alpha',
    {
      spans: [
        {
          id: 'span-alpha-root',
          trace_id: 'trace-alpha',
          span_id: 'alpha-root',
          name: 'Alpha root',
          kind: 'CHAIN',
          status: 'COMPLETED',
          started_at: traces[2].started_at,
          ended_at: traces[2].ended_at,
          latency_ms: 3000,
          tokens_in: 20,
          tokens_out: 10,
          cost_usd: 0.01,
          input: { prompt: 'alpha' },
          output: { answer: 'complete' },
          metadata: { phase: 'alpha' },
        },
        {
          id: 'span-alpha-llm',
          trace_id: 'trace-alpha',
          span_id: 'alpha-llm',
          parent_span_id: 'alpha-root',
          name: 'Alpha LLM call',
          kind: 'LLM',
          status: 'COMPLETED',
          started_at: '2026-03-14T09:00:00.500Z',
          ended_at: '2026-03-14T09:00:02.500Z',
          latency_ms: 2000,
          tokens_in: 20,
          tokens_out: 10,
          cost_usd: 0.01,
          input: { messages: [{ role: 'user', content: 'alpha' }] },
          output: { content: 'complete' },
          metadata: { model: 'gpt-4o-mini', provider: 'openai' },
        },
      ],
    },
  ],
  [
    'trace-latency',
    {
      spans: [
        {
          id: 'span-latency-root',
          trace_id: 'trace-latency',
          span_id: 'latency-root',
          name: 'Latency root',
          kind: 'CHAIN',
          status: 'STARTED',
          started_at: traces[1].started_at,
          latency_ms: null,
          tokens_in: 50,
          tokens_out: 25,
          cost_usd: 0.03,
          input: { prompt: 'latency check' },
          metadata: { phase: 'latency' },
        },
      ],
    },
  ],
]);

const traceTimelinesByTrace = new Map([
  [
    'trace-checkout',
    {
      events: [
        {
          id: 'timeline-checkout-error',
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
    },
  ],
  [
    'trace-alpha',
    {
      events: [
        {
          id: 'timeline-alpha-complete',
          trace_id: 'trace-alpha',
          span_id: 'alpha-llm',
          span_name: 'Alpha LLM call',
          event_type: 'log',
          timestamp: '2026-03-14T09:00:02.500Z',
          source: 'explicit',
          level: 'info',
          message: 'LLM call completed',
          payload: { tokens_out: 10 },
        },
      ],
      trace_status: 'COMPLETED',
      has_more: false,
    },
  ],
  [
    'trace-latency',
    {
      events: [],
      trace_status: 'RUNNING',
      has_more: false,
    },
  ],
]);

const emptySpans = { spans: [] };
const emptyTimeline = { events: [], trace_status: 'RUNNING', has_more: false };

const sessionNarrativesBySession = new Map([
  [
    sessionId,
    {
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
    },
  ],
  [
    otherSessionId,
    {
      summary: {
        total_trace_count: 1,
        returned_trace_count: 1,
        truncated: false,
        running_trace_count: 1,
        completed_trace_count: 0,
        failed_trace_count: 0,
        total_cost_usd: 0.03,
        total_tokens_in: 50,
        total_tokens_out: 25,
        started_at: '2026-03-14T11:00:00.000Z',
        last_activity_at: '2026-03-14T11:00:00.000Z',
        explicit_link_count: 0,
        inferred_link_count: 0,
        unlinked_trace_count: 1,
      },
      traces: [
        {
          id: 'trace-latency',
          trace_id: 'external-trace-latency',
          name: 'Latency Narrative',
          status: 'RUNNING',
          started_at: '2026-03-14T11:00:00.000Z',
          duration_ms: null,
          error_count: 0,
          total_cost_usd: 0.03,
          total_tokens_in: 50,
          total_tokens_out: 25,
          latest_activity_at: '2026-03-14T11:00:00.000Z',
          semantic_events: [],
          lineage: { type: 'unlinked' },
        },
      ],
    },
  ],
]);

const sessionComparesBySession = new Map([
  [
    sessionId,
    {
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
    },
  ],
]);

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
    const traceId = url.pathname.split('/').at(-2);
    if (!traceDetails.has(traceId)) {
      return notFound();
    }
    return json(traceSpansByTrace.get(traceId) ?? emptySpans);
  }

  if (/^\/api\/traces\/[^/]+\/events$/.test(url.pathname)) {
    const traceId = url.pathname.split('/').at(-2);
    const detail = traceDetails.get(traceId);
    if (!detail) {
      return notFound();
    }
    const timeline =
      traceTimelinesByTrace.get(traceId) ??
      { ...emptyTimeline, trace_status: detail.status };
    return json(timeline);
  }

  if (/^\/api\/traces\/[^/]+$/.test(url.pathname)) {
    const traceId = url.pathname.split('/').at(-1);
    const detail = traceDetails.get(traceId);
    if (!detail) {
      return notFound();
    }
    return json(detail);
  }

  if (url.pathname === '/api/sessions') {
    return json(filterSessions(url));
  }

  if (/^\/api\/sessions\/[^/]+\/narrative$/.test(url.pathname)) {
    const id = url.pathname.split('/').at(-2);
    const narrative = sessionNarrativesBySession.get(id);
    return narrative ? json(narrative) : notFound();
  }

  if (/^\/api\/sessions\/[^/]+\/compare$/.test(url.pathname)) {
    const id = url.pathname.split('/').at(-2);
    const compare = sessionComparesBySession.get(id);
    return compare ? json(compare) : notFound();
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

    // Proxy /docs/* to the Mintlify-hosted docs site when MINTLIFY_HOST is configured.
    // Set MINTLIFY_HOST (e.g. "continua.mintlify.dev") in Cloudflare Pages env vars to activate.
    const mintlifyHost = env?.MINTLIFY_HOST;
    if (mintlifyHost && (url.pathname === '/docs' || url.pathname.startsWith('/docs/'))) {
      const proxyUrl = new URL(url.pathname + url.search, `https://${mintlifyHost}`);
      const proxyRequest = new Request(proxyUrl, request);
      proxyRequest.headers.set('Host', mintlifyHost);
      proxyRequest.headers.set('X-Forwarded-Host', url.host);
      proxyRequest.headers.set('X-Forwarded-Proto', 'https');
      return fetch(proxyRequest);
    }

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
