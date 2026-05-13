import { act, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { clearApiKey, setApiKey } from '../api/client';
import { downloadJsonFile } from '../utils/downloadJson';
import {
  RUNNING_SESSION_NARRATIVE,
  SESSION_EXTERNAL_ID,
  SESSION_ID,
  SESSION_NARRATIVE,
  SESSION_ONE,
  TRACE_ONE,
  TRACE_TWO,
  buildFetchHandler,
  createSessionNarrativeTrace,
  createDeferredResponse,
  getRequests,
  jsonResponse,
  renderTraceRoutes,
} from './testUtils';

vi.mock('../utils/downloadJson', () => ({
  downloadJsonFile: vi.fn(),
}));

let fetchMock: ReturnType<typeof vi.fn>;

function getTraceListRequests(): URL[] {
  return getRequests(fetchMock, '/api/traces');
}

function getSessionNarrativeRequests(): URL[] {
  return getRequests(fetchMock, `/api/sessions/${SESSION_ID}/narrative`);
}

async function waitForSessionTraceFetch(search: string) {
  await waitFor(() => {
    expect(getTraceListRequests().at(-1)?.search).toBe(search);
  });
}

beforeEach(() => {
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
});

describe('SessionDetailPage', () => {
  const journeyBaselineId = '423e4567-e89b-12d3-a456-426614174000';
  const journeyCandidateId = '423e4567-e89b-12d3-a456-426614174001';
  const journeyNarrative = {
    ...SESSION_NARRATIVE,
    traces: [
      createSessionNarrativeTrace({
        ...SESSION_NARRATIVE.traces[0],
        id: journeyBaselineId,
        name: 'Alpha Narrative',
        status: 'COMPLETED',
      }),
      createSessionNarrativeTrace({
        ...SESSION_NARRATIVE.traces[1],
        id: journeyCandidateId,
        name: 'Checkout Narrative',
        status: 'FAILED',
      }),
    ],
  };

  it('renders the debugger-kit session header, metrics, tabs, and journey rail', async () => {
    fetchMock.mockImplementation(buildFetchHandler());

    renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    expect(await screen.findByRole('heading', { name: 'Checkout Session' })).toBeInTheDocument();
    expect(screen.getAllByText(SESSION_EXTERNAL_ID).length).toBeGreaterThan(0);
    expect(screen.getByText('Success rate')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Journey/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Traces/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Context/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Feedback/i })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Session journey' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Alpha Narrative' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Checkout Narrative' })).toBeInTheDocument();
    expect(screen.getByText('Latency by trace')).toBeInTheDocument();
    expect(screen.queryByLabelText('Search')).not.toBeInTheDocument();
    await waitForSessionTraceFetch(`?limit=20&session_id=${SESSION_ID}`);
  });

  it('keeps narrative loading and error states local to the redesigned journey area', async () => {
    const deferredNarrative = createDeferredResponse();
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionNarrative: () => deferredNarrative.promise,
      })
    );

    renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    expect(await screen.findByRole('heading', { name: 'Checkout Session' })).toBeInTheDocument();
    expect(screen.getByText('Loading session journey...')).toBeInTheDocument();

    deferredNarrative.resolve(jsonResponse(SESSION_NARRATIVE));
    expect(await screen.findByRole('link', { name: 'Alpha Narrative' })).toBeInTheDocument();

    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionNarrative: () => jsonResponse({ message: 'Narrative failed' }, 500),
      })
    );
  });

  it('shows an inline narrative error without breaking the shell', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionNarrative: () => jsonResponse({ message: 'Narrative failed' }, 500),
      })
    );

    renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    expect(await screen.findByRole('heading', { name: 'Checkout Session' })).toBeInTheDocument();
    expect(await screen.findByText(/Error loading narrative:/)).toHaveTextContent(
      'Narrative failed'
    );
  });

  it('renders the trace table tab with URL-backed sort and pagination state', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(
      buildFetchHandler({
        list: (url) =>
          jsonResponse({
            traces: url.searchParams.get('limit') === '50' ? [TRACE_TWO] : [TRACE_ONE],
            total: 60,
          }),
      })
    );

    const { router } = renderTraceRoutes([
      `/sessions/${SESSION_ID}?sort_by=started_at&sort_dir=asc&limit=50&offset=20`,
    ]);

    await screen.findByRole('heading', { name: 'Checkout Session' });
    await user.click(screen.getByRole('button', { name: /Traces/i }));
    expect(await screen.findByRole('link', { name: 'Latency Trace' })).toBeInTheDocument();
    await waitForSessionTraceFetch(
      `?limit=50&offset=20&session_id=${SESSION_ID}&sort_by=started_at&sort_dir=asc`
    );

    await user.click(screen.getByRole('button', { name: 'Started' }));
    await waitForSessionTraceFetch(
      `?limit=50&session_id=${SESSION_ID}&sort_by=started_at&sort_dir=desc`
    );
    await waitFor(() => {
      expect(router.state.location.search).toBe('?limit=50&sort_by=started_at&sort_dir=desc');
    });

    await user.selectOptions(screen.getByRole('combobox', { name: 'Rows per page' }), '20');
    await waitForSessionTraceFetch(
      `?limit=20&session_id=${SESSION_ID}&sort_by=started_at&sort_dir=desc`
    );
  });

  it('tracks compare selections from the journey in the URL and opens comparison', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionNarrative: () => jsonResponse(journeyNarrative),
      })
    );

    const { router } = renderTraceRoutes([
      `/sessions/${SESSION_ID}?sort_by=started_at&sort_dir=asc&offset=20`,
    ]);

    expect(await screen.findByRole('link', { name: 'Alpha Narrative' })).toBeInTheDocument();
    const alphaCard = screen.getByRole('link', { name: 'Alpha Narrative' }).closest('.grid');
    const checkoutCard = screen.getByRole('link', { name: 'Checkout Narrative' }).closest('.grid');
    if (!alphaCard || !checkoutCard) {
      throw new Error('Expected journey trace cards');
    }
    await user.click(within(alphaCard as HTMLElement).getByRole('button', { name: 'Set baseline' }));
    await user.click(within(checkoutCard as HTMLElement).getByRole('button', { name: 'Set candidate' }));

    await waitFor(() => {
      expect(router.state.location.search).toContain(`baseline_trace_id=${journeyBaselineId}`);
      expect(router.state.location.search).toContain(`candidate_trace_id=${journeyCandidateId}`);
      expect(router.state.location.search).toContain('sort_by=started_at');
      expect(router.state.location.search).toContain('sort_dir=asc');
      expect(router.state.location.search).toContain('offset=20');
    });

    expect(screen.getByRole('link', { name: 'Open comparison' })).toHaveAttribute(
      'href',
      `/sessions/${SESSION_ID}/compare?baseline_trace_id=${journeyBaselineId}&candidate_trace_id=${journeyCandidateId}`
    );
  });

  it('keeps compare buttons disabled for running traces', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionNarrative: () => jsonResponse(RUNNING_SESSION_NARRATIVE),
      })
    );

    renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    expect(await screen.findByRole('link', { name: 'Alpha Narrative' })).toBeInTheDocument();
    expect(screen.getAllByRole('button', { name: 'Set baseline' })[0]).toBeDisabled();
    expect(screen.getAllByRole('button', { name: 'Set candidate' })[0]).toBeDisabled();
  });

  it('filters the journey without mutating URL-backed table state', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(buildFetchHandler());

    const { router } = renderTraceRoutes([`/sessions/${SESSION_ID}?limit=50&offset=20`]);

    expect(await screen.findByRole('link', { name: 'Alpha Narrative' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Failed' }));

    expect(screen.queryByRole('link', { name: 'Alpha Narrative' })).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Checkout Narrative' })).toBeInTheDocument();
    expect(router.state.location.search).toBe('?limit=50&offset=20');
  });

  it('preserves returnTo when navigating from the journey to trace detail', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(buildFetchHandler());

    renderTraceRoutes([
      `/sessions/${SESSION_ID}?sort_by=started_at&sort_dir=asc&offset=20`,
    ]);

    expect(await screen.findByRole('link', { name: 'Checkout Narrative' })).toBeInTheDocument();
    await user.click(screen.getByRole('link', { name: 'Checkout Narrative' }));

    expect(await screen.findByRole('button', { name: /Trace Context/i })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '← Session' })).toHaveAttribute(
      'href',
      `/sessions/${SESSION_ID}?offset=20&sort_by=started_at&sort_dir=asc`
    );
  });

  it('renders context and feedback tabs from session metadata', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionDetail: () =>
          jsonResponse({
            ...SESSION_ONE,
            metadata: {
              plan: 'premium',
              feedback: [
                {
                  score: 'positive',
                  text: 'Checkout recovery worked after retry.',
                  trace: 'Checkout Narrative',
                  user: 'user-123',
                },
              ],
            },
          }),
      })
    );

    renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    expect(await screen.findByRole('heading', { name: 'Checkout Session' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /Context/i }));
    expect(screen.getByText('plan')).toBeInTheDocument();
    expect(screen.getByText('premium')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /Feedback/i }));
    expect(screen.getByText('Checkout recovery worked after retry.')).toBeInTheDocument();
    expect(screen.getByText('Checkout Narrative')).toBeInTheDocument();
  });

  it('exports the current session, narrative, traces, and compare state as JSON', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionNarrative: () => jsonResponse(journeyNarrative),
      })
    );

    renderTraceRoutes([`/sessions/${SESSION_ID}?baseline_trace_id=${journeyBaselineId}`]);

    expect(await screen.findByRole('link', { name: 'Alpha Narrative' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Export' }));

    expect(downloadJsonFile).toHaveBeenCalledWith(
      `continua-session-${SESSION_ID}.json`,
      expect.objectContaining({
        session: expect.objectContaining({ id: SESSION_ID }),
        narrative: expect.objectContaining({
          summary: expect.objectContaining({ total_trace_count: 2 }),
        }),
        traces: expect.arrayContaining([expect.objectContaining({ id: TRACE_ONE.id })]),
        compare: expect.objectContaining({ baseline_trace_id: journeyBaselineId }),
      })
    );
  });

  it('polls the narrative while running traces exist and stops when none remain', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionNarrative: () => jsonResponse(RUNNING_SESSION_NARRATIVE),
      })
    );

    renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    expect(await screen.findByRole('link', { name: 'Alpha Narrative' })).toBeInTheDocument();
    await waitFor(() => {
      expect(getSessionNarrativeRequests()).toHaveLength(1);
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000);
    });
    await waitFor(() => {
      expect(getSessionNarrativeRequests()).toHaveLength(2);
    });
  }, 10_000);

  it('shows an auth recovery banner when the session request returns 401', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionDetail: () => jsonResponse({ message: 'Invalid or missing API key' }, 401),
      })
    );

    renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    expect(await screen.findByRole('alert')).toHaveTextContent('Invalid or missing API key');
    expect(screen.getByRole('link', { name: 'Sign in again' })).toHaveAttribute(
      'href',
      `/sessions/${SESSION_ID}`
    );
  });

  it('supports compare selection from the trace table tab', async () => {
    const user = userEvent.setup();
    const baselineTrace = {
      ...TRACE_ONE,
      id: '323e4567-e89b-12d3-a456-426614174000',
      session_id: SESSION_ID,
      name: 'Checkout Compare Trace',
      status: 'FAILED' as const,
    };
    const candidateTrace = {
      ...TRACE_ONE,
      id: '323e4567-e89b-12d3-a456-426614174001',
      session_id: SESSION_ID,
      name: 'Alpha Compare Trace',
      status: 'COMPLETED' as const,
    };
    fetchMock.mockImplementation(
      buildFetchHandler({
        list: () => jsonResponse({ traces: [baselineTrace, candidateTrace], total: 2 }),
      })
    );

    const { router } = renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    await screen.findByRole('heading', { name: 'Checkout Session' });
    await user.click(screen.getByRole('button', { name: /Traces/i }));
    expect(await screen.findByRole('link', { name: 'Checkout Compare Trace' })).toBeInTheDocument();

    const table = screen.getByRole('table');
    const baselineRow = within(table).getByRole('link', { name: 'Checkout Compare Trace' }).closest('tr');
    const candidateRow = within(table).getByRole('link', { name: 'Alpha Compare Trace' }).closest('tr');
    if (!baselineRow || !candidateRow) {
      throw new Error('Expected compare rows');
    }

    await user.click(within(baselineRow).getByRole('button', { name: 'Base' }));
    await user.click(within(candidateRow).getByRole('button', { name: 'Cand' }));

    await waitFor(() => {
      expect(router.state.location.search).toContain(`baseline_trace_id=${baselineTrace.id}`);
      expect(router.state.location.search).toContain(`candidate_trace_id=${candidateTrace.id}`);
    });
  });

  it('strips malformed compare params while preserving valid table params', async () => {
    fetchMock.mockImplementation(buildFetchHandler());

    const { router } = renderTraceRoutes([
      `/sessions/${SESSION_ID}?baseline_trace_id=bad&candidate_trace_id=also-bad&limit=50`,
    ]);

    expect(await screen.findByRole('heading', { name: 'Checkout Session' })).toBeInTheDocument();
    await waitFor(() => {
      expect(router.state.location.search).toBe('?limit=50');
    });
  });
});
