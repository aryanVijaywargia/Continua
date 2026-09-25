import { useQuery, useQueryClient, type QueryClient } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { Link, useLocation } from 'react-router-dom';
import { ArrowUpRight, CircleAlert, PowerOff, RefreshCw } from 'lucide-react';
import {
  ApiError,
  fetchEngineHealth,
  isAuthError,
  type EngineHealthResponse,
} from '../api/client';
import { AuthErrorBanner } from '../components/AuthErrorBanner';
import { StaleNotice } from '../components/DataState';
import {
  Btn,
  Chip,
  DataTable,
  PageHeader,
  Td,
  Th,
  Tr,
} from '../components/DebuggerKit';
import { formatTimestamp } from '../utils/format';
import {
  buildProjectPath,
  getProjectIdFromSearchParams,
} from '../utils/projectSearchParams';

const ENGINE_HEALTH_PATH = '/v1/engine/health';
const ENABLE_ENGINE_API_DOCS_URL = 'https://www.continua.in/docs/debugger/engine-runs';
const POLL_INTERVAL_MS = 5000;

const PRIMARY_LINK_CLASS =
  'inline-flex h-7 items-center justify-center gap-1.5 rounded-md border border-transparent bg-[var(--c-text-primary)] px-2.5 text-xs font-semibold text-[var(--c-app-bg)] transition';
const SECONDARY_LINK_CLASS =
  'inline-flex h-7 items-center justify-center gap-1.5 rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-2.5 text-xs font-medium text-[var(--c-text-primary)] transition hover:border-[var(--c-border-strong)]';

/**
 * Every /v1/engine/* route returns 404 when the server runs with
 * ENGINE_PUBLIC_API_ENABLED off. That is a configuration fact, so the page
 * neither retries nor polls it.
 */
function isEngineApiDisabled(error: unknown): error is ApiError {
  return error instanceof ApiError && error.status === 404;
}

/** Keeps the client's retry policy for real failures and skips retries that cannot help. */
function shouldRetry(queryClient: QueryClient, failureCount: number, error: Error): boolean {
  if (isEngineApiDisabled(error) || isAuthError(error)) {
    return false;
  }
  const retry = queryClient.getDefaultOptions().queries?.retry ?? 3;
  if (typeof retry === 'function') {
    return retry(failureCount, error);
  }
  if (typeof retry === 'boolean') {
    return retry;
  }
  return failureCount < retry;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Unknown error';
}

function describeAge(updatedAtMs: number, nowMs: number): string {
  const minutes = Math.floor(Math.max(0, nowMs - updatedAtMs) / 60_000);
  if (minutes < 1) {
    return 'less than a minute';
  }
  if (minutes < 60) {
    return minutes === 1 ? '1 minute' : `${minutes} minutes`;
  }
  const hours = Math.floor(minutes / 60);
  return hours === 1 ? '1 hour' : `${hours} hours`;
}

function StatTile({
  label,
  staleAge,
  state = 'ok',
  value,
}: {
  label: string;
  staleAge?: string;
  state?: 'ok' | 'warn';
  value: number;
}) {
  return (
    <div
      data-state={state}
      className={`rounded-md border px-3.5 py-3 ${
        state === 'warn'
          ? 'border-[var(--c-amber-border)] bg-[var(--c-amber-faint)]'
          : 'border-[var(--c-border)] bg-[var(--c-surface)]'
      } ${staleAge ? 'opacity-75' : ''}`}
    >
      <div className="text-[11px] font-medium text-[var(--c-text-secondary)]">{label}</div>
      <div className="mt-1.5 font-mono text-xl font-bold tabular-nums text-[var(--c-text-primary)]">
        {value}
      </div>
      {staleAge ? (
        <div className="mt-1">
          <StaleNotice age={`${staleAge} old`} />
        </div>
      ) : null}
    </div>
  );
}

function StateCard({
  children,
  icon,
  state,
  title,
  tone,
}: {
  children: ReactNode;
  icon: ReactNode;
  state: 'capability-disabled' | 'request-failed';
  title: string;
  tone: 'muted' | 'error';
}) {
  return (
    <section
      data-state={state}
      role={tone === 'error' ? 'alert' : undefined}
      className="rounded-lg border border-[var(--c-border)] bg-[var(--c-surface)] p-6"
    >
      <div className="flex items-start gap-3.5">
        <span
          className={`inline-flex h-[34px] w-[34px] shrink-0 items-center justify-center rounded-lg ${
            tone === 'error'
              ? 'bg-[var(--c-red-faint)] text-[var(--c-red)]'
              : 'bg-[var(--c-surface-muted)] text-[var(--c-text-secondary)]'
          }`}
        >
          {icon}
        </span>
        <div className="min-w-0 flex-1">
          <h2 className="text-sm font-bold text-[var(--c-text-primary)]">{title}</h2>
          {children}
        </div>
      </div>
    </section>
  );
}

function HealthMetrics({
  health,
  staleAge,
}: {
  health: EngineHealthResponse;
  staleAge?: string;
}) {
  return (
    <>
      <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
        <StatTile
          label="Projector lag"
          staleAge={staleAge}
          state={health.projector.lag_rows > 0 ? 'warn' : 'ok'}
          value={health.projector.lag_rows}
        />
        <StatTile
          label="Runs catching up"
          staleAge={staleAge}
          value={health.projector.runs_catching_up}
        />
        <StatTile label="Runs ready" staleAge={staleAge} value={health.queues.runs_ready} />
        <StatTile
          label="Activity tasks pending"
          staleAge={staleAge}
          value={health.queues.activity_tasks_pending}
        />
        <StatTile label="Inbox pending" staleAge={staleAge} value={health.queues.inbox_pending} />
      </section>

      <section className={`mt-6 ${staleAge ? 'opacity-75' : ''}`}>
        <h2 className="text-[13px] font-semibold text-[var(--c-text-primary)]">Workers</h2>
        <div className="mt-2.5 overflow-hidden rounded-md border border-[var(--c-border)]">
          {health.workers.length === 0 ? (
            <div className="px-4 py-8 text-center text-[13px] text-[var(--c-text-muted)]">
              No workers have claimed engine work.
            </div>
          ) : (
            <DataTable>
              <thead>
                <tr>
                  <Th>Worker</Th>
                  <Th>Last claim</Th>
                  <Th>Leases</Th>
                  <Th>Status</Th>
                </tr>
              </thead>
              <tbody>
                {health.workers.map((worker) => (
                  <Tr
                    key={worker.id}
                    data-state={worker.status}
                    className={worker.status === 'stale' ? 'bg-[var(--c-red-faint)]' : ''}
                  >
                    <Td mono>
                      <span className="text-xs">{worker.id}</span>
                    </Td>
                    <Td mono dim>
                      <time className="text-xs" dateTime={worker.last_claim_at}>
                        {formatTimestamp(worker.last_claim_at)}
                      </time>
                    </Td>
                    <Td mono>
                      <span className="text-xs">
                        {worker.active_leases} active / {worker.expired_leases} expired
                      </span>
                    </Td>
                    <Td>
                      <Chip tone={worker.status === 'active' ? 'success' : 'error'}>
                        {worker.status}
                      </Chip>
                    </Td>
                  </Tr>
                ))}
              </tbody>
            </DataTable>
          )}
        </div>
      </section>

      <section className={`mt-6 ${staleAge ? 'opacity-75' : ''}`}>
        <h2 className="text-[13px] font-semibold text-[var(--c-text-primary)]">Retention</h2>
        <div className="mt-2.5 grid gap-3 sm:grid-cols-2">
          <StatTile
            label="Summary-only runs"
            staleAge={staleAge}
            value={health.retention.summary_only_runs}
          />
          <StatTile
            label="Journal-expired runs"
            staleAge={staleAge}
            value={health.retention.journal_expired_runs}
          />
        </div>
      </section>

      <p className="mt-4 text-right font-mono text-[11px] text-[var(--c-text-muted)]">
        Generated {formatTimestamp(health.generated_at)}
      </p>
    </>
  );
}

export function EngineHealthPage() {
  const location = useLocation();
  const projectId = getProjectIdFromSearchParams(new URLSearchParams(location.search));
  const queryClient = useQueryClient();
  const healthQuery = useQuery({
    queryKey: ['engine-health', projectId ?? null],
    queryFn: fetchEngineHealth,
    refetchInterval: (query) =>
      isEngineApiDisabled(query.state.error) || isAuthError(query.state.error)
        ? false
        : POLL_INTERVAL_MS,
    retry: (failureCount, error) => shouldRetry(queryClient, failureCount, error),
  });
  const health = healthQuery.data;
  const error = healthQuery.error;

  let body: ReactNode = null;
  if (error && isAuthError(error)) {
    body = <AuthErrorBanner message={errorMessage(error)} />;
  } else if (error && isEngineApiDisabled(error)) {
    body = (
      <StateCard
        icon={<PowerOff aria-hidden="true" className="h-[17px] w-[17px]" />}
        state="capability-disabled"
        title="Engine health is not enabled on this server"
        tone="muted"
      >
        <p className="mt-1.5 max-w-[520px] text-[12.5px] leading-[1.6] text-[var(--c-text-secondary)]">
          A configuration state, not an error. The engine API is disabled, so there is no
          projector, queue, or worker data to report. Retrying will not change the result.
        </p>
        <div className="mt-3.5 flex flex-wrap gap-2">
          <Link className={PRIMARY_LINK_CLASS} to={buildProjectPath('/traces', projectId)}>
            Back to Traces
          </Link>
          <a
            className={SECONDARY_LINK_CLASS}
            href={ENABLE_ENGINE_API_DOCS_URL}
            rel="noreferrer"
            target="_blank"
          >
            How to enable the engine API
            <ArrowUpRight aria-hidden="true" className="h-3 w-3" />
          </a>
        </div>
        <p className="mt-3.5 font-mono text-[11px] text-[var(--c-text-muted)]">
          engine api · disabled · returned {error.status}
        </p>
      </StateCard>
    );
  } else if (error) {
    const staleAge = health
      ? describeAge(healthQuery.dataUpdatedAt, healthQuery.errorUpdatedAt || Date.now())
      : undefined;
    const serverAnswered = error instanceof ApiError;
    body = (
      <>
        <StateCard
          icon={<CircleAlert aria-hidden="true" className="h-[17px] w-[17px]" />}
          state="request-failed"
          title="Could not load engine health"
          tone="error"
        >
          <p className="mt-1.5 max-w-[520px] text-[12.5px] leading-[1.6] text-[var(--c-text-secondary)]">
            {serverAnswered
              ? 'The engine API is enabled but the request failed.'
              : 'The request failed before the server answered.'}{' '}
            {staleAge
              ? `The last successful read was ${staleAge} ago, shown below as stale rather than hidden.`
              : 'No successful read is available yet.'}
          </p>
          <div className="mt-3.5 flex flex-wrap gap-2">
            <Btn
              disabled={healthQuery.isFetching}
              kind="primary"
              leadingIcon={RefreshCw}
              size="sm"
              onClick={() => void healthQuery.refetch()}
            >
              Retry
            </Btn>
            <Link className={SECONDARY_LINK_CLASS} to={buildProjectPath('/engine/runs', projectId)}>
              Back to Engine Runs
            </Link>
          </div>
          <p className="mt-3.5 break-words font-mono text-[11px] text-[var(--c-text-muted)]">
            GET {ENGINE_HEALTH_PATH} · {serverAnswered ? error.status : 'no response'} ·{' '}
            {errorMessage(error)}
          </p>
        </StateCard>
        {health ? (
          <div className="mt-6">
            <HealthMetrics health={health} staleAge={staleAge} />
          </div>
        ) : null}
      </>
    );
  } else if (healthQuery.isPending) {
    body = <div className="app-empty-state">Loading engine health...</div>;
  } else if (health) {
    body = <HealthMetrics health={health} />;
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <PageHeader
        description={
          error && isEngineApiDisabled(error)
            ? 'Projector progress, ready work, and worker leases · engine API disabled'
            : 'Projector progress, ready work, and worker leases · auto-refreshing every 5s'
        }
        title="Engine health"
      />
      <div className="min-h-0 flex-1 overflow-auto p-6">{body}</div>
    </div>
  );
}
