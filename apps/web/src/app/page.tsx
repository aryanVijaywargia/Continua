'use client';

import { useEffect, useState } from 'react';
import { ContinuaClient, type HealthResponse } from '@continua/api-client';

export default function Home() {
  const [health, setHealth] = useState<HealthResponse | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const client = new ContinuaClient({
      baseUrl: process.env.NEXT_PUBLIC_CONTINUA_API_URL || 'http://localhost:8243',
    });

    client
      .health()
      .then(setHealth)
      .catch((err) => setError(err.message));
  }, []);

  return (
    <main className="min-h-screen p-8">
      <div className="max-w-4xl mx-auto">
        <h1 className="text-4xl font-bold mb-8">Continua</h1>
        <p className="text-gray-400 mb-8">
          Durable execution platform for AI agents
        </p>

        <div className="bg-gray-800 rounded-lg p-6 mb-8">
          <h2 className="text-xl font-semibold mb-4">Server Status</h2>
          {error ? (
            <div className="text-red-400">
              <span className="font-mono">Error:</span> {error}
            </div>
          ) : health ? (
            <div className="space-y-2">
              <div className="flex items-center gap-2">
                <span className="w-3 h-3 bg-green-500 rounded-full"></span>
                <span className="text-green-400 font-semibold">
                  {health.status.toUpperCase()}
                </span>
              </div>
              <div className="grid grid-cols-2 gap-4 mt-4 text-sm">
                <div>
                  <span className="text-gray-500">Version:</span>{' '}
                  <span className="font-mono">{health.version}</span>
                </div>
                <div>
                  <span className="text-gray-500">Commit:</span>{' '}
                  <span className="font-mono">{health.commit}</span>
                </div>
                <div className="col-span-2">
                  <span className="text-gray-500">Build Time:</span>{' '}
                  <span className="font-mono">{health.build_time}</span>
                </div>
              </div>
            </div>
          ) : (
            <div className="text-gray-500">Loading...</div>
          )}
        </div>

        <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
          <a
            href="/executions"
            className="bg-gray-800 rounded-lg p-6 hover:bg-gray-750 transition-colors"
          >
            <h3 className="text-lg font-semibold mb-2">Executions</h3>
            <p className="text-gray-400 text-sm">
              View and debug agent executions
            </p>
          </a>
          <a
            href="/settings"
            className="bg-gray-800 rounded-lg p-6 hover:bg-gray-750 transition-colors"
          >
            <h3 className="text-lg font-semibold mb-2">Settings</h3>
            <p className="text-gray-400 text-sm">Configure your environment</p>
          </a>
        </div>
      </div>
    </main>
  );
}
