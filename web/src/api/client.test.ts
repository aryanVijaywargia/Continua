import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  ApiError,
  clearApiKey,
  fetchAPI,
  fetchRuntimeAuthConfig,
  forgetProjectApiKey,
  getFallbackProjectApiKey,
  getKnownProjectApiKey,
  rememberProjectApiKey,
  setAccessTokenProvider,
  setPublicDemoMode,
  setSelectedProjectIdProvider,
} from './client';

const PROJECT_ID = '11111111-1111-1111-1111-111111111111';

let fetchMock: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchMock = vi.fn();
  vi.stubGlobal('fetch', fetchMock);
  window.localStorage.clear();
  clearApiKey();
  setAccessTokenProvider(null);
  setPublicDemoMode(false);
  setSelectedProjectIdProvider(null);
});

afterEach(() => {
  clearApiKey();
  setAccessTokenProvider(null);
  setPublicDemoMode(false);
  setSelectedProjectIdProvider(null);
  vi.unstubAllGlobals();
});

describe('client', () => {
  it('sends bearer auth and the selected project_id on debugger requests', async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ traces: [], total: 0 }), {
        status: 200,
        headers: {
          'Content-Type': 'application/json',
        },
      })
    );
    setAccessTokenProvider(async () => 'operator-token');
    setSelectedProjectIdProvider(() => PROJECT_ID);

    await fetchAPI('/api/traces');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const requestUrl = new URL(
      String(fetchMock.mock.calls[0]?.[0]),
      'http://localhost'
    );
    expect(requestUrl.pathname).toBe('/api/traces');
    expect(requestUrl.searchParams.get('project_id')).toBe(PROJECT_ID);
    expect(
      (fetchMock.mock.calls[0]?.[1] as RequestInit | undefined)?.headers
    ).toMatchObject({
      'Content-Type': 'application/json',
      Authorization: 'Bearer operator-token',
    });
  });

  it('does not append project_id to engine requests', async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ run_id: 'run-1' }), {
        status: 200,
        headers: {
          'Content-Type': 'application/json',
        },
      })
    );
    setAccessTokenProvider(async () => 'operator-token');
    setSelectedProjectIdProvider(() => PROJECT_ID);

    await fetchAPI('/v1/engine/runs/run-1/pending-work');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const requestUrl = new URL(
      String(fetchMock.mock.calls[0]?.[0]),
      'http://localhost'
    );
    expect(requestUrl.pathname).toBe('/v1/engine/runs/run-1/pending-work');
    expect(requestUrl.searchParams.get('project_id')).toBeNull();
  });

  it('loads runtime auth config from the public endpoint without debugger auth', async () => {
    fetchMock.mockResolvedValue(
      new Response(
        JSON.stringify({
          enabled: true,
          domain: 'continua.us.auth0.com',
          client_id: 'client-id',
          audience: 'https://continua/api',
        }),
        {
          status: 200,
          headers: {
            'Content-Type': 'application/json',
          },
        }
      )
    );

    const config = await fetchRuntimeAuthConfig();

    expect(config).toEqual({
      enabled: true,
      domain: 'continua.us.auth0.com',
      client_id: 'client-id',
      audience: 'https://continua/api',
    });
    expect(fetchMock).toHaveBeenCalledWith('/api/auth/config');
  });

  it('treats an HTML auth config response as a static landing deployment', async () => {
    fetchMock.mockResolvedValue(
      new Response('<!DOCTYPE html><html></html>', {
        status: 200,
        headers: {
          'Content-Type': 'text/html; charset=utf-8',
        },
      })
    );

    const config = await fetchRuntimeAuthConfig();

    expect(config).toEqual({
      enabled: false,
      console_available: false,
    });
  });

  it('fails fast when no operator token provider is installed', async () => {
    await expect(fetchAPI('/api/traces')).rejects.toMatchObject({
      status: 401,
      code: 'unauthorized',
      message: 'Sign in required',
    } satisfies Partial<ApiError>);
  });

  it('uses a stored local API key when no operator token provider is installed', async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ traces: [], total: 0 }), {
        status: 200,
        headers: {
          'Content-Type': 'application/json',
        },
      })
    );
    window.localStorage.setItem('continua_api_key', 'stored-local-key');

    await fetchAPI('/api/traces');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(
      (fetchMock.mock.calls[0]?.[1] as RequestInit | undefined)?.headers
    ).toMatchObject({
      'Content-Type': 'application/json',
      Authorization: 'Bearer stored-local-key',
    });
  });

  it('allows unauthenticated demo reads when public demo mode is enabled', async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ traces: [], total: 0 }), {
        status: 200,
        headers: {
          'Content-Type': 'application/json',
        },
      })
    );
    setPublicDemoMode(true);

    await fetchAPI('/api/traces');

    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(
      (fetchMock.mock.calls[0]?.[1] as RequestInit | undefined)?.headers
    ).toMatchObject({
      'Content-Type': 'application/json',
    });
    expect(
      (fetchMock.mock.calls[0]?.[1] as RequestInit | undefined)?.headers
    ).not.toMatchObject({
      Authorization: expect.anything(),
    });
  });

  it('remembers project API keys and exposes a surviving fallback key', () => {
    rememberProjectApiKey('project-a', 'pk_project_a');
    rememberProjectApiKey('project-b', 'pk_project_b');

    expect(getKnownProjectApiKey('project-a')).toBe('pk_project_a');
    forgetProjectApiKey('project-a');
    expect(getKnownProjectApiKey('project-a')).toBeNull();
    expect(getFallbackProjectApiKey()).toBe('pk_project_b');
    expect(getFallbackProjectApiKey('pk_project_b')).toBeNull();
  });
});
