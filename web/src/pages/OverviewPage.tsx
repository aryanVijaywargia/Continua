import { keepPreviousData, useQueries, useQuery } from '@tanstack/react-query';
import { useMemo } from 'react';
import { useLocation, useSearchParams } from 'react-router-dom';
import { Clock3, RefreshCw } from 'lucide-react';
import {
  fetchEngineHealth,
  fetchProjects,
  fetchSessions,
  fetchSpans,
  fetchTrace,
  fetchTraces,
  isAuthError,
} from '../api/client';
import { AuthErrorBanner } from '../components/AuthErrorBanner';
import { Btn } from '../components/DebuggerKit';
import { ReadOnlyBadge } from '../components/DataState';
import {
  appendProjectToPath,
  getProjectIdFromSearchParams,
} from '../utils/projectSearchParams';
import {
  AttentionSection,
  CoverageCard,
  DurationByTrace,
  RecentTracesTable,
  SectionHeading,
  ViewAllLink,
  type AttentionItem,
  type RecentTraceRowData,
} from './overview/OverviewSections';
import {
  buildTracesLink,
  deriveCoverage,
  OVERVIEW_RANGES,
  overviewRangeLabel,
  overviewRangeStart,
  parseOverviewRange,
  recordedDurations,
  summarizeEngineProjections,
  summarizeRequest,
  type OverviewRange,
} from './overview/overviewData';

/** Traces loaded for the duration chart and coverage checks. */
const OVERVIEW_TRACE_LIMIT = 12;
/** Rows in the recent table; each row loads its detail and spans. */
const RECENT_ROW_LIMIT = 5;
const ATTENTION_LIMIT = 5;
const SAMPLE_STALE_MS = 30_000;

export function OverviewPage() {
  return <OverviewContent />;
}

function OverviewContent() {
  const location = useLocation();
  const [searchParams, setSearchParams] = useSearchParams();
  const returnTo = `${location.pathname}${location.search}`;
  const currentProjectId = getProjectIdFromSearchParams(searchParams);
  const projectQueryKey = currentProjectId ?? null;
  const range = parseOverviewRange(searchParams.get('range'));
  // Fix the range start per selection so the query key stays stable across renders.
  const rangeStart = useMemo(() => overviewRangeStart(range, Date.now()), [range]);
  const rangeKey = rangeStart ?? 'all';

  const recentTracesQuery = useQuery({
    queryKey: ['overview', 'recent-traces', projectQueryKey, rangeKey],
    queryFn: () =>
      fetchTraces({
        project_id: currentProjectId,
        limit: OVERVIEW_TRACE_LIMIT,
        start_time_from: rangeStart,
      }),
    placeholderData: keepPreviousData,
  });
  const failedTracesQuery = useQuery({
    queryKey: ['overview', 'failed-traces', projectQueryKey, rangeKey],
    queryFn: () =>
      fetchTraces({
        project_id: currentProjectId,
        limit: ATTENTION_LIMIT,
        status: 'failed',
        start_time_from: rangeStart,
      }),
    placeholderData: keepPreviousData,
  });
  const runningTracesQuery = useQuery({
    queryKey: ['overview', 'running-traces', projectQueryKey, rangeKey],
    queryFn: () =>
      fetchTraces({
        project_id: currentProjectId,
        limit: ATTENTION_LIMIT,
        status: 'running',
        start_time_from: rangeStart,
      }),
    placeholderData: keepPreviousData,
  });
  const erroredTracesQuery = useQuery({
    queryKey: ['overview', 'errored-traces', projectQueryKey, rangeKey],
    queryFn: () =>
      fetchTraces({
        project_id: currentProjectId,
        limit: ATTENTION_LIMIT,
        has_errors: true,
        start_time_from: rangeStart,
      }),
    placeholderData: keepPreviousData,
  });
  const sessionsQuery = useQuery({
    queryKey: ['overview', 'sessions', projectQueryKey],
    queryFn: () => fetchSessions({ project_id: currentProjectId, limit: 1 }),
    placeholderData: keepPreviousData,
  });
  // Supporting queries: a failure here degrades one label, never the page.
  const projectsQuery = useQuery({
    queryKey: ['projects'],
    queryFn: fetchProjects,
    retry: false,
  });
  const engineHealthQuery = useQuery({
    queryKey: ['engine-health', projectQueryKey],
    queryFn: fetchEngineHealth,
    retry: false,
  });

  const recentTraces = recentTracesQuery.data?.traces ?? [];
  const recentRows = recentTraces.slice(0, RECENT_ROW_LIMIT);
  const detailQueries = useQueries({
    queries: recentRows.map((trace) => ({
      queryKey: ['trace', trace.id, projectQueryKey],
      queryFn: () => fetchTrace(trace.id, currentProjectId),
      staleTime: SAMPLE_STALE_MS,
    })),
  });
  const spansQueries = useQueries({
    queries: recentRows.map((trace) => ({
      queryKey: ['spans', trace.id, projectQueryKey],
      queryFn: () => fetchSpans(trace.id, currentProjectId),
      staleTime: SAMPLE_STALE_MS,
    })),
  });

  const primaryQueries = [
    recentTracesQuery,
    failedTracesQuery,
    runningTracesQuery,
    erroredTracesQuery,
    sessionsQuery,
  ];
  const authError = primaryQueries.map((query) => query.error).find(isAuthError);
  if (authError) {
    return <AuthErrorBanner message={authError.message} />;
  }
  const errors = primaryQueries
    .map((query) => query.error)
    .filter((error): error is Error => error instanceof Error);

  const totalTraces = recentTracesQuery.data?.total;
  const totalSessions = sessionsQuery.data?.total;
  const projects = projectsQuery.data?.projects ?? [];
  const project =
    projects.find((item) => item.id === currentProjectId) ??
    projects.find((item) => item.id === projectsQuery.data?.authenticated_project_id) ??
    (projects.length === 1 ? projects[0] : undefined);
  const rangeLabel = overviewRangeLabel(range);
  const tracesLink = (extra?: Record<string, string>) =>
    appendProjectToPath(buildTracesLink(rangeStart, extra), currentProjectId);

  const failedTraces = failedTracesQuery.data?.traces ?? [];
  const runningTraces = runningTracesQuery.data?.traces ?? [];
  const erroredTraces = erroredTracesQuery.data?.traces ?? [];
  const seen = new Set<string>();
  const attentionItems: AttentionItem[] = [
    ...failedTraces.map((trace) => ({ reason: 'failed' as const, trace })),
    ...runningTraces.map((trace) => ({ reason: 'running' as const, trace })),
    ...erroredTraces.map((trace) => ({ reason: 'errors' as const, trace })),
  ].filter((item) => {
    if (seen.has(item.trace.id)) {
      return false;
    }
    seen.add(item.trace.id);
    return true;
  });
  const failedTotal = failedTracesQuery.data?.total;
  const runningTotal = runningTracesQuery.data?.total;
  const erroredTotal = erroredTracesQuery.data?.total;
  const attentionQueries = [failedTracesQuery, runningTracesQuery, erroredTracesQuery];
  const attentionQueryFailed = attentionQueries.some((query) => query.isError);
  const attentionState =
    attentionItems.length > 0
      ? 'items'
      : attentionQueryFailed
        ? 'unavailable'
        : attentionQueries.some((query) => !query.data)
          ? 'loading'
          : failedTotal === 0 && runningTotal === 0 && erroredTotal === 0
            ? 'clear'
            : 'items';

  const rowData: RecentTraceRowData[] = recentRows.map((trace, index) => {
    const detailQuery = detailQueries[index];
    const spansQuery = spansQueries[index];
    const requestText = detailQuery?.data ? summarizeRequest(detailQuery.data.input) : null;
    return {
      trace,
      request: detailQuery?.data
        ? requestText
          ? { state: 'recorded', text: requestText }
          : { state: 'absent' }
        : detailQuery?.isError
          ? { state: 'unavailable' }
          : { state: 'loading' },
      steps: spansQuery?.data
        ? { state: 'recorded', count: spansQuery.data.spans.length }
        : spansQuery?.isError
          ? { state: 'unavailable' }
          : { state: 'loading' },
    };
  });

  const coverageRows = deriveCoverage({
    engineHealthLoaded: Boolean(engineHealthQuery.data),
    sampledDetails: detailQueries.map((query) => query.data),
    sampledSpans: spansQueries.map((query) => query.data?.spans),
    traces: recentRows,
  });

  const refreshAll = () => {
    for (const query of [...primaryQueries, engineHealthQuery]) {
      void query.refetch();
    }
  };

  const setRange = (next: OverviewRange) => {
    const params = new URLSearchParams(searchParams);
    if (next === 'all') {
      params.delete('range');
    } else {
      params.set('range', next);
    }
    setSearchParams(params, { replace: true });
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
      {errors.length > 0 ? (
        <div className="border-b border-[var(--c-red-border)] bg-[var(--c-red-faint)] px-6 py-3 text-sm text-[var(--c-red-text)]">
          Overview data is partially unavailable. {errors[0].message}
        </div>
      ) : null}

      <header className="flex flex-wrap items-center gap-x-4 gap-y-2 border-b border-[var(--c-border)] px-6 py-3.5">
        <h1 className="text-lg font-bold tracking-[-0.015em] text-[var(--c-text-primary)]">
          {project?.name ?? 'Overview'}
        </h1>
        <label className="inline-flex h-8 items-center gap-1.5 rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] pl-2.5 pr-1 text-[13px] text-[var(--c-text-primary)]">
          <Clock3 aria-hidden="true" className="h-3.5 w-3.5 text-[var(--c-text-muted)]" />
          <select
            aria-label="Date range"
            value={range}
            onChange={(event) => setRange(event.target.value as OverviewRange)}
            className="h-full border-0 bg-transparent pr-1 text-[13px] text-[var(--c-text-primary)] outline-none focus:ring-0"
          >
            {OVERVIEW_RANGES.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
        <span className="text-[12.5px] text-[var(--c-text-secondary)]">
          {totalTraces ?? '—'} {totalTraces === 1 ? 'trace' : 'traces'}
          {range === 'all' ? '' : ' in range'} · {totalSessions ?? '—'}{' '}
          {totalSessions === 1 ? 'session' : 'sessions'}
          {range === 'all' ? '' : ' (all time)'}
        </span>
        <div className="ml-auto flex items-center gap-2">
          <Btn kind="secondary" leadingIcon={RefreshCw} size="sm" onClick={refreshAll}>
            Refresh
          </Btn>
          <ReadOnlyBadge />
        </div>
      </header>

      <AttentionSection
        counts={{
          errors: erroredTotal,
          failed: failedTotal,
          running: runningTotal,
          total: totalTraces,
        }}
        engineProjections={summarizeEngineProjections(
          engineHealthQuery.data,
          engineHealthQuery.isError
        )}
        items={attentionItems}
        moreLinks={
          <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1">
            {(failedTotal ?? 0) > failedTraces.length ? (
              <ViewAllLink to={tracesLink({ status: 'failed' })}>
                All {failedTotal} failed traces
              </ViewAllLink>
            ) : null}
            {(runningTotal ?? 0) > runningTraces.length ? (
              <ViewAllLink to={tracesLink({ status: 'running' })}>
                All {runningTotal} running traces
              </ViewAllLink>
            ) : null}
            {(erroredTotal ?? 0) > erroredTraces.length ? (
              <ViewAllLink to={tracesLink({ has_errors: 'true' })}>
                All {erroredTotal} traces with failed steps
              </ViewAllLink>
            ) : null}
          </div>
        }
        projectId={currentProjectId}
        queryFailed={attentionQueryFailed}
        returnTo={returnTo}
        state={attentionState}
      />

      <section className="px-6 pt-6">
        <SectionHeading
          action={<ViewAllLink to={tracesLink()} />}
          subtitle={
            recentTraces.length > 0
              ? `${recentRows.length} most recent of ${totalTraces ?? '—'}`
              : undefined
          }
          title="Recent traces"
        />
        {recentTracesQuery.isPending && !recentTracesQuery.data ? (
          <div className="app-empty-state">Loading traces...</div>
        ) : recentTraces.length === 0 ? (
          <div className="app-empty-state">
            {range === 'all' ? 'No traces yet.' : 'No traces in this range.'}
          </div>
        ) : (
          <RecentTracesTable
            projectId={currentProjectId}
            returnTo={returnTo}
            rows={rowData}
          />
        )}
      </section>

      <section className="grid gap-4 px-6 pb-8 pt-6 lg:grid-cols-[minmax(0,1fr)_minmax(280px,380px)]">
        <DurationByTrace
          durations={recordedDurations(recentTraces)}
          loadedCount={recentTraces.length}
          rangeLabel={rangeLabel}
          totalInRange={totalTraces ?? recentTraces.length}
        />
        <CoverageCard rows={coverageRows} sampleSize={recentRows.length} />
      </section>
    </div>
  );
}
