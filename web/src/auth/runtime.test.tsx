import { useEffect, useState } from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  clearApiKey,
  fetchAPI,
  isLocalSingleUserMode,
  setAccessTokenProvider,
  setLocalSingleUserMode,
  setPublicDemoMode,
} from '../api/client';
import {
  ConsoleRoute,
  ProtectedRoute,
  RuntimeAuthProvider,
  useRuntimeAuthState,
} from './runtime';

const PROJECT_ID = '11111111-1111-1111-1111-111111111111';
const RUN_LOCALLY_DOCS_URL =
  'https://www.continua.in/docs/guides/installation';

function RuntimeAuthProbe() {
  const auth = useRuntimeAuthState();

  return (
    <div>
      <div data-testid="auth-status">{auth.status}</div>
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
});

afterEach(() => {
  clearApiKey();
  setAccessTokenProvider(null);
  setPublicDemoMode(false);
  setLocalSingleUserMode(false);
  vi.unstubAllGlobals();
});

describe('runtime auth', () => {
  it('loads the runtime config from /api/auth/config', async () => {
    fetchMock.mockResolvedValue(
      new Response(
        JSON.stringify({
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
    expect(screen.getByTestId('auth-public-demo')).toHaveTextContent('false');
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

  it('allows public demo console routes without an access gate', () => {
    render(
      <MemoryRouter initialEntries={['/traces']}>
        <Routes>
          <Route
            element={
              <ConsoleRoute
                auth={{
                  status: 'ready',
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
  });

  it('sends public demo reads without an Authorization header', async () => {
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
        <RuntimeAuthProvider
          auth={{
            status: 'ready',
            public_demo_enabled: true,
            public_demo_label: 'Sample data',
          }}
        >
          <DemoReadProbe />
        </RuntimeAuthProvider>
      </MemoryRouter>
    );

    expect(await screen.findByText('loaded')).toBeInTheDocument();
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(
      (fetchMock.mock.calls[0]?.[1] as RequestInit | undefined)?.headers
    ).not.toMatchObject({
      Authorization: expect.anything(),
    });
  });

  it('lets local self-hosted consoles continue with a project API key', async () => {
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

  it('shows a retry action when runtime auth bootstrap fails', () => {
    render(
      <MemoryRouter initialEntries={['/dashboard']}>
        <Routes>
          <Route
            element={
              <ProtectedRoute
                auth={{
                  status: 'error',
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

  it('puts the API client in local single-user mode only while the server offers it', () => {
    const localDeployment = render(
      <MemoryRouter>
        <RuntimeAuthProvider auth={{ status: 'ready', local_mode_enabled: true }}>
          <div>Local console</div>
        </RuntimeAuthProvider>
      </MemoryRouter>
    );

    expect(screen.getByText('Local console')).toBeInTheDocument();
    expect(isLocalSingleUserMode()).toBe(true);
    localDeployment.unmount();
    expect(isLocalSingleUserMode()).toBe(false);

    // The public demo always wins over local mode.
    render(
      <MemoryRouter>
        <RuntimeAuthProvider
          auth={{ status: 'ready', local_mode_enabled: true, public_demo_enabled: true }}
        >
          <div>Demo console</div>
        </RuntimeAuthProvider>
      </MemoryRouter>
    );

    expect(screen.getByText('Demo console')).toBeInTheDocument();
    expect(isLocalSingleUserMode()).toBe(false);
  });
});
