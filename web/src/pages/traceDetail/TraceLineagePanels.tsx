import { Link } from 'react-router-dom';
import type { Trace, TraceDetail } from '../../api/client';
import { StatusBadge } from '../../components/StatusBadge';
import { appendProjectToPath } from '../../utils/projectSearchParams';

/**
 * Lineage chrome for engine-backed traces. These stay presentational
 * (props-based): they are rendered with different data combinations — the
 * header, the mobile summary, and the trace-context drawer each feed them a
 * different chain/children slice.
 */
export function TraceLineageBreadcrumb({
  chain,
  isLoading,
  projectId,
  returnTo,
  trace,
}: {
  chain: Trace[];
  isLoading: boolean;
  projectId?: string;
  returnTo: string;
  trace: TraceDetail;
}) {
  if (!trace.engine?.parent_run_id) {
    return null;
  }

  if (!isLoading && chain.length <= 1) {
    return null;
  }

  return (
    <section className="mt-4">
      <p className="text-[11px] font-semibold uppercase tracking-[0.18em] text-[var(--continua-text-muted)]">
        Lineage
      </p>
      <nav
        aria-label="Trace lineage"
        className="mt-2 flex flex-wrap items-center gap-2 text-sm"
      >
        {chain.length > 1 ? (
          chain.map((lineageTrace, index) => {
            const isCurrent = lineageTrace.id === trace.id;
            return (
              <div
                key={lineageTrace.engine?.run_id ?? lineageTrace.id}
                className="contents"
              >
                {index > 0 ? (
                  <span className="text-[var(--continua-text-muted)]">›</span>
                ) : null}
                {isCurrent ? (
                  <span className="rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-3 py-1.5 font-medium text-[var(--continua-text-primary)]">
                    {lineageTrace.name}
                  </span>
                ) : (
                  <Link
                    to={appendProjectToPath(`/traces/${lineageTrace.id}`, projectId)}
                    state={{ returnTo }}
                    className="rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-3 py-1.5 font-medium text-[var(--continua-text-secondary)] transition hover:border-[var(--continua-border-strong)] hover:text-[var(--continua-accent)]"
                  >
                    {lineageTrace.name}
                  </Link>
                )}
              </div>
            );
          })
        ) : (
          <span className="rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-3 py-1.5 text-[var(--continua-text-muted)]">
            Loading lineage...
          </span>
        )}
      </nav>
    </section>
  );
}

export function TraceLineageCard({
  childTraces,
  childTracesLoading,
  framed = true,
  hasChildTracesError,
  lineageChain,
  lineageLoading,
  projectId,
  returnTo,
  showEmptyChildren = false,
  showLineageSummary = false,
  trace,
}: {
  childTraces: Trace[];
  childTracesLoading: boolean;
  framed?: boolean;
  hasChildTracesError: boolean;
  lineageChain: Trace[];
  lineageLoading: boolean;
  projectId?: string;
  returnTo: string;
  showEmptyChildren?: boolean;
  showLineageSummary?: boolean;
  trace: TraceDetail;
}) {
  const hasParent = Boolean(trace.engine?.parent_run_id);
  const hasRenderableLineage =
    showLineageSummary && hasParent && (lineageLoading || lineageChain.length > 1);
  const shouldRender =
    hasRenderableLineage ||
    childTracesLoading ||
    hasChildTracesError ||
    childTraces.length > 0 ||
    showEmptyChildren;

  if (!trace.engine || !shouldRender) {
    return null;
  }

  return (
    <section
      className={
        framed
          ? 'overflow-hidden rounded-[1rem] border border-[var(--continua-border-strong)] bg-[var(--continua-surface)] shadow-[var(--continua-shadow-soft)]'
          : 'space-y-4'
      }
    >
      <div
        className={
          framed
            ? 'border-b border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-4 py-3'
            : ''
        }
      >
        <h2 className="text-sm font-semibold uppercase tracking-[0.18em] text-[var(--continua-text-secondary)]">
          Child Workflows
        </h2>
      </div>
      <div className={framed ? 'space-y-4 p-4' : 'space-y-4'}>
        {showLineageSummary ? (
          <TraceLineageSummaryBreadcrumb
            chain={lineageChain}
            isLoading={lineageLoading}
            projectId={projectId}
            returnTo={returnTo}
            trace={trace}
          />
        ) : null}
        <ChildWorkflowSection
          childTraces={childTraces}
          isError={hasChildTracesError}
          isLoading={childTracesLoading}
          projectId={projectId}
          returnTo={returnTo}
          showEmptyState={showEmptyChildren}
        />
      </div>
    </section>
  );
}

function TraceLineageSummaryBreadcrumb({
  chain,
  isLoading,
  projectId,
  returnTo,
  trace,
}: {
  chain: Trace[];
  isLoading: boolean;
  projectId?: string;
  returnTo: string;
  trace: TraceDetail;
}) {
  if (!trace.engine?.parent_run_id) {
    return null;
  }

  if (!isLoading && chain.length <= 1) {
    return null;
  }

  return (
    <div className="space-y-2">
      <p className="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--continua-text-muted)]">
        Lineage
      </p>
      {chain.length > 1 ? (
        <nav
          aria-label="Trace lineage summary"
          className="flex flex-wrap items-center gap-2 text-sm"
        >
          {chain.map((lineageTrace, index) => {
            const isCurrent = lineageTrace.id === trace.id;
            return (
              <div
                key={lineageTrace.engine?.run_id ?? lineageTrace.id}
                className="contents"
              >
                {index > 0 ? (
                  <span className="text-[var(--continua-text-muted)]">›</span>
                ) : null}
                {isCurrent ? (
                  <span className="rounded-lg border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-3 py-2 font-medium text-[var(--continua-text-primary)]">
                    {lineageTrace.name}
                  </span>
                ) : (
                  <Link
                    to={appendProjectToPath(`/traces/${lineageTrace.id}`, projectId)}
                    state={{ returnTo }}
                    className="inline-flex max-w-full items-center rounded-lg border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-3 py-2 font-medium text-[var(--continua-accent)] transition hover:border-[var(--continua-border-strong)] hover:opacity-80"
                  >
                    <span className="truncate">{lineageTrace.name}</span>
                  </Link>
                )}
              </div>
            );
          })}
        </nav>
      ) : isLoading ? (
        <p className="text-sm text-[var(--continua-text-muted)]">
          Loading lineage...
        </p>
      ) : null}
    </div>
  );
}

function ChildWorkflowSection({
  childTraces,
  isError,
  isLoading,
  projectId,
  returnTo,
  showEmptyState,
}: {
  childTraces: Trace[];
  isError: boolean;
  isLoading: boolean;
  projectId?: string;
  returnTo: string;
  showEmptyState: boolean;
}) {
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <p className="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--continua-text-muted)]">
          Direct Children
        </p>
        {!isLoading && !isError ? (
          <span className="text-xs text-[var(--continua-text-muted)]">
            {childTraces.length}
          </span>
        ) : null}
      </div>

      {isLoading ? (
        <p className="text-sm text-[var(--continua-text-muted)]">
          Loading child workflows...
        </p>
      ) : null}

      {isError ? (
        <p className="text-sm text-[var(--continua-text-muted)]">
          Child workflow lineage is temporarily unavailable.
        </p>
      ) : null}

      {!isLoading && !isError && childTraces.length === 0 && showEmptyState ? (
        <p className="text-sm text-[var(--continua-text-muted)]">
          No direct child workflows yet.
        </p>
      ) : null}

      {!isLoading && !isError && childTraces.length > 0 ? (
        <div className="space-y-3">
          {childTraces.map((childTrace) => (
            <Link
              key={childTrace.id}
              to={appendProjectToPath(`/traces/${childTrace.id}`, projectId)}
              state={{ returnTo }}
              className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-3 py-3 transition hover:border-[var(--continua-border-strong)] hover:bg-[var(--continua-surface-elevated)] hover:opacity-90 focus:outline-none focus:ring-2 focus:ring-[var(--continua-accent)]"
            >
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-mono text-xs text-[var(--continua-text-muted)]">
                    {childTrace.engine?.child_key ?? 'child'}
                  </span>
                  <StatusBadge status={childTrace.status} />
                </div>
                <p className="mt-2 truncate text-sm font-medium text-[var(--continua-text-primary)]">
                  {childTrace.name}
                </p>
                {childTrace.engine ? (
                  <p className="mt-1 text-xs text-[var(--continua-text-muted)]">
                    {childTrace.engine.definition_name}@
                    {childTrace.engine.definition_version}
                  </p>
                ) : null}
              </div>
              <span
                className="inline-flex items-center rounded-lg border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-3 py-2 text-sm font-medium text-[var(--continua-accent)] transition hover:border-[var(--continua-border-strong)] hover:opacity-80"
              >
                Open trace
              </span>
            </Link>
          ))}
        </div>
      ) : null}
    </div>
  );
}
