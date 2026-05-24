import { useState } from 'react';
import { Link, Navigate, useLocation } from 'react-router-dom';
import { clearApiKey, getApiKey, setApiKey } from '../api/client';
import { useTheme } from '../hooks/useTheme';
import { useOperatorAuth, useRuntimeAuth } from '../auth/runtime';
import { PageHeader } from '../components/DebuggerKit';
import {
  buildProjectPath,
  getProjectIdFromSearchParams,
} from '../utils/projectSearchParams';

const THEME_OPTIONS = [
  { id: 'system', label: 'System' },
  { id: 'light', label: 'Light' },
  { id: 'dark', label: 'Dark' },
] as const;

export function SettingsPage() {
  const location = useLocation();
  const runtimeAuth = useRuntimeAuth();
  const { logout, user } = useOperatorAuth();
  const { mode, resolvedTheme, setMode } = useTheme();
  const isLocalApiKeyMode =
    runtimeAuth.status === 'ready' &&
    !runtimeAuth.enabled &&
    runtimeAuth.public_demo_enabled !== true;
  const [localApiKeyDraft, setLocalApiKeyDraft] = useState(() => getApiKey() ?? '');
  const [localApiKeySaved, setLocalApiKeySaved] = useState(() => Boolean(getApiKey()));
  const projectId = getProjectIdFromSearchParams(new URLSearchParams(location.search));

  if (runtimeAuth.public_demo_enabled) {
    return <Navigate to="/dashboard" replace />;
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <PageHeader
        description={
          isLocalApiKeyMode
            ? 'Local self-host access uses a project API key. Theme controls stay here.'
            : 'Auth is handled through Auth0. Theme controls and sign-out stay here.'
        }
        title="Settings"
        tabs={[
          { id: 'general', label: 'General', active: true },
          { id: 'keys', label: 'API keys' },
          { id: 'appearance', label: 'Appearance' },
        ]}
      />

      <div className="grid max-w-5xl gap-8 p-6 xl:grid-cols-[minmax(0,1.05fr)_minmax(0,0.95fr)]">
        <section>
          {isLocalApiKeyMode ? (
            <LocalApiKeySettings
              draft={localApiKeyDraft}
              saved={localApiKeySaved}
              onDraftChange={setLocalApiKeyDraft}
              onSave={() => {
                const normalizedKey = localApiKeyDraft.trim();
                if (!normalizedKey) {
                  return;
                }
                setApiKey(normalizedKey);
                setLocalApiKeyDraft(normalizedKey);
                setLocalApiKeySaved(true);
              }}
              onClear={() => {
                clearApiKey();
                setLocalApiKeyDraft('');
                setLocalApiKeySaved(false);
              }}
            />
          ) : (
            <OperatorAccountSettings
              user={user}
              onSignOut={() =>
                logout({
                  logoutParams: {
                    returnTo: window.location.origin,
                  },
                })
              }
            />
          )}
        </section>

        <section>
          <div className="mb-4">
            <h2 className="text-sm font-semibold text-[var(--c-text-primary)]">
              Appearance
            </h2>
            <p className="mt-1 text-sm text-[var(--c-text-secondary)]">
              Choose how the debugger resolves its theme on this device.
            </p>
          </div>

          <div className="grid gap-3 sm:grid-cols-3">
            {THEME_OPTIONS.map((option) => {
              const active = mode === option.id;

              return (
                <button
                  key={option.id}
                  type="button"
                  onClick={() => setMode(option.id)}
                  className={`rounded-md border px-4 py-4 text-left transition ${
                    active
                      ? 'border-[var(--c-accent-border)] bg-[var(--c-accent-faint)] text-[var(--c-text-primary)]'
                      : 'border-[var(--c-border)] bg-[var(--c-surface)] text-[var(--c-text-secondary)] hover:text-[var(--c-text-primary)]'
                  }`}
                >
                  <div className="text-sm font-bold">{option.label}</div>
                  <div className="mt-2 text-xs uppercase tracking-[0.08em] text-[var(--c-text-muted)]">
                    {active ? 'Active' : 'Available'}
                  </div>
                </button>
              );
            })}
          </div>

          <div className="app-surface-muted mt-5 p-4">
            <div className="text-xs font-semibold uppercase tracking-[0.08em] text-[var(--c-text-muted)]">
              Resolved theme
            </div>
            <div className="mt-2 text-sm text-[var(--c-text-primary)]">
              {resolvedTheme}
            </div>
          </div>

          <section className="mt-8">
            <div className="mb-4">
              <h2 className="text-sm font-semibold text-[var(--c-text-primary)]">
                Operations
              </h2>
              <p className="mt-1 text-sm text-[var(--c-text-secondary)]">
                Operator tools for maintaining engine projections.
              </p>
            </div>
            <Link
              className="block rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-4 py-3 transition hover:border-[var(--c-border-strong)]"
              to={buildProjectPath('/tools/engine-projections', projectId)}
            >
              <span className="block text-sm font-semibold text-[var(--c-text-primary)]">
                Engine projection repair
              </span>
              <span className="mt-1 block text-sm text-[var(--c-text-secondary)]">
                Re-run engine projections for stale or expired runs.
              </span>
            </Link>
          </section>
        </section>
      </div>
    </div>
  );
}

function OperatorAccountSettings({
  user,
  onSignOut,
}: {
  user: ReturnType<typeof useOperatorAuth>['user'];
  onSignOut: () => void;
}) {
  return (
    <>
      <div className="mb-4">
        <h2 className="text-xl font-black tight-headline text-[var(--continua-text-primary)]">
          Operator account
        </h2>
        <p className="mt-1 text-sm text-[var(--continua-text-secondary)]">
          Review the signed-in identity used for debugger access.
        </p>
      </div>

      <dl className="space-y-4">
        <div className="app-surface-muted p-4">
          <dt className="text-xs font-semibold uppercase tracking-[0.16em] text-[var(--continua-text-muted)]">
            Email
          </dt>
          <dd className="mt-2 text-sm text-[var(--continua-text-primary)]">
            {user?.email ?? 'Unavailable'}
          </dd>
        </div>

        <div className="app-surface-muted p-4">
          <dt className="text-xs font-semibold uppercase tracking-[0.16em] text-[var(--continua-text-muted)]">
            Name
          </dt>
          <dd className="mt-2 text-sm text-[var(--continua-text-primary)]">
            {user?.name ?? user?.nickname ?? 'Unavailable'}
          </dd>
        </div>

        <div className="app-surface-muted p-4">
          <dt className="text-xs font-semibold uppercase tracking-[0.16em] text-[var(--continua-text-muted)]">
            Subject
          </dt>
          <dd className="mt-2 break-all font-mono text-xs text-[var(--continua-text-secondary)]">
            {user?.sub ?? 'Unavailable'}
          </dd>
        </div>
      </dl>

      <button
        type="button"
        onClick={onSignOut}
        className="app-button-secondary mt-5"
      >
        Sign out
      </button>
    </>
  );
}

function LocalApiKeySettings({
  draft,
  saved,
  onDraftChange,
  onSave,
  onClear,
}: {
  draft: string;
  saved: boolean;
  onDraftChange: (value: string) => void;
  onSave: () => void;
  onClear: () => void;
}) {
  return (
    <>
      <div className="mb-4">
        <h2 className="text-xl font-black tight-headline text-[var(--continua-text-primary)]">
          Local API key
        </h2>
        <p className="mt-1 text-sm text-[var(--continua-text-secondary)]">
          Store a project API key in this browser for local self-hosted debugging.
        </p>
      </div>

      <div className="app-surface-muted p-4">
        <div className="text-xs font-semibold uppercase tracking-[0.16em] text-[var(--continua-text-muted)]">
          Current key
        </div>
        <div className="mt-2 text-sm text-[var(--continua-text-primary)]">
          {saved ? 'Saved in this browser' : 'No key saved'}
        </div>
      </div>

      <div className="mt-5">
        <label
          htmlFor="settings-local-api-key"
          className="mb-2 block text-sm font-semibold text-[var(--continua-text-primary)]"
        >
          Project API key
        </label>
        <input
          id="settings-local-api-key"
          className="app-input w-full"
          type="password"
          autoComplete="off"
          value={draft}
          onChange={(event) => onDraftChange(event.target.value)}
          placeholder="pk_..."
        />
      </div>

      <div className="mt-5 flex flex-col gap-3 sm:flex-row">
        <button
          type="button"
          className="app-button-primary"
          onClick={onSave}
          disabled={draft.trim() === ''}
        >
          Save local key
        </button>
        <button
          type="button"
          className="app-button-secondary"
          onClick={onClear}
          disabled={!saved}
        >
          Clear local key
        </button>
      </div>
    </>
  );
}
