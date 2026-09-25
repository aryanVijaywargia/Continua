import { useMemo, useState } from 'react';
import {
  SLOW_STEP_THRESHOLD_MS,
  formatShareOfTrace,
  isSlowStep,
} from '../../components/trace/traceSteps';
import type { StepDurations } from '../../components/trace/useStepDurations';
import { calculateDuration, formatDerivedDuration } from '../../utils/format';
import { TraceStatePanels } from './TraceStatePanels';
import { useTraceDetailWorkspace } from './traceDetailWorkspaceContext';

/** Narrow-width step list, sorted by duration, with fast steps grouped. */
export function MobileStepList({
  durations,
  onOpenStep,
}: {
  durations: StepDurations;
  onOpenStep: (spanId: string) => void;
}) {
  const { selectedSpanId, spans, trace } = useTraceDetailWorkspace();
  const [showFast, setShowFast] = useState(false);
  const traceMs = calculateDuration(trace.started_at, trace.ended_at);

  const { fast, slow } = useMemo(() => {
    const sorted = [...spans].sort(
      (a, b) =>
        (durations.byId.get(b.span_id)?.ms ?? -1) - (durations.byId.get(a.span_id)?.ms ?? -1)
    );
    return {
      slow: sorted.filter((span) => {
        const duration = durations.byId.get(span.span_id);
        return !duration || isSlowStep(duration);
      }),
      fast: sorted.filter((span) => {
        const duration = durations.byId.get(span.span_id);
        return duration ? !isSlowStep(duration) : false;
      }),
    };
  }, [durations, spans]);

  const shown = showFast ? [...slow, ...fast] : slow;

  return (
    <div className="min-h-0 flex-1 overflow-y-auto">
      <div className="p-4 empty:hidden">
        <TraceStatePanels />
      </div>
      <p className="border-b border-[var(--c-border)] px-4 py-2 text-[11px] text-[var(--c-text-muted)]">
        {durations.anyDerived ? 'Sorted by derived duration' : 'Sorted by duration'} ·{' '}
        {spans.length} {spans.length === 1 ? 'step' : 'steps'}
      </p>
      {spans.length === 0 ? (
        <p className="px-4 py-6 text-sm text-[var(--c-text-muted)]">No steps were recorded for this trace.</p>
      ) : null}
      <ul>
        {shown.map((span) => {
          const duration = durations.byId.get(span.span_id);
          const share = formatShareOfTrace(duration?.ms ?? null, traceMs);
          const isSelected = span.span_id === selectedSpanId;
          return (
            <li key={span.id} className="border-b border-[var(--c-border-subtle)]">
              <button
                type="button"
                aria-label={`Select step ${span.name}`}
                aria-current={isSelected ? 'true' : undefined}
                onClick={() => onOpenStep(span.span_id)}
                className={`flex w-full items-center gap-3 border-l-2 px-4 py-2.5 text-left ${
                  isSelected
                    ? 'border-[var(--c-accent)] bg-[var(--c-row-selected-bg)]'
                    : 'border-transparent hover:bg-[var(--c-row-hover-bg)]'
                }`}
              >
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-mono text-[12.5px] text-[var(--c-text-primary)]">
                    {span.name}
                  </span>
                  <span className="mt-0.5 block text-[11px] text-[var(--c-text-muted)]">
                    {span.status === 'FAILED' ? 'Failed · ' : ''}
                    {share ? `${share} of the trace` : span.kind.toLowerCase()}
                  </span>
                </span>
                <span className="shrink-0 font-mono text-xs tabular-nums text-[var(--c-text-secondary)]">
                  {formatDerivedDuration(duration?.ms ?? null)}
                </span>
              </button>
            </li>
          );
        })}
      </ul>
      {fast.length > 0 ? (
        <button
          type="button"
          aria-expanded={showFast}
          onClick={() => setShowFast((value) => !value)}
          className="w-full px-4 py-3 text-left text-xs font-medium text-[var(--c-accent-text)] hover:underline"
        >
          {showFast
            ? `Hide the ${fast.length} steps under ${SLOW_STEP_THRESHOLD_MS} ms`
            : `${fast.length} ${fast.length === 1 ? 'step' : 'steps'} under ${SLOW_STEP_THRESHOLD_MS} ms — show all`}
        </button>
      ) : null}
    </div>
  );
}
