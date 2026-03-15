import { useEffect, useMemo, useRef } from 'react';
import type { Span, TimelineEvent } from '../api/client';
import { useVirtualRows } from '../hooks/useVirtualRows';
import { formatDuration } from '../utils/format';
import type { SpanTreeRow } from '../utils/spanTree';
import {
  buildWaterfallTicks,
  deriveWaterfallWindow,
  getWaterfallBarLayout,
} from '../utils/waterfallTime';

interface ExecutionWaterfallProps {
  events: TimelineEvent[];
  rows: SpanTreeRow[];
  selectedSpanId: string | null;
  onSelectSpanAndShowDetails: (spanId: string) => void;
  revealTarget: string | null;
  revealVersion: number;
  spans: Span[];
  traceEndedAt?: string;
  traceStartedAt?: string;
}

const MIN_BAR_WIDTH_REM = 0.875;
const WATERFALL_ROW_HEIGHT = 68;
const TICK_LINE_COLOR = 'rgba(209, 213, 219, 0.7)';

export function ExecutionWaterfall({
  events,
  rows,
  selectedSpanId,
  onSelectSpanAndShowDetails,
  revealTarget,
  revealVersion,
  spans,
  traceEndedAt,
  traceStartedAt,
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
      <section className="flex h-full items-center justify-center rounded-2xl border border-gray-200 bg-white shadow-sm">
        <div className="text-sm text-gray-500">No spans available for execution timing.</div>
      </section>
    );
  }

  return (
    <section className="flex h-full min-h-0 flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm">
      <div className="border-b border-gray-200 bg-gray-50 px-4 py-3">
        <h2 className="text-sm font-semibold uppercase tracking-[0.2em] text-gray-600">
          Execution Waterfall
        </h2>
        <p className="mt-1 text-sm text-gray-500">
          Timing bars follow the visible tree order and selection state.
        </p>
      </div>

      <div className="grid grid-cols-[minmax(0,13rem)_minmax(0,1fr)] border-b border-gray-200 bg-white">
        <div className="border-r border-gray-200 px-4 py-3 text-xs font-semibold uppercase tracking-[0.18em] text-gray-500">
          Visible spans
        </div>
        <div className="relative px-4 py-3">
          <div className="relative flex h-full items-start justify-between gap-2 text-[11px] font-medium uppercase tracking-[0.16em] text-gray-500">
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

      <div
        ref={containerRef}
        className="min-h-0 flex-1 overflow-y-auto"
        onScroll={onScroll}
      >
        <div
          className="divide-y divide-gray-100"
          style={{
            paddingBottom: `${paddingBottom}px`,
            paddingTop: `${paddingTop}px`,
          }}
        >
          {virtualRows.map(({ row }) => {
            const bar = getWaterfallBarLayout(row.span, window);
            const isSelected = row.span.span_id === selectedSpanId;

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
              >
                <button
                  type="button"
                  className={`min-w-0 border-r border-gray-200 px-4 py-3 text-left transition ${
                    isSelected ? 'bg-blue-50' : 'bg-white hover:bg-gray-50'
                  }`}
                  onClick={() => onSelectSpanAndShowDetails(row.span.span_id)}
                >
                  <div
                    className="truncate text-sm font-medium text-gray-900"
                    style={{ paddingLeft: `${row.depth * 12}px` }}
                  >
                    {row.span.name}
                  </div>
                  <div className="mt-1 text-xs text-gray-500">
                    {row.span.status} · {formatDuration(row.span.latency_ms)}
                  </div>
                </button>

                <div className="px-4 py-3">
                  <div
                    className="relative h-11"
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
                      className={`absolute top-1/2 flex h-6 -translate-y-1/2 items-center rounded-full border px-2 text-xs font-medium transition focus:outline-none focus:ring-2 focus:ring-blue-200 ${
                        bar.isRunning
                          ? 'border-blue-300 bg-blue-100 text-blue-800'
                          : row.span.status === 'FAILED'
                            ? 'border-red-300 bg-red-100 text-red-800'
                            : isSelected
                              ? 'border-blue-300 bg-blue-200 text-blue-900'
                              : 'border-emerald-200 bg-emerald-100 text-emerald-800'
                      }`}
                      style={{
                        left: `${bar.leftPercent}%`,
                        width: `${Math.max(bar.widthPercent, 0.35)}%`,
                        minWidth: `${MIN_BAR_WIDTH_REM}rem`,
                      }}
                      aria-label={`Select waterfall span ${row.span.name}`}
                      title={`${row.span.name} • ${row.span.status} • ${formatDuration(
                        row.span.latency_ms
                      )}`}
                      onClick={() => onSelectSpanAndShowDetails(row.span.span_id)}
                    >
                      <span className="truncate">{row.span.kind}</span>
                      {bar.isRunning ? (
                        <span className="ml-1 h-2 w-2 shrink-0 rounded-full bg-blue-600" />
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
