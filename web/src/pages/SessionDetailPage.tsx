import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link, useParams } from 'react-router-dom';
import {
  fetchSession,
  fetchTracesBySession,
  Trace,
} from '../api/client';
import { PaginationControls } from '../components/PaginationControls';
import { StatusBadge } from '../components/StatusBadge';
import { useRequireApiKey } from '../hooks/useRequireApiKey';
import {
  formatDuration,
  formatTokens,
  formatCost,
  formatRelativeTime,
  calculateDuration,
} from '../utils/format';

const PAGE_SIZE = 20;

/**
 * Session detail page showing session metadata and related traces.
 */
export function SessionDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { hasApiKey, prompt } = useRequireApiKey();
  const [offset, setOffset] = useState(0);

  if (!hasApiKey) {
    return prompt;
  }

  if (!id) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-red-600">Session ID is required</div>
      </div>
    );
  }

  return (
    <SessionDetailContent sessionId={id} offset={offset} setOffset={setOffset} />
  );
}

interface SessionDetailContentProps {
  sessionId: string;
  offset: number;
  setOffset: (offset: number) => void;
}

function SessionDetailContent({
  sessionId,
  offset,
  setOffset,
}: SessionDetailContentProps) {
  const {
    data: session,
    isLoading: isSessionLoading,
    error: sessionError,
  } = useQuery({
    queryKey: ['session', sessionId],
    queryFn: () => fetchSession(sessionId),
  });

  const {
    data: tracesData,
    isLoading: isTracesLoading,
    error: tracesError,
  } = useQuery({
    queryKey: ['session-traces', sessionId, offset],
    queryFn: () => fetchTracesBySession(sessionId, PAGE_SIZE, offset),
  });

  if (isSessionLoading) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-gray-500">Loading session...</div>
      </div>
    );
  }

  if (sessionError) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-red-600">
          Error loading session:{' '}
          {sessionError instanceof Error ? sessionError.message : 'Unknown error'}
        </div>
      </div>
    );
  }

  if (!session) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-gray-500">Session not found</div>
      </div>
    );
  }

  const traces = tracesData?.traces ?? [];
  const total = tracesData?.total ?? 0;

  return (
    <div className="min-h-screen bg-gray-50">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        {/* Back link */}
        <Link
          to="/sessions"
          className="text-blue-600 hover:text-blue-800 text-sm mb-4 inline-block"
        >
          &larr; Back to Sessions
        </Link>

        {/* Session Header */}
        <div className="bg-white rounded-lg shadow p-6 mb-6">
          <h1 className="text-2xl font-bold text-gray-900 mb-4">
            {session.name || 'Unnamed Session'}
          </h1>
          <dl className="grid grid-cols-2 md:grid-cols-4 gap-4">
            <div>
              <dt className="text-xs font-medium text-gray-500 uppercase">
                Session ID
              </dt>
              <dd className="mt-1 text-sm text-gray-900 font-mono">
                {session.id.slice(0, 12)}...
              </dd>
            </div>
            <div>
              <dt className="text-xs font-medium text-gray-500 uppercase">
                User ID
              </dt>
              <dd className="mt-1 text-sm text-gray-900">
                {session.user_id || '-'}
              </dd>
            </div>
            <div>
              <dt className="text-xs font-medium text-gray-500 uppercase">
                Trace Count
              </dt>
              <dd className="mt-1 text-sm text-gray-900">
                {session.trace_count ?? 0}
              </dd>
            </div>
            <div>
              <dt className="text-xs font-medium text-gray-500 uppercase">
                Created
              </dt>
              <dd className="mt-1 text-sm text-gray-900">
                {formatRelativeTime(session.created_at)}
              </dd>
            </div>
          </dl>
        </div>

        {/* Traces Section */}
        <div className="flex justify-between items-center mb-4">
          <h2 className="text-lg font-semibold text-gray-900">Traces</h2>
          <span className="text-sm text-gray-500">{total} traces</span>
        </div>

        {isTracesLoading ? (
          <div className="bg-white rounded-lg shadow p-8 text-center text-gray-500">
            Loading traces...
          </div>
        ) : tracesError ? (
          <div className="bg-white rounded-lg shadow p-8 text-center text-red-600">
            Error loading traces:{' '}
            {tracesError instanceof Error ? tracesError.message : 'Unknown error'}
          </div>
        ) : traces.length === 0 ? (
          <div className="bg-white rounded-lg shadow p-8 text-center text-gray-500">
            No traces in this session.
          </div>
        ) : (
          <>
            <div className="bg-white shadow overflow-hidden rounded-lg">
              <table className="min-w-full divide-y divide-gray-200">
                <thead className="bg-gray-50">
                  <tr>
                    <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                      Name
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                      Status
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                      Duration
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                      Tokens
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                      Cost
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider">
                      Started
                    </th>
                  </tr>
                </thead>
                <tbody className="bg-white divide-y divide-gray-200">
                  {traces.map((trace) => (
                    <TraceRow key={trace.id} trace={trace} />
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

interface TraceRowProps {
  trace: Trace;
}

function TraceRow({ trace }: TraceRowProps) {
  const duration = calculateDuration(trace.started_at, trace.ended_at);
  const totalTokens =
    (trace.total_tokens_in ?? 0) + (trace.total_tokens_out ?? 0);

  return (
    <tr className="hover:bg-gray-50">
      <td className="px-6 py-4 whitespace-nowrap">
        <Link
          to={`/traces/${trace.id}`}
          className="text-blue-600 hover:text-blue-800 font-medium"
        >
          {trace.name}
        </Link>
      </td>
      <td className="px-6 py-4 whitespace-nowrap">
        <StatusBadge status={trace.status} />
      </td>
      <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
        {formatDuration(duration)}
      </td>
      <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
        {formatTokens(totalTokens)}
      </td>
      <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-900">
        {formatCost(trace.total_cost_usd)}
      </td>
      <td className="px-6 py-4 whitespace-nowrap text-sm text-gray-500">
        {formatRelativeTime(trace.started_at)}
      </td>
    </tr>
  );
}
