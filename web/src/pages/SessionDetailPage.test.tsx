import { act, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { clearApiKey, setApiKey } from '../api/client';
import {
  EMPTY_SESSION_NARRATIVE,
  OTHER_SESSION_ID,
  RUNNING_SESSION_NARRATIVE,
  SESSION_EXTERNAL_ID,
  SESSION_ID,
  SESSION_NARRATIVE,
  SESSION_ONE,
  TRUNCATED_SESSION_NARRATIVE,
  TRACE_DETAIL,
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

function getTraceRow(name: string): HTMLElement {
  const row = within(screen.getByRole('table')).getByRole('link', { name }).closest('tr');
  if (!row) {
    throw new Error(`Expected a table row for ${name}`);
  }
  return row;
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

  it('shows engine badges for engine-backed traces in the session table', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        list: () =>
          jsonResponse({
            traces: [
              {
                ...TRACE_ONE,
                engine: {
                  run_id: '123e4567-e89b-12d3-a456-426614174101',
                  definition_name: 'checkout',
                  definition_version: 'v1',
                  projection_state: 'catching_up',
                },
              },
            ],
            total: 1,
          }),
      })
    );

    renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();
    expect(within(getTraceRow('Checkout Trace')).getByText('Engine')).toBeInTheDocument();
  });

  it('keeps non-engine session trace rows unchanged', async () => {
    fetchMock.mockImplementation(buildFetchHandler());

    renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();
    expect(within(getTraceRow('Checkout Trace')).queryByText('Engine')).not.toBeInTheDocument();
  });

  it('shows the auth recovery banner when the session request returns 401', async () => {
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

    expect(
      await screen.findByRole('button', { name: /Trace Context/i })
    ).toBeInTheDocument();
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

    expect(
      await screen.findByRole('button', { name: /Trace Context/i })
    ).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '← Session' })).toHaveAttribute(
      'href',
      `/sessions/${SESSION_ID}?offset=20&sort_by=started_at&sort_dir=asc`
    );
  });

  it('tracks baseline and candidate selection in the URL while preserving table state', async () => {
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

    const { router } = renderTraceRoutes([
      `/sessions/${SESSION_ID}?sort_by=started_at&sort_dir=asc&offset=20`,
    ]);

    expect(await screen.findByText('Checkout Compare Trace')).toBeInTheDocument();

    await user.click(
      within(getTraceRow('Checkout Compare Trace')).getByRole('button', { name: 'Set as baseline' })
    );
    await user.click(
      within(getTraceRow('Alpha Compare Trace')).getByRole('button', { name: 'Set as candidate' })
    );

    await waitFor(() => {
      expect(router.state.location.search).toContain(`baseline_trace_id=${baselineTrace.id}`);
      expect(router.state.location.search).toContain(`candidate_trace_id=${candidateTrace.id}`);
      expect(router.state.location.search).toContain('sort_by=started_at');
      expect(router.state.location.search).toContain('sort_dir=asc');
      expect(router.state.location.search).toContain('offset=20');
    });

    expect(screen.getByRole('link', { name: 'Open comparison' })).toHaveAttribute(
      'href',
      `/sessions/${SESSION_ID}/compare?baseline_trace_id=${baselineTrace.id}&candidate_trace_id=${candidateTrace.id}`
    );
  });

  it('renders the compare bar above the narrative and trace browser surfaces', async () => {
    const user = userEvent.setup();
    const baselineTrace = {
      ...TRACE_ONE,
      id: '323e4567-e89b-12d3-a456-426614174060',
      session_id: SESSION_ID,
      name: 'Layout Baseline Trace',
      status: 'FAILED' as const,
    };
    const candidateTrace = {
      ...TRACE_ONE,
      id: '323e4567-e89b-12d3-a456-426614174061',
      session_id: SESSION_ID,
      name: 'Layout Candidate Trace',
      status: 'COMPLETED' as const,
    };

    fetchMock.mockImplementation(
      buildFetchHandler({
        list: () => jsonResponse({ traces: [baselineTrace, candidateTrace], total: 2 }),
      })
    );

    renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    expect(await screen.findByText('Layout Baseline Trace')).toBeInTheDocument();

    await user.click(
      within(getTraceRow('Layout Baseline Trace')).getByRole('button', { name: 'Set as baseline' })
    );
    await user.click(
      within(getTraceRow('Layout Candidate Trace')).getByRole('button', { name: 'Set as candidate' })
    );

    const compareSection = (await screen.findByRole('link', { name: 'Open comparison' })).closest(
      'section'
    );
    const narrativeSection = screen.getByLabelText('Session narrative summary');
    const tracesHeading = screen.getByRole('heading', { name: 'Traces' });

    if (!compareSection) {
      throw new Error('Expected compare bar section');
    }

    expect(
      Boolean(compareSection.compareDocumentPosition(narrativeSection) & Node.DOCUMENT_POSITION_FOLLOWING)
    ).toBe(true);
    expect(
      Boolean(compareSection.compareDocumentPosition(tracesHeading) & Node.DOCUMENT_POSITION_FOLLOWING)
    ).toBe(true);
  });

  it('clears the displaced role when the same trace is reassigned to candidate', async () => {
    const user = userEvent.setup();
    const trace = {
      ...TRACE_ONE,
      id: '323e4567-e89b-12d3-a456-426614174002',
      session_id: SESSION_ID,
      name: 'Single Compare Trace',
      status: 'FAILED' as const,
    };
    fetchMock.mockImplementation(
      buildFetchHandler({
        list: () => jsonResponse({ traces: [trace], total: 1 }),
      })
    );

    const { router } = renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    expect(await screen.findByText('Single Compare Trace')).toBeInTheDocument();

    const row = getTraceRow('Single Compare Trace');
    await user.click(within(row).getByRole('button', { name: 'Set as baseline' }));
    await user.click(within(row).getByRole('button', { name: 'Set as candidate' }));

    await waitFor(() => {
      expect(router.state.location.search).not.toContain('baseline_trace_id=');
      expect(router.state.location.search).toContain(`candidate_trace_id=${trace.id}`);
    });
  });

  it('clears the displaced role when the same storyline trace is reassigned to candidate', async () => {
    const user = userEvent.setup();
    const traceId = '323e4567-e89b-12d3-a456-426614174003';
    const traceName = 'Storyline Compare Trace';

    fetchMock.mockImplementation(
      buildFetchHandler({
        list: () =>
          jsonResponse({
            traces: [{ ...TRACE_ONE, id: traceId, session_id: SESSION_ID, name: traceName }],
            total: 1,
          }),
        sessionNarrative: () =>
          jsonResponse({
            ...SESSION_NARRATIVE,
            traces: [
              createSessionNarrativeTrace({
                id: traceId,
                trace_id: 'external-storyline-compare',
                name: traceName,
                status: 'FAILED',
                lineage: { type: 'unlinked' },
              }),
            ],
          }),
      })
    );

    const { router } = renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    const storyline = await screen.findByLabelText('Session narrative storyline');
    const storylineCard = (await within(storyline).findByRole('link', {
      name: traceName,
    })).closest('article');

    if (!storylineCard) {
      throw new Error(`Expected a storyline card for ${traceName}`);
    }

    await user.click(within(storylineCard).getByRole('button', { name: 'Set as baseline' }));
    await user.click(within(storylineCard).getByRole('button', { name: 'Set as candidate' }));

    await waitFor(() => {
      expect(router.state.location.search).not.toContain('baseline_trace_id=');
      expect(router.state.location.search).toContain(`candidate_trace_id=${traceId}`);
    });
  });

  it('disables compare selection on running traces', async () => {
    const runningTrace = {
      ...TRACE_ONE,
      id: 'running-trace',
      name: 'Running Trace',
      status: 'RUNNING' as const,
      session_id: SESSION_ID,
      ended_at: undefined,
    };

    fetchMock.mockImplementation(
      buildFetchHandler({
        list: () => jsonResponse({ traces: [runningTrace], total: 1 }),
        sessionNarrative: () =>
          jsonResponse({
            ...RUNNING_SESSION_NARRATIVE,
            traces: [
              createSessionNarrativeTrace({
                id: runningTrace.id,
                trace_id: 'external-running-trace',
                name: runningTrace.name,
                status: 'RUNNING',
              }),
            ],
          }),
      })
    );

    renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    await screen.findAllByRole('link', { name: 'Running Trace' });

    const tableRow = getTraceRow('Running Trace');
    const storylineCard = within(screen.getByLabelText('Session narrative storyline'))
      .getByRole('link', { name: 'Running Trace' })
      .closest('article');

    if (!storylineCard) {
      throw new Error('Expected a storyline card for Running Trace');
    }

    expect(
      within(tableRow).getByRole('button', { name: 'Set as baseline' })
    ).toBeDisabled();
    expect(
      within(tableRow).getByRole('button', { name: 'Set as candidate' })
    ).toBeDisabled();
    expect(
      within(tableRow).getByRole('button', { name: 'Set as baseline' })
    ).toHaveAttribute('title', 'Trace must complete before it can be compared');

    expect(
      within(storylineCard).getByRole('button', { name: 'Set as baseline' })
    ).toBeDisabled();
    expect(
      within(storylineCard).getByRole('button', { name: 'Set as candidate' })
    ).toBeDisabled();
    expect(
      within(storylineCard).getByRole('button', { name: 'Set as candidate' })
    ).toHaveAttribute('title', 'Trace must complete before it can be compared');
  });

  it('keeps comparison disabled while a selected-trace lookup is pending', async () => {
    const deferredDetail = createDeferredResponse();
    const baselineTrace = {
      ...TRACE_DETAIL,
      id: '323e4567-e89b-12d3-a456-426614174050',
      session_id: SESSION_ID,
      trace_id: 'external-pending-baseline',
      name: 'Pending Baseline Trace',
      status: 'COMPLETED' as const,
    };
    const candidateTrace = {
      ...TRACE_ONE,
      id: '323e4567-e89b-12d3-a456-426614174051',
      session_id: SESSION_ID,
      name: 'Pending Candidate Trace',
      status: 'FAILED' as const,
    };

    fetchMock.mockImplementation(
      buildFetchHandler({
        list: () => jsonResponse({ traces: [candidateTrace], total: 1 }),
        detail: (url) =>
          url.pathname === `/api/traces/${baselineTrace.id}`
            ? deferredDetail.promise
            : jsonResponse(TRACE_DETAIL),
      })
    );

    renderTraceRoutes([
      `/sessions/${SESSION_ID}?baseline_trace_id=${baselineTrace.id}&candidate_trace_id=${candidateTrace.id}`,
    ]);

    expect(await screen.findAllByText('Pending Candidate Trace')).toHaveLength(2);
    expect(screen.getByText('Loading trace…')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Open comparison' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Open comparison' })).toHaveAttribute(
      'title',
      'Both selections must resolve before comparison can open'
    );
    expect(screen.getByText('Resolving selected trace details...')).toBeInTheDocument();

    deferredDetail.resolve(jsonResponse(baselineTrace));

    expect(await screen.findByText('Pending Baseline Trace')).toBeInTheDocument();
    expect(await screen.findByRole('link', { name: 'Open comparison' })).toHaveAttribute(
      'href',
      `/sessions/${SESSION_ID}/compare?baseline_trace_id=${baselineTrace.id}&candidate_trace_id=${candidateTrace.id}`
    );
  });

  it('keeps comparison disabled when a selected trace is still running', async () => {
    const runningBaseline = {
      ...TRACE_ONE,
      id: '323e4567-e89b-12d3-a456-426614174052',
      session_id: SESSION_ID,
      name: 'Running Baseline Trace',
      status: 'RUNNING' as const,
      ended_at: undefined,
    };
    const candidateTrace = {
      ...TRACE_ONE,
      id: '323e4567-e89b-12d3-a456-426614174053',
      session_id: SESSION_ID,
      name: 'Completed Candidate Trace',
      status: 'COMPLETED' as const,
    };

    fetchMock.mockImplementation(
      buildFetchHandler({
        list: () => jsonResponse({ traces: [runningBaseline, candidateTrace], total: 2 }),
      })
    );

    renderTraceRoutes([
      `/sessions/${SESSION_ID}?baseline_trace_id=${runningBaseline.id}&candidate_trace_id=${candidateTrace.id}`,
    ]);

    expect(await screen.findAllByText('Running Baseline Trace')).toHaveLength(2);
    expect(screen.getByRole('button', { name: 'Open comparison' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Open comparison' })).toHaveAttribute(
      'title',
      'Both selected traces must be terminal before comparison can open'
    );
    expect(
      screen.getByText(
        'Running traces stay visible here, but comparison remains disabled until they finish.'
      )
    ).toBeInTheDocument();
  });

  it('offers compare to parent on storyline cards and assigns the pair', async () => {
    const user = userEvent.setup();
    const parentTraceId = '323e4567-e89b-12d3-a456-426614174010';
    const childTraceId = '323e4567-e89b-12d3-a456-426614174011';
    fetchMock.mockImplementation(
      buildFetchHandler({
        list: () =>
          jsonResponse({
            traces: [
              { ...TRACE_ONE, id: childTraceId, session_id: SESSION_ID, name: 'Child Compare Trace' },
              { ...TRACE_ONE, id: parentTraceId, session_id: SESSION_ID, name: 'Parent Compare Trace', status: 'COMPLETED' as const },
            ],
            total: 2,
          }),
        sessionNarrative: () =>
          jsonResponse({
            ...SESSION_NARRATIVE,
            traces: [
              createSessionNarrativeTrace({
                id: parentTraceId,
                trace_id: 'external-parent-compare',
                name: 'Parent Compare Trace',
                status: 'COMPLETED',
                lineage: { type: 'unlinked' },
              }),
              createSessionNarrativeTrace({
                id: childTraceId,
                trace_id: 'external-child-compare',
                name: 'Child Compare Trace',
                status: 'FAILED',
                lineage: { type: 'inferred', parent_trace_id: 'external-parent-compare' },
              }),
            ],
          }),
      })
    );

    const { router } = renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    expect(await screen.findByRole('button', { name: 'Compare to parent' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Compare to parent' }));

    await waitFor(() => {
      expect(router.state.location.search).toContain(`baseline_trace_id=${parentTraceId}`);
      expect(router.state.location.search).toContain(`candidate_trace_id=${childTraceId}`);
    });
  });

  it('hides compare to parent when either trace is not terminal', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        list: () =>
          jsonResponse({
            traces: [
              { ...TRACE_ONE, id: '323e4567-e89b-12d3-a456-426614174054', session_id: SESSION_ID, name: 'Running Parent Trace', status: 'RUNNING' as const, ended_at: undefined },
              { ...TRACE_ONE, id: '323e4567-e89b-12d3-a456-426614174055', session_id: SESSION_ID, name: 'Terminal Child Trace', status: 'FAILED' as const },
            ],
            total: 2,
          }),
        sessionNarrative: () =>
          jsonResponse({
            ...SESSION_NARRATIVE,
            traces: [
              createSessionNarrativeTrace({
                id: '323e4567-e89b-12d3-a456-426614174054',
                trace_id: 'external-running-parent',
                name: 'Running Parent Trace',
                status: 'RUNNING',
                ended_at: undefined,
                latest_activity_at: '2026-03-14T10:00:00.000Z',
                lineage: { type: 'unlinked' },
              }),
              createSessionNarrativeTrace({
                id: '323e4567-e89b-12d3-a456-426614174055',
                trace_id: 'external-terminal-child',
                name: 'Terminal Child Trace',
                status: 'FAILED',
                lineage: { type: 'inferred', parent_trace_id: 'external-running-parent' },
              }),
            ],
          }),
      })
    );

    renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    expect(await screen.findAllByRole('link', { name: 'Terminal Child Trace' })).toHaveLength(2);
    expect(screen.queryByRole('button', { name: 'Compare to parent' })).not.toBeInTheDocument();
  });

  it('swaps the selected baseline and candidate traces', async () => {
    const user = userEvent.setup();
    const baselineTrace = {
      ...TRACE_ONE,
      id: '323e4567-e89b-12d3-a456-426614174030',
      session_id: SESSION_ID,
      name: 'Swap Baseline Trace',
      status: 'FAILED' as const,
    };
    const candidateTrace = {
      ...TRACE_ONE,
      id: '323e4567-e89b-12d3-a456-426614174031',
      session_id: SESSION_ID,
      name: 'Swap Candidate Trace',
      status: 'COMPLETED' as const,
    };

    fetchMock.mockImplementation(
      buildFetchHandler({
        list: () => jsonResponse({ traces: [baselineTrace, candidateTrace], total: 2 }),
      })
    );

    const { router } = renderTraceRoutes([`/sessions/${SESSION_ID}`]);

    expect(await screen.findByText('Swap Baseline Trace')).toBeInTheDocument();

    await user.click(
      within(getTraceRow('Swap Baseline Trace')).getByRole('button', { name: 'Set as baseline' })
    );
    await user.click(
      within(getTraceRow('Swap Candidate Trace')).getByRole('button', { name: 'Set as candidate' })
    );
    await user.click(await screen.findByRole('button', { name: 'Swap' }));

    await waitFor(() => {
      expect(router.state.location.search).toContain(`baseline_trace_id=${candidateTrace.id}`);
      expect(router.state.location.search).toContain(`candidate_trace_id=${baselineTrace.id}`);
    });
  });

  it('hydrates selected traces outside the loaded slice with bounded trace lookups', async () => {
    const candidateTrace = {
      ...TRACE_ONE,
      id: '323e4567-e89b-12d3-a456-426614174020',
      session_id: SESSION_ID,
      trace_id: 'external-candidate-trace',
      name: 'Candidate Compare Trace',
      status: 'FAILED' as const,
    };
    const lookupTrace = {
      ...TRACE_DETAIL,
      id: '223e4567-e89b-12d3-a456-426614174000',
      session_id: SESSION_ID,
      trace_id: 'external-lookup-trace',
      name: 'Lookup Trace',
      status: 'COMPLETED' as const,
    };

    fetchMock.mockImplementation(
      buildFetchHandler({
        list: () => jsonResponse({ traces: [candidateTrace], total: 1 }),
        detail: (url) =>
          url.pathname === `/api/traces/${lookupTrace.id}`
            ? jsonResponse(lookupTrace)
            : jsonResponse(TRACE_DETAIL),
      })
    );

    renderTraceRoutes([
      `/sessions/${SESSION_ID}?baseline_trace_id=${lookupTrace.id}&candidate_trace_id=${candidateTrace.id}`,
    ]);

    expect(await screen.findByText('Lookup Trace')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Open comparison' })).toHaveAttribute(
      'href',
      `/sessions/${SESSION_ID}/compare?baseline_trace_id=${lookupTrace.id}&candidate_trace_id=${candidateTrace.id}`
    );
  });

  it('strips malformed compare params from the URL on load', async () => {
    const candidateTrace = {
      ...TRACE_ONE,
      id: '323e4567-e89b-12d3-a456-426614174040',
      session_id: SESSION_ID,
      name: 'Canonical Candidate Trace',
      status: 'FAILED' as const,
    };

    fetchMock.mockImplementation(
      buildFetchHandler({
        list: () => jsonResponse({ traces: [candidateTrace], total: 1 }),
      })
    );

    const { router } = renderTraceRoutes([
      `/sessions/${SESSION_ID}?baseline_trace_id=not-a-uuid&candidate_trace_id=${candidateTrace.id}`,
    ]);

    expect(
      await screen.findByRole('link', { name: 'Canonical Candidate Trace' })
    ).toBeInTheDocument();

    await waitFor(() => {
      expect(router.state.location.search).toBe(`?candidate_trace_id=${candidateTrace.id}`);
    });
  });

  it('canonicalizes same-trace compare params on load by clearing candidate', async () => {
    const trace = {
      ...TRACE_ONE,
      id: '323e4567-e89b-12d3-a456-426614174056',
      session_id: SESSION_ID,
      name: 'Canonical Baseline Trace',
      status: 'FAILED' as const,
    };

    fetchMock.mockImplementation(
      buildFetchHandler({
        list: () => jsonResponse({ traces: [trace], total: 1 }),
      })
    );

    const { router } = renderTraceRoutes([
      `/sessions/${SESSION_ID}?baseline_trace_id=${trace.id}&candidate_trace_id=${trace.id}`,
    ]);

    expect(await screen.findByRole('link', { name: 'Canonical Baseline Trace' })).toBeInTheDocument();

    await waitFor(() => {
      expect(router.state.location.search).toBe(`?baseline_trace_id=${trace.id}`);
    });
  });

  it('strips stale compare params when a bounded selected-trace lookup returns 404', async () => {
    const staleBaselineId = '323e4567-e89b-12d3-a456-426614174041';
    const candidateTrace = {
      ...TRACE_ONE,
      id: '323e4567-e89b-12d3-a456-426614174042',
      session_id: SESSION_ID,
      name: 'Stale Candidate Trace',
      status: 'FAILED' as const,
    };

    fetchMock.mockImplementation(
      buildFetchHandler({
        list: () => jsonResponse({ traces: [candidateTrace], total: 1 }),
        detail: (url) =>
          url.pathname === `/api/traces/${staleBaselineId}`
            ? jsonResponse({ code: 'not_found', message: 'Resource not found' }, 404)
            : jsonResponse(TRACE_DETAIL),
      })
    );

    const { router } = renderTraceRoutes([
      `/sessions/${SESSION_ID}?baseline_trace_id=${staleBaselineId}&candidate_trace_id=${candidateTrace.id}`,
    ]);

    expect(
      await screen.findByRole('link', { name: 'Stale Candidate Trace' })
    ).toBeInTheDocument();

    await waitFor(() => {
      expect(router.state.location.search).toBe(`?candidate_trace_id=${candidateTrace.id}`);
    });
  });

  it('strips stale compare params when a bounded selected-trace lookup resolves to another session', async () => {
    const staleBaselineId = '323e4567-e89b-12d3-a456-426614174057';
    const candidateTrace = {
      ...TRACE_ONE,
      id: '323e4567-e89b-12d3-a456-426614174058',
      session_id: SESSION_ID,
      name: 'Wrong Session Candidate Trace',
      status: 'FAILED' as const,
    };
    const otherSessionTrace = {
      ...TRACE_DETAIL,
      id: staleBaselineId,
      session_id: OTHER_SESSION_ID,
      trace_id: 'external-other-session-trace',
      name: 'Other Session Trace',
      status: 'COMPLETED' as const,
    };

    fetchMock.mockImplementation(
      buildFetchHandler({
        list: () => jsonResponse({ traces: [candidateTrace], total: 1 }),
        detail: (url) =>
          url.pathname === `/api/traces/${staleBaselineId}`
            ? jsonResponse(otherSessionTrace)
            : jsonResponse(TRACE_DETAIL),
      })
    );

    const { router } = renderTraceRoutes([
      `/sessions/${SESSION_ID}?baseline_trace_id=${staleBaselineId}&candidate_trace_id=${candidateTrace.id}`,
    ]);

    expect(
      await screen.findByRole('link', { name: 'Wrong Session Candidate Trace' })
    ).toBeInTheDocument();

    await waitFor(() => {
      expect(router.state.location.search).toBe(`?candidate_trace_id=${candidateTrace.id}`);
    });
  });
});
