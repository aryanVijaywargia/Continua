import { act, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { clearApiKey, setApiKey } from '../api/client';
import {
  EMPTY_SESSION_NARRATIVE,
  RUNNING_SESSION_NARRATIVE,
  SESSION_EXTERNAL_ID,
  SESSION_ID,
  SESSION_NARRATIVE,
  SESSION_ONE,
  TRUNCATED_SESSION_NARRATIVE,
  TRACE_ONE,
  TRACE_TWO,
  buildFetchHandler,
  createDeferredResponse,
  createSessionNarrativeTrace,
  getRequests,
  jsonResponse,
  renderTraceRoutes,
} from './testUtils';

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
  it('renders external-first identity and hides the trace filter bar', async () => {
    fetchMock.mockImplementation(buildFetchHandler());

    renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    expect(await screen.findByRole('heading', { name: SESSION_EXTERNAL_ID })).toBeInTheDocument();
    expect(screen.getByText(SESSION_ID)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Copy session external ID' })).toBeInTheDocument();
    expect(screen.queryByLabelText('Search')).not.toBeInTheDocument();
    await waitForSessionTraceFetch(`?limit=20&session_id=${SESSION_ID}`);
  });

  it('shows the auth recovery banner when the session request returns 401', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionDetail: () => jsonResponse({ message: 'Invalid or missing API key' }, 401),
      })
    );

    renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    expect(await screen.findByRole('alert')).toHaveTextContent('Invalid or missing API key');
    expect(screen.getByRole('link', { name: 'Go to Settings' })).toHaveAttribute(
      'href',
      '/settings'
    );
  });

  it('rehydrates URL state, toggles sorting, and updates page size', async () => {
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

    expect(await screen.findByText('Latency Trace')).toBeInTheDocument();
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

  it('passes the full session detail URL as returnTo and trace detail accepts it', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(buildFetchHandler());

    renderTraceRoutes([
      `/sessions/${SESSION_ID}?sort_by=started_at&sort_dir=asc&offset=20`,
    ]);

    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();
    await user.click(screen.getByRole('link', { name: 'Checkout Trace' }));

    expect(await screen.findByText('Trace Context')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '← Session' })).toHaveAttribute(
      'href',
      `/sessions/${SESSION_ID}?offset=20&sort_by=started_at&sort_dir=asc`
    );
  });

  it('keeps narrative loading local to the summary and storyline area', async () => {
    const deferredNarrative = createDeferredResponse();
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionNarrative: () => deferredNarrative.promise,
      })
    );

    renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    expect(await screen.findByRole('heading', { name: SESSION_EXTERNAL_ID })).toBeInTheDocument();
    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();
    expect(screen.getByText('Loading narrative...')).toBeInTheDocument();

    deferredNarrative.resolve(jsonResponse(SESSION_NARRATIVE));
    expect(await screen.findByRole('heading', { name: 'Session Narrative' })).toBeInTheDocument();
  });

  it('shows an inline narrative error without breaking the header or trace table', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionNarrative: () => jsonResponse({ message: 'Narrative failed' }, 500),
      })
    );

    renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    expect(await screen.findByRole('heading', { name: SESSION_EXTERNAL_ID })).toBeInTheDocument();
    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();
    expect(await screen.findByText(/Error loading narrative:/)).toHaveTextContent(
      'Narrative failed'
    );
  });

  it('renders the narrative summary above the trace table and keeps storyline ordering stable', async () => {
    const tieNarrative = {
      summary: {
        ...SESSION_NARRATIVE.summary,
        total_trace_count: 2,
        returned_trace_count: 2,
      },
      traces: [
        createSessionNarrativeTrace({
          id: 'narrative-a',
          trace_id: 'external-a',
          name: 'Narrative A',
          started_at: '2026-03-14T09:00:00.000Z',
          latest_activity_at: '2026-03-14T09:00:10.000Z',
          lineage: { type: 'unlinked' },
        }),
        createSessionNarrativeTrace({
          id: 'narrative-b',
          trace_id: 'external-b',
          name: 'Narrative B',
          started_at: '2026-03-14T09:00:00.000Z',
          latest_activity_at: '2026-03-14T09:00:11.000Z',
          lineage: { type: 'inferred', parent_trace_id: 'external-a' },
        }),
      ],
    };

    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionNarrative: () => jsonResponse(tieNarrative),
      })
    );

    renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    const summaryHeading = await screen.findByRole('heading', { name: 'Session Narrative' });
    const tracesHeading = screen.getByRole('heading', { name: 'Traces' });
    expect(
      Boolean(summaryHeading.compareDocumentPosition(tracesHeading) & Node.DOCUMENT_POSITION_FOLLOWING)
    ).toBe(true);

    expect(
      screen.getAllByRole('link', { name: /Narrative [AB]/ }).map((link) => link.textContent)
    ).toEqual(['Narrative A', 'Narrative B']);
  });

  it('renders lineage badges, truncated coverage copy, and the truncation banner', async () => {
    const narrativeWithBadges = {
      summary: {
        ...TRUNCATED_SESSION_NARRATIVE.summary,
        explicit_link_count: 1,
        inferred_link_count: 1,
        unlinked_trace_count: 1,
      },
      traces: [
        createSessionNarrativeTrace({
          id: 'story-explicit',
          trace_id: 'story-explicit-external',
          name: 'Story Explicit',
          lineage: {
            type: 'explicit',
            parent_trace_id: 'root-story',
            trigger_span_id: 'span-1',
            link_kind: 'handoff',
          },
        }),
        createSessionNarrativeTrace({
          id: 'story-inferred',
          trace_id: 'story-inferred-external',
          name: 'Story Inferred',
          lineage: { type: 'inferred', parent_trace_id: 'root-story' },
        }),
        createSessionNarrativeTrace({
          id: 'story-unlinked',
          trace_id: 'story-unlinked-external',
          name: 'Story Unlinked',
          lineage: { type: 'unlinked' },
        }),
      ],
    };

    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionNarrative: () => jsonResponse(narrativeWithBadges),
      })
    );

    renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    expect(await screen.findByText('Explicit')).toBeInTheDocument();
    expect(screen.getByText('Inferred')).toBeInTheDocument();
    expect(screen.getByText('Unlinked')).toBeInTheDocument();
    expect(screen.getByText(/Lineage coverage applies to the first 100 traces shown\./)).toBeInTheDocument();
    expect(screen.getByText(/Narrative limited to the first 100 traces\./)).toBeInTheDocument();
  });

  it('shows a compact zero-trace narrative placeholder above the existing empty table state', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionDetail: () =>
          jsonResponse({
            ...SESSION_ONE,
            trace_count: 0,
          }),
        sessionNarrative: () => jsonResponse(EMPTY_SESSION_NARRATIVE),
        list: () => jsonResponse({ traces: [], total: 0 }),
      })
    );

    renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    expect(await screen.findByText('No narrative yet')).toBeInTheDocument();
    expect(screen.queryByLabelText('Session narrative summary')).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Storyline' })).not.toBeInTheDocument();
    expect(screen.queryByText('Returned / Total')).not.toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'No traces in this session' })).toBeInTheDocument();
  });

  it('polls the narrative every 30 seconds while running traces exist', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionNarrative: () => jsonResponse(RUNNING_SESSION_NARRATIVE),
      })
    );

    renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    expect(await screen.findByRole('heading', { name: 'Session Narrative' })).toBeInTheDocument();
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

  it('does not poll the narrative when no running traces remain', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionNarrative: () => jsonResponse(SESSION_NARRATIVE),
      })
    );

    renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    expect(await screen.findByRole('heading', { name: 'Session Narrative' })).toBeInTheDocument();
    await waitFor(() => {
      expect(getSessionNarrativeRequests()).toHaveLength(1);
    });

    await act(async () => {
      await vi.advanceTimersByTimeAsync(30_000);
    });
    expect(getSessionNarrativeRequests()).toHaveLength(1);
  }, 10_000);

  it('preserves returnTo when navigating from the storyline to trace detail', async () => {
    const user = userEvent.setup();
    const narrative = {
      ...SESSION_NARRATIVE,
      traces: [
        createSessionNarrativeTrace({
          id: TRACE_ONE.id,
          trace_id: 'external-story-link',
          name: 'Storyline Route Trace',
          lineage: { type: 'unlinked' },
        }),
      ],
    };

    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionNarrative: () => jsonResponse(narrative),
      })
    );

    renderTraceRoutes([
      `/sessions/${SESSION_ID}?sort_by=started_at&sort_dir=asc&offset=20`,
    ]);

    expect(await screen.findByRole('link', { name: 'Storyline Route Trace' })).toBeInTheDocument();
    await user.click(screen.getByRole('link', { name: 'Storyline Route Trace' }));

    expect(await screen.findByText('Trace Context')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '← Session' })).toHaveAttribute(
      'href',
      `/sessions/${SESSION_ID}?offset=20&sort_by=started_at&sort_dir=asc`
    );
  });
});
