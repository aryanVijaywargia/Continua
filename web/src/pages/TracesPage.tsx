import { useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { fetchTraces, Trace } from '../api/client';
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
 * Traces list page with pagination.
 */
export function TracesPage() {
  const { hasApiKey, prompt } = useRequireApiKey();
  const [offset, setOffset] = useState(0);

  // Show API key prompt if not configured
  if (!hasApiKey) {
    return prompt;
  }

  return <TracesContent offset={offset} setOffset={setOffset} />;
}

interface TracesContentProps {
  offset: number;
  setOffset: (offset: number) => void;
}

function TracesContent({ offset, setOffset }: TracesContentProps) {
  const { data, isLoading, error } = useQuery({
    queryKey: ['traces', offset],
    queryFn: () => fetchTraces(PAGE_SIZE, offset),
  });

  if (isLoading) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-gray-500">Loading traces...</div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-red-600">
          Error loading traces: {error instanceof Error ? error.message : 'Unknown error'}
        </div>
      </div>
    );
  }

  const traces = data?.traces ?? [];
  const total = data?.total ?? 0;

  return (
    <div className="min-h-screen bg-gray-50">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        <div className="flex justify-between items-center mb-6">
          <h1 className="text-2xl font-bold text-gray-900">Traces</h1>
          <span className="text-sm text-gray-500">{total} total</span>
        </div>

        {traces.length === 0 ? (
          <div className="bg-white rounded-lg shadow p-8 text-center text-gray-500">
            No traces found. Start sending traces from your application.
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
        {trace.session_id && (
          <p className="text-xs text-gray-400 mt-1">
            Session: {trace.session_id.slice(0, 8)}...
          </p>
        )}
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
