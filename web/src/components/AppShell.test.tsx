import type { ReactNode } from 'react';
import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { clearApiKey, setApiKey } from '../api/client';
import {
  RuntimeAuthStateProvider,
  type RuntimeAuthState,
} from '../auth/runtime';
import { ThemeProvider } from '../hooks/ThemeProvider';
import { setMatchMediaMatches } from '../test/matchMedia';
import { AppShell } from './AppShell';

const PRIMARY_PROJECT_ID = '11111111-1111-1111-1111-111111111111';
const SECONDARY_PROJECT_ID = '22222222-2222-2222-2222-222222222222';

const auth0State = vi.hoisted(() => ({
  error: undefined as Error | undefined,
  getAccessTokenSilently: vi.fn(async () => 'test-key'),
  isAuthenticated: true,
  isLoading: false,
  loginWithRedirect: vi.fn(),
  logout: vi.fn(),
  user: {
    email: 'operator@continua.dev',
    name: 'Continua Operator',
    sub: 'google-oauth2|operator',
  },
}));

vi.mock('@auth0/auth0-react', () => ({
  Auth0Provider: ({ children }: { children: ReactNode }) => children,
  useAuth0: () => auth0State,
}));

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

function LocationProbe() {
  const location = useLocation();

  return (
    <div data-testid="location-probe">
      {location.pathname}
      {location.search}
    </div>
  );
}

async function renderShell(
  initialEntry = `/dashboard?project_id=${PRIMARY_PROJECT_ID}`,
  auth?: Partial<RuntimeAuthState>
) {
  const queryClient = createQueryClient();
  const runtimeAuth: RuntimeAuthState = {
    status: 'ready',
    enabled: true,
    ...auth,
  };

  return render(
    <QueryClientProvider client={queryClient}>
      <ThemeProvider>
        <RuntimeAuthStateProvider auth={runtimeAuth}>
          <MemoryRouter initialEntries={[initialEntry]}>
            <Routes>
              <Route element={<AppShell />}>
                <Route
                  path="/dashboard"
                  element={
                    <>
                      <div>Overview content</div>
                      <LocationProbe />
                    </>
                  }
                />
                <Route
                  path="/traces"
                  element={
                    <>
                      <div>Trace list content</div>
                      <LocationProbe />
                    </>
                  }
                />
                <Route
                  path="/sessions"
                  element={
                    <>
                      <div>Session list content</div>
                      <LocationProbe />
                    </>
                  }
                />
                <Route
                  path="/settings"
                  element={
                    <>
                      <div>Settings content</div>
                      <LocationProbe />
                    </>
                  }
                />
              </Route>
            </Routes>
          </MemoryRouter>
        </RuntimeAuthStateProvider>
      </ThemeProvider>
    </QueryClientProvider>
  );
}

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn(async (input, init) => {
    const requestUrl =
      typeof input === 'string'
        ? input
        : input instanceof URL
          ? input.toString()
          : input.url;
    const url = new URL(requestUrl, 'http://localhost');

    if (url.pathname === '/api/projects') {
      return new Response(
        JSON.stringify({
          projects: [
            {
              id: PRIMARY_PROJECT_ID,
              name: 'Primary Project',
              created_at: '2026-03-14T10:00:00.000Z',
              updated_at: '2026-03-14T10:00:00.000Z',
            },
            {
              id: SECONDARY_PROJECT_ID,
              name: 'Secondary Project',
              created_at: '2026-03-15T10:00:00.000Z',
              updated_at: '2026-03-15T10:00:00.000Z',
            },
          ],
        }),
        {
          status: 200,
          headers: {
            'Content-Type': 'application/json',
          },
        }
      );
    }

    throw new Error(`Unhandled request: ${url.pathname} ${String(init?.method ?? 'GET')}`);
  });
  vi.stubGlobal('fetch', fetchMock);
  clearApiKey();
  setApiKey('test-key');
  auth0State.error = undefined;
  auth0State.getAccessTokenSilently.mockResolvedValue('test-key');
  auth0State.isAuthenticated = true;
  auth0State.isLoading = false;
  auth0State.loginWithRedirect.mockReset();
  auth0State.logout.mockReset();
  auth0State.user = {
    email: 'operator@continua.dev',
    name: 'Continua Operator',
    sub: 'google-oauth2|operator',
  };
});

afterEach(() => {
  cleanup();
  clearApiKey();
  vi.unstubAllGlobals();
});

describe('AppShell', () => {
  it('renders the operator shell, project switcher, and project-aware navigation', async () => {
    await renderShell(`/sessions?project_id=${PRIMARY_PROJECT_ID}`);

    expect(await screen.findByDisplayValue('Primary Project')).toBeInTheDocument();
    expect(screen.getByText('operator@continua.dev')).toBeInTheDocument();
    expect(screen.getByText('Session list content')).toBeInTheDocument();

    const primaryNav = screen.getByRole('navigation', { name: 'Primary' });
    expect(within(primaryNav).getByText('Overview').closest('a')).toHaveAttribute(
      'href',
      `/dashboard?project_id=${PRIMARY_PROJECT_ID}`
    );
    expect(within(primaryNav).getByText('Traces').closest('a')).toHaveAttribute(
      'href',
      `/traces?project_id=${PRIMARY_PROJECT_ID}`
    );
    expect(within(primaryNav).getByText('Sessions').closest('a')).toHaveAttribute(
      'aria-current',
      'page'
    );
    expect(screen.getByTestId('location-probe')).toHaveTextContent(
      `/sessions?project_id=${PRIMARY_PROJECT_ID}`
    );

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(
      new URL(String(fetchMock.mock.calls[0]?.[0]), 'http://localhost').pathname
    ).toBe('/api/projects');
    expect(
      (fetchMock.mock.calls[0]?.[1] as RequestInit | undefined)?.headers
    ).toMatchObject({
      'Content-Type': 'application/json',
      Authorization: 'Bearer test-key',
    });
  });

  it('switches the active project and keeps the selected project in route state', async () => {
    const user = userEvent.setup();
    await renderShell(`/dashboard?project_id=${PRIMARY_PROJECT_ID}`);

    expect(await screen.findByDisplayValue('Primary Project')).toBeInTheDocument();
    await user.selectOptions(
      screen.getByLabelText('Active project'),
      SECONDARY_PROJECT_ID
    );

    await waitFor(() => {
      expect(screen.getByTestId('location-probe')).toHaveTextContent(
        `/dashboard?project_id=${SECONDARY_PROJECT_ID}`
      );
    });
    expect(screen.getByRole('link', { name: 'Traces' })).toHaveAttribute(
      'href',
      `/traces?project_id=${SECONDARY_PROJECT_ID}`
    );
  });

  it('selects the first project and canonicalizes the URL on first protected visit', async () => {
    await renderShell('/dashboard');

    expect(await screen.findByDisplayValue('Primary Project')).toBeInTheDocument();
    expect(screen.getByText('Overview content')).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByTestId('location-probe')).toHaveTextContent(
        `/dashboard?project_id=${PRIMARY_PROJECT_ID}`
      );
    });
  });

  it('loads projects with a saved local API key when Auth0 is disabled', async () => {
    auth0State.isAuthenticated = false;
    await renderShell('/dashboard', {
      enabled: false,
    });

    expect(await screen.findByDisplayValue('Primary Project')).toBeInTheDocument();
    expect(screen.getByText('Local API key')).toBeInTheDocument();
    expect(screen.getByText('Overview content')).toBeInTheDocument();
    expect(
      (fetchMock.mock.calls[0]?.[1] as RequestInit | undefined)?.headers
    ).toMatchObject({
      Authorization: 'Bearer test-key',
    });
  });

  it('shows project loading failures instead of staying on the loading state', async () => {
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ code: 'invalid_api_key', message: 'Invalid API key' }), {
        status: 401,
        headers: {
          'Content-Type': 'application/json',
        },
      })
    );

    await renderShell('/dashboard', {
      enabled: false,
    });

    expect(await screen.findByText('Project loading failed')).toBeInTheDocument();
    expect(screen.getByText('Invalid API key')).toBeInTheDocument();
    expect(screen.queryByText('Loading projects')).not.toBeInTheDocument();
  });

  it('refetches projects when the local API key changes', async () => {
    auth0State.isAuthenticated = false;
    fetchMock.mockImplementation(async (_input, init) => {
      const headers = init?.headers as Record<string, string> | undefined;
      const token = headers?.Authorization;
      const project =
        token === 'Bearer next-key'
          ? {
              id: SECONDARY_PROJECT_ID,
              name: 'Secondary Project',
              created_at: '2026-03-15T10:00:00.000Z',
              updated_at: '2026-03-15T10:00:00.000Z',
            }
          : {
              id: PRIMARY_PROJECT_ID,
              name: 'Primary Project',
              created_at: '2026-03-14T10:00:00.000Z',
              updated_at: '2026-03-14T10:00:00.000Z',
            };

      return new Response(JSON.stringify({ projects: [project] }), {
        status: 200,
        headers: {
          'Content-Type': 'application/json',
        },
      });
    });

    await renderShell('/dashboard', {
      enabled: false,
    });

    expect(await screen.findByDisplayValue('Primary Project')).toBeInTheDocument();

    act(() => {
      setApiKey('next-key');
    });

    expect(await screen.findByDisplayValue('Secondary Project')).toBeInTheDocument();
    expect(fetchMock).toHaveBeenLastCalledWith(
      expect.any(String),
      expect.objectContaining({
        headers: expect.objectContaining({
          Authorization: 'Bearer next-key',
        }),
      })
    );
  });

  it('leaves the mounted shell when the local API key is cleared', async () => {
    auth0State.isAuthenticated = false;
    await renderShell('/dashboard', {
      enabled: false,
    });

    expect(await screen.findByDisplayValue('Primary Project')).toBeInTheDocument();

    act(() => {
      clearApiKey();
    });

    expect(await screen.findByText('Local API key required')).toBeInTheDocument();
  });

  it('opens the command palette from the shell control', async () => {
    const user = userEvent.setup();
    await renderShell();

    expect(await screen.findByDisplayValue('Primary Project')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: /Command Palette/i }));
    expect(screen.getByRole('combobox', { name: 'Search commands' })).toBeInTheDocument();
  });

  it('uses a utility drawer instead of duplicate navigation when primary nav is visible', async () => {
    const user = userEvent.setup();
    setMatchMediaMatches(true);

    await renderShell();

    expect(await screen.findByDisplayValue('Primary Project')).toBeInTheDocument();
    expect(screen.getByText('Tools')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Open operator tools' }));

    expect(screen.getByText('Operator tools')).toBeInTheDocument();
    expect(
      screen.getByText(/Use the top bar to switch projects and move between views/i)
    ).toBeInTheDocument();
    expect(screen.queryByRole('navigation', { name: 'Mobile primary' })).not.toBeInTheDocument();
    expect(screen.queryByLabelText('Project')).not.toBeInTheDocument();
  });

  it('keeps full navigation in the drawer on mobile breakpoints', async () => {
    const user = userEvent.setup();
    setMatchMediaMatches(false);

    await renderShell();

    expect(await screen.findByDisplayValue('Primary Project')).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Open navigation' }));

    expect(screen.getByRole('navigation', { name: 'Mobile primary' })).toBeInTheDocument();
    expect(screen.getByLabelText('Project')).toHaveValue(PRIMARY_PROJECT_ID);
  });

  it('switches the shell into read-only demo mode without loading projects', async () => {
    setMatchMediaMatches(true);

    await renderShell('/dashboard', {
      enabled: false,
      public_demo_enabled: true,
      public_demo_label: 'Sample data',
    });

    expect(screen.getByText('Overview content')).toBeInTheDocument();
    expect(screen.getByText('Sample data')).toBeInTheDocument();
    expect(screen.getByText(/read-only demo: sample traces are hosted here/i)).toBeInTheDocument();
    expect(
      screen.getByRole('link', { name: 'Run locally with your own traces' })
    ).toBeInTheDocument();
    expect(screen.queryByLabelText('Active project')).not.toBeInTheDocument();
    expect(screen.queryByText('operator@continua.dev')).not.toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Settings' })).not.toBeInTheDocument();
    expect(fetchMock).not.toHaveBeenCalled();
  });
});
