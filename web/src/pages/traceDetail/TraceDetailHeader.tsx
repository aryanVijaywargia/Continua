import { Link } from 'react-router-dom';
import { Download, ExternalLink, RotateCcw, Zap } from 'lucide-react';
import type { Trace } from '../../api/client';
import { CopyButton } from '../../components/CopyButton';
import { Btn, Chip, StatusDot } from '../../components/DebuggerKit';
import {
  calculateDuration,
  formatCost,
  formatDuration,
  formatRelativeTime,
  formatTokens,
} from '../../utils/format';
import { appendProjectToPath } from '../../utils/projectSearchParams';
import { EngineWaitStateSummary, TraceHeaderMetric } from './TraceDetailChrome';
import { TraceLineageBreadcrumb } from './TraceLineagePanels';
import { useTraceDetailWorkspace } from './traceDetailWorkspaceContext';

/** Trace identity, status, lineage, headline metrics, and page actions. */
export function TraceDetailHeader({
  isDesktop,
  isTraceContextOpen,
  lineageChain,
  lineageLoading,
  onShowReplay,
  onToggleTraceContext,
  replayPreviewEnabled,
}: {
  isDesktop: boolean;
  isTraceContextOpen: boolean;
  lineageChain: Trace[];
  lineageLoading: boolean;
  onShowReplay: () => void;
  onToggleTraceContext: () => void;
  replayPreviewEnabled: boolean;
}) {
  const {
    buildCopyTraceUrl,
    exportTrace,
    projectId,
    returnTo,
    spans,
    timelineStatus,
    trace,
  } = useTraceDetailWorkspace();

  const duration = calculateDuration(trace.started_at, trace.ended_at);
  const totalTokens = (trace.total_tokens_in ?? 0) + (trace.total_tokens_out ?? 0);

  return (
    <header className="border-b border-[var(--c-border)] bg-[var(--c-app-bg)] px-6 py-3.5">
      <div className="flex flex-col gap-5 xl:flex-row xl:items-start xl:justify-between">
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2 text-xs text-[var(--c-text-muted)]">
            <Link
              to={returnTo}
              aria-label={
                returnTo.startsWith('/sessions/')
                  ? '← Session'
                  : returnTo.startsWith('/engine/runs')
                    ? '← Engine Runs'
                    : '← Traces'
              }
              className="inline-flex items-center gap-1 font-medium text-[var(--c-text-secondary)] transition hover:text-[var(--c-accent-text)]"
            >
              ‹ {returnTo.startsWith('/sessions/')
                ? 'Session'
                : returnTo.startsWith('/engine/runs')
                  ? 'Engine Runs'
                  : 'Traces'}
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

          <div className="mt-2 flex flex-wrap items-center gap-2.5">
            <h1 className="truncate font-mono text-lg font-semibold tracking-[-0.01em] text-[var(--c-text-primary)]">
              {trace.name}
            </h1>
            <StatusDot status={timelineStatus ?? trace.status} />
            {trace.engine ? (
              <Chip icon={Zap}>
                {trace.engine.definition_name}
              </Chip>
            ) : null}
            {trace.error_count && trace.error_count > 0 ? (
              <Chip tone="error">{trace.error_count} error{trace.error_count === 1 ? '' : 's'}</Chip>
            ) : null}
          </div>

          {isDesktop ? (
            <TraceLineageBreadcrumb
              chain={lineageChain}
              isLoading={lineageLoading}
              projectId={projectId}
              returnTo={returnTo}
              trace={trace}
            />
          ) : null}

          {trace.engine?.continued_from_trace_id ||
          trace.engine?.continued_to_trace_id ? (
            <div className="mt-3 flex flex-wrap gap-2 text-sm">
              {trace.engine.continued_from_trace_id ? (
                <Link
                  to={appendProjectToPath(
                    `/traces/${trace.engine.continued_from_trace_id}`,
                    projectId
                  )}
                  state={{ returnTo }}
                  className="inline-flex items-center rounded border border-[var(--c-border)] bg-[var(--c-surface-muted)] px-2.5 py-1 text-xs font-medium text-[var(--c-text-secondary)] transition hover:border-[var(--c-border-strong)] hover:text-[var(--c-accent-text)]"
                >
                  ← Previous run
                </Link>
              ) : null}
              {trace.engine.continued_to_trace_id ? (
                <Link
                  to={appendProjectToPath(
                    `/traces/${trace.engine.continued_to_trace_id}`,
                    projectId
                  )}
                  state={{ returnTo }}
                  className="inline-flex items-center rounded border border-[var(--c-border)] bg-[var(--c-surface-muted)] px-2.5 py-1 text-xs font-medium text-[var(--c-text-secondary)] transition hover:border-[var(--c-border-strong)] hover:text-[var(--c-accent-text)]"
                >
                  Next run →
                </Link>
              ) : null}
            </div>
          ) : null}

          {trace.engine?.status === 'WAITING' ? (
            <EngineWaitStateSummary engine={trace.engine} />
          ) : null}

          <div className="mt-3 flex flex-wrap gap-7">
            <TraceHeaderMetric label="Duration" value={formatDuration(duration)} />
            <TraceHeaderMetric label="Spans" value={String(spans.length)} />
            <TraceHeaderMetric label="Tokens" value={formatTokens(totalTokens)} />
            <TraceHeaderMetric label="Cost" value={formatCost(trace.total_cost_usd)} />
            <TraceHeaderMetric label="Started" value={formatRelativeTime(trace.started_at)} />
            <TraceHeaderMetric label="User" value={trace.user_id ?? '—'} mono />
          </div>
        </div>

        <div className="flex flex-wrap gap-1.5 xl:justify-end">
          {replayPreviewEnabled ? (
            <Btn
              kind="secondary"
              leadingIcon={RotateCcw}
              size="sm"
              type="button"
              onClick={onShowReplay}
            >
              Replay
            </Btn>
          ) : null}
          <Btn kind="secondary" leadingIcon={Download} size="sm" type="button" onClick={exportTrace}>
            Export JSON
          </Btn>
          <Btn
            kind="secondary"
            leadingIcon={ExternalLink}
            size="sm"
            type="button"
            disabled
            title="Coming soon"
          >
            Open in workspace
          </Btn>
          <button
            type="button"
            aria-expanded={isTraceContextOpen}
            aria-label="Trace Context"
            onClick={onToggleTraceContext}
            className="app-button-secondary"
          >
            {isTraceContextOpen ? 'Hide context' : 'Trace context'}
          </button>
        </div>
      </div>
    </header>
  );
}
