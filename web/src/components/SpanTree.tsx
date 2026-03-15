import { useEffect, useRef } from 'react';
import { formatCost, formatDuration, formatTokens } from '../utils/format';
import type { SpanTreeRow } from '../utils/spanTree';
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
  revealKey: number;
  inlineErrorPreviews: ReadonlyMap<string, string>;
  showMetrics?: boolean;
  matchedSpanIds?: ReadonlySet<string> | null;
}

const kindColors: Record<string, string> = {
  LLM: 'bg-purple-100 text-purple-800',
  TOOL: 'bg-yellow-100 text-yellow-800',
  CHAIN: 'bg-blue-100 text-blue-800',
  AGENT: 'bg-green-100 text-green-800',
  CUSTOM: 'bg-gray-100 text-gray-800',
};

export function SpanTree({
  rows,
  expandedSpanIds,
  selectedSpanId,
  onSelectSpan,
  onToggleExpand,
  failedSpanIds,
  primaryAncestorPath,
  revealPath,
  revealKey,
  inlineErrorPreviews,
  showMetrics = false,
  matchedSpanIds = null,
}: SpanTreeProps) {
  const rowRefs = useRef(new Map<string, HTMLDivElement>());

  useEffect(() => {
    const targetSpanId =
      selectedSpanId && revealPath.has(selectedSpanId) ? selectedSpanId : null;
    if (!targetSpanId) {
      return;
    }

    rowRefs.current.get(targetSpanId)?.scrollIntoView?.({ block: 'nearest' });
  }, [revealKey, revealPath, selectedSpanId]);

  if (rows.length === 0) {
    return (
      <div className="p-4 text-center text-gray-500">
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

        const rowClasses = isSelected
          ? 'border-blue-200 bg-blue-50 shadow-sm ring-1 ring-blue-200'
          : isOnPrimaryPath
            ? 'border-amber-300 bg-amber-50'
            : isFailed
              ? 'border-red-200 bg-red-50/80'
              : 'border-transparent bg-white hover:bg-gray-50';

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
                  className="mt-3 flex h-6 w-6 shrink-0 items-center justify-center rounded text-gray-400 transition hover:bg-gray-100 hover:text-gray-600 focus:outline-none focus:ring-2 focus:ring-blue-200"
                  aria-label={`${isExpanded ? 'Collapse' : 'Expand'} span ${span.name}`}
                  onClick={() => onToggleExpand(span.span_id)}
                >
                  {isExpanded ? '▼' : '▶'}
                </button>
              ) : (
                <span
                  className="mt-3 flex h-6 w-6 shrink-0 items-center justify-center text-gray-300"
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
                          ? 'rounded bg-amber-100 px-1 text-amber-950'
                          : 'text-gray-900'
                      }`}
                    >
                      {span.name}
                    </span>
                    {isSelected ? (
                      <span className="rounded-full border border-blue-200 bg-white px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.16em] text-blue-700">
                        Selected
                      </span>
                    ) : null}
                    {isOnPrimaryPath ? (
                      <span className="rounded-full border border-amber-200 bg-white px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.16em] text-amber-700">
                        Failure path
                      </span>
                    ) : null}
                    {isFailed ? (
                      <span className="rounded-full border border-red-200 bg-white px-2 py-0.5 text-[11px] font-semibold uppercase tracking-[0.16em] text-red-700">
                        Failed
                      </span>
                    ) : null}
                  </div>

                  {errorPreview ? (
                    <p className="mt-2 text-sm text-red-700 line-clamp-2">
                      {errorPreview}
                    </p>
                  ) : null}

                  {showMetrics ? (
                    <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-gray-500">
                      <span>{formatTokens((span.tokens_in ?? 0) + (span.tokens_out ?? 0))} tokens</span>
                      <span>{formatCost(span.cost_usd)}</span>
                    </div>
                  ) : null}

                  <div
                    id={rowStateId}
                    className="mt-2 flex flex-wrap items-center gap-2 text-xs text-gray-500"
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
