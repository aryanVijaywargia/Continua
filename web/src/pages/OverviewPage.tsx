import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { Link, useLocation } from 'react-router-dom';
import { ArrowRight, RefreshCw, Zap } from 'lucide-react';
import {
  fetchSessions,
  fetchTraces,
  isAuthError,
  type Trace,
} from '../api/client';
import { AuthErrorBanner } from '../components/AuthErrorBanner';
import {
  Btn,
  Chip,
  DataTable,
  StatusDot,
  Td,
  Th,
  Tr,
} from '../components/DebuggerKit';
import {
  calculateDuration,
  formatCost,
  formatDuration,
  formatRelativeTime,
  formatTokens,
} from '../utils/format';
import {
  appendProjectToPath,
  getProjectIdFromSearchParams,
} from '../utils/projectSearchParams';

const OVERVIEW_TRACE_LIMIT = 12;
const OVERVIEW_SESSION_LIMIT = 6;

export function OverviewPage() {
  return <OverviewContent />;
}

function OverviewContent() {
  const location = useLocation();
  const returnTo = `${location.pathname}${location.search}`;
  const currentProjectId = getProjectIdFromSearchParams(
    new URLSearchParams(location.search)
  );
  const projectQueryKey = currentProjectId ?? null;
  const recentTracesQuery = useQuery({
    queryKey: ['overview', 'recent-traces', projectQueryKey],
    queryFn: () =>
      fetchTraces({ project_id: currentProjectId, limit: OVERVIEW_TRACE_LIMIT }),
    placeholderData: keepPreviousData,
  });
  const failedTracesQuery = useQuery({
    queryKey: ['overview', 'failed-traces', projectQueryKey],
    queryFn: () =>
      fetchTraces({
        project_id: currentProjectId,
        limit: OVERVIEW_TRACE_LIMIT,
        status: 'failed',
      }),
    placeholderData: keepPreviousData,
  });
  const runningTracesQuery = useQuery({
    queryKey: ['overview', 'running-traces', projectQueryKey],
    queryFn: () =>
      fetchTraces({
        project_id: currentProjectId,
        limit: OVERVIEW_TRACE_LIMIT,
        status: 'running',
      }),
    placeholderData: keepPreviousData,
  });
  const sessionsQuery = useQuery({
    queryKey: ['overview', 'sessions', projectQueryKey],
    queryFn: () =>
      fetchSessions({ project_id: currentProjectId, limit: OVERVIEW_SESSION_LIMIT }),
    placeholderData: keepPreviousData,
  });

  const authError = [
    recentTracesQuery.error,
    failedTracesQuery.error,
    runningTracesQuery.error,
    sessionsQuery.error,
  ].find(isAuthError);
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
  const totalTokens = recentTraces.reduce(
    (sum, trace) => sum + (trace.total_tokens_in ?? 0) + (trace.total_tokens_out ?? 0),
    0
  );
  const totalSpend = recentTraces.reduce(
    (sum, trace) => sum + (trace.total_cost_usd ?? 0),
    0
  );
  const errors = [
    recentTracesQuery.error,
    failedTracesQuery.error,
    runningTracesQuery.error,
    sessionsQuery.error,
  ].filter((error): error is Error => error instanceof Error && !isAuthError(error));

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {errors.length > 0 ? (
        <div className="border-b border-[var(--c-red-border)] bg-[var(--c-red-faint)] px-6 py-3 text-sm text-[var(--c-red-text)]">
          Overview data is partially unavailable. {errors[0].message}
        </div>
      ) : null}

      <section className="flex border-b border-[var(--c-border)] bg-[var(--c-app-bg)]">
        <KpiCard
          label="Tracked traces"
          value={formatNumber(totalTraces)}
          delta={`${recentTraces.length} loaded`}
          spark={recentTraces.map((trace) => trace.error_count ?? 0)}
        />
        <KpiCard
          label="Running now"
          value={formatNumber(totalRunningTraces)}
          delta="polling"
          tone="running"
          spark={runningTraces.map((trace) => trace.error_count ?? 0)}
        />
        <KpiCard
          label="Failed traces"
          value={formatNumber(totalFailedTraces)}
          delta={totalTraces ? `${Math.round((totalFailedTraces / totalTraces) * 100)}%` : '0%'}
          tone="failed"
          spark={failedTraces.map((trace) => trace.error_count ?? 0)}
        />
        <KpiCard
          label="Sessions"
          value={formatNumber(totalSessions)}
          delta={`${sessions.length} loaded`}
          spark={sessions.map((session) => session.trace_count ?? 0)}
        />
        <KpiCard label="Tokens loaded" value={formatTokens(totalTokens)} delta={formatCost(totalSpend)} />
        <div className="flex items-center px-4">
          <Btn
            kind="secondary"
            leadingIcon={RefreshCw}
            size="sm"
            onClick={() => {
              void recentTracesQuery.refetch();
              void failedTracesQuery.refetch();
              void runningTracesQuery.refetch();
              void sessionsQuery.refetch();
            }}
          >
            Refresh
          </Btn>
        </div>
      </section>

      <section className="grid border-b border-[var(--c-border)] lg:grid-cols-[minmax(0,1fr)_360px]">
        <div className="border-r border-[var(--c-border)] px-6 py-5">
          <div className="mb-3 flex items-start justify-between gap-4">
            <div>
              <h2 className="text-[13px] font-semibold text-[var(--c-text-primary)]">
                Trace volume
              </h2>
              <p className="mt-0.5 text-[11.5px] text-[var(--c-text-muted)]">
                Current page sample · existing trace endpoints
              </p>
            </div>
            <div className="flex items-center gap-3 text-[11.5px] text-[var(--c-text-secondary)]">
              <Legend color="var(--c-bar-success)" label="Completed" />
              <Legend color="var(--c-bar-running)" label="Running" />
              <Legend color="var(--c-bar-failed)" label="Failed" />
            </div>
          </div>
          <ActivityBars traces={recentTraces} />
        </div>

        <div className="px-6 py-5">
          <div className="mb-3 flex items-start justify-between gap-4">
            <div>
              <h2 className="text-[13px] font-semibold text-[var(--c-text-primary)]">
                Live runs
              </h2>
              <p className="mt-0.5 text-[11.5px] text-[var(--c-text-muted)]">
                Currently executing
              </p>
            </div>
            <span className="font-mono text-[11px] tabular-nums text-[var(--c-text-muted)]">
              {totalRunningTraces} active
            </span>
          </div>
          <div className="flex flex-col gap-2.5">
            {runningTraces.length === 0 ? (
              <div className="text-sm text-[var(--c-text-muted)]">No running traces.</div>
            ) : (
              runningTraces.slice(0, 5).map((trace) => (
                <Link
                  key={trace.id}
                  to={appendProjectToPath(`/traces/${trace.id}`, currentProjectId)}
                  state={{ returnTo }}
                  className="flex items-center justify-between gap-3 text-xs hover:text-[var(--c-accent-text)]"
                >
                  <span className="flex min-w-0 items-center gap-2">
                    <StatusDot status={trace.status} withLabel={false} />
                    <span className="truncate font-mono text-[var(--c-text-primary)]">
                      {trace.name}
                    </span>
                  </span>
                  <span className="truncate text-[var(--c-text-muted)]">
                    {trace.engine?.definition_name ?? 'trace'}
                  </span>
                  <span className="min-w-[3rem] text-right font-mono text-[var(--c-text-muted)]">
                    {formatRelativeTime(trace.started_at)}
                  </span>
                </Link>
              ))
            )}
          </div>
        </div>
      </section>

      <section className="flex min-h-0 flex-1 flex-col">
        <div className="flex items-center justify-between px-6 py-4">
          <h2 className="text-[13px] font-semibold text-[var(--c-text-primary)]">
            Recent traces
          </h2>
          <Link
            to={appendProjectToPath('/traces', currentProjectId)}
            className="inline-flex items-center gap-1 text-xs font-medium text-[var(--c-accent-text)]"
          >
            View all <ArrowRight className="h-3 w-3" />
          </Link>
        </div>
        {recentTracesQuery.isPending && !recentTracesQuery.data ? (
          <div className="app-empty-state">Loading traces...</div>
        ) : recentTraces.length === 0 ? (
          <div className="app-empty-state">No traces yet.</div>
        ) : (
          <DataTable className="flex-none overflow-visible">
            <colgroup>
              <col className="w-[34%]" />
              <col className="w-[110px]" />
              <col className="w-[120px]" />
              <col className="w-[100px]" />
              <col className="w-[90px]" />
              <col className="w-[130px]" />
            </colgroup>
            <thead>
              <tr>
                <Th>Trace</Th>
                <Th>Status</Th>
                <Th align="right">Duration</Th>
                <Th align="right">Tokens</Th>
                <Th align="right">Cost</Th>
                <Th align="right">Started</Th>
              </tr>
            </thead>
            <tbody>
              {recentTraces.slice(0, 8).map((trace) => (
                <OverviewTraceRow
                  key={trace.id}
                  projectId={currentProjectId}
                  returnTo={returnTo}
                  trace={trace}
                />
              ))}
            </tbody>
          </DataTable>
        )}
      </section>
    </div>
  );
}

function KpiCard({
  delta,
  label,
  spark = [],
  tone = 'muted',
  value,
}: {
  delta?: string;
  label: string;
  spark?: number[];
  tone?: 'muted' | 'running' | 'failed';
  value: string;
}) {
  const color =
    tone === 'failed'
      ? 'var(--c-red)'
      : tone === 'running'
        ? 'var(--c-blue)'
        : 'var(--c-accent)';

  return (
    <div className="min-w-0 flex-1 border-r border-[var(--c-border)] px-4 py-3.5">
      <div className="mb-2 text-[11.5px] font-medium text-[var(--c-text-muted)]">
        {label}
      </div>
      <div className="mb-1 flex items-baseline gap-1.5">
        <span className="text-[22px] font-semibold tracking-[-0.02em] text-[var(--c-text-primary)]">
          {value}
        </span>
      </div>
      <div className="flex items-center justify-between gap-2">
        {delta ? (
          <span className="text-[11.5px] font-medium text-[var(--c-text-muted)]">
            {delta}
          </span>
        ) : null}
        <Sparkline color={color} data={spark} />
      </div>
    </div>
  );
}

function Sparkline({ color, data }: { color: string; data: number[] }) {
  const normalizedData = data.length > 1 ? data : [0, 1, 0, 1, 0];
  const max = Math.max(...normalizedData, 1);
  const min = Math.min(...normalizedData, 0);
  const range = max - min || 1;
  const width = 96;
  const height = 28;
  const points = normalizedData
    .map(
      (value, index) =>
        `${(index / (normalizedData.length - 1)) * width},${height - ((value - min) / range) * (height - 4) - 2}`
    )
    .join(' ');

  return (
    <svg width={width} height={height} className="block">
      <polyline
        fill="none"
        points={points}
        stroke={color}
        strokeLinecap="round"
        strokeLinejoin="round"
        strokeWidth="1.5"
      />
    </svg>
  );
}

function ActivityBars({ traces }: { traces: Trace[] }) {
  const buckets = buildActivityBuckets(traces);
  const max = Math.max(
    ...buckets.map((bucket) => bucket.completed + bucket.failed + bucket.running),
    1
  );

  return (
    <>
      <div className="flex h-36 items-end gap-1">
        {buckets.map((bucket, index) => {
          const total = bucket.completed + bucket.failed + bucket.running;
          const height = Math.max((total / max) * 140, total ? 8 : 2);
          return (
            <div
              key={index}
              className="flex min-w-0 flex-1 flex-col-reverse gap-px"
              style={{ height }}
            >
              <div
                style={{
                  background: 'var(--c-bar-success)',
                  height: `${total ? (bucket.completed / total) * 100 : 0}%`,
                }}
              />
              <div
                style={{
                  background: 'var(--c-bar-running)',
                  height: `${total ? (bucket.running / total) * 100 : 0}%`,
                }}
              />
              <div
                style={{
                  background: 'var(--c-bar-failed)',
                  height: `${total ? (bucket.failed / total) * 100 : 0}%`,
                }}
              />
            </div>
          );
        })}
      </div>
      <div className="mt-2 flex justify-between font-mono text-[10.5px] text-[var(--c-text-muted)]">
        <span>oldest</span>
        <span>recent</span>
      </div>
    </>
  );
}

function buildActivityBuckets(traces: Trace[]) {
  const buckets = Array.from({ length: 24 }, () => ({
    completed: 0,
    failed: 0,
    running: 0,
  }));
  traces.forEach((trace, index) => {
    const bucket = buckets[index % buckets.length];
    if (trace.status === 'FAILED') {
      bucket.failed += 1;
    } else if (trace.status === 'RUNNING') {
      bucket.running += 1;
    } else {
      bucket.completed += 1;
    }
  });
  return buckets;
}

function Legend({ color, label }: { color: string; label: string }) {
  return (
    <span className="inline-flex items-center gap-1.5">
      <span className="h-2 w-2" style={{ background: color }} />
      {label}
    </span>
  );
}

function OverviewTraceRow({
  projectId,
  returnTo,
  trace,
}: {
  projectId?: string;
  returnTo: string;
  trace: Trace;
}) {
  const duration = calculateDuration(trace.started_at, trace.ended_at);
  const totalTokens = (trace.total_tokens_in ?? 0) + (trace.total_tokens_out ?? 0);

  return (
    <Tr>
      <Td>
        <Link
          to={appendProjectToPath(`/traces/${trace.id}`, projectId)}
          state={{ returnTo }}
          className="flex min-w-0 items-center gap-2 hover:text-[var(--c-accent-text)]"
        >
          <span className="truncate font-mono text-[12.5px] font-medium text-[var(--c-text-primary)]">
            {trace.name}
          </span>
          {trace.engine ? (
            <Chip icon={Zap}>{trace.engine.definition_name}</Chip>
          ) : null}
        </Link>
      </Td>
      <Td>
        <StatusDot status={trace.status} />
      </Td>
      <Td align="right" mono>
        {formatDuration(duration)}
      </Td>
      <Td align="right" mono>
        {formatTokens(totalTokens)}
      </Td>
      <Td align="right" mono>
        {formatCost(trace.total_cost_usd)}
      </Td>
      <Td align="right" dim>
        {formatRelativeTime(trace.started_at)}
      </Td>
    </Tr>
  );
}

function formatNumber(value: number) {
  return new Intl.NumberFormat('en-US', {
    notation: value >= 10000 ? 'compact' : 'standard',
  }).format(value);
}
