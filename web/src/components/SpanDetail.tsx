import { type ReactNode } from 'react';
import { Span, TimelineEvent } from '../api/client';
import { CopyButton } from './CopyButton';
import { StatusBadge } from './StatusBadge';
import { JsonViewer } from './JsonViewer';
import {
  formatCost,
  formatDuration,
  formatTimestamp,
  formatTokens,
} from '../utils/format';
import type { BreadcrumbSegment } from '../utils/failureAnalysis';
import {
  formatInlineSemanticValue,
  getDecisionDetails,
} from '../utils/eventSemantics';
import { SpanBreadcrumb } from './SpanBreadcrumb';
import { TruncationBanner } from './TruncationBanner';

interface SpanDetailProps {
  span: Span | null;
  breadcrumbPath: BreadcrumbSegment[];
  onSelectSpan: (spanId: string) => void;
  spanIndex: ReadonlyMap<string, Span>;
  events?: TimelineEvent[];
}

/**
 * Panel showing detailed information about a selected span.
 */
export function SpanDetail({
  span,
  breadcrumbPath,
  onSelectSpan,
  spanIndex,
  events = [],
}: SpanDetailProps) {
  if (!span) {
    return (
      <div className="h-full flex items-center justify-center text-slate-500 dark:text-slate-400">
        Select a span to view details
      </div>
    );
  }

  const totalTokens = (span.tokens_in ?? 0) + (span.tokens_out ?? 0);
  const showLLMContext =
    span.kind === 'LLM' &&
    (span.model !== undefined || span.provider !== undefined);
  const parentSpan = span.parent_span_id
    ? spanIndex.get(span.parent_span_id) ?? null
    : null;
  const decisions = events.flatMap((event) => {
    if (event.span_id !== span.span_id) {
      return [];
    }

    const decision = getDecisionDetails(event);
    return decision ? [{ event, ...decision }] : [];
  });

  return (
    <div className="h-full overflow-y-auto p-4">
      {/* Header */}
      <div className="mb-4">
        <SpanBreadcrumb
          path={breadcrumbPath}
          onSelectSpan={onSelectSpan}
          className="mb-3"
        />
        <h2 className="text-lg font-semibold text-slate-900 dark:text-slate-100">{span.name}</h2>
        <div className="flex items-center gap-2 mt-1">
          <span className="text-sm text-slate-500 dark:text-slate-400">{span.kind}</span>
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
          <h3 className="text-sm font-medium text-slate-700 mb-2 dark:text-slate-200">Token Breakdown</h3>
          <div className="rounded bg-slate-50 p-3 text-sm dark:bg-slate-950/70">
            <div className="flex justify-between">
              <span className="text-slate-600 dark:text-slate-300">Input tokens:</span>
              <span className="font-mono">{span.tokens_in ?? 0}</span>
            </div>
            <div className="flex justify-between mt-1">
              <span className="text-slate-600 dark:text-slate-300">Output tokens:</span>
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
          <h3 className="mb-2 text-sm font-medium text-slate-700 dark:text-slate-200">LLM Context</h3>
          <div className="rounded bg-slate-50 p-3 text-sm dark:bg-slate-950/70">
            <DetailRow label="Model" value={span.model} />
            <DetailRow label="Provider" value={span.provider} className="mt-1" />
          </div>
        </div>
      )}

      {/* Input */}
      {span.input !== undefined && (
        <div className="mb-6">
          <h3 className="text-sm font-medium text-slate-700 mb-2 dark:text-slate-200">Input</h3>
          <TruncationBanner
            title="Input payload"
            truncated={span.input_truncated}
            originalSizeBytes={span.input_original_size_bytes}
            reason={span.input_truncation_reason}
          />
          <JsonViewer data={span.input} />
        </div>
      )}

      {/* Output */}
      {span.output !== undefined && (
        <div className="mb-6">
          <h3 className="text-sm font-medium text-slate-700 mb-2 dark:text-slate-200">Output</h3>
          <TruncationBanner
            title="Output payload"
            truncated={span.output_truncated}
            originalSizeBytes={span.output_original_size_bytes}
            reason={span.output_truncation_reason}
          />
          <JsonViewer data={span.output} />
        </div>
      )}

      {/* Metadata */}
      {span.metadata && Object.keys(span.metadata).length > 0 && (
        <div className="mb-6">
          <h3 className="text-sm font-medium text-slate-700 mb-2 dark:text-slate-200">Metadata</h3>
          <JsonViewer data={span.metadata} />
        </div>
      )}

      {decisions.length > 0 && (
        <div className="mb-6">
          <h3 className="text-sm font-medium text-slate-700 mb-2 dark:text-slate-200">Decisions</h3>
          <div className="space-y-3">
            {decisions.map((decision) => (
              <div
                key={decision.event.id}
                className="rounded border border-blue-100 bg-blue-50/70 p-3 dark:border-sky-500/30 dark:bg-sky-500/10"
              >
                <div className="text-sm font-semibold text-slate-900 dark:text-slate-100">
                  {decision.question}
                </div>
                <div className="mt-2 flex flex-wrap items-center gap-2 text-sm text-slate-600 dark:text-slate-300">
                  <span>Chosen</span>
                  <DecisionValuePill tone="accent">
                    {formatInlineSemanticValue(decision.chosen)}
                  </DecisionValuePill>
                </div>
                {decision.reasoning ? (
                  <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">{decision.reasoning}</p>
                ) : null}
                {decision.alternatives && decision.alternatives.length > 0 ? (
                  <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-slate-500 dark:text-slate-400">
                    <span>Alternatives</span>
                    {decision.alternatives.map((alternative, index) => (
                      <DecisionValuePill
                        key={`${decision.event.id}-alternative-${index}`}
                      >
                        {formatInlineSemanticValue(alternative)}
                      </DecisionValuePill>
                    ))}
                  </div>
                ) : null}
              </div>
            ))}
          </div>
        </div>
      )}

      {/* IDs */}
      <div className="mb-6">
          <h3 className="text-sm font-medium text-slate-700 mb-2 dark:text-slate-200">Identifiers</h3>
          <div className="rounded bg-slate-50 p-3 text-sm dark:bg-slate-950/70">
          <DetailRow label="Span UUID" value={span.id} mono />
          <DetailRow
            label="Span ID"
            value={span.span_id}
            mono
            className="mt-1"
            action={
              <CopyButton
                aria-label="Copy span ID"
                value={span.span_id}
              />
            }
          />
          <DetailRow label="Trace ID" value={span.trace_id} mono className="mt-1" />
          {span.parent_span_id && (
            <DetailRow
              label="Parent Span ID"
              value={
                parentSpan ? (
                  <button
                    type="button"
                    aria-label={`Select parent span ${span.parent_span_id}`}
                    className="rounded-full border border-slate-200 bg-white px-2.5 py-1 text-xs font-medium text-slate-700 transition hover:bg-slate-100 focus:outline-none focus:ring-2 focus:ring-blue-200 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200 dark:hover:bg-slate-800"
                    onClick={() => onSelectSpan(parentSpan.span_id)}
                  >
                    {span.parent_span_id}
                  </button>
                ) : (
                  span.parent_span_id
                )
              }
              mono
              className="mt-1"
              action={
                <CopyButton
                  aria-label="Copy parent span ID"
                  value={span.parent_span_id}
                />
              }
            />
          )}
        </div>
      </div>

      {/* Timestamps */}
      <div>
        <h3 className="text-sm font-medium text-slate-700 mb-2 dark:text-slate-200">Timestamps</h3>
        <div className="rounded bg-slate-50 p-3 text-sm dark:bg-slate-950/70">
          <DetailRow
            label="Started"
            value={formatTimestamp(span.started_at)}
            mono
          />
          {span.ended_at && (
            <DetailRow
              label="Ended"
              value={formatTimestamp(span.ended_at)}
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
    <div className="rounded p-3 text-center bg-slate-50 dark:bg-slate-950/70">
      <div className="text-xl font-semibold text-slate-900 dark:text-slate-100">{value}</div>
      <div className="text-xs text-slate-500 mt-1 dark:text-slate-400">{label}</div>
    </div>
  );
}

function DecisionValuePill({
  children,
  tone = 'neutral',
}: {
  children: string;
  tone?: 'neutral' | 'accent';
}) {
  return (
    <span
      className={`rounded-full border px-2.5 py-1 text-xs font-medium ${
        tone === 'accent'
          ? 'border-blue-200 bg-white text-blue-800 dark:border-sky-500/40 dark:bg-slate-950 dark:text-sky-200'
          : 'border-slate-200 bg-white text-slate-700 dark:border-slate-700 dark:bg-slate-950 dark:text-slate-200'
      }`}
    >
      {children}
    </span>
  );
}

interface DetailRowProps {
  label: string;
  value?: ReactNode;
  mono?: boolean;
  className?: string;
  action?: ReactNode;
}

function DetailRow({
  label,
  value,
  mono = false,
  className = '',
  action,
}: DetailRowProps) {
  const hasValue = value !== undefined && value !== null && value !== '';

  return (
    <div className={`flex justify-between gap-4 ${className}`.trim()}>
      <span className="text-slate-600 dark:text-slate-300">{label}:</span>
      {hasValue ? (
        <span className="flex min-w-0 items-center justify-end gap-2">
          <span className={mono ? 'font-mono text-xs text-right' : 'text-right'}>
            {value}
          </span>
          {action}
        </span>
      ) : (
        <span className="text-slate-400 dark:text-slate-500">-</span>
      )}
    </div>
  );
}
