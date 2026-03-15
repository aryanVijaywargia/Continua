import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { clearApiKey, setApiKey } from '../api/client';
import {
  SESSION_EXTERNAL_ID,
  SESSION_ID,
  TRACE_ONE,
  TRACE_TWO,
  buildFetchHandler,
  getRequests,
  jsonResponse,
  renderTraceRoutes,
} from './testUtils';

let fetchMock: ReturnType<typeof vi.fn>;

function getTraceListRequests(): URL[] {
  return getRequests(fetchMock, '/api/traces');
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

    expect(await screen.findByText('Trace Context')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: '← Session' })).toHaveAttribute(
      'href',
      `/sessions/${SESSION_ID}?offset=20&sort_by=started_at&sort_dir=asc`
    );
  });
});
