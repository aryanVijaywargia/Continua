import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render } from '@testing-library/react';
import { RouterProvider, createMemoryRouter } from 'react-router-dom';
import { vi } from 'vitest';
import { ThemeProvider } from '../hooks/ThemeProvider';
import {
  EMPTY_SESSION_NARRATIVE,
  OTHER_SESSION_EXTERNAL_ID,
  OTHER_SESSION_ID,
  RUNNING_SESSION_NARRATIVE,
  SESSION_COMPARE,
  SESSION_EXTERNAL_ID,
  SESSION_ID,
  SESSION_NARRATIVE,
  SESSION_ONE,
  SESSION_TWO,
  TRACE_DETAIL,
  TRACE_ONE,
  TRACE_THREE,
  TRACE_TWO,
  TRACE_ZETA,
  TRUNCATED_SESSION_NARRATIVE,
  createSessionNarrativeTrace,
  createSpan,
  createTimelineEvent,
  resetTestEntityCounter,
} from '../test/traceFixtures';
import { TraceDetailPage } from './TraceDetailPage';
import { SessionComparePage } from './SessionComparePage';
import { SessionDetailPage } from './SessionDetailPage';
import { SessionsPage } from './SessionsPage';
import { SettingsPage } from './SettingsPage';
import { TracesPage } from './TracesPage';
import { OverviewPage } from './OverviewPage';
export {
  EMPTY_SESSION_NARRATIVE,
  OTHER_SESSION_EXTERNAL_ID,
  OTHER_SESSION_ID,
  RUNNING_SESSION_NARRATIVE,
  SESSION_COMPARE,
  SESSION_EXTERNAL_ID,
  SESSION_ID,
  SESSION_NARRATIVE,
  SESSION_ONE,
  SESSION_TWO,
  TRACE_DETAIL,
  TRACE_ONE,
  TRACE_THREE,
  TRACE_TWO,
  TRACE_ZETA,
  TRUNCATED_SESSION_NARRATIVE,
  createSessionNarrativeTrace,
  createSpan,
  createTimelineEvent,
  resetTestEntityCounter,
};

export type RequestInput = string | URL | Request;

export type JsonHandler = (
  url: URL,
  init?: RequestInit
) => Promise<Response> | Response;

export function jsonResponse(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(data), {
    status,
    headers: {
      'Content-Type': 'application/json',
    },
  });
}

export function buildFetchHandler({
  list,
  detail,
  sessionsList,
  sessionDetail,
  sessionCompare,
  sessionNarrative,
  spans,
  timeline,
  enginePendingWork,
  engineAction,
}: {
  list?: JsonHandler;
  detail?: JsonHandler;
  sessionsList?: JsonHandler;
  sessionDetail?: JsonHandler;
  sessionCompare?: JsonHandler;
  sessionNarrative?: JsonHandler;
  spans?: JsonHandler;
  timeline?: JsonHandler;
  enginePendingWork?: JsonHandler;
  engineAction?: JsonHandler;
} = {}) {
  return async (input: RequestInput, init?: RequestInit) => {
    const url = new URL(readRequestUrl(input), 'http://localhost');

    if (url.pathname === '/api/traces') {
      return (
        list?.(url, init) ??
        jsonResponse({
          traces: [TRACE_ONE, TRACE_TWO],
          total: 2,
        })
      );
    }

    if (/^\/api\/traces\/[^/]+\/spans$/.test(url.pathname)) {
      return spans?.(url, init) ?? jsonResponse({ spans: [] });
    }

    if (/^\/api\/traces\/[^/]+\/events$/.test(url.pathname)) {
      return (
        timeline?.(url, init) ??
        jsonResponse({
          events: [],
          trace_status: 'COMPLETED',
          has_more: false,
        })
      );
    }

    if (/^\/api\/traces\/[^/]+$/.test(url.pathname)) {
      if (detail) {
        return detail(url, init);
      }

      const traceId = url.pathname.split('/').at(-1);
      if (traceId === TRACE_ONE.id) {
        return jsonResponse(TRACE_DETAIL);
      }
      if (traceId === TRACE_TWO.id) {
        return jsonResponse({
          ...TRACE_DETAIL,
          ...TRACE_TWO,
          trace_id: 'external-trace-latency',
          user_id: 'user-456',
          tags: ['latency'],
        });
      }
      if (traceId === TRACE_THREE.id) {
        return jsonResponse({
          ...TRACE_DETAIL,
          ...TRACE_THREE,
          trace_id: 'external-trace-alpha',
          user_id: 'user-123',
          tags: ['alpha'],
        });
      }

      return jsonResponse(TRACE_DETAIL);
    }

    if (url.pathname === '/api/sessions') {
      return (
        sessionsList?.(url, init) ??
        jsonResponse({
          sessions: [SESSION_ONE, SESSION_TWO],
          total: 2,
        })
      );
    }

    if (/^\/api\/sessions\/[^/]+\/narrative$/.test(url.pathname)) {
      if (sessionNarrative) {
        return sessionNarrative(url, init);
      }

      const sessionId = url.pathname.split('/').at(-2);
      if (sessionId === SESSION_ID) {
        return jsonResponse(SESSION_NARRATIVE);
      }
      if (sessionId === OTHER_SESSION_ID) {
        return jsonResponse(RUNNING_SESSION_NARRATIVE);
      }
      return jsonResponse(EMPTY_SESSION_NARRATIVE);
    }

    if (/^\/api\/sessions\/[^/]+\/compare$/.test(url.pathname)) {
      if (sessionCompare) {
        return sessionCompare(url, init);
      }

      return jsonResponse({ code: 'not_found', message: 'Resource not found' }, 404);
    }

    if (/^\/api\/sessions\/[^/]+$/.test(url.pathname)) {
      if (sessionDetail) {
        return sessionDetail(url, init);
      }

      const sessionId = url.pathname.split('/').at(-1);
      if (sessionId === SESSION_ID) {
        return jsonResponse(SESSION_ONE);
      }
      if (sessionId === OTHER_SESSION_ID) {
        return jsonResponse(SESSION_TWO);
      }
      return jsonResponse({ code: 'not_found', message: 'Resource not found' }, 404);
    }

    if (/^\/v1\/engine\/runs\/[^/]+\/pending-work$/.test(url.pathname)) {
      if (enginePendingWork) {
        return enginePendingWork(url, init);
      }

      const runId = url.pathname.split('/').at(-2);
      return jsonResponse({
        run_id: runId,
        current_wait: null,
        activities: [],
        timers: [],
        signals: [],
        pending_activity_tasks: 0,
        pending_inbox_items: 0,
      });
    }

    if (
      /^\/v1\/engine\/runs\/[^/]+\/(signal|cancel|suspend|resume|terminate|purge|repair)$/.test(
        url.pathname
      )
    ) {
      if (engineAction) {
        return engineAction(url, init);
      }

      const runId = url.pathname.split('/')[4];
      const action = url.pathname.split('/').at(-1);

      switch (action) {
        case 'signal':
        case 'cancel':
          return jsonResponse({
            run_id: runId,
            instance_key: 'instance-1',
            accepted: true,
            wake_applied: false,
          });
        case 'suspend':
          return jsonResponse(createEngineRunResponse(runId, 'SUSPENDED'));
        case 'resume':
          return jsonResponse(createEngineRunResponse(runId, 'RUNNING'));
        case 'terminate':
          return jsonResponse({
            run_id: runId,
            status: 'TERMINATED',
            result: null,
          });
        case 'purge':
          return jsonResponse({
            run_id: runId,
            mode: 'projection_only',
            projection_state: 'summary_only',
            deleted: true,
          });
        case 'repair':
          return jsonResponse({
            run_id: runId,
            accepted: true,
            reason: 'repair_requested',
            projection_state: 'catching_up',
          });
        default:
          break;
      }
    }

    throw new Error(`Unhandled request: ${url.pathname}${url.search}`);
  };
}

function createEngineRunResponse(
  runId: string | undefined,
  status: 'RUNNING' | 'SUSPENDED'
) {
  return {
    run_id: runId,
    instance_id: 'instance-id-1',
    instance_key: 'instance-1',
    definition_name: 'checkout',
    definition_version: 'v1',
    projection_state: 'summary_only',
    status,
    created_at: '2026-03-14T10:00:00.000Z',
    updated_at: '2026-03-14T10:00:05.000Z',
    pending_work: {
      pending_activity_tasks: 0,
      pending_inbox_items: 0,
    },
  };
}

export function readRequestUrl(input: RequestInput): string {
  if (typeof input === 'string') {
    return input;
  }

  if (input instanceof URL) {
    return input.toString();
  }

  return input.url;
}

export function getRequests(
  fetchMock: ReturnType<typeof vi.fn>,
  pathname: string
): URL[] {
  return fetchMock.mock.calls
    .map(([input]) => new URL(readRequestUrl(input as RequestInput), 'http://localhost'))
    .filter((url) => url.pathname === pathname);
}

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
      mutations: {
        gcTime: Infinity,
      },
    },
  });
}

export function renderTraceRoutes(
  initialEntries: Array<string | { pathname: string; search?: string; state?: unknown }>,
  options: { initialIndex?: number } = {}
) {
  const queryClient = createQueryClient();
  const router = createMemoryRouter(
    [
      { path: '/dashboard', element: <OverviewPage /> },
      { path: '/traces', element: <TracesPage /> },
      { path: '/traces/:id', element: <TraceDetailPage /> },
      { path: '/sessions', element: <SessionsPage /> },
      { path: '/sessions/:id', element: <SessionDetailPage /> },
      { path: '/sessions/:id/compare', element: <SessionComparePage /> },
      { path: '/settings', element: <SettingsPage /> },
    ],
    {
      initialEntries,
      initialIndex: options.initialIndex,
    }
  );

  const view = render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <RouterProvider router={router} />
      </ThemeProvider>
    </QueryClientProvider>
  );

  return { ...view, queryClient, router };
}

export function mockClipboard() {
  const writeText = vi.fn().mockResolvedValue(undefined);

  Object.defineProperty(window.navigator, 'clipboard', {
    configurable: true,
    value: { writeText },
  });

  return writeText;
}

export function createDeferredResponse() {
  let resolveResponse: (response: Response) => void = () => {};
  const promise = new Promise<Response>((resolve) => {
    resolveResponse = resolve;
  });

  return {
    promise,
    resolve: resolveResponse,
  };
}
