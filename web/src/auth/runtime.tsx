/* eslint-disable react-refresh/only-export-components */
import {
  createContext,
  useContext,
  useEffect,
  useLayoutEffect,
  useState,
  type ReactNode,
} from 'react';
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom';
import {
  fetchProjects,
  fetchRuntimeAuthConfig,
  getApiKey,
  LOCAL_API_KEY_CHANGED_EVENT,
  setAccessTokenProvider,
  setApiKey,
  setLocalApiKeyMode,
  setLocalSingleUserMode,
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
};

const RuntimeAuthContext = createContext<RuntimeAuthState>(defaultRuntimeAuthState);

/**
 * The console session as the shell and settings see it. A session is
 * authenticated when the browser holds a project API key or the server offered
 * API-key-free local mode to this loopback caller.
 */
export interface OperatorAuth {
  isAuthenticated: boolean;
  user?: { email: string };
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
            error:
              error instanceof Error
                ? error.message
                : 'Failed to load authentication configuration',
          });
          return;
        }

        setState({
          status: 'error',
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

export function RuntimeAuthProvider({
  auth,
  children,
}: {
  auth: RuntimeAuthState;
  children: ReactNode;
}) {
  const publicDemoEnabled = auth.public_demo_enabled === true;
  const localSingleUserEnabled = isLocalSingleUserRuntime(auth);

  useLayoutEffect(() => {
    setLocalApiKeyMode(false);
    setAccessTokenProvider(null);
    setPublicDemoMode(publicDemoEnabled);
    setLocalSingleUserMode(localSingleUserEnabled);

    return () => {
      setAccessTokenProvider(null);
      setPublicDemoMode(false);
      setLocalSingleUserMode(false);
    };
  }, [localSingleUserEnabled, publicDemoEnabled]);

  return <RuntimeAuthStateProvider auth={auth}>{children}</RuntimeAuthStateProvider>;
}

// isLocalSingleUserRuntime reports whether the server offered API-key-free local
// access. The server only advertises it to loopback callers, and it is mutually
// exclusive with the public demo.
function isLocalSingleUserRuntime(auth: RuntimeAuthState): boolean {
  return auth.local_mode_enabled === true && auth.public_demo_enabled !== true;
}

export function ConsoleRoute({ auth }: { auth: RuntimeAuthState }) {
  if (auth.public_demo_enabled) {
    return <Outlet />;
  }

  return <ProtectedRoute auth={auth} />;
}

export function ProtectedRoute({ auth }: { auth: RuntimeAuthState }) {
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

  return (
    <LocalApiKeyProtectedOutlet
      localSingleUserEnabled={isLocalSingleUserRuntime(auth)}
    />
  );
}

function LocalApiKeyProtectedOutlet({
  localSingleUserEnabled,
}: {
  localSingleUserEnabled: boolean;
}) {
  const location = useLocation();
  const navigate = useNavigate();
  const [draftKey, setDraftKey] = useState(() => getApiKey() ?? '');
  const [storedKey, setStoredKey] = useState(() => getApiKey());
  const [error, setError] = useState<string | null>(null);
  const [bootstrapChecked, setBootstrapChecked] = useState(false);
  const isProjectBootstrapRoute = location.pathname === '/projects';

  useLayoutEffect(() => {
    if (!storedKey) {
      return;
    }

    setAccessTokenProvider(async () => storedKey);
    setLocalApiKeyMode(true);
    return () => {
      setLocalApiKeyMode(false);
      setAccessTokenProvider(null);
    };
  }, [storedKey]);

  useEffect(() => {
    const syncStoredKey = () => {
      const nextKey = getApiKey();
      setStoredKey(nextKey);
      if (nextKey) {
        setDraftKey(nextKey);
      }
    };

    window.addEventListener(LOCAL_API_KEY_CHANGED_EVENT, syncStoredKey);
    return () => {
      window.removeEventListener(LOCAL_API_KEY_CHANGED_EVENT, syncStoredKey);
    };
  }, []);

  useEffect(() => {
    if (storedKey || isProjectBootstrapRoute) {
      setBootstrapChecked(true);
      return;
    }

    let cancelled = false;
    setBootstrapChecked(false);
    void fetchProjects()
      .then((projectList) => {
        if (cancelled) {
          return;
        }
        if (projectList.projects.length === 0) {
          navigate('/projects', { replace: true });
          return;
        }
        setBootstrapChecked(true);
      })
      .catch(() => {
        if (!cancelled) {
          setBootstrapChecked(true);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [isProjectBootstrapRoute, navigate, storedKey]);

  if (storedKey) {
    return <Outlet />;
  }

  if (isProjectBootstrapRoute) {
    return <Outlet />;
  }

  if (!bootstrapChecked) {
    return <RouteGateState message="Checking local project setup..." />;
  }

  // API-key-free local mode: the server already trusts this loopback caller, so
  // there is nothing to prompt for. The project selector picks the read scope.
  if (localSingleUserEnabled) {
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
          For local self-hosting, use a project API key to inspect traces from
          your own database.
        </p>
        <p className="mt-3 text-sm leading-6 text-[var(--continua-text-secondary)]">
          First time here?{' '}
          <Link
            to="/projects"
            className="font-medium text-[var(--c-accent-text)] hover:underline"
          >
            Create your first project →
          </Link>
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
              placeholder="pk_..."
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

export function useOperatorAuth(): OperatorAuth {
  const runtimeAuth = useRuntimeAuth();

  if (runtimeAuth.status !== 'ready' || runtimeAuth.public_demo_enabled) {
    return { isAuthenticated: false };
  }
  if (getApiKey()) {
    return { isAuthenticated: true, user: { email: 'Local API key' } };
  }
  // API-key-free local mode represents the box owner: the session counts as
  // authenticated but carries no credential, so the client sends no
  // Authorization header and the server scopes reads by the selected project_id.
  if (isLocalSingleUserRuntime(runtimeAuth)) {
    return { isAuthenticated: true, user: { email: 'Local mode' } };
  }
  return { isAuthenticated: false };
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
