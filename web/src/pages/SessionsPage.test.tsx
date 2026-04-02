import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { clearApiKey, setApiKey } from '../api/client';
import {
  OTHER_SESSION_ID,
  SESSION_ONE,
  SESSION_TWO,
  buildFetchHandler,
  getRequests,
  jsonResponse,
  readRequestUrl,
  renderTraceRoutes,
} from './testUtils';

let fetchMock: ReturnType<typeof vi.fn>;

function getSessionListRequests(): URL[] {
  return getRequests(fetchMock, '/api/sessions');
}

function countNarrativeRequests(): number {
  return fetchMock.mock.calls.filter(([input]) => {
    const url = new URL(readRequestUrl(input), 'http://localhost');
    return /^\/api\/sessions\/[^/]+\/narrative$/.test(url.pathname);
  }).length;
}

async function waitForSessionListFetch(search: string) {
  await waitFor(() => {
    expect(getSessionListRequests().at(-1)?.search).toBe(search);
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

describe('SessionsPage', () => {
  it('rehydrates URL state and fetches with the typed sessions params', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionsList: () => jsonResponse({ sessions: [SESSION_ONE], total: 1 }),
      })
    );

    const { router } = renderTraceRoutes([
      '/sessions?q=conv&user_id=user-42&sort_by=trace_count&sort_dir=desc&limit=50&offset=100',
    ]);

    expect(await screen.findByText('conv-checkout-123')).toBeInTheDocument();
    await waitForSessionListFetch('?limit=50&offset=100&q=conv&user_id=user-42');
    await waitFor(() => {
      expect(router.state.location.search).toBe('?limit=50&offset=100&q=conv&user_id=user-42');
    });

    expect(screen.getByLabelText('Search')).toHaveValue('conv');
    expect(screen.getByLabelText('User ID')).toHaveValue('user-42');
    expect(screen.getByRole('combobox', { name: 'Rows per page' })).toHaveDisplayValue('50');
  });

  it('shows the auth recovery banner when the sessions request returns 401', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionsList: () => jsonResponse({ message: 'Invalid or missing API key' }, 401),
      })
    );

    renderTraceRoutes(['/sessions']);

    expect(await screen.findByRole('alert')).toHaveTextContent('Invalid or missing API key');
    expect(screen.getByRole('link', { name: 'Go to Settings' })).toHaveAttribute(
      'href',
      '/settings'
    );
  });

  it('normalizes malformed params in the URL', async () => {
    fetchMock.mockImplementation(buildFetchHandler());

    const { router } = renderTraceRoutes(['/sessions?sort_by=invalid&limit=999']);
    expect(await screen.findByText('conv-checkout-123')).toBeInTheDocument();

    await waitForSessionListFetch('?limit=100');
    await waitFor(() => {
      expect(router.state.location.search).toBe('?limit=100');
    });
  });

  it('strips sort params from the URL while search is active', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(buildFetchHandler());

    const { router } = renderTraceRoutes(['/sessions?sort_by=trace_count&sort_dir=desc']);
    expect(await screen.findByText('conv-checkout-123')).toBeInTheDocument();

    await user.type(screen.getByLabelText('Search'), 'conv');
    await waitForSessionListFetch('?limit=20&q=conv');
    await waitFor(() => {
      expect(router.state.location.search).toBe('?q=conv');
    });
    expect(screen.queryByRole('button', { name: 'Traces' })).not.toBeInTheDocument();
  });

  it('repairs stale offsets and keeps external ID first in the row UI', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionsList: (url) => {
          const offset = url.searchParams.get('offset');
          if (offset === '40') {
            return jsonResponse({ sessions: [], total: 21 });
          }
          if (offset === '20') {
            return jsonResponse({ sessions: [SESSION_TWO], total: 21 });
          }
          return jsonResponse({ sessions: [SESSION_ONE], total: 21 });
        },
      })
    );

    const { router } = renderTraceRoutes(['/sessions?offset=40']);
    expect(await screen.findByText('conv-latency-456')).toBeInTheDocument();

    await waitFor(() => {
      expect(getSessionListRequests().map((request) => request.search)).toEqual([
        '?limit=20&offset=40',
        '?limit=20&offset=20',
      ]);
    });
    expect(router.state.location.search).toBe('?offset=20');
    expect(screen.getByText('conv-latency-456')).toBeInTheDocument();
    expect(screen.getByText(OTHER_SESSION_ID)).toBeInTheDocument();
  });

  it('does not fan out narrative requests while rendering the session index', async () => {
    fetchMock.mockImplementation(buildFetchHandler());

    renderTraceRoutes(['/sessions']);

    expect(await screen.findByText('conv-checkout-123')).toBeInTheDocument();
    await waitFor(() => {
      expect(countNarrativeRequests()).toBe(0);
    });
  });

  it('resets offset when sort and page size change', async () => {
    const user = userEvent.setup();
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionsList: (url) =>
          jsonResponse({
            sessions: url.searchParams.get('offset') === '20' ? [SESSION_TWO] : [SESSION_ONE],
            total: 60,
          }),
      })
    );

    const { router } = renderTraceRoutes(['/sessions']);
    expect(await screen.findByText('conv-checkout-123')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Next page' }));
    await waitForSessionListFetch('?limit=20&offset=20');

    await user.click(screen.getByRole('button', { name: 'Created' }));
    await waitForSessionListFetch('?limit=20&sort_by=created_at&sort_dir=asc');
    await waitFor(() => {
      expect(router.state.location.search).toBe('?sort_by=created_at&sort_dir=asc');
    });

    await user.selectOptions(screen.getByRole('combobox', { name: 'Rows per page' }), '50');
    await waitForSessionListFetch('?limit=50&sort_by=created_at&sort_dir=asc');
    await waitFor(() => {
      expect(router.state.location.search).toBe('?limit=50&sort_by=created_at&sort_dir=asc');
    });
  });
});
