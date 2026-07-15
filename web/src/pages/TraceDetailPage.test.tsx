import { act, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  clearApiKey,
  setApiKey,
  type TraceDetail,
} from '../api/client';
import { setMatchMediaMatches } from '../test/matchMedia';
import { downloadJsonFile } from '../utils/downloadJson';
import {
  TIMELINE_POLL_INTERVAL_MS,
} from './useTraceTimeline';
import {
  SESSION_ID,
  TRACE_DETAIL,
  TRACE_ONE,
  buildFetchHandler,
  createSpan,
  createTimelineEvent,
  getRequests,
  jsonResponse,
  mockClipboard,
  readRequestUrl,
  renderTraceRoutes,
  resetTestEntityCounter,
} from './testUtils';

vi.mock('../utils/downloadJson', () => ({
  downloadJsonFile: vi.fn(),
}));

let fetchMock: ReturnType<typeof vi.fn>;

function getTraceDetailRequests(): URL[] {
  return getRequests(fetchMock, `/api/traces/${TRACE_ONE.id}`);
}

function countSpanRequests(): number {
  return fetchMock.mock.calls.filter(([input]) => {
    const url = new URL(readRequestUrl(input), 'http://localhost');
    return /^\/api\/traces\/[^/]+\/spans$/.test(url.pathname);
  }).length;
}

function createEngineTraceDetail(
  overrides: Partial<TraceDetail> = {}
): TraceDetail {
  return {
    ...TRACE_DETAIL,
    status: 'RUNNING',
    ended_at: undefined,
    error_count: 0,
    engine: {
      run_id: '123e4567-e89b-12d3-a456-426614174103',
      instance_key: 'instance-1',
      definition_name: 'checkout',
      definition_version: 'v1',
      projection_state: 'summary_only',
      status: 'WAITING',
      created_at: '2026-03-14T10:00:00.000Z',
      updated_at: '2026-03-14T10:00:05.000Z',
      pending_work: {
        pending_activity_tasks: 1,
        pending_inbox_items: 2,
      },
      wait_state: {
        kind: 'signal',
        signal_name: 'approval',
      },
    },
    ...overrides,
  };
}

function createPendingWorkResponse() {
  return {
    run_id: '123e4567-e89b-12d3-a456-426614174103',
    current_wait: {
      kind: 'signal',
      signal_name: 'approval',
    },
    activities: [
      {
        task_id: 'task-1',
        activity_key: 'charge-card',
        activity_type: 'payments.charge',
        status: 'scheduled',
        available_at: '2026-03-14T10:01:00.000Z',
        attempt_count: 2,
      },
    ],
    timers: [
      {
        inbox_id: 'timer-1',
        timer_key: 'approval-timeout',
        status: 'scheduled',
        available_at: '2026-03-14T10:02:00.000Z',
      },
    ],
    signals: [
      {
        inbox_id: 'signal-1',
        signal_name: 'manual_override',
        status: 'queued',
        available_at: '2026-03-14T10:03:00.000Z',
      },
    ],
    pending_activity_tasks: 1,
    pending_inbox_items: 2,
  };
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
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.unstubAllEnvs();
  Object.defineProperty(window.navigator, 'clipboard', {
    configurable: true,
    value: undefined,
  });
});

describe('TraceDetailPage', () => {
  it('renders the debugger-kit trace header, waterfall workspace, and inspector', async () => {
    const rootSpan = createSpan({
      span_id: 'root',
      name: 'Root span',
      input: { cart_id: 'cart_123' },
    });
    const childSpan = createSpan({
      span_id: 'child',
      name: 'Child span',
      parent_span_id: 'root',
      output: { ok: true },
    });
    fetchMock.mockImplementation(
      buildFetchHandler({
        spans: () => jsonResponse({ spans: [rootSpan, childSpan] }),
        timeline: () =>
          jsonResponse({
            events: [
              createTimelineEvent({
                id: 'event-child',
                span_id: 'child',
                span_name: 'Child span',
                event_type: 'effect',
                message: 'Called checkout tool',
              }),
            ],
            trace_status: 'COMPLETED',
            has_more: false,
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(await screen.findByRole('heading', { name: 'Checkout Trace' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Replay' })).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Export JSON' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Open in workspace' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Trace Context' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Execution Waterfall' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Root span COMPLETED 1.0s' })).toBeInTheDocument();
    expect(await screen.findByRole('button', { name: 'Child span COMPLETED 1.0s' })).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'Child span COMPLETED 1.0s' }));
    expect(screen.getByRole('heading', { name: 'Child span' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Output' })).toHaveAttribute('aria-pressed', 'false');
    await userEvent.click(screen.getByRole('button', { name: 'Output' }));
    expect(screen.getByText('"ok"')).toBeInTheDocument();

    const sectionTabs = screen.getByRole('navigation', { name: 'Trace detail sections' });
    await userEvent.click(within(sectionTabs).getByRole('button', { name: /Timeline/i }));
    expect(screen.getAllByRole('heading', { name: 'Timeline' }).length).toBeGreaterThan(0);
    expect(within(sectionTabs).getByRole('button', { name: /Timeline/i })).toHaveAttribute(
      'aria-pressed',
      'true'
    );

    await userEvent.click(within(sectionTabs).getByRole('button', { name: /Logs/i }));
    expect(screen.getByText(/Explicit logs, errors, exceptions/i)).toBeInTheDocument();
    expect(screen.getByText('Called checkout tool')).toBeInTheDocument();

    await userEvent.click(within(sectionTabs).getByRole('button', { name: /Metrics/i }));
    expect(screen.getByRole('heading', { name: 'Metrics' })).toBeInTheDocument();
    expect(screen.getByText('Latency by span')).toBeInTheDocument();
    expect(screen.getByText('Span signals')).toBeInTheDocument();

    expect(within(sectionTabs).queryByRole('button', { name: 'Replay' })).not.toBeInTheDocument();

    await userEvent.click(within(sectionTabs).getByRole('button', { name: 'Overview' }));
    expect(screen.getByRole('heading', { name: 'Execution Waterfall' })).toBeInTheDocument();
  });

  it('shows truncation metadata inside the active span inspector', async () => {
    const truncatedSpan = createSpan({
      span_id: 'truncated',
      name: 'Truncated payload span',
      input: { prompt: 'shortened' },
      input_truncated: true,
      input_original_size_bytes: 70_000,
      input_truncation_reason: 'max_bytes_exceeded',
      output: { answer: 'shortened' },
      output_truncated: true,
      output_original_size_bytes: 80_000,
      output_truncation_reason: 'max_bytes_exceeded',
    });
    fetchMock.mockImplementation(
      buildFetchHandler({
        spans: () => jsonResponse({ spans: [truncatedSpan] }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}?span=truncated`]);

    expect(await screen.findByRole('heading', { name: 'Truncated payload span' })).toBeInTheDocument();
    expect(screen.getByText('Input payload was truncated before storage.')).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'Output' }));

    expect(screen.getByText('Output payload was truncated before storage.')).toBeInTheDocument();
  });

  it('exposes the replay preview only when the explicit feature flag is enabled', async () => {
    vi.stubEnv('VITE_CONTINUA_REPLAY_PREVIEW', '1');
    fetchMock.mockImplementation(buildFetchHandler());

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(await screen.findByRole('heading', { name: 'Checkout Trace' })).toBeInTheDocument();
    const sectionTabs = screen.getByRole('navigation', { name: 'Trace detail sections' });
    expect(within(sectionTabs).getByRole('button', { name: 'Replay' })).toBeInTheDocument();

    await userEvent.click(within(sectionTabs).getByRole('button', { name: 'Replay' }));
    expect(screen.getByText('Replay is coming soon')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Run replay' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Open in workspace' })).toBeDisabled();
    expect(screen.getByText('Replay source')).toBeInTheDocument();
    expect(screen.getByText('Determinism overrides')).toBeInTheDocument();
  });

  it('selects a span from ?span= and keeps URL-backed selection synchronized', async () => {
    const user = userEvent.setup();
    const rootSpan = createSpan({ span_id: 'root', name: 'Root span' });
    const childSpan = createSpan({
      span_id: 'child',
      name: 'Child span',
      parent_span_id: 'root',
      input: { selected: true },
    });
    fetchMock.mockImplementation(
      buildFetchHandler({
        spans: () => jsonResponse({ spans: [rootSpan, childSpan] }),
      })
    );

    const { router } = renderTraceRoutes([`/traces/${TRACE_ONE.id}?span=child&debug=1`]);

    expect(await screen.findByRole('heading', { name: 'Child span' })).toBeInTheDocument();
    expect(router.state.location.search).toContain('span=child');

    await user.click(screen.getByRole('button', { name: 'Root span COMPLETED 1.0s' }));
    await waitFor(() => {
      expect(router.state.location.search).toContain('span=root');
      expect(router.state.location.search).toContain('debug=1');
    });
    expect(screen.getByRole('heading', { name: 'Root span' })).toBeInTheDocument();
  });

  it('clears an invalid ?span= once spans load and does not treat it as selected', async () => {
    const span = createSpan({ span_id: 'real', name: 'Real span' });
    fetchMock.mockImplementation(
      buildFetchHandler({
        spans: () => jsonResponse({ spans: [span] }),
      })
    );

    const { router } = renderTraceRoutes([
      `/traces/${TRACE_ONE.id}?span=ghost&debug=1`,
    ]);

    // span data renders...
    expect(
      await screen.findByRole('button', { name: 'Real span COMPLETED 1.0s' })
    ).toBeInTheDocument();

    // ...and the invalid param is stripped while unrelated params survive.
    await waitFor(() => {
      expect(router.state.location.search).not.toContain('span=ghost');
    });
    expect(router.state.location.search).toContain('debug=1');

    // the phantom span is not selected: inspector shows the empty prompt
    expect(
      screen.getByText('Select a span to inspect payloads.')
    ).toBeInTheDocument();
  });

  it('copies the effective trace URL and exports trace JSON', async () => {
    const user = userEvent.setup();
    const writeText = mockClipboard();
    const span = createSpan({ span_id: 'copy-span', name: 'Copy span' });
    fetchMock.mockImplementation(
      buildFetchHandler({
        spans: () => jsonResponse({ spans: [span] }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}?debug=1`]);

    expect(await screen.findByRole('button', { name: 'Copy span COMPLETED 1.0s' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Copy span COMPLETED 1.0s' }));
    await user.click(screen.getByRole('button', { name: 'Copy Trace URL' }));

    expect(writeText).toHaveBeenCalledWith(
      expect.stringContaining(`/traces/${TRACE_ONE.id}?debug=1&span=copy-span`)
    );

    await user.click(screen.getByRole('button', { name: 'Export JSON' }));
    expect(downloadJsonFile).toHaveBeenCalledWith(
      `continua-trace-${TRACE_ONE.id}.json`,
      expect.objectContaining({
        trace: expect.objectContaining({ id: TRACE_ONE.id }),
        spans: [expect.objectContaining({ span_id: 'copy-span' })],
        selected_span_id: 'copy-span',
      })
    );
  });

  it('opens the trace context drawer with copyable identity and session links', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(buildFetchHandler());

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(await screen.findByRole('heading', { name: 'Checkout Trace' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Trace Context' }));

    const dialog = await screen.findByRole('dialog', { name: 'Trace context' });
    expect(within(dialog).getByText('Trace Context')).toBeInTheDocument();
    expect(within(dialog).getByText('External Trace ID')).toBeInTheDocument();
    expect(within(dialog).getByRole('link', { name: /conv-checkout-123/i })).toHaveAttribute(
      'href',
      `/sessions/${SESSION_ID}`
    );
    expect(screen.getByRole('button', { name: 'Trace Context' })).toHaveAttribute(
      'aria-expanded',
      'true'
    );
  });

  it('renders engine projection, wait summary, tabs, and control actions', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(createEngineTraceDetail()),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(await screen.findByText('checkout')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Engine state' }));

    expect(screen.getByRole('button', { name: 'Overview 0' })).toHaveAttribute(
      'aria-pressed',
      'true'
    );
    expect(screen.getByRole('button', { name: 'Pending 0' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Engine history 0' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Result 0' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Summary retained' })).toBeInTheDocument();
    expect(screen.getByText('Waiting on signal')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Signal' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Suspend' })).toBeInTheDocument();
    expect(screen.getAllByRole('button', { name: 'Signal' })).toHaveLength(1);

    await user.click(screen.getByRole('button', { name: 'Pending 0' }));
    expect(screen.getAllByRole('button', { name: 'Signal' })).toHaveLength(1);
  });

  it('counts continued-as-new engine result panes and marks the state closed', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () =>
          jsonResponse(
            createEngineTraceDetail({
              engine: {
                ...createEngineTraceDetail().engine!,
                status: 'CONTINUED_AS_NEW',
                failure: undefined,
              },
            })
          ),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    await user.click(await screen.findByRole('button', { name: 'Engine state' }));
    expect(screen.getByRole('button', { name: 'Result 1' })).toBeInTheDocument();
    expect(screen.getAllByText('Closed').length).toBeGreaterThan(0);
    expect(screen.queryAllByText('Closing')).toHaveLength(0);
  });

  it('renders expired engine history in the Engine history tab', async () => {
    const user = userEvent.setup();
    const detail = createEngineTraceDetail();
    const runId = detail.engine!.run_id;
    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(detail),
        engineHistory: () =>
          jsonResponse({ events: [], has_more: false, expired: true }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    await user.click(await screen.findByRole('button', { name: 'Engine state' }));
    await user.click(screen.getByRole('button', { name: 'Engine history 0' }));

    expect(
      await screen.findByText(
        'History expired. Retained history for this run has been purged.'
      )
    ).toBeInTheDocument();
    const historyRequests = getRequests(fetchMock, `/v1/engine/runs/${runId}/history`);
    expect(historyRequests).toHaveLength(1);
    expect(historyRequests[0].searchParams.get('limit')).toBe('50');
  });

  it('renders the completed result payload in the Result tab', async () => {
    const user = userEvent.setup();
    const base = createEngineTraceDetail();
    const detail = createEngineTraceDetail({
      status: 'COMPLETED',
      ended_at: '2026-03-14T10:00:10.000Z',
      engine: {
        ...base.engine!,
        status: 'COMPLETED',
        completed_at: '2026-03-14T10:00:10.000Z',
      },
    });
    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(detail),
        engineResult: () =>
          jsonResponse({
            run_id: detail.engine!.run_id,
            status: 'COMPLETED',
            result: { output: 42 },
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    await user.click(await screen.findByRole('button', { name: 'Engine state' }));
    await user.click(screen.getByRole('button', { name: 'Result 1' }));

    expect(await screen.findByText('Completed result')).toBeInTheDocument();
    expect(screen.getByText('"output"')).toBeInTheDocument();
    expect(screen.getByText('42')).toBeInTheDocument();
  });

  it('renders the failure shell for failed runs', async () => {
    const user = userEvent.setup();
    const base = createEngineTraceDetail();
    const detail = createEngineTraceDetail({
      status: 'FAILED',
      ended_at: '2026-03-14T10:00:10.000Z',
      engine: {
        ...base.engine!,
        status: 'FAILED',
        completed_at: '2026-03-14T10:00:10.000Z',
      },
    });
    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(detail),
        engineResult: () =>
          jsonResponse({
            run_id: detail.engine!.run_id,
            status: 'FAILED',
            result: null,
            failure: { error_code: 'boom', error_message: 'exploded' },
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    await user.click(await screen.findByRole('button', { name: 'Engine state' }));
    await user.click(screen.getByRole('button', { name: 'Result 1' }));

    expect(await screen.findByText('FAILED result shell')).toBeInTheDocument();
    expect(screen.getByText('boom')).toBeInTheDocument();
    expect(screen.getByText('exploded')).toBeInTheDocument();
  });

  it('keeps the result unavailable for non-terminal runs', async () => {
    const user = userEvent.setup();
    const detail = createEngineTraceDetail();
    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(detail),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    await user.click(await screen.findByRole('button', { name: 'Engine state' }));
    await user.click(screen.getByRole('button', { name: 'Result 0' }));

    expect(
      screen.getByText(
        'Result is not available until the run reaches a terminal state.'
      )
    ).toBeInTheDocument();
    expect(
      getRequests(fetchMock, `/v1/engine/runs/${detail.engine!.run_id}/result`)
    ).toHaveLength(0);
  });

  it('shows pending engine work in the mobile summary workspace', async () => {
    setMatchMediaMatches(false);
    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(createEngineTraceDetail()),
        enginePendingWork: () => jsonResponse(createPendingWorkResponse()),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(await screen.findByRole('heading', { name: 'Engine wait state and queued work' })).toBeInTheDocument();
    expect(await screen.findByText('payments.charge · charge-card')).toBeInTheDocument();
    expect(screen.getByText('approval-timeout')).toBeInTheDocument();
    expect(screen.getByText('manual_override')).toBeInTheDocument();
  });

  it('keeps the mobile execution tree available for search, expansion, and selection', async () => {
    const user = userEvent.setup();
    setMatchMediaMatches(false);
    const rootSpan = createSpan({ span_id: 'root', name: 'Root span' });
    const childSpan = createSpan({
      span_id: 'child',
      name: 'Needle child',
      parent_span_id: 'root',
      input: { needle: true },
    });
    fetchMock.mockImplementation(
      buildFetchHandler({
        spans: () => jsonResponse({ spans: [rootSpan, childSpan] }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(await screen.findByRole('button', { name: 'Execution' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Execution' }));
    await user.click(screen.getByRole('button', { name: 'Tree' }));

    expect(screen.getByRole('heading', { name: 'Spans (2)' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Collapse all' }));
    expect(screen.queryByRole('button', { name: 'Select span Needle child' })).not.toBeInTheDocument();

    await user.type(screen.getByLabelText('Search spans'), 'Needle');
    expect(await screen.findByRole('button', { name: 'Select span Needle child' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Select span Needle child' }));

    await user.click(screen.getByRole('button', { name: 'Summary' }));
    expect(screen.getAllByText('Needle child').length).toBeGreaterThan(0);
  });

  it('does not expose duplicate mobile timeline and state workspaces inside overview', async () => {
    setMatchMediaMatches(false);
    fetchMock.mockImplementation(
      buildFetchHandler({
        timeline: () =>
          jsonResponse({
            events: [
              createTimelineEvent({
                event_type: 'span_started',
                message: 'checkout started',
              }),
            ],
            trace_status: 'RUNNING',
            has_more: false,
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    const mobileWorkspace = await screen.findByRole('navigation', {
      name: 'Mobile trace workspace',
    });
    expect(within(mobileWorkspace).getByRole('button', { name: 'Summary' })).toBeInTheDocument();
    expect(within(mobileWorkspace).getByRole('button', { name: 'Execution' })).toBeInTheDocument();
    expect(within(mobileWorkspace).queryByRole('button', { name: 'Timeline' })).not.toBeInTheDocument();
    expect(within(mobileWorkspace).queryByRole('button', { name: 'State' })).not.toBeInTheDocument();

    const sectionTabs = screen.getByRole('navigation', { name: 'Trace detail sections' });
    await userEvent.click(within(sectionTabs).getByRole('button', { name: /Timeline/i }));
    expect(screen.getByRole('heading', { name: 'Timeline' })).toBeInTheDocument();
  });

  it('surfaces failure-first guidance in the mobile summary workspace', async () => {
    setMatchMediaMatches(false);
    const failedSpan = createSpan({
      span_id: 'failed',
      name: 'Failed tool',
      kind: 'TOOL',
      status: 'FAILED',
      error_message: 'Tool timeout',
    });
    fetchMock.mockImplementation(
      buildFetchHandler({
        spans: () => jsonResponse({ spans: [failedSpan] }),
        timeline: () =>
          jsonResponse({
            events: [
              createTimelineEvent({
                span_id: 'failed',
                span_name: 'Failed tool',
                event_type: 'effect',
                message: 'Tool timed out',
              }),
            ],
            trace_status: 'FAILED',
            has_more: false,
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(await screen.findByRole('heading', { name: 'Failure Summary' })).toBeInTheDocument();
    expect(screen.getAllByText('Failed tool').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Tool timeout').length).toBeGreaterThan(0);
  });

  it('polls spans while a trace is live so new waterfall rows appear', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const firstSpan = createSpan({
      span_id: 'first',
      name: 'First live span',
      status: 'STARTED',
      ended_at: undefined,
    });
    const secondSpan = createSpan({
      span_id: 'second',
      name: 'Second live span',
      status: 'STARTED',
      ended_at: undefined,
    });
    let spans = [firstSpan];
    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () =>
          jsonResponse({
            ...TRACE_DETAIL,
            status: 'RUNNING',
            ended_at: undefined,
          }),
        spans: () => jsonResponse({ spans }),
        timeline: () =>
          jsonResponse({
            events: [],
            trace_status: 'RUNNING',
            has_more: false,
            poll_cursor: 'cursor-running',
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect((await screen.findAllByRole('button', { name: /First live span/i })).length).toBeGreaterThan(0);
    await waitFor(() => {
      expect(countSpanRequests()).toBe(1);
    });

    spans = [firstSpan, secondSpan];
    await act(async () => {
      await vi.advanceTimersByTimeAsync(TIMELINE_POLL_INTERVAL_MS + 100);
    });

    expect((await screen.findAllByRole('button', { name: /Second live span/i })).length).toBeGreaterThan(0);
  }, 10_000);

  it('shows auth recovery when the trace request is unauthorized', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse({ message: 'Invalid or missing API key' }, 401),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(await screen.findByRole('alert')).toHaveTextContent('Invalid or missing API key');
    expect(screen.getByRole('link', { name: 'Sign in again' })).toHaveAttribute(
      'href',
      `/traces/${TRACE_ONE.id}`
    );
  });

  it('does not refetch trace data when only span selection changes', async () => {
    const user = userEvent.setup();
    const rootSpan = createSpan({ span_id: 'root', name: 'Root span' });
    const childSpan = createSpan({
      span_id: 'child',
      name: 'Child span',
      parent_span_id: 'root',
    });
    fetchMock.mockImplementation(
      buildFetchHandler({
        spans: () => jsonResponse({ spans: [rootSpan, childSpan] }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(await screen.findByRole('button', { name: 'Root span COMPLETED 1.0s' })).toBeInTheDocument();
    await waitFor(() => {
      expect(getTraceDetailRequests()).toHaveLength(1);
    });

    await user.click(screen.getByRole('button', { name: 'Child span COMPLETED 1.0s' }));
    await user.click(screen.getByRole('button', { name: 'Root span COMPLETED 1.0s' }));

    expect(getTraceDetailRequests()).toHaveLength(1);
  });
});
