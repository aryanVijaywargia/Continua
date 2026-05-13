import { act } from 'react';
import {
  fireEvent,
  screen,
  waitFor,
  within,
} from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  clearApiKey,
  setApiKey,
} from '../api/client';
import { downloadJsonFile } from '../utils/downloadJson';
import { localDateToISOEnd, localDateToISOStart } from '../utils/tracesSearchParams';
import {
  SESSION_ID,
  TRACE_ONE,
  TRACE_THREE,
  TRACE_TWO,
  TRACE_ZETA,
  buildFetchHandler,
  createDeferredResponse,
  getRequests,
  jsonResponse,
  readRequestUrl,
  renderTraceRoutes,
  resetTestEntityCounter,
} from './testUtils';

vi.mock('../utils/downloadJson', () => ({
  downloadJsonFile: vi.fn(),
}));

let fetchMock: ReturnType<typeof vi.fn>;

function getTraceListRequests(): URL[] {
  return getRequests(fetchMock, '/api/traces');
}

function countTraceDetailRequests(): number {
  return fetchMock.mock.calls.filter(([input]) => {
    const url = new URL(readRequestUrl(input), 'http://localhost');
    return /^\/api\/traces\/[^/]+$/.test(url.pathname);
  }).length;
}

function getTraceRow(name: string): HTMLElement {
  const row = screen.getByRole('link', { name }).closest('article');
  if (!row) {
    throw new Error(`Expected a table row for ${name}`);
  }
  return row;
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

beforeEach(() => {
  resetTestEntityCounter();
  fetchMock = vi.fn();
  vi.mocked(downloadJsonFile).mockReset();
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
      screen.getAllByText('Search names, user IDs, and matching span names.').length
    ).toBeGreaterThan(0);
    await waitForListFetch('?limit=20');
    await waitFor(() => {
      expect(countTraceDetailRequests()).toBe(0);
    });
    expect(screen.queryByRole('button', { name: /Clear \(/ })).not.toBeInTheDocument();
  });

  it('shows an engine badge for engine-backed traces', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        list: () =>
          jsonResponse({
            traces: [
              {
                ...TRACE_ONE,
                engine: {
                  run_id: '123e4567-e89b-12d3-a456-426614174100',
                  definition_name: 'checkout',
                  definition_version: 'v1',
                  projection_state: 'up_to_date',
                },
              },
            ],
            total: 1,
          }),
      })
    );

    renderTraceRoutes(['/traces']);

    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();
    expect(screen.getByText('Engine')).toBeInTheDocument();
  });

  it('exports the current trace page as JSON', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(buildFetchHandler());

    renderTraceRoutes(['/traces?q=checkout&limit=50']);

    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Export' }));

    expect(downloadJsonFile).toHaveBeenCalledWith(
      'continua-traces.json',
      expect.objectContaining({
        source: '/traces?limit=50&q=checkout',
        total: 2,
        count: 2,
        filters: expect.objectContaining({
          q: 'checkout',
          limit: 50,
        }),
        traces: expect.arrayContaining([
          expect.objectContaining({ id: TRACE_ONE.id }),
        ]),
      })
    );
  });

  it('keeps non-engine trace rows visually unchanged', async () => {
    fetchMock.mockImplementation(buildFetchHandler());

    renderTraceRoutes(['/traces']);

    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();
    expect(within(getTraceRow('Checkout Trace')).queryByText('Engine')).not.toBeInTheDocument();
  });

  it('shows the auth recovery banner when the traces request returns 401', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        list: () => jsonResponse({ message: 'Invalid or missing API key' }, 401),
      })
    );

    renderTraceRoutes(['/traces']);

    expect(await screen.findByRole('alert')).toHaveTextContent('Invalid or missing API key');
    expect(screen.getByRole('link', { name: 'Sign in again' })).toHaveAttribute(
      'href',
      '/traces'
    );
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
    expect(screen.getByLabelText('Filter Failed traces')).toBeChecked();
    expect(screen.getByLabelText('Start Date')).toHaveValue('2026-03-10');
    expect(screen.getByLabelText('End Date')).toHaveValue('2026-03-12');
    expect(screen.getByLabelText('User ID')).toHaveValue('user-123');
    expect(screen.getByLabelText('Min Duration (ms)')).toHaveValue(1200);
    expect(screen.getByLabelText('Only show traces with errors')).toBeChecked();
  });

  it('renders engine filters in the facet panel and rehydrates them from the URL', async () => {
    fetchMock.mockImplementation(buildFetchHandler());

    const firstView = renderTraceRoutes(['/traces']);
    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();
    expect(screen.getByText('No engine definitions')).toBeInTheDocument();
    expect(screen.getByLabelText('Engine Instance Key')).toHaveValue('');
    firstView.unmount();

    renderTraceRoutes([
      '/traces?engine_definition_name=checkout&engine_run_status=waiting&engine_projection_state=summary_only',
    ]);

    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();
    await waitForListFetch(
      '?limit=20&engine_definition_name=checkout&engine_run_status=waiting&engine_projection_state=summary_only'
    );

    expect(screen.getByLabelText('Filter checkout engine definition')).toBeChecked();
    expect(screen.getByLabelText('Engine Status')).toHaveValue('waiting');
    expect(screen.getByLabelText('Projection State')).toHaveValue(
      'summary_only'
    );
    expect(
      screen.getByText(
        'Advanced operator filter for inspecting projection health across engine traces.'
      )
    ).toBeInTheDocument();
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

  it('replaces history entries for debounced text filters', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(buildFetchHandler());

    const { router } = renderTraceRoutes(['/dashboard', '/traces'], {
      initialIndex: 1,
    });
    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();

    const searchInput = screen.getByLabelText('Search');
    await user.type(searchInput, 'latency');
    await waitForDebounce();
    await waitForListFetch('?limit=20&q=latency');

    await act(async () => {
      await router.navigate(-1);
    });

    await screen.findByRole('heading', { name: 'Trace volume' });
    expect(router.state.location.pathname).toBe('/dashboard');
  });

  it('commits engine filters with lowercase query values and removable chips', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(buildFetchHandler());

    renderTraceRoutes(['/traces']);
    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();

    fetchMock.mockClear();
    const instanceKeyInput = screen.getByLabelText('Engine Instance Key');
    await user.type(instanceKeyInput, 'order-123');
    await waitForDebounce();
    await waitForListFetch('?limit=20&engine_instance_key=order-123');

    await user.selectOptions(screen.getByLabelText('Engine Status'), 'suspended');
    await waitForListFetch(
      '?limit=20&engine_instance_key=order-123&engine_run_status=suspended'
    );

    await user.selectOptions(
      screen.getByLabelText('Projection State'),
      'summary_only'
    );
    await waitForListFetch(
      '?limit=20&engine_instance_key=order-123&engine_run_status=suspended&engine_projection_state=summary_only'
    );

    expect(screen.getByText('Engine status:')).toBeInTheDocument();
    expect(screen.getAllByText('Suspended').length).toBeGreaterThan(0);
    expect(
      screen.getByRole('button', { name: 'Clear Engine status filter' })
    ).toBeInTheDocument();

    await user.click(
      screen.getByRole('button', { name: 'Clear Engine status filter' })
    );
    await waitForListFetch(
      '?limit=20&engine_instance_key=order-123&engine_projection_state=summary_only'
    );
  });

  it('commits text filters after debounce even if focus moves away', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(buildFetchHandler());

    const { router } = renderTraceRoutes(['/traces']);
    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();

    fetchMock.mockClear();
    const userIdInput = screen.getByLabelText('User ID');

    await user.type(userIdInput, 'owner-42');
    fireEvent.blur(userIdInput);
    await waitForDebounce();

    await waitForListFetch('?limit=20&user_id=owner-42');
    expect(router.state.location.search).toBe('?user_id=owner-42');
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

  it('commits min duration after debounce even if focus moves away', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(buildFetchHandler());

    const { router } = renderTraceRoutes(['/traces']);
    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();

    fetchMock.mockClear();
    const minDurationInput = screen.getByLabelText('Min Duration (ms)');

    await user.type(minDurationInput, '1500');
    fireEvent.blur(minDurationInput);
    await waitForDebounce();

    await waitForListFetch('?limit=20&min_duration_ms=1500');
    expect(router.state.location.search).toBe('?min_duration_ms=1500');
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

    await user.click(screen.getByLabelText('Filter Running traces'));
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

    expect(screen.getAllByText(SESSION_ID).length).toBeGreaterThan(0);

    await user.click(screen.getByRole('button', { name: 'Clear Search filter' }));
    await waitForListFetch(`?limit=20&session_id=${SESSION_ID}&has_errors=true`);

    await user.click(screen.getByRole('button', { name: 'Clear Session filter' }));
    await waitForListFetch('?limit=20&has_errors=true');

    await user.click(screen.getByRole('button', { name: 'Clear (1)' }));
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

    await user.click(screen.getByLabelText('Filter Running traces'));

    expect(screen.getByText('Checkout Trace')).toBeInTheDocument();
    expect(screen.getByText('Refreshing…')).toBeInTheDocument();

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
    expect(screen.getByRole('button', { name: 'Clear (1)' })).toBeInTheDocument();
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

    const links = screen.getAllByRole('link', {
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

    expect(
      await screen.findByRole('button', { name: /Trace Context/i })
    ).toBeInTheDocument();
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
    expect(
      await screen.findByRole('button', { name: /Trace Context/i })
    ).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '← Traces' })).toHaveAttribute(
      'href',
      '/traces'
    );
    unrelatedState.unmount();

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
    expect(
      await screen.findByRole('button', { name: /Trace Context/i })
    ).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '← Traces' })).toHaveAttribute(
      'href',
      '/traces'
    );
  });

  it('keeps dense filter controls and chip dismiss buttons reachable with visible focus classes', async () => {
    fetchMock.mockImplementation(buildFetchHandler());

    renderTraceRoutes([
      `/traces?q=checkout&has_errors=true&session_id=${SESSION_ID}`,
    ]);
    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();

    const searchInput = screen.getByLabelText('Search');
    const statusCheckbox = screen.getByLabelText('Filter Failed traces');
    const startDate = screen.getByLabelText('Start Date');
    const endDate = screen.getByLabelText('End Date');
    const userIdInput = screen.getByLabelText('User ID');
    const minDurationInput = screen.getByLabelText('Min Duration (ms)');
    const hasErrorsCheckbox = screen.getByLabelText('Only show traces with errors');
    const engineInstanceInput = screen.getByLabelText('Engine Instance Key');
    const clearSearch = screen.getByRole('button', { name: 'Clear Search filter' });
    const clearErrors = screen.getByRole('button', { name: 'Clear Errors filter' });
    const clearSession = screen.getByRole('button', { name: 'Clear Session filter' });
    const clearAll = screen.getByRole('button', { name: 'Clear (3)' });
    expect(searchInput).toBeEnabled();
    expect(statusCheckbox).toBeEnabled();
    expect(startDate).toBeEnabled();
    expect(endDate).toBeEnabled();
    expect(userIdInput).toBeEnabled();
    expect(minDurationInput).toBeEnabled();
    expect(hasErrorsCheckbox).toBeEnabled();
    expect(engineInstanceInput).toBeEnabled();
    expect(clearSearch).toBeEnabled();
    expect(clearErrors).toBeEnabled();
    expect(clearSession).toBeEnabled();
    expect(clearAll).toBeEnabled();

    expect(searchInput).toHaveClass('app-input');
    expect(clearSearch.className).toContain('focus:ring-2');
    expect(clearAll.className).toContain('focus:ring-2');
  });

  it('rehydrates draft text inputs on back and forward navigation', async () => {
    fetchMock.mockImplementation(buildFetchHandler());

    const { router } = renderTraceRoutes(['/traces?q=alpha']);
    const searchInput = (await screen.findByLabelText('Search')) as HTMLInputElement;
    expect(searchInput.value).toBe('alpha');

    await act(async () => {
      await router.navigate('/traces?q=beta');
    });
    await waitFor(() => {
      expect(searchInput.value).toBe('beta');
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

  it('toggles started sorting and resets offset when the page size changes', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(
      buildFetchHandler({
        list: (url) =>
          jsonResponse({
            traces: url.searchParams.get('offset') === '20' ? [TRACE_TWO] : [TRACE_ONE],
            total: 60,
          }),
      })
    );

    const { router } = renderTraceRoutes(['/traces']);
    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Started' }));
    await waitForListFetch('?limit=20&sort_by=started_at&sort_dir=asc');

    await user.click(screen.getByRole('button', { name: 'Next page' }));
    await waitForListFetch('?limit=20&offset=20&sort_by=started_at&sort_dir=asc');

    await user.selectOptions(screen.getByRole('combobox', { name: 'Rows per page' }), '50');
    await waitForListFetch('?limit=50&sort_by=started_at&sort_dir=asc');
    await waitFor(() => {
      expect(router.state.location.search).toBe('?limit=50&sort_by=started_at&sort_dir=asc');
    });
  });

  it('disables the started sort header while search is active', async () => {
    fetchMock.mockImplementation(buildFetchHandler());

    renderTraceRoutes(['/traces?q=checkout']);
    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();

    expect(screen.queryByRole('button', { name: 'Started' })).not.toBeInTheDocument();
    expect(screen.getAllByText('Started').length).toBeGreaterThan(0);
  });

  it('renders trace ID as the secondary trace row identifier', async () => {
    fetchMock.mockImplementation(buildFetchHandler());

    renderTraceRoutes(['/traces']);
    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();

    expect(screen.getByText(TRACE_ONE.id)).toBeInTheDocument();
    expect(screen.queryByText('conv-checkout-123')).not.toBeInTheDocument();
    expect(screen.queryByText(SESSION_ID)).not.toBeInTheDocument();
  });
});
