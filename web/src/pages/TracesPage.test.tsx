import { act } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { RouterProvider, createMemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  clearApiKey,
  setApiKey,
  type Span,
  type TimelineEvent,
  type Trace,
  type TraceDetail,
} from '../api/client';
import { localDateToISOEnd, localDateToISOStart } from '../utils/tracesSearchParams';
import { TraceDetailPage } from './TraceDetailPage';
import { TracesPage } from './TracesPage';

const SESSION_ID = '123e4567-e89b-12d3-a456-426614174000';
const OTHER_SESSION_ID = '123e4567-e89b-12d3-a456-426614174001';

const TRACE_ONE: Trace = {
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

const TRACE_TWO: Trace = {
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

const TRACE_THREE: Trace = {
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

const TRACE_ZETA: Trace = {
  ...TRACE_ONE,
  id: 'trace-zeta',
  name: 'Zeta Trace',
  session_id: OTHER_SESSION_ID,
  error_count: 1,
};

const TRACE_DETAIL: TraceDetail = {
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

type RequestInput = string | URL | Request;

type JsonHandler = (url: URL) => Promise<Response> | Response;

let fetchMock: ReturnType<typeof vi.fn>;

function jsonResponse(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(data), {
    status,
    headers: {
      'Content-Type': 'application/json',
    },
  });
}

function buildFetchHandler({
  list,
  detail,
  spans,
  timeline,
}: {
  list?: JsonHandler;
  detail?: JsonHandler;
  spans?: JsonHandler;
  timeline?: JsonHandler;
} = {}) {
  return async (input: RequestInput) => {
    const url = new URL(readRequestUrl(input), 'http://localhost');

    if (url.pathname === '/api/traces') {
      return (
        list?.(url) ??
        jsonResponse({
          traces: [TRACE_ONE, TRACE_TWO],
          total: 2,
        })
      );
    }

    if (/^\/api\/traces\/[^/]+\/spans$/.test(url.pathname)) {
      return spans?.(url) ?? jsonResponse({ spans: [] });
    }

    if (/^\/api\/traces\/[^/]+\/events$/.test(url.pathname)) {
      return (
        timeline?.(url) ??
        jsonResponse({
          events: [],
          trace_status: 'COMPLETED',
          has_more: false,
        })
      );
    }

    if (/^\/api\/traces\/[^/]+$/.test(url.pathname)) {
      return detail?.(url) ?? jsonResponse(TRACE_DETAIL);
    }

    throw new Error(`Unhandled request: ${url.pathname}${url.search}`);
  };
}

function createSpan(overrides: Partial<Span> = {}): Span {
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

function createTimelineEvent(overrides: Partial<TimelineEvent> = {}): TimelineEvent {
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

function readRequestUrl(input: RequestInput): string {
  if (typeof input === 'string') {
    return input;
  }

  if (input instanceof URL) {
    return input.toString();
  }

  return input.url;
}

function getTraceListRequests(): URL[] {
  return fetchMock.mock.calls
    .map(([input]) => new URL(readRequestUrl(input as RequestInput), 'http://localhost'))
    .filter((url) => url.pathname === '/api/traces');
}

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
}

function renderTraceRoutes(
  initialEntries: Array<string | { pathname: string; state?: unknown }>,
  options: { initialIndex?: number } = {}
) {
  const queryClient = createQueryClient();
  const router = createMemoryRouter(
    [
      { path: '/traces', element: <TracesPage /> },
      { path: '/traces/:id', element: <TraceDetailPage /> },
    ],
    {
      initialEntries,
      initialIndex: options.initialIndex,
    }
  );

  const view = render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
    </QueryClientProvider>
  );

  return { ...view, queryClient, router };
}

function mockClipboard() {
  const writeText = vi.fn().mockResolvedValue(undefined);

  Object.defineProperty(window.navigator, 'clipboard', {
    configurable: true,
    value: { writeText },
  });

  return writeText;
}

async function waitForListFetch(search: string) {
  await waitFor(() => {
    const requests = getTraceListRequests();
    expect(requests.at(-1)?.search).toBe(search);
  });
}

async function waitForDebounce() {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 350));
  });
}

function createDeferredResponse() {
  let resolveResponse: (response: Response) => void = () => {};
  const promise = new Promise<Response>((resolve) => {
    resolveResponse = resolve;
  });

  return {
    promise,
    resolve: resolveResponse,
  };
}

beforeEach(() => {
  testEntityCounter = 0;
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
  localStorage.clear();
  setApiKey('test-key');
});

afterEach(() => {
  clearApiKey();
  localStorage.clear();
  vi.useRealTimers();
  vi.unstubAllGlobals();
  Object.defineProperty(window.navigator, 'clipboard', {
    configurable: true,
    value: undefined,
  });
});

describe('TracesPage', () => {
  it('loads the default trace list from /traces', async () => {
    fetchMock.mockImplementation(buildFetchHandler());

    renderTraceRoutes(['/traces']);

    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();
    expect(
      screen.getByText('Search names, user IDs, and matching span names.')
    ).toBeInTheDocument();
    await waitForListFetch('?limit=20');
    expect(screen.queryByRole('button', { name: 'Clear all' })).not.toBeInTheDocument();
  });

  it('pre-populates controls from a deep link and issues the filtered request', async () => {
    const start = localDateToISOStart('2026-03-10');
    const end = localDateToISOEnd('2026-03-12');
    fetchMock.mockImplementation(buildFetchHandler());

    renderTraceRoutes([
      `/traces?session_id=${SESSION_ID}&q=checkout&status=failed&start_time_from=${encodeURIComponent(
        start
      )}&start_time_to=${encodeURIComponent(end)}&user_id=user-123&has_errors=true&min_duration_ms=1200`,
    ]);

    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();
    await waitForListFetch(
      `?limit=20&session_id=${SESSION_ID}&q=checkout&status=failed&start_time_from=${encodeURIComponent(
        start
      )}&start_time_to=${encodeURIComponent(
        end
      )}&user_id=user-123&has_errors=true&min_duration_ms=1200`
    );

    expect(screen.getByLabelText('Search')).toHaveValue('checkout');
    expect(screen.getByLabelText('Status')).toHaveValue('failed');
    expect(screen.getByLabelText('Start Date')).toHaveValue('2026-03-10');
    expect(screen.getByLabelText('End Date')).toHaveValue('2026-03-12');
    expect(screen.getByLabelText('User ID')).toHaveValue('user-123');
    expect(screen.getByLabelText('Min Duration (ms)')).toHaveValue(1200);
    expect(screen.getByLabelText('Only show traces with errors')).toBeChecked();
  });

  it('commits text filters after debounce and immediately on Enter', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(buildFetchHandler());

    renderTraceRoutes(['/traces']);
    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();

    fetchMock.mockClear();
    const searchInput = screen.getByLabelText('Search');

    await user.type(searchInput, ' latency');
    expect(getTraceListRequests()).toHaveLength(0);

    await waitForDebounce();
    await waitForListFetch('?limit=20&q=latency');

    fetchMock.mockClear();
    await user.clear(searchInput);
    await user.type(searchInput, 'errors');
    await user.keyboard('{Enter}');
    await waitForListFetch('?limit=20&q=errors');
  });

  it('does not commit text filters on blur before debounce elapses', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(buildFetchHandler());

    const { router } = renderTraceRoutes(['/traces']);
    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();

    fetchMock.mockClear();
    const userIdInput = screen.getByLabelText('User ID');

    await user.type(userIdInput, 'owner-42');
    fireEvent.blur(userIdInput);
    await waitForDebounce();

    expect(getTraceListRequests()).toHaveLength(0);
    expect(router.state.location.search).toBe('');
  });

  it('commits min duration after debounce and on Enter', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(buildFetchHandler());

    renderTraceRoutes(['/traces']);
    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();

    fetchMock.mockClear();
    const minDurationInput = screen.getByLabelText('Min Duration (ms)');

    await user.type(minDurationInput, '1500');
    await waitForDebounce();
    await waitForListFetch('?limit=20&min_duration_ms=1500');

    fetchMock.mockClear();
    await user.clear(minDurationInput);
    await user.type(minDurationInput, '2400');
    await user.keyboard('{Enter}');
    await waitForListFetch('?limit=20&min_duration_ms=2400');
  });

  it('does not commit min duration on blur before debounce elapses', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(buildFetchHandler());

    const { router } = renderTraceRoutes(['/traces']);
    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();

    fetchMock.mockClear();
    const minDurationInput = screen.getByLabelText('Min Duration (ms)');

    await user.type(minDurationInput, '1500');
    fireEvent.blur(minDurationInput);
    await waitForDebounce();

    expect(getTraceListRequests()).toHaveLength(0);
    expect(router.state.location.search).toBe('');
  });

  it('composes filters and resets pagination when a filter changes', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(
      buildFetchHandler({
        list: (url) =>
          jsonResponse({
            traces: url.searchParams.get('offset') === '20' ? [TRACE_TWO] : [TRACE_ONE],
            total: 42,
          }),
      })
    );

    const { router } = renderTraceRoutes(['/traces']);
    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Next page' }));
    await waitForListFetch('?limit=20&offset=20');

    await user.selectOptions(screen.getByLabelText('Status'), 'running');
    await waitForListFetch('?limit=20&status=running');
    await waitFor(() => {
      expect(router.state.location.search).toBe('?status=running');
    });
  });

  it('repairs stale offsets with replace and refetches the last valid page', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        list: (url) => {
          const offset = url.searchParams.get('offset');

          if (offset === '40') {
            return jsonResponse({ traces: [], total: 21 });
          }
          if (offset === '20') {
            return jsonResponse({ traces: [TRACE_TWO], total: 21 });
          }

          return jsonResponse({ traces: [TRACE_ONE], total: 21 });
        },
      })
    );

    const { router } = renderTraceRoutes(['/traces?offset=40']);
    expect(await screen.findByText('Latency Trace')).toBeInTheDocument();

    await waitFor(() => {
      expect(getTraceListRequests().map((request) => request.search)).toEqual([
        '?limit=20&offset=40',
        '?limit=20&offset=20',
      ]);
    });
    expect(router.state.location.search).toBe('?offset=20');
  });

  it('validates inverted date ranges and suppresses the fetch until corrected', async () => {
    fetchMock.mockImplementation(buildFetchHandler());

    renderTraceRoutes(['/traces']);
    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();

    fetchMock.mockClear();

    fireEvent.change(screen.getByLabelText('End Date'), {
      target: { value: '2026-03-14' },
    });
    await waitForListFetch(
      `?limit=20&start_time_to=${encodeURIComponent(localDateToISOEnd('2026-03-14'))}`
    );

    fetchMock.mockClear();

    fireEvent.change(screen.getByLabelText('Start Date'), {
      target: { value: '2026-03-15' },
    });

    expect(
      await screen.findByText('Start date must be on or before the end date.')
    ).toBeInTheDocument();
    expect(screen.getByText('Fix the date range to load traces.')).toBeInTheDocument();
    expect(getTraceListRequests()).toHaveLength(0);
  });

  it('honors session_id filters, clears chips individually, and clears all filters', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(buildFetchHandler());

    const { router } = renderTraceRoutes([
      `/traces?offset=20&session_id=${SESSION_ID}&q=checkout&has_errors=true`,
    ]);
    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();
    await waitForListFetch(
      `?limit=20&offset=20&session_id=${SESSION_ID}&q=checkout&has_errors=true`
    );

    expect(screen.getByText(SESSION_ID)).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Clear Search filter' }));
    await waitForListFetch(`?limit=20&session_id=${SESSION_ID}&has_errors=true`);

    await user.click(screen.getByRole('button', { name: 'Clear Session filter' }));
    await waitForListFetch('?limit=20&has_errors=true');

    await user.click(screen.getByRole('button', { name: 'Clear all' }));
    await waitForListFetch('?limit=20');
    await waitFor(() => {
      expect(router.state.location.search).toBe('');
    });
  });

  it('shows an error banner and retries without disabling the controls', async () => {
    const user = userEvent.setup();
    let shouldFail = true;

    fetchMock.mockImplementation(
      buildFetchHandler({
        list: () =>
          shouldFail
            ? jsonResponse({ code: 'error', message: 'backend exploded' }, 500)
            : jsonResponse({ traces: [TRACE_ONE], total: 1 }),
      })
    );

    renderTraceRoutes(['/traces']);

    expect(await screen.findByText('Could not load traces')).toBeInTheDocument();
    expect(screen.getByLabelText('Search')).toBeEnabled();

    shouldFail = false;
    await user.click(screen.getByRole('button', { name: 'Retry' }));

    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();
    await waitFor(() => {
      expect(getTraceListRequests()).toHaveLength(2);
    });
  });

  it('keeps previous rows visible and shows updating while a refetch is pending', async () => {
    const user = userEvent.setup();
    const deferred = createDeferredResponse();

    fetchMock.mockImplementation(
      buildFetchHandler({
        list: (url) =>
          url.searchParams.get('status') === 'running'
            ? deferred.promise
            : jsonResponse({ traces: [TRACE_ONE], total: 1 }),
      })
    );

    renderTraceRoutes(['/traces']);
    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();

    await user.selectOptions(screen.getByLabelText('Status'), 'running');

    expect(screen.getByText('Checkout Trace')).toBeInTheDocument();
    expect(screen.getByText('Updating...')).toBeInTheDocument();

    deferred.resolve(jsonResponse({ traces: [TRACE_TWO], total: 1 }));

    expect(await screen.findByText('Latency Trace')).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.queryByText('Updating...')).not.toBeInTheDocument();
    });
  });

  it('renders distinct empty states for onboarding and filtered no-match results', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        list: () => jsonResponse({ traces: [], total: 0 }),
      })
    );

    const firstView = renderTraceRoutes(['/traces']);
    expect(await screen.findByText('No traces yet')).toBeInTheDocument();
    firstView.unmount();

    renderTraceRoutes(['/traces?q=missing']);
    expect(await screen.findByText('No matching traces')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Clear all' })).toBeInTheDocument();
  });

  it('preserves API response ordering when rendering rows', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        list: () =>
          jsonResponse({
            traces: [TRACE_ZETA, TRACE_THREE],
            total: 2,
          }),
      })
    );

    renderTraceRoutes(['/traces?q=trace']);
    expect(await screen.findByText('Zeta Trace')).toBeInTheDocument();

    const links = within(screen.getByRole('table')).getAllByRole('link', {
      name: /(Zeta|Alpha) Trace/,
    });
    expect(links.map((link) => link.textContent)).toEqual([
      'Zeta Trace',
      'Alpha Trace',
    ]);
  });

  it('uses the filtered return URL in detail and falls back to /traces otherwise', async () => {
    fetchMock.mockImplementation(buildFetchHandler());

    const withState = renderTraceRoutes([
      {
        pathname: `/traces/${TRACE_ONE.id}`,
        state: { returnTo: '/traces?q=checkout&status=failed' },
      },
    ]);

    expect(await screen.findByText('Trace Context')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '← Traces' })).toHaveAttribute(
      'href',
      '/traces?q=checkout&status=failed'
    );
    withState.unmount();

    const unrelatedState = renderTraceRoutes([
      {
        pathname: `/traces/${TRACE_ONE.id}`,
        state: { returnTo: '/sessions' },
      },
    ]);
    expect(await screen.findByText('Trace Context')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '← Traces' })).toHaveAttribute(
      'href',
      '/traces'
    );
    unrelatedState.unmount();

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
    expect(await screen.findByText('Trace Context')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '← Traces' })).toHaveAttribute(
      'href',
      '/traces'
    );
  });

  it('keeps controls and chip dismiss buttons keyboard reachable with visible focus classes', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(buildFetchHandler());

    renderTraceRoutes([
      `/traces?q=checkout&has_errors=true&session_id=${SESSION_ID}`,
    ]);
    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();

    const searchInput = screen.getByLabelText('Search');
    const statusSelect = screen.getByLabelText('Status');
    const startDate = screen.getByLabelText('Start Date');
    const endDate = screen.getByLabelText('End Date');
    const userIdInput = screen.getByLabelText('User ID');
    const minDurationInput = screen.getByLabelText('Min Duration (ms)');
    const hasErrorsCheckbox = screen.getByLabelText('Only show traces with errors');
    const clearSearch = screen.getByRole('button', { name: 'Clear Search filter' });
    const clearErrors = screen.getByRole('button', { name: 'Clear Errors filter' });
    const clearSession = screen.getByRole('button', { name: 'Clear Session filter' });
    const clearAll = screen.getByRole('button', { name: 'Clear all' });

    await user.tab();
    expect(searchInput).toHaveFocus();
    await user.tab();
    expect(statusSelect).toHaveFocus();
    await user.tab();
    expect(startDate).toHaveFocus();
    await user.tab();
    expect(endDate).toHaveFocus();
    await user.tab();
    expect(userIdInput).toHaveFocus();
    await user.tab();
    expect(minDurationInput).toHaveFocus();
    await user.tab();
    expect(hasErrorsCheckbox).toHaveFocus();
    await user.tab();
    expect(clearSearch).toHaveFocus();
    await user.tab();
    expect(clearErrors).toHaveFocus();
    await user.tab();
    expect(clearSession).toHaveFocus();
    await user.tab();
    expect(clearAll).toHaveFocus();

    expect(searchInput.className).toContain('focus:ring-2');
    expect(clearSearch.className).toContain('focus:ring-2');
    expect(clearAll.className).toContain('focus:ring-2');
  });

  it('rehydrates draft text inputs on back and forward navigation', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(buildFetchHandler());

    const { router } = renderTraceRoutes(['/traces?q=alpha']);
    const searchInput = (await screen.findByLabelText('Search')) as HTMLInputElement;
    expect(searchInput.value).toBe('alpha');

    await user.clear(searchInput);
    await user.type(searchInput, 'beta');
    await waitForDebounce();
    await waitFor(() => {
      expect(router.state.location.search).toBe('?q=beta');
    });

    await act(async () => {
      await router.navigate(-1);
    });
    await waitFor(() => {
      expect(searchInput.value).toBe('alpha');
    });

    await act(async () => {
      await router.navigate(1);
    });
    await waitFor(() => {
      expect(searchInput.value).toBe('beta');
    });
  });

  it('normalizes malformed session_id params away before fetching', async () => {
    fetchMock.mockImplementation(buildFetchHandler());

    const { router } = renderTraceRoutes(['/traces?session_id=not-a-uuid']);
    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();

    await waitForListFetch('?limit=20');
    await waitFor(() => {
      expect(router.state.location.search).toBe('');
    });
    expect(screen.queryByText('Session:')).not.toBeInTheDocument();
  });

  it('normalizes whitespace-only q params away without rendering a ghost chip', async () => {
    fetchMock.mockImplementation(buildFetchHandler());

    const { router } = renderTraceRoutes(['/traces?q=%20%20%20']);
    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();

    await waitForListFetch('?limit=20');
    await waitFor(() => {
      expect(router.state.location.search).toBe('');
    });
    expect(screen.getByLabelText('Search')).toHaveValue('');
    expect(
      screen.queryByRole('button', { name: 'Clear Search filter' })
    ).not.toBeInTheDocument();
  });
});

describe('TraceDetailPage', () => {
  it('auto-selects the primary failed span, highlights the failure path, and reveals it from the summary jump action', async () => {
    const user = userEvent.setup();
    const rootSpan = createSpan({
      span_id: 'root-agent',
      name: 'Root agent',
      status: 'COMPLETED',
      ended_at: '2026-03-14T10:00:05.000Z',
    });
    const primaryFailedSpan = createSpan({
      span_id: 'failed-tool',
      name: 'Failed tool',
      kind: 'TOOL',
      parent_span_id: 'root-agent',
      status: 'FAILED',
      started_at: '2026-03-14T10:00:05.000Z',
      ended_at: '2026-03-14T10:00:10.000Z',
      error_message: 'Primary failure preview',
    });
    const siblingFailedSpan = createSpan({
      span_id: 'sibling-failure',
      name: 'Sibling failure',
      parent_span_id: 'root-agent',
      status: 'FAILED',
      started_at: '2026-03-14T10:00:12.000Z',
      ended_at: '2026-03-14T10:00:15.000Z',
      error_message: 'Secondary failure preview',
    });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(TRACE_DETAIL),
        spans: () =>
          jsonResponse({
            spans: [rootSpan, primaryFailedSpan, siblingFailedSpan],
          }),
        timeline: () =>
          jsonResponse({
            events: [
              createTimelineEvent({
                span_id: 'failed-tool',
                span_name: 'Failed tool',
                event_type: 'error',
                timestamp: '2026-03-14T10:00:10.000Z',
                message: 'Primary failure preview',
              }),
              createTimelineEvent({
                span_id: 'sibling-failure',
                span_name: 'Sibling failure',
                event_type: 'span_failed',
                timestamp: '2026-03-14T10:00:15.000Z',
                message: 'Sibling failure synthetic event',
              }),
            ],
            trace_status: 'FAILED',
            has_more: false,
          }),
      })
    );

    const view = renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(
      await screen.findByRole('heading', { name: 'Failure Summary' })
    ).toBeInTheDocument();
    expect(view.router.state.location.search).toBe('');

    const detailBreadcrumb = screen.getByLabelText('Span breadcrumb');
    expect(within(detailBreadcrumb).getByText('Failed tool')).toBeInTheDocument();
    expect(
      within(detailBreadcrumb).getByRole('button', {
        name: 'Select ancestor span Root agent',
      })
    ).toBeInTheDocument();

    const rootRow = screen.getByRole('button', {
      name: 'Select span Root agent',
    });
    const primaryRow = screen.getByRole('button', {
      name: 'Select span Failed tool',
    });
    const siblingRow = screen.getByRole('button', {
      name: 'Select span Sibling failure',
    });

    expect(rootRow).toHaveClass('bg-amber-50');
    expect(rootRow).toHaveAttribute('aria-pressed', 'false');
    expect(within(rootRow).getByText('Failure path')).toBeInTheDocument();
    expect(primaryRow).toHaveClass('bg-blue-50');
    expect(primaryRow).toHaveAttribute('aria-pressed', 'true');
    expect(within(primaryRow).getByText('Selected')).toBeInTheDocument();
    expect(within(primaryRow).getByText('Failure path')).toBeInTheDocument();
    expect(siblingRow).toHaveClass('bg-red-50/80');
    expect(
      within(siblingRow).queryByText('Failure path')
    ).not.toBeInTheDocument();

    await user.click(rootRow);
    await user.click(
      screen.getByRole('button', { name: 'Collapse span Root agent' })
    );

    expect(
      screen.queryByRole('button', { name: 'Select span Failed tool' })
    ).not.toBeInTheDocument();

    await user.click(
      screen.getByRole('button', {
        name: 'Jump to failed span Failed tool',
      })
    );

    expect(
      await screen.findByRole('button', { name: 'Select span Failed tool' })
    ).toBeInTheDocument();
    expect(
      within(screen.getByLabelText('Span breadcrumb')).getByText('Failed tool')
    ).toBeInTheDocument();
  });

  it('selects a valid span from the URL and does not let failure-first auto-selection override it', async () => {
    const rootSpan = createSpan({
      span_id: 'url-root',
      name: 'URL root',
      status: 'COMPLETED',
    });
    const requestedSpan = createSpan({
      span_id: 'url-child',
      name: 'URL child',
      parent_span_id: 'url-root',
      status: 'COMPLETED',
    });
    const failedSpan = createSpan({
      span_id: 'url-failed',
      name: 'URL failed',
      parent_span_id: 'url-root',
      status: 'FAILED',
      ended_at: '2026-03-14T10:00:10.000Z',
      error_message: 'Failure preview',
    });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(TRACE_DETAIL),
        spans: () => jsonResponse({ spans: [rootSpan, requestedSpan, failedSpan] }),
        timeline: () =>
          jsonResponse({
            events: [
              createTimelineEvent({
                id: 'url-error',
                span_id: 'url-failed',
                span_name: 'URL failed',
                event_type: 'error',
                message: 'Failure preview',
                timestamp: '2026-03-14T10:00:10.000Z',
              }),
            ],
            trace_status: 'FAILED',
            has_more: false,
          }),
      })
    );

    const view = renderTraceRoutes([`/traces/${TRACE_ONE.id}?span=url-child`]);

    expect(
      await screen.findByRole('button', { name: 'Select span URL child' })
    ).toHaveAttribute('aria-pressed', 'true');
    expect(
      within(screen.getByLabelText('Span breadcrumb')).getByText('URL child')
    ).toBeInTheDocument();
    expect(
      within(screen.getByLabelText('Span breadcrumb')).queryByText('URL failed')
    ).not.toBeInTheDocument();
    expect(view.router.state.location.search).toBe('?span=url-child');
  });

  it('removes unknown span params while preserving unrelated params and re-running failure-first selection', async () => {
    const rootSpan = createSpan({
      span_id: 'cleanup-root',
      name: 'Cleanup root',
      status: 'COMPLETED',
    });
    const failedSpan = createSpan({
      span_id: 'cleanup-failed',
      name: 'Cleanup failed',
      parent_span_id: 'cleanup-root',
      status: 'FAILED',
      ended_at: '2026-03-14T10:00:10.000Z',
      error_message: 'Cleanup failure',
    });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(TRACE_DETAIL),
        spans: () => jsonResponse({ spans: [rootSpan, failedSpan] }),
        timeline: () =>
          jsonResponse({
            events: [
              createTimelineEvent({
                id: 'cleanup-error',
                span_id: 'cleanup-failed',
                span_name: 'Cleanup failed',
                event_type: 'error',
                message: 'Cleanup failure',
                timestamp: '2026-03-14T10:00:10.000Z',
              }),
            ],
            trace_status: 'FAILED',
            has_more: false,
          }),
      })
    );

    const view = renderTraceRoutes([
      `/traces/${TRACE_ONE.id}?debug=true&span=missing-span`,
    ]);

    await waitFor(() => {
      expect(view.router.state.location.search).toBe('?debug=true');
    });
    await waitFor(() => {
      expect(
        screen.getByRole('button', { name: 'Select span Cleanup failed' })
      ).toHaveAttribute('aria-pressed', 'true');
    });
  });

  it('reacts to browser back and forward span changes while staying on the same trace', async () => {
    const rootSpan = createSpan({
      span_id: 'history-root',
      name: 'History root',
      status: 'COMPLETED',
    });
    const alphaSpan = createSpan({
      span_id: 'history-alpha',
      name: 'History alpha',
      parent_span_id: 'history-root',
      status: 'COMPLETED',
    });
    const betaSpan = createSpan({
      span_id: 'history-beta',
      name: 'History beta',
      parent_span_id: 'history-root',
      status: 'COMPLETED',
    });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () =>
          jsonResponse({
            ...TRACE_DETAIL,
            status: 'COMPLETED',
            error_count: 0,
          }),
        spans: () => jsonResponse({ spans: [rootSpan, alphaSpan, betaSpan] }),
        timeline: () =>
          jsonResponse({
            events: [],
            trace_status: 'COMPLETED',
            has_more: false,
          }),
      })
    );

    const view = renderTraceRoutes(
      [
        `/traces/${TRACE_ONE.id}?span=history-alpha`,
        `/traces/${TRACE_ONE.id}?span=history-beta`,
      ],
      { initialIndex: 1 }
    );

    expect(
      await screen.findByRole('button', { name: 'Select span History beta' })
    ).toHaveAttribute('aria-pressed', 'true');

    await act(async () => {
      await view.router.navigate(-1);
    });

    await waitFor(() => {
      expect(
        within(screen.getByLabelText('Span breadcrumb')).getByText('History alpha')
      ).toBeInTheDocument();
    });
    expect(view.router.state.location.search).toBe('?span=history-alpha');
  });

  it('re-runs auto-selection when browser back removes the span param', async () => {
    const rootSpan = createSpan({
      span_id: 'back-root',
      name: 'Back root',
      status: 'COMPLETED',
    });
    const manualSpan = createSpan({
      span_id: 'back-manual',
      name: 'Back manual',
      parent_span_id: 'back-root',
      status: 'COMPLETED',
    });
    const failedSpan = createSpan({
      span_id: 'back-failed',
      name: 'Back failed',
      parent_span_id: 'back-root',
      status: 'FAILED',
      ended_at: '2026-03-14T10:00:10.000Z',
      error_message: 'Back failure',
    });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(TRACE_DETAIL),
        spans: () => jsonResponse({ spans: [rootSpan, manualSpan, failedSpan] }),
        timeline: () =>
          jsonResponse({
            events: [
              createTimelineEvent({
                id: 'back-error',
                span_id: 'back-failed',
                span_name: 'Back failed',
                event_type: 'error',
                message: 'Back failure',
                timestamp: '2026-03-14T10:00:10.000Z',
              }),
            ],
            trace_status: 'FAILED',
            has_more: false,
          }),
      })
    );

    const view = renderTraceRoutes(
      [
        `/traces/${TRACE_ONE.id}`,
        `/traces/${TRACE_ONE.id}?span=back-manual`,
      ],
      { initialIndex: 1 }
    );

    expect(
      await screen.findByRole('button', { name: 'Select span Back manual' })
    ).toHaveAttribute('aria-pressed', 'true');

    await act(async () => {
      await view.router.navigate(-1);
    });

    await waitFor(() => {
      expect(
        within(screen.getByLabelText('Span breadcrumb')).getByText('Back failed')
      ).toBeInTheDocument();
    });
    expect(view.router.state.location.search).toBe('');
  });

  it('keeps trace detail selection changes local and does not refetch trace or span data', async () => {
    const user = userEvent.setup();
    const rootSpan = createSpan({
      span_id: 'query-root',
      name: 'Query root',
      status: 'COMPLETED',
    });
    const childSpan = createSpan({
      span_id: 'query-child',
      name: 'Query child',
      parent_span_id: 'query-root',
      status: 'COMPLETED',
    });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () =>
          jsonResponse({
            ...TRACE_DETAIL,
            status: 'COMPLETED',
            error_count: 0,
          }),
        spans: () => jsonResponse({ spans: [rootSpan, childSpan] }),
        timeline: () =>
          jsonResponse({
            events: [],
            trace_status: 'COMPLETED',
            has_more: false,
          }),
      })
    );

    const view = renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
    expect(await screen.findByText('Trace Context')).toBeInTheDocument();

    fetchMock.mockClear();

    await user.click(screen.getByRole('button', { name: 'Select span Query child' }));

    expect(
      within(screen.getByLabelText('Span breadcrumb')).getByText('Query child')
    ).toBeInTheDocument();
    expect(view.router.state.location.search).toBe('?span=query-child');
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('copies an absolute trace URL that preserves unrelated params and includes the effective selected span', async () => {
    const user = userEvent.setup();
    const writeText = mockClipboard();
    const rootSpan = createSpan({
      span_id: 'copy-root',
      name: 'Copy root',
      status: 'COMPLETED',
    });
    const failedSpan = createSpan({
      span_id: 'copy-failed',
      name: 'Copy failed',
      parent_span_id: 'copy-root',
      status: 'FAILED',
      ended_at: '2026-03-14T10:00:10.000Z',
      error_message: 'Copy failure',
    });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(TRACE_DETAIL),
        spans: () => jsonResponse({ spans: [rootSpan, failedSpan] }),
        timeline: () =>
          jsonResponse({
            events: [
              createTimelineEvent({
                id: 'copy-error',
                span_id: 'copy-failed',
                span_name: 'Copy failed',
                event_type: 'error',
                message: 'Copy failure',
                timestamp: '2026-03-14T10:00:10.000Z',
              }),
            ],
            trace_status: 'FAILED',
            has_more: false,
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}?debug=true`]);
    expect(
      await screen.findByRole('button', { name: 'Select span Copy failed' })
    ).toHaveAttribute('aria-pressed', 'true');

    await user.click(screen.getByRole('button', { name: 'Copy Trace URL' }));

    expect(writeText).toHaveBeenCalledWith(
      `${window.location.origin}/traces/trace-checkout?debug=true&span=copy-failed`
    );
  });

  it('returns to the filtered trace list after span inspection without stepping through span history', async () => {
    const user = userEvent.setup();
    const rootSpan = createSpan({
      span_id: 'history-return-root',
      name: 'History return root',
      status: 'COMPLETED',
    });
    const failedSpan = createSpan({
      span_id: 'history-return-failed',
      name: 'History return failed',
      parent_span_id: 'history-return-root',
      status: 'FAILED',
      ended_at: '2026-03-14T10:00:10.000Z',
      error_message: 'History return failure',
    });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(TRACE_DETAIL),
        spans: () => jsonResponse({ spans: [rootSpan, failedSpan] }),
        timeline: () =>
          jsonResponse({
            events: [
              createTimelineEvent({
                id: 'history-return-error',
                span_id: 'history-return-failed',
                span_name: 'History return failed',
                event_type: 'error',
                message: 'History return failure',
                timestamp: '2026-03-14T10:00:10.000Z',
              }),
            ],
            trace_status: 'FAILED',
            has_more: false,
          }),
      })
    );

    const view = renderTraceRoutes(['/traces?status=failed']);
    await user.click(await screen.findByRole('link', { name: 'Checkout Trace' }));
    expect(await screen.findByText('Trace Context')).toBeInTheDocument();

    await user.click(
      screen.getByRole('button', { name: 'Select span History return root' })
    );
    await user.click(
      screen.getByRole('button', { name: 'Select span History return failed' })
    );

    await act(async () => {
      await view.router.navigate(-1);
    });

    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();
    expect(view.router.state.location.pathname).toBe('/traces');
    expect(view.router.state.location.search).toBe('?status=failed');
  });

  it('keeps tree, detail, parent navigation, failure summary, and timeline selections synchronized with the URL', async () => {
    const user = userEvent.setup();
    const rootSpan = createSpan({
      span_id: 'sync-root',
      name: 'Sync root',
      status: 'COMPLETED',
    });
    const failedSpan = createSpan({
      span_id: 'sync-failed',
      name: 'Sync failed',
      parent_span_id: 'sync-root',
      status: 'FAILED',
      ended_at: '2026-03-14T10:00:10.000Z',
      error_message: 'Sync failure',
    });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(TRACE_DETAIL),
        spans: () => jsonResponse({ spans: [rootSpan, failedSpan] }),
        timeline: () =>
          jsonResponse({
            events: [
              createTimelineEvent({
                id: 'sync-root-event',
                span_id: 'sync-root',
                event_type: 'message',
                message: 'Root event',
                timestamp: '2026-03-14T10:00:05.000Z',
              }),
              createTimelineEvent({
                id: 'sync-error-event',
                span_id: 'sync-failed',
                span_name: 'Sync failed',
                event_type: 'error',
                message: 'Sync failure',
                timestamp: '2026-03-14T10:00:10.000Z',
              }),
            ],
            trace_status: 'FAILED',
            has_more: false,
          }),
      })
    );

    const view = renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
    expect(
      await screen.findByRole('button', { name: 'Select span Sync failed' })
    ).toHaveAttribute('aria-pressed', 'true');

    await user.click(
      screen.getByRole('button', { name: 'Select parent span sync-root' })
    );
    expect(view.router.state.location.search).toBe('?span=sync-root');
    expect(
      within(screen.getByLabelText('Span breadcrumb')).getByText('Sync root')
    ).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Select span Sync failed' }));
    expect(view.router.state.location.search).toBe('?span=sync-failed');

    const timelineSection = screen
      .getByRole('heading', { name: 'Timeline' })
      .closest('section');
    if (!timelineSection) {
      throw new Error('Expected timeline section');
    }

    await user.click(within(timelineSection).getByRole('button', { name: 'sync-root' }));
    expect(view.router.state.location.search).toBe('?span=sync-root');
    expect(
      within(screen.getByLabelText('Span breadcrumb')).getByText('Sync root')
    ).toBeInTheDocument();

    await user.click(
      screen.getByRole('button', { name: 'Jump to failed span Sync failed' })
    );
    expect(view.router.state.location.search).toBe('?span=sync-failed');
    expect(
      within(screen.getByLabelText('Span breadcrumb')).getByText('Sync failed')
    ).toBeInTheDocument();
  });

  it(
    'preserves a manual selection across running-trace polling updates',
    async () => {
      const user = userEvent.setup();
    const runningTraceDetail: TraceDetail = {
      ...TRACE_DETAIL,
      status: 'RUNNING',
      ended_at: undefined,
      error_count: 0,
    };
    const rootSpan = createSpan({
      span_id: 'running-root',
      name: 'Running root',
      status: 'STARTED',
    });
    const childSpan = createSpan({
      span_id: 'running-child',
      name: 'Running child',
      parent_span_id: 'running-root',
      status: 'STARTED',
    });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(runningTraceDetail),
        spans: () => jsonResponse({ spans: [rootSpan, childSpan] }),
        timeline: (url) => {
          if (url.searchParams.get('after') === 'cursor-running') {
            return jsonResponse({
              events: [
                createTimelineEvent({
                  id: 'poll-running-event',
                  span_id: 'running-child',
                  span_name: 'Running child',
                  event_type: 'message',
                  timestamp: '2026-03-14T10:00:05.000Z',
                  message: 'Poll update',
                }),
              ],
              trace_status: 'RUNNING',
              has_more: false,
              poll_cursor: 'cursor-running',
            });
          }

          return jsonResponse({
            events: [
              createTimelineEvent({
                id: 'bootstrap-running-event',
                span_id: 'running-root',
                span_name: 'Running root',
                event_type: 'message',
                timestamp: '2026-03-14T10:00:00.000Z',
                message: 'Initial event',
              }),
            ],
            trace_status: 'RUNNING',
            has_more: false,
            poll_cursor: 'cursor-running',
          });
        },
      })
    );

    const view = renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
    expect(
      await screen.findByRole('button', { name: 'Select span Running child' })
    ).toBeInTheDocument();

    await user.click(
      screen.getByRole('button', { name: 'Select span Running child' })
    );
    expect(
      within(screen.getByLabelText('Span breadcrumb')).getByText('Running child')
    ).toBeInTheDocument();
    expect(view.router.state.location.search).toBe('?span=running-child');

      await act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 3100));
      });

      expect(await screen.findByText('Poll update')).toBeInTheDocument();
      expect(
        within(screen.getByLabelText('Span breadcrumb')).getByText('Running child')
      ).toBeInTheDocument();
      expect(view.router.state.location.search).toBe('?span=running-child');
    },
    10000
  );

  it(
    'refetches trace and span data when a running trace transitions to failed and then auto-selects the refreshed failed span',
    async () => {
    const runningTraceDetail: TraceDetail = {
      ...TRACE_DETAIL,
      status: 'RUNNING',
      ended_at: undefined,
      error_count: 0,
    };
    const failedTraceDetail: TraceDetail = {
      ...TRACE_DETAIL,
      status: 'FAILED',
      ended_at: '2026-03-14T10:00:20.000Z',
      error_count: 1,
    };
    const rootSpan = createSpan({
      span_id: 'transition-root',
      name: 'Transition root',
      status: 'STARTED',
    });
    const inFlightSpan = createSpan({
      span_id: 'in-flight',
      name: 'In-flight work',
      parent_span_id: 'transition-root',
      status: 'STARTED',
    });
    const failedSpan = createSpan({
      span_id: 'transition-failed',
      name: 'Transition failed',
      parent_span_id: 'transition-root',
      kind: 'TOOL',
      status: 'FAILED',
      started_at: '2026-03-14T10:00:05.000Z',
      ended_at: '2026-03-14T10:00:12.000Z',
      error_message: 'Transition failure preview',
    });

    let detailCalls = 0;
    let spansCalls = 0;
    let fullTimelineCalls = 0;

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => {
          detailCalls += 1;
          return jsonResponse(detailCalls === 1 ? runningTraceDetail : failedTraceDetail);
        },
        spans: () => {
          spansCalls += 1;
          return jsonResponse({
            spans:
              spansCalls === 1 ? [rootSpan, inFlightSpan] : [rootSpan, failedSpan],
          });
        },
        timeline: (url) => {
          if (url.searchParams.get('after') === 'cursor-transition') {
            return jsonResponse({
              events: [
                createTimelineEvent({
                  id: 'transition-error',
                  span_id: 'transition-failed',
                  span_name: 'Transition failed',
                  event_type: 'error',
                  timestamp: '2026-03-14T10:00:12.000Z',
                  message: 'Transition failure preview',
                }),
              ],
              trace_status: 'FAILED',
              has_more: false,
              poll_cursor: 'cursor-terminal',
            });
          }

          fullTimelineCalls += 1;
          if (fullTimelineCalls === 1) {
            return jsonResponse({
              events: [],
              trace_status: 'RUNNING',
              has_more: false,
              poll_cursor: 'cursor-transition',
            });
          }

          return jsonResponse({
            events: [
              createTimelineEvent({
                id: 'terminal-error',
                span_id: 'transition-failed',
                span_name: 'Transition failed',
                event_type: 'error',
                timestamp: '2026-03-14T10:00:12.000Z',
                message: 'Transition failure preview',
              }),
            ],
            trace_status: 'FAILED',
            has_more: false,
            poll_cursor: 'cursor-terminal',
          });
        },
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
    expect(await screen.findByText('Trace Context')).toBeInTheDocument();

      await act(async () => {
        await new Promise((resolve) => setTimeout(resolve, 3100));
      });

      expect(
        await screen.findByRole('button', {
          name: 'Jump to failed span Transition failed',
        })
      ).toBeInTheDocument();
      await waitFor(() => {
        expect(detailCalls).toBeGreaterThanOrEqual(2);
        expect(spansCalls).toBeGreaterThanOrEqual(2);
      });
      expect(
        within(screen.getByLabelText('Span breadcrumb')).getByText('Transition failed')
      ).toBeInTheDocument();
    },
    10000
  );

  it('resets selection and timeline filter state when navigating between traces', async () => {
    const user = userEvent.setup();
    const traceADetail: TraceDetail = {
      ...TRACE_DETAIL,
      id: 'trace-a',
      name: 'Trace A',
    };
    const traceBDetail: TraceDetail = {
      ...TRACE_DETAIL,
      id: 'trace-b',
      name: 'Trace B',
      status: 'COMPLETED',
      ended_at: '2026-03-14T10:00:20.000Z',
      error_count: 0,
    };
    const traceASpans = [
      createSpan({
        trace_id: 'trace-a',
        span_id: 'trace-a-root',
        name: 'Trace A root',
        status: 'COMPLETED',
      }),
      createSpan({
        trace_id: 'trace-a',
        span_id: 'trace-a-failed',
        name: 'Trace A failed child',
        parent_span_id: 'trace-a-root',
        status: 'FAILED',
        ended_at: '2026-03-14T10:00:10.000Z',
        error_message: 'Trace A failure',
      }),
    ];
    const traceBSpans = [
      createSpan({
        trace_id: 'trace-b',
        span_id: 'trace-b-root',
        name: 'Trace B root',
        status: 'COMPLETED',
      }),
    ];

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: (url) =>
          url.pathname.endsWith('/trace-b')
            ? jsonResponse(traceBDetail)
            : jsonResponse(traceADetail),
        spans: (url) =>
          url.pathname.includes('/trace-b/')
            ? jsonResponse({ spans: traceBSpans })
            : jsonResponse({ spans: traceASpans }),
        timeline: (url) =>
          url.pathname.includes('/trace-b/')
            ? jsonResponse({
                events: [
                  createTimelineEvent({
                    trace_id: 'trace-b',
                    span_id: 'trace-b-root',
                    span_name: 'Trace B root',
                    event_type: 'message',
                    message: 'Beta info event',
                    timestamp: '2026-03-14T10:00:15.000Z',
                  }),
                ],
                trace_status: 'COMPLETED',
                has_more: false,
              })
            : jsonResponse({
                events: [
                  createTimelineEvent({
                    trace_id: 'trace-a',
                    span_id: 'trace-a-failed',
                    span_name: 'Trace A failed child',
                    event_type: 'error',
                    message: 'Trace A error event',
                    timestamp: '2026-03-14T10:00:10.000Z',
                  }),
                  createTimelineEvent({
                    trace_id: 'trace-a',
                    span_id: 'trace-a-root',
                    span_name: 'Trace A root',
                    event_type: 'message',
                    message: 'Trace A info event',
                    timestamp: '2026-03-14T10:00:05.000Z',
                  }),
                ],
                trace_status: 'FAILED',
                has_more: false,
              }),
      })
    );

    const view = renderTraceRoutes(['/traces/trace-a']);
    expect(await screen.findByText('Trace A')).toBeInTheDocument();

    await user.click(
      screen.getByRole('button', { name: 'Select span Trace A root' })
    );
    expect(view.router.state.location.search).toBe('?span=trace-a-root');
    await user.click(screen.getByRole('button', { name: 'Show error events only' }));
    expect(screen.getByRole('button', { name: 'Show error events only' })).toHaveAttribute(
      'aria-pressed',
      'true'
    );

    await act(async () => {
      await view.router.navigate('/traces/trace-b');
    });

    expect(await screen.findByText('Trace B')).toBeInTheDocument();
    expect(view.router.state.location.search).toBe('');
    expect(screen.getByText('Select a span to view details')).toBeInTheDocument();
    expect(screen.getByText('Beta info event')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Show error events only' })).toHaveAttribute(
      'aria-pressed',
      'false'
    );
  });

  it('filters the timeline to error events only and restores all events when toggled off', async () => {
    const user = userEvent.setup();

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(TRACE_DETAIL),
        spans: () => jsonResponse({ spans: [] }),
        timeline: () =>
          jsonResponse({
            events: [
              createTimelineEvent({
                id: 'info-event',
                event_type: 'message',
                message: 'Informational update',
                timestamp: '2026-03-14T10:00:05.000Z',
              }),
              createTimelineEvent({
                id: 'error-event',
                event_type: 'error',
                message: 'Critical failure',
                timestamp: '2026-03-14T10:00:10.000Z',
              }),
            ],
            trace_status: 'FAILED',
            has_more: false,
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
    expect(await screen.findByText('Informational update')).toBeInTheDocument();
    expect(screen.getByText('Critical failure')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Show error events only' }));

    expect(screen.getByText('Critical failure')).toBeInTheDocument();
    expect(screen.queryByText('Informational update')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Show error events only' })).toHaveAttribute(
      'aria-pressed',
      'true'
    );

    await user.click(screen.getByRole('button', { name: 'Show error events only' }));

    expect(await screen.findByText('Informational update')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Show error events only' })).toHaveAttribute(
      'aria-pressed',
      'false'
    );
  });

  it('shows the filtered empty state when error-only mode has no matches', async () => {
    const user = userEvent.setup();

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () =>
          jsonResponse({
            ...TRACE_DETAIL,
            status: 'COMPLETED',
            error_count: 0,
          }),
        spans: () => jsonResponse({ spans: [] }),
        timeline: () =>
          jsonResponse({
            events: [
              createTimelineEvent({
                id: 'message-only-event',
                event_type: 'message',
                message: 'Only info here',
              }),
            ],
            trace_status: 'COMPLETED',
            has_more: false,
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
    expect(await screen.findByText('Only info here')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Show error events only' }));

    expect(
      screen.getByText('No error events for this trace.')
    ).toBeInTheDocument();
  });

  it('supports keyboard activation for the summary jump, breadcrumb ancestor, and error-only toggle', async () => {
    const user = userEvent.setup();
    const rootSpan = createSpan({
      span_id: 'keyboard-root',
      name: 'Keyboard root',
      status: 'COMPLETED',
      ended_at: '2026-03-14T10:00:05.000Z',
    });
    const failedSpan = createSpan({
      span_id: 'keyboard-failed',
      name: 'Keyboard failed',
      parent_span_id: 'keyboard-root',
      status: 'FAILED',
      ended_at: '2026-03-14T10:00:10.000Z',
      error_message: 'Keyboard failure',
    });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(TRACE_DETAIL),
        spans: () => jsonResponse({ spans: [rootSpan, failedSpan] }),
        timeline: () =>
          jsonResponse({
            events: [
              createTimelineEvent({
                id: 'keyboard-info',
                span_id: 'keyboard-root',
                span_name: 'Keyboard root',
                event_type: 'message',
                message: 'Keyboard info',
                timestamp: '2026-03-14T10:00:05.000Z',
              }),
              createTimelineEvent({
                id: 'keyboard-error',
                span_id: 'keyboard-failed',
                span_name: 'Keyboard failed',
                event_type: 'error',
                message: 'Keyboard error',
                timestamp: '2026-03-14T10:00:10.000Z',
              }),
            ],
            trace_status: 'FAILED',
            has_more: false,
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
    expect(
      await screen.findByRole('button', {
        name: 'Jump to failed span Keyboard failed',
      })
    ).toBeInTheDocument();

    const errorsOnlyToggle = screen.getByRole('button', {
      name: 'Show error events only',
    });
    errorsOnlyToggle.focus();
    await user.keyboard('{Enter}');
    expect(errorsOnlyToggle).toHaveAttribute('aria-pressed', 'true');
    expect(screen.queryByText('Keyboard info')).not.toBeInTheDocument();

    const breadcrumbAncestor = within(
      screen.getByLabelText('Span breadcrumb')
    ).getByRole('button', {
      name: 'Select ancestor span Keyboard root',
    });
    breadcrumbAncestor.focus();
    await user.keyboard('{Enter}');
    expect(
      within(screen.getByLabelText('Span breadcrumb')).getByText('Keyboard root')
    ).toBeInTheDocument();

    const jumpButton = screen.getByRole('button', {
      name: 'Jump to failed span Keyboard failed',
    });
    jumpButton.focus();
    await user.keyboard('{Enter}');
    expect(
      within(screen.getByLabelText('Span breadcrumb')).getByText('Keyboard failed')
    ).toBeInTheDocument();
  });

  it('renders the generic failure summary when a failed trace has no failed spans', async () => {
    const rootSpan = createSpan({
      span_id: 'no-failed-root',
      name: 'No failed root',
      status: 'COMPLETED',
      ended_at: '2026-03-14T10:00:05.000Z',
    });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(TRACE_DETAIL),
        spans: () => jsonResponse({ spans: [rootSpan] }),
        timeline: () =>
          jsonResponse({
            events: [
              createTimelineEvent({
                id: 'no-failed-error-log',
                event_type: 'log',
                level: 'error',
                message: 'Top-level failure log',
                timestamp: '2026-03-14T10:00:10.000Z',
              }),
            ],
            trace_status: 'FAILED',
            has_more: false,
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(
      await screen.findByText(
        'This trace is marked as failed, but no failed span could be identified from the current span data.'
      )
    ).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /Jump to failed span/i })).not.toBeInTheDocument();
    expect(screen.getByText('Select a span to view details')).toBeInTheDocument();
  });

  it('renders truncation banners for span payloads only', async () => {
    const rootSpan = createSpan({
      span_id: 'trunc-root',
      name: 'Trunc root',
      status: 'COMPLETED',
    });
    const failedSpan = createSpan({
      span_id: 'trunc-failed',
      name: 'Trunc failed',
      parent_span_id: 'trunc-root',
      status: 'FAILED',
      ended_at: '2026-03-14T10:00:10.000Z',
      error_message: 'Trunc failure',
      input: { prompt: 'large input' },
      input_truncated: true,
      input_original_size_bytes: 2048,
      input_truncation_reason: 'size_limit',
      output: { answer: 'large output' },
      output_truncated: true,
      output_original_size_bytes: 1048576,
      metadata: { mode: 'debug' },
    });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () =>
          jsonResponse({
            ...TRACE_DETAIL,
            input: { trace: 'input' },
            output: { trace: 'output' },
          }),
        spans: () => jsonResponse({ spans: [rootSpan, failedSpan] }),
        timeline: () =>
          jsonResponse({
            events: [
              createTimelineEvent({
                id: 'trunc-error',
                span_id: 'trunc-failed',
                span_name: 'Trunc failed',
                event_type: 'error',
                message: 'Trunc failure',
                timestamp: '2026-03-14T10:00:10.000Z',
                payload: { trace: 'timeline payload' },
              }),
            ],
            trace_status: 'FAILED',
            has_more: false,
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(
      await screen.findByRole('button', { name: 'Select span Trunc failed' })
    ).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getAllByText('Payload truncated')).toHaveLength(2);
    expect(screen.getByText(/Original size: 2.0 KB/)).toBeInTheDocument();
    expect(screen.getByText(/Original size: 1.0 MB/)).toBeInTheDocument();
  });

  it('falls back to the primary failed span when the selected span disappears after a refresh', async () => {
    const rootSpan = createSpan({
      span_id: 'fallback-root',
      name: 'Fallback root',
      status: 'COMPLETED',
      ended_at: '2026-03-14T10:00:05.000Z',
    });
    const disappearingSpan = createSpan({
      span_id: 'disappearing-child',
      name: 'Disappearing child',
      parent_span_id: 'fallback-root',
      status: 'COMPLETED',
      ended_at: '2026-03-14T10:00:07.000Z',
    });
    const fallbackFailedSpan = createSpan({
      span_id: 'fallback-failed',
      name: 'Fallback failed',
      parent_span_id: 'fallback-root',
      status: 'FAILED',
      ended_at: '2026-03-14T10:00:10.000Z',
      error_message: 'Fallback failure preview',
    });
    let currentSpans = [rootSpan, disappearingSpan, fallbackFailedSpan];

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(TRACE_DETAIL),
        spans: () => jsonResponse({ spans: currentSpans }),
        timeline: () =>
          jsonResponse({
            events: [
              createTimelineEvent({
                id: 'fallback-error',
                span_id: 'fallback-failed',
                span_name: 'Fallback failed',
                event_type: 'error',
                message: 'Fallback failure preview',
                timestamp: '2026-03-14T10:00:10.000Z',
              }),
            ],
            trace_status: 'FAILED',
            has_more: false,
          }),
      })
    );

    const view = renderTraceRoutes([
      `/traces/${TRACE_ONE.id}?span=disappearing-child`,
    ]);
    expect(
      await screen.findByRole('button', { name: 'Select span Disappearing child' })
    ).toBeInTheDocument();
    expect(
      within(screen.getByLabelText('Span breadcrumb')).getByText('Disappearing child')
    ).toBeInTheDocument();

    currentSpans = [rootSpan, fallbackFailedSpan];

    await act(async () => {
      await view.queryClient.invalidateQueries({ queryKey: ['spans', TRACE_ONE.id] });
    });

    await waitFor(() => {
      expect(
        within(screen.getByLabelText('Span breadcrumb')).getByText('Fallback failed')
      ).toBeInTheDocument();
    });
    expect(view.router.state.location.search).toBe('');
  });

  it('waits for the initial timeline snapshot before evaluating the stale trace signal', async () => {
    const now = Date.now();
    const staleStartedAt = new Date(now - 20 * 60 * 1000).toISOString();
    const recentActivityAt = new Date(now - 60 * 1000).toISOString();
    const timelineResponse = createDeferredResponse();

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () =>
          jsonResponse({
            ...TRACE_DETAIL,
            status: 'RUNNING',
            started_at: staleStartedAt,
            ended_at: undefined,
            error_count: 0,
          }),
        spans: () =>
          jsonResponse({
            spans: [
              createSpan({
                span_id: 'deferred-stale-root',
                name: 'Deferred stale root',
                status: 'STARTED',
                started_at: staleStartedAt,
              }),
            ],
          }),
        timeline: () => timelineResponse.promise,
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
    expect(await screen.findByText('Trace Context')).toBeInTheDocument();
    expect(
      screen.queryByText(/still marked running\. recent activity is sparse/i)
    ).not.toBeInTheDocument();

    await act(async () => {
      timelineResponse.resolve(
        jsonResponse({
          events: [
            createTimelineEvent({
              id: 'deferred-recent-event',
              span_id: 'deferred-stale-root',
              span_name: 'Deferred stale root',
              event_type: 'message',
              message: 'Recent activity',
              timestamp: recentActivityAt,
            }),
          ],
          trace_status: 'RUNNING',
          has_more: false,
        })
      );
      await Promise.resolve();
    });

    expect(await screen.findByText('Recent activity')).toBeInTheDocument();
    expect(
      screen.queryByText(/still marked running\. recent activity is sparse/i)
    ).not.toBeInTheDocument();
  });

  it('renders the stale trace signal as static informational text', async () => {
    const now = Date.now();
    const staleStartedAt = new Date(now - 20 * 60 * 1000).toISOString();
    const latestActivityAt = new Date(now - 7 * 60 * 1000).toISOString();

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () =>
          jsonResponse({
            ...TRACE_DETAIL,
            status: 'RUNNING',
            started_at: staleStartedAt,
            ended_at: undefined,
            error_count: 0,
          }),
        spans: () =>
          jsonResponse({
            spans: [
              createSpan({
                span_id: 'stale-root',
                name: 'Stale root',
                status: 'STARTED',
                started_at: staleStartedAt,
              }),
            ],
          }),
        timeline: () =>
          jsonResponse({
            events: [
              createTimelineEvent({
                id: 'stale-event',
                span_id: 'stale-root',
                span_name: 'Stale root',
                event_type: 'message',
                timestamp: latestActivityAt,
                message: 'Last known activity',
              }),
            ],
            trace_status: 'RUNNING',
            has_more: false,
            poll_cursor: 'cursor-stale',
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    const staleSignal = await screen.findByText(
      /still marked running\. recent activity is sparse/i
    );
    expect(staleSignal).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(staleSignal.closest('[aria-live]')).toBeNull();
  });
});
