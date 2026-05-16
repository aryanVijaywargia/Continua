/* eslint-disable react-refresh/only-export-components */
import { Auth0Provider, useAuth0 } from '@auth0/auth0-react';
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom';
import {
  fetchRuntimeAuthConfig,
  getApiKey,
  setAccessTokenProvider,
  setApiKey,
  setPublicDemoMode,
  type RuntimeAuthConfig,
} from '../api/client';

const RUN_LOCALLY_DOCS_URL =
  'https://www.continua.in/docs/guides/installation';

type RuntimeAuthStatus = 'loading' | 'ready' | 'error';

export interface RuntimeAuthState extends RuntimeAuthConfig {
  status: RuntimeAuthStatus;
  error?: string;
}

const defaultRuntimeAuthState: RuntimeAuthState = {
  status: 'loading',
  enabled: false,
};

const RuntimeAuthContext = createContext<RuntimeAuthState>(defaultRuntimeAuthState);

declare global {
  interface Window {
    __CONTINUA_E2E_AUTH_BYPASS__?: boolean;
    __CONTINUA_E2E_AUTH_TOKEN__?: string;
  }
}

function isE2EAuthBypassEnabled(): boolean {
  return (
    import.meta.env.VITE_CONTINUA_E2E_AUTH_BYPASS === '1' ||
    (typeof window !== 'undefined' && window.__CONTINUA_E2E_AUTH_BYPASS__ === true)
  );
}

function getE2EAuthToken(): string {
  if (typeof window === 'undefined') {
    return 'e2e-operator-token';
  }
  return window.__CONTINUA_E2E_AUTH_TOKEN__ ?? 'e2e-operator-token';
}

function isLocalBrowserOrigin(): boolean {
  if (typeof window === 'undefined') {
    return false;
  }

  return ['localhost', '127.0.0.1', '::1'].includes(window.location.hostname);
}

export function useRuntimeAuthState(): RuntimeAuthState {
  const [state, setState] = useState<RuntimeAuthState>({
    ...defaultRuntimeAuthState,
  });

  useEffect(() => {
    let cancelled = false;

    void fetchRuntimeAuthConfig()
      .then((config) => {
        if (cancelled) {
          return;
        }

        setState({
          status: 'ready',
          ...config,
        });
      })
      .catch((error) => {
        if (cancelled) {
          return;
        }

        if (isLocalBrowserOrigin()) {
          setState({
            status: 'ready',
            enabled: false,
            error:
              error instanceof Error
                ? error.message
                : 'Failed to load authentication configuration',
          });
          return;
        }

        setState({
          status: 'error',
          enabled: false,
          error:
            error instanceof Error
              ? error.message
              : 'Failed to load authentication configuration',
        });
      });

    return () => {
      cancelled = true;
    };
  }, []);

  return state;
}

export function RuntimeAuthStateProvider({
  auth,
  children,
}: {
  auth: RuntimeAuthState;
  children: ReactNode;
}) {
  return <RuntimeAuthContext.Provider value={auth}>{children}</RuntimeAuthContext.Provider>;
}

export function useRuntimeAuth(): RuntimeAuthState {
  return useContext(RuntimeAuthContext);
}

export function Auth0RuntimeProvider({
  auth,
  children,
}: {
  auth: RuntimeAuthState;
  children: ReactNode;
}) {
  const navigate = useNavigate();

  let content = (
    <UnauthenticatedSessionBridge publicDemoEnabled={auth.public_demo_enabled === true}>
      {children}
    </UnauthenticatedSessionBridge>
  );

  if (auth.public_demo_enabled === true) {
    content = (
      <UnauthenticatedSessionBridge publicDemoEnabled>
        {children}
      </UnauthenticatedSessionBridge>
    );
  } else if (isE2EAuthBypassEnabled()) {
    content = <E2EAuthSessionBridge>{children}</E2EAuthSessionBridge>;
  } else if (
    auth.status === 'ready' &&
    auth.enabled &&
    auth.domain &&
    auth.client_id &&
    auth.audience
  ) {
    content = (
      <Auth0Provider
        domain={auth.domain}
        clientId={auth.client_id}
        authorizationParams={{
          audience: auth.audience,
          redirect_uri: window.location.origin,
          scope: 'openid profile email',
        }}
        onRedirectCallback={(appState) => {
          const returnTo =
            typeof appState?.returnTo === 'string'
              ? appState.returnTo
              : '/dashboard';
          navigate(returnTo, { replace: true });
        }}
      >
        <AuthSessionBridge />
        {children}
      </Auth0Provider>
    );
  }

  return <RuntimeAuthStateProvider auth={auth}>{content}</RuntimeAuthStateProvider>;
}

function AuthSessionBridge() {
  const { getAccessTokenSilently, isAuthenticated } = useOperatorAuth();

  useLayoutEffect(() => {
    setPublicDemoMode(false);
    setAccessTokenProvider(async () => {
      if (!isAuthenticated) {
        return null;
      }

      return getAccessTokenSilently();
    });

    return () => {
      setAccessTokenProvider(null);
    };
  }, [getAccessTokenSilently, isAuthenticated]);

  return null;
}

function E2EAuthSessionBridge({ children }: { children: ReactNode }) {
  setAccessTokenProvider(async () => getE2EAuthToken());
  setPublicDemoMode(false);

  useLayoutEffect(() => {
    setPublicDemoMode(false);
    setAccessTokenProvider(async () => getE2EAuthToken());

    return () => {
      setAccessTokenProvider(null);
    };
  }, []);

  return children;
}

function UnauthenticatedSessionBridge({
  children,
  publicDemoEnabled,
}: {
  children: ReactNode;
  publicDemoEnabled: boolean;
}) {
  useLayoutEffect(() => {
    setAccessTokenProvider(null);
    setPublicDemoMode(publicDemoEnabled);

    return () => {
      setAccessTokenProvider(null);
      setPublicDemoMode(false);
    };
  }, [publicDemoEnabled]);

  return children;
}

export function ConsoleRoute({ auth }: { auth: RuntimeAuthState }) {
  if (isE2EAuthBypassEnabled() || auth.public_demo_enabled) {
    return <Outlet />;
  }

  return <ProtectedRoute auth={auth} />;
}

export function ProtectedRoute({ auth }: { auth: RuntimeAuthState }) {
  if (isE2EAuthBypassEnabled()) {
    return <Outlet />;
  }

  if (auth.status === 'loading') {
    return <RouteGateState message="Loading operator console..." />;
  }

  if (auth.status === 'error') {
    return (
      <RouteGateState
        title="Authentication setup failed"
        message={auth.error ?? 'Failed to load authentication configuration.'}
        primaryAction={{
          label: 'Retry',
          onClick: () => window.location.reload(),
        }}
        secondaryAction={{
          label: 'Return home',
          href: '/',
        }}
      />
    );
  }

  if (auth.console_available === false) {
    return (
      <RouteGateState
        title="Console backend not connected"
        message="This hosted Pages deployment is serving the static landing site only. Run Continua locally, or connect a backend demo deployment, before opening the operator console."
        primaryAction={{
          label: 'Run locally',
          href: RUN_LOCALLY_DOCS_URL,
        }}
        secondaryAction={{
          label: 'Return home',
          href: '/',
        }}
      />
    );
  }

  if (!auth.enabled) {
    return <LocalApiKeyProtectedOutlet />;
  }

  return <Auth0ProtectedOutlet />;
}

function LocalApiKeyProtectedOutlet() {
  const [draftKey, setDraftKey] = useState(() => getApiKey() ?? '');
  const [storedKey, setStoredKey] = useState(() => getApiKey());
  const [error, setError] = useState<string | null>(null);

  useLayoutEffect(() => {
    if (!storedKey) {
      return;
    }

    setAccessTokenProvider(async () => storedKey);
    return () => {
      setAccessTokenProvider(null);
    };
  }, [storedKey]);

  if (storedKey) {
    return <Outlet />;
  }

  return (
    <div className="mx-auto flex min-h-screen max-w-4xl items-center justify-center px-6 py-16">
      <section className="app-surface max-w-xl p-8">
        <div className="app-overline">Local debugger access</div>
        <h1 className="mt-3 text-3xl font-black tight-headline text-[var(--continua-text-primary)]">
          Enter a local project API key.
        </h1>
        <p className="mt-4 text-sm leading-7 text-[var(--continua-text-secondary)] sm:text-base">
          Auth0 is not configured for this deployment. For local self-hosting, use
          a project API key to inspect traces from your own database.
        </p>
        <form
          className="mt-6 space-y-4"
          onSubmit={(event) => {
            event.preventDefault();
            const normalizedKey = draftKey.trim();
            if (!normalizedKey) {
              setError('API key is required.');
              return;
            }
            setApiKey(normalizedKey);
            setStoredKey(normalizedKey);
            setError(null);
          }}
        >
          <div>
            <label
              htmlFor="local-api-key"
              className="mb-2 block text-sm font-semibold text-[var(--continua-text-primary)]"
            >
              Project API key
            </label>
            <input
              id="local-api-key"
              value={draftKey}
              onChange={(event) => setDraftKey(event.target.value)}
              className="app-input w-full"
              placeholder="default"
              type="password"
              autoComplete="off"
            />
            {error ? (
              <p className="mt-2 text-sm text-red-600">{error}</p>
            ) : null}
          </div>
          <div className="flex flex-col gap-3 sm:flex-row">
            <button type="submit" className="app-button-primary">
              Open local console
            </button>
            <Link to="/" className="app-button-secondary">
              Return home
            </Link>
          </div>
        </form>
      </section>
    </div>
  );
}

function Auth0ProtectedOutlet() {
  const { error, isAuthenticated, isLoading, loginWithRedirect } = useOperatorAuth();
  const location = useLocation();
  const loginStartedRef = useRef(false);
  const returnTo = `${location.pathname}${location.search}`;

  const triggerLogin = useCallback(() => {
    loginStartedRef.current = true;
    return loginWithRedirect({
      appState: {
        returnTo,
      },
    });
  }, [loginWithRedirect, returnTo]);

  useEffect(() => {
    if (isLoading || error || isAuthenticated || loginStartedRef.current) {
      return;
    }

    void triggerLogin();
  }, [error, isAuthenticated, isLoading, triggerLogin]);

  if (isLoading) {
    return <RouteGateState message="Loading your operator session..." />;
  }

  if (error) {
    return (
      <RouteGateState
        title="Authentication failed"
        message={error.message}
        primaryAction={{
          label: 'Sign in again',
          onClick: () => {
            void triggerLogin();
          },
        }}
        secondaryAction={{
          label: 'Return home',
          href: '/',
        }}
      />
    );
  }

  if (!isAuthenticated) {
    return (
      <RouteGateState
        message="Redirecting to sign in..."
        primaryAction={{
          label: 'Sign in again',
          onClick: () => {
            void triggerLogin();
          },
        }}
        secondaryAction={{
          label: 'Return home',
          href: '/',
        }}
      />
    );
  }

  return <Outlet />;
}

export function useOperatorAuth(): ReturnType<typeof useAuth0> {
  const runtimeAuth = useRuntimeAuth();
  const auth0 = useAuth0();

  if (isE2EAuthBypassEnabled()) {
    return {
      error: undefined,
      getAccessTokenSilently: async () => getE2EAuthToken(),
      isAuthenticated: true,
      isLoading: false,
      loginWithRedirect: async () => undefined,
      logout: async () => undefined,
      user: {
        email: 'operator@continua.dev',
        name: 'Continua Operator',
        sub: 'google-oauth2|e2e-operator',
      },
    } as ReturnType<typeof useAuth0>;
  }

  if (runtimeAuth.public_demo_enabled) {
    return unauthenticatedOperatorAuth();
  }

  if (runtimeAuth.status === 'ready' && !runtimeAuth.enabled) {
    const localApiKey = getApiKey();
    if (localApiKey) {
      return localApiKeyOperatorAuth(localApiKey);
    }
    return unauthenticatedOperatorAuth();
  }

  return auth0;
}

function localApiKeyOperatorAuth(localApiKey: string): ReturnType<typeof useAuth0> {
  return {
    error: undefined,
    getAccessTokenSilently: async () => localApiKey,
    isAuthenticated: true,
    isLoading: false,
    loginWithRedirect: async () => undefined,
    logout: async () => undefined,
    user: {
      email: 'Local API key',
      name: 'Local self-host',
      sub: 'local-api-key',
    },
  } as ReturnType<typeof useAuth0>;
}

function unauthenticatedOperatorAuth(): ReturnType<typeof useAuth0> {
  return {
    error: undefined,
    getAccessTokenSilently: async () => '',
    isAuthenticated: false,
    isLoading: false,
    loginWithRedirect: async () => undefined,
    logout: async () => undefined,
    user: undefined,
  } as ReturnType<typeof useAuth0>;
}

function RouteGateState({
  title = 'Authentication required',
  message,
  primaryAction,
  secondaryAction,
}: {
  title?: string;
  message: string;
  primaryAction?: {
    label: string;
    href?: string;
    onClick?: () => void;
  };
  secondaryAction?: {
    label: string;
    href?: string;
    onClick?: () => void;
  };
}) {
  return (
    <div className="mx-auto flex min-h-screen max-w-4xl items-center justify-center px-6 py-16">
      <section className="app-surface max-w-xl p-8 text-center">
        <div className="app-overline">Operator access</div>
        <h1 className="mt-3 text-3xl font-black tight-headline text-[var(--continua-text-primary)]">
          {title}
        </h1>
        <p className="mt-4 text-sm leading-7 text-[var(--continua-text-secondary)] sm:text-base">
          {message}
        </p>
        {primaryAction || secondaryAction ? (
          <div className="mt-6 flex flex-col items-center justify-center gap-3 sm:flex-row">
            {primaryAction ? (
              <RouteGateAction action={primaryAction} primary />
            ) : null}
            {secondaryAction ? (
              <RouteGateAction action={secondaryAction} />
            ) : null}
          </div>
        ) : null}
      </section>
    </div>
  );
}

function RouteGateAction({
  action,
  primary = false,
}: {
  action: {
    label: string;
    href?: string;
    onClick?: () => void;
  };
  primary?: boolean;
}) {
  const className = primary
    ? 'app-button-primary'
    : 'app-button-secondary';

  if (action.href) {
    if (/^https?:\/\//.test(action.href)) {
      return (
        <a
          href={action.href}
          target="_blank"
          rel="noreferrer"
          className={className}
        >
          {action.label}
        </a>
      );
    }

    return (
      <Link to={action.href} className={className}>
        {action.label}
      </Link>
    );
  }

  return (
    <button type="button" onClick={action.onClick} className={className}>
      {action.label}
    </button>
  );
}
