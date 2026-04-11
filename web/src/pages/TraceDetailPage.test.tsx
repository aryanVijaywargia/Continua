import { act } from 'react';
import { focusManager, onlineManager } from '@tanstack/react-query';
import { fireEvent, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  clearApiKey,
  setApiKey,
  type TimelineEvent,
  type TraceDetail,
} from '../api/client';
import { setMatchMediaMatches } from '../test/matchMedia';
import { getAccessibleSummary, getReasonExplanation } from '../utils/retrySafety';
import { TIMELINE_POLL_INTERVAL_MS } from './useTraceTimeline';
import {
  SESSION_EXTERNAL_ID,
  SESSION_ID,
  TRACE_DETAIL,
  TRACE_ONE,
  buildFetchHandler,
  createDeferredResponse,
  createSpan,
  createTimelineEvent,
  jsonResponse,
  mockClipboard,
  readRequestUrl,
  renderTraceRoutes,
  resetTestEntityCounter,
} from './testUtils';

let fetchMock: ReturnType<typeof vi.fn>;

function createRunningTraceDetail(
  overrides: Partial<TraceDetail> = {}
): TraceDetail {
  return {
    ...TRACE_DETAIL,
    status: 'RUNNING',
    ended_at: undefined,
    error_count: 0,
    ...overrides,
  };
}

function createEngineTraceDetail({
  engineOverrides = {},
  traceOverrides = {},
}: {
  engineOverrides?: Partial<NonNullable<TraceDetail['engine']>>;
  traceOverrides?: Partial<TraceDetail>;
} = {}): TraceDetail {
  return createRunningTraceDetail({
    ...traceOverrides,
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
      ...engineOverrides,
    },
  });
}

function createPendingWorkResponse(
  overrides: Partial<{
    current_wait: Record<string, unknown> | null;
    activities: Array<Record<string, unknown>>;
    timers: Array<Record<string, unknown>>;
    signals: Array<Record<string, unknown>>;
    pending_activity_tasks: number;
    pending_inbox_items: number;
  }> = {}
) {
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
    ...overrides,
  };
}

function countRequests(pathname: string): number {
  return fetchMock.mock.calls.filter(([input]) => {
    const url = new URL(readRequestUrl(input), 'http://localhost');
    return url.pathname === pathname;
  }).length;
}

function getRequestCallsMatching(pattern: RegExp) {
  return fetchMock.mock.calls.flatMap(([input, init]) => {
    const url = new URL(readRequestUrl(input), 'http://localhost');
    return pattern.test(url.pathname)
      ? [{ url, init: init as RequestInit | undefined }]
      : [];
  });
}

function createWaitEvent({
  waitKind = 'external',
  phase = 'entered',
  waitId,
  resolution,
  ...eventOverrides
}: {
  waitKind?: string;
  phase?: string;
  waitId?: string;
  resolution?: string;
} & Partial<TimelineEvent> = {}): TimelineEvent {
  return createTimelineEvent({
    ...eventOverrides,
    event_type: 'wait',
    payload: {
      wait_kind: waitKind,
      phase,
      ...(waitId ? { wait_id: waitId } : {}),
      ...(resolution ? { resolution } : {}),
    },
  });
}

function createRunningTimelineResponse(
  events: TimelineEvent[],
  pollCursor = 'cursor-running'
) {
  return jsonResponse({
    events,
    trace_status: 'RUNNING',
    has_more: false,
    poll_cursor: pollCursor,
  });
}

function getRunningStatePanel(): HTMLElement {
  const panel = screen.getByText('Running state').closest('section');
  if (!panel) {
    throw new Error('Running state panel not found');
  }
  return panel;
}

async function advancePollingInterval(
  ms = TIMELINE_POLL_INTERVAL_MS + 100
): Promise<void> {
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, ms));
  });
}

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

  it('shows engine wait-state summary and projection banner for engine-backed traces', async () => {
    const rootSpan = createSpan({ span_id: 'engine-root', name: 'Engine root' });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () =>
          jsonResponse({
            ...createRunningTraceDetail(),
            engine: {
              run_id: '123e4567-e89b-12d3-a456-426614174103',
              instance_key: 'instance-1',
              definition_name: 'checkout',
              definition_version: 'v1',
              projection_state: 'catching_up',
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
          }),
        spans: () => jsonResponse({ spans: [rootSpan] }),
        timeline: () =>
          jsonResponse({
            events: [],
            trace_status: 'RUNNING',
            has_more: false,
            engine: {
              projection_state: 'catching_up',
            },
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(await screen.findByText('Projection catching up')).toBeInTheDocument();
    expect(screen.getByText('Waiting on signal')).toBeInTheDocument();
    expect(screen.getByText('approval')).toBeInTheDocument();
    expect(screen.getByText('Engine')).toBeInTheDocument();
  });

  it('keeps non-engine trace detail free of engine surfaces', async () => {
    const rootSpan = createSpan({ span_id: 'plain-root', name: 'Plain root' });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(createRunningTraceDetail()),
        spans: () => jsonResponse({ spans: [rootSpan] }),
        timeline: () =>
          jsonResponse({
            events: [],
            trace_status: 'RUNNING',
            has_more: false,
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(await screen.findByRole('heading', { name: 'Execution Waterfall' })).toBeInTheDocument();
    expect(screen.queryByText('Projection catching up')).not.toBeInTheDocument();
    expect(screen.queryByText('Projection summary only')).not.toBeInTheDocument();
    expect(screen.queryByText('Projection journal expired')).not.toBeInTheDocument();
    expect(screen.queryByText('Waiting on signal')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Signal' })).not.toBeInTheDocument();
    expect(screen.queryByText('Pending work')).not.toBeInTheDocument();
  });

  it('renders pending work details and engine controls for engine-backed traces', async () => {
    const rootSpan = createSpan({ span_id: 'engine-control-root', name: 'Engine control root' });
    const runId = '123e4567-e89b-12d3-a456-426614174103';

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () =>
          jsonResponse(
            createEngineTraceDetail({
              engineOverrides: { projection_state: 'catching_up' },
            })
          ),
        spans: () => jsonResponse({ spans: [rootSpan] }),
        timeline: () =>
          jsonResponse({
            events: [],
            trace_status: 'RUNNING',
            has_more: false,
            engine: {
              projection_state: 'catching_up',
            },
          }),
        enginePendingWork: () => jsonResponse(createPendingWorkResponse()),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(
      await screen.findByRole('heading', { name: 'Drive the engine run from the debugger' })
    ).toBeInTheDocument();
    expect(screen.getByText('Engine wait state and queued work')).toBeInTheDocument();
    expect(screen.getByText('payments.charge · charge-card')).toBeInTheDocument();
    expect(screen.getByText('approval-timeout')).toBeInTheDocument();
    expect(screen.getByText('manual_override')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Signal' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Suspend' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Resume' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Purge' })).toBeDisabled();
    expect(countRequests(`/v1/engine/runs/${runId}/pending-work`)).toBeGreaterThan(0);
  });

  it.each([
    {
      status: 'WAITING',
      enabled: ['Signal', 'Cancel', 'Suspend', 'Terminate', 'Repair'],
      disabled: ['Resume', 'Purge'],
    },
    {
      status: 'SUSPENDED',
      enabled: ['Signal', 'Cancel', 'Resume', 'Terminate', 'Repair'],
      disabled: ['Suspend', 'Purge'],
    },
    {
      status: 'COMPLETED',
      enabled: ['Purge', 'Repair'],
      disabled: ['Signal', 'Cancel', 'Suspend', 'Resume', 'Terminate'],
    },
    {
      status: 'CONTINUED_AS_NEW',
      enabled: ['Purge', 'Repair'],
      disabled: ['Signal', 'Cancel', 'Suspend', 'Resume', 'Terminate'],
    },
  ] as const)(
    'gates engine actions for $status runs',
    async ({ status, enabled, disabled }) => {
      const rootSpan = createSpan({ span_id: `root-${status}`, name: `Root ${status}` });

      fetchMock.mockImplementation(
        buildFetchHandler({
          detail: () =>
            jsonResponse(
              createEngineTraceDetail({
                engineOverrides: { status },
                traceOverrides: {
                  status: status === 'WAITING' || status === 'SUSPENDED'
                    ? 'RUNNING'
                    : 'FAILED',
                },
              })
            ),
          spans: () => jsonResponse({ spans: [rootSpan] }),
          timeline: () =>
            jsonResponse({
              events: [],
              trace_status:
                status === 'WAITING' || status === 'SUSPENDED'
                  ? 'RUNNING'
                  : 'FAILED',
              has_more: false,
            }),
        })
      );

      renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
      expect(await screen.findByRole('button', { name: 'Repair' })).toBeInTheDocument();

      enabled.forEach((label) => {
        expect(screen.getByRole('button', { name: label })).toBeEnabled();
      });
      disabled.forEach((label) => {
        expect(screen.getByRole('button', { name: label })).toBeDisabled();
      });
    }
  );

  it('validates signal input, shows in-flight state, sends preview headers, and invalidates caches on success', async () => {
    const user = userEvent.setup();
    const rootSpan = createSpan({ span_id: 'signal-root', name: 'Signal root' });
    const deferred = createDeferredResponse();

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(createEngineTraceDetail()),
        spans: () => jsonResponse({ spans: [rootSpan] }),
        timeline: () =>
          jsonResponse({
            events: [],
            trace_status: 'RUNNING',
            has_more: false,
          }),
        enginePendingWork: () => jsonResponse(createPendingWorkResponse()),
        engineAction: (url) => {
          if (url.pathname.endsWith('/signal')) {
            return deferred.promise;
          }

          return jsonResponse({ code: 'not_found', message: 'unexpected action' }, 404);
        },
      })
    );

    const view = renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
    const invalidateSpy = vi.spyOn(view.queryClient, 'invalidateQueries');

    expect(await screen.findByRole('button', { name: 'Signal' })).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Signal' }));

    const signalNameInput = screen.getByLabelText('Signal name');
    const payloadInput = screen.getByLabelText('Payload (JSON)');
    const sendButton = screen.getByRole('button', { name: 'Send signal' });

    expect(sendButton).toBeDisabled();
    await user.type(signalNameInput, '   ');
    expect(screen.getByText('Signal name is required.')).toBeInTheDocument();
    expect(sendButton).toBeDisabled();

    await user.clear(signalNameInput);
    await user.type(signalNameInput, ' approve ');
    fireEvent.change(payloadInput, { target: { value: '{invalid' } });
    expect(screen.getByText('Payload must be valid JSON.')).toBeInTheDocument();
    expect(sendButton).toBeDisabled();

    fireEvent.change(payloadInput, {
      target: { value: '{"approved":true}' },
    });
    expect(sendButton).toBeEnabled();

    await user.click(sendButton);
    expect(sendButton).toBeDisabled();
    expect(
      screen.getAllByRole('button', { name: 'Submitting...' }).length
    ).toBeGreaterThan(0);
    expect(screen.getByText('Submitting signal...')).toBeInTheDocument();

    const signalCalls = getRequestCallsMatching(/\/v1\/engine\/runs\/[^/]+\/signal$/);
    expect(signalCalls).toHaveLength(1);
    expect(signalCalls[0]?.init?.headers).toMatchObject({
      'X-Continua-Engine-Preview': '1',
      'X-API-Key': 'test-key',
    });
    expect(signalCalls[0]?.init?.body).toBe(
      JSON.stringify({
        signal_name: 'approve',
        payload: { approved: true },
      })
    );

    deferred.resolve(
      jsonResponse({
        run_id: '123e4567-e89b-12d3-a456-426614174103',
        instance_key: 'instance-1',
        accepted: true,
        wake_applied: true,
      })
    );

    expect(await screen.findByText('Signal accepted')).toBeInTheDocument();
    expect(screen.queryByText('Signal name is required.')).not.toBeInTheDocument();
    expect(screen.queryByText('Payload must be valid JSON.')).not.toBeInTheDocument();

    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['trace', TRACE_ONE.id],
    });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['timeline', TRACE_ONE.id],
    });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['spans', TRACE_ONE.id],
    });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['enginePendingWork', '123e4567-e89b-12d3-a456-426614174103'],
    });
    expect(invalidateSpy).toHaveBeenCalledWith({
      queryKey: ['traces'],
    });
  });

  it('supports full purge mode for CONTINUED_AS_NEW runs and sends the selected mode', async () => {
    const user = userEvent.setup();
    const rootSpan = createSpan({ span_id: 'purge-root', name: 'Purge root' });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () =>
          jsonResponse(
            createEngineTraceDetail({
              engineOverrides: { status: 'CONTINUED_AS_NEW' },
              traceOverrides: { status: 'COMPLETED' },
            })
          ),
        spans: () => jsonResponse({ spans: [rootSpan] }),
        timeline: () =>
          jsonResponse({
            events: [],
            trace_status: 'COMPLETED',
            has_more: false,
          }),
        engineAction: (url) => {
          if (url.pathname.endsWith('/purge')) {
            return jsonResponse({
              run_id: '123e4567-e89b-12d3-a456-426614174103',
              mode: 'full',
              projection_state: 'journal_expired',
              deleted: true,
            });
          }

          return jsonResponse({ code: 'not_found', message: 'unexpected action' }, 404);
        },
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    const purgeButton = await screen.findByRole('button', { name: 'Purge' });
    expect(purgeButton).toBeEnabled();
    await user.click(purgeButton);

    expect(screen.getByRole('radio', { name: /Projection only/i })).toBeChecked();
    await user.click(screen.getByRole('radio', { name: /Full purge/i }));
    expect(
      screen.getByText('Permanently delete retained engine history. This cannot be recovered.')
    ).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Confirm purge' }));

    expect(await screen.findByText('Purge applied')).toBeInTheDocument();

    const purgeCalls = getRequestCallsMatching(/\/v1\/engine\/runs\/[^/]+\/purge$/);
    expect(purgeCalls).toHaveLength(1);
    expect(purgeCalls[0]?.init?.headers).toMatchObject({
      'X-Continua-Engine-Preview': '1',
      'X-API-Key': 'test-key',
    });
    expect(purgeCalls[0]?.init?.body).toBe(JSON.stringify({ mode: 'full' }));
  });

  it('replaces control feedback messages instead of stacking them', async () => {
    const user = userEvent.setup();
    const rootSpan = createSpan({ span_id: 'feedback-root', name: 'Feedback root' });
    let repairCallCount = 0;
    let signalCallCount = 0;

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(createEngineTraceDetail()),
        spans: () => jsonResponse({ spans: [rootSpan] }),
        timeline: () =>
          jsonResponse({
            events: [],
            trace_status: 'RUNNING',
            has_more: false,
          }),
        engineAction: (url) => {
          if (url.pathname.endsWith('/repair')) {
            repairCallCount += 1;

            if (repairCallCount === 1) {
              return jsonResponse({
                run_id: '123e4567-e89b-12d3-a456-426614174103',
                accepted: true,
                reason: 'repair_requested',
                projection_state: 'catching_up',
              });
            }

            if (repairCallCount === 2) {
              return jsonResponse({
                run_id: '123e4567-e89b-12d3-a456-426614174103',
                accepted: false,
                reason: 'history_expired',
                projection_state: 'journal_expired',
              });
            }

            return jsonResponse({
              run_id: '123e4567-e89b-12d3-a456-426614174103',
              accepted: false,
              reason: 'already_up_to_date',
              projection_state: 'up_to_date',
            });
          }

          if (url.pathname.endsWith('/signal')) {
            signalCallCount += 1;
            if (signalCallCount === 1) {
              return jsonResponse(
                { code: 'error', message: 'Signal delivery failed' },
                500
              );
            }

            return jsonResponse({
              run_id: '123e4567-e89b-12d3-a456-426614174103',
              instance_key: 'instance-1',
              accepted: true,
              wake_applied: false,
            });
          }

          return jsonResponse({ code: 'not_found', message: 'unexpected action' }, 404);
        },
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(await screen.findByRole('button', { name: 'Repair' })).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Repair' }));
    expect(await screen.findByText('Repair requested')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Signal' }));
    await user.type(screen.getByLabelText('Signal name'), 'approve');
    await user.click(screen.getByRole('button', { name: 'Send signal' }));
    expect(await screen.findByText('Signal failed')).toBeInTheDocument();
    expect(screen.queryByText('Repair requested')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Send signal' }));
    expect(await screen.findByText('Signal accepted')).toBeInTheDocument();
    expect(screen.queryByText('Signal failed')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Repair' }));
    expect(await screen.findByText('History expired')).toBeInTheDocument();
    expect(screen.queryByText('Signal accepted')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Repair' }));
    expect(await screen.findByText('Already up to date')).toBeInTheDocument();
    expect(screen.queryByText('History expired')).not.toBeInTheDocument();
  });

  it('shows inline conflict errors and refetches trace detail plus pending work on 409 responses', async () => {
    const user = userEvent.setup();
    const rootSpan = createSpan({ span_id: 'conflict-root', name: 'Conflict root' });
    const runId = '123e4567-e89b-12d3-a456-426614174103';

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(createEngineTraceDetail()),
        spans: () => jsonResponse({ spans: [rootSpan] }),
        timeline: () =>
          jsonResponse({
            events: [],
            trace_status: 'RUNNING',
            has_more: false,
          }),
        enginePendingWork: () => jsonResponse(createPendingWorkResponse()),
        engineAction: (url) => {
          if (url.pathname.endsWith('/suspend')) {
            return jsonResponse(
              {
                code: 'run_not_suspendable',
                message: 'Run is not suspendable',
              },
              409
            );
          }

          return jsonResponse({ code: 'not_found', message: 'unexpected action' }, 404);
        },
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(await screen.findByRole('button', { name: 'Suspend' })).toBeInTheDocument();
    const initialDetailRequests = countRequests(`/api/traces/${TRACE_ONE.id}`);
    const initialPendingRequests = countRequests(
      `/v1/engine/runs/${runId}/pending-work`
    );

    await user.click(screen.getByRole('button', { name: 'Suspend' }));

    expect(await screen.findByText('Suspend failed')).toBeInTheDocument();
    expect(screen.getByText('Run is not suspendable')).toBeInTheDocument();

    await waitFor(() => {
      expect(countRequests(`/api/traces/${TRACE_ONE.id}`)).toBeGreaterThan(
        initialDetailRequests
      );
      expect(countRequests(`/v1/engine/runs/${runId}/pending-work`)).toBeGreaterThan(
        initialPendingRequests
      );
    });
  });

  it('shows the exact-match mismatch banner and preserves returnTo through continuation links', async () => {
    const user = userEvent.setup();
    const rootSpan = createSpan({ span_id: 'continue-root', name: 'Continue root' });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: (url) => {
          const traceId = url.pathname.split('/').at(-1);

          if (traceId === TRACE_ONE.id) {
            return jsonResponse(
              createEngineTraceDetail({
                engineOverrides: {
                  failure: {
                    error_code: 'definition_version_mismatch',
                    error_message: 'missing definition version',
                    status: 'FAILED',
                  },
                  continued_from_trace_id: 'trace-prev',
                  continued_to_trace_id: 'trace-next',
                  status: 'FAILED',
                },
                traceOverrides: {
                  status: 'FAILED',
                  error_count: 1,
                },
              })
            );
          }

          if (traceId === 'trace-next') {
            return jsonResponse(
              createEngineTraceDetail({
                engineOverrides: {
                  failure: {
                    error_code: 'other_error',
                    error_message: 'other failure',
                    status: 'FAILED',
                  },
                },
                traceOverrides: {
                  ...TRACE_DETAIL,
                  id: 'trace-next',
                  name: 'Continued Trace',
                  status: 'FAILED',
                  error_count: 1,
                },
              })
            );
          }

          return jsonResponse(createEngineTraceDetail());
        },
        spans: () => jsonResponse({ spans: [rootSpan] }),
        timeline: () =>
          jsonResponse({
            events: [],
            trace_status: 'FAILED',
            has_more: false,
          }),
      })
    );

    renderTraceRoutes([
      {
        pathname: `/traces/${TRACE_ONE.id}`,
        state: { returnTo: `/sessions/${SESSION_ID}?sort_by=started_at&offset=20` },
      },
    ]);

    expect(
      await screen.findByText(
        'This run failed because the engine definition version could not be matched during activation.'
      )
    ).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '← Previous run' })).toHaveAttribute(
      'href',
      '/traces/trace-prev'
    );
    expect(screen.getByRole('link', { name: 'Next run →' })).toHaveAttribute(
      'href',
      '/traces/trace-next'
    );

    await user.click(screen.getByRole('link', { name: 'Next run →' }));

    expect(await screen.findByText('Continued Trace')).toBeInTheDocument();
    expect(screen.queryByText('Definition version mismatch')).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: '← Session' })).toHaveAttribute(
      'href',
      `/sessions/${SESSION_ID}?sort_by=started_at&offset=20`
    );
  });

  it('shows the auth recovery banner when the trace request returns 401', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse({ message: 'Invalid or missing API key' }, 401),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(await screen.findByRole('alert')).toHaveTextContent('Invalid or missing API key');
    expect(screen.getByRole('link', { name: 'Go to Settings' })).toHaveAttribute(
      'href',
      '/settings'
    );
  });

  it('shows the auth recovery banner when the timeline request returns 401', async () => {
    const rootSpan = createSpan({ span_id: 'timeline-auth-root', name: 'Timeline auth root' });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse({ ...TRACE_DETAIL, status: 'COMPLETED', error_count: 0 }),
        spans: () => jsonResponse({ spans: [rootSpan] }),
        timeline: () => jsonResponse({ message: 'Invalid or missing API key' }, 401),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(await screen.findByRole('alert')).toHaveTextContent('Invalid or missing API key');
    expect(screen.getByRole('link', { name: 'Go to Settings' })).toHaveAttribute(
      'href',
      '/settings'
    );
    expect(screen.getByRole('heading', { name: 'Execution Waterfall' })).toBeInTheDocument();
  });

  it('renders the non-desktop stacked layout with Summary active by default', async () => {
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

    expect(await screen.findByRole('button', { name: 'Summary' })).toHaveAttribute(
      'aria-pressed',
      'true'
    );
    expect(screen.getByRole('button', { name: 'Execution' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Timeline' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'State' })).toBeInTheDocument();
    expect(screen.getByLabelText('Trace Context')).toHaveAttribute(
      'aria-expanded',
      'false'
    );
  });

  it('opens the trace context sheet from a non-summary mobile tab', async () => {
    setMatchMediaMatches(false);
    const user = userEvent.setup();
    const rootSpan = createSpan({ span_id: 'mobile-context-root', name: 'Mobile context root' });

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

    await screen.findByRole('button', { name: 'Timeline' });
    await user.click(screen.getByRole('button', { name: 'Timeline' }));
    expect(screen.getByLabelText('Trace Context')).toHaveAttribute('aria-expanded', 'false');

    await user.click(screen.getByRole('button', { name: /Trace Context/i }));

    expect(await screen.findByText('External Trace ID')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Close trace context sheet' })).toBeInTheDocument();
    expect(screen.getByLabelText('Trace Context')).toHaveAttribute('aria-expanded', 'true');

    await user.click(screen.getByRole('button', { name: 'Close trace context sheet' }));
    await waitFor(() => {
      expect(screen.queryByText('External Trace ID')).not.toBeInTheDocument();
    });
    expect(screen.getByLabelText('Trace Context')).toHaveAttribute('aria-expanded', 'false');
  });

  it('renders the state tab with a badge and shows span decisions in details', async () => {
    const statefulSpan = createSpan({
      span_id: 'decision-span',
      name: 'Decision span',
    });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse({ ...TRACE_DETAIL, status: 'COMPLETED', error_count: 0 }),
        spans: () => jsonResponse({ spans: [statefulSpan] }),
        timeline: () =>
          jsonResponse({
            events: [
              createTimelineEvent({
                id: 'state-change',
                span_id: 'decision-span',
                span_name: 'Decision span',
                event_type: 'state_change',
                payload: {
                  key: 'status',
                  namespace: 'order',
                  old_value: 'pending',
                  new_value: 'approved',
                },
              }),
              createTimelineEvent({
                id: 'decision-event',
                span_id: 'decision-span',
                span_name: 'Decision span',
                event_type: 'decision',
                payload: {
                  question: 'Which model?',
                  chosen: 'gpt-4.1',
                  reasoning: 'Need higher accuracy',
                },
              }),
            ],
            trace_status: 'COMPLETED',
            has_more: false,
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}?span=decision-span`]);

    const decisionsHeading = await screen.findByText('Decisions');
    expect(
      within(decisionsHeading.parentElement!).getByText('Which model?')
    ).toBeInTheDocument();

    const stateTab = screen.getByRole('button', { name: 'State' });
    expect(stateTab).toHaveTextContent('1');

    await userEvent.setup().click(stateTab);
    expect((await screen.findAllByText('status')).length).toBeGreaterThan(0);
    expect(screen.getAllByText('pending').length).toBeGreaterThan(0);
    expect(screen.getAllByText('approved').length).toBeGreaterThan(0);
  });

  it('keeps the state tab available in the mobile workspace', async () => {
    setMatchMediaMatches(false);
    const mobileSpan = createSpan({ span_id: 'mobile-state', name: 'Mobile state span' });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse({ ...TRACE_DETAIL, status: 'COMPLETED', error_count: 0 }),
        spans: () => jsonResponse({ spans: [mobileSpan] }),
        timeline: () =>
          jsonResponse({
            events: [
              createTimelineEvent({
                id: 'mobile-state-change',
                span_id: 'mobile-state',
                span_name: 'Mobile state span',
                event_type: 'state_change',
                payload: {
                  key: 'phase',
                  old_value: 'queued',
                  new_value: 'running',
                },
              }),
            ],
            trace_status: 'COMPLETED',
            has_more: false,
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}?span=mobile-state`]);

    const stateTab = await screen.findByRole('button', { name: 'State' });
    expect(stateTab).toBeInTheDocument();

    await userEvent.setup().click(stateTab);
    expect((await screen.findAllByText('phase')).length).toBeGreaterThan(0);
    expect(screen.getAllByText('queued').length).toBeGreaterThan(0);
    expect(screen.getAllByText('running').length).toBeGreaterThan(0);
  });

  it('selects a span from the reasoning tab and switches back to details', async () => {
    const user = userEvent.setup();
    const rootSpan = createSpan({
      span_id: 'reasoning-root',
      name: 'Reasoning root',
      status: 'COMPLETED',
    });
    const childSpan = createSpan({
      span_id: 'reasoning-child',
      name: 'Reasoning child',
      parent_span_id: rootSpan.span_id,
      status: 'COMPLETED',
    });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse({ ...TRACE_DETAIL, status: 'COMPLETED', error_count: 0 }),
        spans: () => jsonResponse({ spans: [rootSpan, childSpan] }),
        timeline: () =>
          jsonResponse({
            events: [
              createTimelineEvent({
                id: 'reasoning-decision',
                span_id: childSpan.span_id,
                span_name: childSpan.name,
                event_type: 'decision',
                timestamp: '2026-03-14T10:00:03.000Z',
                payload: {
                  question: 'Choose a tool?',
                  chosen: 'calculator',
                  reasoning: 'The arithmetic branch needs deterministic output.',
                },
              }),
            ],
            trace_status: 'COMPLETED',
            has_more: false,
          }),
      })
    );

    const view = renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(
      await screen.findByRole('button', { name: 'Select span Reasoning root' })
    ).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Reasoning' }));
    await user.click(
      screen.getByRole('button', { name: /Reasoning child.*Choose a tool\?/i })
    );

    expect(view.router.state.location.search).toBe('?span=reasoning-child');
    expect(screen.getByRole('button', { name: 'Details' })).toHaveAttribute(
      'aria-pressed',
      'true'
    );
    expect(
      within(screen.getByLabelText('Span breadcrumb')).getByText('Reasoning child')
    ).toBeInTheDocument();
  });

  it('selects a span from the summary workspace and keeps summary active on mobile', async () => {
    setMatchMediaMatches(false);
    const user = userEvent.setup();
    const rootSpan = createSpan({
      span_id: 'mobile-reasoning-root',
      name: 'Mobile reasoning root',
      status: 'COMPLETED',
    });
    const childSpan = createSpan({
      span_id: 'mobile-reasoning-child',
      name: 'Mobile reasoning child',
      parent_span_id: rootSpan.span_id,
      status: 'COMPLETED',
    });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse({ ...TRACE_DETAIL, status: 'COMPLETED', error_count: 0 }),
        spans: () => jsonResponse({ spans: [rootSpan, childSpan] }),
        timeline: () =>
          jsonResponse({
            events: [
              createTimelineEvent({
                id: 'mobile-reasoning-decision',
                span_id: childSpan.span_id,
                span_name: childSpan.name,
                event_type: 'decision',
                timestamp: '2026-03-14T10:00:03.000Z',
                payload: {
                  question: 'Choose a mobile tool?',
                  chosen: 'calculator',
                },
              }),
            ],
            trace_status: 'COMPLETED',
            has_more: false,
          }),
      })
    );

    const view = renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(
      await screen.findByRole('button', { name: 'Summary' })
    ).toBeInTheDocument();

    expect(screen.getByRole('button', { name: 'Summary' })).toHaveAttribute(
      'aria-pressed',
      'true'
    );

    await user.click(
      screen.getByRole('button', {
        name: /Mobile reasoning child.*Choose a mobile tool\?/i,
      })
    );

    expect(view.router.state.location.search).toBe('?span=mobile-reasoning-child');
    expect(screen.getByRole('button', { name: 'Summary' })).toHaveAttribute(
      'aria-pressed',
      'true'
    );
    expect(
      within(screen.getByLabelText('Span breadcrumb')).getByText(
        'Mobile reasoning child'
      )
    ).toBeInTheDocument();
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

    const treeSection = screen.getByRole('heading', { name: 'Spans (2)' }).closest('section');
    expect(treeSection).not.toBeNull();
    expect(within(treeSection!).queryByText('42 tokens')).not.toBeInTheDocument();
    expect(within(treeSection!).queryByText('$0.05')).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Show metrics' }));

    expect(within(treeSection!).getByText('42 tokens')).toBeInTheDocument();
    expect(within(treeSection!).getByText('$0.05')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Show metrics' }));

    expect(within(treeSection!).queryByText('42 tokens')).not.toBeInTheDocument();
    expect(within(treeSection!).queryByText('$0.05')).not.toBeInTheDocument();
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
    const breadcrumb = await screen.findByLabelText('Span breadcrumb');
    expect(
      within(breadcrumb).getByText('Failed tool')
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
      within(await screen.findByLabelText('Span breadcrumb')).getByText('Failed tool')
    ).toBeInTheDocument();
  });

  it('surfaces consistent retry-safety guidance across failure summary, tree, waterfall, span detail, and timeline', async () => {
    const user = userEvent.setup();
    const rootSpan = createSpan({
      span_id: 'retry-root',
      name: 'Retry root',
      status: 'COMPLETED',
      ended_at: '2026-03-14T10:00:20.000Z',
    });
    const primaryFailedSpan = createSpan({
      span_id: 'retry-primary',
      name: 'Primary failed',
      parent_span_id: 'retry-root',
      status: 'FAILED',
      started_at: '2026-03-14T10:00:01.000Z',
      ended_at: '2026-03-14T10:00:05.000Z',
      error_message: 'Read-only failure',
    });
    const secondaryFailedSpan = createSpan({
      span_id: 'retry-secondary',
      name: 'Secondary failed',
      parent_span_id: 'retry-root',
      status: 'FAILED',
      started_at: '2026-03-14T10:00:06.000Z',
      ended_at: '2026-03-14T10:00:10.000Z',
      error_message: 'Unsafe failure',
    });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(TRACE_DETAIL),
        spans: () =>
          jsonResponse({
            spans: [rootSpan, primaryFailedSpan, secondaryFailedSpan],
          }),
        timeline: () =>
          jsonResponse({
            events: [
              createTimelineEvent({
                id: 'primary-effect',
                span_id: primaryFailedSpan.span_id,
                span_name: primaryFailedSpan.name,
                event_type: 'effect',
                timestamp: '2026-03-14T10:00:04.000Z',
                payload: {
                  effect_kind: 'model_call',
                  has_external_side_effect: false,
                },
              }),
              createTimelineEvent({
                id: 'secondary-effect',
                span_id: secondaryFailedSpan.span_id,
                span_name: secondaryFailedSpan.name,
                event_type: 'effect',
                timestamp: '2026-03-14T10:00:09.000Z',
                payload: {
                  effect_kind: 'api_call',
                  has_external_side_effect: true,
                  idempotent: false,
                },
              }),
            ],
            trace_status: 'FAILED',
            has_more: false,
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    const failureSummarySection = (
      await screen.findByRole('heading', { name: 'Failure Summary' })
    ).closest('section');
    expect(failureSummarySection).not.toBeNull();
    expect(
      within(failureSummarySection!).getByLabelText(getAccessibleSummary('unsafe'))
    ).toBeInTheDocument();
    expect(
      within(failureSummarySection!).getByText(
        getReasonExplanation('mutating_non_idempotent')
      )
    ).toBeInTheDocument();

    const treeSection = screen.getByRole('heading', { name: 'Spans (3)' }).closest('section');
    expect(treeSection).not.toBeNull();
    expect(
      within(treeSection!).getByLabelText(getAccessibleSummary('retryable'))
    ).toBeInTheDocument();
    expect(
      within(treeSection!).getByLabelText(getAccessibleSummary('unsafe'))
    ).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Expand all' }));

    const waterfallSection = screen
      .getByRole('heading', { name: 'Execution Waterfall' })
      .closest('section');
    expect(waterfallSection).not.toBeNull();
    await screen.findByRole('button', {
      name: 'Select waterfall span Secondary failed',
    });
    expect(
      within(waterfallSection!).getByLabelText(getAccessibleSummary('unsafe'))
    ).toBeInTheDocument();

    await user.click(
      within(failureSummarySection!).getByRole('button', {
        name: 'Jump to decisive span Secondary failed',
      })
    );

    expect(
      within(screen.getByLabelText('Span breadcrumb')).getByText('Secondary failed')
    ).toBeInTheDocument();
    expect(await screen.findByText('Retry Safety')).toBeInTheDocument();
    expect(screen.getByText('effect_kind:')).toBeInTheDocument();
    expect(screen.getAllByText('api_call').length).toBeGreaterThan(0);

    await user.click(screen.getByRole('button', { name: 'Timeline' }));

    const timelineSection = screen.getByRole('heading', { name: 'Timeline' }).closest('section');
    expect(timelineSection).not.toBeNull();
    expect(
      within(timelineSection!).getByLabelText(getAccessibleSummary('retryable'))
    ).toBeInTheDocument();
    expect(
      within(timelineSection!).getByLabelText(getAccessibleSummary('unsafe'))
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
    expect(
      await screen.findByRole('button', { name: /Trace Context/i })
    ).toBeInTheDocument();

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

  it('persists the segmented timeline filter across inspector tab switches', async () => {
    const user = userEvent.setup();
    const rootSpan = createSpan({ span_id: 'semantic-root', name: 'Semantic root' });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse({ ...TRACE_DETAIL, status: 'COMPLETED', error_count: 0 }),
        spans: () => jsonResponse({ spans: [rootSpan] }),
        timeline: () =>
          jsonResponse({
            events: [
              createTimelineEvent({
                id: 'narrative-event',
                span_id: 'semantic-root',
                span_name: 'Semantic root',
                event_type: 'message',
                message: 'Narrative event',
              }),
              createTimelineEvent({
                id: 'semantic-effect',
                span_id: 'semantic-root',
                span_name: 'Semantic root',
                event_type: 'effect',
                payload: {
                  effect_kind: 'tool_call',
                  has_external_side_effect: true,
                },
              }),
            ],
            trace_status: 'COMPLETED',
            has_more: false,
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
    expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Timeline' }));
    expect(screen.getByText('Narrative event')).toBeInTheDocument();

    await user.click(screen.getByRole('radio', { name: 'Semantic' }));
    expect(screen.getByRole('radio', { name: 'Semantic' })).toHaveAttribute(
      'aria-checked',
      'true'
    );
    expect(screen.queryByText('Narrative event')).not.toBeInTheDocument();
    expect(screen.getByText('tool_call')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Details' }));
    expect(screen.getByRole('button', { name: 'Details' })).toHaveAttribute(
      'aria-pressed',
      'true'
    );

    await user.click(screen.getByRole('button', { name: 'Timeline' }));
    expect(screen.getByRole('button', { name: 'Timeline' })).toHaveAttribute(
      'aria-pressed',
      'true'
    );
    expect(screen.getByRole('radio', { name: 'Semantic' })).toHaveAttribute(
      'aria-checked',
      'true'
    );
    expect(screen.queryByText('Narrative event')).not.toBeInTheDocument();
    expect(screen.getByText('tool_call')).toBeInTheDocument();
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

    await advancePollingInterval();

    await user.click(screen.getByRole('button', { name: 'Timeline' }));
    expect(await screen.findByText('Poll update')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Details' }));
    expect(
      within(screen.getByLabelText('Span breadcrumb')).getByText('Running child')
    ).toBeInTheDocument();
    expect(view.router.state.location.search).toBe('?span=running-child');
  }, 10000);

  it('re-derives the running-trace cost strip from the existing polling refresh without adding a new polling path', async () => {
    const runningTraceDetail = createRunningTraceDetail({
      total_cost_usd: 0.05,
    });
    const completedCostSpan = createSpan({
      span_id: 'cost-completed',
      name: 'Completed cost span',
      status: 'COMPLETED',
      started_at: '2026-03-14T10:00:00.000Z',
      ended_at: '2026-03-14T10:00:02.000Z',
      latency_ms: 2000,
      cost_usd: 0.05,
    });
    const runningSpanInitial = createSpan({
      span_id: 'cost-running',
      name: 'Running cost span',
      parent_span_id: completedCostSpan.span_id,
      status: 'STARTED',
      started_at: '2026-03-14T10:00:02.000Z',
      latency_ms: 1000,
    });
    const runningSpanCompleted = {
      ...runningSpanInitial,
      status: 'COMPLETED' as const,
      ended_at: '2026-03-14T10:00:04.000Z',
      latency_ms: 2000,
      cost_usd: 0.12,
    };
    let currentSpans = [completedCostSpan, runningSpanInitial];

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(runningTraceDetail),
        spans: () => jsonResponse({ spans: currentSpans }),
        timeline: (url) =>
          jsonResponse({
            events: [],
            trace_status: 'RUNNING',
            has_more: false,
            poll_cursor: url.searchParams.get('after') ?? 'cursor-running',
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    const waterfallSection = (
      await screen.findByRole('heading', { name: 'Execution Waterfall' })
    ).closest('section');
    expect(waterfallSection).not.toBeNull();
    expect(within(waterfallSection!).getByText('Cumulative cost')).toBeInTheDocument();
    expect(within(waterfallSection!).getByLabelText('Cumulative cost chart')).toBeInTheDocument();
    expect(within(waterfallSection!).getAllByText('$0.05').length).toBeGreaterThan(0);
    expect(within(waterfallSection!).getByText('Partial')).toBeInTheDocument();

    fetchMock.mockClear();
    currentSpans = [completedCostSpan, runningSpanCompleted];

    await advancePollingInterval();
    await act(async () => {
      await Promise.resolve();
    });

    await waitFor(() => {
      expect(screen.getByText('$0.17')).toBeInTheDocument();
    });
    expect(within(waterfallSection!).getByText('Partial')).toBeInTheDocument();

    const polledPaths = new Set(
      fetchMock.mock.calls.map(([input]) => {
        const url = new URL(readRequestUrl(input), 'http://localhost');
        return url.pathname;
      })
    );

    expect(polledPaths).toEqual(
      new Set([
        `/api/traces/${TRACE_ONE.id}/events`,
        `/api/traces/${TRACE_ONE.id}/spans`,
      ])
    );
  }, 10000);

  it('keeps failed-trace retry-safety surfaces stable when timeline data refreshes with only non-effect updates', async () => {
    const user = userEvent.setup();
    const failedTraceDetail: TraceDetail = {
      ...TRACE_DETAIL,
      status: 'FAILED',
      ended_at: '2026-03-14T10:00:02.000Z',
      error_count: 1,
    };
    const failedSpan = createSpan({
      span_id: 'poll-failed',
      name: 'Polling failed span',
      status: 'FAILED',
      started_at: '2026-03-14T10:00:00.000Z',
      ended_at: '2026-03-14T10:00:02.000Z',
      error_message: 'Initial failure',
    });
    let currentTimelineEvents = [
      createTimelineEvent({
        id: 'bootstrap-effect',
        span_id: failedSpan.span_id,
        span_name: failedSpan.name,
        event_type: 'effect',
        timestamp: '2026-03-14T10:00:01.000Z',
        payload: {
          effect_kind: 'model_call',
          has_external_side_effect: false,
        },
      }),
    ];

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(failedTraceDetail),
        spans: () => jsonResponse({ spans: [failedSpan] }),
        timeline: () =>
          jsonResponse({
            events: currentTimelineEvents,
            trace_status: 'FAILED',
            has_more: false,
          }),
      })
    );

    const view = renderTraceRoutes([`/traces/${TRACE_ONE.id}?span=poll-failed`]);

    expect(await screen.findByText('Retry Safety')).toBeInTheDocument();
    const treeSection = screen.getByRole('heading', { name: 'Spans (1)' }).closest('section');
    const waterfallSection = screen
      .getByRole('heading', { name: 'Execution Waterfall' })
      .closest('section');
    expect(treeSection).not.toBeNull();
    expect(waterfallSection).not.toBeNull();
    expect(
      within(treeSection!).getAllByLabelText(getAccessibleSummary('retryable'))
    ).toHaveLength(1);
    expect(
      within(waterfallSection!).getAllByLabelText(getAccessibleSummary('retryable'))
    ).toHaveLength(1);

    currentTimelineEvents = [
      ...currentTimelineEvents,
      createTimelineEvent({
        id: 'refreshed-log',
        span_id: failedSpan.span_id,
        span_name: failedSpan.name,
        event_type: 'message',
        timestamp: '2026-03-14T10:00:03.000Z',
        message: 'non-effect refresh update',
      }),
    ];

    await act(async () => {
      await view.queryClient.invalidateQueries({
        queryKey: ['timeline', TRACE_ONE.id, 'bootstrap'],
      });
    });

    await waitFor(() => {
      expect(
        fetchMock.mock.calls.some(([input]) => {
          const url = new URL(readRequestUrl(input), 'http://localhost');
          return url.pathname === `/api/traces/${TRACE_ONE.id}/events`;
        })
      ).toBe(true);
    });

    await user.click(screen.getByRole('button', { name: 'Timeline' }));
    expect(await screen.findByText('non-effect refresh update')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Details' }));

    const refreshedTreeSection = screen
      .getByRole('heading', { name: 'Spans (1)' })
      .closest('section');
    const refreshedWaterfallSection = screen
      .getByRole('heading', { name: 'Execution Waterfall' })
      .closest('section');
    expect(refreshedTreeSection).not.toBeNull();
    expect(refreshedWaterfallSection).not.toBeNull();
    expect(screen.getByText('Retry Safety')).toBeInTheDocument();
    expect(
      within(refreshedTreeSection!).getAllByLabelText(getAccessibleSummary('retryable'))
    ).toHaveLength(1);
    expect(
      within(refreshedWaterfallSection!).getAllByLabelText(getAccessibleSummary('retryable'))
    ).toHaveLength(1);
    expect(screen.getByRole('heading', { name: 'Failure Summary' })).toBeInTheDocument();
  }, 10000);

  it.each(['RUNNING', 'COMPLETED'] as const)(
    'does not surface retry-safety UI on %s traces',
    async (traceStatus) => {
      const user = userEvent.setup();
      const traceDetail: TraceDetail =
        traceStatus === 'RUNNING'
          ? {
              ...TRACE_DETAIL,
              status: 'RUNNING',
              ended_at: undefined,
              error_count: 0,
            }
          : {
              ...TRACE_DETAIL,
              status: 'COMPLETED',
              ended_at: '2026-03-14T10:00:05.000Z',
              error_count: 0,
            };
      const failedSpan = createSpan({
        span_id: `${traceStatus.toLowerCase()}-failed`,
        name: `${traceStatus} failed span`,
        status: 'FAILED',
        started_at: '2026-03-14T10:00:00.000Z',
        ended_at: '2026-03-14T10:00:02.000Z',
        error_message: 'Out-of-scope failure',
      });

      fetchMock.mockImplementation(
        buildFetchHandler({
          detail: () => jsonResponse(traceDetail),
          spans: () => jsonResponse({ spans: [failedSpan] }),
          timeline: () =>
            jsonResponse({
              events: [
                createTimelineEvent({
                  id: `${traceStatus.toLowerCase()}-effect`,
                  span_id: failedSpan.span_id,
                  span_name: failedSpan.name,
                  event_type: 'effect',
                  payload: {
                    effect_kind: 'model_call',
                    has_external_side_effect: false,
                  },
                }),
              ],
              trace_status: traceStatus,
              has_more: false,
            }),
        })
      );

      renderTraceRoutes([`/traces/${TRACE_ONE.id}?span=${failedSpan.span_id}`]);

      expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();
      expect(screen.queryByRole('heading', { name: 'Failure Summary' })).not.toBeInTheDocument();
      expect(screen.queryByText('Retry Safety')).not.toBeInTheDocument();

      const treeSection = screen.getByRole('heading', { name: 'Spans (1)' }).closest('section');
      const waterfallSection = screen
        .getByRole('heading', { name: 'Execution Waterfall' })
        .closest('section');
      expect(treeSection).not.toBeNull();
      expect(waterfallSection).not.toBeNull();
      expect(
        within(treeSection!).queryByLabelText(getAccessibleSummary('retryable'))
      ).not.toBeInTheDocument();
      expect(
        within(waterfallSection!).queryByLabelText(getAccessibleSummary('retryable'))
      ).not.toBeInTheDocument();

      await user.click(screen.getByRole('button', { name: 'Timeline' }));
      expect(screen.queryByLabelText(getAccessibleSummary('retryable'))).not.toBeInTheDocument();
    }
  );

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
    await user.click(screen.getByRole('radio', { name: 'Semantic' }));
    expect(screen.getByRole('radio', { name: 'Semantic' })).toHaveAttribute(
      'aria-checked',
      'true'
    );
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
    expect(screen.getByRole('radio', { name: 'All' })).toHaveAttribute(
      'aria-checked',
      'true'
    );
    expect(screen.getByRole('radio', { name: 'Semantic' })).toHaveAttribute(
      'aria-checked',
      'false'
    );
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

  it('waits for the initial timeline snapshot before showing the running-state panel', async () => {
    const now = Date.now();
    const staleStartedAt = new Date(now - 20 * 60 * 1000).toISOString();
    const recentActivityAt = new Date(now - 60 * 1000).toISOString();
    const timelineResponse = createDeferredResponse();

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(createRunningTraceDetail({ started_at: staleStartedAt })),
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
    expect(
      await screen.findByRole('button', { name: /Trace Context/i })
    ).toBeInTheDocument();
    expect(screen.queryByText('Running state')).not.toBeInTheDocument();

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

    expect(await screen.findByText('Running state')).toBeInTheDocument();
    expect(screen.getByText('Actively executing')).toBeInTheDocument();
    await userEvent.setup().click(screen.getByRole('button', { name: 'Timeline' }));
    expect(await screen.findByText('Recent activity')).toBeInTheDocument();
  });

  it.each(['COMPLETED', 'FAILED'] as const)(
    'does not render the running-state panel for %s traces',
    async (traceStatus) => {
      fetchMock.mockImplementation(
        buildFetchHandler({
          detail: () =>
            jsonResponse({
              ...TRACE_DETAIL,
              status: traceStatus,
              ended_at: '2026-03-14T10:03:00.000Z',
            }),
          spans: () => jsonResponse({ spans: [] }),
          timeline: () =>
            jsonResponse({
              events: [],
              trace_status: traceStatus,
              has_more: false,
            }),
        })
      );

      renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
      expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();
      expect(screen.queryByText('Running state')).not.toBeInTheDocument();
    }
  );

  it('renders declared waits with wait-specific text from the decisive event', async () => {
    const waitingSpan = createSpan({
      span_id: 'wait-span',
      name: 'Waiting span',
      status: 'STARTED',
      started_at: '2026-03-14T10:00:30.000Z',
    });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(createRunningTraceDetail()),
        spans: () => jsonResponse({ spans: [waitingSpan] }),
        timeline: () =>
          jsonResponse({
            events: [
              createTimelineEvent({
                id: 'wait-event',
                span_id: waitingSpan.span_id,
                span_name: waitingSpan.name,
                event_type: 'wait',
                timestamp: '2026-03-14T10:01:30.000Z',
                payload: {
                  wait_kind: 'model_response',
                  phase: 'entered',
                  wait_id: 'wait-1',
                },
              }),
            ],
            trace_status: 'RUNNING',
            has_more: false,
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    expect(await screen.findByText('Declared wait')).toBeInTheDocument();
    const runningStatePanel = within(getRunningStatePanel());
    expect(
      screen.getByText(
        'Execution declared a wait and has not yet recorded a matching resolution.'
      )
    ).toBeInTheDocument();
    expect(runningStatePanel.getByText('Current wait')).toBeInTheDocument();
    expect(runningStatePanel.getAllByText('model_response')).toHaveLength(2);
    expect(
      runningStatePanel.getAllByRole('button', { name: 'Jump to Waiting span' })
    ).toHaveLength(2);
  });

  it('renders a single unresolved wait row with metadata and jump action', async () => {
    const waitingSpan = createSpan({
      span_id: 'wait-row-span',
      name: 'Wait row span',
      status: 'STARTED',
      started_at: '2026-03-14T10:00:30.000Z',
    });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(createRunningTraceDetail()),
        spans: () => jsonResponse({ spans: [waitingSpan] }),
        timeline: () =>
          createRunningTimelineResponse([
            createWaitEvent({
              id: 'wait-row-event',
              span_id: waitingSpan.span_id,
              span_name: waitingSpan.name,
              timestamp: '2026-03-14T10:01:30.000Z',
              waitKind: 'external_api',
              waitId: 'wait-123',
              message: 'Awaiting webhook callback',
            }),
          ]),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    await screen.findByText('Running state');
    const runningStatePanel = within(getRunningStatePanel());
    expect(runningStatePanel.getByText('Open waits')).toBeInTheDocument();
    expect(runningStatePanel.getByText('Wait gate')).toBeInTheDocument();
    expect(runningStatePanel.getAllByText('external_api')).toHaveLength(2);
    expect(runningStatePanel.getByText('wait-123')).toBeInTheDocument();
    expect(runningStatePanel.getByText('2026-03-14T10:01:30.000Z')).toBeInTheDocument();
    expect(runningStatePanel.getByText('Open duration')).toBeInTheDocument();
    expect(runningStatePanel.getByText('Awaiting webhook callback')).toBeInTheDocument();
    expect(
      runningStatePanel.getAllByRole('button', { name: 'Jump to Wait row span' })
    ).toHaveLength(2);
  });

  it('renders unresolved waits newest-first', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(createRunningTraceDetail()),
        spans: () => jsonResponse({ spans: [] }),
        timeline: () =>
          createRunningTimelineResponse([
            createWaitEvent({
              id: 'oldest-wait',
              timestamp: '2026-03-14T10:01:00.000Z',
              waitKind: 'oldest_wait',
              message: 'Oldest pending wait',
            }),
            createWaitEvent({
              id: 'middle-wait',
              timestamp: '2026-03-14T10:03:00.000Z',
              waitKind: 'middle_wait',
              message: 'Middle pending wait',
            }),
            createWaitEvent({
              id: 'newest-wait',
              timestamp: '2026-03-14T10:05:00.000Z',
              waitKind: 'newest_wait',
              message: 'Newest pending wait',
            }),
          ]),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    await screen.findByText('Open waits');
    const runningStatePanel = within(getRunningStatePanel());
    expect(
      runningStatePanel
        .getAllByText(/pending wait$/)
        .map((element) => element.textContent)
    ).toEqual([
      'Newest pending wait',
      'Middle pending wait',
      'Oldest pending wait',
    ]);
  });

  it('uses the Approval gate title for human approval waits', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(createRunningTraceDetail()),
        spans: () => jsonResponse({ spans: [] }),
        timeline: () =>
          createRunningTimelineResponse([
            createWaitEvent({
              id: 'approval-wait',
              timestamp: '2026-03-14T10:01:30.000Z',
              waitKind: 'human_approval',
              message: 'Awaiting manager sign-off',
            }),
          ]),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    await screen.findByText('Approval gate');
    const runningStatePanel = within(getRunningStatePanel());
    expect(runningStatePanel.getByText('Approval gate')).toBeInTheDocument();
    expect(runningStatePanel.getByText('Awaiting manager sign-off')).toBeInTheDocument();
  });

  it('does not render an open-wait jump button when the span is not resolvable', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(createRunningTraceDetail()),
        spans: () => jsonResponse({ spans: [] }),
        timeline: () =>
          createRunningTimelineResponse([
            createWaitEvent({
              id: 'orphaned-wait',
              span_id: 'missing-span',
              span_name: 'Missing span',
              timestamp: '2026-03-14T10:01:30.000Z',
              waitKind: 'external_api',
              message: 'Awaiting orphaned callback',
            }),
          ]),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    await screen.findByText('Running state');
    const runningStatePanel = within(getRunningStatePanel());
    expect(runningStatePanel.getByText('Awaiting orphaned callback')).toBeInTheDocument();
    expect(
      runningStatePanel.queryByRole('button', { name: 'Jump to Missing span' })
    ).not.toBeInTheDocument();
  });

  it('removes a resolved wait row on the next poll cycle', async () => {
    const openWait = createWaitEvent({
      id: 'open-wait',
      timestamp: '2026-03-14T10:01:30.000Z',
      waitKind: 'external_api',
      waitId: 'wait-1',
      message: 'Waiting on external API',
    });
    let currentPollEvents: TimelineEvent[] = [];
    let currentPollCursor = 'cursor-poll-1';

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(createRunningTraceDetail()),
        spans: () => jsonResponse({ spans: [] }),
        timeline: (url) => {
          if (url.searchParams.has('after')) {
            return createRunningTimelineResponse(currentPollEvents, currentPollCursor);
          }

          return createRunningTimelineResponse([openWait], 'cursor-open-wait');
        },
      })
    );

    const view = renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
    await screen.findByText('Running state');
    const runningStatePanel = within(getRunningStatePanel());
    expect(runningStatePanel.getByText('wait-1')).toBeInTheDocument();

    currentPollEvents = [
      createWaitEvent({
        id: 'resolved-wait',
        timestamp: '2026-03-14T10:02:00.000Z',
        waitKind: 'external_api',
        phase: 'resolved',
        waitId: 'wait-1',
        resolution: 'completed',
      }),
    ];
    currentPollCursor = 'cursor-poll-2';

    await act(async () => {
      await view.queryClient.refetchQueries({
        queryKey: ['timeline', TRACE_ONE.id, 'poll'],
      });
    });

    await waitFor(() => {
      expect(screen.queryByText('wait-1')).not.toBeInTheDocument();
    });
    expect(screen.queryByText('Open waits')).not.toBeInTheDocument();
  });

  it('keeps anonymous waits visible as standalone rows', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(createRunningTraceDetail()),
        spans: () => jsonResponse({ spans: [] }),
        timeline: () =>
          createRunningTimelineResponse([
            createWaitEvent({
              id: 'anonymous-entered',
              timestamp: '2026-03-14T10:01:30.000Z',
              waitKind: 'custom',
              message: 'Anonymous gate entered',
            }),
            createWaitEvent({
              id: 'anonymous-resolved',
              timestamp: '2026-03-14T10:02:00.000Z',
              waitKind: 'custom',
              phase: 'resolved',
              resolution: 'ignored',
              message: 'Anonymous gate resolved',
            }),
          ]),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

    await screen.findByText('Running state');
    const runningStatePanel = within(getRunningStatePanel());
    expect(runningStatePanel.getByText('Anonymous gate entered')).toBeInTheDocument();
    expect(runningStatePanel.getAllByText('custom')).toHaveLength(2);
    expect(screen.queryByText('wait-1')).not.toBeInTheDocument();
  });

  it('refreshes open wait durations only when the timeline poll re-renders the panel', async () => {
    const baseNow = new Date('2026-03-14T10:02:00.000Z').getTime();
    const traceStartedAt = new Date(baseNow - 2 * 60 * 1000).toISOString();
    const dateNowSpy = vi.spyOn(Date, 'now').mockReturnValue(baseNow);

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () =>
          jsonResponse(createRunningTraceDetail({ started_at: traceStartedAt })),
        spans: () => jsonResponse({ spans: [] }),
        timeline: (url) => {
          if (url.searchParams.get('after') === 'cursor-duration') {
            return createRunningTimelineResponse([], 'cursor-duration');
          }

          return createRunningTimelineResponse(
            [
              createWaitEvent({
                id: 'duration-wait',
                timestamp: new Date(baseNow - 60 * 1000).toISOString(),
                waitKind: 'timer',
                message: 'Waiting on timer',
              }),
            ],
            'cursor-duration'
          );
        },
      })
    );

    const view = renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
    expect(await screen.findByText('Waiting on timer')).toBeInTheDocument();
    expect(screen.getByText('Open duration').parentElement).toHaveTextContent('1.0m');

    dateNowSpy.mockReturnValue(baseNow + 30 * 1000);
    expect(screen.getByText('Open duration').parentElement).toHaveTextContent('1.0m');

    await act(async () => {
      await view.queryClient.refetchQueries({
        queryKey: ['timeline', TRACE_ONE.id, 'poll'],
      });
    });

    await waitFor(() => {
      expect(screen.getByText('Open duration').parentElement).toHaveTextContent('1.5m');
    });
  });

  it.each([
    {
      kind: 'LLM' as const,
      label: 'Waiting on model',
      spanId: 'model-span',
      spanName: 'Model call',
    },
    {
      kind: 'TOOL' as const,
      label: 'Waiting on tool',
      spanId: 'tool-span',
      spanName: 'Tool call',
    },
  ])(
    'uses the decisive span jump action for $label states',
    async ({ kind, label, spanId, spanName }) => {
      const user = userEvent.setup();
      const decisiveSpan = createSpan({
        span_id: spanId,
        name: spanName,
        kind,
        status: 'STARTED',
        started_at: '2026-03-14T10:01:00.000Z',
      });

      fetchMock.mockImplementation(
        buildFetchHandler({
          detail: () => jsonResponse(createRunningTraceDetail()),
          spans: () => jsonResponse({ spans: [decisiveSpan] }),
          timeline: () =>
            jsonResponse({
              events: [],
              trace_status: 'RUNNING',
              has_more: false,
            }),
        })
      );

      renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);

      expect(await screen.findByText(label)).toBeInTheDocument();
      await user.click(screen.getByRole('button', { name: `Jump to ${spanName}` }));
      expect(
        within(screen.getByLabelText('Span breadcrumb')).getByText(spanName)
      ).toBeInTheDocument();
    }
  );

  it('renders actively executing for recent work between spans', async () => {
    const now = Date.now();
    const traceStartedAt = new Date(now - 2 * 60 * 1000).toISOString();

    const completedSpan = createSpan({
      span_id: 'completed-work',
      name: 'Completed work',
      status: 'COMPLETED',
      started_at: new Date(now - 90 * 1000).toISOString(),
      ended_at: new Date(now - 30 * 1000).toISOString(),
    });

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () =>
          jsonResponse(createRunningTraceDetail({ started_at: traceStartedAt })),
        spans: () => jsonResponse({ spans: [completedSpan] }),
        timeline: () =>
          jsonResponse({
            events: [],
            trace_status: 'RUNNING',
            has_more: false,
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
    expect(await screen.findByText('Actively executing')).toBeInTheDocument();
    expect(
      screen.getByText(
        'Recent activity suggests execution is still progressing between spans.'
      )
    ).toBeInTheDocument();
    expect(screen.queryByText('Unknown')).not.toBeInTheDocument();
  });

  it('renders possibly stalled when a running trace has no stronger signal', async () => {
    const traceStartedAt = new Date(Date.now() - 20 * 60 * 1000).toISOString();

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(createRunningTraceDetail({ started_at: traceStartedAt })),
        spans: () => jsonResponse({ spans: [] }),
        timeline: () =>
          jsonResponse({
            events: [],
            trace_status: 'RUNNING',
            has_more: false,
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
    expect(await screen.findByText('Possibly stalled')).toBeInTheDocument();
    expect(
      screen.getByText(
        'Execution is still marked running, but recent activity is sparse.'
      )
    ).toBeInTheDocument();
  });

  it('renders unknown with conservative copy when the debugger lacks running evidence', async () => {
    const traceStartedAt = new Date(Date.now() - 2 * 60 * 1000).toISOString();

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () => jsonResponse(createRunningTraceDetail({ started_at: traceStartedAt })),
        spans: () => jsonResponse({ spans: [] }),
        timeline: () =>
          jsonResponse({
            events: [],
            trace_status: 'RUNNING',
            has_more: false,
          }),
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
    expect(await screen.findByText('Unknown')).toBeInTheDocument();
    expect(
      screen.getByText('The debugger cannot yet explain where it is waiting.')
    ).toBeInTheDocument();
  });

  it('refreshes spans during live execution so new spans become visible within one poll cycle', async () => {
    const now = Date.now();
    const traceStartedAt = new Date(now - 2 * 60 * 1000).toISOString();

    let currentSpans = [
      createSpan({
        span_id: 'root-running',
        name: 'Root running span',
        kind: 'CHAIN',
        status: 'STARTED',
        started_at: new Date(now - 90 * 1000).toISOString(),
      }),
    ];

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () =>
          jsonResponse(createRunningTraceDetail({ started_at: traceStartedAt })),
        spans: () => jsonResponse({ spans: currentSpans }),
        timeline: (url) => {
          if (url.searchParams.get('after') === 'cursor-live') {
            return jsonResponse({
              events: [],
              trace_status: 'RUNNING',
              has_more: false,
              poll_cursor: 'cursor-live',
            });
          }

          return jsonResponse({
            events: [],
            trace_status: 'RUNNING',
            has_more: false,
            poll_cursor: 'cursor-live',
          });
        },
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
    expect(await screen.findByText('Actively executing')).toBeInTheDocument();

    currentSpans = [
      ...currentSpans,
      createSpan({
        span_id: 'llm-live',
        name: 'Live model span',
        kind: 'LLM',
        status: 'STARTED',
        started_at: new Date(now - 30 * 1000).toISOString(),
      }),
    ];

    await advancePollingInterval();

    expect(await screen.findByText('Waiting on model')).toBeInTheDocument();
    expect(countRequests(`/api/traces/${TRACE_ONE.id}/spans`)).toBeGreaterThanOrEqual(2);
  }, 10000);

  it('updates the running-state classification when an open span completes during live execution', async () => {
    const now = Date.now();
    const traceStartedAt = new Date(now - 2 * 60 * 1000).toISOString();

    let currentSpans = [
      createSpan({
        span_id: 'tool-live',
        name: 'Live tool span',
        kind: 'TOOL',
        status: 'STARTED',
        started_at: new Date(now - 45 * 1000).toISOString(),
      }),
    ];

    fetchMock.mockImplementation(
      buildFetchHandler({
        detail: () =>
          jsonResponse(createRunningTraceDetail({ started_at: traceStartedAt })),
        spans: () => jsonResponse({ spans: currentSpans }),
        timeline: (url) => {
          if (url.searchParams.get('after') === 'cursor-live-complete') {
            return jsonResponse({
              events: [],
              trace_status: 'RUNNING',
              has_more: false,
              poll_cursor: 'cursor-live-complete',
            });
          }

          return jsonResponse({
            events: [],
            trace_status: 'RUNNING',
            has_more: false,
            poll_cursor: 'cursor-live-complete',
          });
        },
      })
    );

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
    expect(await screen.findByText('Waiting on tool')).toBeInTheDocument();

    currentSpans = [
      {
        ...currentSpans[0],
        status: 'COMPLETED',
        ended_at: new Date(now - 5 * 1000).toISOString(),
      },
    ];

    await advancePollingInterval();

    expect(await screen.findByText('Actively executing')).toBeInTheDocument();
    expect(screen.queryByText('Waiting on tool')).not.toBeInTheDocument();
  }, 10000);

  it.each(['COMPLETED', 'FAILED'] as const)(
    'does not refresh spans for %s traces after terminal status',
    async (traceStatus) => {
      fetchMock.mockImplementation(
        buildFetchHandler({
          detail: () =>
            jsonResponse({
              ...TRACE_DETAIL,
              status: traceStatus,
              ended_at: '2026-03-14T10:04:00.000Z',
            }),
          spans: () =>
            jsonResponse({
              spans: [
                createSpan({
                  span_id: `${traceStatus.toLowerCase()}-root`,
                  name: `${traceStatus} root span`,
                  status: traceStatus === 'FAILED' ? 'FAILED' : 'COMPLETED',
                }),
              ],
            }),
          timeline: () =>
            jsonResponse({
              events: [],
              trace_status: traceStatus,
              has_more: false,
            }),
        })
      );

      renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
      expect(await screen.findByText('Checkout Trace')).toBeInTheDocument();

      const initialSpansRequestCount = countRequests(
        `/api/traces/${TRACE_ONE.id}/spans`
      );

      await advancePollingInterval();

      expect(countRequests(`/api/traces/${TRACE_ONE.id}/spans`)).toBe(
        initialSpansRequestCount
      );

      await act(async () => {
        focusManager.setFocused(false);
        focusManager.setFocused(true);
        onlineManager.setOnline(false);
        onlineManager.setOnline(true);
        await new Promise((resolve) => setTimeout(resolve, 50));
      });

      expect(countRequests(`/api/traces/${TRACE_ONE.id}/spans`)).toBe(
        initialSpansRequestCount
      );

      focusManager.setFocused(undefined);
    }
  );

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

  it('accepts session-detail returnTo links', async () => {
    fetchMock.mockImplementation(buildFetchHandler());

    renderTraceRoutes([
      {
        pathname: `/traces/${TRACE_ONE.id}`,
        state: { returnTo: `/sessions/${SESSION_ID}?sort_by=started_at&offset=20` },
      },
    ]);

    expect(
      await screen.findByRole('button', { name: /Trace Context/i })
    ).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '← Session' })).toHaveAttribute(
      'href',
      `/sessions/${SESSION_ID}?sort_by=started_at&offset=20`
    );
  });

  it('shows session external ID before the UUID in trace context', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(buildFetchHandler());

    renderTraceRoutes([`/traces/${TRACE_ONE.id}`]);
    await screen.findByRole('button', { name: /Trace Context/i });

    await user.click(screen.getByRole('button', { name: /Trace Context/i }));

    expect(
      screen.getByRole('link', {
        name: new RegExp(`${SESSION_EXTERNAL_ID}\\s*${SESSION_ID}`),
      })
    ).toBeInTheDocument();
  });
});
