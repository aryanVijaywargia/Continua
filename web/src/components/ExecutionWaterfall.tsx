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
export const WATERFALL_ROW_HEIGHT = 68;
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
      <section className="flex h-full items-center justify-center rounded-[1rem] border border-[var(--continua-border-strong)] bg-[var(--continua-surface)] shadow-[var(--continua-shadow-soft)]">
        <div className="text-sm text-[var(--continua-text-muted)]">No spans available for execution timing.</div>
      </section>
    );
  }

  return (
    <section className="flex h-full min-h-0 flex-col overflow-hidden rounded-[1rem] border border-[var(--continua-border-strong)] bg-[var(--continua-surface)] shadow-[var(--continua-shadow-soft)]">
      <div className="border-b border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-4 py-3">
        <h2 className="text-sm font-semibold uppercase tracking-[0.2em] text-[var(--continua-text-secondary)]">
          Execution Waterfall
        </h2>
        <p className="mt-1 text-sm text-[var(--continua-text-muted)]">
          Timing bars follow the visible tree order and selection state.
        </p>
      </div>

      <div className="grid grid-cols-[minmax(0,13rem)_minmax(0,1fr)] border-b border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)]">
        <div className="border-r border-[var(--continua-border-soft)] px-4 py-3 text-xs font-semibold uppercase tracking-[0.18em] text-[var(--continua-text-muted)]">
          Visible spans
        </div>
        <div className="relative px-4 py-3">
          <div className="relative flex h-full items-start justify-between gap-2 text-[11px] font-medium uppercase tracking-[0.16em] text-[var(--continua-text-muted)]">
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
      </div>

      <CostStrip series={costSeries} window={window} />

      <div
        ref={containerRef}
        className="min-h-0 flex-1 overflow-y-auto"
        onScroll={onScroll}
      >
        <div
          className="divide-y divide-[var(--continua-border-soft)]"
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
                className="grid grid-cols-[minmax(0,13rem)_minmax(0,1fr)]"
                style={{ height: `${WATERFALL_ROW_HEIGHT}px` }}
              >
                <button
                  type="button"
                  className={`flex h-full min-w-0 items-center border-r border-[var(--continua-border-soft)] px-4 py-3 text-left transition ${
                    isSelected
                      ? 'bg-[var(--continua-accent-faint)]'
                      : 'bg-[var(--continua-surface-elevated)] hover:bg-[var(--continua-surface-muted)]'
                  }`}
                  onClick={() => onSelectSpanAndShowDetails(row.span.span_id)}
                >
                  <div
                    className="min-w-0"
                    style={{ paddingLeft: `${row.depth * 12}px` }}
                  >
                    <div className="flex items-center gap-2">
                      <div className="min-w-0 flex-1 truncate text-sm font-medium text-[var(--continua-text-primary)]">
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

                    <div className="mt-1 flex items-center gap-1 overflow-hidden whitespace-nowrap text-xs text-[var(--continua-text-muted)]">
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

                <div className="flex h-full items-center px-4 py-3">
                  <div
                    className="relative h-11 w-full"
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
                      className={`absolute top-1/2 flex h-6 -translate-y-1/2 items-center rounded-full border px-2 text-xs font-medium text-[var(--continua-text-primary)] transition focus:outline-none focus:ring-2 focus:ring-[var(--continua-accent-faint)]`}
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
              </div>
            );
          })}
        </div>
      </div>
    </section>
  );
}
