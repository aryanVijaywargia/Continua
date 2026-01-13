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
    <div className="min-h-screen bg-gray-50 flex items-center justify-center">
      <div className="bg-white p-8 rounded-lg shadow-lg max-w-md w-full">
        <h2 className="text-2xl font-bold text-gray-900 mb-4">API Key Required</h2>
        <p className="text-gray-600 mb-6">
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
            className="w-full px-4 py-2 border border-gray-300 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-transparent"
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
