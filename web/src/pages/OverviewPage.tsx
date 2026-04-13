import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { Link, useLocation } from 'react-router-dom';
import {
  fetchSessions,
  fetchTraces,
  isAuthError,
  type Session,
  type Trace,
} from '../api/client';
import { AuthErrorBanner } from '../components/AuthErrorBanner';
import { StatusBadge } from '../components/StatusBadge';
import { useRequireApiKey } from '../hooks/useRequireApiKey';
import {
  formatCost,
  formatRelativeTime,
  formatTokens,
} from '../utils/format';

const OVERVIEW_TRACE_LIMIT = 6;
const OVERVIEW_SESSION_LIMIT = 6;

export function OverviewPage() {
  const { hasApiKey, prompt } = useRequireApiKey();

  if (!hasApiKey) {
    return prompt;
  }

  return <OverviewContent />;
}

function OverviewContent() {
  const location = useLocation();
  const returnTo = `${location.pathname}${location.search}`;
  const recentTracesQuery = useQuery({
    queryKey: ['overview', 'recent-traces'],
    queryFn: () => fetchTraces({ limit: OVERVIEW_TRACE_LIMIT }),
    placeholderData: keepPreviousData,
  });
  const failedTracesQuery = useQuery({
    queryKey: ['overview', 'failed-traces'],
    queryFn: () => fetchTraces({ limit: OVERVIEW_TRACE_LIMIT, status: 'failed' }),
    placeholderData: keepPreviousData,
  });
  const runningTracesQuery = useQuery({
    queryKey: ['overview', 'running-traces'],
    queryFn: () => fetchTraces({ limit: OVERVIEW_TRACE_LIMIT, status: 'running' }),
    placeholderData: keepPreviousData,
  });
  const sessionsQuery = useQuery({
    queryKey: ['overview', 'sessions'],
    queryFn: () => fetchSessions({ limit: OVERVIEW_SESSION_LIMIT }),
    placeholderData: keepPreviousData,
  });

  const authError = [
    recentTracesQuery.error,
    failedTracesQuery.error,
    runningTracesQuery.error,
    sessionsQuery.error,
  ].find(isAuthError);
  const recentTracesError = getOverviewQueryError(recentTracesQuery.error, recentTracesQuery.data);
  const failedTracesError = getOverviewQueryError(failedTracesQuery.error, failedTracesQuery.data);
  const runningTracesError = getOverviewQueryError(runningTracesQuery.error, runningTracesQuery.data);
  const sessionsError = getOverviewQueryError(sessionsQuery.error, sessionsQuery.data);
  const otherErrors = [
    recentTracesQuery.error,
    failedTracesQuery.error,
    runningTracesQuery.error,
    sessionsQuery.error,
  ].filter((error): error is Error => error instanceof Error && !isAuthError(error));

  if (authError) {
    return <AuthErrorBanner message={authError.message} />;
  }

  const recentTraces = recentTracesQuery.data?.traces ?? [];
  const failedTraces = failedTracesQuery.data?.traces ?? [];
  const runningTraces = runningTracesQuery.data?.traces ?? [];
  const sessions = sessionsQuery.data?.sessions ?? [];
  const totalTraces = recentTracesQuery.data?.total ?? 0;
  const totalFailedTraces = failedTracesQuery.data?.total ?? 0;
  const totalRunningTraces = runningTracesQuery.data?.total ?? 0;
  const totalSessions = sessionsQuery.data?.total ?? 0;
  const activeLoadState = [
    recentTracesQuery.isFetching,
    failedTracesQuery.isFetching,
    runningTracesQuery.isFetching,
    sessionsQuery.isFetching,
  ].some(Boolean);
  return (
    <div className="app-page">
      <section className="app-surface relative overflow-hidden">
        <div className="absolute inset-y-0 right-0 hidden w-[28rem] bg-[radial-gradient(circle_at_top,rgba(0,92,171,0.12),transparent_62%)] lg:block" />
        <div className="relative flex flex-col gap-8 xl:flex-row xl:items-end xl:justify-between">
          <div className="max-w-3xl">
            <div className="app-overline">Operator Overview</div>
            <h1 className="mt-4 max-w-2xl text-4xl font-black tight-headline text-[var(--continua-text-primary)] sm:text-5xl">
              Trace the work that matters before it turns into support debt.
            </h1>
            <p className="mt-4 max-w-2xl text-base leading-7 text-[var(--continua-text-secondary)] sm:text-lg">
              Recent failures, live executions, and session workflows stay in one
              operator surface so you can move from signal to root cause quickly.
            </p>
          </div>

          <div className="flex flex-wrap gap-3">
            <Link to="/traces" className="app-button-primary">
              Open traces
            </Link>
            <Link to="/sessions" className="app-button-secondary">
              Open sessions
            </Link>
          </div>
        </div>
      </section>

      {otherErrors.length > 0 ? (
        <div className="app-alert-error">
          Overview data is partially unavailable. {otherErrors[0].message}
        </div>
      ) : null}

      <section className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <OverviewMetric
          label="Tracked traces"
          value={formatOverviewMetricValue(totalTraces, recentTracesError)}
          hint="Recent request inventory"
        />
        <OverviewMetric
          label="Running now"
          value={formatOverviewMetricValue(totalRunningTraces, runningTracesError)}
          hint="Currently polling for progress"
        />
        <OverviewMetric
          label="Failed traces"
          value={formatOverviewMetricValue(totalFailedTraces, failedTracesError)}
          hint="Failure queue in scope"
        />
        <OverviewMetric
          label="Sessions"
          value={formatOverviewMetricValue(totalSessions, sessionsError)}
          hint="Workflow groups in scope"
        />
      </section>

      <section className="grid gap-5 xl:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]">
        <div className="app-surface">
          <div className="app-section-header">
            <div>
              <div className="app-overline">Failures</div>
              <h2 className="mt-2 text-2xl font-black tight-headline text-[var(--continua-text-primary)]">
                Recent failed traces
              </h2>
            </div>
            <Link to="/traces?status=failed" className="app-inline-link">
              View all failed traces
            </Link>
          </div>

            <TraceActivityList
              traces={failedTraces}
              emptyTitle="No failed traces"
              emptyBody="Failures will surface here as soon as they happen."
              errorMessage={failedTracesError}
              isLoading={failedTracesQuery.isPending && !failedTracesQuery.data}
              returnTo={returnTo}
            />
        </div>

        <div className="space-y-5">
          <div className="app-surface">
            <div className="app-section-header">
              <div>
                <div className="app-overline">Live work</div>
                <h2 className="mt-2 text-2xl font-black tight-headline text-[var(--continua-text-primary)]">
                  Running traces
                </h2>
              </div>
              <Link to="/traces?status=running" className="app-inline-link">
                Open live triage
              </Link>
            </div>

            <TraceActivityList
              traces={runningTraces}
              emptyTitle="No running traces"
              emptyBody="When executions are in flight they show up here."
              errorMessage={runningTracesError}
              isLoading={runningTracesQuery.isPending && !runningTracesQuery.data}
              returnTo={returnTo}
            />
          </div>

          <div className="app-surface">
            <div className="app-section-header">
              <div>
                <div className="app-overline">Session workflows</div>
                <h2 className="mt-2 text-2xl font-black tight-headline text-[var(--continua-text-primary)]">
                  Recent sessions
                </h2>
              </div>
              <Link to="/sessions" className="app-inline-link">
                Open session index
              </Link>
            </div>

            <SessionActivityList
              sessions={sessions}
              emptyTitle="No sessions yet"
              emptyBody="Sessions appear when traces are grouped under a shared session identifier."
              errorMessage={sessionsError}
              isLoading={sessionsQuery.isPending && !sessionsQuery.data}
              returnTo={returnTo}
            />
          </div>
        </div>
      </section>

      <section className="app-surface">
        <div className="app-section-header">
          <div>
            <div className="app-overline">Recent activity</div>
            <h2 className="mt-2 text-2xl font-black tight-headline text-[var(--continua-text-primary)]">
              Latest traces
            </h2>
          </div>
          <div className="flex flex-wrap items-center gap-2 text-sm text-[var(--continua-text-muted)]">
            <span>{totalTraces} total traces</span>
            {activeLoadState ? <span>Refreshing…</span> : null}
          </div>
        </div>

        <TraceActivityList
          traces={recentTraces}
          emptyTitle="No traces yet"
          emptyBody="Start ingesting spans or run the demo flow to populate the debugger."
          errorMessage={recentTracesError}
          isLoading={recentTracesQuery.isPending && !recentTracesQuery.data}
          returnTo={returnTo}
        />
      </section>
    </div>
  );
}

function OverviewMetric({
  label,
  value,
  hint,
}: {
  label: string;
  value: string;
  hint: string;
}) {
  return (
    <article className="app-metric-panel">
      <div className="app-overline">{label}</div>
      <div className="mt-3 text-3xl font-black tight-headline text-[var(--continua-text-primary)]">
        {value}
      </div>
      <p className="mt-2 text-sm text-[var(--continua-text-muted)]">{hint}</p>
    </article>
  );
}

function TraceActivityList({
  traces,
  emptyTitle,
  emptyBody,
  errorMessage,
  isLoading,
  returnTo,
}: {
  traces: Trace[];
  emptyTitle: string;
  emptyBody: string;
  errorMessage: string | null;
  isLoading: boolean;
  returnTo: string;
}) {
  if (isLoading) {
    return <div className="app-empty-state">Loading traces…</div>;
  }

  if (errorMessage) {
    return <div className="app-alert-error mt-5">Could not load traces: {errorMessage}</div>;
  }

  if (traces.length === 0) {
    return (
      <div className="app-empty-state">
        <h3 className="text-base font-bold text-[var(--continua-text-primary)]">{emptyTitle}</h3>
        <p className="mt-2">{emptyBody}</p>
      </div>
    );
  }

  return (
    <div className="mt-5 space-y-3">
      {traces.map((trace) => {
        const totalTokens = (trace.total_tokens_in ?? 0) + (trace.total_tokens_out ?? 0);

        return (
          <Link
            key={trace.id}
            to={`/traces/${trace.id}`}
            state={{ returnTo }}
            className="app-list-row group"
          >
            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <div className="truncate text-sm font-semibold text-[var(--continua-text-primary)] transition group-hover:text-[var(--continua-accent)]">
                  {trace.name}
                </div>
                <StatusBadge status={trace.status} />
              </div>

              <div className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-[var(--continua-text-muted)]">
                <span>{formatRelativeTime(trace.started_at)}</span>
                <span>{trace.status === 'RUNNING' ? 'Live now' : 'Trace complete'}</span>
                {trace.session_external_id ? <span>{trace.session_external_id}</span> : null}
                {trace.error_count && trace.error_count > 0 ? (
                  <span className="text-[var(--continua-error)]">{trace.error_count} errors</span>
                ) : null}
              </div>
            </div>

            <div className="flex shrink-0 flex-col items-end gap-1 text-right text-xs text-[var(--continua-text-muted)]">
              <span>{formatTokens(totalTokens)}</span>
              <span>{formatCost(trace.total_cost_usd)}</span>
            </div>
          </Link>
        );
      })}
    </div>
  );
}

function SessionActivityList({
  sessions,
  emptyTitle,
  emptyBody,
  errorMessage,
  isLoading,
  returnTo,
}: {
  sessions: Session[];
  emptyTitle: string;
  emptyBody: string;
  errorMessage: string | null;
  isLoading: boolean;
  returnTo: string;
}) {
  if (isLoading) {
    return <div className="app-empty-state">Loading sessions…</div>;
  }

  if (errorMessage) {
    return <div className="app-alert-error mt-5">Could not load sessions: {errorMessage}</div>;
  }

  if (sessions.length === 0) {
    return (
      <div className="app-empty-state">
        <h3 className="text-base font-bold text-[var(--continua-text-primary)]">{emptyTitle}</h3>
        <p className="mt-2">{emptyBody}</p>
      </div>
    );
  }

  return (
    <div className="mt-5 space-y-3">
      {sessions.map((session) => (
        <Link
          key={session.id}
          to={`/sessions/${session.id}`}
          state={{ returnTo }}
          className="app-list-row group"
        >
          <div className="min-w-0">
            <div className="truncate text-sm font-semibold text-[var(--continua-text-primary)] transition group-hover:text-[var(--continua-accent)]">
              {session.external_id}
            </div>
            <div className="mt-1 text-xs text-[var(--continua-text-muted)]">
              {session.name || 'Unnamed session'}
            </div>
          </div>

          <div className="flex shrink-0 flex-col items-end gap-1 text-right text-xs text-[var(--continua-text-muted)]">
            <span>{session.trace_count ?? 0} traces</span>
            <span>{formatRelativeTime(session.created_at)}</span>
          </div>
        </Link>
      ))}
    </div>
  );
}

function getOverviewQueryError(
  error: unknown,
  data: unknown
): string | null {
  if (!error || data || isAuthError(error)) {
    return null;
  }

  return error instanceof Error ? error.message : 'Unknown error';
}

function formatOverviewMetricValue(value: number, errorMessage: string | null): string {
  return errorMessage ? 'Unavailable' : String(value);
}
