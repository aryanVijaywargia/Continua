import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  MemoryRouter,
  Route,
  Routes,
  useLocation,
} from 'react-router-dom';
import { clearApiKey, setApiKey, type Trace } from '../api/client';
import { TRACE_ONE } from '../test/traceFixtures';
import { ThemeProvider } from '../hooks/ThemeProvider';
import { DEFAULT_PAGE_SIZE } from '../utils/pagination';
import { jsonResponse, readRequestUrl, type RequestInput } from './testUtils';
import { EngineRunsPage } from './EngineRunsPage';

let fetchMock: ReturnType<typeof vi.fn>;

const ENGINE_TRACE: Trace = {
  ...TRACE_ONE,
  id: 'engine-trace-1',
  name: 'Darklaunch run',
  error_count: 0,
  engine: {
    run_id: '123e4567-e89b-12d3-a456-426614174100',
    definition_name: 'darklaunch.demo',
    definition_version: 'v1',
    projection_state: 'up_to_date',
    instance_key: 'darklaunch-1',
    status: 'QUEUED',
    pending_work: {
      pending_activity_tasks: 0,
      pending_inbox_items: 0,
    },
    updated_at: '2026-03-14T10:00:03.000Z',
  },
};

const QUARANTINED_ENGINE_TRACE: Trace = {
  ...ENGINE_TRACE,
  id: 'engine-trace-quarantined',
  engine: {
    ...ENGINE_TRACE.engine!,
    status: 'QUARANTINED',
    wait_state: {
      kind: 'replay_mismatch',
      expected_type: 'activity_scheduled',
      expected_key: 'charge-card',
      actual_type: 'timer_started',
      actual_key: 'timeout',
      detail: 'replay produced a different next event',
    },
  },
};

const DEFINITIONS = [
  {
    definition_name: 'defA',
    definition_version: 'v1',
    enabled: true,
    live: true,
    runtime_published_at: '2026-03-10T10:00:00.000Z',
    published_at: '2026-03-10T09:00:00.000Z',
  },
  {
    definition_name: 'defA',
    definition_version: 'v2',
    enabled: true,
    live: true,
    runtime_published_at: '2026-03-11T10:00:00.000Z',
    published_at: '2026-03-11T09:00:00.000Z',
  },
  {
    definition_name: 'defA',
    definition_version: 'v0',
    enabled: true,
    live: false,
    runtime_published_at: '2026-03-09T10:00:00.000Z',
    published_at: '2026-03-09T09:00:00.000Z',
  },
  {
    definition_name: 'defB',
    definition_version: 'v9',
    enabled: true,
    live: true,
    runtime_published_at: '2026-03-12T10:00:00.000Z',
    published_at: '2026-03-12T09:00:00.000Z',
  },
  {
    definition_name: 'legacy',
    definition_version: 'v0',
    enabled: true,
    live: false,
    runtime_published_at: '2026-03-09T10:00:00.000Z',
    published_at: '2026-03-09T09:00:00.000Z',
  },
];

function LocationProbe() {
  const location = useLocation();
  const state = location.state as { returnTo?: string } | null;

  return (
    <>
      <div data-testid="probe-pathname">{location.pathname}</div>
      <div data-testid="probe-search">{location.search}</div>
      <div data-testid="probe-return-to">{state?.returnTo}</div>
    </>
  );
}

function renderEngineRunsPage(initialEntry = '/engine/runs') {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <MemoryRouter initialEntries={[initialEntry]}>
          <Routes>
            <Route
              path="/engine/runs"
              element={
                <>
                  <EngineRunsPage />
                  <LocationProbe />
                </>
              }
            />
            <Route path="/traces/:traceId" element={<LocationProbe />} />
          </Routes>
        </MemoryRouter>
      </ThemeProvider>
    </QueryClientProvider>
  );
}

function mockEngineRequests({
  traces = [ENGINE_TRACE],
  total = traces.length,
  definitions = () => jsonResponse({ definitions: DEFINITIONS }),
  instance,
  start,
}: {
  traces?: Trace[];
  total?: number;
  definitions?: (url: URL, init?: RequestInit) => Promise<Response> | Response;
  instance?: (url: URL, init?: RequestInit) => Promise<Response> | Response;
  start?: (url: URL, init?: RequestInit) => Promise<Response> | Response;
} = {}) {
  fetchMock.mockImplementation((input: RequestInput, init?: RequestInit) => {
    const url = new URL(readRequestUrl(input), 'http://localhost');

    if (url.pathname === '/api/traces') {
      return jsonResponse({ traces, total });
    }
    if (
      url.pathname === '/v1/engine/definitions' &&
      (init?.method ?? 'GET') === 'GET'
    ) {
      return definitions(url, init);
    }
    if (/^\/v1\/engine\/instances\/[^/]+$/.test(url.pathname) && instance) {
      return instance(url, init);
    }
    if (url.pathname === '/v1/engine/runs' && init?.method === 'POST' && start) {
      return start(url, init);
    }

    throw new Error(`Unhandled request: ${init?.method ?? 'GET'} ${url.pathname}`);
  });
}

async function openStartDialog(user: ReturnType<typeof userEvent.setup>) {
  await screen.findByRole('table');
  await user.click(screen.getByRole('button', { name: 'Start run' }));
  return screen.findByRole('heading', { name: 'Start run' });
}

beforeEach(() => {
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
  localStorage.clear();
  setApiKey('test-key');
});

afterEach(() => {
  clearApiKey();
  localStorage.clear();
  vi.unstubAllGlobals();
});

describe('EngineRunsPage', () => {
  it('requests engine-only traces and renders engine status labels', async () => {
    mockEngineRequests();

    renderEngineRunsPage();

    expect(await screen.findByText('darklaunch.demo · v1')).toBeInTheDocument();
    expect(screen.getByText('Queued')).toBeInTheDocument();
    expect(screen.queryByText('darklaunch.demo · vv1')).not.toBeInTheDocument();

    await waitFor(() => {
      const requestUrl = new URL(
        readRequestUrl(fetchMock.mock.calls[0]?.[0] as RequestInput),
        'http://localhost'
      );
      expect(requestUrl.pathname).toBe('/api/traces');
      expect(requestUrl.searchParams.get('engine_only')).toBe('true');
    });
  });

  it('renders quarantined runs with a quarantined status label and mismatch wait summary', async () => {
    mockEngineRequests({ traces: [QUARANTINED_ENGINE_TRACE] });

    renderEngineRunsPage();

    const table = await screen.findByRole('table');
    const status = await within(table).findByText('Quarantined');
    const row = status.closest('tr');
    expect(row).not.toBeNull();
    expect(
      within(row as HTMLTableRowElement).getByText(/Replay mismatch/)
    ).toBeInTheDocument();
    expect(
      within(row as HTMLTableRowElement).queryByText(/Waiting on engine state/)
    ).not.toBeInTheDocument();
  });

  it('filters runs by engine status through the URL-driven quarantined filter', async () => {
    const user = userEvent.setup();
    mockEngineRequests({ total: DEFAULT_PAGE_SIZE + 1 });

    const firstRender = renderEngineRunsPage();

    await screen.findByText('darklaunch.demo · v1');
    await user.click(screen.getByRole('button', { name: 'Next page' }));
    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(([input]) => {
          const url = new URL(readRequestUrl(input as RequestInput), 'http://localhost');
          return (
            url.pathname === '/api/traces' &&
            url.searchParams.get('offset') === String(DEFAULT_PAGE_SIZE)
          );
        })
      ).toBe(true);
    });

    const statusFilter = screen.getByRole('combobox', { name: /status/i });
    expect(statusFilter).toHaveDisplayValue('All statuses');
    expect(
      within(statusFilter).getByRole('option', { name: 'Quarantined' })
    ).toHaveValue('quarantined');

    await user.selectOptions(statusFilter, 'quarantined');

    await waitFor(() => {
      const filteredRequest = fetchMock.mock.calls
        .map(
          ([input]) =>
            new URL(readRequestUrl(input as RequestInput), 'http://localhost')
        )
        .find((url) => url.searchParams.get('engine_run_status') === 'quarantined');
      expect(filteredRequest).toBeDefined();
      expect(['0', null]).toContain(filteredRequest?.searchParams.get('offset'));
    });
    expect(
      new URLSearchParams(
        screen.getByTestId('probe-search').textContent ?? ''
      ).get('engine_run_status')
    ).toBe('quarantined');

    firstRender.unmount();
    fetchMock.mockClear();

    renderEngineRunsPage('/engine/runs?engine_run_status=quarantined');

    await waitFor(() => {
      const firstRequest = new URL(
        readRequestUrl(fetchMock.mock.calls[0]?.[0] as RequestInput),
        'http://localhost'
      );
      expect(firstRequest.searchParams.get('engine_run_status')).toBe('quarantined');
    });
  });

  it('sends engine_only with pagination params and renders the locked column order', async () => {
    mockEngineRequests();

    renderEngineRunsPage();

    expect(await screen.findByText('darklaunch.demo · v1')).toBeInTheDocument();
    const requestUrl = new URL(
      readRequestUrl(fetchMock.mock.calls[0]?.[0] as RequestInput),
      'http://localhost'
    );
    expect(requestUrl.searchParams.get('engine_only')).toBe('true');
    expect(requestUrl.searchParams.get('limit')).toBe(String(DEFAULT_PAGE_SIZE));
    expect(requestUrl.searchParams.get('offset')).toBeNull();
    expect(
      screen.getAllByRole('columnheader').map((header) => header.textContent?.trim())
    ).toEqual([
      'Status',
      'Definition',
      'Instance key',
      'Wait',
      'Pending',
      'Projection',
      'Updated',
      '→',
    ]);
  });

  it('navigates rows through project-scoped links carrying returnTo state', async () => {
    const user = userEvent.setup();
    const projectId = '123e4567-e89b-12d3-a456-426614174999';
    mockEngineRequests();

    renderEngineRunsPage(`/engine/runs?project_id=${projectId}`);

    const rowLink = await screen.findByRole('link', { name: `Open ${ENGINE_TRACE.id}` });
    expect(rowLink).toHaveAttribute(
      'href',
      `/traces/${ENGINE_TRACE.id}?project_id=${projectId}`
    );

    await user.click(rowLink);

    expect(screen.getByTestId('probe-pathname')).toHaveTextContent(
      `/traces/${ENGINE_TRACE.id}`
    );
    expect(screen.getByTestId('probe-search')).toHaveTextContent(`project_id=${projectId}`);
    expect(screen.getByTestId('probe-return-to')).toHaveTextContent(
      `/engine/runs?project_id=${projectId}`
    );
  });

  it('shows the empty state and opens the start dialog from it', async () => {
    const user = userEvent.setup();
    mockEngineRequests({ traces: [] });

    renderEngineRunsPage();

    const emptyHeading = await screen.findByRole('heading', { name: 'No engine runs yet' });
    const emptyState = emptyHeading.closest('.app-empty-state');
    expect(emptyState).not.toBeNull();
    await user.click(within(emptyState as HTMLElement).getByRole('button', { name: 'Start run' }));

    expect(await screen.findByRole('heading', { name: 'Start run' })).toBeInTheDocument();
    expect(screen.getByLabelText(/^Instance key/)).toBeInTheDocument();
  });
});

describe('StartEngineRunDialog', () => {
  it('generates an editable UUID idempotency key', async () => {
    const user = userEvent.setup();
    mockEngineRequests();
    renderEngineRunsPage();

    await openStartDialog(user);
    const requestKey = screen.getByLabelText(/^Idempotency key/) as HTMLInputElement;
    expect(requestKey.value).toMatch(
      /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i
    );

    await user.clear(requestKey);
    await user.type(requestKey, 'custom-request-key');
    expect(requestKey).toHaveValue('custom-request-key');
  });

  it('treats instance preflight 404 as available', async () => {
    const user = userEvent.setup();
    mockEngineRequests({
      instance: () =>
        jsonResponse({ code: 'not_found', message: 'Instance not found' }, 404),
    });
    renderEngineRunsPage();

    await openStartDialog(user);
    await user.type(screen.getByLabelText(/^Instance key/), 'available-key');
    await user.click(screen.getByRole('button', { name: 'Check' }));

    expect(await screen.findByText('Instance key is available.')).toBeInTheDocument();
  });

  it('warns when preflight finds an active instance', async () => {
    const user = userEvent.setup();
    mockEngineRequests({
      instance: () =>
        jsonResponse({
          instance_id: 'instance-id-1',
          instance_key: 'active-key',
          definition_name: 'checkout',
          status: 'active',
          current_run: {
            run_id: '123e4567-e89b-12d3-a456-426614174101',
            instance_key: 'active-key',
            definition_name: 'checkout',
            definition_version: 'v1',
            projection_state: 'up_to_date',
            status: 'RUNNING',
            created_at: '2026-03-14T10:00:00.000Z',
            updated_at: '2026-03-14T10:00:03.000Z',
            pending_work: {
              pending_activity_tasks: 0,
              pending_inbox_items: 0,
            },
          },
        }),
    });
    renderEngineRunsPage();

    await openStartDialog(user);
    await user.type(screen.getByLabelText(/^Instance key/), 'active-key');
    await user.click(screen.getByRole('button', { name: 'Check' }));

    expect(
      await screen.findByText('Active instance exists for checkout (RUNNING).')
    ).toBeInTheDocument();
  });

  it('keeps submit enabled when preflight fails', async () => {
    const user = userEvent.setup();
    mockEngineRequests({
      instance: () => jsonResponse({ code: 'server_error', message: 'Unavailable' }, 500),
    });
    renderEngineRunsPage();

    const dialogHeading = await openStartDialog(user);
    await user.type(screen.getByLabelText(/^Instance key/), 'unknown-key');
    await user.click(screen.getByRole('button', { name: 'Check' }));

    expect(
      await screen.findByText(/^Preflight unavailable\. Submit remains enabled\./)
    ).toBeInTheDocument();
    const form = dialogHeading.closest('form');
    expect(form).not.toBeNull();
    expect(
      within(form as HTMLFormElement).getByRole('button', { name: 'Start run' })
    ).not.toBeDisabled();
  });

  it('scopes version suggestions to the selected definition and resets version on name change', async () => {
    const user = userEvent.setup();
    const traces: Trace[] = [
      {
        ...ENGINE_TRACE,
        id: 'trace-def-a-v1',
        engine: { ...ENGINE_TRACE.engine!, definition_name: 'defA', definition_version: 'v1' },
      },
      {
        ...ENGINE_TRACE,
        id: 'trace-def-a-v2',
        engine: { ...ENGINE_TRACE.engine!, definition_name: 'defA', definition_version: 'v2' },
      },
      {
        ...ENGINE_TRACE,
        id: 'trace-def-b-v9',
        engine: { ...ENGINE_TRACE.engine!, definition_name: 'defB', definition_version: 'v9' },
      },
    ];
    mockEngineRequests({ traces });
    renderEngineRunsPage();

    await openStartDialog(user);
    await user.click(screen.getByRole('button', { name: 'Enter manually' }));
    const definitionName = screen.getByLabelText(/^Definition name/);
    const definitionVersion = screen.getByLabelText(/^Definition version/);
    await user.type(definitionName, 'defA');

    expect(
      Array.from(document.querySelectorAll('#engine-definition-versions option')).map(
        (option) => (option as HTMLOptionElement).value
      )
    ).toEqual(expect.arrayContaining(['v1', 'v2']));
    expect(document.querySelectorAll('#engine-definition-versions option')).toHaveLength(2);

    await user.type(definitionVersion, 'v1');
    await user.clear(definitionName);
    await user.type(definitionName, 'defB');

    expect(definitionVersion).toHaveValue('');
    expect(
      Array.from(document.querySelectorAll('#engine-definition-versions option')).map(
        (option) => (option as HTMLOptionElement).value
      )
    ).toEqual(['v9']);
  });

  it('submits the start request and navigates to the projected trace', async () => {
    const user = userEvent.setup();
    const startedRunId = '123e4567-e89b-12d3-a456-426614174102';
    const startedTrace: Trace = {
      ...ENGINE_TRACE,
      id: 'trace-uuid-new-1',
      engine: {
        ...ENGINE_TRACE.engine!,
        run_id: startedRunId,
        instance_key: 'checkout-42',
      },
    };
    mockEngineRequests({
      traces: [ENGINE_TRACE, startedTrace],
      instance: () =>
        jsonResponse({ code: 'not_found', message: 'Instance not found' }, 404),
      start: () =>
        jsonResponse({
          run_id: startedRunId,
          instance_key: 'checkout-42',
          trace_id: 'trace-new-1',
        }),
    });
    renderEngineRunsPage();

    const dialogHeading = await openStartDialog(user);
    await user.click(screen.getByRole('button', { name: 'Enter manually' }));
    const form = dialogHeading.closest('form');
    expect(form).not.toBeNull();
    const dialog = within(form as HTMLFormElement);
    const requestKey = (dialog.getByLabelText(/^Idempotency key/) as HTMLInputElement).value;
    await user.type(dialog.getByLabelText(/^Instance key/), 'checkout-42');
    await user.type(dialog.getByLabelText(/^Definition name/), 'checkout');
    await user.type(dialog.getByLabelText(/^Definition version/), 'v3');
    await user.click(dialog.getByRole('button', { name: 'Start run' }));

    await waitFor(() => {
      expect(screen.getByTestId('probe-pathname')).toHaveTextContent(
        '/traces/trace-uuid-new-1'
      );
    });
    const postCall = fetchMock.mock.calls.find(([input, init]) => {
      const url = new URL(readRequestUrl(input as RequestInput), 'http://localhost');
      return url.pathname === '/v1/engine/runs' && (init as RequestInit | undefined)?.method === 'POST';
    });
    expect(postCall).toBeDefined();
    const [postInput, postInit] = postCall as [RequestInput, RequestInit];
    expect((postInit.method ?? 'GET').toUpperCase()).toBe('POST');
    expect(new URL(readRequestUrl(postInput), 'http://localhost').pathname).toBe(
      '/v1/engine/runs'
    );
    expect(new Headers(postInit.headers).get('X-Continua-Engine-Preview')).not.toBeNull();
    expect(JSON.parse(postInit.body as string)).toEqual({
      instance_key: 'checkout-42',
      definition_name: 'checkout',
      definition_version: 'v3',
      request_key: requestKey,
    });
    const resolutionCall = fetchMock.mock.calls.find(([input]) => {
      const url = new URL(readRequestUrl(input as RequestInput), 'http://localhost');
      return url.searchParams.get('engine_run_id') === startedRunId;
    });
    expect(resolutionCall).toBeDefined();
    expect(screen.getByTestId('probe-return-to')).toHaveTextContent('/engine/runs');
  });
});

describe('StartEngineRunDialog definition picker', () => {
  it('fetches live definitions when the dialog opens and lists them in the picker', async () => {
    const user = userEvent.setup();
    mockEngineRequests();
    renderEngineRunsPage();

    await openStartDialog(user);

    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(([input, init]) => {
          const url = new URL(readRequestUrl(input as RequestInput), 'http://localhost');
          return (
            url.pathname === '/v1/engine/definitions' &&
            ((init as RequestInit | undefined)?.method ?? 'GET') === 'GET'
          );
        })
      ).toBe(true);
    });

    const definitionName = await screen.findByRole('combobox', {
      name: /^Definition name/,
    });
    expect(definitionName.tagName).toBe('SELECT');
    const picker = within(definitionName);
    expect(picker.getByRole('option', { name: /^defA(?:\s|$)/ })).toBeEnabled();
    expect(picker.getByRole('option', { name: /^defB(?:\s|$)/ })).toBeEnabled();
    expect(picker.getByRole('option', { name: /legacy.*not live/i })).toBeDisabled();
  });

  it('disables picker submission while definitions are loading', async () => {
    const user = userEvent.setup();
    mockEngineRequests({
      definitions: () => new Promise<Response>(() => undefined),
    });
    renderEngineRunsPage();

    const dialogHeading = await openStartDialog(user);
    const form = dialogHeading.closest('form');
    expect(form).not.toBeNull();
    const dialog = within(form as HTMLFormElement);

    expect(dialog.getByText('Loading registered definitions…')).toBeInTheDocument();
    expect(dialog.getByRole('button', { name: 'Start run' })).toBeDisabled();
  });

  it('scopes the version dropdown to the selected definition and resets it on change', async () => {
    const user = userEvent.setup();
    mockEngineRequests();
    renderEngineRunsPage();

    await openStartDialog(user);
    const definitionName = screen.getByLabelText(/^Definition name/);
    await screen.findByRole('option', { name: /^defA(?:\s|$)/ });
    await user.selectOptions(definitionName, 'defA');

    const definitionVersion = screen.getByLabelText(/^Definition version/);
    const defAVersionOptions = within(definitionVersion).getAllByRole('option');
    expect(
      defAVersionOptions
        .map((option) => (option as HTMLOptionElement).value)
        .filter(Boolean)
    ).toEqual(['v1', 'v2', 'v0']);
    expect(
      defAVersionOptions.find(
        (option) => (option as HTMLOptionElement).value === 'v0'
      )
    ).toBeDisabled();

    await user.selectOptions(definitionVersion, 'v2');
    expect(definitionVersion).toHaveValue('v2');
    await user.selectOptions(definitionName, 'defB');

    expect(definitionVersion).toHaveValue('');
    expect(
      within(definitionVersion)
        .getAllByRole('option')
        .map((option) => (option as HTMLOptionElement).value)
        .filter(Boolean)
    ).toEqual(['v9']);
  });

  it('builds the start payload from the picker selection', async () => {
    const user = userEvent.setup();
    const startedRunId = '123e4567-e89b-12d3-a456-426614174102';
    const startedTrace: Trace = {
      ...ENGINE_TRACE,
      id: 'trace-picker-1',
      engine: {
        ...ENGINE_TRACE.engine!,
        run_id: startedRunId,
        instance_key: 'picker-instance',
      },
    };
    mockEngineRequests({
      traces: [ENGINE_TRACE, startedTrace],
      instance: () =>
        jsonResponse({ code: 'not_found', message: 'Instance not found' }, 404),
      start: () =>
        jsonResponse({
          run_id: startedRunId,
          instance_key: 'picker-instance',
          trace_id: 'trace-picker-1',
        }),
    });
    renderEngineRunsPage();

    const dialogHeading = await openStartDialog(user);
    const form = dialogHeading.closest('form');
    expect(form).not.toBeNull();
    const dialog = within(form as HTMLFormElement);
    await screen.findByRole('option', { name: /^defA(?:\s|$)/ });
    await user.selectOptions(dialog.getByLabelText(/^Definition name/), 'defA');
    await user.selectOptions(dialog.getByLabelText(/^Definition version/), 'v2');
    await user.type(dialog.getByLabelText(/^Instance key/), 'picker-instance');
    await user.click(dialog.getByRole('button', { name: 'Check' }));
    expect(await screen.findByText('Instance key is available.')).toBeInTheDocument();
    const requestKey = (dialog.getByLabelText(/^Idempotency key/) as HTMLInputElement)
      .value;
    await user.click(dialog.getByRole('button', { name: 'Start run' }));

    let postCall: ReturnType<typeof fetchMock>['mock']['calls'][number] | undefined;
    await waitFor(() => {
      postCall = fetchMock.mock.calls.find(([input, init]) => {
        const url = new URL(readRequestUrl(input as RequestInput), 'http://localhost');
        return (
          url.pathname === '/v1/engine/runs' &&
          (init as RequestInit | undefined)?.method === 'POST'
        );
      });
      expect(postCall).toBeDefined();
    });
    expect(JSON.parse((postCall?.[1] as RequestInit).body as string)).toEqual({
      instance_key: 'picker-instance',
      definition_name: 'defA',
      definition_version: 'v2',
      request_key: requestKey,
    });
    await waitFor(() => {
      expect(screen.getByTestId('probe-pathname')).toHaveTextContent(
        '/traces/trace-picker-1'
      );
    });
    expect(screen.getByTestId('probe-return-to')).toHaveTextContent('/engine/runs');
  });

  it('cannot start a non-live definition through the picker', async () => {
    const user = userEvent.setup();
    mockEngineRequests();
    renderEngineRunsPage();

    const dialogHeading = await openStartDialog(user);
    const form = dialogHeading.closest('form');
    expect(form).not.toBeNull();
    const dialog = within(form as HTMLFormElement);
    expect(
      await dialog.findByRole('option', { name: /legacy.*not live/i })
    ).toBeDisabled();
    await user.type(dialog.getByLabelText(/^Instance key/), 'blocked-instance');
    await user.click(dialog.getByRole('button', { name: 'Start run' }));

    expect(
      await dialog.findByText(/(?:select|choose) a live definition/i)
    ).toBeInTheDocument();
    expect(
      fetchMock.mock.calls.some(([input, init]) => {
        const url = new URL(readRequestUrl(input as RequestInput), 'http://localhost');
        return url.pathname === '/v1/engine/runs' && init?.method === 'POST';
      })
    ).toBe(false);
  });

  it('shows liveness and last-published metadata for stale definitions', async () => {
    const user = userEvent.setup();
    mockEngineRequests();
    renderEngineRunsPage();

    await openStartDialog(user);

    expect(await screen.findByText(/legacy.*not live/i)).toBeInTheDocument();
    expect(await screen.findAllByText(/last published/i)).not.toHaveLength(0);
  });

  it('manual entry fallback still allows free-text definitions', async () => {
    const user = userEvent.setup();
    const startedRunId = '123e4567-e89b-12d3-a456-426614174103';
    const startedTrace: Trace = {
      ...ENGINE_TRACE,
      id: 'trace-manual-1',
      engine: {
        ...ENGINE_TRACE.engine!,
        run_id: startedRunId,
        instance_key: 'manual-instance',
      },
    };
    mockEngineRequests({
      traces: [ENGINE_TRACE, startedTrace],
      instance: () =>
        jsonResponse({ code: 'not_found', message: 'Instance not found' }, 404),
      start: () =>
        jsonResponse({
          run_id: startedRunId,
          instance_key: 'manual-instance',
          trace_id: 'trace-manual-1',
        }),
    });
    renderEngineRunsPage();

    const dialogHeading = await openStartDialog(user);
    await user.click(screen.getByRole('button', { name: 'Enter manually' }));
    const form = dialogHeading.closest('form');
    expect(form).not.toBeNull();
    const dialog = within(form as HTMLFormElement);
    const definitionName = dialog.getByLabelText(/^Definition name/);
    const definitionVersion = dialog.getByLabelText(/^Definition version/);
    expect(definitionName.tagName).toBe('INPUT');
    expect(definitionVersion.tagName).toBe('INPUT');
    await user.type(dialog.getByLabelText(/^Instance key/), 'manual-instance');
    await user.type(definitionName, 'custom-def');
    await user.type(definitionVersion, 'v42');
    const requestKey = (dialog.getByLabelText(/^Idempotency key/) as HTMLInputElement)
      .value;
    await user.click(dialog.getByRole('button', { name: 'Start run' }));

    let postCall: ReturnType<typeof fetchMock>['mock']['calls'][number] | undefined;
    await waitFor(() => {
      postCall = fetchMock.mock.calls.find(([input, init]) => {
        const url = new URL(readRequestUrl(input as RequestInput), 'http://localhost');
        return (
          url.pathname === '/v1/engine/runs' &&
          (init as RequestInit | undefined)?.method === 'POST'
        );
      });
      expect(postCall).toBeDefined();
    });
    expect(JSON.parse((postCall?.[1] as RequestInit).body as string)).toEqual({
      instance_key: 'manual-instance',
      definition_name: 'custom-def',
      definition_version: 'v42',
      request_key: requestKey,
    });
    await waitFor(() => {
      expect(screen.getByTestId('probe-pathname')).toHaveTextContent(
        '/traces/trace-manual-1'
      );
    });
  });

  it.each([
    {
      scenario: 'the definitions endpoint fails',
      definitions: () =>
        jsonResponse({ code: 'server_error', message: 'boom' }, 500),
    },
    {
      scenario: 'the definitions catalog is empty',
      definitions: () => jsonResponse({ definitions: [] }),
    },
  ])('falls back to manual entry when $scenario', async ({ definitions }) => {
    const user = userEvent.setup();
    mockEngineRequests({ definitions });
    renderEngineRunsPage();

    await openStartDialog(user);

    expect(
      await screen.findByText(
        /definitions.*(?:unavailable|failed)|(?:could not|unable to) load.*definitions/i
      )
    ).toBeInTheDocument();
    const definitionName = screen.getByLabelText(/^Definition name/);
    const definitionVersion = screen.getByLabelText(/^Definition version/);
    expect(definitionName.tagName).toBe('INPUT');
    expect(definitionVersion.tagName).toBe('INPUT');
    await user.type(definitionName, 'fallback-def');
    await user.type(definitionVersion, 'v7');
    expect(definitionName).toHaveValue('fallback-def');
    expect(definitionVersion).toHaveValue('v7');
  });
});
