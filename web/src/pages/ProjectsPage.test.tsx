import type { ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { clearApiKey, setApiKey } from '../api/client';
import { ProjectsPage } from './ProjectsPage';
import { jsonResponse, mockClipboard, type RequestInput } from './testUtils';

vi.mock('@auth0/auth0-react', () => ({
  Auth0Provider: ({ children }: { children: ReactNode }) => children,
  useAuth0: () => ({
    getAccessTokenSilently: vi.fn(async () => 'test-key'),
    isAuthenticated: true,
    isLoading: false,
    user: { email: 'test@example.com' },
  }),
}));

const DEFAULT_PROJECT = {
  id: '00000000-0000-0000-0000-000000000001',
  name: 'Default Project',
  created_at: '2026-05-01T00:00:00.000Z',
  updated_at: '2026-05-01T00:00:00.000Z',
};

function renderProjectsPage() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, gcTime: Infinity } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter>
        <ProjectsPage />
      </MemoryRouter>
    </QueryClientProvider>
  );
}

beforeEach(() => {
  setApiKey('test-key');
});

afterEach(() => {
  clearApiKey();
  vi.restoreAllMocks();
});

describe('ProjectsPage', () => {
  it('lists projects from the API', async () => {
    const fetchMock = vi.fn(async (input: RequestInput) => {
      const url = typeof input === 'string' ? input : input.toString();
      if (url.includes('/api/projects')) {
        return jsonResponse({
          projects: [
            DEFAULT_PROJECT,
            {
              id: 'other-id',
              name: 'chatbot',
              created_at: '2026-05-02T00:00:00.000Z',
              updated_at: '2026-05-02T00:00:00.000Z',
            },
          ],
        });
      }
      throw new Error(`unexpected ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    renderProjectsPage();

    expect(await screen.findByText('Default Project')).toBeInTheDocument();
    expect(screen.getByText('chatbot')).toBeInTheDocument();
  });

  it('creates a project and reveals the api_key once', async () => {
    const user = userEvent.setup();
    mockClipboard();

    let listCalls = 0;
    const fetchMock = vi.fn(async (input: RequestInput, init?: RequestInit) => {
      const url = typeof input === 'string' ? input : input.toString();
      if (url.includes('/api/projects') && (!init?.method || init.method === 'GET')) {
        listCalls += 1;
        if (listCalls === 1) {
          return jsonResponse({ projects: [DEFAULT_PROJECT] });
        }
        return jsonResponse({
          projects: [
            DEFAULT_PROJECT,
            {
              id: 'new-id',
              name: 'new-bot',
              created_at: '2026-05-03T00:00:00.000Z',
              updated_at: '2026-05-03T00:00:00.000Z',
            },
          ],
        });
      }
      if (url.includes('/api/projects') && init?.method === 'POST') {
        return jsonResponse(
          {
            id: 'new-id',
            name: 'new-bot',
            created_at: '2026-05-03T00:00:00.000Z',
            updated_at: '2026-05-03T00:00:00.000Z',
            api_key: 'pk_revealedkey123',
          },
          201
        );
      }
      throw new Error(`unexpected ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    renderProjectsPage();

    await screen.findByText('Default Project');

    await user.click(screen.getAllByRole('button', { name: /create project/i })[0]);
    const createDialog = await screen.findByRole('dialog', { name: /create project/i });
    await user.type(within(createDialog).getByLabelText(/project name/i), 'new-bot');
    await user.click(within(createDialog).getByRole('button', { name: /^create project$/i }));

    expect(await screen.findByTestId('revealed-api-key')).toHaveTextContent(
      'pk_revealedkey123'
    );
  });

  it('disables Delete on the seeded default project', async () => {
    const fetchMock = vi.fn(async (input: RequestInput) => {
      const url = typeof input === 'string' ? input : input.toString();
      if (url.includes('/api/projects')) {
        return jsonResponse({ projects: [DEFAULT_PROJECT] });
      }
      throw new Error(`unexpected ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    renderProjectsPage();

    await screen.findByText('Default Project');
    const row = screen.getByTestId(`project-row-${DEFAULT_PROJECT.id}`);
    const deleteBtn = within(row).getByRole('button', { name: /delete/i });
    expect(deleteBtn).toBeDisabled();
  });

  it('renames a project via the rename dialog', async () => {
    const user = userEvent.setup();

    const target = {
      id: 'rename-id',
      name: 'old-name',
      created_at: '2026-05-04T00:00:00.000Z',
      updated_at: '2026-05-04T00:00:00.000Z',
    };

    let listCalls = 0;
    const fetchMock = vi.fn(async (input: RequestInput, init?: RequestInit) => {
      const url = typeof input === 'string' ? input : input.toString();
      if (url.includes('/api/projects') && (!init?.method || init.method === 'GET')) {
        listCalls += 1;
        if (listCalls === 1) {
          return jsonResponse({ projects: [DEFAULT_PROJECT, target] });
        }
        return jsonResponse({
          projects: [DEFAULT_PROJECT, { ...target, name: 'new-name' }],
        });
      }
      if (url.includes(`/api/projects/${target.id}`) && init?.method === 'PATCH') {
        return jsonResponse({ ...target, name: 'new-name' });
      }
      throw new Error(`unexpected ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    renderProjectsPage();

    await screen.findByText('old-name');
    const row = screen.getByTestId(`project-row-${target.id}`);
    await user.click(within(row).getByRole('button', { name: /rename/i }));

    const renameDialog = await screen.findByRole('dialog', { name: /rename "old-name"/i });
    const input = within(renameDialog).getByLabelText(/new name/i);
    await user.clear(input);
    await user.type(input, 'new-name');
    await user.click(within(renameDialog).getByRole('button', { name: /save/i }));

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).toBeNull();
    });
    expect(await screen.findByText('new-name')).toBeInTheDocument();
  });

  it('deletes a project after typing its name to confirm', async () => {
    const user = userEvent.setup();

    const target = {
      id: 'delete-id',
      name: 'doomed-bot',
      created_at: '2026-05-05T00:00:00.000Z',
      updated_at: '2026-05-05T00:00:00.000Z',
    };

    let listCalls = 0;
    const deleteCalls: string[] = [];
    const fetchMock = vi.fn(async (input: RequestInput, init?: RequestInit) => {
      const url = typeof input === 'string' ? input : input.toString();
      if (url.includes('/api/projects') && (!init?.method || init.method === 'GET')) {
        listCalls += 1;
        if (listCalls === 1) {
          return jsonResponse({ projects: [DEFAULT_PROJECT, target] });
        }
        return jsonResponse({ projects: [DEFAULT_PROJECT] });
      }
      if (url.includes(`/api/projects/${target.id}`) && init?.method === 'DELETE') {
        deleteCalls.push(url);
        return new Response(null, { status: 204 });
      }
      throw new Error(`unexpected ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    renderProjectsPage();

    await screen.findByText('doomed-bot');
    const row = screen.getByTestId(`project-row-${target.id}`);
    await user.click(within(row).getByRole('button', { name: /delete/i }));

    const deleteDialog = await screen.findByRole('dialog', { name: /delete "doomed-bot"/i });
    const confirmBtn = within(deleteDialog).getByRole('button', { name: /delete project/i });
    expect(confirmBtn).toBeDisabled();

    await user.type(within(deleteDialog).getByLabelText(/type the project name/i), 'doomed-bot');
    expect(confirmBtn).toBeEnabled();
    await user.click(confirmBtn);

    await waitFor(() => {
      expect(screen.queryByRole('dialog')).toBeNull();
    });
    expect(deleteCalls.length).toBe(1);
    await waitFor(() => {
      expect(screen.queryByText('doomed-bot')).toBeNull();
    });
  });

  it('clears the revealed key when the reveal dialog is closed', async () => {
    const user = userEvent.setup();
    mockClipboard();

    const fetchMock = vi.fn(async (input: RequestInput, init?: RequestInit) => {
      const url = typeof input === 'string' ? input : input.toString();
      if (url.includes('/api/projects') && (!init?.method || init.method === 'GET')) {
        return jsonResponse({ projects: [DEFAULT_PROJECT] });
      }
      if (url.includes('/api/projects') && init?.method === 'POST') {
        return jsonResponse(
          {
            id: 'new-id',
            name: 'closer-bot',
            created_at: '2026-05-06T00:00:00.000Z',
            updated_at: '2026-05-06T00:00:00.000Z',
            api_key: 'pk_sensitive_value',
          },
          201
        );
      }
      throw new Error(`unexpected ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    renderProjectsPage();

    await screen.findByText('Default Project');
    await user.click(screen.getAllByRole('button', { name: /create project/i })[0]);
    const createDialog = await screen.findByRole('dialog', { name: /create project/i });
    await user.type(within(createDialog).getByLabelText(/project name/i), 'closer-bot');
    await user.click(within(createDialog).getByRole('button', { name: /^create project$/i }));

    const reveal = await screen.findByTestId('revealed-api-key');
    expect(reveal).toHaveTextContent('pk_sensitive_value');

    const revealDialog = screen.getByRole('dialog', { name: /api key for/i });
    await user.click(within(revealDialog).getByRole('button', { name: /done/i }));

    await waitFor(() => {
      expect(screen.queryByTestId('revealed-api-key')).toBeNull();
    });
  });

  it('rotates the key and reveals the new plaintext value', async () => {
    const user = userEvent.setup();
    mockClipboard();

    const target = {
      id: 'rotate-id',
      name: 'rotate-bot',
      created_at: '2026-05-04T00:00:00.000Z',
      updated_at: '2026-05-04T00:00:00.000Z',
    };

    const fetchMock = vi.fn(async (input: RequestInput, init?: RequestInit) => {
      const url = typeof input === 'string' ? input : input.toString();
      if (url.endsWith('/api/projects') || url.includes('/api/projects?')) {
        return jsonResponse({ projects: [DEFAULT_PROJECT, target] });
      }
      if (url.includes('/rotate') && init?.method === 'POST') {
        return jsonResponse({
          ...target,
          api_key: 'pk_newkey789',
        });
      }
      throw new Error(`unexpected ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    renderProjectsPage();

    await screen.findByText('rotate-bot');
    const row = screen.getByTestId(`project-row-${target.id}`);
    await user.click(within(row).getByRole('button', { name: /rotate key/i }));

    const rotateDialog = await screen.findByRole('dialog', { name: /rotate key for/i });
    await user.click(within(rotateDialog).getByRole('button', { name: /^rotate key$/i }));

    expect(await screen.findByTestId('revealed-api-key')).toHaveTextContent(
      'pk_newkey789'
    );
  });
});
