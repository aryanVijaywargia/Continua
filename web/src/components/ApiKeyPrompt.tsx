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
    <div className="min-h-screen bg-slate-50 flex items-center justify-center dark:bg-slate-950">
      <div className="w-full max-w-md rounded-lg bg-white p-8 shadow-lg dark:bg-slate-900">
        <h2 className="mb-4 text-2xl font-bold text-slate-900 dark:text-slate-100">API Key Required</h2>
        <p className="mb-6 text-slate-600 dark:text-slate-300">
          Enter your Continua API key to view traces.
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
            className="w-full rounded-lg border border-slate-300 bg-white px-4 py-2 text-slate-900 focus:border-transparent focus:ring-2 focus:ring-blue-500 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-100"
          />
          {error && (
            <p className="mt-2 text-red-600 text-sm">{error}</p>
          )}
          <button
            type="submit"
            className="mt-4 w-full bg-blue-600 text-white py-2 px-4 rounded-lg hover:bg-blue-700 transition-colors"
          >
            Connect
          </button>
        </form>
      </div>
    </div>
  );
}
