import { Span } from '../api/client';
import { StatusBadge } from './StatusBadge';
import { formatDuration, formatTokens, formatCost } from '../utils/format';

interface SpanDetailProps {
  span: Span | null;
}

/**
 * Panel showing detailed information about a selected span.
 */
export function SpanDetail({ span }: SpanDetailProps) {
  if (!span) {
    return (
      <div className="h-full flex items-center justify-center text-gray-500">
        Select a span to view details
      </div>
    );
  }

  const totalTokens = (span.tokens_in ?? 0) + (span.tokens_out ?? 0);

  return (
    <div className="h-full overflow-y-auto p-4">
      {/* Header */}
      <div className="mb-4">
        <h2 className="text-lg font-semibold text-gray-900">{span.name}</h2>
        <div className="flex items-center gap-2 mt-1">
          <span className="text-sm text-gray-500">{span.kind}</span>
          <StatusBadge status={span.status} />
        </div>
      </div>

      {/* Metrics */}
      <div className="grid grid-cols-3 gap-4 mb-6">
        <MetricCard label="Duration" value={formatDuration(span.latency_ms)} />
        <MetricCard label="Tokens" value={formatTokens(totalTokens)} />
        <MetricCard label="Cost" value={formatCost(span.cost_usd)} />
      </div>

      {/* Token breakdown */}
      {(span.tokens_in || span.tokens_out) && (
        <div className="mb-6">
          <h3 className="text-sm font-medium text-gray-700 mb-2">Token Breakdown</h3>
          <div className="bg-gray-50 rounded p-3 text-sm">
            <div className="flex justify-between">
              <span className="text-gray-600">Input tokens:</span>
              <span className="font-mono">{span.tokens_in ?? 0}</span>
            </div>
            <div className="flex justify-between mt-1">
              <span className="text-gray-600">Output tokens:</span>
              <span className="font-mono">{span.tokens_out ?? 0}</span>
            </div>
          </div>
        </div>
      )}

      {/* Error message */}
      {span.error_message && (
        <div className="mb-6">
          <h3 className="text-sm font-medium text-red-700 mb-2">Error</h3>
          <div className="bg-red-50 border border-red-200 rounded p-3 text-sm text-red-800 font-mono whitespace-pre-wrap">
            {span.error_message}
          </div>
        </div>
      )}

      {/* Input */}
      {span.input && (
        <div className="mb-6">
          <h3 className="text-sm font-medium text-gray-700 mb-2">Input</h3>
          <JsonViewer data={span.input} />
        </div>
      )}

      {/* Output */}
      {span.output && (
        <div className="mb-6">
          <h3 className="text-sm font-medium text-gray-700 mb-2">Output</h3>
          <JsonViewer data={span.output} />
        </div>
      )}

      {/* Metadata */}
      {span.metadata && Object.keys(span.metadata).length > 0 && (
        <div className="mb-6">
          <h3 className="text-sm font-medium text-gray-700 mb-2">Metadata</h3>
          <JsonViewer data={span.metadata} />
        </div>
      )}

      {/* IDs */}
      <div className="mb-6">
        <h3 className="text-sm font-medium text-gray-700 mb-2">Identifiers</h3>
        <div className="bg-gray-50 rounded p-3 text-sm">
          <div className="flex justify-between mb-1">
            <span className="text-gray-600">Span ID:</span>
            <span className="font-mono text-xs">{span.id}</span>
          </div>
          <div className="flex justify-between mb-1">
            <span className="text-gray-600">Trace ID:</span>
            <span className="font-mono text-xs">{span.trace_id}</span>
          </div>
          {span.parent_span_id && (
            <div className="flex justify-between">
              <span className="text-gray-600">Parent Span ID:</span>
              <span className="font-mono text-xs">{span.parent_span_id}</span>
            </div>
          )}
        </div>
      </div>

      {/* Timestamps */}
      <div>
        <h3 className="text-sm font-medium text-gray-700 mb-2">Timestamps</h3>
        <div className="bg-gray-50 rounded p-3 text-sm">
          <div className="flex justify-between mb-1">
            <span className="text-gray-600">Started:</span>
            <span className="font-mono text-xs">
              {new Date(span.started_at).toISOString()}
            </span>
          </div>
          {span.ended_at && (
            <div className="flex justify-between">
              <span className="text-gray-600">Ended:</span>
              <span className="font-mono text-xs">
                {new Date(span.ended_at).toISOString()}
              </span>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

interface MetricCardProps {
  label: string;
  value: string;
}

function MetricCard({ label, value }: MetricCardProps) {
  return (
    <div className="bg-gray-50 rounded p-3 text-center">
      <div className="text-xl font-semibold text-gray-900">{value}</div>
      <div className="text-xs text-gray-500 mt-1">{label}</div>
    </div>
  );
}

interface JsonViewerProps {
  data: Record<string, unknown>;
}

function JsonViewer({ data }: JsonViewerProps) {
  return (
    <pre className="bg-gray-50 border rounded p-3 text-xs font-mono overflow-x-auto max-h-64 overflow-y-auto">
      {JSON.stringify(data, null, 2)}
    </pre>
  );
}
