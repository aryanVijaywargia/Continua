import { useQuery } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { Link, useLocation, useParams, useSearchParams } from 'react-router-dom';
import { AuthErrorBanner } from '../components/AuthErrorBanner';
import { StatusBadge } from '../components/StatusBadge';
import {
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
      <div className="min-h-screen flex items-center justify-center bg-slate-50 dark:bg-slate-950">
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
        <section className="rounded-xl border border-amber-200 bg-amber-50 p-6 text-amber-900 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-100">
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
        <div className="rounded-xl border border-slate-200 bg-white p-8 text-center text-slate-500 shadow-sm dark:border-slate-800 dark:bg-slate-900 dark:text-slate-400">
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
          <div className="rounded-xl border border-red-200 bg-red-50 p-6 text-red-700 dark:border-red-500/40 dark:bg-red-500/10 dark:text-red-200">
            Error loading comparison: {queryErrorMessage(comparisonQuery.error)}
          </div>
        )}
      </ComparePageShell>
    );
  }

  if (!comparisonQuery.data) {
    return (
      <ComparePageShell returnTo={returnTo}>
        <div className="rounded-xl border border-slate-200 bg-white p-8 text-center text-slate-500 shadow-sm dark:border-slate-800 dark:bg-slate-900 dark:text-slate-400">
          Comparison not found.
        </div>
      </ComparePageShell>
    );
  }

  return (
    <ComparePageShell returnTo={returnTo}>
      <CompareOverview comparison={comparisonQuery.data} currentCompareUrl={currentCompareUrl} />

      <section className="rounded-xl border border-slate-200 bg-white p-6 shadow-sm dark:border-slate-800 dark:bg-slate-900">
        <div className="flex flex-col gap-2 sm:flex-row sm:items-end sm:justify-between">
          <div>
            <h2 className="text-lg font-semibold text-slate-900 dark:text-slate-100">Span Diff</h2>
            <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
              Ordered baseline-first diff with inline semantic event comparison.
            </p>
          </div>
          <div className="text-sm text-slate-500 dark:text-slate-400">
            {comparisonQuery.data.span_diffs.length} rows
          </div>
        </div>

        {comparisonQuery.data.span_diffs.length === 0 ? (
          <div className="mt-6 rounded-lg border border-dashed border-slate-300 bg-slate-50 px-4 py-5 text-sm text-slate-600 dark:border-slate-700 dark:bg-slate-950/40 dark:text-slate-300">
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
                    <div className="mb-3 rounded-lg border border-sky-200 bg-sky-50 px-3 py-2 text-sm font-medium text-sky-900 dark:border-sky-500/30 dark:bg-sky-500/10 dark:text-sky-100">
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
    <div className="min-h-screen bg-slate-50 dark:bg-slate-950">
      <div className="mx-auto max-w-7xl px-4 py-8 sm:px-6 lg:px-8">
        <Link
          to={returnTo}
          className="mb-4 inline-block text-sm text-blue-600 hover:text-blue-800 dark:text-sky-400 dark:hover:text-sky-300"
        >
          &larr; Back to Session
        </Link>
        <div className="space-y-6">{children}</div>
      </div>
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
    <section className="rounded-xl border border-slate-200 bg-white p-6 shadow-sm dark:border-slate-800 dark:bg-slate-900">
      <div className="flex flex-col gap-6 xl:flex-row xl:items-start xl:justify-between">
        <div>
          <p className="text-xs font-medium uppercase tracking-[0.16em] text-slate-500 dark:text-slate-400">
            Session Compare
          </p>
          <h1 className="mt-2 text-2xl font-bold text-slate-900 dark:text-slate-100">
            {comparison.session.external_id}
          </h1>
          {comparison.session.name ? (
            <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">{comparison.session.name}</p>
          ) : null}
          <p className="mt-2 font-mono text-xs text-slate-500 dark:text-slate-400">
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
    <article className="rounded-xl border border-slate-200 bg-slate-50/80 p-4 dark:border-slate-800 dark:bg-slate-950/50">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-xs font-medium uppercase tracking-[0.16em] text-slate-500 dark:text-slate-400">
            {label}
          </p>
          <Link
            to={`/traces/${trace.id}`}
            state={{ returnTo: currentCompareUrl }}
            className="mt-2 inline-block text-sm font-semibold text-blue-700 hover:text-blue-900 dark:text-sky-400 dark:hover:text-sky-300"
          >
            {trace.name}
          </Link>
          <p className="mt-2 font-mono text-xs text-slate-500 dark:text-slate-400">{trace.trace_id}</p>
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
  return (
    <article
      className={`rounded-xl border p-4 shadow-sm ${
        row.diff_status === 'changed'
          ? 'border-amber-200 bg-amber-50/70 dark:border-amber-500/30 dark:bg-amber-500/10'
          : row.diff_status === 'baseline_only'
            ? 'border-rose-200 bg-rose-50/70 dark:border-rose-500/30 dark:bg-rose-500/10'
            : row.diff_status === 'candidate_only'
              ? 'border-sky-200 bg-sky-50/70 dark:border-sky-500/30 dark:bg-sky-500/10'
              : 'border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-900'
      }`}
      style={{ marginLeft: `${row.depth * 12}px` }}
    >
      <div className="flex flex-col gap-4">
        <div className="flex flex-wrap items-center gap-2">
          <DiffStatusPill diffStatus={row.diff_status} />
          {row.match_source ? <MatchSourcePill matchSource={row.match_source} matchReason={row.match_reason} /> : null}
          {row.changed_fields.map((field) => (
            <span
              key={field}
              className="rounded-full bg-slate-200 px-2 py-0.5 text-xs font-medium text-slate-700 dark:bg-slate-800 dark:text-slate-200"
            >
              {field}
            </span>
          ))}
          {hasSemanticContent ? (
            <button
              type="button"
              onClick={onToggleExpanded}
              className="ml-auto rounded-md border border-slate-300 px-2.5 py-1 text-xs font-medium text-slate-700 hover:border-slate-400 hover:text-slate-900 dark:border-slate-700 dark:text-slate-200 dark:hover:border-slate-500 dark:hover:text-slate-100"
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
          <div className="rounded-lg border border-slate-200 bg-slate-50/70 p-3 dark:border-slate-800 dark:bg-slate-950/40">
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
      <div className="rounded-lg border border-dashed border-slate-300 bg-slate-50 px-3 py-4 text-sm text-slate-500 dark:border-slate-700 dark:bg-slate-950/40 dark:text-slate-400">
        {label}: no matching span
      </div>
    );
  }

  return (
    <div className="rounded-lg border border-slate-200 bg-slate-50/80 p-3 dark:border-slate-800 dark:bg-slate-950/50">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-xs font-medium uppercase tracking-[0.16em] text-slate-500 dark:text-slate-400">
            {label}
          </p>
          <Link
            to={`/traces/${traceId}?span=${encodeURIComponent(span.span_id)}`}
            state={{ returnTo: currentCompareUrl }}
            className="mt-2 inline-block text-sm font-semibold text-blue-700 hover:text-blue-900 dark:text-sky-400 dark:hover:text-sky-300"
          >
            {span.name}
          </Link>
          <p className="mt-1 font-mono text-xs text-slate-500 dark:text-slate-400">{span.span_id}</p>
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
    <div className="rounded-lg border border-slate-200 bg-white p-3 dark:border-slate-800 dark:bg-slate-900">
      <div className="flex flex-wrap items-center gap-2">
        <span className="rounded-full bg-slate-200 px-2 py-0.5 text-xs font-medium uppercase tracking-wide text-slate-700 dark:bg-slate-800 dark:text-slate-200">
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
      <div className="rounded-lg border border-dashed border-slate-300 bg-slate-50 px-3 py-4 text-sm text-slate-500 dark:border-slate-700 dark:bg-slate-950/40 dark:text-slate-400">
        {label}: no matching event
      </div>
    );
  }

  return (
    <div className="rounded-lg border border-slate-200 bg-slate-50/70 p-3 dark:border-slate-800 dark:bg-slate-950/40">
      <p className="text-xs font-medium uppercase tracking-[0.16em] text-slate-500 dark:text-slate-400">
        {label}
      </p>
      <p className="mt-2 text-sm font-medium text-slate-900 dark:text-slate-100">
        {event.message ?? '(no message)'}
      </p>
      <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">
        {formatRelativeTime(event.timestamp)}
      </p>
      {event.payload ? (
        <pre className="mt-3 overflow-x-auto rounded-md bg-slate-950 px-3 py-2 text-xs text-slate-100">
          {JSON.stringify(event.payload, null, 2)}
        </pre>
      ) : null}
    </div>
  );
}

function ComparisonTooLargePanel({
  error,
}: {
  error: ReturnType<typeof useQuery>['error'] & {
    detail: {
      baseline_span_count: number;
      candidate_span_count: number;
      baseline_semantic_count: number;
      candidate_semantic_count: number;
      max_spans: number;
      max_semantic_events: number;
    };
  };
}) {
  return (
    <section className="rounded-xl border border-amber-200 bg-amber-50 p-6 text-amber-900 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-100">
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
    <div className={compact ? '' : 'rounded-lg border border-slate-200 p-4 dark:border-slate-800'}>
      <dt className="text-xs font-medium uppercase tracking-[0.16em] text-slate-500 dark:text-slate-400">
        {label}
      </dt>
      <dd className="mt-2 text-sm font-semibold text-slate-900 dark:text-slate-100">{value}</dd>
      {hint ? <p className="mt-1 text-xs text-slate-500 dark:text-slate-400">{hint}</p> : null}
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
      className={`rounded-full bg-slate-200 px-2 py-0.5 font-medium text-slate-700 dark:bg-slate-800 dark:text-slate-200 ${
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
