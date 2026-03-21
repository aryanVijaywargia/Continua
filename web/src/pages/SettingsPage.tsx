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
    <div className="min-h-screen bg-slate-50 dark:bg-slate-950">
      <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6 lg:px-8">
        <div className="mb-6">
          <h1 className="text-2xl font-bold text-slate-900 dark:text-slate-100">
            Settings
          </h1>
          <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
            Manage the debugger API key and local UI preferences for this browser.
          </p>
        </div>

        <div className="grid gap-6">
          <section className="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm dark:border-slate-800 dark:bg-slate-900">
            <div className="mb-4">
              <h2 className="text-lg font-semibold text-slate-900 dark:text-slate-100">
                API Key
              </h2>
              <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
                The key is stored locally and sent as `X-API-Key` on API requests.
              </p>
            </div>

            <dl className="rounded-xl border border-slate-200 bg-slate-50 p-4 dark:border-slate-800 dark:bg-slate-950/60">
              <dt className="text-xs font-semibold uppercase tracking-[0.16em] text-slate-500 dark:text-slate-400">
                Current key
              </dt>
              <dd className="mt-2 font-mono text-sm text-slate-900 dark:text-slate-100">
                {storedKey ? maskApiKey(storedKey) : 'Not configured'}
              </dd>
            </dl>

            <form className="mt-4 space-y-4" onSubmit={handleSave}>
              <div>
                <label
                  htmlFor="settings-api-key"
                  className="mb-1 block text-sm font-medium text-slate-700 dark:text-slate-200"
                >
                  New API key
                </label>
                <input
                  id="settings-api-key"
                  type="password"
                  value={draftKey}
                  onChange={(event) => setDraftKey(event.target.value)}
                  placeholder="ck_live_..."
                  className="w-full rounded-xl border border-slate-300 bg-white px-3 py-2 text-sm text-slate-900 shadow-sm transition focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-200 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-100 dark:focus:border-sky-400 dark:focus:ring-sky-900"
                />
              </div>

              <div className="flex flex-wrap items-center gap-3">
                <button
                  type="submit"
                  className="rounded-lg bg-blue-600 px-3 py-2 text-sm font-medium text-white transition hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-200"
                >
                  Save key
                </button>
                <button
                  type="button"
                  onClick={handleClear}
                  className="rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm font-medium text-slate-700 transition hover:bg-slate-100 focus:outline-none focus:ring-2 focus:ring-blue-200 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-200 dark:hover:bg-slate-900"
                >
                  Clear key
                </button>
                {statusMessage ? (
                  <span className="text-sm text-slate-500 dark:text-slate-400">
                    {statusMessage}
                  </span>
                ) : null}
              </div>
            </form>
          </section>

          <section className="rounded-2xl border border-slate-200 bg-white p-6 shadow-sm dark:border-slate-800 dark:bg-slate-900">
            <div className="mb-4">
              <h2 className="text-lg font-semibold text-slate-900 dark:text-slate-100">
                Theme
              </h2>
              <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
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
                    className={`rounded-full px-4 py-2 text-sm font-medium transition focus:outline-none focus:ring-2 focus:ring-blue-200 ${
                      isActive
                        ? 'bg-slate-900 text-white dark:bg-slate-100 dark:text-slate-950'
                        : 'border border-slate-300 bg-white text-slate-700 hover:bg-slate-100 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-200 dark:hover:bg-slate-900'
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
