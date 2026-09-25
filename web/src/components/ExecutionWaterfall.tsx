import { useEffect, useMemo, useState, type CSSProperties } from 'react';
import { ChevronDown, ChevronRight, Filter } from 'lucide-react';
import type { Span, TimelineEvent } from '../api/client';
import { useVirtualRows } from '../hooks/useVirtualRows';
import { formatDerivedDuration } from '../utils/format';
import {
  getAccessibleSummary,
  type RetrySafetyAssessment,
} from '../utils/retrySafety';
import type { SpanTreeRow } from '../utils/spanTree';
import { deriveWaterfallWindow } from '../utils/waterfallTime';
import { DerivedTag, HonestyNote } from './DataState';
import { RetrySafetyBadge } from './RetrySafetyBadge';
import {
  SLOW_STEP_THRESHOLD_MS,
  getStepBarLayout,
  isSlowStep,
  type StepDuration,
} from './trace/traceSteps';
import type { StepDurations } from './trace/useStepDurations';

interface ExecutionWaterfallProps {
  events: TimelineEvent[];
  /** Visible tree rows, in tree order. */
  rows: SpanTreeRow[];
  spans: Span[];
  durations: StepDurations;
  selectedSpanId: string | null;
  onSelectSpan: (spanId: string) => void;
  revealTarget: string | null;
  expandedSpanIds?: ReadonlySet<string>;
  onToggleExpand?: (spanId: string) => void;
  traceEndedAt?: string;
  traceStartedAt?: string;
  spanAssessments?: ReadonlyMap<string, RetrySafetyAssessment>;
}

export const WATERFALL_ROW_HEIGHT = 33;
const INDENT_PER_DEPTH_PX = 14;
const MIN_BAR_WIDTH_PX = 3;
const GRID_COLUMNS = 'grid-cols-[minmax(12rem,300px)_minmax(0,1fr)_7.5rem]';
const EMPTY_SPAN_ASSESSMENTS = new Map<string, RetrySafetyAssessment>();
const EMPTY_EXPANDED = new Set<string>();
const UNKNOWN_DURATION: StepDuration = { ms: null, derived: false, running: false };

export const DERIVED_DURATION_NOTE =
  'Per-span duration was not recorded. These values are computed from recorded start and end timestamps.';

function statusDotColor(status: Span['status']): string {
  if (status === 'FAILED') return 'var(--c-red)';
  if (status === 'STARTED' || status === 'SCHEDULED') return 'var(--c-blue)';
  return 'var(--c-green)';
}

function barColor(status: Span['status'], selected: boolean, slow: boolean, running: boolean): string {
  if (selected) return 'color-mix(in srgb, var(--c-accent) 55%, transparent)';
  if (status === 'FAILED') return 'color-mix(in srgb, var(--c-red) 50%, transparent)';
  if (running) return 'color-mix(in srgb, var(--c-blue) 42%, transparent)';
  return slow
    ? 'color-mix(in srgb, var(--c-green) 42%, transparent)'
    : 'color-mix(in srgb, var(--c-green) 22%, transparent)';
}

/**
 * One table that merges the span tree and the waterfall: STEP (indented tree
 * name), TIMING (a bar on the trace range), and DURATION. Durations that were
 * computed from timestamps carry a "derived" tag.
 */
export function ExecutionWaterfall({
  events,
  rows,
  spans,
  durations,
  selectedSpanId,
  onSelectSpan,
  revealTarget,
  expandedSpanIds = EMPTY_EXPANDED,
  onToggleExpand,
  traceEndedAt,
  traceStartedAt,
  spanAssessments = EMPTY_SPAN_ASSESSMENTS,
}: ExecutionWaterfallProps) {
  const [focusSlow, setFocusSlow] = useState(false);
  const range = useMemo(
    () => deriveWaterfallWindow({ traceStartedAt, traceEndedAt, spans, events }),
    [events, spans, traceEndedAt, traceStartedAt]
  );

  const shownRows = useMemo(() => {
    if (!focusSlow) {
      return rows;
    }
    const parentById = new Map(spans.map((span) => [span.span_id, span.parent_span_id]));
    const keep = new Set<string>();
    for (const span of spans) {
      const duration = durations.byId.get(span.span_id) ?? UNKNOWN_DURATION;
      if (!isSlowStep(duration) && span.span_id !== selectedSpanId) {
        continue;
      }
      let current: string | undefined = span.span_id;
      while (current && !keep.has(current)) {
        keep.add(current);
        current = parentById.get(current);
      }
    }
    return rows.filter((row) => keep.has(row.span.span_id));
  }, [durations, focusSlow, rows, selectedSpanId, spans]);

  const hiddenCount = rows.length - shownRows.length;
  const perRowDerivedTag = durations.anyDerived && !durations.allDerived;

  const {
    containerRef,
    onScroll,
    paddingBottom,
    paddingTop,
    scrollToIndex,
    virtualRows,
  } = useVirtualRows({ estimatedRowHeight: WATERFALL_ROW_HEIGHT, rows: shownRows });

  useEffect(() => {
    if (!revealTarget) {
      return;
    }
    const rowIndex = shownRows.findIndex((row) => row.span.span_id === revealTarget);
    if (rowIndex !== -1) {
      scrollToIndex(rowIndex);
    }
  }, [revealTarget, shownRows, scrollToIndex]);

  if (rows.length === 0 || !range) {
    return (
      <section className="flex h-full items-center justify-center bg-[var(--c-app-bg)]">
        <div className="text-sm text-[var(--c-text-muted)]">No steps were recorded for this trace.</div>
      </section>
    );
  }

  return (
    <section
      aria-label="Execution steps"
      className="flex h-full min-h-0 flex-col overflow-hidden bg-[var(--c-app-bg)]"
    >
      <div className="flex min-h-10 flex-wrap items-center gap-x-3 gap-y-1.5 border-b border-[var(--c-border)] px-4 py-1.5">
        <button
          type="button"
          aria-pressed={focusSlow}
          onClick={() => setFocusSlow((value) => !value)}
          className={`inline-flex h-7 items-center gap-1.5 rounded-md border px-2.5 text-xs font-medium transition ${
            focusSlow
              ? 'border-[var(--c-accent-border)] bg-[var(--c-accent-faint)] text-[var(--c-accent-text)]'
              : 'border-[var(--c-border)] bg-[var(--c-surface)] text-[var(--c-text-secondary)] hover:border-[var(--c-border-strong)] hover:text-[var(--c-text-primary)]'
          }`}
        >
          <Filter aria-hidden="true" className="h-3.5 w-3.5" />
          Focus slow steps
        </button>
        <span className="text-xs text-[var(--c-text-muted)]">
          {focusSlow
            ? `${shownRows.length} of ${rows.length} steps`
            : `All ${spans.length} steps`}
        </span>
        <div className="ml-auto flex items-center gap-3 text-[11px] text-[var(--c-text-muted)]">
          <LegendSwatch color="color-mix(in srgb, var(--c-green) 42%, transparent)" label="completed" />
          <LegendSwatch color="color-mix(in srgb, var(--c-accent) 55%, transparent)" label="selected" />
          <span className="font-mono tabular-nums">
            0 – {formatDerivedDuration(range.durationMs)}
          </span>
        </div>
      </div>

      <div
        className={`grid h-7 shrink-0 items-center ${GRID_COLUMNS} border-b border-[var(--c-border)] bg-[var(--c-table-head-bg)] text-[10.5px] font-semibold uppercase tracking-[0.06em] text-[var(--c-text-muted)]`}
      >
        <div className="px-4">Step</div>
        <div className="px-3">Timing</div>
        <div className="flex items-center justify-end gap-1.5 px-4">
          Duration
          {durations.allDerived ? <DerivedTag className="normal-case tracking-normal" /> : null}
        </div>
      </div>

      <div ref={containerRef} className="min-h-0 flex-1 overflow-y-auto" onScroll={onScroll}>
        <div style={{ paddingBottom: `${paddingBottom}px`, paddingTop: `${paddingTop}px` }}>
          {virtualRows.map(({ row }) => {
            const { span } = row;
            const duration = durations.byId.get(span.span_id) ?? UNKNOWN_DURATION;
            const isSelected = span.span_id === selectedSpanId;
            const slow = isSlowStep(duration);
            const bar = getStepBarLayout(span, duration, range);
            const retrySafety = spanAssessments.get(span.span_id) ?? null;
            const isExpanded = expandedSpanIds.has(span.span_id);
            const rowStyle: CSSProperties = {
              height: `${WATERFALL_ROW_HEIGHT}px`,
              boxShadow: isSelected ? 'inset 2px 0 0 var(--c-accent)' : undefined,
            };

            return (
              <div
                key={span.id}
                data-selected={isSelected ? 'true' : undefined}
                className={`group relative grid ${GRID_COLUMNS} border-b border-[var(--c-border-subtle)] ${
                  isSelected ? 'bg-[var(--c-row-selected-bg)]' : ''
                }`}
                style={rowStyle}
              >
                <button
                  type="button"
                  aria-label={`Select step ${span.name}`}
                  aria-current={isSelected ? 'true' : undefined}
                  onClick={() => onSelectSpan(span.span_id)}
                  className={`absolute inset-0 z-0 focus:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-[var(--c-accent)] ${
                    isSelected ? '' : 'hover:bg-[var(--c-row-hover-bg)]'
                  }`}
                />
                <div
                  className="pointer-events-none relative z-10 flex min-w-0 items-center gap-1.5 pr-3"
                  style={{ paddingLeft: `${10 + row.depth * INDENT_PER_DEPTH_PX}px` }}
                >
                  {row.hasChildren && onToggleExpand ? (
                    <button
                      type="button"
                      aria-label={`${isExpanded ? 'Collapse' : 'Expand'} ${span.name}`}
                      aria-expanded={isExpanded}
                      onClick={() => onToggleExpand(span.span_id)}
                      className="pointer-events-auto inline-flex h-4 w-4 shrink-0 items-center justify-center rounded text-[var(--c-text-muted)] hover:text-[var(--c-text-primary)]"
                    >
                      {isExpanded ? (
                        <ChevronDown aria-hidden="true" className="h-3.5 w-3.5" />
                      ) : (
                        <ChevronRight aria-hidden="true" className="h-3.5 w-3.5" />
                      )}
                    </button>
                  ) : (
                    <span aria-hidden="true" className="w-4 shrink-0" />
                  )}
                  <span
                    aria-hidden="true"
                    className="h-1.5 w-1.5 shrink-0 rounded-full"
                    style={{ background: statusDotColor(span.status) }}
                  />
                  <span
                    className={`min-w-0 truncate font-mono text-xs ${
                      slow || isSelected
                        ? 'text-[var(--c-text-primary)]'
                        : 'text-[var(--c-text-secondary)]'
                    }`}
                  >
                    {span.name}
                  </span>
                  <span className="shrink-0 rounded-[3px] border border-[var(--c-border)] px-1 text-[9.5px] font-semibold uppercase leading-4 tracking-[0.04em] text-[var(--c-text-muted)]">
                    {span.kind}
                  </span>
                  {span.status === 'FAILED' && retrySafety ? (
                    <RetrySafetyBadge
                      classification={retrySafety.classification}
                      variant="compact"
                      aria-label={getAccessibleSummary(retrySafety.classification)}
                    />
                  ) : null}
                </div>

                <div className="pointer-events-none relative z-10 flex items-center px-3">
                  <div className="relative h-3.5 w-full">
                    <span
                      data-testid="step-bar"
                      className="absolute inset-y-0 rounded-[2px]"
                      style={{
                        left: `${bar.leftPercent}%`,
                        width: `${bar.widthPercent}%`,
                        minWidth: `${MIN_BAR_WIDTH_PX}px`,
                        background: barColor(span.status, isSelected, slow, duration.running),
                      }}
                    />
                  </div>
                </div>

                <div className="pointer-events-none relative z-10 flex items-center justify-end gap-1.5 whitespace-nowrap px-4 font-mono text-[11.5px] tabular-nums text-[var(--c-text-secondary)]">
                  {perRowDerivedTag && duration.derived ? <DerivedTag /> : null}
                  <span>{formatDerivedDuration(duration.ms)}</span>
                </div>
              </div>
            );
          })}
        </div>

        {focusSlow && hiddenCount > 0 ? (
          <div className="flex flex-wrap items-center gap-1.5 border-b border-[var(--c-border-subtle)] px-4 py-2 text-xs text-[var(--c-text-muted)]">
            <span>
              {hiddenCount} {hiddenCount === 1 ? 'step' : 'steps'} under {SLOW_STEP_THRESHOLD_MS} ms{' '}
              {hiddenCount === 1 ? 'is' : 'are'} hidden.
            </span>
            <button
              type="button"
              onClick={() => setFocusSlow(false)}
              className="font-medium text-[var(--c-accent-text)] hover:underline"
            >
              Show all {rows.length}
            </button>
          </div>
        ) : null}

        {durations.anyDerived ? (
          <HonestyNote className="px-4 py-3">{DERIVED_DURATION_NOTE}</HonestyNote>
        ) : null}
      </div>
    </section>
  );
}

function LegendSwatch({ color, label }: { color: string; label: string }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span aria-hidden="true" className="h-2 w-3 rounded-[2px]" style={{ background: color }} />
      {label}
    </span>
  );
}
