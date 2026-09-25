import { Link } from 'react-router-dom';
import { ChevronLeft, Download, PanelRight, RotateCcw, Waypoints, Zap } from 'lucide-react';
import type { Trace } from '../../api/client';
import { CopyButton } from '../../components/CopyButton';
import { ReadOnlyBadge, UnverifiedPill } from '../../components/DataState';
import { Btn, Chip, StatusDot } from '../../components/DebuggerKit';
import { hasRecordedUsage } from '../../components/trace/traceSteps';
import {
  calculateDuration,
  formatCost,
  formatDerivedDuration,
  formatTokens,
} from '../../utils/format';
import { appendProjectToPath } from '../../utils/projectSearchParams';
import { EngineWaitStateSummary } from './TraceDetailChrome';
import { TraceLineageBreadcrumb } from './TraceLineagePanels';
import { useTraceDetailWorkspace } from './traceDetailWorkspaceContext';

function returnLabel(returnTo: string): string {
  if (returnTo.startsWith('/sessions/')) return 'Session';
  if (returnTo.startsWith('/engine/runs')) return 'Engine Runs';
  return 'Traces';
}

function Divider() {
  return <span aria-hidden="true" className="h-3.5 w-px bg-[var(--c-border)]" />;
}

/** Trace identity, status, honesty pills, lineage, and page actions. */
export function TraceDetailHeader({
  isDesktop,
  isNarrow,
  isTraceContextOpen,
  lineageChain,
  lineageLoading,
  onShowReplay,
  onToggleTraceContext,
  replayPreviewEnabled,
}: {
  isDesktop: boolean;
  isNarrow: boolean;
  isTraceContextOpen: boolean;
  lineageChain: Trace[];
  lineageLoading: boolean;
  onShowReplay: () => void;
  onToggleTraceContext: () => void;
  replayPreviewEnabled: boolean;
}) {
  const {
    buildCopyTraceUrl,
    events,
    exportTrace,
    projectId,
    returnTo,
    spans,
    timelineStatus,
    trace,
  } = useTraceDetailWorkspace();

  const duration = formatDerivedDuration(calculateDuration(trace.started_at, trace.ended_at));
  const label = returnLabel(returnTo);
  const usageRecorded = hasRecordedUsage(trace);
  const totalTokens = (trace.total_tokens_in ?? 0) + (trace.total_tokens_out ?? 0);
  const status = timelineStatus ?? trace.status;

  const contextButton = (
    <button
      type="button"
      aria-expanded={isTraceContextOpen}
      aria-label="Trace Context"
      onClick={onToggleTraceContext}
      className="inline-flex h-7 items-center gap-1.5 rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-2.5 text-xs font-medium text-[var(--c-text-secondary)] transition hover:border-[var(--c-border-strong)] hover:text-[var(--c-text-primary)]"
    >
      <PanelRight aria-hidden="true" className="h-3.5 w-3.5" />
      {isNarrow ? null : isTraceContextOpen ? 'Hide context' : 'Trace context'}
    </button>
  );

  const engineExtras = (
    <>
      {isDesktop ? (
        <TraceLineageBreadcrumb
          chain={lineageChain}
          isLoading={lineageLoading}
          projectId={projectId}
          returnTo={returnTo}
          trace={trace}
        />
      ) : null}
      {trace.engine?.continued_from_trace_id || trace.engine?.continued_to_trace_id ? (
        <div className="mt-2 flex flex-wrap gap-2 text-sm">
          {trace.engine.continued_from_trace_id ? (
            <Link
              to={appendProjectToPath(`/traces/${trace.engine.continued_from_trace_id}`, projectId)}
              state={{ returnTo }}
              className="inline-flex items-center rounded border border-[var(--c-border)] bg-[var(--c-surface-muted)] px-2.5 py-1 text-xs font-medium text-[var(--c-text-secondary)] transition hover:border-[var(--c-border-strong)] hover:text-[var(--c-accent-text)]"
            >
              ← Previous run
            </Link>
          ) : null}
          {trace.engine.continued_to_trace_id ? (
            <Link
              to={appendProjectToPath(`/traces/${trace.engine.continued_to_trace_id}`, projectId)}
              state={{ returnTo }}
              className="inline-flex items-center rounded border border-[var(--c-border)] bg-[var(--c-surface-muted)] px-2.5 py-1 text-xs font-medium text-[var(--c-text-secondary)] transition hover:border-[var(--c-border-strong)] hover:text-[var(--c-accent-text)]"
            >
              Next run →
            </Link>
          ) : null}
        </div>
      ) : null}
      {trace.engine?.status === 'WAITING' ? <EngineWaitStateSummary engine={trace.engine} /> : null}
    </>
  );

  if (isNarrow) {
    return (
      <header className="border-b border-[var(--c-border)] bg-[var(--c-app-bg)] px-4 py-3">
        <div className="flex items-start gap-2">
          <Link
            to={returnTo}
            aria-label={`← ${label}`}
            className="mt-0.5 inline-flex h-6 w-6 shrink-0 items-center justify-center rounded text-[var(--c-text-secondary)] hover:bg-[var(--c-row-hover-bg)] hover:text-[var(--c-text-primary)]"
          >
            <ChevronLeft aria-hidden="true" className="h-4 w-4" />
          </Link>
          <div className="min-w-0 flex-1">
            <h1 className="truncate text-[15px] font-bold text-[var(--c-text-primary)]">{trace.name}</h1>
            <div className="mt-1 flex flex-wrap items-center gap-2 text-xs text-[var(--c-text-muted)]">
              <StatusDot status={status} />
              <span className="font-mono tabular-nums">{duration}</span>
              <span>
                {spans.length} {spans.length === 1 ? 'step' : 'steps'}
              </span>
              {trace.engine ? <Chip icon={Zap}>{trace.engine.definition_name}</Chip> : null}
            </div>
          </div>
          <ReadOnlyBadge />
          {contextButton}
        </div>
        {engineExtras}
      </header>
    );
  }

  return (
    <header className="border-b border-[var(--c-border)] bg-[var(--c-app-bg)] px-6 py-3">
      <div className="flex flex-wrap items-center gap-2 text-xs text-[var(--c-text-muted)]">
        <Link
          to={returnTo}
          aria-label={`← ${label}`}
          className="inline-flex items-center gap-1 font-medium text-[var(--c-text-secondary)] transition hover:text-[var(--c-accent-text)]"
        >
          ‹ {label}
        </Link>
        <span aria-hidden="true">›</span>
        <span className="font-mono">{trace.trace_id ?? trace.id}</span>
        <CopyButton
          aria-label="Copy Trace URL"
          getValue={buildCopyTraceUrl}
          idleLabel=""
          successLabel=""
          className="h-5 min-w-5 border-0 bg-transparent px-1 text-[var(--c-text-muted)] shadow-none hover:text-[var(--c-text-primary)]"
        />
      </div>

      <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-2">
        <h1 className="min-w-0 truncate text-[17px] font-bold tracking-[-0.01em] text-[var(--c-text-primary)]">
          {trace.name}
        </h1>
        <StatusDot status={status} />
        <Divider />
        <span
          className="font-mono text-xs tabular-nums text-[var(--c-text-secondary)]"
          title="Computed from the trace start and end timestamps."
        >
          {duration}
        </span>
        {trace.session_id ? (
          <>
            <Divider />
            <Link
              to={appendProjectToPath(`/sessions/${trace.session_id}`, projectId)}
              aria-label={`Open session ${trace.session_external_id ?? trace.session_id}`}
              className="inline-flex max-w-[16rem] items-center gap-1 text-xs text-[var(--c-text-secondary)] hover:text-[var(--c-accent-text)]"
            >
              <Waypoints aria-hidden="true" className="h-3.5 w-3.5 shrink-0" />
              <span className="truncate font-mono">{trace.session_external_id ?? trace.session_id}</span>
            </Link>
          </>
        ) : null}
        <span className="inline-flex h-5 items-center rounded border border-[var(--c-border)] bg-[var(--c-surface)] px-1.5 font-mono text-[11.5px] text-[var(--c-text-secondary)]">
          {spans.length} {spans.length === 1 ? 'span' : 'spans'} · {events.length}{' '}
          {events.length === 1 ? 'event' : 'events'}
        </span>
        {trace.engine ? <Chip icon={Zap}>{trace.engine.definition_name}</Chip> : null}
        {trace.error_count && trace.error_count > 0 ? (
          <Chip tone="error">
            {trace.error_count} error{trace.error_count === 1 ? '' : 's'}
          </Chip>
        ) : null}
        {usageRecorded ? (
          <Chip>
            {formatTokens(totalTokens)} tokens · {formatCost(trace.total_cost_usd)}
          </Chip>
        ) : (
          <UnverifiedPill title="Token and cost totals are absent or zero, so this trace cannot prove its usage.">
            Usage not verified
          </UnverifiedPill>
        )}

        <div className="ml-auto flex flex-wrap items-center gap-1.5">
          <ReadOnlyBadge />
          {replayPreviewEnabled ? (
            <Btn kind="secondary" leadingIcon={RotateCcw} size="sm" type="button" onClick={onShowReplay}>
              Replay
            </Btn>
          ) : null}
          <Btn kind="secondary" leadingIcon={Download} size="sm" type="button" onClick={exportTrace}>
            Export JSON
          </Btn>
          {contextButton}
        </div>
      </div>

      {engineExtras}
    </header>
  );
}
