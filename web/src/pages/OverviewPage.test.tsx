import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { clearApiKey, setApiKey } from '../api/client';
import { TRACE_THREE } from '../test/traceFixtures';
import {
  buildFetchHandler,
  getRequests,
  jsonResponse,
  readRequestUrl,
  renderTraceRoutes,
} from './testUtils';

let fetchMock: ReturnType<typeof vi.fn>;

type Handler = ReturnType<typeof buildFetchHandler>;

/** Adds the project and engine-health routes that the base handler does not serve. */
function withProjectAndEngineHealth(base: Handler): Handler {
  return async (input, init) => {
    const url = new URL(readRequestUrl(input), 'http://localhost');
    if (url.pathname === '/api/projects') {
      return jsonResponse({
        projects: [
          {
            id: 'project-1',
            name: 'Demo Project',
            created_at: '2026-03-01T00:00:00.000Z',
            updated_at: '2026-03-01T00:00:00.000Z',
          },
        ],
        authenticated_project_id: 'project-1',
      });
    }
    if (url.pathname === '/v1/engine/health') {
      return jsonResponse({
        generated_at: '2026-03-14T12:00:00.000Z',
        projector: { lag_rows: 0, runs_catching_up: 0 },
        queues: { runs_ready: 0, activity_tasks_pending: 0, inbox_pending: 0 },
        workers: [],
        retention: { summary_only_runs: 0, journal_expired_runs: 0 },
      });
    }
    return base(input, init);
  };
}

const COMPLETED_TRACE = { ...TRACE_THREE, status: 'COMPLETED' as const, error_count: 0 };

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

describe('OverviewPage', () => {
  it('lists failures first and renders recent traces, durations, and coverage', async () => {
    fetchMock.mockImplementation(buildFetchHandler());

    renderTraceRoutes(['/dashboard']);

    expect(await screen.findByRole('heading', { name: 'Recent traces' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Needs attention' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Duration by trace' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Data coverage' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /View all/i })).toHaveAttribute('href', '/traces');

    const attention = screen.getByRole('region', { name: 'Needs attention' });
    const attentionLinks = await within(attention).findAllByRole('link');
    expect(attentionLinks[0]).toHaveTextContent('Checkout Trace');
    expect(attentionLinks[0]).toHaveTextContent('Failed · 2 failed steps');
    expect(within(attention).getByText('Running · no end time recorded yet')).toBeInTheDocument();
    expect(
      within(attention).queryByText('Nothing needs attention in this range')
    ).not.toBeInTheDocument();

    // Engine health is not served by the base handler, so it must read as unavailable.
    expect(
      await within(attention).findByText('engine projections — unavailable')
    ).toBeInTheDocument();

    // Too few recorded durations must not be drawn as a trend.
    expect(screen.getByText(/points? (is|are) not a trend/)).toBeInTheDocument();

    // No decorative sparklines or invented tiles remain.
    expect(screen.queryByText('Tracked traces')).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Trace volume' })).not.toBeInTheDocument();
  });

  it('shows the green empty state with recorded counts when nothing needs attention', async () => {
    fetchMock.mockImplementation(
      withProjectAndEngineHealth(
        buildFetchHandler({
          list: (url) =>
            url.searchParams.has('status') || url.searchParams.has('has_errors')
              ? jsonResponse({ traces: [], total: 0 })
              : jsonResponse({ traces: [COMPLETED_TRACE], total: 1 }),
        })
      )
    );

    renderTraceRoutes(['/dashboard']);

    expect(
      await screen.findByText('Nothing needs attention in this range')
    ).toBeInTheDocument();
    expect(await screen.findByRole('heading', { name: 'Demo Project' })).toBeInTheDocument();
    expect(
      await screen.findByText('engine projections — up to date')
    ).toBeInTheDocument();
    expect(screen.getByText(/with failed steps across 1 traces/)).toBeInTheDocument();
  });

  it('stores the date range in the URL and scopes trace requests to it', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(buildFetchHandler());

    const { router } = renderTraceRoutes(['/dashboard']);
    expect(await screen.findByRole('heading', { name: 'Recent traces' })).toBeInTheDocument();

    await user.selectOptions(screen.getByLabelText('Date range'), '7d');

    expect(router.state.location.search).toBe('?range=7d');
    const viewAll = screen.getByRole('link', { name: /View all/i });
    expect(viewAll.getAttribute('href')).toMatch(/^\/traces\?.*start_time_from=/);
    expect(
      getRequests(fetchMock, '/api/traces').some((url) => url.searchParams.has('start_time_from'))
    ).toBe(true);
  });

  it('keeps overview content visible when one supporting query fails', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionsList: () => jsonResponse({ message: 'Session list unavailable' }, 500),
      })
    );

    renderTraceRoutes(['/dashboard']);

    expect(await screen.findByText(/Overview data is partially unavailable/i)).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Recent traces' })).toBeInTheDocument();
    expect(screen.getAllByText('Checkout Trace').length).toBeGreaterThan(0);
    expect(screen.getByText(/Session list unavailable/)).toBeInTheDocument();
    expect(screen.queryByText('No sessions yet')).not.toBeInTheDocument();
  });
});
