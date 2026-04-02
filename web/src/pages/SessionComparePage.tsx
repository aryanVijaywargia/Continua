import { useQuery } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { Link, useLocation, useParams, useSearchParams } from 'react-router-dom';
import { AuthErrorBanner } from '../components/AuthErrorBanner';
import { StatusBadge } from '../components/StatusBadge';
import {
  ApiError,
  type ComparisonTooLargeErrorDetail,
  fetchSessionComparison,
  isAuthError,
  isComparisonTooLargeError,
  type CompareSemanticSummary,
  type CompareSpanSummary,
  type SessionCompareResponse,
  type SpanDiffRow,
} from '../api/client';
import { useRequireApiKey } from '../hooks/useRequireApiKey';
import { formatCost, formatDuration, formatRelativeTime, formatTokens } from '../utils/format';
import {
  buildCompareSearchParams,
  getCompareReturnToDestination,
  normalizeCompareTraceIdParam,
} from './sessionCompareUtils';

function queryErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Unknown error';
}

export function SessionComparePage() {
  const { id } = useParams<{ id: string }>();
  const { hasApiKey, prompt } = useRequireApiKey();

  if (!hasApiKey) {
    return prompt;
  }

  if (!id) {
    return (
      <div className="flex min-h-full items-center justify-center">
        <div className="text-red-600 dark:text-red-300">Session ID is required</div>
      </div>
    );
  }

  return <SessionCompareContent sessionId={id} />;
}

function SessionCompareContent({ sessionId }: { sessionId: string }) {
  const location = useLocation();
  const [searchParams, setSearchParams] = useSearchParams();
  const [expandedRows, setExpandedRows] = useState<Record<string, boolean>>({});

  const baselineTraceId = normalizeCompareTraceIdParam(searchParams.get('baseline_trace_id'));
  const candidateTraceIdRaw = normalizeCompareTraceIdParam(searchParams.get('candidate_trace_id'));
  const candidateTraceId =
    candidateTraceIdRaw && candidateTraceIdRaw !== baselineTraceId ? candidateTraceIdRaw : undefined;
  const canonicalSearch = buildCompareSearchParams(baselineTraceId, candidateTraceId).toString();
  const currentCompareUrl = `${location.pathname}${canonicalSearch ? `?${canonicalSearch}` : ''}`;
  const returnTo = getCompareReturnToDestination(location.state, sessionId, searchParams);

  useEffect(() => {
    if (searchParams.toString() === canonicalSearch) {
      return;
    }

    setSearchParams(new URLSearchParams(canonicalSearch), { replace: true });
  }, [canonicalSearch, searchParams, setSearchParams]);

  const comparisonQuery = useQuery({
    queryKey: ['session-compare', sessionId, baselineTraceId, candidateTraceId],
    queryFn: () => fetchSessionComparison(sessionId, baselineTraceId!, candidateTraceId!),
    enabled: Boolean(baselineTraceId && candidateTraceId),
  });

  if (!baselineTraceId || !candidateTraceId) {
    return (
      <ComparePageShell returnTo={returnTo}>
        <section className="rounded-[1.5rem] border border-amber-300/40 bg-amber-50/80 p-6 text-amber-900 dark:border-amber-400/20 dark:bg-amber-400/10 dark:text-amber-100">
          <h1 className="text-lg font-semibold">Comparison needs two traces</h1>
          <p className="mt-2 text-sm">
            Open this page from a session with both a baseline and candidate trace selected.
          </p>
        </section>
      </ComparePageShell>
    );
  }

  if (comparisonQuery.isLoading) {
    return (
      <ComparePageShell returnTo={returnTo}>
        <div className="app-empty-state">
          Loading comparison...
        </div>
      </ComparePageShell>
    );
  }

  if (comparisonQuery.error) {
    return (
      <ComparePageShell returnTo={returnTo}>
        {isAuthError(comparisonQuery.error) ? (
          <AuthErrorBanner message={queryErrorMessage(comparisonQuery.error)} />
        ) : isComparisonTooLargeError(comparisonQuery.error) ? (
          <ComparisonTooLargePanel error={comparisonQuery.error} />
        ) : (
          <div className="app-alert-error">
            Error loading comparison: {queryErrorMessage(comparisonQuery.error)}
          </div>
        )}
      </ComparePageShell>
    );
  }

  if (!comparisonQuery.data) {
    return (
      <ComparePageShell returnTo={returnTo}>
        <div className="app-empty-state">
          Comparison not found.
        </div>
      </ComparePageShell>
    );
  }

  return (
    <ComparePageShell returnTo={returnTo}>
      <CompareOverview comparison={comparisonQuery.data} currentCompareUrl={currentCompareUrl} />

      <section className="app-surface p-6">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <div className="app-overline">Diff workspace</div>
            <h2 className="mt-2 text-2xl font-semibold tracking-[-0.03em] text-[var(--continua-text-primary)]">Span Diff</h2>
            <p className="mt-1 text-sm text-[var(--continua-text-secondary)]">
              Ordered baseline-first diff with inline semantic event comparison.
            </p>
          </div>
          <div className="text-sm text-[var(--continua-text-muted)]">
            {comparisonQuery.data.span_diffs.length} rows
          </div>
        </div>

        {comparisonQuery.data.span_diffs.length === 0 ? (
          <div className="app-empty-state mt-6">
            No span rows were returned for this comparison. Both traces may be empty.
          </div>
        ) : (
          <div className="mt-6 space-y-3">
            {comparisonQuery.data.span_diffs.map((row, index) => {
              const rowKey = `${row.baseline_span?.id ?? 'none'}:${row.candidate_span?.id ?? 'none'}:${index}`;
              const hasSemanticContent = row.semantic_groups.length > 0;
              const isExpanded = expandedRows[rowKey] ?? false;
              const showCandidateOnlyDivider =
                row.diff_status === 'candidate_only' &&
                row.depth === 0 &&
                (index === 0 ||
                  comparisonQuery.data.span_diffs[index - 1].diff_status !== 'candidate_only' ||
                  comparisonQuery.data.span_diffs[index - 1].depth !== 0);

              return (
                <div key={rowKey}>
                  {showCandidateOnlyDivider ? (
                    <div className="mb-3 rounded-xl border border-[var(--continua-accent)] bg-[var(--continua-accent-faint)] px-3 py-2 text-sm font-medium text-[var(--continua-accent)]">
                      Candidate-only branches
                    </div>
                  ) : null}
                  <CompareSpanRow
                    currentCompareUrl={currentCompareUrl}
                    expanded={isExpanded}
                    hasSemanticContent={hasSemanticContent}
                    onToggleExpanded={() =>
                      setExpandedRows((current) => ({
                        ...current,
                        [rowKey]: !current[rowKey],
                      }))
                    }
                    row={row}
                    traces={comparisonQuery.data}
                  />
                </div>
              );
            })}
          </div>
        )}
      </section>
    </ComparePageShell>
  );
}

function ComparePageShell({
  returnTo,
  children,
}: {
  returnTo: string;
  children: React.ReactNode;
}) {
  return (
    <div className="app-page">
      <Link
        to={returnTo}
        className="inline-flex text-sm font-medium text-[var(--continua-accent)] transition hover:opacity-80"
      >
        &larr; Back to Session
      </Link>
      <div className="space-y-6">{children}</div>
    </div>
  );
}

function CompareOverview({
  comparison,
  currentCompareUrl,
}: {
  comparison: SessionCompareResponse;
  currentCompareUrl: string;
}) {
  return (
    <section className="app-surface sticky top-[4.9rem] z-10 p-6">
      <div className="flex flex-col gap-6 xl:flex-row xl:items-start xl:justify-between">
        <div>
          <p className="text-xs font-medium uppercase tracking-[0.16em] text-[var(--continua-text-muted)]">
            Session Compare
          </p>
          <h1 className="mt-2 text-3xl font-semibold tracking-[-0.04em] text-[var(--continua-text-primary)]">
            {comparison.session.external_id}
          </h1>
          {comparison.session.name ? (
            <p className="mt-2 text-sm text-[var(--continua-text-secondary)]">{comparison.session.name}</p>
          ) : null}
          <p className="mt-2 font-mono text-xs text-[var(--continua-text-muted)]">
            {comparison.session.id}
          </p>
        </div>

        <div className="grid gap-4 sm:grid-cols-2 xl:w-[32rem]">
          <CompareTraceCard
            currentCompareUrl={currentCompareUrl}
            label="Baseline"
            trace={comparison.baseline}
          />
          <CompareTraceCard
            currentCompareUrl={currentCompareUrl}
            label="Candidate"
            trace={comparison.candidate}
          />
        </div>
      </div>

      <dl className="mt-6 grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <OverviewMetric
          label="Match Counts"
          value={`${comparison.summary.matched_spans} matched`}
          hint={`${comparison.summary.unmatched_baseline_spans} baseline-only · ${comparison.summary.unmatched_candidate_spans} candidate-only`}
        />
        <OverviewMetric
          label="Heuristic Matches"
          value={String(comparison.summary.heuristic_matches)}
          hint="Span-level heuristic matches only"
        />
        <OverviewMetric
          label="Duration Delta"
          value={formatSignedDuration(comparison.summary.duration_delta_ms)}
        />
        <OverviewMetric
          label="Tokens Delta"
          value={`${formatSignedNumber(comparison.summary.tokens_in_delta)} in`}
          hint={`${formatSignedNumber(comparison.summary.tokens_out_delta)} out`}
        />
        <OverviewMetric
          label="Cost Delta"
          value={formatSignedCost(comparison.summary.cost_delta_usd)}
        />
        <OverviewMetric
          label="Span Counts"
          value={`${comparison.summary.total_spans_baseline} baseline`}
          hint={`${comparison.summary.total_spans_candidate} candidate`}
        />
        <OverviewMetric
          label="Semantic Events"
          value={`${comparison.summary.total_semantic_baseline} baseline`}
          hint={`${comparison.summary.total_semantic_candidate} candidate`}
        />
      </dl>
    </section>
  );
}

function CompareTraceCard({
  label,
  trace,
  currentCompareUrl,
}: {
  label: string;
  trace: SessionCompareResponse['baseline'];
  currentCompareUrl: string;
}) {
  return (
    <article className="app-surface-muted p-4">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-xs font-medium uppercase tracking-[0.16em] text-[var(--continua-text-muted)]">
            {label}
          </p>
          <Link
            to={`/traces/${trace.id}`}
            state={{ returnTo: currentCompareUrl }}
            className="mt-2 inline-block text-sm font-semibold text-[var(--continua-accent)] hover:opacity-80"
          >
            {trace.name}
          </Link>
          <p className="mt-2 font-mono text-xs text-[var(--continua-text-muted)]">{trace.trace_id}</p>
        </div>
        <StatusBadge status={trace.status} />
      </div>

      <dl className="mt-4 grid gap-3 sm:grid-cols-2">
        <OverviewMetric label="Started" value={formatRelativeTime(trace.started_at)} compact />
        <OverviewMetric label="Ended" value={formatRelativeTime(trace.ended_at)} compact />
        <OverviewMetric label="Duration" value={formatDuration(trace.duration_ms)} compact />
        <OverviewMetric
          label="Tokens"
          value={`${formatTokens(trace.total_tokens_in)} in`}
          hint={`${formatTokens(trace.total_tokens_out)} out`}
          compact
        />
        <OverviewMetric label="Cost" value={formatCost(trace.total_cost_usd)} compact />
        <OverviewMetric label="Errors" value={String(trace.error_count ?? 0)} compact />
      </dl>
    </article>
  );
}

function CompareSpanRow({
  row,
  traces,
  expanded,
  hasSemanticContent,
  onToggleExpanded,
  currentCompareUrl,
}: {
  row: SpanDiffRow;
  traces: SessionCompareResponse;
  expanded: boolean;
  hasSemanticContent: boolean;
  onToggleExpanded: () => void;
  currentCompareUrl: string;
}) {
  const rowToneClass =
    row.diff_status === 'changed'
      ? 'border-amber-300/35 bg-amber-50/60 dark:border-amber-400/20 dark:bg-amber-400/10'
      : row.diff_status === 'baseline_only'
        ? 'border-rose-300/35 bg-rose-50/60 dark:border-rose-400/20 dark:bg-rose-400/10'
        : row.diff_status === 'candidate_only'
          ? 'border-sky-300/35 bg-sky-50/60 dark:border-sky-400/20 dark:bg-sky-400/10'
          : 'border-[var(--continua-border-strong)] bg-[var(--continua-surface-elevated)]';

  return (
    <article
      className={`rounded-[1.35rem] border p-4 shadow-[var(--continua-shadow-soft)] ${rowToneClass}`}
      style={{ marginLeft: `${row.depth * 12}px` }}
    >
      <div className="flex flex-col gap-4">
        <div className="flex flex-wrap items-center gap-2">
          <DiffStatusPill diffStatus={row.diff_status} />
          {row.match_source ? <MatchSourcePill matchSource={row.match_source} matchReason={row.match_reason} /> : null}
          {row.changed_fields.map((field) => (
            <span
              key={field}
              className="rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-2 py-0.5 text-xs font-medium text-[var(--continua-text-secondary)]"
            >
              {field}
            </span>
          ))}
          {hasSemanticContent ? (
            <button
              type="button"
              onClick={onToggleExpanded}
              className="ml-auto rounded-full border border-[var(--continua-border-strong)] bg-[var(--continua-surface-elevated)] px-3 py-1.5 text-xs font-medium text-[var(--continua-text-secondary)] transition hover:border-[var(--continua-accent)] hover:text-[var(--continua-accent)]"
            >
              {expanded ? 'Hide semantic details' : 'Show semantic details'}
            </button>
          ) : null}
        </div>

        <div className="grid gap-4 lg:grid-cols-2">
          <CompareSpanSide
            currentCompareUrl={currentCompareUrl}
            label="Baseline"
            span={row.baseline_span}
            traceId={traces.baseline.id}
          />
          <CompareSpanSide
            currentCompareUrl={currentCompareUrl}
            label="Candidate"
            span={row.candidate_span}
            traceId={traces.candidate.id}
          />
        </div>

        {expanded && hasSemanticContent ? (
          <div className="rounded-[1.1rem] border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] p-3">
            <div className="space-y-3">
              {row.semantic_groups.map((group, index) => (
                <CompareSemanticGroupRow
                  key={`${group.event_type}:${group.baseline_event?.id ?? 'none'}:${group.candidate_event?.id ?? 'none'}:${index}`}
                  group={group}
                />
              ))}
            </div>
          </div>
        ) : null}
      </div>
    </article>
  );
}

function CompareSpanSide({
  label,
  span,
  traceId,
  currentCompareUrl,
}: {
  label: string;
  span: CompareSpanSummary | null;
  traceId: string;
  currentCompareUrl: string;
}) {
  if (!span) {
    return (
      <div className="rounded-[1.05rem] border border-dashed border-[var(--continua-border-strong)] bg-[var(--continua-surface-muted)] px-3 py-4 text-sm text-[var(--continua-text-muted)]">
        {label}: no matching span
      </div>
    );
  }

  return (
    <div className="rounded-[1.05rem] border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] p-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-xs font-medium uppercase tracking-[0.16em] text-[var(--continua-text-muted)]">
            {label}
          </p>
          <Link
            to={`/traces/${traceId}?span=${encodeURIComponent(span.span_id)}`}
            state={{ returnTo: currentCompareUrl }}
            className="mt-2 inline-block text-sm font-semibold text-[var(--continua-accent)] hover:opacity-80"
          >
            {span.name}
          </Link>
          <p className="mt-1 font-mono text-xs text-[var(--continua-text-muted)]">{span.span_id}</p>
        </div>
        <StatusBadge status={span.status} />
      </div>

      <dl className="mt-3 grid gap-3 sm:grid-cols-2">
        <OverviewMetric label="Duration" value={formatDuration(span.latency_ms)} compact />
        <OverviewMetric
          label="Tokens"
          value={`${formatTokens(span.tokens_in)} in`}
          hint={`${formatTokens(span.tokens_out)} out`}
          compact
        />
        <OverviewMetric label="Cost" value={formatCost(span.cost_usd)} compact />
        <OverviewMetric label="Model" value={span.model ?? '-'} compact />
      </dl>
    </div>
  );
}

function CompareSemanticGroupRow({
  group,
}: {
  group: SpanDiffRow['semantic_groups'][number];
}) {
  return (
    <div className="rounded-[1.05rem] border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] p-3">
      <div className="flex flex-wrap items-center gap-2">
        <span className="rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-2 py-0.5 text-xs font-medium uppercase tracking-wide text-[var(--continua-text-secondary)]">
          {group.event_type}
        </span>
        <DiffStatusPill diffStatus={group.diff_status} small />
        {group.match_source ? <MatchSourcePill matchSource={group.match_source} matchReason={group.match_reason} small /> : null}
        {group.changed_fields.map((field) => (
          <span
            key={field}
            className="rounded-full bg-slate-200 px-2 py-0.5 text-xs font-medium text-slate-700 dark:bg-slate-800 dark:text-slate-200"
          >
            {field}
          </span>
        ))}
      </div>

      <div className="mt-3 grid gap-3 lg:grid-cols-2">
        <CompareSemanticSide label="Baseline" event={group.baseline_event} />
        <CompareSemanticSide label="Candidate" event={group.candidate_event} />
      </div>
    </div>
  );
}

function CompareSemanticSide({
  label,
  event,
}: {
  label: string;
  event: CompareSemanticSummary | null;
}) {
  if (!event) {
    return (
      <div className="rounded-[1.05rem] border border-dashed border-[var(--continua-border-strong)] bg-[var(--continua-surface-muted)] px-3 py-4 text-sm text-[var(--continua-text-muted)]">
        {label}: no matching event
      </div>
    );
  }

  return (
    <div className="rounded-[1.05rem] border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] p-3">
      <p className="text-xs font-medium uppercase tracking-[0.16em] text-[var(--continua-text-muted)]">
        {label}
      </p>
      <p className="mt-2 text-sm font-medium text-[var(--continua-text-primary)]">
        {event.message ?? '(no message)'}
      </p>
      <p className="mt-1 text-xs text-[var(--continua-text-muted)]">
        {formatRelativeTime(event.timestamp)}
      </p>
      {event.payload ? (
        <pre className="mt-3 overflow-x-auto rounded-[0.95rem] bg-slate-950 px-3 py-2 text-xs text-slate-100">
          {JSON.stringify(event.payload, null, 2)}
        </pre>
      ) : null}
    </div>
  );
}

function ComparisonTooLargePanel({
  error,
}: {
  error: ApiError & { detail: ComparisonTooLargeErrorDetail };
}) {
  return (
    <section className="rounded-[1.5rem] border border-amber-300/40 bg-amber-50/80 p-6 text-amber-900 dark:border-amber-500/25 dark:bg-amber-500/10 dark:text-amber-100">
      <h1 className="text-lg font-semibold">Comparison exceeds the v1 ceiling</h1>
      <p className="mt-2 text-sm">{error.message}</p>
      <dl className="mt-4 grid gap-3 md:grid-cols-2">
        <OverviewMetric
          label="Baseline"
          value={`${error.detail.baseline_span_count} spans`}
          hint={`${error.detail.baseline_semantic_count} semantic events`}
        />
        <OverviewMetric
          label="Candidate"
          value={`${error.detail.candidate_span_count} spans`}
          hint={`${error.detail.candidate_semantic_count} semantic events`}
        />
        <OverviewMetric
          label="Max Spans"
          value={String(error.detail.max_spans)}
          compact
        />
        <OverviewMetric
          label="Max Semantic Events"
          value={String(error.detail.max_semantic_events)}
          compact
        />
      </dl>
    </section>
  );
}

function OverviewMetric({
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
    <div
      className={
        compact
          ? 'rounded-[1rem] border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] p-3'
          : 'app-metric-panel'
      }
    >
      <dt className="text-xs font-medium uppercase tracking-[0.16em] text-[var(--continua-text-muted)]">
        {label}
      </dt>
      <dd className="mt-2 text-sm font-semibold text-[var(--continua-text-primary)]">{value}</dd>
      {hint ? <p className="mt-1 text-xs text-[var(--continua-text-secondary)]">{hint}</p> : null}
    </div>
  );
}

function DiffStatusPill({
  diffStatus,
  small = false,
}: {
  diffStatus: SpanDiffRow['diff_status'];
  small?: boolean;
}) {
  const classes =
    diffStatus === 'changed'
      ? 'bg-amber-100 text-amber-900 dark:bg-amber-500/20 dark:text-amber-100'
      : diffStatus === 'baseline_only'
        ? 'bg-rose-100 text-rose-900 dark:bg-rose-500/20 dark:text-rose-100'
        : diffStatus === 'candidate_only'
          ? 'bg-sky-100 text-sky-900 dark:bg-sky-500/20 dark:text-sky-100'
          : 'bg-emerald-100 text-emerald-900 dark:bg-emerald-500/20 dark:text-emerald-100';

  return (
    <span
      className={`rounded-full px-2 py-0.5 font-medium ${classes} ${small ? 'text-[11px]' : 'text-xs'}`}
    >
      {diffStatus.replace('_', ' ')}
    </span>
  );
}

function MatchSourcePill({
  matchSource,
  matchReason,
  small = false,
}: {
  matchSource: NonNullable<SpanDiffRow['match_source']>;
  matchReason?: string;
  small?: boolean;
}) {
  return (
    <span
      className={`rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-2 py-0.5 font-medium text-[var(--continua-text-secondary)] ${
        small ? 'text-[11px]' : 'text-xs'
      }`}
      title={matchReason ?? undefined}
    >
      {matchSource === 'stable_id' ? 'Stable ID' : 'Heuristic'}
    </span>
  );
}

function formatSignedDuration(value: number): string {
  if (value === 0) {
    return '0ms';
  }

  const prefix = value > 0 ? '+' : '-';
  return `${prefix}${formatDuration(Math.abs(value))}`;
}

function formatSignedNumber(value: number): string {
  if (value === 0) {
    return '0';
  }
  return `${value > 0 ? '+' : ''}${value}`;
}

function formatSignedCost(value: number): string {
  if (value === 0) {
    return '$0.0000';
  }
  const prefix = value > 0 ? '+' : '-';
  return `${prefix}${formatCost(Math.abs(value))}`;
}
