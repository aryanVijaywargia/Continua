import { act, cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { clearApiKey, setApiKey } from '../api/client';
import { ThemeProvider } from '../hooks/ThemeProvider';
import { AppShell } from './AppShell';

async function renderShell(initialEntry = '/') {
  return render(
    <ThemeProvider>
      <MemoryRouter initialEntries={[initialEntry]}>
        <Routes>
          <Route element={<AppShell />}>
            <Route path="/" element={<div>Overview content</div>} />
            <Route path="/traces" element={<div>Trace list content</div>} />
            <Route path="/sessions" element={<div>Session list content</div>} />
            <Route path="/settings" element={<div>Settings content</div>} />
          </Route>
        </Routes>
      </MemoryRouter>
    </ThemeProvider>
  );
}

beforeEach(() => {
  localStorage.clear();
  localStorage.setItem('continua_api_key', 'test-key');
});

afterEach(() => {
  cleanup();
  localStorage.clear();
});

describe('AppShell', () => {
  it('renders the operator shell, primary navigation, and active route content', async () => {
    await renderShell('/sessions');

    const primaryNav = screen.getByRole('navigation', { name: 'Primary' });
    expect(screen.getByText('Operator Console')).toBeInTheDocument();
    expect(within(primaryNav).getByText('Overview').closest('a')).toHaveAttribute('href', '/');
    expect(within(primaryNav).getByText('Traces').closest('a')).toHaveAttribute('href', '/traces');
    expect(within(primaryNav).getByText('Sessions').closest('a')).toHaveAttribute(
      'aria-current',
      'page'
    );
    expect(screen.getByText('Session list content')).toBeInTheDocument();
    expect(screen.getByText('API key connected')).toBeInTheDocument();
  });

  it('updates the API key indicator when local auth state changes', async () => {
    await renderShell('/');
    expect(screen.getByText('API key connected')).toBeInTheDocument();

    act(() => {
      clearApiKey();
    });
    await waitFor(() => {
      expect(screen.getByText('API key required')).toBeInTheDocument();
    });

    act(() => {
      setApiKey('restored-key');
    });
    await waitFor(() => {
      expect(screen.getByText('API key connected')).toBeInTheDocument();
    });
  });

  it('opens the command palette from the shell control', async () => {
    const user = userEvent.setup();
    await renderShell('/');

    await user.click(screen.getByRole('button', { name: /Command Palette/i }));
    expect(screen.getByRole('combobox', { name: 'Search commands' })).toBeInTheDocument();
  });
});
