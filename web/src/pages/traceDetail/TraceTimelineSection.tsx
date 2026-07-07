import { useMemo, useState } from 'react';
import { formatDuration } from '../../utils/format';
import {
  buildTimelinePhases,
  buildTimelineRows,
  findRootSpanId,
  formatOffset,
  type TimelinePhase,
  type TimelineRow,
  type TimelineRowLevel,
  type TimelineRowType,
} from '../../utils/timelineRows';
import { InspectorEmptyState } from './CompactPayloadInspector';
import { useTraceDetailWorkspace } from './traceDetailWorkspaceContext';

const TIMELINE_FILTERS: Array<{ id: TimelineRowType | 'all'; label: string }> = [
  { id: 'all', label: 'All' },
  { id: 'engine', label: 'Engine' },
  { id: 'span', label: 'Spans' },
  { id: 'log', label: 'Logs' },
];

/** Chronological trace events with a zoomable phase ribbon and type filters. */
export function TraceTimelineSection() {
  const { events, selectSpanAndShowDetails, spanIndex, spans, trace } =
    useTraceDetailWorkspace();
  const [zoom, setZoom] = useState(1);
  const [filter, setFilter] = useState<TimelineRowType | 'all'>('all');

  const traceStartMs = useMemo(
    () => new Date(trace.started_at).getTime(),
    [trace.started_at]
  );
  const traceEndMs = useMemo(
    () => (trace.ended_at ? new Date(trace.ended_at).getTime() : Date.now()),
    [trace.ended_at]
  );
  const totalMs = Math.max(traceEndMs - traceStartMs, 1);

  const rootSpanId = useMemo(() => findRootSpanId(spans), [spans]);

  const phases = useMemo<TimelinePhase[]>(
    () => buildTimelinePhases(spans, rootSpanId, traceStartMs, totalMs),
    [rootSpanId, spans, totalMs, traceStartMs]
  );

  const rows = useMemo<TimelineRow[]>(
    () => buildTimelineRows(events, rootSpanId, traceStartMs),
    [events, rootSpanId, traceStartMs]
  );

  const counts = useMemo(() => {
    const counts: Record<TimelineRowType, number> = { engine: 0, span: 0, log: 0 };
    rows.forEach((row) => {
      counts[row.type] += 1;
    });
    return counts;
  }, [rows]);

  const filteredRows = filter === 'all' ? rows : rows.filter((row) => row.type === filter);

  const filterCount = (id: TimelineRowType | 'all') =>
    id === 'all' ? rows.length : counts[id];

  const typeColor: Record<TimelineRowType, string> = {
    engine: 'var(--c-accent-text)',
    span: 'var(--c-text-secondary)',
    log: 'var(--c-text-muted)',
  };
  const levelColor: Record<TimelineRowLevel, string> = {
    info: 'var(--c-text-muted)',
    warn: 'var(--c-amber-text)',
    error: 'var(--c-red-text)',
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex items-center justify-between border-b border-[var(--c-border-subtle)] px-4 py-2.5">
        <div className="flex gap-1 rounded-md border border-[var(--c-border)] bg-[var(--c-surface-muted)] p-0.5">
          {TIMELINE_FILTERS.map((option) => {
            const active = filter === option.id;
            return (
              <button
                key={option.id}
                type="button"
                onClick={() => setFilter(option.id)}
                className={`inline-flex items-center gap-1.5 rounded px-2.5 py-1 text-[11.5px] font-medium transition ${
                  active
                    ? 'border border-[var(--c-border)] bg-[var(--c-surface)] text-[var(--c-text-primary)]'
                    : 'border border-transparent text-[var(--c-text-secondary)] hover:text-[var(--c-text-primary)]'
                }`}
              >
                {option.label}
                <span className="font-mono text-[10.5px] text-[var(--c-text-muted)]">
                  {filterCount(option.id)}
                </span>
              </button>
            );
          })}
        </div>
        <label className="flex items-center gap-2.5 text-[11.5px] text-[var(--c-text-muted)]">
          <span>Zoom</span>
          <input
            type="range"
            min={1}
            max={4}
            step={0.5}
            value={zoom}
            onChange={(event) => setZoom(parseFloat(event.target.value))}
            className="h-1 w-24 cursor-pointer accent-[var(--c-accent)]"
            aria-label="Zoom"
          />
          <span className="min-w-[32px] font-mono">{zoom}×</span>
        </label>
      </div>

      <div className="border-b border-[var(--c-border-subtle)] px-4 py-3">
        <div className="overflow-x-auto">
          <div style={{ width: `${zoom * 100}%`, minWidth: '100%' }}>
            <div className="relative h-8 overflow-hidden rounded border border-[var(--c-border)] bg-[var(--c-surface-muted)]">
              <TimelinePhaseRibbon phases={phases} totalMs={totalMs} />
              {rows.map((row) => (
                <div
                  key={row.id}
                  className="absolute top-0 bottom-0"
                  style={{
                    left: `${(row.offsetMs / totalMs) * 100}%`,
                    width: 1.5,
                    background:
                      row.level === 'error'
                        ? 'var(--c-red)'
                        : row.level === 'warn'
                          ? 'var(--c-amber)'
                          : typeColor[row.type],
                    opacity: 0.7,
                  }}
                />
              ))}
              <div
                className="pointer-events-none absolute inset-0 grid"
                style={{ gridTemplateColumns: 'repeat(4, 1fr)' }}
              >
                <span className="border-r border-dashed border-[var(--c-bar-tick)]" />
                <span className="border-r border-dashed border-[var(--c-bar-tick)]" />
                <span className="border-r border-dashed border-[var(--c-bar-tick)]" />
                <span />
              </div>
            </div>
          </div>
        </div>
        <div className="mt-1 flex justify-between font-mono text-[10.5px] text-[var(--c-text-muted)]">
          <span>0</span>
          <span>{formatDuration(totalMs * 0.25)}</span>
          <span>{formatDuration(totalMs * 0.5)}</span>
          <span>{formatDuration(totalMs * 0.75)}</span>
          <span>{formatDuration(totalMs)}</span>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div
          className="sticky top-0 z-[1] grid border-b border-[var(--c-border)] bg-[var(--c-table-head-bg)] px-4 py-1.5 text-[10px] font-semibold uppercase tracking-[0.05em] text-[var(--c-text-muted)]"
          style={{ gridTemplateColumns: '90px 70px 70px 220px minmax(0, 1fr)' }}
        >
          <span>Offset</span>
          <span>Type</span>
          <span>Level</span>
          <span>Span</span>
          <span>Message</span>
        </div>

        {filteredRows.length === 0 ? (
          <InspectorEmptyState>
            No {filter === 'all' ? '' : `${filter} `}events recorded for this trace.
          </InspectorEmptyState>
        ) : (
          filteredRows.map((row) => {
            const canSelectSpan = Boolean(row.spanId && spanIndex.has(row.spanId));
            return (
              <button
                key={row.id}
                type="button"
                disabled={!canSelectSpan}
                onClick={
                  canSelectSpan
                    ? () => selectSpanAndShowDetails(row.spanId!)
                    : undefined
                }
                className={`grid w-full items-baseline gap-0 border-b border-[var(--c-border-subtle)] px-4 py-1.5 text-left text-xs transition ${
                  canSelectSpan ? 'cursor-pointer hover:bg-[var(--c-row-hover-bg)]' : 'cursor-default'
                }`}
                style={{ gridTemplateColumns: '90px 70px 70px 220px minmax(0, 1fr)' }}
              >
                <span className="font-mono tabular-nums text-[var(--c-text-muted)]">
                  {formatOffset(row.offsetMs)}
                </span>
                <span
                  className="font-mono text-[11px] font-medium"
                  style={{ color: typeColor[row.type] }}
                >
                  {row.type}
                </span>
                <span
                  className="font-mono text-[10.5px] font-semibold uppercase"
                  style={{ color: levelColor[row.level] }}
                >
                  {row.level}
                </span>
                <span className="truncate font-mono text-[var(--c-text-secondary)]">
                  {row.spanName}
                </span>
                <span className="min-w-0 text-[var(--c-text-primary)]">
                  {row.message}
                  {row.meta ? (
                    <span className="ml-2.5 font-mono text-[11px] text-[var(--c-text-muted)]">
                      {row.meta}
                    </span>
                  ) : null}
                </span>
              </button>
            );
          })
        )}
      </div>
    </div>
  );
}

function TimelinePhaseRibbon({
  phases,
  totalMs,
}: {
  phases: TimelinePhase[];
  totalMs: number;
}) {
  if (totalMs <= 0 || phases.length === 0) return null;
  return (
    <>
      {phases.map((phase) => {
        const left = (phase.startMs / totalMs) * 100;
        const width = Math.max((phase.durationMs / totalMs) * 100, 0.4);
        const tone =
          phase.status === 'FAILED'
            ? { bg: 'rgba(239,68,68,0.16)', border: 'var(--c-bar-failed)' }
            : phase.status === 'STARTED'
              ? { bg: 'rgba(59,130,246,0.16)', border: 'var(--c-bar-running)' }
              : { bg: 'rgba(16,185,129,0.18)', border: 'var(--c-bar-success)' };
        return (
          <div
            key={phase.id}
            className="absolute top-1 bottom-1 pointer-events-auto"
            style={{
              left: `${left}%`,
              width: `${width}%`,
              background: tone.bg,
              borderLeft: `2px solid ${tone.border}`,
            }}
            title={`${phase.name} · ${formatDuration(phase.durationMs)}`}
          />
        );
      })}
    </>
  );
}
