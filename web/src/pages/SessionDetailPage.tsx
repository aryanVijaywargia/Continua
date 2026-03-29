import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { useCallback, useEffect } from 'react';
import { Link, useLocation, useParams, useSearchParams } from 'react-router-dom';
import { AuthErrorBanner } from '../components/AuthErrorBanner';
import { CopyButton } from '../components/CopyButton';
import { PaginationControls } from '../components/PaginationControls';
import { SortableHeader } from '../components/SortableHeader';
import { StatusBadge } from '../components/StatusBadge';
import {
  fetchSession,
  fetchSessionNarrative,
  fetchTraces,
  isAuthError,
  type SessionNarrative,
  type SessionNarrativeLineage,
  type SessionNarrativeTrace,
  type Trace,
} from '../api/client';
import { useRequireApiKey } from '../hooks/useRequireApiKey';
import { DEFAULT_PAGE_SIZE, getLastValidOffset } from '../utils/pagination';
import { buildCanonicalQueryString, parseTracesParams, serializeTracesParams } from '../utils/tracesSearchParams';
import {
  calculateDuration,
  formatCost,
  formatDuration,
  formatRelativeTime,
  formatTokens,
} from '../utils/format';
import { summarizeTimelineEvent } from '../utils/timeline';

type HistoryMode = 'push' | 'replace';

function shouldResetOffset(
  currentState: ReturnType<typeof getSessionTraceTableState>,
  updates: Partial<ReturnType<typeof getSessionTraceTableState>>
): boolean {
  return Object.entries(updates).some(
    ([key, value]) =>
      key !== 'offset' &&
      currentState[key as keyof ReturnType<typeof getSessionTraceTableState>] !== value
  );
}

function getSessionTraceTableState(searchParams: URLSearchParams) {
  const parsed = parseTracesParams(searchParams);

  return {
    limit: parsed.limit,
    offset: parsed.offset,
    sort_by: parsed.sort_by,
    sort_dir: parsed.sort_dir,
  };
}

function useSessionTraceTableSearchParams() {
  const [searchParams, setSearchParams] = useSearchParams();
  const filters = getSessionTraceTableState(searchParams);
  const canonicalSearch = serializeTracesParams(filters).toString();

  useEffect(() => {
    if (searchParams.toString() === canonicalSearch) {
      return;
    }

    setSearchParams(new URLSearchParams(canonicalSearch), { replace: true });
  }, [canonicalSearch, searchParams, setSearchParams]);

  const setFilters = useCallback(
    (
      updates: Partial<ReturnType<typeof getSessionTraceTableState>>,
      mode: HistoryMode = 'push'
    ) => {
      const next = {
        ...filters,
        ...updates,
      };

      const normalizedNext = shouldResetOffset(filters, updates)
        ? { ...next, offset: 0 }
        : next;

      setSearchParams(serializeTracesParams(normalizedNext), {
        replace: mode === 'replace',
      });
    },
    [filters, setSearchParams]
  );

  return {
    filters,
    setFilters,
  };
}

function getSessionsReturnToDestination(state: unknown): string {
  if (
    typeof state !== 'object' ||
    state === null ||
    !('returnTo' in state) ||
    typeof state.returnTo !== 'string'
  ) {
    return '/sessions';
  }

  const { returnTo } = state;
  return returnTo === '/sessions' || returnTo.startsWith('/sessions?')
    ? returnTo
    : '/sessions';
}

function queryErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Unknown error';
}

export function SessionDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { hasApiKey, prompt } = useRequireApiKey();

  if (!hasApiKey) {
    return prompt;
  }

  if (!id) {
    return (
      <div className="min-h-screen bg-slate-50 flex items-center justify-center dark:bg-slate-950">
        <div className="text-red-600">Session ID is required</div>
      </div>
    );
  }

  return <SessionDetailContent sessionId={id} />;
}

function SessionDetailContent({ sessionId }: { sessionId: string }) {
  const location = useLocation();
  const { filters, setFilters } = useSessionTraceTableSearchParams();
  const returnTo = getSessionsReturnToDestination(location.state);
  const currentSessionDetailUrl = `${location.pathname}${location.search}`;
  const isAscending = filters.sort_dir === 'asc';

  const sessionQuery = useQuery({
    queryKey: ['session', sessionId],
    queryFn: () => fetchSession(sessionId),
  });
  const narrativeQuery = useQuery({
    queryKey: ['session-narrative', sessionId],
    queryFn: () => fetchSessionNarrative(sessionId),
    refetchInterval: (query) =>
      (query.state.data?.summary.running_trace_count ?? 0) > 0 ? 30_000 : false,
  });

  const traceQueryParams = {
    session_id: sessionId,
    limit: filters.limit,
    offset: filters.offset,
    sort_by: filters.sort_by,
    sort_dir: filters.sort_dir,
  };
  const tracesQuery = useQuery({
    queryKey: ['session-traces', sessionId, buildCanonicalQueryString(traceQueryParams)],
    queryFn: () => fetchTraces(traceQueryParams),
    placeholderData: keepPreviousData,
  });
  const traces = tracesQuery.data?.traces ?? [];
  const total = tracesQuery.data?.total ?? 0;

  useEffect(() => {
    if (traces.length !== 0 || total === 0 || filters.offset === 0) {
      return;
    }

    const lastValidOffset = getLastValidOffset(total, filters.limit ?? DEFAULT_PAGE_SIZE);
    if (lastValidOffset !== filters.offset) {
      setFilters({ offset: lastValidOffset }, 'replace');
    }
  }, [filters.limit, filters.offset, setFilters, total, traces.length]);

  const handleStartedSortToggle = useCallback(() => {
    setFilters(
      {
        sort_by: 'started_at',
        sort_dir:
          filters.sort_by === 'started_at' && filters.sort_dir === 'asc'
            ? 'desc'
            : 'asc',
      },
      'push'
    );
  }, [filters.sort_by, filters.sort_dir, setFilters]);

  if (sessionQuery.isLoading) {
    return (
      <div className="min-h-screen bg-slate-50 flex items-center justify-center dark:bg-slate-950">
        <div className="text-slate-500 dark:text-slate-400">Loading session...</div>
      </div>
    );
  }

  if (sessionQuery.error) {
    return (
      <div className="min-h-screen bg-slate-50 dark:bg-slate-950">
        <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6 lg:px-8">
          {isAuthError(sessionQuery.error) ? (
            <AuthErrorBanner message={queryErrorMessage(sessionQuery.error)} />
          ) : (
            <div className="rounded-xl border border-red-200 bg-red-50 p-4 text-red-700 dark:border-red-500/40 dark:bg-red-500/10 dark:text-red-200">
              Error loading session: {queryErrorMessage(sessionQuery.error)}
            </div>
          )}
        </div>
      </div>
    );
  }

  if (!sessionQuery.data) {
    return (
      <div className="min-h-screen bg-slate-50 flex items-center justify-center dark:bg-slate-950">
        <div className="text-slate-500 dark:text-slate-400">Session not found</div>
      </div>
    );
  }

  const session = sessionQuery.data;

  return (
    <div className="min-h-screen bg-slate-50 dark:bg-slate-950">
      <div className="mx-auto max-w-7xl px-4 py-8 sm:px-6 lg:px-8">
        <Link
          to={returnTo}
          className="mb-4 inline-block text-sm text-blue-600 hover:text-blue-800 dark:text-sky-400 dark:hover:text-sky-300"
        >
          &larr; Back to Sessions
        </Link>

        <section className="mb-6 rounded-xl border border-slate-200 bg-white p-6 shadow-sm dark:border-slate-800 dark:bg-slate-900">
          <div className="flex flex-col gap-4 lg:flex-row lg:items-start lg:justify-between">
            <div>
              <div className="flex flex-wrap items-center gap-3">
                <h1 className="text-2xl font-bold text-slate-900 dark:text-slate-100">{session.external_id}</h1>
                <CopyButton
                  aria-label="Copy session external ID"
                  value={session.external_id}
                  idleLabel="Copy external ID"
                  successLabel="Copied external ID"
                />
              </div>
              <div className="mt-2 flex flex-wrap items-center gap-3">
                <span className="font-mono text-sm text-slate-500 dark:text-slate-400">{session.id}</span>
                <CopyButton
                  aria-label="Copy session UUID"
                  value={session.id}
                  idleLabel="Copy UUID"
                  successLabel="Copied UUID"
                />
              </div>
              {session.name ? (
                <p className="mt-3 text-sm text-slate-600 dark:text-slate-300">{session.name}</p>
              ) : null}
            </div>

            <dl className="grid gap-4 sm:grid-cols-3">
              <div>
                <dt className="text-xs font-medium uppercase tracking-[0.16em] text-slate-500 dark:text-slate-400">
                  User ID
                </dt>
                <dd className="mt-1 text-sm text-slate-900 dark:text-slate-100">{session.user_id || '-'}</dd>
              </div>
              <div>
                <dt className="text-xs font-medium uppercase tracking-[0.16em] text-slate-500 dark:text-slate-400">
                  Trace Count
                </dt>
                <dd className="mt-1 text-sm text-slate-900 dark:text-slate-100">{session.trace_count ?? 0}</dd>
              </div>
              <div>
                <dt className="text-xs font-medium uppercase tracking-[0.16em] text-slate-500 dark:text-slate-400">
                  Created
                </dt>
                <dd className="mt-1 text-sm text-slate-900 dark:text-slate-100">
                  {formatRelativeTime(session.created_at)}
                </dd>
              </div>
            </dl>
          </div>
        </section>

        <SessionNarrativeSections
          narrative={narrativeQuery.data}
          error={narrativeQuery.error}
          isFetching={narrativeQuery.isFetching}
          isPending={narrativeQuery.isPending && !narrativeQuery.data}
          returnTo={currentSessionDetailUrl}
        />

        <div className="mb-4 flex items-center justify-between">
          <div>
            <h2 className="text-lg font-semibold text-slate-900 dark:text-slate-100">Traces</h2>
            <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
              Sort by started time and preserve table state in the URL.
            </p>
          </div>
          <div className="text-sm text-slate-500 dark:text-slate-400">
            <span>{total} traces</span>
            {tracesQuery.isFetching && !tracesQuery.isPending && (
              <span className="ml-2 text-blue-600 dark:text-sky-400">Updating...</span>
            )}
          </div>
        </div>

        {tracesQuery.error && (
          isAuthError(tracesQuery.error) ? (
            <div className="mb-4">
              <AuthErrorBanner message={queryErrorMessage(tracesQuery.error)} />
            </div>
          ) : (
            <div className="mb-4 rounded-xl border border-red-200 bg-red-50 p-4 text-red-700 dark:border-red-500/40 dark:bg-red-500/10 dark:text-red-200">
              Error loading traces: {queryErrorMessage(tracesQuery.error)}
            </div>
          )
        )}

        {tracesQuery.isPending && !tracesQuery.data ? (
          <div className="rounded-xl border border-slate-200 bg-white p-8 text-center text-slate-500 shadow-sm dark:border-slate-800 dark:bg-slate-900 dark:text-slate-400">
            Loading traces...
          </div>
        ) : traces.length === 0 ? (
          <div className="rounded-xl border border-slate-200 bg-white p-8 text-center shadow-sm dark:border-slate-800 dark:bg-slate-900">
            <h2 className="text-lg font-semibold text-slate-900 dark:text-slate-100">No traces in this session</h2>
            <p className="mt-2 text-sm text-slate-500 dark:text-slate-400">
              Traces will appear here as they are ingested for this session.
            </p>
          </div>
        ) : (
          <>
            <div className="overflow-x-auto rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
              <table className="min-w-full divide-y divide-slate-200 dark:divide-slate-800">
                <thead className="bg-slate-50 dark:bg-slate-950/70">
                  <tr>
                    <th className="px-6 py-3 text-left text-xs font-semibold uppercase tracking-[0.16em] text-slate-500 dark:text-slate-400">
                      Name
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-semibold uppercase tracking-[0.16em] text-slate-500 dark:text-slate-400">
                      Status
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-semibold uppercase tracking-[0.16em] text-slate-500 dark:text-slate-400">
                      Duration
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-semibold uppercase tracking-[0.16em] text-slate-500 dark:text-slate-400">
                      Tokens
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-semibold uppercase tracking-[0.16em] text-slate-500 dark:text-slate-400">
                      Cost
                    </th>
                    <th className="px-6 py-3 text-left text-xs font-semibold uppercase tracking-[0.16em] text-slate-500 dark:text-slate-400">
                      <SortableHeader
                        label="Started"
                        isActive={filters.sort_by === 'started_at'}
                        isAscending={isAscending}
                        onClick={handleStartedSortToggle}
                      />
                    </th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-200 bg-white dark:divide-slate-800 dark:bg-slate-900">
                  {traces.map((trace) => (
                    <SessionTraceRow
                      key={trace.id}
                      returnTo={currentSessionDetailUrl}
                      trace={trace}
                    />
                  ))}
                </tbody>
              </table>
            </div>

            <PaginationControls
              offset={filters.offset}
              pageSize={filters.limit ?? DEFAULT_PAGE_SIZE}
              total={total}
              currentItemCount={traces.length}
              onOffsetChange={(offset) => setFilters({ offset }, 'push')}
              onPageSizeChange={(limit) => setFilters({ limit }, 'push')}
              onRepairOffset={(offset) => setFilters({ offset }, 'replace')}
            />
          </>
        )}
      </div>
    </div>
  );
}

function SessionNarrativeSections({
  narrative,
  error,
  isFetching,
  isPending,
  returnTo,
}: {
  narrative?: SessionNarrative;
  error: unknown;
  isFetching: boolean;
  isPending: boolean;
  returnTo: string;
}) {
  if (error) {
    return (
      <div className="mb-6">
        {isAuthError(error) ? (
          <AuthErrorBanner message={queryErrorMessage(error)} />
        ) : (
          <div className="rounded-xl border border-red-200 bg-red-50 p-4 text-red-700 dark:border-red-500/40 dark:bg-red-500/10 dark:text-red-200">
            Error loading narrative: {queryErrorMessage(error)}
          </div>
        )}
      </div>
    );
  }

  if (isPending || !narrative) {
    return (
      <section
        aria-label="Session narrative loading"
        className="mb-6 rounded-xl border border-slate-200 bg-white p-6 shadow-sm dark:border-slate-800 dark:bg-slate-900"
      >
        <div className="animate-pulse">
          <h2 className="text-lg font-semibold text-slate-900 dark:text-slate-100">
            Session Narrative
          </h2>
          <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">Loading narrative...</p>
          <div className="mt-5 grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            {Array.from({ length: 6 }).map((_, index) => (
              <div
                key={index}
                className="h-20 rounded-lg bg-slate-100 dark:bg-slate-800"
              />
            ))}
          </div>
        </div>
      </section>
    );
  }

  const lineageCoverageLabel = narrative.summary.truncated
    ? `Lineage coverage applies to the first ${narrative.summary.returned_trace_count} traces shown.`
    : 'Lineage coverage applies to the shown narrative only.';

  if (narrative.summary.total_trace_count === 0) {
    return (
      <section
        aria-label="Session narrative placeholder"
        className="mb-6 rounded-xl border border-slate-200 bg-white p-6 shadow-sm dark:border-slate-800 dark:bg-slate-900"
      >
        <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <h2 className="text-lg font-semibold text-slate-900 dark:text-slate-100">
              Session Narrative
            </h2>
            <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
              A compact session storyline will appear here as traces are ingested for this session.
            </p>
          </div>
          {isFetching ? (
            <div className="text-sm text-blue-600 dark:text-sky-400">Updating narrative...</div>
          ) : null}
        </div>

        <div className="mt-4 rounded-lg border border-dashed border-slate-300 bg-slate-50 px-4 py-5 text-sm text-slate-600 dark:border-slate-700 dark:bg-slate-950/50 dark:text-slate-300">
          <p className="font-medium text-slate-900 dark:text-slate-100">No narrative yet</p>
          <p className="mt-1">
            This placeholder stays compact until the session has at least one ingested trace.
          </p>
        </div>
      </section>
    );
  }

  return (
    <>
      <section
        aria-label="Session narrative summary"
        className="mb-6 rounded-xl border border-slate-200 bg-white p-6 shadow-sm dark:border-slate-800 dark:bg-slate-900"
      >
        <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <h2 className="text-lg font-semibold text-slate-900 dark:text-slate-100">
              Session Narrative
            </h2>
            <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
              Chronological summary for the returned storyline above the full trace browser.
            </p>
          </div>
          {isFetching && (
            <div className="text-sm text-blue-600 dark:text-sky-400">Updating narrative...</div>
          )}
        </div>

        <dl className="mt-5 grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          <MetricBlock
            label="Returned / Total"
            value={`${narrative.summary.returned_trace_count} shown / ${narrative.summary.total_trace_count} total`}
          />
          <MetricBlock
            label="Status Breakdown"
            value={`${narrative.summary.running_trace_count} running`}
            hint={`${narrative.summary.completed_trace_count} completed · ${narrative.summary.failed_trace_count} failed`}
          />
          <MetricBlock
            label="Aggregate Usage"
            value={formatCost(narrative.summary.total_cost_usd)}
            hint={`${formatTokens(narrative.summary.total_tokens_in)} in · ${formatTokens(
              narrative.summary.total_tokens_out
            )} out`}
          />
          <MetricBlock
            label="Started"
            value={formatRelativeTime(narrative.summary.started_at)}
          />
          <MetricBlock
            label="Last Activity"
            value={formatRelativeTime(narrative.summary.last_activity_at)}
          />
          <MetricBlock
            label="Lineage Coverage"
            value={`${narrative.summary.explicit_link_count} explicit · ${narrative.summary.inferred_link_count} inferred · ${narrative.summary.unlinked_trace_count} unlinked`}
            hint={lineageCoverageLabel}
          />
        </dl>
      </section>

      <section
        aria-label="Session narrative storyline"
        className="mb-6 rounded-xl border border-slate-200 bg-white p-6 shadow-sm dark:border-slate-800 dark:bg-slate-900"
      >
        <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
          <div>
            <h2 className="text-lg font-semibold text-slate-900 dark:text-slate-100">Storyline</h2>
            <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
              Oldest-first trace flow for the narrative that was returned.
            </p>
          </div>
        </div>

        {narrative.summary.truncated && (
          <div className="mt-4 rounded-lg border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-100">
            Narrative limited to the first {narrative.summary.returned_trace_count} traces. The
            table below remains the full browser.
          </div>
        )}

        {narrative.traces.length === 0 ? (
          <div className="mt-4 rounded-lg border border-dashed border-slate-300 bg-slate-50 px-4 py-5 text-sm text-slate-600 dark:border-slate-700 dark:bg-slate-950/50 dark:text-slate-300">
            <p className="font-medium text-slate-900 dark:text-slate-100">No narrative yet</p>
            <p className="mt-1">
              A compact session storyline will appear here as traces are ingested for this session.
            </p>
          </div>
        ) : (
          <div className="mt-4 space-y-4">
            {narrative.traces.map((trace) => (
              <StorylineTraceCard key={trace.id} trace={trace} returnTo={returnTo} />
            ))}
          </div>
        )}
      </section>
    </>
  );
}

function StorylineTraceCard({
  trace,
  returnTo,
}: {
  trace: SessionNarrativeTrace;
  returnTo: string;
}) {
  const semanticSnippet = getSemanticSnippet(trace);

  return (
    <article className="rounded-xl border border-slate-200 bg-slate-50/80 p-5 dark:border-slate-800 dark:bg-slate-950/60">
      <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <Link
              to={`/traces/${trace.id}`}
              state={{ returnTo }}
              className="text-sm font-semibold text-blue-700 transition hover:text-blue-900 focus:outline-none focus:ring-2 focus:ring-blue-200 dark:text-sky-400 dark:hover:text-sky-300"
            >
              {trace.name}
            </Link>
            <StatusBadge status={trace.status} />
            <NarrativeLineageBadge lineage={trace.lineage} />
          </div>

          <div className="mt-2 flex flex-wrap items-center gap-3 text-xs text-slate-500 dark:text-slate-400">
            <span className="font-mono">{trace.trace_id}</span>
            {trace.user_id ? <span>User {trace.user_id}</span> : null}
          </div>

          {semanticSnippet ? (
            <p className="mt-3 text-sm text-slate-600 dark:text-slate-300">{semanticSnippet}</p>
          ) : null}
        </div>

        <dl className="grid gap-3 sm:grid-cols-2 xl:w-[28rem] xl:grid-cols-3">
          <MetricBlock label="Started" value={formatRelativeTime(trace.started_at)} compact />
          <MetricBlock label="Ended" value={formatRelativeTime(trace.ended_at)} compact />
          <MetricBlock
            label="Latest Activity"
            value={formatRelativeTime(trace.latest_activity_at)}
            compact
          />
          <MetricBlock label="Duration" value={formatDuration(trace.duration_ms)} compact />
          <MetricBlock
            label="Tokens"
            value={`${formatTokens(trace.total_tokens_in)} in / ${formatTokens(
              trace.total_tokens_out
            )} out`}
            compact
          />
          <MetricBlock
            label="Cost / Errors"
            value={`${formatCost(trace.total_cost_usd)} · ${trace.error_count ?? 0} errors`}
            compact
          />
        </dl>
      </div>
    </article>
  );
}

function MetricBlock({
  label,
  value,
  hint,
  compact = false,
}: {
  label: string;
  value: string;
  hint?: string;
  compact?: boolean;
}) {
  return (
    <div className={compact ? '' : 'rounded-lg border border-slate-200 p-4 dark:border-slate-800'}>
      <dt className="text-xs font-medium uppercase tracking-[0.16em] text-slate-500 dark:text-slate-400">
        {label}
      </dt>
      <dd className="mt-2 text-sm font-semibold text-slate-900 dark:text-slate-100">{value}</dd>
      {hint ? (
        <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">{hint}</p>
      ) : null}
    </div>
  );
}

function NarrativeLineageBadge({ lineage }: { lineage: SessionNarrativeLineage }) {
  const label = formatLineageLabel(lineage.type);
  const colorClass =
    lineage.type === 'explicit'
      ? 'bg-amber-100 text-amber-900 dark:bg-amber-500/15 dark:text-amber-100'
      : lineage.type === 'inferred'
        ? 'bg-indigo-100 text-indigo-900 dark:bg-indigo-500/15 dark:text-indigo-100'
        : 'bg-slate-200 text-slate-700 dark:bg-slate-800 dark:text-slate-200';

  return (
    <span
      className={`inline-flex items-center rounded-full px-2.5 py-0.5 text-xs font-medium ${colorClass}`}
    >
      {label}
    </span>
  );
}

function formatLineageLabel(type: SessionNarrativeLineage['type']): string {
  switch (type) {
    case 'explicit':
      return 'Explicit';
    case 'inferred':
      return 'Inferred';
    default:
      return 'Unlinked';
  }
}

function getSemanticSnippet(trace: SessionNarrativeTrace): string | null {
  const latestEvent = trace.semantic_events.at(-1);
  return latestEvent ? summarizeTimelineEvent(latestEvent) : null;
}

function SessionTraceRow({
  trace,
  returnTo,
}: {
  trace: Trace;
  returnTo: string;
}) {
  const duration = calculateDuration(trace.started_at, trace.ended_at);
  const totalTokens = (trace.total_tokens_in ?? 0) + (trace.total_tokens_out ?? 0);

  return (
    <tr className="transition hover:bg-slate-50 dark:hover:bg-slate-800/60">
      <td className="px-6 py-4 align-top">
        <Link
          to={`/traces/${trace.id}`}
          state={{ returnTo }}
          className="text-sm font-semibold text-blue-700 transition hover:text-blue-900 focus:outline-none focus:ring-2 focus:ring-blue-200 dark:text-sky-400 dark:hover:text-sky-300"
        >
          {trace.name}
        </Link>
      </td>
      <td className="px-6 py-4 whitespace-nowrap align-top">
        <StatusBadge status={trace.status} />
      </td>
      <td className="px-6 py-4 whitespace-nowrap text-sm text-slate-900 align-top dark:text-slate-100">
        {formatDuration(duration)}
      </td>
      <td className="px-6 py-4 whitespace-nowrap text-sm text-slate-900 align-top dark:text-slate-100">
        {formatTokens(totalTokens)}
      </td>
      <td className="px-6 py-4 whitespace-nowrap text-sm text-slate-900 align-top dark:text-slate-100">
        {formatCost(trace.total_cost_usd)}
      </td>
      <td className="px-6 py-4 whitespace-nowrap text-sm text-slate-500 align-top dark:text-slate-400">
        {formatRelativeTime(trace.started_at)}
      </td>
    </tr>
  );
}
