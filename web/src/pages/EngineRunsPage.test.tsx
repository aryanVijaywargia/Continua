import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { MemoryRouter } from 'react-router-dom';
import { clearApiKey, setApiKey } from '../api/client';
import { TRACE_ONE } from '../test/traceFixtures';
import { ThemeProvider } from '../hooks/ThemeProvider';
import { jsonResponse, readRequestUrl, type RequestInput } from './testUtils';
import { EngineRunsPage } from './EngineRunsPage';

let fetchMock: ReturnType<typeof vi.fn>;

function renderEngineRunsPage() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });

  return render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <MemoryRouter initialEntries={['/engine/runs']}>
          <EngineRunsPage />
        </MemoryRouter>
      </ThemeProvider>
    </QueryClientProvider>
  );
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
  vi.unstubAllGlobals();
});

describe('EngineRunsPage', () => {
  it('requests engine-only traces and renders engine status labels', async () => {
    fetchMock.mockImplementation((input: RequestInput) => {
      const url = new URL(readRequestUrl(input), 'http://localhost');
      if (url.pathname !== '/api/traces') {
        throw new Error(`Unhandled request: ${url.pathname}`);
      }

      return jsonResponse({
        traces: [
          {
            ...TRACE_ONE,
            id: 'engine-trace-1',
            name: 'Darklaunch run',
            error_count: 0,
            engine: {
              run_id: '123e4567-e89b-12d3-a456-426614174100',
              definition_name: 'darklaunch.demo',
              definition_version: 'v1',
              projection_state: 'up_to_date',
              instance_key: 'darklaunch-1',
              status: 'QUEUED',
              pending_work: {
                pending_activity_tasks: 0,
                pending_inbox_items: 0,
              },
              updated_at: '2026-03-14T10:00:03.000Z',
            },
          },
        ],
        total: 1,
      });
    });

    renderEngineRunsPage();

    expect(await screen.findByText('darklaunch.demo · v1')).toBeInTheDocument();
    expect(screen.getByText('Queued')).toBeInTheDocument();
    expect(screen.queryByText('darklaunch.demo · vv1')).not.toBeInTheDocument();

    await waitFor(() => {
      const requestUrl = new URL(
        readRequestUrl(fetchMock.mock.calls[0]?.[0] as RequestInput),
        'http://localhost'
      );
      expect(requestUrl.pathname).toBe('/api/traces');
      expect(requestUrl.searchParams.get('engine_only')).toBe('true');
    });
  });
});
