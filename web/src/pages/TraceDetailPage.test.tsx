import { act } from 'react';
import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { clearApiKey, setApiKey, type TraceDetail } from '../api/client';
import { setMatchMediaMatches } from '../test/matchMedia';
import {
  TRACE_DETAIL,
  TRACE_ONE,
  buildFetchHandler,
  createDeferredResponse,
  createSpan,
  createTimelineEvent,
  jsonResponse,
  mockClipboard,
  renderTraceRoutes,
  resetTestEntityCounter,
} from './testUtils';

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  resetTestEntityCounter();
  fetchMock = vi.fn();
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
  Object.defineProperty(window.navigator, 'clipboard', {
    configurable: true,
    value: undefined,
  });
});

describe('TraceDetailPage', () => {
  it('renders the desktop workspace with tree, waterfall, and details surfaces', async () => {
    const rootSpan = createSpan({ span_id: 'root', name: 'Root span' });
    const childSpan = createSpan({
      span_id: 'child',
      name: 'Child span',
      parent_span_id: 'root',
    });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(TRACE_DETAIL),
        spans: () => jsonResponse({ spans: [rootSpan, childSpan] }),
        timeline: () =>
          jsonResponse({
            events: [],
            trace_status: 'COMPLETED',
            has_more: false,
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(await screen.findByRole('heading', { name: 'Execution Waterfall' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Spans (2)' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Details' })).toHaveAttribute(
      'aria-pressed',
      'true'
    );
    expect(screen.getByRole('button', { name: 'Timeline' })).toHaveAttribute(
      'aria-pressed',
      'false'
    );
  });

  it('renders the non-desktop stacked layout with Details active by default', async () => {
    setMatchMediaMatches(false);
    const rootSpan = createSpan({ span_id: 'mobile-root', name: 'Mobile root' });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse({ ...TRACE_DETAIL, status: 'COMPLETED', error_count: 0 }),
        spans: () => jsonResponse({ spans: [rootSpan] }),
        timeline: () =>
          jsonResponse({
            events: [],
            trace_status: 'COMPLETED',
            has_more: false,
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(await screen.findByRole('button', { name: 'Details' })).toHaveAttribute(
      'aria-pressed',
      'true'
    );
    expect(screen.getByRole('button', { name: 'Waterfall' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Tree' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Timeline' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Trace Context/i })).toHaveAttribute(
      'aria-expanded',
      'false'
    );
  });

  it('shows and hides inline tree metrics from the tree-rail toggle', async () => {
    const user = userEvent.setup();
    const rootSpan = createSpan({ span_id: 'metrics-root', name: 'Metrics root' });
    const childSpan = createSpan({
      span_id: 'metrics-child',
      name: 'Metrics child',
      parent_span_id: 'metrics-root',
      tokens_in: 10,
      tokens_out: 32,
      cost_usd: 0.05,
    });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse({ ...TRACE_DETAIL, status: 'COMPLETED', error_count: 0 }),
        spans: () => jsonResponse({ spans: [rootSpan, childSpan] }),
        timeline: () =>
          jsonResponse({
            events: [],
            trace_status: 'COMPLETED',
            has_more: false,
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
    await screen.findByRole('button', { name: 'Select span Metrics child' });

    expect(screen.queryByText('42 tokens')).not.toBeInTheDocument();
    expect(screen.queryByText('$0.05')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Show metrics' }));

    expect(screen.getByText('42 tokens')).toBeInTheDocument();
    expect(screen.getByText('$0.05')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Show metrics' }));

    expect(screen.queryByText('42 tokens')).not.toBeInTheDocument();
    expect(screen.queryByText('$0.05')).not.toBeInTheDocument();
  });

  it('confirms before expand all when the projected reveal cost exceeds the threshold', async () => {
    const user = userEvent.setup();
    const confirmSpy = vi.spyOn(window, 'confirm');
    const rootSpan = createSpan({ span_id: 'guard-root', name: 'Guard root' });
    const childSpans = Array.from({ length: 701 }, (_, index) =>
      createSpan({
        span_id: `guard-child-${index}`,
        name: `Guard child ${index}`,
        parent_span_id: 'guard-root',
      })
    );

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse({ ...TRACE_DETAIL, status: 'COMPLETED', error_count: 0 }),
        spans: () => jsonResponse({ spans: [rootSpan, ...childSpans] }),
        timeline: () =>
          jsonResponse({
            events: [],
            trace_status: 'COMPLETED',
            has_more: false,
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
    await screen.findByRole('button', { name: 'Collapse all' });

    await user.click(screen.getByRole('button', { name: 'Collapse all' }));
    expect(
      screen.queryByRole('button', { name: 'Select span Guard child 0' })
    ).not.toBeInTheDocument();

    confirmSpy.mockReturnValueOnce(false);
    await user.click(screen.getByRole('button', { name: 'Expand all' }));
    expect(confirmSpy).toHaveBeenCalledOnce();
    expect(
      screen.queryByRole('button', { name: 'Select span Guard child 0' })
    ).not.toBeInTheDocument();

    confirmSpy.mockReturnValueOnce(true);
    await user.click(screen.getByRole('button', { name: 'Expand all' }));
    expect(confirmSpy).toHaveBeenCalledTimes(2);
    expect(
      await screen.findByRole('button', { name: 'Select span Guard child 0' })
    ).toBeInTheDocument();
    confirmSpy.mockRestore();
  }, 15000);

  it('supports keyboard activation on waterfall timing bars', async () => {
    const user = userEvent.setup();
    const rootSpan = createSpan({
      span_id: 'keyboard-root',
      name: 'Keyboard root',
      status: 'COMPLETED',
    });
    const childSpan = createSpan({
      span_id: 'keyboard-child',
      name: 'Keyboard child',
      parent_span_id: 'keyboard-root',
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
    const childBar = await screen.findByRole('button', {
      name: 'Select waterfall span Keyboard child',
    });

    childBar.focus();
    await user.keyboard('{Enter}');

    expect(view.router.state.location.search).toBe('?span=keyboard-child');
    expect(
      within(screen.getByLabelText('Span breadcrumb')).getByText('Keyboard child')
    ).toBeInTheDocument();
  });

  it('auto-selects the primary failed span and re-reveals it from the summary jump after collapse all', async () => {
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

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(await screen.findByRole('heading', { name: 'Failure Summary' })).toBeInTheDocument();
    expect(
      within(screen.getByLabelText('Span breadcrumb')).getByText('Failed tool')
    ).toBeInTheDocument();

    const rootRow = screen.getByRole('button', { name: 'Select span Root agent' });
    const primaryRow = screen.getByRole('button', { name: 'Select span Failed tool' });
    expect(rootRow).toHaveClass('bg-amber-50');
    expect(primaryRow).toHaveAttribute('aria-pressed', 'true');
    expect(
      await screen.findByRole('button', { name: 'Select waterfall span Failed tool' })
    ).toHaveAttribute('title', expect.stringContaining('Failed tool'));

    await user.click(screen.getByRole('button', { name: 'Collapse all' }));
    expect(
      screen.queryByRole('button', { name: 'Select span Failed tool' })
    ).not.toBeInTheDocument();
    expect(
      screen.queryByRole('button', { name: 'Select waterfall span Failed tool' })
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

    await waitFor(() => {
      expect(writeText).toHaveBeenCalledWith(
        `${window.location.origin}/traces/trace-checkout?debug=true&span=copy-failed`
      );
    });
  });

  it('keeps tree, waterfall, details, and timeline selections synchronized with the URL', async () => {
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

    await user.click(screen.getByRole('button', { name: 'Select parent span sync-root' }));
    expect(view.router.state.location.search).toBe('?span=sync-root');
    expect(
      within(screen.getByLabelText('Span breadcrumb')).getByText('Sync root')
    ).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Select waterfall span Sync failed' }));
    expect(view.router.state.location.search).toBe('?span=sync-failed');
    expect(screen.getByRole('button', { name: 'Details' })).toHaveAttribute(
      'aria-pressed',
      'true'
    );

    await user.click(screen.getByRole('button', { name: 'Timeline' }));
    const timelineSection = await screen.findByRole('heading', { name: 'Timeline' });
    await user.click(
      within(timelineSection.closest('section')!).getByRole('button', { name: 'sync-root' })
    );
    expect(view.router.state.location.search).toBe('?span=sync-root');
    expect(
      within(screen.getByLabelText('Span breadcrumb')).getByText('Sync root')
    ).toBeInTheDocument();
  });

  it('preserves timeline state across tab switches', async () => {
    const user = userEvent.setup();

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse({ ...TRACE_DETAIL, status: 'FAILED', error_count: 1 }),
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
    await user.click(await screen.findByRole('button', { name: 'Timeline' }));
    await user.click(screen.getByRole('button', { name: 'Show error events only' }));
    expect(screen.queryByText('Informational update')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Details' }));
    await user.click(screen.getByRole('button', { name: 'Timeline' }));

    expect(screen.getByRole('button', { name: 'Show error events only' })).toHaveAttribute(
      'aria-pressed',
      'true'
    );
    expect(screen.queryByText('Informational update')).not.toBeInTheDocument();
  });

  it('preserves a manual selection across running-trace polling updates', async () => {
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

    await user.click(screen.getByRole('button', { name: 'Timeline' }));
    expect(await screen.findByText('Poll update')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Details' }));
    expect(
      within(screen.getByLabelText('Span breadcrumb')).getByText('Running child')
    ).toBeInTheDocument();
    expect(view.router.state.location.search).toBe('?span=running-child');
  }, 10000);

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

    await user.click(screen.getByRole('button', { name: 'Timeline' }));
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
    await user.click(screen.getByRole('button', { name: 'Timeline' }));
    expect(screen.getByText('Beta info event')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Show error events only' })).toHaveAttribute(
      'aria-pressed',
      'false'
    );
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

    await userEvent.setup().click(screen.getByRole('button', { name: 'Timeline' }));
    expect(await screen.findByText('Recent activity')).toBeInTheDocument();
    expect(
      screen.queryByText(/still marked running\. recent activity is sparse/i)
    ).not.toBeInTheDocument();
  });

  it('auto-expands matching ancestors during search and restores prior expansion when cleared', async () => {
    const user = userEvent.setup();
    const rootSpan = createSpan({
      span_id: 'search-root',
      name: 'Search root',
      status: 'COMPLETED',
    });
    const childSpan = createSpan({
      span_id: 'search-child',
      name: 'Needle child',
      parent_span_id: 'search-root',
      status: 'COMPLETED',
      error_message: 'Needle failure preview',
    });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse({ ...TRACE_DETAIL, status: 'COMPLETED', error_count: 0 }),
        spans: () => jsonResponse({ spans: [rootSpan, childSpan] }),
        timeline: () =>
          jsonResponse({
            events: [],
            trace_status: 'COMPLETED',
            has_more: false,
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
    expect(await screen.findByRole('button', { name: 'Select span Needle child' })).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Collapse all' }));
    expect(
      screen.queryByRole('button', { name: 'Select span Needle child' })
    ).not.toBeInTheDocument();

    await user.type(screen.getByRole('searchbox', { name: 'Search spans' }), 'needle');
    expect(
      await screen.findByRole('button', { name: 'Select span Needle child' })
    ).toBeInTheDocument();

    await user.clear(screen.getByRole('searchbox', { name: 'Search spans' }));
    await waitFor(() => {
      expect(
        screen.queryByRole('button', { name: 'Select span Needle child' })
      ).not.toBeInTheDocument();
    });
  });
});
