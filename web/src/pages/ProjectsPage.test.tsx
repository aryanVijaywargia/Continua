import type { ReactNode } from 'react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  clearApiKey,
  rememberProjectApiKey,
  setApiKey,
  setAuthProviderEnabled,
} from '../api/client';
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

const EXISTING_PROJECT = {
  id: '00000000-0000-0000-0000-000000000001',
  name: 'Existing Project',
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
  window.localStorage.clear();
  setAuthProviderEnabled(false);
  setApiKey('test-key');
});

afterEach(() => {
  clearApiKey();
  setAuthProviderEnabled(false);
  vi.restoreAllMocks();
});

describe('ProjectsPage', () => {
  it('lists projects from the API', async () => {
    const fetchMock = vi.fn(async (input: RequestInput) => {
      const url = typeof input === 'string' ? input : input.toString();
      if (url.includes('/api/projects')) {
        return jsonResponse({
          projects: [
            EXISTING_PROJECT,
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

    expect(await screen.findByText('Existing Project')).toBeInTheDocument();
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
          return jsonResponse({ projects: [EXISTING_PROJECT] });
        }
        return jsonResponse({
          projects: [
            EXISTING_PROJECT,
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

    await screen.findByText('Existing Project');

    await user.click(screen.getAllByRole('button', { name: /create project/i })[0]);
    const createDialog = await screen.findByRole('dialog', { name: /create project/i });
    await user.type(within(createDialog).getByLabelText(/project name/i), 'new-bot');
    await user.click(within(createDialog).getByRole('button', { name: /^create project$/i }));

    expect(await screen.findByTestId('revealed-api-key')).toHaveTextContent(
      'pk_revealedkey123'
    );
  });

  it('stores the first project key for local first-run onboarding', async () => {
    const user = userEvent.setup();
    mockClipboard();
    clearApiKey();

    let listCalls = 0;
    const fetchMock = vi.fn(async (input: RequestInput, init?: RequestInit) => {
      const url = typeof input === 'string' ? input : input.toString();
      if (url.includes('/api/projects') && (!init?.method || init.method === 'GET')) {
        listCalls += 1;
        if (listCalls === 1) {
          return jsonResponse({ projects: [] });
        }
        return jsonResponse({
          projects: [
            {
              id: 'first-id',
              name: 'first-project',
              created_at: '2026-05-03T00:00:00.000Z',
              updated_at: '2026-05-03T00:00:00.000Z',
            },
          ],
        });
      }
      if (url.includes('/api/projects') && init?.method === 'POST') {
        return jsonResponse(
          {
            id: 'first-id',
            name: 'first-project',
            created_at: '2026-05-03T00:00:00.000Z',
            updated_at: '2026-05-03T00:00:00.000Z',
            api_key: 'pk_first_project_key',
          },
          201
        );
      }
      throw new Error(`unexpected ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    renderProjectsPage();

    expect(await screen.findByTestId('projects-first-run-hero')).toBeInTheDocument();
    await user.click(screen.getAllByRole('button', { name: /create project/i })[0]);
    const createDialog = await screen.findByRole('dialog', { name: /create project/i });
    await user.type(within(createDialog).getByLabelText(/project name/i), 'first-project');
    await user.click(within(createDialog).getByRole('button', { name: /^create project$/i }));

    expect(await screen.findByTestId('revealed-api-key')).toHaveTextContent(
      'pk_first_project_key'
    );
    expect(window.localStorage.getItem('continua_api_key')).toBeNull();

    const revealDialog = screen.getByRole('dialog', { name: /api key for/i });
    await user.click(within(revealDialog).getByRole('button', { name: /done/i }));

    expect(window.localStorage.getItem('continua_api_key')).toBe(
      'pk_first_project_key'
    );
  });

  it('shows the first-run welcome hero when no key and no projects exist', async () => {
    clearApiKey();
    const fetchMock = vi.fn(async (input: RequestInput) => {
      const url = typeof input === 'string' ? input : input.toString();
      if (url.includes('/api/projects')) {
        return jsonResponse({ projects: [] });
      }
      throw new Error(`unexpected ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    renderProjectsPage();

    expect(await screen.findByTestId('projects-first-run-hero')).toBeInTheDocument();
    expect(
      screen.getByText(/create your first project to mint an api key/i)
    ).toBeInTheDocument();
    expect(screen.queryByText(/no projects yet/i)).toBeNull();
  });

  it('falls back to the terse empty state when a key exists but projects are empty', async () => {
    setApiKey('pk_returning_operator');
    const fetchMock = vi.fn(async (input: RequestInput) => {
      const url = typeof input === 'string' ? input : input.toString();
      if (url.includes('/api/projects')) {
        return jsonResponse({ projects: [] });
      }
      throw new Error(`unexpected ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    renderProjectsPage();

    expect(await screen.findByText(/no projects yet/i)).toBeInTheDocument();
    expect(screen.queryByTestId('projects-first-run-hero')).toBeNull();
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
          return jsonResponse({ projects: [EXISTING_PROJECT, target] });
        }
        return jsonResponse({
          projects: [EXISTING_PROJECT, { ...target, name: 'new-name' }],
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
          return jsonResponse({ projects: [EXISTING_PROJECT, target] });
        }
        return jsonResponse({ projects: [EXISTING_PROJECT] });
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

  it('switches to another known local key after deleting the current-key project', async () => {
    const user = userEvent.setup();
    const currentKeyProject = {
      id: 'same-name-a',
      name: 'same-name',
      created_at: '2026-05-05T00:00:00.000Z',
      updated_at: '2026-05-05T00:00:00.000Z',
    };
    const fallbackProject = {
      id: 'same-name-b',
      name: 'same-name',
      created_at: '2026-05-06T00:00:00.000Z',
      updated_at: '2026-05-06T00:00:00.000Z',
    };

    setApiKey('pk_current_project');
    rememberProjectApiKey(fallbackProject.id, 'pk_fallback_project');

    let listCalls = 0;
    const fetchMock = vi.fn(async (input: RequestInput, init?: RequestInit) => {
      const url = typeof input === 'string' ? input : input.toString();
      if (url.includes('/api/projects') && (!init?.method || init.method === 'GET')) {
        listCalls += 1;
        return jsonResponse({
          authenticated_project_id: currentKeyProject.id,
          projects:
            listCalls === 1
              ? [currentKeyProject, fallbackProject]
              : [fallbackProject],
        });
      }
      if (
        url.includes(`/api/projects/${currentKeyProject.id}`) &&
        init?.method === 'DELETE'
      ) {
        return new Response(null, { status: 204 });
      }
      throw new Error(`unexpected ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    renderProjectsPage();

    await screen.findAllByText('same-name');
    const row = screen.getByTestId(`project-row-${currentKeyProject.id}`);
    await user.click(within(row).getByRole('button', { name: /delete/i }));
    const deleteDialog = await screen.findByRole('dialog', { name: /delete "same-name"/i });
    await user.type(
      within(deleteDialog).getByLabelText(/type the project name/i),
      'same-name'
    );
    await user.click(within(deleteDialog).getByRole('button', { name: /delete project/i }));

    await waitFor(() => {
      expect(window.localStorage.getItem('continua_api_key')).toBe(
        'pk_fallback_project'
      );
    });
    expect(screen.getByTestId(`project-row-${fallbackProject.id}`)).toBeInTheDocument();
  });

  it('retries delete with a remembered fallback key after a stale-key 401', async () => {
    const user = userEvent.setup();
    const target = {
      id: 'stale-key-target',
      name: 'stale-key-target',
      created_at: '2026-05-05T00:00:00.000Z',
      updated_at: '2026-05-05T00:00:00.000Z',
    };
    const fallbackProject = {
      id: 'fallback-id',
      name: 'fallback-project',
      created_at: '2026-05-06T00:00:00.000Z',
      updated_at: '2026-05-06T00:00:00.000Z',
    };

    setApiKey('pk_stale_project');
    rememberProjectApiKey(fallbackProject.id, 'pk_fallback_project');

    let deleteCalls = 0;
    let listCalls = 0;
    const fetchMock = vi.fn(async (input: RequestInput, init?: RequestInit) => {
      const url = typeof input === 'string' ? input : input.toString();
      if (url.includes('/api/projects') && (!init?.method || init.method === 'GET')) {
        listCalls += 1;
        return jsonResponse({
          projects:
            listCalls === 1
              ? [target, fallbackProject]
              : [fallbackProject],
        });
      }
      if (url.includes(`/api/projects/${target.id}`) && init?.method === 'DELETE') {
        deleteCalls += 1;
        if (deleteCalls === 1) {
          return jsonResponse(
            { code: 'invalid_api_key', message: 'Invalid API key' },
            401
          );
        }
        expect((init?.headers as Record<string, string>).Authorization).toBe(
          'Bearer pk_fallback_project'
        );
        return new Response(null, { status: 204 });
      }
      throw new Error(`unexpected ${url}`);
    });
    vi.stubGlobal('fetch', fetchMock);

    renderProjectsPage();

    const row = await screen.findByTestId(`project-row-${target.id}`);
    await user.click(within(row).getByRole('button', { name: /delete/i }));
    const deleteDialog = await screen.findByRole('dialog', {
      name: /delete "stale-key-target"/i,
    });
    await user.type(
      within(deleteDialog).getByLabelText(/type the project name/i),
      target.name
    );
    await user.click(within(deleteDialog).getByRole('button', { name: /delete project/i }));

    await waitFor(() => {
      expect(deleteCalls).toBe(2);
      expect(screen.queryByRole('dialog')).toBeNull();
    });
    expect(window.localStorage.getItem('continua_api_key')).toBe(
      'pk_fallback_project'
    );
  });

  it('clears the revealed key when the reveal dialog is closed', async () => {
    const user = userEvent.setup();
    mockClipboard();

    const fetchMock = vi.fn(async (input: RequestInput, init?: RequestInit) => {
      const url = typeof input === 'string' ? input : input.toString();
      if (url.includes('/api/projects') && (!init?.method || init.method === 'GET')) {
        return jsonResponse({ projects: [EXISTING_PROJECT] });
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

    await screen.findByText('Existing Project');
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
        return jsonResponse({ projects: [EXISTING_PROJECT, target] });
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

    const revealDialog = screen.getByRole('dialog', { name: /api key for/i });
    await user.click(within(revealDialog).getByRole('button', { name: /done/i }));

    expect(window.localStorage.getItem('continua_api_key')).toBe('pk_newkey789');
  });

  it('explains a failed rotate as a missing project API key in local mode', async () => {
    const user = userEvent.setup();
    clearApiKey();
    setAuthProviderEnabled(false);

    const fetchMock = mockProjectListOnly();
    vi.stubGlobal('fetch', fetchMock);

    renderProjectsPage();

    await openRotateDialogAndSubmit(user);

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent(/project api key/i);
    expect(screen.queryByText(/sign in required/i)).toBeNull();
    expect(mutationRequests(fetchMock)).toEqual([]);
  });

  it('keeps sign-in wording for a failed rotate when Auth0 is enabled', async () => {
    const user = userEvent.setup();
    clearApiKey();
    setAuthProviderEnabled(true);

    const fetchMock = mockProjectListOnly();
    vi.stubGlobal('fetch', fetchMock);

    renderProjectsPage();

    await openRotateDialogAndSubmit(user);

    expect(await screen.findByText(/sign in required/i)).toBeInTheDocument();
    expect(mutationRequests(fetchMock)).toEqual([]);
  });

  it('keeps rotate, rename, and delete protected when no project API key is available', async () => {
    const user = userEvent.setup();
    clearApiKey();
    setAuthProviderEnabled(false);

    const fetchMock = mockProjectListOnly();
    vi.stubGlobal('fetch', fetchMock);

    renderProjectsPage();

    await openRotateDialogAndSubmit(user);
    const rotateDialog = screen.getByRole('dialog', { name: /rotate key for/i });
    expect(await within(rotateDialog).findByRole('alert')).toBeInTheDocument();
    expect(screen.queryByTestId('revealed-api-key')).toBeNull();
    await user.click(within(rotateDialog).getByRole('button', { name: /cancel/i }));

    const row = screen.getByTestId(`project-row-${EXISTING_PROJECT.id}`);
    await user.click(within(row).getByRole('button', { name: /^rename$/i }));
    const renameDialog = await screen.findByRole('dialog', { name: /rename/i });
    await user.clear(within(renameDialog).getByLabelText(/new name/i));
    await user.type(within(renameDialog).getByLabelText(/new name/i), 'Renamed');
    await user.click(within(renameDialog).getByRole('button', { name: /^save$/i }));
    expect(await within(renameDialog).findByRole('alert')).toBeInTheDocument();
    await user.click(within(renameDialog).getByRole('button', { name: /cancel/i }));

    await user.click(within(row).getByRole('button', { name: /^delete$/i }));
    const deleteDialog = await screen.findByRole('dialog', { name: /delete "/i });
    await user.type(
      within(deleteDialog).getByLabelText(/type the project name to confirm/i),
      EXISTING_PROJECT.name
    );
    await user.click(
      within(deleteDialog).getByRole('button', { name: /delete project/i })
    );
    expect(await within(deleteDialog).findByRole('alert')).toBeInTheDocument();

    expect(mutationRequests(fetchMock)).toEqual([]);
  });
});

function mockProjectListOnly() {
  return vi.fn(async (input: RequestInput, init?: RequestInit) => {
    const url = typeof input === 'string' ? input : input.toString();
    const method = (init?.method ?? 'GET').toUpperCase();
    if (method === 'GET' && url.includes('/api/projects')) {
      return jsonResponse({ projects: [EXISTING_PROJECT] });
    }
    throw new Error(`unexpected ${method} ${url}`);
  });
}

function mutationRequests(fetchMock: ReturnType<typeof vi.fn>): string[] {
  return fetchMock.mock.calls
    .map(([input, init]) => {
      const url = typeof input === 'string' ? input : String(input);
      const method = ((init as RequestInit | undefined)?.method ?? 'GET').toUpperCase();
      return `${method} ${url}`;
    })
    .filter((call) => !call.startsWith('GET '));
}

async function openRotateDialogAndSubmit(
  user: ReturnType<typeof userEvent.setup>
): Promise<void> {
  await screen.findByText(EXISTING_PROJECT.name);
  const row = screen.getByTestId(`project-row-${EXISTING_PROJECT.id}`);
  await user.click(within(row).getByRole('button', { name: /rotate key/i }));
  const rotateDialog = await screen.findByRole('dialog', { name: /rotate key for/i });
  await user.click(within(rotateDialog).getByRole('button', { name: /^rotate key$/i }));
}
