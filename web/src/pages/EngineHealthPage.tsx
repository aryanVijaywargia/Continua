import { useQuery } from '@tanstack/react-query';
import { useLocation } from 'react-router-dom';
import { fetchEngineHealth } from '../api/client';
import {
  Chip,
  DataTable,
  PageHeader,
  Td,
  Th,
  Tr,
} from '../components/DebuggerKit';
import { formatTimestamp } from '../utils/format';
import { getProjectIdFromSearchParams } from '../utils/projectSearchParams';

function StatTile({
  label,
  state = 'ok',
  value,
}: {
  label: string;
  state?: 'ok' | 'warn';
  value: number;
}) {
  return (
    <div
      data-state={state}
      className={`rounded-md border p-4 ${
        state === 'warn'
          ? 'border-[var(--c-amber-border)] bg-[var(--c-amber-faint)]'
          : 'border-[var(--c-border)] bg-[var(--c-surface)]'
      }`}
    >
      <div className="text-xs font-medium text-[var(--c-text-secondary)]">
        {label}
      </div>
      <div className="mt-2 font-mono text-2xl font-bold tabular-nums text-[var(--c-text-primary)]">
        {value}
      </div>
    </div>
  );
}

export function EngineHealthPage() {
  const location = useLocation();
  const projectId = getProjectIdFromSearchParams(new URLSearchParams(location.search));
  const healthQuery = useQuery({
    queryKey: ['engine-health', projectId ?? null],
    queryFn: fetchEngineHealth,
    refetchInterval: 5000,
  });
  const health = healthQuery.data;

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <PageHeader
        description="Projector progress, ready work, and worker leases · auto-refreshing every 5s"
        title="Engine health"
      />

      {healthQuery.error ? (
        <div
          role="alert"
          className="border-b border-[var(--c-red-border)] bg-[var(--c-red-faint)] px-6 py-3 text-sm text-[var(--c-red-text)]"
        >
          Could not load engine health: {healthQuery.error.message}
        </div>
      ) : null}

      {healthQuery.isPending ? (
        <div className="app-empty-state">Loading engine health...</div>
      ) : health ? (
        <div className="min-h-0 flex-1 overflow-auto p-6">
          <section className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
            <StatTile
              label="Projector lag"
              state={health.projector.lag_rows > 0 ? 'warn' : 'ok'}
              value={health.projector.lag_rows}
            />
            <StatTile
              label="Runs catching up"
              value={health.projector.runs_catching_up}
            />
            <StatTile label="Runs ready" value={health.queues.runs_ready} />
            <StatTile
              label="Activity tasks pending"
              value={health.queues.activity_tasks_pending}
            />
            <StatTile label="Inbox pending" value={health.queues.inbox_pending} />
          </section>

          <section className="mt-6">
            <h2 className="text-sm font-semibold text-[var(--c-text-primary)]">
              Workers
            </h2>
            <div className="mt-3 overflow-hidden rounded-md border border-[var(--c-border)]">
              {health.workers.length === 0 ? (
                <div className="px-4 py-8 text-center text-sm text-[var(--c-text-muted)]">
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
                        className={
                          worker.status === 'stale'
                            ? 'bg-[var(--c-red-faint)]'
                            : ''
                        }
                      >
                        <Td mono>{worker.id}</Td>
                        <Td mono>{formatTimestamp(worker.last_claim_at)}</Td>
                        <Td mono>
                          {worker.active_leases} active / {worker.expired_leases} expired
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

          <section className="mt-6">
            <h2 className="text-sm font-semibold text-[var(--c-text-primary)]">
              Retention
            </h2>
            <div className="mt-3 grid gap-3 sm:grid-cols-2">
              <StatTile
                label="Summary-only runs"
                value={health.retention.summary_only_runs}
              />
              <StatTile
                label="Journal-expired runs"
                value={health.retention.journal_expired_runs}
              />
            </div>
          </section>

          <p className="mt-4 text-right font-mono text-[11px] text-[var(--c-text-muted)]">
            Generated {formatTimestamp(health.generated_at)}
          </p>
        </div>
      ) : null}
    </div>
  );
}
