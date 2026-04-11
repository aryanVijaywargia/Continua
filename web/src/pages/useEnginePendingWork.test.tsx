import { type ReactNode } from 'react';
import { act, renderHook, waitFor } from '@testing-library/react';
import {
  QueryClient,
  QueryClientProvider,
} from '@tanstack/react-query';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { clearApiKey, setApiKey } from '../api/client';
import { shouldPollEnginePendingWork, useEnginePendingWork } from './useEnginePendingWork';
import { TIMELINE_POLL_INTERVAL_MS } from './useTraceTimeline';

let fetchMock: ReturnType<typeof vi.fn>;

function createWrapper(queryClient: QueryClient) {
  return function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
  };
}

function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });
}

beforeEach(() => {
  fetchMock = vi.fn().mockResolvedValue(
    new Response(
      JSON.stringify({
        run_id: 'run-1',
        current_wait: null,
        activities: [],
        timers: [],
        signals: [],
        pending_activity_tasks: 0,
        pending_inbox_items: 0,
      }),
      {
        status: 200,
        headers: {
          'Content-Type': 'application/json',
        },
      }
    )
  );
  vi.stubGlobal('fetch', fetchMock);
  setApiKey('test-key');
});

afterEach(() => {
  clearApiKey();
  localStorage.clear();
  vi.unstubAllGlobals();
});

describe('useEnginePendingWork', () => {
  it.each([
    ['QUEUED', true],
    ['RUNNING', true],
    ['WAITING', true],
    ['SUSPENDED', true],
    ['COMPLETED', false],
    ['FAILED', false],
    ['CANCELLED', false],
    ['TERMINATED', false],
    ['CONTINUED_AS_NEW', false],
    [undefined, false],
  ] as const)(
    'returns %s polling state as %s',
    (status, expected) => {
      expect(shouldPollEnginePendingWork(status)).toBe(expected);
    }
  );

  it('uses the enginePendingWork query key, polls active statuses, and omits the preview header', async () => {
    const queryClient = createQueryClient();
    const wrapper = createWrapper(queryClient);

    const { result } = renderHook(
      () => useEnginePendingWork('run-1', 'WAITING'),
      { wrapper }
    );

    await waitFor(() => {
      expect(result.current.data?.run_id).toBe('run-1');
    });

    expect(queryClient.getQueryState(['enginePendingWork', 'run-1'])).toBeDefined();
    expect(fetchMock.mock.calls.length).toBeGreaterThan(0);
    expect(fetchMock.mock.calls[0]?.[0]).toBe('/v1/engine/runs/run-1/pending-work');
    expect(
      (fetchMock.mock.calls[0]?.[1] as RequestInit | undefined)?.headers
    ).toMatchObject({
      'Content-Type': 'application/json',
      'X-API-Key': 'test-key',
    });
    expect(
      ((fetchMock.mock.calls[0]?.[1] as RequestInit | undefined)?.headers as Record<
        string,
        string
      > | undefined)?.['X-Continua-Engine-Preview']
    ).toBeUndefined();

    const initialCallCount = fetchMock.mock.calls.length;

    await act(async () => {
      await new Promise((resolve) =>
        setTimeout(resolve, TIMELINE_POLL_INTERVAL_MS + 20)
      );
    });

    await waitFor(() => {
      expect(fetchMock.mock.calls.length).toBeGreaterThan(initialCallCount);
    });
  });

  it('does not poll terminal statuses after the initial fetch', async () => {
    const queryClient = createQueryClient();
    const wrapper = createWrapper(queryClient);

    renderHook(() => useEnginePendingWork('run-1', 'COMPLETED'), { wrapper });

    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalledTimes(1);
    });

    await act(async () => {
      await new Promise((resolve) =>
        setTimeout(resolve, TIMELINE_POLL_INTERVAL_MS + 20)
      );
    });

    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
