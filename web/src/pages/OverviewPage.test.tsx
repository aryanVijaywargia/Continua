import { screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { clearApiKey, setApiKey } from '../api/client';
import { buildFetchHandler, jsonResponse, renderTraceRoutes } from './testUtils';

let fetchMock: ReturnType<typeof vi.fn>;

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

describe('OverviewPage', () => {
  it('renders snapshot metrics and operator jump actions from existing trace/session endpoints', async () => {
    fetchMock.mockImplementation(buildFetchHandler());

    renderTraceRoutes(['/dashboard']);

    expect(await screen.findByRole('heading', { name: 'Trace volume' })).toBeInTheDocument();
    expect(screen.getByText('Tracked traces')).toBeInTheDocument();
    expect(screen.getByText('Running now')).toBeInTheDocument();
    expect(screen.getByText('Failed traces')).toBeInTheDocument();
    expect(screen.getByText('Sessions')).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Live runs' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Recent traces' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /View all/i })).toHaveAttribute('href', '/traces');
  });

  it('keeps overview content visible when one supporting query fails', async () => {
    fetchMock.mockImplementation(
      buildFetchHandler({
        sessionsList: () => jsonResponse({ message: 'Session list unavailable' }, 500),
      })
    );

    renderTraceRoutes(['/dashboard']);

    expect(await screen.findByText(/Overview data is partially unavailable/i)).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Recent traces' })).toBeInTheDocument();
    expect(screen.getAllByText('Checkout Trace').length).toBeGreaterThan(0);
    expect(screen.getByText(/Session list unavailable/)).toBeInTheDocument();
    expect(screen.queryByText('No sessions yet')).not.toBeInTheDocument();
  });
});
