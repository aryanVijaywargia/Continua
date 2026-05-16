import { type ReactNode } from 'react';
import { Span, TimelineEvent } from '../api/client';
import { CopyButton } from './CopyButton';
import { DecisionValuePill } from './DecisionValuePill';
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
import {
  getAccessibleSummary,
  getReasonExplanation,
  type RetrySafetyAssessment,
} from '../utils/retrySafety';
import { RetrySafetyBadge } from './RetrySafetyBadge';
import { SpanBreadcrumb } from './SpanBreadcrumb';
import { TruncationBanner } from './TruncationBanner';

interface SpanDetailProps {
  span: Span | null;
  breadcrumbPath: BreadcrumbSegment[];
  onSelectSpan: (spanId: string) => void;
  spanIndex: ReadonlyMap<string, Span>;
  events?: TimelineEvent[];
  retrySafety?: RetrySafetyAssessment | null;
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
  retrySafety = null,
}: SpanDetailProps) {
  if (!span) {
    return (
      <div className="flex h-full items-center justify-center text-[var(--c-text-muted)]">
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
    <div className="h-full overflow-y-auto bg-[var(--c-app-bg)] p-4">
      {/* Header */}
      <div className="mb-4">
        <SpanBreadcrumb
          path={breadcrumbPath}
          onSelectSpan={onSelectSpan}
          className="mb-3"
        />
        <h2 className="text-lg font-semibold text-[var(--c-text-primary)]">
          {span.name}
        </h2>
        <div className="mt-2 flex items-center gap-2">
          <span className="rounded border border-[var(--c-border)] bg-[var(--c-surface)] px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.08em] text-[var(--c-text-secondary)]">
            {span.kind}
          </span>
          <StatusBadge status={span.status} />
        </div>
      </div>

      {/* Metrics */}
      <div className="mb-6 grid grid-cols-1 gap-3 sm:grid-cols-3">
        <MetricCard label="Duration" value={formatDuration(span.latency_ms)} />
        <MetricCard label="Tokens" value={formatTokens(totalTokens)} />
        <MetricCard label="Cost" value={formatCost(span.cost_usd)} />
      </div>

      {/* Token breakdown */}
      {(span.tokens_in || span.tokens_out) && (
        <div className="mb-6">
          <h3 className="mb-2 text-sm font-medium text-[var(--c-text-secondary)]">Token Breakdown</h3>
          <div className="app-surface-muted p-3 text-sm">
            <div className="flex justify-between">
              <span className="text-[var(--c-text-secondary)]">Input tokens:</span>
              <span className="font-mono">{span.tokens_in ?? 0}</span>
            </div>
            <div className="flex justify-between mt-1">
              <span className="text-[var(--c-text-secondary)]">Output tokens:</span>
              <span className="font-mono">{span.tokens_out ?? 0}</span>
            </div>
          </div>
        </div>
      )}

      {/* Error message */}
      {span.error_message && (
        <div className="mb-6">
          <h3 className="mb-2 text-sm font-medium text-[var(--c-red-text)]">Error</h3>
          <div className="whitespace-pre-wrap rounded border border-[var(--c-red-border)] bg-[var(--c-red-faint)] p-3 font-mono text-sm text-[var(--c-red-text)]">
            {span.error_message}
          </div>
        </div>
      )}

      {/* LLM context */}
      {showLLMContext && (
        <div className="mb-6">
          <h3 className="mb-2 text-sm font-medium text-[var(--c-text-secondary)]">LLM Context</h3>
          <div className="app-surface-muted p-3 text-sm">
            <DetailRow label="Model" value={span.model} />
            <DetailRow label="Provider" value={span.provider} className="mt-1" />
          </div>
        </div>
      )}

      {/* Input */}
      {span.input !== undefined && (
        <div className="mb-6">
          <h3 className="mb-2 text-sm font-medium text-[var(--c-text-secondary)]">Input</h3>
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
          <h3 className="mb-2 text-sm font-medium text-[var(--c-text-secondary)]">Output</h3>
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
          <h3 className="mb-2 text-sm font-medium text-[var(--c-text-secondary)]">Metadata</h3>
          <JsonViewer data={span.metadata} />
        </div>
      )}

      {decisions.length > 0 && (
        <div className="mb-6">
          <h3 className="mb-2 text-sm font-medium text-[var(--c-text-secondary)]">Decisions</h3>
          <div className="space-y-3">
            {decisions.map((decision) => (
              <div
                key={decision.event.id}
                className="rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] p-3"
              >
                <div className="text-sm font-semibold text-[var(--c-text-primary)]">
                  {decision.question}
                </div>
                <div className="mt-2 flex flex-wrap items-center gap-2 text-sm text-[var(--c-text-secondary)]">
                  <span>Chosen</span>
                  <DecisionValuePill tone="accent">
                    {formatInlineSemanticValue(decision.chosen)}
                  </DecisionValuePill>
                </div>
                {decision.reasoning ? (
                  <p className="mt-2 text-sm text-[var(--c-text-secondary)]">{decision.reasoning}</p>
                ) : null}
                {decision.alternatives && decision.alternatives.length > 0 ? (
                  <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-[var(--c-text-muted)]">
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

      {span.status === 'FAILED' && retrySafety ? (
        <div className="mb-6">
          <h3 className="mb-2 text-sm font-medium text-[var(--c-text-secondary)]">
            Retry Safety
          </h3>
          <div className="app-surface-muted p-4">
            <div className="flex flex-wrap items-center gap-3">
              <RetrySafetyBadge
                classification={retrySafety.classification}
                variant="full"
                aria-label={getAccessibleSummary(retrySafety.classification)}
              />
              <span className="text-sm text-[var(--c-text-secondary)]">
                Advisory only. Retry safety is inferred from recorded effect metadata.
              </span>
            </div>
            <p className="mt-3 text-sm text-[var(--c-text-secondary)]">
              {getReasonExplanation(retrySafety.reason)}
            </p>
            {(retrySafety.effectKind !== undefined ||
              retrySafety.hasExternalSideEffect !== undefined ||
              retrySafety.idempotent !== undefined ||
              retrySafety.idempotencyKey !== undefined) ? (
              <div className="mt-4 rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] p-3 text-sm">
                <DetailRow label="effect_kind" value={retrySafety.effectKind} mono />
                {retrySafety.hasExternalSideEffect !== undefined ? (
                  <DetailRow
                    label="has_external_side_effect"
                    value={formatInlineSemanticValue(retrySafety.hasExternalSideEffect)}
                    mono
                    className="mt-1"
                  />
                ) : null}
                {retrySafety.idempotent !== undefined ? (
                  <DetailRow
                    label="idempotent"
                    value={formatInlineSemanticValue(retrySafety.idempotent)}
                    mono
                    className="mt-1"
                  />
                ) : null}
                {retrySafety.idempotencyKey !== undefined ? (
                  <DetailRow
                    label="idempotency_key"
                    value={retrySafety.idempotencyKey}
                    mono
                    className="mt-1"
                  />
                ) : null}
              </div>
            ) : null}
          </div>
        </div>
      ) : null}

      {/* IDs */}
      <div className="mb-6">
          <h3 className="mb-2 text-sm font-medium text-[var(--c-text-secondary)]">Identifiers</h3>
          <div className="app-surface-muted p-3 text-sm">
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
                    className="rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-2.5 py-1 text-xs font-medium text-[var(--c-text-secondary)] transition hover:border-[var(--c-border-strong)] hover:text-[var(--c-accent-text)] focus:outline-none focus:ring-2 focus:ring-[var(--c-accent-faint)]"
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
        <h3 className="mb-2 text-sm font-medium text-[var(--c-text-secondary)]">Timestamps</h3>
        <div className="app-surface-muted p-3 text-sm text-[var(--c-text-primary)]">
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
    <div className="app-surface-muted p-3 text-center">
      <div className="text-lg font-semibold text-[var(--c-text-primary)]">{value}</div>
      <div className="mt-1 text-xs text-[var(--c-text-muted)]">{label}</div>
    </div>
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
      <span className="text-[var(--c-text-secondary)]">{label}:</span>
      {hasValue ? (
        <span className="flex min-w-0 items-center justify-end gap-2">
          <span className={mono ? 'text-right font-mono text-xs text-[var(--c-text-primary)]' : 'text-right text-[var(--c-text-primary)]'}>
            {value}
          </span>
          {action}
        </span>
      ) : (
        <span className="text-[var(--c-text-muted)]">-</span>
      )}
    </div>
  );
}
