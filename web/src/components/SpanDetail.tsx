import { type ReactNode } from 'react';
import { Span } from '../api/client';
import { StatusBadge } from './StatusBadge';
import { JsonViewer } from './JsonViewer';
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
  const showLLMContext =
    span.kind === 'LLM' &&
    (span.model !== undefined || span.provider !== undefined);

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

      {/* LLM context */}
      {showLLMContext && (
        <div className="mb-6">
          <h3 className="mb-2 text-sm font-medium text-gray-700">LLM Context</h3>
          <div className="rounded bg-gray-50 p-3 text-sm">
            <DetailRow label="Model" value={span.model} />
            <DetailRow label="Provider" value={span.provider} className="mt-1" />
          </div>
        </div>
      )}

      {/* Input */}
      {span.input !== undefined && (
        <div className="mb-6">
          <h3 className="text-sm font-medium text-gray-700 mb-2">Input</h3>
          <JsonViewer data={span.input} />
        </div>
      )}

      {/* Output */}
      {span.output !== undefined && (
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
          <DetailRow label="Span ID" value={span.id} mono />
          <DetailRow label="Trace ID" value={span.trace_id} mono className="mt-1" />
          {span.parent_span_id && (
            <DetailRow
              label="Parent Span ID"
              value={span.parent_span_id}
              mono
              className="mt-1"
            />
          )}
        </div>
      </div>

      {/* Timestamps */}
      <div>
        <h3 className="text-sm font-medium text-gray-700 mb-2">Timestamps</h3>
        <div className="bg-gray-50 rounded p-3 text-sm">
          <DetailRow
            label="Started"
            value={new Date(span.started_at).toISOString()}
            mono
          />
          {span.ended_at && (
            <DetailRow
              label="Ended"
              value={new Date(span.ended_at).toISOString()}
              mono
              className="mt-1"
            />
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

interface DetailRowProps {
  label: string;
  value?: ReactNode;
  mono?: boolean;
  className?: string;
}

function DetailRow({ label, value, mono = false, className = '' }: DetailRowProps) {
  const hasValue = value !== undefined && value !== null && value !== '';

  return (
    <div className={`flex justify-between gap-4 ${className}`.trim()}>
      <span className="text-gray-600">{label}:</span>
      {hasValue ? (
        <span className={mono ? 'font-mono text-xs text-right' : 'text-right'}>
          {value}
        </span>
      ) : (
        <span className="text-gray-400">-</span>
      )}
    </div>
  );
}
