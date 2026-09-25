import type { ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { ArrowRight, Check, Zap } from 'lucide-react';
import type { Trace } from '../../api/client';
import { Chip, DataTable, StatusDot, Td, Th, Tr } from '../../components/DebuggerKit';
import { HonestyIcon, HonestyNote, HonestyRow } from '../../components/DataState';
import { formatDerivedDuration, formatExactTime } from '../../utils/format';
import { appendProjectToPath } from '../../utils/projectSearchParams';
import {
  type CoverageRow,
  type EngineProjectionSummary,
  type RecordedDuration,
} from './overviewData';

const TREND_MIN_POINTS = 5;

export function SectionHeading({
  action,
  id,
  subtitle,
  title,
}: {
  action?: ReactNode;
  id?: string;
  subtitle?: ReactNode;
  title: string;
}) {
  return (
    <div className="mb-2.5 flex items-baseline justify-between gap-4">
      <div className="flex min-w-0 flex-wrap items-baseline gap-x-2.5">
        <h2 id={id} className="text-[13px] font-semibold text-[var(--c-text-primary)]">
          {title}
        </h2>
        {subtitle ? (
          <span className="text-[11.5px] text-[var(--c-text-muted)]">{subtitle}</span>
        ) : null}
      </div>
      {action}
    </div>
  );
}

function CountStat({ label, value }: { label: string; value: number | undefined }) {
  return (
    <span className="inline-flex items-baseline gap-1.5">
      <span className="font-mono text-xs font-semibold tabular-nums text-[var(--c-text-primary)]">
        {value ?? '—'}
      </span>
      <span>{label}</span>
    </span>
  );
}

function EngineProjectionStat({ summary }: { summary: EngineProjectionSummary }) {
  if (summary.kind === 'checking') {
    return <span className="text-[var(--c-text-muted)]">engine projections — checking</span>;
  }
  if (summary.kind === 'up-to-date') {
    return (
      <span className="inline-flex items-center gap-1.5 text-[var(--c-green-text)]">
        <HonestyIcon kind="recorded" className="h-3.5 w-3.5" />
        engine projections — up to date
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1.5 text-[var(--c-amber-text)]">
      <HonestyIcon kind="unverified" className="h-3.5 w-3.5" />
      {summary.kind === 'unavailable'
        ? 'engine projections — unavailable'
        : `engine projections — ${summary.runs} ${summary.runs === 1 ? 'run' : 'runs'} catching up`}
    </span>
  );
}

export type AttentionReason = 'failed' | 'running' | 'errors';

export interface AttentionItem {
  reason: AttentionReason;
  trace: Trace;
}

/** Describes the row from the recorded trace status, not from the query that found it. */
function describeAttention({ trace }: AttentionItem): string {
  const failedSteps = trace.error_count ?? 0;
  const stepsText = `${failedSteps} failed ${failedSteps === 1 ? 'step' : 'steps'}`;
  if (trace.status === 'FAILED') {
    return failedSteps > 0 ? `Failed · ${stepsText}` : 'Failed';
  }
  if (trace.status === 'RUNNING') {
    return failedSteps > 0
      ? `Running · ${stepsText} so far`
      : 'Running · no end time recorded yet';
  }
  return `Completed with ${stepsText}`;
}

export function AttentionSection({
  counts,
  engineProjections,
  items,
  moreLinks,
  projectId,
  queryFailed,
  returnTo,
  state,
}: {
  counts: { failed?: number; running?: number; errors?: number; total?: number };
  engineProjections: EngineProjectionSummary;
  items: AttentionItem[];
  moreLinks: ReactNode;
  projectId?: string;
  queryFailed: boolean;
  returnTo: string;
  state: 'loading' | 'clear' | 'items' | 'unavailable';
}) {
  const statsLine = (
    <div className="mt-3 flex flex-wrap items-center gap-x-4 gap-y-1.5 text-xs text-[var(--c-text-secondary)]">
      <CountStat label="failed" value={counts.failed} />
      <CountStat label="running" value={counts.running} />
      <CountStat
        label={`with failed steps across ${counts.total ?? '—'} traces`}
        value={counts.errors}
      />
      <EngineProjectionStat summary={engineProjections} />
    </div>
  );

  return (
    <section aria-labelledby="overview-attention" className="px-6 pt-5">
      <SectionHeading
        id="overview-attention"
        subtitle="failures and running work come first"
        title="Needs attention"
      />
      {state === 'loading' ? (
        <div className="rounded-md border border-[var(--c-border)] px-4 py-3 text-[13px] text-[var(--c-text-muted)]">
          Checking for failed and running traces…
        </div>
      ) : state === 'unavailable' ? (
        <div className="rounded-md border border-dashed border-[var(--c-border-strong)] px-4 py-3">
          <div className="flex items-center gap-2 text-[13px] font-semibold text-[var(--c-text-primary)]">
            <HonestyIcon kind="unavailable" className="h-3.5 w-3.5" />
            Could not check what needs attention
          </div>
          <p className="mt-1 text-xs leading-5 text-[var(--c-text-secondary)]">
            The failed, running, or failed-step counts did not load. This is not a recorded zero.
          </p>
          {statsLine}
        </div>
      ) : state === 'clear' ? (
        <div className="flex gap-3 rounded-md border border-[var(--c-green-border)] bg-[var(--c-green-faint)] px-4 py-3.5">
          <span
            aria-hidden="true"
            className="mt-0.5 inline-flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-[var(--c-surface)] text-[var(--c-green-text)]"
          >
            <Check className="h-4 w-4" strokeWidth={2.5} />
          </span>
          <div className="min-w-0">
            <div className="text-[14px] font-semibold text-[var(--c-text-primary)]">
              Nothing needs attention in this range
            </div>
            <p className="mt-1 max-w-2xl text-[12.5px] leading-5 text-[var(--c-text-secondary)]">
              No failed traces, no runs currently executing, and no traces with failed steps.
              These are recorded zeros: every trace in range reported a terminal status.
            </p>
            {statsLine}
          </div>
        </div>
      ) : (
        <div className="rounded-md border border-[var(--c-border)] bg-[var(--c-surface)]">
          <ul className="divide-y divide-[var(--c-border-subtle)]">
            {items.map((item) => (
              <li key={`${item.reason}-${item.trace.id}`}>
                <Link
                  to={appendProjectToPath(`/traces/${item.trace.id}`, projectId)}
                  state={{ returnTo }}
                  className="flex items-center gap-3 px-4 py-2.5 text-[13px] hover:bg-[var(--c-row-hover-bg)]"
                >
                  <StatusDot status={item.trace.status} withLabel={false} />
                  <span className="min-w-0 truncate font-mono text-[12.5px] font-medium text-[var(--c-text-primary)]">
                    {item.trace.name}
                  </span>
                  <span
                    className={`min-w-0 flex-1 truncate text-xs ${
                      item.trace.status === 'RUNNING' && (item.trace.error_count ?? 0) === 0
                        ? 'text-[var(--c-text-secondary)]'
                        : 'text-[var(--c-red-text)]'
                    }`}
                  >
                    {describeAttention(item)}
                  </span>
                  <span className="shrink-0 font-mono text-[11.5px] tabular-nums text-[var(--c-text-muted)]">
                    {formatExactTime(item.trace.started_at)}
                  </span>
                </Link>
              </li>
            ))}
          </ul>
          <div className="border-t border-[var(--c-border-subtle)] px-4 pb-3">
            {statsLine}
            {moreLinks}
            {queryFailed ? (
              <HonestyNote className="mt-2" kind="unavailable">
                One attention query did not load, so this list can be incomplete.
              </HonestyNote>
            ) : null}
          </div>
        </div>
      )}
    </section>
  );
}

export interface RecentTraceRowData {
  trace: Trace;
  request:
    | { state: 'loading' }
    | { state: 'unavailable' }
    | { state: 'absent' }
    | { state: 'recorded'; text: string };
  steps: { state: 'loading' } | { state: 'unavailable' } | { state: 'recorded'; count: number };
}

function RequestCell({ request }: { request: RecentTraceRowData['request'] }) {
  if (request.state === 'recorded') {
    return (
      <span className="block truncate text-[12.5px] text-[var(--c-text-secondary)]" title={request.text}>
        {request.text}
      </span>
    );
  }
  const text =
    request.state === 'loading'
      ? 'Loading…'
      : request.state === 'unavailable'
        ? 'Unavailable'
        : 'Not recorded for this trace';
  return <span className="text-[12.5px] text-[var(--c-text-muted)]">{text}</span>;
}

function StepsCell({ steps }: { steps: RecentTraceRowData['steps'] }) {
  if (steps.state === 'recorded') {
    return <>{steps.count}</>;
  }
  return (
    <span
      className="text-[var(--c-text-muted)]"
      title={steps.state === 'unavailable' ? 'Steps did not load for this trace.' : undefined}
    >
      {steps.state === 'loading' ? '…' : '—'}
    </span>
  );
}

export function RecentTracesTable({
  projectId,
  returnTo,
  rows,
}: {
  projectId?: string;
  returnTo: string;
  rows: RecentTraceRowData[];
}) {
  return (
    <div className="overflow-hidden rounded-md border border-[var(--c-border)]">
      <DataTable className="flex-none overflow-x-auto overflow-y-visible">
        <colgroup>
          <col className="w-[34%]" />
          <col />
          <col className="w-[72px]" />
          <col className="w-[96px]" />
          <col className="w-[120px]" />
        </colgroup>
        <thead>
          <tr>
            <Th>Trace</Th>
            <Th>Request</Th>
            <Th align="right">Steps</Th>
            <Th align="right">Duration</Th>
            <Th align="right">Started</Th>
          </tr>
        </thead>
        <tbody>
          {rows.map(({ request, steps, trace }) => (
            <Tr key={trace.id} className="hover:bg-[var(--c-row-hover-bg)]">
              <Td>
                <Link
                  to={appendProjectToPath(`/traces/${trace.id}`, projectId)}
                  state={{ returnTo }}
                  className="flex min-w-0 items-center gap-2 hover:text-[var(--c-accent-text)]"
                >
                  <StatusDot status={trace.status} withLabel={false} />
                  <span className="truncate font-mono text-[12.5px] font-medium text-[var(--c-text-primary)]">
                    {trace.name}
                  </span>
                  {trace.engine ? (
                    <Chip icon={Zap}>{trace.engine.definition_name}</Chip>
                  ) : null}
                </Link>
              </Td>
              <Td>
                <RequestCell request={request} />
              </Td>
              <Td align="right" mono>
                <StepsCell steps={steps} />
              </Td>
              <Td align="right" mono>
                {trace.ended_at ? (
                  <span title="Derived from recorded start and end times.">
                    {formatDerivedDuration(
                      new Date(trace.ended_at).getTime() - new Date(trace.started_at).getTime()
                    )}
                  </span>
                ) : (
                  <span className="font-sans text-[12px] text-[var(--c-text-muted)]">running</span>
                )}
              </Td>
              <Td align="right" dim>
                <span title={trace.started_at}>{formatExactTime(trace.started_at)}</span>
              </Td>
            </Tr>
          ))}
        </tbody>
      </DataTable>
    </div>
  );
}

export function DurationByTrace({
  durations,
  loadedCount,
  rangeLabel,
  totalInRange,
}: {
  durations: RecordedDuration[];
  loadedCount: number;
  rangeLabel: string;
  totalInRange: number;
}) {
  const max = Math.max(...durations.map((item) => item.durationMs), 1);
  const pointsText = `${durations.length} ${durations.length === 1 ? 'point is' : 'points are'}`;
  const unloaded = Math.max(totalInRange - loadedCount, 0);

  return (
    <section className="rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-4 py-3.5">
      <SectionHeading
        subtitle={`${rangeLabel} · ${durations.length} ${
          durations.length === 1 ? 'trace' : 'traces'
        } with recorded start and end times`}
        title="Duration by trace"
      />
      {durations.length === 0 ? (
        <p className="py-3 text-[12.5px] text-[var(--c-text-muted)]">
          No loaded trace has an end time yet, so no duration can be derived.
        </p>
      ) : (
        <ul className="flex flex-col gap-2 py-1" aria-label="Duration by trace">
          {durations.map(({ durationMs, trace }) => (
            <li
              key={trace.id}
              className="grid grid-cols-[minmax(0,160px)_minmax(0,1fr)_64px] items-center gap-3"
            >
              <span className="truncate font-mono text-[11.5px] text-[var(--c-text-primary)]" title={trace.name}>
                {trace.name}
              </span>
              <span className="h-2.5 overflow-hidden rounded-sm bg-[var(--c-surface-muted)]">
                <span
                  className="block h-full rounded-sm"
                  style={{
                    width: `${Math.max((durationMs / max) * 100, 1)}%`,
                    background:
                      trace.status === 'FAILED' ? 'var(--c-bar-failed)' : 'var(--c-bar-success)',
                  }}
                />
              </span>
              <span className="text-right font-mono text-[11.5px] tabular-nums text-[var(--c-text-secondary)]">
                {formatDerivedDuration(durationMs)}
              </span>
            </li>
          ))}
        </ul>
      )}
      <div className="mt-3 border-t border-[var(--c-border-subtle)] pt-2.5">
        <HonestyNote kind="derived">
          {durations.length > 0 && durations.length < TREND_MIN_POINTS
            ? `No trend line: ${pointsText} not a trend. `
            : 'Bars compare single traces; they are not a trend. '}
          Durations are derived from recorded start and end times of the {loadedCount} most
          recent traces.
          {unloaded > 0 ? ` The other ${unloaded} traces in range are not loaded here.` : ''}
        </HonestyNote>
      </div>
    </section>
  );
}

export function CoverageCard({ rows, sampleSize }: { rows: CoverageRow[]; sampleSize: number }) {
  return (
    <section className="rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-4 py-3.5">
      <h2 className="text-[13px] font-semibold text-[var(--c-text-primary)]">Data coverage</h2>
      <p className="mt-0.5 text-[11.5px] text-[var(--c-text-muted)]">
        What this project can and cannot answer right now
        {sampleSize > 0 ? `, checked on the ${sampleSize} most recent traces.` : '.'}
      </p>
      <ul className="mt-2">
        {rows.map((row) => (
          <HonestyRow key={row.key} kind={row.kind} label={row.label} value={row.value} />
        ))}
      </ul>
      <p className="mt-2 border-t border-[var(--c-border-subtle)] pt-2 text-[11.5px] leading-5 text-[var(--c-text-muted)]">
        Values marked unverified or not captured are not shown as numbers on these pages.
      </p>
    </section>
  );
}

export function ViewAllLink({ to, children = 'View all' }: { to: string; children?: ReactNode }) {
  return (
    <Link
      to={to}
      className="inline-flex shrink-0 items-center gap-1 text-xs font-medium text-[var(--c-accent-text)] hover:underline"
    >
      {children} <ArrowRight className="h-3 w-3" />
    </Link>
  );
}
