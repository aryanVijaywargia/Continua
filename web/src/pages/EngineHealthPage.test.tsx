import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import {
  ApiError,
  fetchEngineHealth,
  type EngineHealthResponse,
} from '../api/client';
import { ThemeProvider } from '../hooks/ThemeProvider';
import { EngineHealthPage } from './EngineHealthPage';

vi.mock('../api/client', async () => {
  const actual = await vi.importActual<typeof import('../api/client')>('../api/client');
  return {
    ...actual,
    fetchEngineHealth: vi.fn(),
  };
});

const mockedFetchEngineHealth = vi.mocked(fetchEngineHealth);

const HEALTH_RESPONSE: EngineHealthResponse = {
  generated_at: '2026-07-16T10:00:00.000Z',
  projector: {
    lag_rows: 7,
    runs_catching_up: 2,
  },
  queues: {
    runs_ready: 3,
    activity_tasks_pending: 4,
    inbox_pending: 5,
  },
  workers: [
    {
      id: 'worker-a',
      last_claim_at: '2026-07-16T09:59:55.000Z',
      active_leases: 2,
      expired_leases: 0,
      status: 'active',
    },
    {
      id: 'worker-b',
      last_claim_at: '2026-07-16T09:50:00.000Z',
      active_leases: 0,
      expired_leases: 1,
      status: 'stale',
    },
  ],
  retention: {
    summary_only_runs: 6,
    journal_expired_runs: 8,
  },
};

function renderEngineHealthPage(initialEntry = '/') {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });

  const rendered = render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <MemoryRouter initialEntries={[initialEntry]}>
          <EngineHealthPage />
        </MemoryRouter>
      </ThemeProvider>
    </QueryClientProvider>
  );

  return { ...rendered, queryClient };
}

function healthResponse(overrides: Partial<EngineHealthResponse> = {}): EngineHealthResponse {
  return {
    ...HEALTH_RESPONSE,
    ...overrides,
  };
}

function closestStateElement(text: string): HTMLElement {
  const element = screen.getByText(text).closest('[data-state]');
  expect(element, `${text} should be inside an element with data-state`).not.toBeNull();
  return element as HTMLElement;
}

beforeEach(() => {
  mockedFetchEngineHealth.mockReset();
  mockedFetchEngineHealth.mockResolvedValue(healthResponse());
});

afterEach(() => {
  vi.useRealTimers();
});

describe('EngineHealthPage', () => {
  it('renders projector, queue, and worker metrics', async () => {
    renderEngineHealthPage();

    expect(await screen.findByText('Projector lag')).toBeInTheDocument();
    expect(screen.getByText('7')).toBeInTheDocument();
    expect(screen.getByText('Runs catching up')).toBeInTheDocument();
    expect(screen.getByText('2')).toBeInTheDocument();
    expect(screen.getByText('Runs ready')).toBeInTheDocument();
    expect(screen.getByText('3')).toBeInTheDocument();
    expect(screen.getByText('Activity tasks pending')).toBeInTheDocument();
    expect(screen.getByText('4')).toBeInTheDocument();
    expect(screen.getByText('Inbox pending')).toBeInTheDocument();
    expect(screen.getByText('5')).toBeInTheDocument();
    expect(screen.getByText('worker-a')).toBeInTheDocument();
    expect(screen.getByText('worker-b')).toBeInTheDocument();
  });

  it('scopes cached health data to the URL project', async () => {
    const projectId = '11111111-1111-4111-8111-111111111111';
    const { queryClient } = renderEngineHealthPage(
      `/tools/engine-health?project_id=${projectId}`
    );

    expect(await screen.findByText('Projector lag')).toBeInTheDocument();
    expect(queryClient.getQueryData(['engine-health', projectId])).toEqual(
      HEALTH_RESPONSE
    );
  });

  it('marks high projector lag as degraded', async () => {
    const firstRender = renderEngineHealthPage();

    expect(await screen.findByText('Projector lag')).toBeInTheDocument();
    expect(closestStateElement('Projector lag')).toHaveAttribute('data-state', 'warn');

    firstRender.unmount();
    mockedFetchEngineHealth.mockReset();
    mockedFetchEngineHealth.mockResolvedValue(
      healthResponse({
        projector: {
          lag_rows: 0,
          runs_catching_up: 0,
        },
      })
    );
    renderEngineHealthPage();

    expect(await screen.findByText('Projector lag')).toBeInTheDocument();
    expect(closestStateElement('Projector lag')).toHaveAttribute('data-state', 'ok');
  });

  it('marks stale workers as visually distinct', async () => {
    renderEngineHealthPage();

    expect(await screen.findByText('worker-a')).toBeInTheDocument();
    expect(closestStateElement('worker-a')).toHaveAttribute('data-state', 'active');
    expect(closestStateElement('worker-b')).toHaveAttribute('data-state', 'stale');
  });

  it('polls for fresh health data', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    renderEngineHealthPage();

    await waitFor(() => {
      expect(mockedFetchEngineHealth).toHaveBeenCalledTimes(1);
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(5_100);
    });

    await waitFor(() => {
      expect(mockedFetchEngineHealth.mock.calls.length).toBeGreaterThanOrEqual(2);
    });
  });

  it('surfaces fetch errors with the request line and a retry', async () => {
    const user = userEvent.setup();
    mockedFetchEngineHealth.mockRejectedValue(new ApiError(503, 'error', 'request timed out'));

    renderEngineHealthPage('/tools/engine-health?project_id=11111111-1111-4111-8111-111111111111');

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Could not load engine health');
    expect(alert).toHaveTextContent('The engine API is enabled but the request failed.');
    expect(alert).toHaveTextContent('No successful read is available yet.');
    expect(alert).toHaveTextContent('GET /v1/engine/health · 503 · request timed out');
    expect(within(alert).getByRole('link', { name: 'Back to Engine Runs' })).toHaveAttribute(
      'href',
      '/engine/runs?project_id=11111111-1111-4111-8111-111111111111'
    );
    expect(screen.queryByText('Projector lag')).not.toBeInTheDocument();

    mockedFetchEngineHealth.mockResolvedValue(healthResponse());
    await user.click(within(alert).getByRole('button', { name: 'Retry' }));

    expect(await screen.findByText('Projector lag')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('names a network failure without claiming the server answered', async () => {
    mockedFetchEngineHealth.mockRejectedValue(new TypeError('Failed to fetch'));

    renderEngineHealthPage();

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('The request failed before the server answered.');
    expect(alert).toHaveTextContent('GET /v1/engine/health · no response · Failed to fetch');
  });

  it('keeps the last successful read visible as stale when a later poll fails', async () => {
    const { queryClient } = renderEngineHealthPage();

    expect(await screen.findByText('Projector lag')).toBeInTheDocument();

    mockedFetchEngineHealth.mockRejectedValue(new ApiError(500, 'error', 'projector query failed'));
    await act(async () => {
      await queryClient.refetchQueries({ queryKey: ['engine-health', null] });
    });

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent(
      'The last successful read was less than a minute ago, shown below as stale rather than hidden.'
    );
    expect(alert).toHaveTextContent('GET /v1/engine/health · 500 · projector query failed');
    expect(screen.getByText('Projector lag')).toBeInTheDocument();
    expect(screen.getByText('7')).toBeInTheDocument();
    expect(screen.getAllByText(/stale · less than a minute old/)).toHaveLength(7);
  });

  it('shows the disabled capability as a configuration state without retrying', async () => {
    mockedFetchEngineHealth.mockRejectedValue(new ApiError(404, 'not_found', 'Resource not found'));

    renderEngineHealthPage('/tools/engine-health?project_id=11111111-1111-4111-8111-111111111111');

    const heading = await screen.findByRole('heading', {
      name: 'Engine health is not enabled on this server',
    });
    const card = heading.closest('[data-state="capability-disabled"]') as HTMLElement;
    expect(card).not.toBeNull();
    const scoped = within(card);
    expect(scoped.getByText(/A configuration state, not an error\./)).toBeInTheDocument();
    expect(scoped.getByText('engine api · disabled · returned 404')).toBeInTheDocument();
    expect(scoped.getByRole('link', { name: 'Back to Traces' })).toHaveAttribute(
      'href',
      '/traces?project_id=11111111-1111-4111-8111-111111111111'
    );
    expect(
      scoped.getByRole('link', { name: /How to enable the engine API/ })
    ).toHaveAttribute('href', 'https://www.continua.in/docs/debugger/engine-runs');
    expect(screen.queryByRole('button', { name: 'Retry' })).not.toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
    expect(screen.queryByText('Could not load engine health')).not.toBeInTheDocument();
    expect(mockedFetchEngineHealth).toHaveBeenCalledTimes(1);
  });

  it('stops polling once the engine API reports disabled', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    mockedFetchEngineHealth.mockRejectedValue(new ApiError(404, 'not_found', 'Resource not found'));
    renderEngineHealthPage();

    expect(
      await screen.findByRole('heading', { name: 'Engine health is not enabled on this server' })
    ).toBeInTheDocument();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(15_000);
    });

    expect(mockedFetchEngineHealth).toHaveBeenCalledTimes(1);
  });
});
