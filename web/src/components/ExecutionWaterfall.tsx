import { useEffect, useMemo, useRef } from 'react';
import type { Span, TimelineEvent } from '../api/client';
import { useVirtualRows } from '../hooks/useVirtualRows';
import { formatCost, formatDuration, formatTokens } from '../utils/format';
import type { TraceCostSeries } from '../utils/reasoning';
import {
  getAccessibleSummary,
  type RetrySafetyAssessment,
} from '../utils/retrySafety';
import type { SpanTreeRow } from '../utils/spanTree';
import {
  buildWaterfallTicks,
  deriveWaterfallWindow,
  getWaterfallBarLayout,
} from '../utils/waterfallTime';
import { CostStrip } from './CostStrip';
import { RetrySafetyBadge } from './RetrySafetyBadge';

interface ExecutionWaterfallProps {
  events: TimelineEvent[];
  rows: SpanTreeRow[];
  selectedSpanId: string | null;
  onSelectSpanAndShowDetails: (spanId: string) => void;
  revealTarget: string | null;
  revealVersion: number;
  spans: Span[];
  costSeries?: TraceCostSeries | null;
  traceEndedAt?: string;
  traceStartedAt?: string;
  spanAssessments?: ReadonlyMap<string, RetrySafetyAssessment>;
}

const MIN_BAR_WIDTH_REM = 0.875;
export const WATERFALL_ROW_HEIGHT = 40;
const TICK_LINE_COLOR = 'var(--continua-waterfall-tick-color)';
const EMPTY_SPAN_ASSESSMENTS = new Map<string, RetrySafetyAssessment>();

export function ExecutionWaterfall({
  events,
  rows,
  selectedSpanId,
  onSelectSpanAndShowDetails,
  revealTarget,
  revealVersion,
  spans,
  costSeries = null,
  traceEndedAt,
  traceStartedAt,
  spanAssessments = EMPTY_SPAN_ASSESSMENTS,
}: ExecutionWaterfallProps) {
  const rowRefs = useRef(new Map<string, HTMLDivElement>());
  const window = useMemo(
    () =>
      deriveWaterfallWindow({
        traceStartedAt,
        traceEndedAt,
        spans,
        events,
      }),
    [events, spans, traceEndedAt, traceStartedAt]
  );
  const ticks = useMemo(
    () => (window ? buildWaterfallTicks(window) : []),
    [window]
  );
  const timingGridBackground = useMemo(
    () =>
      ticks.length === 0
        ? undefined
        : ticks
            .map(
              (tick) =>
                `linear-gradient(to right, transparent calc(${tick.leftPercent}% - 0.5px), ${TICK_LINE_COLOR} calc(${tick.leftPercent}% - 0.5px), ${TICK_LINE_COLOR} calc(${tick.leftPercent}% + 0.5px), transparent calc(${tick.leftPercent}% + 0.5px))`
            )
            .join(', '),
    [ticks]
  );
  const {
    containerRef,
    onScroll,
    paddingBottom,
    paddingTop,
    scrollToIndex,
    virtualRows,
  } = useVirtualRows({
    estimatedRowHeight: WATERFALL_ROW_HEIGHT,
    rows,
  });

  useEffect(() => {
    if (!revealTarget) {
      return;
    }

    const rowIndex = rows.findIndex((row) => row.span.span_id === revealTarget);
    if (rowIndex !== -1) {
      scrollToIndex(rowIndex);
    }

    rowRefs.current.get(revealTarget)?.scrollIntoView?.({ block: 'nearest' });
  }, [revealTarget, revealVersion, rows, scrollToIndex]);

  if (rows.length === 0 || !window) {
    return (
      <section className="flex h-full items-center justify-center border border-[var(--c-border)] bg-[var(--c-surface)]">
        <div className="text-sm text-[var(--c-text-muted)]">No spans available for execution timing.</div>
      </section>
    );
  }

  return (
    <section className="flex h-full min-h-0 flex-col overflow-hidden bg-[var(--c-app-bg)]">
      <div className="border-b border-[var(--c-border)] bg-[var(--c-app-bg)] px-6 py-3">
        <h2 className="text-[11px] font-semibold uppercase tracking-[0.04em] text-[var(--c-text-muted)]">
          Execution Waterfall
        </h2>
        <p className="mt-1 text-xs text-[var(--c-text-muted)]">
          Timing bars follow the visible tree order and selection state.
        </p>
      </div>

      <div className="grid grid-cols-[minmax(12rem,16rem)_minmax(0,1fr)_5rem] border-b border-[var(--c-border)] bg-[var(--c-table-head-bg)]">
        <div className="px-6 py-2 text-[11px] font-semibold uppercase tracking-[0.04em] text-[var(--c-text-muted)]">
          Span
        </div>
        <div className="relative border-l border-[var(--c-border)] px-4 py-2">
          <div className="relative flex h-full items-start justify-between gap-2 text-[10.5px] font-medium uppercase tracking-[0.04em] text-[var(--c-text-muted)]">
            {ticks.map((tick) => (
              <span
                key={tick.leftPercent}
                className="translate-x-[-50%] whitespace-nowrap"
                style={{ marginLeft: `${tick.leftPercent}%` }}
              >
                +{tick.label}
              </span>
            ))}
          </div>
        </div>
        <div className="px-4 py-2 text-right text-[11px] font-semibold uppercase tracking-[0.04em] text-[var(--c-text-muted)]">
          Duration
        </div>
      </div>

      <CostStrip series={costSeries} window={window} />

      <div
        ref={containerRef}
        className="min-h-0 flex-1 overflow-y-auto"
        onScroll={onScroll}
      >
        <div
          className="divide-y divide-[var(--c-border-subtle)]"
          style={{
            paddingBottom: `${paddingBottom}px`,
            paddingTop: `${paddingTop}px`,
          }}
        >
          {virtualRows.map(({ row }) => {
            const bar = getWaterfallBarLayout(row.span, window);
            const isSelected = row.span.span_id === selectedSpanId;
            const retrySafety = spanAssessments.get(row.span.span_id) ?? null;
            const totalTokens = (row.span.tokens_in ?? 0) + (row.span.tokens_out ?? 0);
            const hasTokenData = totalTokens !== 0;
            const hasCostData = (row.span.cost_usd ?? 0) !== 0;

            return (
              <div
                key={row.span.id}
                ref={(element) => {
                  if (element) {
                    rowRefs.current.set(row.span.span_id, element);
                    return;
                  }

                  rowRefs.current.delete(row.span.span_id);
                }}
                className={`grid grid-cols-[minmax(12rem,16rem)_minmax(0,1fr)_5rem] ${
                  isSelected ? 'bg-[var(--c-row-selected-bg)]' : ''
                }`}
                style={{ height: `${WATERFALL_ROW_HEIGHT}px` }}
              >
                <button
                  type="button"
                  className={`flex h-full min-w-0 items-center px-6 py-2 text-left transition ${
                    isSelected
                      ? ''
                      : 'hover:bg-[var(--c-row-hover-bg)]'
                  }`}
                  onClick={() => onSelectSpanAndShowDetails(row.span.span_id)}
                >
                  <div
                    className="min-w-0"
                    style={{ paddingLeft: `${row.depth * 12}px` }}
                  >
                    <div className="flex items-center gap-2">
                      <span
                        className="h-1.5 w-1.5 shrink-0 rounded-full"
                        style={{
                          background:
                            row.span.status === 'FAILED'
                              ? 'var(--c-red)'
                              : row.span.status === 'STARTED'
                                ? 'var(--c-blue)'
                                : 'var(--c-green)',
                        }}
                      />
                      <div className="min-w-0 flex-1 truncate text-[12.5px] font-medium text-[var(--c-text-primary)]">
                        {row.span.name}
                      </div>
                      {row.span.status === 'FAILED' && retrySafety ? (
                        <RetrySafetyBadge
                          classification={retrySafety.classification}
                          variant="compact"
                          aria-label={getAccessibleSummary(retrySafety.classification)}
                        />
                      ) : null}
                    </div>

                    <div className="mt-0.5 flex items-center gap-1 overflow-hidden whitespace-nowrap text-[11px] text-[var(--c-text-muted)]">
                      <span className="shrink-0">{row.span.status}</span>
                      <span aria-hidden="true">·</span>
                      <span className="shrink-0">{formatDuration(row.span.latency_ms)}</span>
                      {hasTokenData ? (
                        <>
                          <span aria-hidden="true">·</span>
                          <span className="truncate">{formatTokens(totalTokens)} tokens</span>
                        </>
                      ) : null}
                      {hasCostData ? (
                        <>
                          <span aria-hidden="true">·</span>
                          <span className="truncate">{formatCost(row.span.cost_usd)}</span>
                        </>
                      ) : null}
                    </div>
                  </div>
                </button>

                <div className="flex h-full items-center border-l border-[var(--c-border)] px-4 py-2">
                  <div
                    className="relative h-10 w-full"
                    style={
                      timingGridBackground
                        ? {
                            backgroundImage: timingGridBackground,
                            backgroundRepeat: 'no-repeat',
                          }
                        : undefined
                    }
                  >
                    <button
                      type="button"
                      className="absolute top-1/2 flex h-4 -translate-y-1/2 items-center rounded-[3px] border-l-2 px-1.5 text-[10px] font-medium text-[var(--c-text-secondary)] transition focus:outline-none focus:ring-2 focus:ring-[var(--c-accent-faint)]"
                      style={{
                        left: `${bar.leftPercent}%`,
                        width: `${Math.max(bar.widthPercent, 0.35)}%`,
                        minWidth: `${MIN_BAR_WIDTH_REM}rem`,
                        backgroundColor: bar.isRunning
                          ? 'var(--continua-waterfall-running-bg)'
                          : row.span.status === 'FAILED'
                            ? 'var(--continua-waterfall-failed-bg)'
                            : isSelected
                              ? 'var(--continua-waterfall-success-selected-bg)'
                              : 'var(--continua-waterfall-success-bg)',
                        borderColor: bar.isRunning
                          ? 'var(--continua-waterfall-running-border)'
                          : row.span.status === 'FAILED'
                            ? 'var(--continua-waterfall-failed-border)'
                            : isSelected
                              ? 'var(--continua-waterfall-success-selected-border)'
                              : 'var(--continua-waterfall-success-border)',
                      }}
                      aria-label={`Select waterfall span ${row.span.name}`}
                      title={`${row.span.name} • ${row.span.status} • ${formatDuration(
                        row.span.latency_ms
                      )}`}
                      onClick={() => onSelectSpanAndShowDetails(row.span.span_id)}
                    >
                      <span className="truncate">{row.span.kind}</span>
                      {bar.isRunning ? (
                        <span
                          className="ml-1 h-2 w-2 shrink-0 rounded-full"
                          style={{ backgroundColor: 'var(--continua-waterfall-running-dot)' }}
                        />
                      ) : null}
                    </button>
                  </div>
                </div>
                <button
                  type="button"
                  className={`h-full px-4 text-right font-mono text-xs tabular-nums text-[var(--c-text-secondary)] transition ${
                    isSelected ? '' : 'hover:bg-[var(--c-row-hover-bg)]'
                  }`}
                  onClick={() => onSelectSpanAndShowDetails(row.span.span_id)}
                >
                  {formatDuration(row.span.latency_ms)}
                </button>
              </div>
            );
          })}
        </div>
      </div>
    </section>
  );
}
