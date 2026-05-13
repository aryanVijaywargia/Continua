import type { ReactNode } from 'react';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { clearApiKey } from '../api/client';
import { RuntimeAuthStateProvider } from '../auth/runtime';
import { ThemeProvider } from '../hooks/ThemeProvider';
import { renderTraceRoutes } from './testUtils';
import { SettingsPage } from './SettingsPage';

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

beforeEach(() => {
  clearApiKey();
  auth0State.logout.mockReset();
  auth0State.user = {
    email: 'operator@continua.dev',
    name: 'Continua Operator',
    sub: 'google-oauth2|operator',
  };
});

afterEach(() => {
  clearApiKey();
  vi.restoreAllMocks();
});

describe('SettingsPage', () => {
  it('shows the operator identity, theme controls, and sign-out action', async () => {
    const user = userEvent.setup();

    renderTraceRoutes(['/settings']);

    expect(
      await screen.findByRole('heading', {
        name: 'Settings',
      })
    ).toBeInTheDocument();
    expect(
      screen.getByText('Auth is handled through Auth0. Theme controls and sign-out stay here.')
    ).toBeInTheDocument();
    expect(screen.getByText('operator@continua.dev')).toBeInTheDocument();
    expect(screen.getByText('Continua Operator')).toBeInTheDocument();
    expect(screen.getByText('google-oauth2|operator')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^System\b/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^Light\b/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^Dark\b/ })).toBeInTheDocument();
    expect(screen.queryByLabelText('New API key')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Save key' })).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Sign out' }));

    expect(auth0State.logout).toHaveBeenCalledWith({
      logoutParams: {
        returnTo: window.location.origin,
      },
    });
  });

  it('redirects the settings route back to the dashboard in public demo mode', () => {
    render(
      <ThemeProvider>
        <RuntimeAuthStateProvider
          auth={{
            status: 'ready',
            enabled: false,
            public_demo_enabled: true,
            public_demo_label: 'Sample data',
          }}
        >
          <MemoryRouter initialEntries={['/settings']}>
            <Routes>
              <Route path="/settings" element={<SettingsPage />} />
              <Route path="/dashboard" element={<div>Demo dashboard</div>} />
            </Routes>
          </MemoryRouter>
        </RuntimeAuthStateProvider>
      </ThemeProvider>
    );

    expect(screen.getByText('Demo dashboard')).toBeInTheDocument();
  });

  it('lets local self-hosted users save and clear the project API key', async () => {
    const user = userEvent.setup();

    render(
      <ThemeProvider>
        <RuntimeAuthStateProvider
          auth={{
            status: 'ready',
            enabled: false,
          }}
        >
          <MemoryRouter initialEntries={['/settings']}>
            <SettingsPage />
          </MemoryRouter>
        </RuntimeAuthStateProvider>
      </ThemeProvider>
    );

    expect(screen.getByRole('heading', { name: 'Local API key' })).toBeInTheDocument();
    expect(screen.getByText('No key saved')).toBeInTheDocument();

    await user.type(screen.getByLabelText('Project API key'), 'default');
    await user.click(screen.getByRole('button', { name: 'Save local key' }));

    expect(window.localStorage.getItem('continua_api_key')).toBe('default');
    expect(screen.getByText('Saved in this browser')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Clear local key' }));

    expect(window.localStorage.getItem('continua_api_key')).toBeNull();
    expect(screen.getByText('No key saved')).toBeInTheDocument();
  });
});
