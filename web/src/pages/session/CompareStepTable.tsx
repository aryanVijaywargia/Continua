import { Fragment, useState } from 'react';
import { Link } from 'react-router-dom';
import { ChevronDown, ChevronRight } from 'lucide-react';
import type {
  CompareSemanticSummary,
  CompareSpanSummary,
  SessionCompareResponse,
  SpanDiffRow,
} from '../../api/client';
import { HonestyNote, NotAvailableCard } from '../../components/DataState';
import { Chip, StatusDot } from '../../components/DebuggerKit';
import {
  formatCost,
  formatDerivedDuration,
  formatExactTime,
  formatTokens,
} from '../../utils/format';
import { appendProjectToPath } from '../../utils/projectSearchParams';
import {
  formatClockTime,
  formatSignedMs,
  formatSignedPercent,
  isUsageUnverified,
} from './sessionDisplay';

type StepFilter = 'changed' | 'all';

interface IndexedRow {
  key: string;
  row: SpanDiffRow;
}

/** Latency as recorded, else end minus start when both timestamps exist. */
function spanDurationMs(span: CompareSpanSummary | null): number | undefined {
  if (!span) {
    return undefined;
  }
  if (typeof span.latency_ms === 'number') {
    return span.latency_ms;
  }
  if (!span.ended_at) {
    return undefined;
  }
  const duration = new Date(span.ended_at).getTime() - new Date(span.started_at).getTime();
  return Number.isFinite(duration) && duration >= 0 ? duration : undefined;
}

function stepName(row: SpanDiffRow): string {
  return row.candidate_span?.name ?? row.baseline_span?.name ?? 'Unnamed step';
}

/**
 * Step-by-step comparison table. Changed rows show by default; unchanged
 * rows collapse into one line. Every row keeps a word label, not only colour.
 */
export function CompareStepTable({
  comparison,
  currentCompareUrl,
  projectId,
}: {
  comparison: SessionCompareResponse;
  currentCompareUrl: string;
  projectId?: string;
}) {
  const rows: IndexedRow[] = comparison.span_diffs.map((row, index) => ({
    key: `${row.baseline_span?.id ?? 'none'}:${row.candidate_span?.id ?? 'none'}:${index}`,
    row,
  }));
  const changedRows = rows.filter(({ row }) => row.diff_status !== 'unchanged');
  const unchangedRows = rows.filter(({ row }) => row.diff_status === 'unchanged');
  const [filter, setFilter] = useState<StepFilter>('changed');
  const [expandedRows, setExpandedRows] = useState<Record<string, boolean>>({});
  const firstChangeKey = changedRows[0]?.key;
  const visibleRows = filter === 'changed' ? changedRows : rows;
  const heuristicRows = rows.filter(({ row }) => row.match_source === 'heuristic').length;
  const matchedRows = rows.filter(
    ({ row }) => row.baseline_span !== null && row.candidate_span !== null
  ).length;

  if (rows.length === 0) {
    return (
      <div className="app-empty-state">
        No span rows were returned for this comparison. Both traces may be empty.
      </div>
    );
  }

  return (
    <div>
      <div className="flex flex-wrap items-center gap-3 pb-3">
        <div className="flex gap-0.5 rounded-md border border-[var(--c-border)] bg-[var(--c-surface-muted)] p-0.5">
          <FilterButton active={filter === 'changed'} onClick={() => setFilter('changed')}>
            Changed only · {changedRows.length}
          </FilterButton>
          <FilterButton active={filter === 'all'} onClick={() => setFilter('all')}>
            Show all {rows.length}
          </FilterButton>
        </div>
        <span className="text-[11.5px] text-[var(--c-text-muted)]">
          aligned by span ID
          {heuristicRows > 0 ? ` · ${heuristicRows} by name + kind` : ''} · {matchedRows} of{' '}
          {rows.length} matched
        </span>
      </div>

      <div className="overflow-x-auto rounded-md border border-[var(--c-border)]">
        <table className="w-full min-w-[560px] border-separate border-spacing-0 text-[13px]">
          <colgroup>
            <col />
            <col className="w-[16%]" />
            <col className="w-[16%]" />
            <col className="w-[20%]" />
          </colgroup>
          <thead>
            <tr>
              <HeadCell>Step</HeadCell>
              <HeadCell align="right">Baseline</HeadCell>
              <HeadCell align="right">Candidate</HeadCell>
              <HeadCell align="right">Delta</HeadCell>
            </tr>
          </thead>
          <tbody>
            {visibleRows.length === 0 ? (
              <tr>
                <td
                  colSpan={4}
                  className="px-4 py-4 text-center text-[12.5px] text-[var(--c-text-muted)]"
                >
                  No step changed between these traces.
                </td>
              </tr>
            ) : null}
            {visibleRows.map(({ key, row }, index) => {
              const previous = visibleRows[index - 1]?.row;
              const showCandidateOnlyDivider =
                row.diff_status === 'candidate_only' &&
                row.depth === 0 &&
                (!previous || previous.diff_status !== 'candidate_only' || previous.depth !== 0);
              const expanded = expandedRows[key] ?? false;

              return (
                <Fragment key={key}>
                  {showCandidateOnlyDivider ? (
                    <tr>
                      <td
                        colSpan={4}
                        className="border-b border-[var(--c-border)] bg-[var(--c-accent-faint)] px-4 py-1.5 text-[11.5px] font-semibold text-[var(--c-accent-text)]"
                      >
                        Candidate-only branches
                      </td>
                    </tr>
                  ) : null}
                  <StepRow
                    expanded={expanded}
                    isFirstChange={key === firstChangeKey}
                    onToggle={() =>
                      setExpandedRows((current) => ({ ...current, [key]: !current[key] }))
                    }
                    row={row}
                  />
                  {expanded ? (
                    <tr>
                      <td
                        colSpan={4}
                        className="border-b border-[var(--c-border)] bg-[var(--c-surface-muted)] px-4 py-3"
                      >
                        <StepDetails
                          comparison={comparison}
                          currentCompareUrl={currentCompareUrl}
                          projectId={projectId}
                          row={row}
                        />
                      </td>
                    </tr>
                  ) : null}
                </Fragment>
              );
            })}
          </tbody>
        </table>
      </div>

      {filter === 'changed' && unchangedRows.length > 0 ? (
        <p className="mt-3 text-[12.5px] text-[var(--c-text-secondary)]">
          {unchangedRows.length} {unchangedRows.length === 1 ? 'step' : 'steps'} unchanged — same
          status, timing, and usage —{' '}
          <span className="font-mono text-[var(--c-text-primary)]">
            {unchangedRows
              .slice(0, 5)
              .map(({ row }) => stepName(row))
              .join(', ')}
          </span>
          {unchangedRows.length > 5 ? ` and ${unchangedRows.length - 5} more` : ''}.{' '}
          <button
            type="button"
            onClick={() => setFilter('all')}
            className="font-medium text-[var(--c-accent-text)] hover:underline"
          >
            Show them
          </button>
        </p>
      ) : null}

      <HonestyNote className="mt-3" kind="recorded">
        Spans match by span ID first, then by name, kind, and order. A step counts as unchanged
        only when its status, latency, tokens, cost, and event count are identical.
      </HonestyNote>
    </div>
  );
}

function FilterButton({
  active,
  children,
  onClick,
}: {
  active: boolean;
  children: React.ReactNode;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={onClick}
      className={`rounded px-2.5 py-1 text-[11.5px] font-medium ${
        active
          ? 'border border-[var(--c-border)] bg-[var(--c-surface)] text-[var(--c-text-primary)]'
          : 'border border-transparent text-[var(--c-text-secondary)] hover:text-[var(--c-text-primary)]'
      }`}
    >
      {children}
    </button>
  );
}

function HeadCell({
  align = 'left',
  children,
}: {
  align?: 'left' | 'right';
  children: React.ReactNode;
}) {
  return (
    <th
      className={`border-b border-[var(--c-border)] bg-[var(--c-table-head-bg)] px-4 py-1.5 text-[11px] font-semibold uppercase tracking-[0.04em] text-[var(--c-text-muted)] ${
        align === 'right' ? 'text-right' : 'text-left'
      }`}
    >
      {children}
    </th>
  );
}

const DIFF_LABELS: Record<
  SpanDiffRow['diff_status'],
  { label: string; tone: 'amber' | 'accent' | 'error' | 'muted' }
> = {
  changed: { label: 'changed', tone: 'amber' },
  candidate_only: { label: 'added', tone: 'accent' },
  baseline_only: { label: 'removed', tone: 'error' },
  unchanged: { label: 'unchanged', tone: 'muted' },
};

function StepRow({
  expanded,
  isFirstChange,
  onToggle,
  row,
}: {
  expanded: boolean;
  isFirstChange: boolean;
  onToggle: () => void;
  row: SpanDiffRow;
}) {
  const name = stepName(row);
  const baselineMs = spanDurationMs(row.baseline_span);
  const candidateMs = spanDurationMs(row.candidate_span);
  const diffLabel = DIFF_LABELS[row.diff_status];
  const Chevron = expanded ? ChevronDown : ChevronRight;

  return (
    <tr className="hover:bg-[var(--c-row-hover-bg)]">
      <td className="border-b border-[var(--c-border-subtle)] px-4 py-2 align-middle">
        <div
          className="flex min-w-0 flex-wrap items-center gap-2"
          style={{ paddingLeft: `${row.depth * 14}px` }}
        >
          <button
            type="button"
            aria-expanded={expanded}
            aria-label={`${expanded ? 'Hide' : 'Show'} details for ${name}`}
            onClick={onToggle}
            className="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded text-[var(--c-text-muted)] hover:bg-[var(--c-surface-muted)] hover:text-[var(--c-text-primary)]"
          >
            <Chevron className="h-3.5 w-3.5" />
          </button>
          <span className="min-w-0 truncate font-mono text-[12.5px] font-medium text-[var(--c-text-primary)]">
            {name}
          </span>
          {isFirstChange ? (
            <span className="rounded border border-[var(--c-text-primary)] px-1 text-[10px] font-semibold uppercase tracking-[0.05em] text-[var(--c-text-primary)]">
              first change
            </span>
          ) : null}
          <Chip tone={diffLabel.tone} className="uppercase tracking-[0.04em]">
            {diffLabel.label}
          </Chip>
        </div>
      </td>
      <td className="border-b border-[var(--c-border-subtle)] px-4 py-2 text-right font-mono tabular-nums text-[var(--c-text-secondary)]">
        {row.baseline_span ? formatDerivedDuration(baselineMs) : '—'}
      </td>
      <td className="border-b border-[var(--c-border-subtle)] px-4 py-2 text-right font-mono tabular-nums text-[var(--c-text-primary)]">
        {row.candidate_span ? formatDerivedDuration(candidateMs) : '—'}
      </td>
      <td className="border-b border-[var(--c-border-subtle)] px-4 py-2 text-right font-mono tabular-nums">
        <DeltaValue
          baselineMs={row.baseline_span ? baselineMs : undefined}
          candidateMs={row.candidate_span ? candidateMs : undefined}
          diffStatus={row.diff_status}
        />
      </td>
    </tr>
  );
}

function DeltaValue({
  baselineMs,
  candidateMs,
  diffStatus,
}: {
  baselineMs?: number;
  candidateMs?: number;
  diffStatus: SpanDiffRow['diff_status'];
}) {
  if (diffStatus === 'candidate_only') {
    return <span className="text-[var(--c-text-muted)]">only in candidate</span>;
  }
  if (diffStatus === 'baseline_only') {
    return <span className="text-[var(--c-text-muted)]">only in baseline</span>;
  }
  if (baselineMs == null || candidateMs == null) {
    return <span className="text-[var(--c-text-muted)]">no timing</span>;
  }
  const delta = candidateMs - baselineMs;
  const percent = formatSignedPercent(delta, baselineMs);
  const toneClass =
    delta > 0
      ? 'text-[var(--c-red-text)]'
      : delta < 0
        ? 'text-[var(--c-green-text)]'
        : 'text-[var(--c-text-muted)]';
  return (
    <span
      className={toneClass}
      title={delta > 0 ? 'Candidate is slower' : delta < 0 ? 'Candidate is faster' : 'Same timing'}
    >
      {formatSignedMs(delta)}
      {percent && delta !== 0 ? ` · ${percent}` : ''}
    </span>
  );
}

function StepDetails({
  comparison,
  currentCompareUrl,
  projectId,
  row,
}: {
  comparison: SessionCompareResponse;
  currentCompareUrl: string;
  projectId?: string;
  row: SpanDiffRow;
}) {
  const changedFields = row.changed_fields ?? [];
  const semanticChanged = row.semantic_groups.some((group) => group.diff_status !== 'unchanged');

  return (
    <div className="flex flex-col gap-3">
      <div className="grid gap-3 md:grid-cols-2">
        <SpanTimingCard
          currentCompareUrl={currentCompareUrl}
          label="Baseline"
          projectId={projectId}
          span={row.baseline_span}
          traceId={comparison.baseline.id}
        />
        <SpanTimingCard
          currentCompareUrl={currentCompareUrl}
          label="Candidate"
          projectId={projectId}
          span={row.candidate_span}
          traceId={comparison.candidate.id}
        />
      </div>

      {row.match_source || changedFields.length > 0 ? (
        <div className="flex flex-wrap items-center gap-1.5 text-[11.5px] text-[var(--c-text-muted)]">
          {row.match_source ? (
            <MatchSourcePill matchReason={row.match_reason} matchSource={row.match_source} />
          ) : null}
          {changedFields.length > 0 ? <span>changed fields</span> : null}
          {changedFields.map((field) => (
            <Chip key={field} tone="muted" className="font-mono">
              {field}
            </Chip>
          ))}
        </div>
      ) : null}

      {row.semantic_groups.length > 0 ? (
        <div className="flex flex-col gap-2">
          {row.semantic_groups.map((group, index) => (
            <SemanticGroup
              group={group}
              key={`${group.event_type}:${group.baseline_event?.id ?? 'none'}:${group.candidate_event?.id ?? 'none'}:${index}`}
            />
          ))}
        </div>
      ) : null}

      {!semanticChanged ? (
        <NotAvailableCard kind="unverified" title="No payload difference established">
          {row.semantic_groups.length === 0
            ? 'No semantic events were recorded for this step, so the comparison covers timing, status, and usage only.'
            : 'The recorded events for this step match. Any difference here is in timing, status, or usage, not in event payloads.'}
        </NotAvailableCard>
      ) : null}
    </div>
  );
}

function SpanTimingCard({
  currentCompareUrl,
  label,
  projectId,
  span,
  traceId,
}: {
  currentCompareUrl: string;
  label: 'Baseline' | 'Candidate';
  projectId?: string;
  span: CompareSpanSummary | null;
  traceId: string;
}) {
  if (!span) {
    return (
      <div className="rounded-md border border-dashed border-[var(--c-border-strong)] px-3 py-2.5 text-[12px] text-[var(--c-text-muted)]">
        {label}: no matching span
      </div>
    );
  }

  const usageVerified = !isUsageUnverified([span.tokens_in, span.tokens_out, span.cost_usd]);

  return (
    <div className="rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-3 py-2.5">
      <div className="flex items-center justify-between gap-2">
        <span className="text-[10.5px] font-semibold uppercase tracking-[0.06em] text-[var(--c-text-muted)]">
          {label} timing
        </span>
        <StatusDot status={span.status} />
      </div>
      <p className="mt-1 font-mono text-[12px] tabular-nums text-[var(--c-text-primary)]">
        {formatClockTime(span.started_at)} → {formatClockTime(span.ended_at)}
      </p>
      <dl className="mt-1.5 grid grid-cols-[72px_minmax(0,1fr)] gap-x-2 gap-y-0.5 text-[11.5px]">
        <dt className="text-[var(--c-text-muted)]">Span ID</dt>
        <dd className="truncate font-mono text-[var(--c-text-secondary)]">{span.span_id}</dd>
        <dt className="text-[var(--c-text-muted)]">Kind</dt>
        <dd className="font-mono text-[var(--c-text-secondary)]">{span.kind}</dd>
        {span.model ? (
          <>
            <dt className="text-[var(--c-text-muted)]">Model</dt>
            <dd className="truncate font-mono text-[var(--c-text-secondary)]">{span.model}</dd>
          </>
        ) : null}
        <dt className="text-[var(--c-text-muted)]">Usage</dt>
        <dd className="font-mono text-[var(--c-text-secondary)]">
          {usageVerified ? (
            `${formatTokens(span.tokens_in)} in · ${formatTokens(span.tokens_out)} out · ${formatCost(span.cost_usd)}`
          ) : (
            <span className="font-sans text-[var(--c-amber-text)]">not verified</span>
          )}
        </dd>
      </dl>
      {span.error_message ? (
        <p className="mt-1.5 rounded border border-[var(--c-red-border)] bg-[var(--c-red-faint)] px-2 py-1 text-[11.5px] text-[var(--c-red-text)]">
          {span.error_message}
        </p>
      ) : null}
      <Link
        to={appendProjectToPath(
          `/traces/${traceId}?span=${encodeURIComponent(span.span_id)}`,
          projectId
        )}
        state={{ returnTo: currentCompareUrl }}
        className="mt-2 inline-block text-[12px] font-medium text-[var(--c-accent-text)] hover:underline"
      >
        Open {label.toLowerCase()} span
      </Link>
    </div>
  );
}

function SemanticGroup({ group }: { group: SpanDiffRow['semantic_groups'][number] }) {
  const diffLabel = DIFF_LABELS[group.diff_status];
  return (
    <div className="rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-3 py-2.5">
      <div className="flex flex-wrap items-center gap-1.5">
        <Chip tone="muted" className="font-mono">
          {group.event_type}
        </Chip>
        <Chip tone={diffLabel.tone} className="uppercase tracking-[0.04em]">
          {diffLabel.label}
        </Chip>
        {group.match_source ? (
          <MatchSourcePill matchReason={group.match_reason} matchSource={group.match_source} />
        ) : null}
        {(group.changed_fields ?? []).map((field) => (
          <Chip key={field} tone="amber" className="font-mono">
            {field}
          </Chip>
        ))}
      </div>
      <div className="mt-2 grid gap-2 md:grid-cols-2">
        <SemanticSide event={group.baseline_event} label="Baseline" />
        <SemanticSide event={group.candidate_event} label="Candidate" />
      </div>
    </div>
  );
}

function SemanticSide({
  event,
  label,
}: {
  event: CompareSemanticSummary | null;
  label: string;
}) {
  if (!event) {
    return (
      <div className="rounded border border-dashed border-[var(--c-border-strong)] px-2.5 py-2 text-[12px] text-[var(--c-text-muted)]">
        {label}: no matching event
      </div>
    );
  }

  return (
    <div className="min-w-0 rounded border border-[var(--c-border)] bg-[var(--c-app-bg)] px-2.5 py-2">
      <div className="flex items-center justify-between gap-2 text-[10.5px] text-[var(--c-text-muted)]">
        <span className="font-semibold uppercase tracking-[0.06em]">{label}</span>
        <span title={event.timestamp}>{formatExactTime(event.timestamp)}</span>
      </div>
      <p className="mt-1 text-[12.5px] font-medium text-[var(--c-text-primary)]">
        {event.message ?? '(no message)'}
      </p>
      {event.payload ? (
        <pre className="mt-1.5 max-h-48 overflow-auto rounded bg-[var(--c-surface-muted)] px-2 py-1.5 font-mono text-[11px] text-[var(--c-text-secondary)]">
          {JSON.stringify(event.payload, null, 2)}
        </pre>
      ) : null}
    </div>
  );
}

function MatchSourcePill({
  matchReason,
  matchSource,
}: {
  matchReason?: string;
  matchSource: NonNullable<SpanDiffRow['match_source']>;
}) {
  return (
    <span
      title={matchReason ?? undefined}
      className="inline-flex h-5 items-center rounded-full border border-[var(--c-border)] bg-[var(--c-surface)] px-2 text-[11px] font-medium text-[var(--c-text-secondary)]"
    >
      {matchSource === 'stable_id' ? 'Stable ID' : 'Heuristic'}
    </span>
  );
}
