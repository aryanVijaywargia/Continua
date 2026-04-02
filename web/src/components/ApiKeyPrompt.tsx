import { useState } from 'react';
import { setApiKey } from '../api/client';

interface ApiKeyPromptProps {
  onSubmit: () => void;
}

/**
 * Component to prompt user for API key.
 */
export function ApiKeyPrompt({ onSubmit }: ApiKeyPromptProps) {
  const [key, setKey] = useState('');
  const [error, setError] = useState('');

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!key.trim()) {
      setError('API key is required');
      return;
    }
    setApiKey(key.trim());
    onSubmit();
  };

  return (
    <div className="app-page min-h-full justify-center py-10">
      <div className="app-surface mx-auto w-full max-w-2xl overflow-hidden p-8 sm:p-10">
        <div className="app-overline">Access required</div>
        <h2 className="mt-4 text-3xl font-semibold tracking-[-0.04em] text-[var(--continua-text-primary)]">
          Connect this browser to the Continua debugger.
        </h2>
        <p className="mt-4 max-w-xl text-base leading-7 text-[var(--continua-text-secondary)]">
          Enter your Continua API key to unlock traces, sessions, and the full
          investigation workspace in this browser profile.
        </p>
        <form onSubmit={handleSubmit}>
          <input
            type="password"
            value={key}
            onChange={(e) => {
              setKey(e.target.value);
              setError('');
            }}
            placeholder="ck_live_..."
            className="app-input mt-8"
          />
          {error && (
            <p className="mt-3 text-sm text-red-600 dark:text-red-300">{error}</p>
          )}
          <button
            type="submit"
            className="app-button-primary mt-5 w-full"
          >
            Connect
          </button>
        </form>
      </div>
    </div>
  );
}
