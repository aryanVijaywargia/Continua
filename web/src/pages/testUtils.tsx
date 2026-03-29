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
import { SessionDetailPage } from './SessionDetailPage';
import { SessionsPage } from './SessionsPage';
import { SettingsPage } from './SettingsPage';
import { TracesPage } from './TracesPage';
export {
  EMPTY_SESSION_NARRATIVE,
  OTHER_SESSION_EXTERNAL_ID,
  OTHER_SESSION_ID,
  RUNNING_SESSION_NARRATIVE,
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

export type JsonHandler = (url: URL) => Promise<Response> | Response;

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
  sessionNarrative,
  spans,
  timeline,
}: {
  list?: JsonHandler;
  detail?: JsonHandler;
  sessionsList?: JsonHandler;
  sessionDetail?: JsonHandler;
  sessionNarrative?: JsonHandler;
  spans?: JsonHandler;
  timeline?: JsonHandler;
} = {}) {
  return async (input: RequestInput) => {
    const url = new URL(readRequestUrl(input), 'http://localhost');

    if (url.pathname === '/api/traces') {
      return (
        list?.(url) ??
        jsonResponse({
          traces: [TRACE_ONE, TRACE_TWO],
          total: 2,
        })
      );
    }

    if (/^\/api\/traces\/[^/]+\/spans$/.test(url.pathname)) {
      return spans?.(url) ?? jsonResponse({ spans: [] });
    }

    if (/^\/api\/traces\/[^/]+\/events$/.test(url.pathname)) {
      return (
        timeline?.(url) ??
        jsonResponse({
          events: [],
          trace_status: 'COMPLETED',
          has_more: false,
        })
      );
    }

    if (/^\/api\/traces\/[^/]+$/.test(url.pathname)) {
      return detail?.(url) ?? jsonResponse(TRACE_DETAIL);
    }

    if (url.pathname === '/api/sessions') {
      return (
        sessionsList?.(url) ??
        jsonResponse({
          sessions: [SESSION_ONE, SESSION_TWO],
          total: 2,
        })
      );
    }

    if (/^\/api\/sessions\/[^/]+\/narrative$/.test(url.pathname)) {
      if (sessionNarrative) {
        return sessionNarrative(url);
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

    if (/^\/api\/sessions\/[^/]+$/.test(url.pathname)) {
      if (sessionDetail) {
        return sessionDetail(url);
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

    throw new Error(`Unhandled request: ${url.pathname}${url.search}`);
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
        retry: false,
      },
    },
  });
}

export function renderTraceRoutes(
  initialEntries: Array<string | { pathname: string; state?: unknown }>,
  options: { initialIndex?: number } = {}
) {
  const queryClient = createQueryClient();
  const router = createMemoryRouter(
    [
      { path: '/traces', element: <TracesPage /> },
      { path: '/traces/:id', element: <TraceDetailPage /> },
      { path: '/sessions', element: <SessionsPage /> },
      { path: '/sessions/:id', element: <SessionDetailPage /> },
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
