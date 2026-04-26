import { useState } from 'react';
import { Navigate } from 'react-router-dom';
import { clearApiKey, getApiKey, setApiKey } from '../api/client';
import { useTheme } from '../hooks/useTheme';
import { useOperatorAuth, useRuntimeAuth } from '../auth/runtime';

const THEME_OPTIONS = [
  { id: 'system', label: 'System' },
  { id: 'light', label: 'Light' },
  { id: 'dark', label: 'Dark' },
] as const;

export function SettingsPage() {
  const runtimeAuth = useRuntimeAuth();
  const { logout, user } = useOperatorAuth();
  const { mode, resolvedTheme, setMode } = useTheme();
  const isLocalApiKeyMode =
    runtimeAuth.status === 'ready' &&
    !runtimeAuth.enabled &&
    runtimeAuth.public_demo_enabled !== true;
  const [localApiKeyDraft, setLocalApiKeyDraft] = useState(() => getApiKey() ?? '');
  const [localApiKeySaved, setLocalApiKeySaved] = useState(() => Boolean(getApiKey()));

  if (runtimeAuth.public_demo_enabled) {
    return <Navigate to="/dashboard" replace />;
  }

  return (
    <div className="app-page max-w-5xl space-y-6">
      <section className="app-surface p-6 sm:p-7">
        <div className="app-overline">Operator settings</div>
        <h1 className="mt-3 text-3xl font-black tight-headline text-[var(--continua-text-primary)] sm:text-4xl">
          Manage your operator session and debugger workspace.
        </h1>
        <p className="mt-3 max-w-3xl text-sm leading-7 text-[var(--continua-text-secondary)] sm:text-base">
          {isLocalApiKeyMode
            ? 'Local self-host access uses a project API key. Theme controls stay here.'
            : 'Auth is handled through Auth0. Theme controls and sign-out stay here.'}
        </p>
      </section>

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1.05fr)_minmax(0,0.95fr)]">
        <section className="app-surface p-6">
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

        <section className="app-surface p-6">
          <div className="mb-4">
            <h2 className="text-xl font-black tight-headline text-[var(--continua-text-primary)]">
              Appearance
            </h2>
            <p className="mt-1 text-sm text-[var(--continua-text-secondary)]">
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
                  className={`rounded-2xl border px-4 py-4 text-left transition ${
                    active
                      ? 'border-[var(--continua-accent)] bg-[var(--continua-accent-faint)] text-[var(--continua-text-primary)]'
                      : 'border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] text-[var(--continua-text-secondary)] hover:text-[var(--continua-text-primary)]'
                  }`}
                >
                  <div className="text-sm font-bold">{option.label}</div>
                  <div className="mt-2 text-xs uppercase tracking-[0.16em] text-[var(--continua-text-muted)]">
                    {active ? 'Active' : 'Available'}
                  </div>
                </button>
              );
            })}
          </div>

          <div className="app-surface-muted mt-5 p-4">
            <div className="text-xs font-semibold uppercase tracking-[0.16em] text-[var(--continua-text-muted)]">
              Resolved theme
            </div>
            <div className="mt-2 text-sm text-[var(--continua-text-primary)]">
              {resolvedTheme}
            </div>
          </div>
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
          placeholder="default"
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
