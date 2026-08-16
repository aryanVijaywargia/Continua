import { useEffect, useState, type ReactNode } from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  clearApiKey,
  fetchAPI,
  isAuthProviderEnabled,
  setAccessTokenProvider,
  setAuthProviderEnabled,
  setLocalSingleUserMode,
  setPublicDemoMode,
} from '../api/client';
import {
  Auth0RuntimeProvider,
  ConsoleRoute,
  ProtectedRoute,
  useRuntimeAuthState,
} from './runtime';

const PROJECT_ID = '11111111-1111-1111-1111-111111111111';
const RUN_LOCALLY_DOCS_URL =
  'https://www.continua.in/docs/guides/installation';

const auth0State = vi.hoisted(() => ({
  error: undefined as Error | undefined,
  getAccessTokenSilently: vi.fn(async () => 'operator-token'),
  isAuthenticated: true,
  isLoading: false,
  loginWithRedirect: vi.fn(async () => undefined),
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

function RuntimeAuthProbe() {
  const auth = useRuntimeAuthState();

  return (
    <div>
      <div data-testid="auth-status">{auth.status}</div>
      <div data-testid="auth-enabled">{String(auth.enabled)}</div>
      <div data-testid="auth-domain">{auth.domain ?? ''}</div>
      <div data-testid="auth-client-id">{auth.client_id ?? ''}</div>
      <div data-testid="auth-audience">{auth.audience ?? ''}</div>
      <div data-testid="auth-public-demo">{String(auth.public_demo_enabled ?? false)}</div>
      <div data-testid="auth-public-demo-label">{auth.public_demo_label ?? ''}</div>
      <div data-testid="auth-console-available">{String(auth.console_available ?? true)}</div>
    </div>
  );
}

function DemoReadProbe() {
  const [status, setStatus] = useState('idle');

  useEffect(() => {
    void fetchAPI<{ traces: unknown[]; total: number }>('/api/traces')
      .then(() => setStatus('loaded'))
      .catch((error) => {
        setStatus(error instanceof Error ? error.message : 'failed');
      });
  }, []);

  return <div>{status}</div>;
}

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
  clearApiKey();
  setAccessTokenProvider(null);
  setPublicDemoMode(false);
  setLocalSingleUserMode(false);
  setAuthProviderEnabled(false);
  auth0State.error = undefined;
  auth0State.isAuthenticated = true;
  auth0State.isLoading = false;
  auth0State.loginWithRedirect.mockClear();
});

afterEach(() => {
  clearApiKey();
  setAccessTokenProvider(null);
  setPublicDemoMode(false);
  setLocalSingleUserMode(false);
  setAuthProviderEnabled(false);
  vi.unstubAllGlobals();
});

describe('runtime auth', () => {
  it('loads the Auth0 runtime config from /api/auth/config', async () => {
    fetchMock.mockResolvedValue(
      new Response(
        JSON.stringify({
          enabled: true,
          domain: 'continua.us.auth0.com',
          client_id: 'client-id',
          audience: 'https://continua/api',
          public_demo_enabled: true,
          public_demo_label: 'Sample data',
        }),
        {
          status: 200,
          headers: {
            'Content-Type': 'application/json',
          },
        }
      )
    );

    render(<RuntimeAuthProbe />);

    await waitFor(() => {
      expect(screen.getByTestId('auth-status')).toHaveTextContent('ready');
    });
    expect(screen.getByTestId('auth-enabled')).toHaveTextContent('true');
    expect(screen.getByTestId('auth-domain')).toHaveTextContent('continua.us.auth0.com');
    expect(screen.getByTestId('auth-client-id')).toHaveTextContent('client-id');
    expect(screen.getByTestId('auth-audience')).toHaveTextContent('https://continua/api');
    expect(screen.getByTestId('auth-public-demo')).toHaveTextContent('true');
    expect(screen.getByTestId('auth-public-demo-label')).toHaveTextContent('Sample data');
    expect(fetchMock).toHaveBeenCalledWith('/api/auth/config');
  });

  it('falls back to local API-key mode when auth config is unavailable on localhost', async () => {
    fetchMock.mockRejectedValue(new TypeError('Failed to fetch'));

    render(<RuntimeAuthProbe />);

    await waitFor(() => {
      expect(screen.getByTestId('auth-status')).toHaveTextContent('ready');
    });
    expect(screen.getByTestId('auth-enabled')).toHaveTextContent('false');
    expect(screen.getByTestId('auth-domain')).toHaveTextContent('');
    expect(fetchMock).toHaveBeenCalledWith('/api/auth/config');
  });

  it('marks the console unavailable when static hosting returns HTML for auth config', async () => {
    fetchMock.mockResolvedValue(
      new Response('<!DOCTYPE html><html></html>', {
        status: 200,
        headers: {
          'Content-Type': 'text/html; charset=utf-8',
        },
      })
    );

    render(<RuntimeAuthProbe />);

    await waitFor(() => {
      expect(screen.getByTestId('auth-status')).toHaveTextContent('ready');
    });
    expect(screen.getByTestId('auth-enabled')).toHaveTextContent('false');
    expect(screen.getByTestId('auth-console-available')).toHaveTextContent('false');
  });

  it('shows a static-hosting message instead of the local API-key form', () => {
    render(
      <MemoryRouter initialEntries={['/dashboard']}>
        <Routes>
          <Route
            element={
              <ProtectedRoute
                auth={{
                  status: 'ready',
                  enabled: false,
                  console_available: false,
                }}
              />
            }
          >
            <Route path="/dashboard" element={<div>Dashboard</div>} />
          </Route>
        </Routes>
      </MemoryRouter>
    );

    expect(screen.getByText('Console backend not connected')).toBeInTheDocument();
    expect(screen.queryByText('Enter a local project API key.')).not.toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Run locally' })).toHaveAttribute(
      'href',
      RUN_LOCALLY_DOCS_URL
    );
  });

  it('allows public demo console routes without triggering Auth0 login', () => {
    auth0State.isAuthenticated = false;

    render(
      <MemoryRouter initialEntries={['/traces']}>
        <Routes>
          <Route
            element={
              <ConsoleRoute
                auth={{
                  status: 'ready',
                  enabled: false,
                  public_demo_enabled: true,
                  public_demo_label: 'Sample data',
                }}
              />
            }
          >
            <Route path="/traces" element={<div>Public demo traces</div>} />
          </Route>
        </Routes>
      </MemoryRouter>
    );

    expect(screen.getByText('Public demo traces')).toBeInTheDocument();
    expect(auth0State.loginWithRedirect).not.toHaveBeenCalled();
  });

  it('keeps public demo mode unauthenticated when Auth0 config is also present', async () => {
    auth0State.isAuthenticated = false;
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ traces: [], total: 0 }), {
        status: 200,
        headers: {
          'Content-Type': 'application/json',
        },
      })
    );

    render(
      <MemoryRouter>
        <Auth0RuntimeProvider
          auth={{
            status: 'ready',
            enabled: true,
            domain: 'continua.us.auth0.com',
            client_id: 'client-id',
            audience: 'https://continua/api',
            public_demo_enabled: true,
            public_demo_label: 'Sample data',
          }}
        >
          <DemoReadProbe />
        </Auth0RuntimeProvider>
      </MemoryRouter>
    );

    expect(await screen.findByText('loaded')).toBeInTheDocument();
    expect(auth0State.loginWithRedirect).not.toHaveBeenCalled();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(
      (fetchMock.mock.calls[0]?.[1] as RequestInit | undefined)?.headers
    ).not.toMatchObject({
      Authorization: expect.anything(),
    });
  });

  it('lets local self-hosted consoles continue with a project API key when Auth0 is disabled', async () => {
    const user = userEvent.setup();
    fetchMock.mockResolvedValue(
      new Response(
        JSON.stringify({
          code: 'missing_credentials',
          message: 'Authentication required',
        }),
        {
          status: 401,
          headers: {
            'Content-Type': 'application/json',
          },
        }
      )
    );

    render(
      <MemoryRouter initialEntries={['/traces']}>
        <Routes>
          <Route
            element={
              <ProtectedRoute
                auth={{
                  status: 'ready',
                  enabled: false,
                }}
              />
            }
          >
            <Route path="/traces" element={<div>Local traces</div>} />
          </Route>
        </Routes>
      </MemoryRouter>
    );

    expect(await screen.findByText('Enter a local project API key.')).toBeInTheDocument();

    await user.type(screen.getByLabelText('Project API key'), 'pk_runtime_key');
    await user.click(screen.getByRole('button', { name: 'Open local console' }));

    expect(screen.getByText('Local traces')).toBeInTheDocument();
    expect(window.localStorage.getItem('continua_api_key')).toBe('pk_runtime_key');
  });

  it('renders the console without an API key prompt when local mode is enabled', async () => {
    fetchMock.mockResolvedValue(
      new Response(
        JSON.stringify({
          projects: [
            {
              id: PROJECT_ID,
              name: 'Local project',
              created_at: '2026-08-15T10:00:00Z',
              updated_at: '2026-08-15T10:00:00Z',
            },
          ],
        }),
        {
          status: 200,
          headers: {
            'Content-Type': 'application/json',
          },
        }
      )
    );

    render(
      <MemoryRouter initialEntries={['/traces']}>
        <Routes>
          <Route
            element={
              <ProtectedRoute
                auth={{
                  status: 'ready',
                  enabled: false,
                  local_mode_enabled: true,
                }}
              />
            }
          >
            <Route path="/traces" element={<div>Local traces</div>} />
            <Route path="/projects" element={<div>Create first project</div>} />
          </Route>
        </Routes>
      </MemoryRouter>
    );

    expect(await screen.findByText('Local traces')).toBeInTheDocument();
    expect(
      screen.queryByText('Enter a local project API key.')
    ).not.toBeInTheDocument();
  });

  it('routes local first-run consoles to project creation when no projects exist', async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ projects: [] }), {
        status: 200,
        headers: {
          'Content-Type': 'application/json',
        },
      })
    );

    render(
      <MemoryRouter initialEntries={['/traces']}>
        <Routes>
          <Route
            element={
              <ProtectedRoute
                auth={{
                  status: 'ready',
                  enabled: false,
                }}
              />
            }
          >
            <Route path="/traces" element={<div>Local traces</div>} />
            <Route path="/projects" element={<div>Create first project</div>} />
          </Route>
        </Routes>
      </MemoryRouter>
    );

    expect(await screen.findByText('Create first project')).toBeInTheDocument();
  });

  it('redirects signed-out operators to Auth0 and preserves the current returnTo URL', async () => {
    auth0State.isAuthenticated = false;

    render(
      <MemoryRouter initialEntries={[`/traces?project_id=${PROJECT_ID}`]}>
        <Routes>
          <Route
            element={
              <ProtectedRoute
                auth={{
                  status: 'ready',
                  enabled: true,
                  domain: 'continua.us.auth0.com',
                  client_id: 'client-id',
                  audience: 'https://continua/api',
                }}
              />
            }
          >
            <Route path="/traces" element={<div>Protected traces</div>} />
          </Route>
        </Routes>
      </MemoryRouter>
    );

    expect(screen.getByText('Redirecting to sign in...')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Sign in again' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Return home' })).toHaveAttribute(
      'href',
      '/'
    );
    await waitFor(() => {
      expect(auth0State.loginWithRedirect).toHaveBeenCalledWith({
        appState: {
          returnTo: `/traces?project_id=${PROJECT_ID}`,
        },
      });
    });
  });

  it('shows a retry action when runtime auth bootstrap fails', () => {
    render(
      <MemoryRouter initialEntries={['/dashboard']}>
        <Routes>
          <Route
            element={
              <ProtectedRoute
                auth={{
                  status: 'error',
                  enabled: false,
                  error: 'Config fetch failed',
                }}
              />
            }
          >
            <Route path="/dashboard" element={<div>Protected dashboard</div>} />
          </Route>
        </Routes>
      </MemoryRouter>
    );

    expect(screen.getByText('Authentication setup failed')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Return home' })).toHaveAttribute(
      'href',
      '/'
    );
  });

  it('tells the API client whether an interactive sign-in provider is configured', async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ traces: [], total: 0 }), {
        status: 200,
        headers: {
          'Content-Type': 'application/json',
        },
      })
    );

    // Authenticated bridge: Auth0 is configured, so sign-in wording applies.
    const auth0Deployment = render(
      <MemoryRouter>
        <Auth0RuntimeProvider
          auth={{
            status: 'ready',
            enabled: true,
            domain: 'continua.us.auth0.com',
            client_id: 'client-id',
            audience: 'https://continua/api',
          }}
        >
          <div>Operator console</div>
        </Auth0RuntimeProvider>
      </MemoryRouter>
    );

    expect(await screen.findByText('Operator console')).toBeInTheDocument();
    expect(isAuthProviderEnabled()).toBe(true);
    auth0Deployment.unmount();

    // Unauthenticated bridge: no Auth0, so the client must fall back to
    // project-API-key wording. Seed the opposite value first so the assertion
    // cannot pass on the module default alone.
    setAuthProviderEnabled(true);
    const localDeployment = render(
      <MemoryRouter>
        <Auth0RuntimeProvider
          auth={{
            status: 'ready',
            enabled: false,
            local_mode_enabled: true,
          }}
        >
          <div>Local console</div>
        </Auth0RuntimeProvider>
      </MemoryRouter>
    );

    expect(await screen.findByText('Local console')).toBeInTheDocument();
    expect(isAuthProviderEnabled()).toBe(false);
    localDeployment.unmount();

    // Public-demo bridge with Auth0 still configured: the demo reads are
    // unauthenticated, but a sign-in provider exists, so the wording stays
    // sign-in oriented.
    setAuthProviderEnabled(false);
    render(
      <MemoryRouter>
        <Auth0RuntimeProvider
          auth={{
            status: 'ready',
            enabled: true,
            domain: 'continua.us.auth0.com',
            client_id: 'client-id',
            audience: 'https://continua/api',
            public_demo_enabled: true,
            public_demo_label: 'Sample data',
          }}
        >
          <div>Demo console</div>
        </Auth0RuntimeProvider>
      </MemoryRouter>
    );

    expect(await screen.findByText('Demo console')).toBeInTheDocument();
    expect(isAuthProviderEnabled()).toBe(true);
  });
});
