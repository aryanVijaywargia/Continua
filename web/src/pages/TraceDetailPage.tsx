import { useState } from 'react';
import { useParams, Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { getApiKey, fetchTrace, fetchSpans, Span } from '../api/client';
import { ApiKeyPrompt } from '../components/ApiKeyPrompt';
import { StatusBadge } from '../components/StatusBadge';
import { SpanTree } from '../components/SpanTree';
import { SpanDetail } from '../components/SpanDetail';
import {
  formatDuration,
  formatTokens,
  formatCost,
  calculateDuration,
} from '../utils/format';

/**
 * Trace detail page with span tree and detail panel.
 */
export function TraceDetailPage() {
  const { id } = useParams<{ id: string }>();
  const [hasApiKey, setHasApiKey] = useState(() => !!getApiKey());
  const [selectedSpan, setSelectedSpan] = useState<Span | null>(null);

  if (!hasApiKey) {
    return <ApiKeyPrompt onSubmit={() => setHasApiKey(true)} />;
  }

  if (!id) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-red-600">Trace ID is required</div>
      </div>
    );
  }

  return (
    <TraceDetailContent
      traceId={id}
      selectedSpan={selectedSpan}
      onSelectSpan={setSelectedSpan}
    />
  );
}

interface TraceDetailContentProps {
  traceId: string;
  selectedSpan: Span | null;
  onSelectSpan: (span: Span | null) => void;
}

function TraceDetailContent({
  traceId,
  selectedSpan,
  onSelectSpan,
}: TraceDetailContentProps) {
  const traceQuery = useQuery({
    queryKey: ['trace', traceId],
    queryFn: () => fetchTrace(traceId),
  });

  const spansQuery = useQuery({
    queryKey: ['spans', traceId],
    queryFn: () => fetchSpans(traceId),
  });

  if (traceQuery.isLoading || spansQuery.isLoading) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-gray-500">Loading trace...</div>
      </div>
    );
  }

  if (traceQuery.error) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-red-600">
          Error loading trace:{' '}
          {traceQuery.error instanceof Error
            ? traceQuery.error.message
            : 'Unknown error'}
        </div>
      </div>
    );
  }

  const trace = traceQuery.data;
  const spans = spansQuery.data?.spans ?? [];

  if (!trace) {
    return (
      <div className="min-h-screen bg-gray-50 flex items-center justify-center">
        <div className="text-red-600">Trace not found</div>
      </div>
    );
  }

  const duration = calculateDuration(trace.started_at, trace.ended_at);
  const totalTokens =
    (trace.total_tokens_in ?? 0) + (trace.total_tokens_out ?? 0);

  return (
    <div className="min-h-screen bg-gray-50 flex flex-col">
      {/* Header */}
      <header className="bg-white border-b px-6 py-4">
        <div className="flex items-center gap-4">
          <Link
            to="/traces"
            className="text-gray-500 hover:text-gray-700"
          >
            ← Traces
          </Link>
          <div className="flex-1">
            <h1 className="text-xl font-semibold text-gray-900">{trace.name}</h1>
            <div className="flex items-center gap-4 mt-1 text-sm text-gray-500">
              <StatusBadge status={trace.status} />
              <span>{formatDuration(duration)}</span>
              <span>{formatTokens(totalTokens)} tokens</span>
              <span>{formatCost(trace.total_cost_usd)}</span>
              {trace.error_count && trace.error_count > 0 && (
                <span className="text-red-600">{trace.error_count} errors</span>
              )}
            </div>
          </div>
        </div>
      </header>

      {/* Main content - two panel layout */}
      <div className="flex-1 flex overflow-hidden">
        {/* Left panel - span tree */}
        <div className="w-1/2 border-r bg-white overflow-y-auto">
          <div className="px-4 py-2 border-b bg-gray-50">
            <h2 className="text-sm font-medium text-gray-700">
              Spans ({spans.length})
            </h2>
          </div>
          <SpanTree
            spans={spans}
            selectedSpanId={selectedSpan?.id ?? null}
            onSelectSpan={onSelectSpan}
          />
        </div>

        {/* Right panel - span detail */}
        <div className="w-1/2 bg-white overflow-hidden">
          <SpanDetail span={selectedSpan} />
        </div>
      </div>
    </div>
  );
}
