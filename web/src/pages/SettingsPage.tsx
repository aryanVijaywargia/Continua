import { useEffect, useState } from 'react';
import { clearApiKey, getApiKey, setApiKey } from '../api/client';
import { useTheme, type ThemeMode } from '../hooks/useTheme';

export function SettingsPage() {
  const { mode, resolvedTheme, setMode } = useTheme();
  const [draftKey, setDraftKey] = useState('');
  const [storedKey, setStoredKey] = useState<string | null>(() => getApiKey());
  const [statusMessage, setStatusMessage] = useState<string | null>(null);

  useEffect(() => {
    if (!statusMessage) {
      return;
    }

    const timeoutId = window.setTimeout(() => {
      setStatusMessage(null);
    }, 2500);

    return () => {
      window.clearTimeout(timeoutId);
    };
  }, [statusMessage]);

  const handleSave = (event: React.FormEvent) => {
    event.preventDefault();

    const nextKey = draftKey.trim();
    if (!nextKey) {
      setStatusMessage('Enter an API key before saving.');
      return;
    }

    setApiKey(nextKey);
    setStoredKey(nextKey);
    setDraftKey('');
    setStatusMessage('API key saved.');
  };

  const handleClear = () => {
    clearApiKey();
    setStoredKey(null);
    setDraftKey('');
    setStatusMessage('API key cleared.');
  };

  return (
    <div className="app-page max-w-5xl">
      <section className="app-surface p-6 sm:p-7">
        <div className="app-overline">Local settings</div>
        <h1 className="mt-3 text-3xl font-black tight-headline text-[var(--continua-text-primary)] sm:text-4xl">
          Configure this browser as an operator workspace.
        </h1>
        <p className="mt-3 max-w-3xl text-sm leading-7 text-[var(--continua-text-secondary)] sm:text-base">
          Manage the debugger API key and local UI preferences for this browser.
        </p>
      </section>

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]">
        <section className="app-surface p-6">
          <div className="mb-4">
            <h2 className="text-xl font-black tight-headline text-[var(--continua-text-primary)]">
              API key
            </h2>
            <p className="mt-1 text-sm text-[var(--continua-text-secondary)]">
              The key is stored locally and sent as `X-API-Key` on API requests.
            </p>
          </div>

          <dl className="app-surface-muted p-4">
            <dt className="text-xs font-semibold uppercase tracking-[0.16em] text-[var(--continua-text-muted)]">
              Current key
            </dt>
            <dd className="mt-2 font-mono text-sm text-[var(--continua-text-primary)]">
              {storedKey ? maskApiKey(storedKey) : 'Not configured'}
            </dd>
          </dl>

          <form className="mt-5 space-y-4" onSubmit={handleSave}>
            <div>
              <label
                htmlFor="settings-api-key"
                className="mb-1 block text-sm font-medium text-[var(--continua-text-secondary)]"
              >
                New API key
              </label>
              <input
                id="settings-api-key"
                type="password"
                value={draftKey}
                onChange={(event) => setDraftKey(event.target.value)}
                placeholder="ck_live_..."
                className="app-input"
              />
            </div>

            <div className="flex flex-wrap items-center gap-3">
              <button
                type="submit"
                className="app-button-primary"
              >
                Save key
              </button>
              <button
                type="button"
                onClick={handleClear}
                className="app-button-secondary"
              >
                Clear key
              </button>
              {statusMessage ? (
                <span className="text-sm text-[var(--continua-text-muted)]">
                  {statusMessage}
                </span>
              ) : null}
            </div>
          </form>
        </section>

        <section className="app-surface p-6">
          <div className="mb-4">
            <h2 className="text-xl font-black tight-headline text-[var(--continua-text-primary)]">
              Theme
            </h2>
            <p className="mt-1 text-sm text-[var(--continua-text-secondary)]">
              Current rendering: {resolvedTheme}. System mode follows the OS preference.
            </p>
          </div>

          <div className="flex flex-wrap gap-3">
            {(['system', 'light', 'dark'] as ThemeMode[]).map((themeMode) => {
              const isActive = themeMode === mode;

              return (
                <button
                  key={themeMode}
                  type="button"
                  aria-pressed={isActive}
                  onClick={() => setMode(themeMode)}
                  className={`rounded-full px-4 py-2 text-sm font-medium transition focus:outline-none focus:ring-2 focus:ring-[var(--continua-accent-faint)] ${
                    isActive
                      ? 'border border-[var(--continua-accent)] bg-[var(--continua-accent-faint)] text-[var(--continua-accent)]'
                      : 'border border-[var(--continua-border-strong)] bg-[var(--continua-surface-elevated)] text-[var(--continua-text-secondary)] hover:border-[var(--continua-accent)] hover:text-[var(--continua-accent)]'
                  }`}
                >
                  {formatThemeMode(themeMode)}
                </button>
              );
            })}
          </div>
        </section>
      </div>
    </div>
  );
}

function maskApiKey(key: string) {
  if (key.length <= 8) {
    return `${key.slice(0, 2)}••••${key.slice(-2)}`;
  }

  return `${key.slice(0, 4)}••••••${key.slice(-4)}`;
}

function formatThemeMode(mode: ThemeMode) {
  return mode.charAt(0).toUpperCase() + mode.slice(1);
}
