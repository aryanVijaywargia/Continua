import type { Span, TimelineEvent } from '../../api/client';
import {
  calculateDuration,
  formatDuration,
  formatRelativeTime,
  formatTimestamp,
} from '../../utils/format';
import {
  formatRunningStateBasis,
  getRunningStatePanelTone,
  getRunningStateSummary,
  resolveDeclaredWaitKind,
} from '../../utils/runningStateSummary';
import type { OpenWait, WaitStallAssessment } from '../../utils/waitStallAnalysis';

export function RunningStatePanel({
  assessment,
  events,
  openWaits,
  spanIndex,
  onSelectSpan,
}: {
  assessment: WaitStallAssessment;
  events: TimelineEvent[];
  openWaits: OpenWait[];
  spanIndex: ReadonlyMap<string, Span>;
  onSelectSpan: (spanId: string) => void;
}) {
  const waitKind = resolveDeclaredWaitKind(assessment, events);
  const panelTone = getRunningStatePanelTone(assessment.classification);
  const summary = getRunningStateSummary(assessment);
  const orderedOpenWaits = [...openWaits].reverse();

  return (
    <section className={`rounded-xl border px-4 py-4 shadow-sm ${panelTone}`}>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-[11px] font-semibold uppercase tracking-[0.18em]">
            Running state
          </div>
          <div className="mt-2 flex flex-wrap items-center gap-2">
            <h2 className="text-base font-bold">{summary.label}</h2>
            <span className="rounded-full border border-current/15 px-2.5 py-1 text-[11px] font-medium uppercase tracking-[0.14em]">
              {formatRunningStateBasis(assessment.basis)}
            </span>
          </div>
          <p className="mt-2 max-w-3xl text-sm leading-6">{summary.copy}</p>
        </div>

        {assessment.decisiveSpanId ? (
          <button
            type="button"
            className="rounded-full border border-current/20 bg-[var(--continua-surface-elevated)]/70 px-3 py-1.5 text-xs font-medium transition hover:bg-[var(--continua-surface-elevated)]"
            onClick={() => onSelectSpan(assessment.decisiveSpanId!)}
          >
            Jump to {assessment.decisiveSpanName ?? assessment.decisiveSpanId}
          </button>
        ) : null}
      </div>

      <div className="mt-4 flex flex-wrap gap-4 text-xs">
        {waitKind ? (
          <div>
            <div className="font-semibold uppercase tracking-[0.16em] opacity-75">
              Current wait
            </div>
            <div className="mt-1 text-sm font-medium">{waitKind}</div>
          </div>
        ) : null}
        {assessment.latestActivityAt ? (
          <div>
            <div className="font-semibold uppercase tracking-[0.16em] opacity-75">
              Latest activity
            </div>
            <div className="mt-1 text-sm font-medium">
              {formatTimestamp(assessment.latestActivityAt)} (
              {formatRelativeTime(assessment.latestActivityAt)})
            </div>
          </div>
        ) : null}
        {assessment.runtimeMs !== null ? (
          <div>
            <div className="font-semibold uppercase tracking-[0.16em] opacity-75">
              Runtime
            </div>
            <div className="mt-1 text-sm font-medium">
              {formatDuration(assessment.runtimeMs)}
            </div>
          </div>
        ) : null}
        {assessment.inactivityMs !== null ? (
          <div>
            <div className="font-semibold uppercase tracking-[0.16em] opacity-75">
              Inactivity
            </div>
            <div className="mt-1 text-sm font-medium">
              {formatDuration(assessment.inactivityMs)}
            </div>
          </div>
        ) : null}
      </div>

      {orderedOpenWaits.length > 0 ? (
        <div className="mt-4 border-t border-current/15 pt-4">
          <div className="text-[11px] font-semibold uppercase tracking-[0.18em] opacity-75">
            Open waits
          </div>
          <div className="mt-3 space-y-3">
            {orderedOpenWaits.map((openWait) => (
              <OpenWaitRow
                key={openWait.event.id}
                openWait={openWait}
                spanIndex={spanIndex}
                onSelectSpan={onSelectSpan}
              />
            ))}
          </div>
        </div>
      ) : null}
    </section>
  );
}

function OpenWaitRow({
  openWait,
  spanIndex,
  onSelectSpan,
}: {
  openWait: OpenWait;
  spanIndex: ReadonlyMap<string, Span>;
  onSelectSpan: (spanId: string) => void;
}) {
  const waitTitle =
    openWait.details.waitKind === 'human_approval' ? 'Approval gate' : 'Wait gate';
  const openDurationMs = calculateDuration(openWait.event.timestamp, undefined);
  const hasNavigableSpan = Boolean(
    openWait.event.span_id && spanIndex.has(openWait.event.span_id)
  );

  return (
    <div className="rounded-lg border border-current/15 bg-[var(--continua-surface)]/60 p-3">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-sm font-semibold">{waitTitle}</div>
          <div className="mt-1 flex flex-wrap items-center gap-2 text-xs">
            <span className="rounded-full border border-current/15 px-2.5 py-1 font-medium">
              {openWait.details.waitKind}
            </span>
            {openWait.details.waitId ? (
              <span className="rounded-full border border-current/15 px-2.5 py-1 font-mono">
                {openWait.details.waitId}
              </span>
            ) : null}
          </div>
        </div>

        {hasNavigableSpan ? (
          <button
            type="button"
            className="rounded-full border border-current/20 bg-[var(--continua-surface-elevated)]/70 px-3 py-1.5 text-xs font-medium transition hover:bg-[var(--continua-surface-elevated)]"
            onClick={() => onSelectSpan(openWait.event.span_id!)}
          >
            Jump to {openWait.event.span_name ?? openWait.event.span_id}
          </button>
        ) : null}
      </div>

      <div className="mt-3 flex flex-wrap gap-4 text-xs">
        <div>
          <div className="font-semibold uppercase tracking-[0.16em] opacity-75">
            Entered
          </div>
          <div className="mt-1 text-sm font-medium">
            {formatTimestamp(openWait.event.timestamp)}
          </div>
        </div>
        <div>
          <div className="font-semibold uppercase tracking-[0.16em] opacity-75">
            Open duration
          </div>
          <div className="mt-1 text-sm font-medium">
            {formatDuration(openDurationMs)}
          </div>
        </div>
      </div>

      {openWait.event.message ? (
        <p className="mt-3 text-sm leading-6 opacity-90">{openWait.event.message}</p>
      ) : null}
    </div>
  );
}
