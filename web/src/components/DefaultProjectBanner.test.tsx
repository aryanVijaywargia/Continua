import type { ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { clearApiKey, setApiKey } from '../api/client';
import { RuntimeAuthStateProvider, type RuntimeAuthState } from '../auth/runtime';
import { DefaultProjectBanner } from './DefaultProjectBanner';
import { jsonResponse, type RequestInput } from '../pages/testUtils';

const LOCAL_AUTH_STATE: RuntimeAuthState = {
  status: 'ready',
  enabled: false,
};

const AUTH0_ENABLED_STATE: RuntimeAuthState = {
  status: 'ready',
  enabled: true,
  domain: 'continua.us.auth0.com',
};

vi.mock('@auth0/auth0-react', () => ({
  Auth0Provider: ({ children }: { children: ReactNode }) => children,
  useAuth0: () => ({
    getAccessTokenSilently: vi.fn(async () => 'test-key'),
    isAuthenticated: true,
    isLoading: false,
  }),
}));

const DEFAULT_PROJECT = {
  id: '00000000-0000-0000-0000-000000000001',
  name: 'Default Project',
  created_at: '2026-05-01T00:00:00.000Z',
  updated_at: '2026-05-01T00:00:00.000Z',
};

function renderBanner(auth: RuntimeAuthState = LOCAL_AUTH_STATE) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <RuntimeAuthStateProvider auth={auth}>
        <MemoryRouter>
          <DefaultProjectBanner />
        </MemoryRouter>
      </RuntimeAuthStateProvider>
    </QueryClientProvider>
  );
}

beforeEach(() => {
  setApiKey('default');
  window.localStorage.removeItem('continua_default_project_nudge_dismissed');
});

afterEach(() => {
  clearApiKey();
  vi.restoreAllMocks();
});

describe('DefaultProjectBanner', () => {
  it('renders only when the default project is the sole project', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInput) => {
        const url = typeof input === 'string' ? input : input.toString();
        if (url.includes('/api/projects')) {
          return jsonResponse({ projects: [DEFAULT_PROJECT] });
        }
        throw new Error(`unexpected ${url}`);
      })
    );

    renderBanner();
    expect(await screen.findByTestId('default-project-banner')).toBeInTheDocument();
  });

  it('hides when additional projects exist', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInput) => {
        const url = typeof input === 'string' ? input : input.toString();
        if (url.includes('/api/projects')) {
          return jsonResponse({
            projects: [
              DEFAULT_PROJECT,
              {
                id: 'other',
                name: 'other',
                created_at: '2026-05-02T00:00:00.000Z',
                updated_at: '2026-05-02T00:00:00.000Z',
              },
            ],
          });
        }
        throw new Error(`unexpected ${url}`);
      })
    );

    renderBanner();
    await waitFor(() => {
      expect(screen.queryByTestId('default-project-banner')).toBeNull();
    });
  });

  it('stays hidden once the user has rotated past the seeded default key', async () => {
    setApiKey('pk_userrotatedkey');
    const fetchMock = vi.fn(async (input: RequestInput) => {
      const url = typeof input === 'string' ? input : input.toString();
      if (url.includes('/api/projects')) {
        return jsonResponse({ projects: [DEFAULT_PROJECT] });
      }
      throw new Error(`unexpected ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    renderBanner();

    // Wait until the projects query has actually resolved at least once, then
    // assert the banner never rendered. This avoids the act() warning from
    // unawaited React Query state updates.
    await waitFor(() => {
      expect(fetchMock).toHaveBeenCalled();
    });
    await waitFor(() => {
      expect(screen.queryByTestId('default-project-banner')).toBeNull();
    });
  });

  it('stays hidden in Auth0 mode even if localStorage still holds "default"', async () => {
    // Edge case: an operator has Auth0 sign-in, but localStorage was never cleared
    // from a previous local-mode session. The banner must not show.
    setApiKey('default');
    const fetchMock = vi.fn(async (input: RequestInput) => {
      const url = typeof input === 'string' ? input : input.toString();
      if (url.includes('/api/projects')) {
        return jsonResponse({ projects: [DEFAULT_PROJECT] });
      }
      throw new Error(`unexpected ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    renderBanner(AUTH0_ENABLED_STATE);

    // No fetch should be issued in Auth0 mode because the query is disabled.
    // Use waitFor to give React time to commit and run effects; the assertion
    // is that nothing changes during that window.
    await waitFor(() => {
      expect(fetchMock).not.toHaveBeenCalled();
      expect(screen.queryByTestId('default-project-banner')).toBeNull();
    });
  });

  it('persists dismissal across renders', async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      'fetch',
      vi.fn(async (input: RequestInput) => {
        const url = typeof input === 'string' ? input : input.toString();
        if (url.includes('/api/projects')) {
          return jsonResponse({ projects: [DEFAULT_PROJECT] });
        }
        throw new Error(`unexpected ${url}`);
      })
    );

    const { unmount } = renderBanner();
    const banner = await screen.findByTestId('default-project-banner');
    expect(banner).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /dismiss/i }));

    await waitFor(() => {
      expect(screen.queryByTestId('default-project-banner')).toBeNull();
    });
    unmount();

    renderBanner();
    // Banner should not appear on re-render because dismissal is persisted.
    await waitFor(() => {
      expect(screen.queryByTestId('default-project-banner')).toBeNull();
    });
  });
});
