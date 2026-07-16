import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import {
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

  it('surfaces fetch errors', async () => {
    mockedFetchEngineHealth.mockRejectedValue(new Error('health unavailable'));

    renderEngineHealthPage();

    expect(await screen.findByRole('alert')).toHaveTextContent('health unavailable');
  });
});
