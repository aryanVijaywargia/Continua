import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render } from '@testing-library/react';
import { RouterProvider, createMemoryRouter } from 'react-router-dom';
import { vi } from 'vitest';
import {
  OTHER_SESSION_ID,
  SESSION_ID,
  TRACE_DETAIL,
  TRACE_ONE,
  TRACE_THREE,
  TRACE_TWO,
  TRACE_ZETA,
  createSpan,
  createTimelineEvent,
  resetTestEntityCounter,
} from '../test/traceFixtures';
import { TraceDetailPage } from './TraceDetailPage';
import { TracesPage } from './TracesPage';
export {
  OTHER_SESSION_ID,
  SESSION_ID,
  TRACE_DETAIL,
  TRACE_ONE,
  TRACE_THREE,
  TRACE_TWO,
  TRACE_ZETA,
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
  spans,
  timeline,
}: {
  list?: JsonHandler;
  detail?: JsonHandler;
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
    ],
    {
      initialEntries,
      initialIndex: options.initialIndex,
    }
  );

  const view = render(
    <QueryClientProvider client={queryClient}>
      <RouterProvider router={router} />
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
