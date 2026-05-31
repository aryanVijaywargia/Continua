import { useEffect, useRef } from 'react';
import { formatCost, formatDuration, formatTokens } from '../utils/format';
import {
  getAccessibleSummary,
  type RetrySafetyAssessment,
} from '../utils/retrySafety';
import type { SpanTreeRow } from '../utils/spanTree';
import { RetrySafetyBadge } from './RetrySafetyBadge';
import { StatusBadge } from './StatusBadge';

interface SpanTreeProps {
  rows: SpanTreeRow[];
  expandedSpanIds: ReadonlySet<string>;
  selectedSpanId: string | null;
  onSelectSpan: (spanId: string) => void;
  onToggleExpand: (spanId: string) => void;
  failedSpanIds: ReadonlySet<string>;
  primaryAncestorPath: ReadonlySet<string>;
  revealPath: ReadonlySet<string>;
  inlineErrorPreviews: ReadonlyMap<string, string>;
  showMetrics?: boolean;
  matchedSpanIds?: ReadonlySet<string> | null;
  spanAssessments?: ReadonlyMap<string, RetrySafetyAssessment>;
}

const kindColors: Record<string, string> = {
  LLM: 'bg-purple-100 text-purple-800 dark:bg-purple-500/15 dark:text-purple-200',
  TOOL: 'bg-yellow-100 text-yellow-800 dark:bg-yellow-500/15 dark:text-yellow-200',
  CHAIN: 'bg-blue-100 text-blue-800 dark:bg-blue-500/15 dark:text-blue-200',
  AGENT: 'bg-green-100 text-green-800 dark:bg-green-500/15 dark:text-green-200',
  CUSTOM: 'bg-gray-100 text-gray-800 dark:bg-[rgba(35,38,48,0.95)] dark:text-[var(--continua-text-secondary)]',
};
const EMPTY_SPAN_ASSESSMENTS = new Map<string, RetrySafetyAssessment>();

export function SpanTree({
  rows,
  expandedSpanIds,
  selectedSpanId,
  onSelectSpan,
  onToggleExpand,
  failedSpanIds,
  primaryAncestorPath,
  revealPath,
  inlineErrorPreviews,
  showMetrics = false,
  matchedSpanIds = null,
  spanAssessments = EMPTY_SPAN_ASSESSMENTS,
}: SpanTreeProps) {
  const rowRefs = useRef(new Map<string, HTMLDivElement>());

  useEffect(() => {
    const targetSpanId =
      selectedSpanId && revealPath.has(selectedSpanId) ? selectedSpanId : null;
    if (!targetSpanId) {
      return;
    }

    rowRefs.current.get(targetSpanId)?.scrollIntoView?.({ block: 'nearest' });
  }, [revealPath, selectedSpanId]);

  if (rows.length === 0) {
    return (
      <div className="p-4 text-center text-[var(--continua-text-muted)]">
        No spans found for this trace.
      </div>
    );
  }

  return (
    <div className="py-2">
      {rows.map((row) => {
        const { span } = row;
        const hasChildren = row.hasChildren;
        const isExpanded = expandedSpanIds.has(span.span_id);
        const isSelected = span.span_id === selectedSpanId;
        const isFailed = failedSpanIds.has(span.span_id);
        const isOnPrimaryPath = primaryAncestorPath.has(span.span_id);
        const errorPreview = inlineErrorPreviews.get(span.span_id);
        const rowStateId = `span-row-state-${span.id}`;
        const isMatch = matchedSpanIds?.has(span.span_id) ?? false;
        const shouldDim = matchedSpanIds !== null && !isMatch;
        const retrySafety = spanAssessments.get(span.span_id) ?? null;

        const rowClasses = isSelected
          ? 'border-[var(--continua-accent)] bg-[var(--continua-accent-faint)] shadow-sm ring-1 ring-[var(--continua-accent-faint)]'
          : isOnPrimaryPath
            ? 'border-amber-300 bg-amber-50 dark:border-amber-500/40 dark:bg-amber-500/10'
            : isFailed
              ? 'border-red-200 bg-red-50/80 dark:border-red-500/40 dark:bg-red-500/10'
              : 'border-transparent bg-[var(--continua-surface-elevated)] hover:bg-[var(--continua-surface-muted)]';

        return (
          <div
            key={span.id}
            ref={(element) => {
              if (element) {
                rowRefs.current.set(span.span_id, element);
                return;
              }

              rowRefs.current.delete(span.span_id);
            }}
          >
            <div
              className={`flex items-start gap-1 px-2 py-1 transition-opacity ${
                shouldDim ? 'opacity-45' : 'opacity-100'
              }`}
              style={{ paddingLeft: `${row.depth * 20 + 8}px` }}
            >
              {hasChildren ? (
                <button
                  type="button"
                  className="mt-3 flex h-6 w-6 shrink-0 items-center justify-center rounded text-[var(--continua-text-muted)] transition hover:bg-[var(--continua-surface-muted)] hover:text-[var(--continua-text-secondary)] focus:outline-none focus:ring-2 focus:ring-[var(--continua-accent)]"
                  aria-label={`${isExpanded ? 'Collapse' : 'Expand'} span ${span.name}`}
                  onClick={() => onToggleExpand(span.span_id)}
                >
                  {isExpanded ? '▼' : '▶'}
                </button>
              ) : (
                <span
                  className="mt-3 flex h-6 w-6 shrink-0 items-center justify-center text-[var(--continua-text-muted)]"
                  aria-hidden="true"
                >
                  ·
                </span>
              )}

              <button
                type="button"
                className={`flex min-w-0 flex-1 items-start justify-between rounded-xl border px-3 py-2 text-left transition ${rowClasses}`}
                aria-label={`Select span ${span.name}`}
                aria-describedby={rowStateId}
                aria-pressed={isSelected}
                onClick={() => onSelectSpan(span.span_id)}
              >
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2">
                    <span
                      className={`rounded px-1.5 py-0.5 text-xs font-medium ${
                        kindColors[span.kind] || kindColors.CUSTOM
                      }`}
                    >
                      {span.kind}
                    </span>
                    <span
                      className={`truncate text-sm font-medium ${
                        isMatch
                          ? 'rounded bg-amber-100 px-1 text-amber-950 dark:bg-amber-500/20 dark:text-amber-100'
                          : 'text-[var(--continua-text-primary)]'
                      }`}
                    >
                      {span.name}
                    </span>
                    {isSelected ? (
                      <span className="rounded-full border border-[var(--continua-accent)] bg-[var(--continua-accent-faint)] px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.16em] text-[var(--continua-accent)]">
                        Selected
                      </span>
                    ) : null}
                    {isOnPrimaryPath ? (
                      <span className="rounded-full border border-amber-200 bg-[var(--continua-surface-elevated)] px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.16em] text-amber-700 dark:border-amber-500/40 dark:text-amber-200">
                        Failure path
                      </span>
                    ) : null}
                    {isFailed ? (
                      <span className="rounded-full border border-red-200 bg-[var(--continua-surface-elevated)] px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.16em] text-[var(--continua-error)] dark:border-red-500/40 dark:text-red-200">
                        Failed
                      </span>
                    ) : null}
                    {isFailed && retrySafety ? (
                      <RetrySafetyBadge
                        classification={retrySafety.classification}
                        variant="compact"
                        aria-label={getAccessibleSummary(retrySafety.classification)}
                      />
                    ) : null}
                  </div>

                  {errorPreview ? (
                    <p className="mt-2 text-sm text-[var(--continua-error)] line-clamp-2">
                      {errorPreview}
                    </p>
                  ) : null}

                  {showMetrics ? (
                    <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-[var(--continua-text-muted)]">
                      <span>{formatTokens((span.tokens_in ?? 0) + (span.tokens_out ?? 0))} tokens</span>
                      <span>{formatCost(span.cost_usd)}</span>
                    </div>
                  ) : null}

                  <div
                    id={rowStateId}
                    className="mt-2 flex flex-wrap items-center gap-2 text-xs text-[var(--continua-text-muted)]"
                  >
                    <StatusBadge status={span.status} />
                    <span>{formatDuration(span.latency_ms)}</span>
                  </div>
                </div>
              </button>
            </div>
          </div>
        );
      })}
    </div>
  );
}
