import { type ReactNode } from 'react';
import {
  type EnginePendingWorkResponse,
} from '../api/client';
import { formatRelativeTime, formatTimestamp } from '../utils/format';
import { describeEngineWaitState } from './engineWaitState';

interface EnginePendingWorkPanelProps {
  data: EnginePendingWorkResponse | undefined;
  isError: boolean;
  isLoading: boolean;
  errorMessage?: string | null;
}

export function EnginePendingWorkPanel({
  data,
  isError,
  isLoading,
  errorMessage,
}: EnginePendingWorkPanelProps) {
  const waitSummary = describeEngineWaitState(data?.current_wait);

  return (
    <section className="rounded-[1rem] border border-[var(--continua-border-strong)] bg-[var(--continua-surface)] p-4 shadow-[var(--continua-shadow-soft)]">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-[11px] font-semibold uppercase tracking-[0.18em] text-[var(--continua-text-muted)]">
            Pending work
          </div>
          <h2 className="mt-2 text-base font-bold text-[var(--continua-text-primary)]">
            Engine wait state and queued work
          </h2>
          <p className="mt-1 text-sm text-[var(--continua-text-secondary)]">
            Inspect outstanding activities, timers, and signals without relying
            on the projected timeline alone.
          </p>
        </div>

        {data ? (
          <div className="flex flex-wrap gap-2 text-xs text-[var(--continua-text-secondary)]">
            <PendingWorkCountPill
              label="Activities"
              value={data.pending_activity_tasks}
            />
            <PendingWorkCountPill
              label="Inbox"
              value={data.pending_inbox_items}
            />
          </div>
        ) : null}
      </div>

      {isError ? (
        <div className="app-alert-error mt-4">
          Pending work is temporarily unavailable.
          {errorMessage ? ` ${errorMessage}` : ''}
        </div>
      ) : (
        <>
          <div className="mt-4 rounded-[1rem] border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] p-4">
            <div className="text-[11px] font-semibold uppercase tracking-[0.18em] text-[var(--continua-text-muted)]">
              Current wait
            </div>
            {isLoading && !data ? (
              <p className="mt-2 text-sm text-[var(--continua-text-secondary)]">
                Loading pending work...
              </p>
            ) : waitSummary ? (
              <>
                <h3 className="mt-2 text-sm font-semibold text-[var(--continua-text-primary)]">
                  {waitSummary.heading}
                </h3>
                <p className="mt-1 text-sm text-[var(--continua-text-secondary)]">
                  {waitSummary.detail}
                </p>
              </>
            ) : (
              <p className="mt-2 text-sm text-[var(--continua-text-secondary)]">
                No active wait reported.
              </p>
            )}
          </div>

          <div className="mt-4 grid gap-4 xl:grid-cols-3">
            <PendingWorkList
              title="Activities"
              emptyMessage="No pending activities."
              items={data?.activities ?? []}
              getKey={(item) => item.task_id}
              renderItem={(item, key) => (
                <PendingWorkCard
                  key={key}
                  title={`${item.activity_type} · ${item.activity_key}`}
                  metadata={[
                    `Status: ${item.status}`,
                    `Available ${formatPendingAvailability(item.available_at)}`,
                    `Attempts: ${item.attempt_count}`,
                  ]}
                  keyValue={`Task ${item.task_id}`}
                />
              )}
            />
            <PendingWorkList
              title="Timers"
              emptyMessage="No pending timers."
              items={data?.timers ?? []}
              getKey={(item) => item.inbox_id}
              renderItem={(item, key) => (
                <PendingWorkCard
                  key={key}
                  title={item.timer_key}
                  metadata={[
                    `Status: ${item.status}`,
                    `Available ${formatPendingAvailability(item.available_at)}`,
                  ]}
                  keyValue={`Inbox ${item.inbox_id}`}
                />
              )}
            />
            <PendingWorkList
              title="Signals"
              emptyMessage="No pending signals."
              items={data?.signals ?? []}
              getKey={(item) => item.inbox_id}
              renderItem={(item, key) => (
                <PendingWorkCard
                  key={key}
                  title={item.signal_name}
                  metadata={[
                    `Status: ${item.status}`,
                    `Available ${formatPendingAvailability(item.available_at)}`,
                  ]}
                  keyValue={`Inbox ${item.inbox_id}`}
                />
              )}
            />
          </div>
        </>
      )}
    </section>
  );
}

function PendingWorkCountPill({
  label,
  value,
}: {
  label: string;
  value: number;
}) {
  return (
    <span className="rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-2.5 py-1">
      {label}: {value}
    </span>
  );
}

function PendingWorkList<Item>({
  title,
  emptyMessage,
  items,
  getKey,
  renderItem,
}: {
  title: string;
  emptyMessage: string;
  items: Item[];
  getKey: (item: Item) => string;
  renderItem: (item: Item, key: string) => ReactNode;
}) {
  return (
    <div className="rounded-[1rem] border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] p-4">
      <div className="text-[11px] font-semibold uppercase tracking-[0.18em] text-[var(--continua-text-muted)]">
        {title}
      </div>
      {items.length === 0 ? (
        <p className="mt-3 text-sm text-[var(--continua-text-secondary)]">
          {emptyMessage}
        </p>
      ) : (
        <div className="mt-3 space-y-3">
          {items.map((item) => renderItem(item, getKey(item)))}
        </div>
      )}
    </div>
  );
}

function PendingWorkCard({
  title,
  metadata,
  keyValue,
}: {
  title: string;
  metadata: string[];
  keyValue: string;
}) {
  return (
    <article className="rounded-[1rem] border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] p-3">
      <h3 className="text-sm font-semibold text-[var(--continua-text-primary)]">
        {title}
      </h3>
      <p className="mt-1 font-mono text-xs text-[var(--continua-text-muted)]">
        {keyValue}
      </p>
      <ul className="mt-3 space-y-1 text-sm text-[var(--continua-text-secondary)]">
        {metadata.map((entry) => (
          <li key={entry}>{entry}</li>
        ))}
      </ul>
    </article>
  );
}

function formatPendingAvailability(value: string): string {
  return `${formatTimestamp(value)} (${formatRelativeTime(value)})`;
}
