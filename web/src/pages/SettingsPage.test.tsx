import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { clearApiKey } from '../api/client';
import { RuntimeAuthStateProvider } from '../auth/runtime';
import { ThemeProvider } from '../hooks/ThemeProvider';
import { buildProjectPath } from '../utils/projectSearchParams';
import { renderTraceRoutes } from './testUtils';
import { SettingsPage } from './SettingsPage';

beforeEach(() => {
  clearApiKey();
});

afterEach(() => {
  clearApiKey();
  vi.restoreAllMocks();
});

describe('SettingsPage', () => {
  it('shows the local API-key settings and theme controls', async () => {
    renderTraceRoutes(['/settings']);

    expect(
      await screen.findByRole('heading', {
        name: 'Settings',
      })
    ).toBeInTheDocument();
    expect(
      screen.getByText(
        'Local self-host access uses a project API key. Theme controls stay here.'
      )
    ).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Local API key' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^System\b/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^Light\b/ })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /^Dark\b/ })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Sign out' })).not.toBeInTheDocument();
  });

  it('redirects the settings route back to the dashboard in public demo mode', () => {
    render(
      <ThemeProvider>
        <RuntimeAuthStateProvider
          auth={{
            status: 'ready',
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

  it('links to engine projection repair while preserving project scope', () => {
    const projectId = '123e4567-e89b-12d3-a456-426614174000';

    render(
      <ThemeProvider>
        <RuntimeAuthStateProvider
          auth={{
            status: 'ready',
          }}
        >
          <MemoryRouter initialEntries={[`/settings?project_id=${projectId}`]}>
            <Routes>
              <Route path="/settings" element={<SettingsPage />} />
            </Routes>
          </MemoryRouter>
        </RuntimeAuthStateProvider>
      </ThemeProvider>
    );

    const link = screen
      .getByText('Engine projection repair')
      .closest('a');

    expect(link).toHaveAttribute(
      'href',
      buildProjectPath('/tools/engine-projections', projectId)
    );
  });

  it('lets local self-hosted users save and clear the project API key', async () => {
    const user = userEvent.setup();

    render(
      <ThemeProvider>
        <RuntimeAuthStateProvider
          auth={{
            status: 'ready',
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

    await user.type(screen.getByLabelText('Project API key'), 'pk_settings_key');
    await user.click(screen.getByRole('button', { name: 'Save local key' }));

    expect(window.localStorage.getItem('continua_api_key')).toBe('pk_settings_key');
    expect(screen.getByText('Saved in this browser')).toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: 'Clear local key' }));

    expect(window.localStorage.getItem('continua_api_key')).toBeNull();
    expect(screen.getByText('No key saved')).toBeInTheDocument();
  });
});
