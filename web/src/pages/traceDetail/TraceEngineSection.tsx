import { useEffect, useRef, useState, type ReactNode } from 'react';
import { useQuery } from '@tanstack/react-query';
import { Zap } from 'lucide-react';
import {
  fetchEngineRunHistory,
  fetchEngineRunResult,
  type EngineHistoryEvent,
  type EnginePendingWorkResponse,
  type EngineRunStatus,
  type Span,
  type TimelineEvent,
  type TraceDetail,
} from '../../api/client';
import { Btn, Chip } from '../../components/DebuggerKit';
import { EngineProjectionBanner } from '../../components/EngineProjectionBanner';
import {
  buildEngineStateMachine,
  isTerminalEngineStatus,
  type StateMachineStep,
} from '../../utils/engineStateMachine';
import { formatTimestamp } from '../../utils/format';
import type { OpenWait, WaitStallAssessment } from '../../utils/waitStallAnalysis';
import { EngineControlBar } from '../EngineControlBar';
import { EnginePendingWorkPanel } from '../EnginePendingWorkPanel';
import {
  CompactPayloadInspector,
  InspectorEmptyState,
} from './CompactPayloadInspector';
import { RunningStatePanel } from './RunningStatePanel';
import { useTraceDetailWorkspace } from './traceDetailWorkspaceContext';

type EngineStatePane = 'overview' | 'pending' | 'history' | 'result';

const ENGINE_PANES: Array<{ id: EngineStatePane; label: string }> = [
  { id: 'overview', label: 'Overview' },
  { id: 'pending', label: 'Pending' },
  { id: 'history', label: 'Engine history' },
  { id: 'result', label: 'Result' },
];

/** Engine projection, state machine, control actions, and journal panes. */
export function TraceEngineSection() {
  const {
    events,
    openWaits,
    pendingWork,
    selectSpanAndShowDetails,
    spanIndex,
    trace,
    traceId,
    waitStallAssessment,
  } = useTraceDetailWorkspace();
  const engine = trace.engine;
  const [pane, setPane] = useState<EngineStatePane>('overview');
  const [selectedEventId, setSelectedEventId] = useState<string | null>(null);

  if (!engine) {
    return <InspectorEmptyState>This trace is not backed by an engine run.</InspectorEmptyState>;
  }

  const journalEvents = events.filter((event) => {
    return (
      event.event_type === 'snapshot_marker' ||
      event.event_type === 'state_change' ||
      event.event_type === 'decision' ||
      event.event_type === 'effect' ||
      event.event_type === 'wait' ||
      event.event_type === 'span_failed'
    );
  });

  const checkpointCount = events.filter((event) => event.event_type === 'snapshot_marker').length;
  const stateChangeCount = events.filter((event) => event.event_type === 'state_change').length;
  const decisionCount = events.filter((event) => event.event_type === 'decision').length;
  const effectCount = events.filter((event) => event.event_type === 'effect').length;
  const waitCount = events.filter((event) => event.event_type === 'wait').length;
  const pendingCount =
    (pendingWork.data?.activities.length ?? 0) +
    (pendingWork.data?.timers.length ?? 0) +
    (pendingWork.data?.signals.length ?? 0);
  const hasFailure = Boolean(engine.failure);
  const waitState = pendingWork.data?.current_wait ?? engine.wait_state ?? null;
  const hasWait = Boolean(waitState && Object.keys(waitState).length > 0);

  const paneCounts: Record<EngineStatePane, number> = {
    overview: journalEvents.length,
    pending: pendingCount,
    history: 0,
    result: isTerminalEngineStatus(engine.status) || hasFailure ? 1 : 0,
  };

  const selectedEvent = selectedEventId
    ? events.find((event) => event.id === selectedEventId) ?? null
    : null;

  const stateMachine = buildEngineStateMachine(engine.status);

  const formatRelative = (iso?: string) => {
    if (!iso) return '—';
    const ts = new Date(iso).getTime();
    if (Number.isNaN(ts)) return '—';
    return formatTimestamp(iso);
  };

  return (
    <div className="flex min-h-0 flex-1">
      <div className="flex min-w-0 flex-1 flex-col overflow-y-auto px-5 py-4">
        <div className="rounded-lg border border-[var(--c-border)] bg-[var(--c-surface)] px-4 py-3">
          <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
            <div className="flex items-center gap-2">
              <Zap className="h-3.5 w-3.5 text-[var(--c-accent-text)]" />
              <span className="font-mono text-[13.5px] font-semibold text-[var(--c-text-primary)]">
                {engine.run_id}
              </span>
              <Chip tone={hasFailure ? 'error' : engine.status === 'RUNNING' ? 'accent' : 'muted'}>
                {engine.status}
              </Chip>
            </div>
          </div>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <EngineKvSmall label="Definition" value={engine.definition_name ?? '—'} />
            <EngineKvSmall label="Version" value={engine.definition_version ?? '—'} />
            <EngineKvSmall label="Instance" value={engine.instance_key ?? '—'} />
            <EngineKvSmall label="Projection" value={engine.projection_state ?? '—'} />
            <EngineKvSmall
              label="Parent run"
              value={engine.parent_run_id ?? '—'}
            />
            <EngineKvSmall label="Created" value={formatRelative(engine.created_at)} />
            <EngineKvSmall label="Updated" value={formatRelative(engine.updated_at)} />
            <EngineKvSmall label="Closed" value={formatRelative(engine.completed_at)} />
          </div>
        </div>

        <div className="mt-4 mb-2 flex items-baseline justify-between">
          <h3 className="text-[12.5px] font-semibold text-[var(--c-text-primary)]">State machine</h3>
          <span className="text-[11px] text-[var(--c-text-muted)]">Workflow lifecycle</span>
        </div>
        <div className="rounded-lg border border-[var(--c-border)] bg-[var(--c-surface)] px-4 py-5">
          <StateMachine steps={stateMachine} />
        </div>

        <div className="mt-4 space-y-3">
          <EngineControlBar engine={engine} traceId={traceId} />
          <EngineProjectionBanner projectionState={engine.projection_state} />
        </div>

        <div className="mt-4 flex border-b border-[var(--c-border)]">
          {ENGINE_PANES.map((tab) => {
            const active = pane === tab.id;
            return (
              <button
                key={tab.id}
                type="button"
                onClick={() => setPane(tab.id)}
                aria-pressed={active}
                className={`-mb-px flex items-center gap-1.5 px-3.5 py-2 text-[12.5px] transition ${
                  active
                    ? 'border-b-2 border-[var(--c-accent)] font-semibold text-[var(--c-text-primary)]'
                    : 'border-b-2 border-transparent font-medium text-[var(--c-text-secondary)] hover:text-[var(--c-text-primary)]'
                }`}
              >
                {tab.label}
                <span className="font-mono text-[10.5px] text-[var(--c-text-muted)]">
                  {paneCounts[tab.id]}
                </span>
              </button>
            );
          })}
        </div>

        <div className="mt-3">
          {pane === 'overview' ? (
            <EngineOverviewPane
              checkpointCount={checkpointCount}
              decisionCount={decisionCount}
              effectCount={effectCount}
              engine={engine}
              events={journalEvents}
              hasWait={hasWait}
              onSelectEvent={(event) => {
                setSelectedEventId(event.id);
                if (event.span_id && spanIndex.has(event.span_id)) {
                  selectSpanAndShowDetails(event.span_id);
                }
              }}
              runningStateAssessment={waitStallAssessment}
              openWaits={openWaits}
              selectedEventId={selectedEventId}
              spanIndex={spanIndex}
              stateChangeCount={stateChangeCount}
              waitCount={waitCount}
              waitState={waitState}
              onSelectSpan={selectSpanAndShowDetails}
            />
          ) : pane === 'pending' ? (
            pendingWork.isLoading ? (
              <EngineEmptyCard>Loading pending work…</EngineEmptyCard>
            ) : pendingWork.isError ? (
              <EngineEmptyCard>{pendingWork.errorMessage}</EngineEmptyCard>
            ) : (
              <EnginePendingWorkPanel
                data={pendingWork.data}
                isError={pendingWork.isError}
                isLoading={pendingWork.isLoading}
                errorMessage={pendingWork.errorMessage}
              />
            )
          ) : pane === 'history' ? (
            <EngineHistoryPane runId={engine.run_id} />
          ) : pane === 'result' ? (
            <EngineResultPane runId={engine.run_id} status={engine.status} />
          ) : null}
        </div>

        {stateChangeCount > 0 || checkpointCount > 0 ? (
          <p className="mt-3 text-[11px] text-[var(--c-text-muted)]">
            {checkpointCount} checkpoint(s) · {stateChangeCount} state change(s) recorded.
          </p>
        ) : null}
      </div>

      <aside className="hidden w-[360px] shrink-0 flex-col border-l border-[var(--c-border)] xl:flex">
        <div className="border-b border-[var(--c-border)] px-4 py-3">
          <div className="text-[10.5px] font-semibold uppercase tracking-[0.05em] text-[var(--c-text-muted)]">
            {selectedEvent ? 'Selected event' : 'Engine projection'}
          </div>
          {selectedEvent ? (
            <>
              <div className="mt-1 font-mono text-[13px] font-semibold text-[var(--c-text-primary)]">
                {selectedEvent.event_type}
              </div>
              <div className="mt-1 font-mono text-[11px] text-[var(--c-text-muted)]">
                seq={selectedEvent.sequence ?? '—'} · {formatTimestamp(selectedEvent.timestamp)}
              </div>
            </>
          ) : (
            <div className="mt-1 font-mono text-[11px] text-[var(--c-text-muted)]">
              {engine.run_id}
            </div>
          )}
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto px-3 py-3">
          {selectedEvent ? (
            <CompactPayloadInspector value={selectedEvent.payload ?? {}} />
          ) : (
            <CompactPayloadInspector value={engine} />
          )}
        </div>
      </aside>
    </div>
  );
}

function StateMachine({ steps }: { steps: StateMachineStep[] }) {
  return (
    <div className="flex items-center justify-between">
      {steps.map((step, index) => {
        const ringColor = step.error
          ? 'var(--c-red)'
          : step.warn
            ? 'var(--c-amber)'
            : step.done
              ? 'var(--c-text-secondary)'
              : 'var(--c-border)';
        const fillColor = step.error
          ? 'var(--c-red-faint)'
          : step.warn
            ? 'var(--c-amber-faint)'
            : step.done
              ? 'var(--c-surface-muted)'
              : 'var(--c-surface)';
        const textColor = step.error
          ? 'var(--c-red-text)'
          : step.warn
            ? 'var(--c-amber-text)'
            : step.done
              ? 'var(--c-text-primary)'
              : 'var(--c-text-muted)';
        const labelColor = step.error ? 'var(--c-red-text)' : 'var(--c-text-secondary)';
        return (
          <div key={step.id} className="flex flex-1 items-center last:flex-initial">
            <div className="flex shrink-0 flex-col items-center gap-2">
              <div
                className="flex h-7 w-7 items-center justify-center rounded-full font-mono text-[11px] font-semibold"
                style={{
                  border: `1.5px solid ${ringColor}`,
                  background: fillColor,
                  color: textColor,
                  boxShadow: step.current && step.error ? '0 0 0 4px rgba(239,68,68,0.18)' : 'none',
                }}
              >
                {step.error ? '!' : step.done ? '✓' : index + 1}
              </div>
              <span
                className="whitespace-nowrap text-[11px]"
                style={{ color: labelColor, fontWeight: step.current ? 600 : 500 }}
              >
                {step.label}
              </span>
            </div>
            {index < steps.length - 1 ? (
              <div
                className="-mb-5 h-px flex-1"
                style={{
                  background: steps[index + 1].done ? 'var(--c-text-muted)' : 'var(--c-border)',
                }}
              />
            ) : null}
          </div>
        );
      })}
    </div>
  );
}

function EngineKvSmall({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-px">
      <span className="text-[10px] font-semibold uppercase tracking-[0.05em] text-[var(--c-text-muted)]">
        {label}
      </span>
      <span className="truncate font-mono text-xs text-[var(--c-text-primary)]">{value}</span>
    </div>
  );
}

function EngineOverviewPane({
  checkpointCount,
  decisionCount,
  effectCount,
  engine,
  events,
  hasWait,
  onSelectEvent,
  onSelectSpan,
  openWaits,
  runningStateAssessment,
  selectedEventId,
  spanIndex,
  stateChangeCount,
  waitCount,
  waitState,
}: {
  checkpointCount: number;
  decisionCount: number;
  effectCount: number;
  engine: NonNullable<TraceDetail['engine']>;
  events: TimelineEvent[];
  hasWait: boolean;
  onSelectEvent: (event: TimelineEvent) => void;
  onSelectSpan: (spanId: string) => void;
  openWaits: OpenWait[];
  runningStateAssessment: WaitStallAssessment | null;
  selectedEventId: string | null;
  spanIndex: ReadonlyMap<string, Span>;
  stateChangeCount: number;
  waitCount: number;
  waitState: EnginePendingWorkResponse['current_wait'] | NonNullable<TraceDetail['engine']>['wait_state'] | null;
}) {
  return (
    <div className="space-y-4">
      {hasWait && runningStateAssessment ? (
        <RunningStatePanel
          assessment={runningStateAssessment}
          events={events}
          openWaits={openWaits}
          spanIndex={spanIndex}
          onSelectSpan={onSelectSpan}
        />
      ) : hasWait && waitState ? (
        <div className="rounded-lg border border-[var(--c-border)] bg-[var(--c-surface)] p-3.5">
          <div className="text-[10.5px] font-semibold uppercase tracking-[0.05em] text-[var(--c-text-muted)]">
            Current wait
          </div>
          <div className="mt-3 rounded border border-[var(--c-border-subtle)] bg-[var(--c-app-bg)] p-3">
            <CompactPayloadInspector value={waitState} />
          </div>
        </div>
      ) : (
        <EngineEmptyCard>No wait state for this run.</EngineEmptyCard>
      )}

      {engine.failure ? (
        <div className="rounded-lg border border-[var(--c-red-border)] bg-[var(--c-red-faint)] p-3.5">
          <div className="text-[10.5px] font-semibold uppercase tracking-[0.05em] text-[var(--c-red-text)]">
            Failure
          </div>
          <div className="mt-2 font-mono text-xs text-[var(--c-red-text)]">
            {engine.failure.error_code}
          </div>
          <p className="mt-2 text-sm text-[var(--c-text-primary)]">
            {engine.failure.error_message}
          </p>
        </div>
      ) : null}

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
        <EngineKvSmall label="Run" value={engine.run_id} />
        <EngineKvSmall label="Instance" value={engine.instance_key ?? '—'} />
        <EngineKvSmall label="Definition" value={engine.definition_name ?? '—'} />
        <EngineKvSmall label="Version" value={engine.definition_version ?? '—'} />
        <EngineKvSmall label="Updated" value={formatTimestamp(engine.updated_at)} />
      </div>

      <div className="grid gap-3 sm:grid-cols-5">
        <JournalCount label="State changes" value={stateChangeCount} />
        <JournalCount label="Decisions" value={decisionCount} />
        <JournalCount label="Effects" value={effectCount} />
        <JournalCount label="Waits" value={waitCount} />
        <JournalCount label="Snapshots" value={checkpointCount} />
      </div>

      {events.length === 0 ? (
        <EngineEmptyCard>No projected journal summary is available for this run.</EngineEmptyCard>
      ) : (
        <ProjectedJournalSummary
          events={events}
          onSelectEvent={onSelectEvent}
          selectedEventId={selectedEventId}
        />
      )}
    </div>
  );
}

function JournalCount({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-lg border border-[var(--c-border)] bg-[var(--c-surface)] px-3 py-2">
      <div className="text-[10.5px] font-semibold uppercase tracking-[0.05em] text-[var(--c-text-muted)]">
        {label}
      </div>
      <div className="mt-1 font-mono text-lg font-semibold text-[var(--c-text-primary)]">
        {value}
      </div>
    </div>
  );
}

function ProjectedJournalSummary({
  events,
  onSelectEvent,
  selectedEventId,
}: {
  events: TimelineEvent[];
  onSelectEvent: (event: TimelineEvent) => void;
  selectedEventId: string | null;
}) {
  return (
    <div className="overflow-hidden rounded-lg border border-[var(--c-border)] bg-[var(--c-surface)]">
      <div
        className="grid border-b border-[var(--c-border)] bg-[var(--c-table-head-bg)] px-3.5 py-1.5 text-[10px] font-semibold uppercase tracking-[0.05em] text-[var(--c-text-muted)]"
        style={{ gridTemplateColumns: '50px 110px 200px minmax(0,1fr)' }}
      >
        <span>Seq</span>
        <span>Time</span>
        <span>Event</span>
        <span>Detail</span>
      </div>
      {events.slice(0, 12).map((event, index) => {
        const isCheckpoint = event.event_type === 'snapshot_marker';
        const isError = event.event_type === 'span_failed';
        return (
          <button
            key={event.id}
            type="button"
            onClick={() => onSelectEvent(event)}
            className={`grid w-full items-baseline gap-0 px-3.5 py-1.5 text-left font-mono text-[11.5px] transition ${
              selectedEventId === event.id
                ? 'bg-[var(--c-row-selected-bg)]'
                : 'hover:bg-[var(--c-row-hover-bg)]'
            }`}
            style={{
              gridTemplateColumns: '50px 110px 200px minmax(0,1fr)',
              borderBottom:
                index < Math.min(events.length, 12) - 1
                  ? '1px solid var(--c-border-subtle)'
                  : 'none',
            }}
          >
            <span className="tabular-nums text-[var(--c-text-muted)]">
              {event.sequence ?? index + 1}
            </span>
            <span className="text-[var(--c-text-muted)]">
              {formatTimestamp(event.timestamp)}
            </span>
            <span
              className="truncate font-semibold"
              style={{
                color: isError
                  ? 'var(--c-red-text)'
                  : isCheckpoint
                    ? 'var(--c-accent-text)'
                    : 'var(--c-text-primary)',
              }}
            >
              {event.event_type}
            </span>
            <span className="truncate text-[var(--c-text-secondary)]">
              {event.message ?? event.span_name ?? '—'}
            </span>
          </button>
        );
      })}
    </div>
  );
}

function EngineHistoryPane({ runId }: { runId: string }) {
  const [after, setAfter] = useState<number | undefined>();
  const [events, setEvents] = useState<EngineHistoryEvent[]>([]);
  const loadedPageKeys = useRef(new Set<string>());
  const historyQuery = useQuery({
    queryKey: ['engineRunHistory', runId, after ?? null],
    queryFn: () => fetchEngineRunHistory(runId, { after, limit: 50 }),
  });

  useEffect(() => {
    if (!historyQuery.data) {
      return;
    }
    const pageKey = after === undefined ? 'initial' : String(after);
    if (loadedPageKeys.current.has(pageKey)) {
      return;
    }
    loadedPageKeys.current.add(pageKey);
    setEvents((current) => [...current, ...historyQuery.data.events]);
  }, [after, historyQuery.data]);

  if (historyQuery.isLoading && events.length === 0) {
    return <EngineEmptyCard>Loading engine history…</EngineEmptyCard>;
  }
  if (historyQuery.isError) {
    return <EngineEmptyCard>Engine history is temporarily unavailable.</EngineEmptyCard>;
  }
  if (historyQuery.data?.expired) {
    return <EngineEmptyCard>History expired. Retained history for this run has been purged.</EngineEmptyCard>;
  }
  if (events.length === 0) {
    return <EngineEmptyCard>No retained engine history events.</EngineEmptyCard>;
  }

  return (
    <div className="space-y-3">
      <div className="overflow-hidden rounded-lg border border-[var(--c-border)] bg-[var(--c-surface)]">
        {events.map((event, index) => (
          <div
            key={`${event.id}-${index}`}
            className="grid gap-3 border-b border-[var(--c-border-subtle)] px-3.5 py-2 last:border-b-0"
            style={{ gridTemplateColumns: '70px 150px minmax(0,1fr)' }}
          >
            <span className="font-mono text-xs text-[var(--c-text-muted)]">
              {event.sequence_no}
            </span>
            <span className="font-mono text-xs text-[var(--c-text-secondary)]">
              {formatTimestamp(event.created_at)}
            </span>
            <div className="min-w-0">
              <div className="font-mono text-xs font-semibold text-[var(--c-text-primary)]">
                {event.event_type}
              </div>
              {event.payload ? (
                <div className="mt-2 rounded border border-[var(--c-border-subtle)] bg-[var(--c-app-bg)] p-2">
                  <CompactPayloadInspector value={event.payload} />
                </div>
              ) : null}
            </div>
          </div>
        ))}
      </div>
      {historyQuery.data?.has_more && historyQuery.data.next_after !== undefined ? (
        <Btn
          kind="secondary"
          type="button"
          disabled={historyQuery.isFetching}
          onClick={() => setAfter(historyQuery.data?.next_after)}
        >
          {historyQuery.isFetching ? 'Loading…' : 'Load more'}
        </Btn>
      ) : null}
    </div>
  );
}

function EngineResultPane({
  runId,
  status,
}: {
  runId: string;
  status: EngineRunStatus;
}) {
  const terminal = isTerminalEngineStatus(status);
  const resultQuery = useQuery({
    queryKey: ['engineRunResult', runId],
    queryFn: () => fetchEngineRunResult(runId),
    enabled: terminal,
  });

  if (!terminal) {
    return <EngineEmptyCard>Result is not available until the run reaches a terminal state.</EngineEmptyCard>;
  }
  if (resultQuery.isLoading) {
    return <EngineEmptyCard>Loading engine result…</EngineEmptyCard>;
  }
  if (resultQuery.isError || !resultQuery.data) {
    return <EngineEmptyCard>Engine result is temporarily unavailable.</EngineEmptyCard>;
  }

  const result = resultQuery.data;
  if (result.status === 'COMPLETED') {
    return (
      <div className="rounded-lg border border-[var(--c-border)] bg-[var(--c-surface)] p-3.5">
        <div className="text-[10.5px] font-semibold uppercase tracking-[0.05em] text-[var(--c-text-muted)]">
          Completed result
        </div>
        <div className="mt-3 rounded border border-[var(--c-border-subtle)] bg-[var(--c-app-bg)] p-3">
          <CompactPayloadInspector value={result.result} />
        </div>
      </div>
    );
  }

  return (
    <div className="rounded-lg border border-[var(--c-red-border)] bg-[var(--c-red-faint)] p-3.5">
      <div className="text-[10.5px] font-semibold uppercase tracking-[0.05em] text-[var(--c-red-text)]">
        {result.status} result shell
      </div>
      {result.failure ? (
        <>
          <div className="mt-2 font-mono text-xs text-[var(--c-red-text)]">
            {result.failure.error_code}
          </div>
          <p className="mt-2 text-sm text-[var(--c-text-primary)]">
            {result.failure.error_message}
          </p>
        </>
      ) : (
        <p className="mt-2 text-sm text-[var(--c-text-primary)]">
          Workflow result payload is null for this terminal shell.
        </p>
      )}
    </div>
  );
}

function EngineEmptyCard({ children }: { children: ReactNode }) {
  return (
    <div className="rounded-lg border border-dashed border-[var(--c-border)] bg-[var(--c-surface)] px-4 py-6 text-center text-[12.5px] text-[var(--c-text-muted)]">
      {children}
    </div>
  );
}
