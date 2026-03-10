import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { fetchSessions, Session } from '../api/client';
import { PaginationControls } from '../components/PaginationControls';
import { useRequireApiKey } from '../hooks/useRequireApiKey';
import { formatRelativeTime } from '../utils/format';

const PAGE_SIZE = 20;

/**
 * Sessions list page with pagination.
 */
export function SessionsPage() {
  const { hasApiKey, prompt } = useRequireApiKey();
  const [offset, setOffset] = useState(0);

  if (!hasApiKey) {
    return prompt;
  }

  return <SessionsContent offset={offset} setOffset={setOffset} />;
}

interface SessionsContentProps {
  offset: number;
  setOffset: (offset: number) => void;
}

function SessionsContent({ offset, setOffset }: SessionsContentProps) {
  const { data, isLoading, error } = useQuery({
    queryKey: ['sessions', offset],
    queryFn: () => fetchSessions(PAGE_SIZE, offset),
  });

  if (isLoading) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-gray-500">Loading sessions...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-red-600">
          Error loading sessions: {error instanceof Error ? error.message : 'Unknown error'}
        </div>
      </div>
    );
  }

  const sessions = data?.sessions ?? [];
  const total = data?.total ?? 0;

  return (
    <div className="min-h-screen bg-gray-50">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        <div className="flex justify-between items-center mb-6">
          <h1 className="text-2xl font-bold text-gray-900">Sessions</h1>
          <span className="text-sm text-gray-500">{total} total</span>
        </div>

        {sessions.length === 0 ? (
          <div className="bg-white rounded-lg shadow p-8 text-center text-gray-500">
            No sessions found. Sessions are created when traces include a session_id.
          </div>
        ) : (
          <>
            <div className="bg-white shadow overflow-hidden rounded-lg">
              <table className="min-w-full divide-y divide-gray-200">
                <thead className="bg-gray-50">
                  <tr>
                    <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                      Session ID
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                      Name
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                      User ID
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                      Traces
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                      Created
                    </th>
                  </tr>
                </thead>
                <tbody className="bg-white divide-y divide-gray-200">
                  {sessions.map((session) => (
                    <SessionRow key={session.id} session={session} />
                  ))}
                </tbody>
              </table>
            </div>
            <PaginationControls
              offset={offset}
              pageSize={PAGE_SIZE}
              total={total}
              onOffsetChange={setOffset}
            />
          </>
        )}
      </div>
    </div>
  );
}

interface SessionRowProps {
  session: Session;
}

function SessionRow({ session }: SessionRowProps) {
  return (
    <tr className="hover:bg-gray-50">
      <td className="px-6 py-4 whitespace-nowrap">
        <Link
          to={`/sessions/${session.id}`}
          className="text-blue-600 hover:text-blue-800 font-mono text-sm"
        >
          {session.id.slice(0, 8)}...
        </Link>
      </td>
      <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
        {session.name || '-'}
      </td>
      <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
        {session.user_id || '-'}
      </td>
      <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
        {session.trace_count ?? 0}
      </td>
      <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
        {formatRelativeTime(session.created_at)}
      </td>
    </tr>
  );
}
