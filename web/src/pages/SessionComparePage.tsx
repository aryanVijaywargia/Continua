import { useQuery } from '@tanstack/react-query';
import { useEffect, type ReactNode } from 'react';
import { Link, useLocation, useParams, useSearchParams } from 'react-router-dom';
import { ArrowLeft, ArrowLeftRight, ArrowRight } from 'lucide-react';
import { AuthErrorBanner } from '../components/AuthErrorBanner';
import { EngineBadge } from '../components/EngineBadge';
import { formatProjectionStateLabel } from '../components/engineProjectionState';
import { HonestyNote, ReadOnlyBadge, UnverifiedPill } from '../components/DataState';
import { Btn, PageHeader, StatusDot } from '../components/DebuggerKit';
import {
  ApiError,
  type CompareTraceHeader,
  type ComparisonTooLargeErrorDetail,
  fetchSessionComparison,
  fetchSessionNarrative,
  isAuthError,
  isComparisonTooLargeError,
  type SessionCompareResponse,
} from '../api/client';
import { formatCost, formatDerivedDuration, formatExactTime } from '../utils/format';
import {
  buildCompareSearchParams,
  getCompareReturnToDestination,
  normalizeCompareTraceIdParam,
} from './sessionCompareUtils';
import { appendProjectToPath, normalizeProjectId } from '../utils/projectSearchParams';
import { CompareStepTable } from './session/CompareStepTable';
import {
  formatSignedMs,
  formatSignedPercent,
  isUsageUnverified,
  shortId,
} from './session/sessionDisplay';

function queryErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Unknown error';
}

export function SessionComparePage() {
  const { id } = useParams<{ id: string }>();

  if (!id) {
    return (
      <div className="flex min-h-full items-center justify-center">
        <div className="text-[var(--c-red-text)]">Session ID is required</div>
      </div>
    );
  }

  return <SessionCompareContent sessionId={id} />;
}

interface TraceOption {
  id: string;
  label: string;
  running: boolean;
}

function SessionCompareContent({ sessionId }: { sessionId: string }) {
  const location = useLocation();
  const [searchParams, setSearchParams] = useSearchParams();

  const baselineTraceId = normalizeCompareTraceIdParam(searchParams.get('baseline_trace_id'));
  const candidateTraceIdRaw = normalizeCompareTraceIdParam(searchParams.get('candidate_trace_id'));
  const projectId = normalizeProjectId(searchParams.get('project_id'));
  const candidateTraceId =
    candidateTraceIdRaw && candidateTraceIdRaw !== baselineTraceId ? candidateTraceIdRaw : undefined;
  const canonicalSearch = buildCompareSearchParams(
    projectId,
    baselineTraceId,
    candidateTraceId
  ).toString();
  const currentCompareUrl = `${location.pathname}${canonicalSearch ? `?${canonicalSearch}` : ''}`;
  const returnTo = getCompareReturnToDestination(location.state, sessionId, searchParams);

  useEffect(() => {
    if (searchParams.toString() === canonicalSearch) {
      return;
    }

    setSearchParams(new URLSearchParams(canonicalSearch), { replace: true });
  }, [canonicalSearch, searchParams, setSearchParams]);

  const comparisonQuery = useQuery({
    queryKey: ['session-compare', sessionId, projectId ?? null, baselineTraceId, candidateTraceId],
    queryFn: () =>
      fetchSessionComparison(sessionId, baselineTraceId!, candidateTraceId!, projectId),
    enabled: Boolean(baselineTraceId && candidateTraceId),
  });

  // The narrative lists the session's traces, which feed the two pickers.
  const narrativeQuery = useQuery({
    queryKey: ['session-narrative', sessionId, projectId ?? null],
    queryFn: () => fetchSessionNarrative(sessionId, projectId),
    enabled: Boolean(baselineTraceId && candidateTraceId),
  });

  const selectPair = (nextBaseline: string, nextCandidate: string) => {
    setSearchParams(buildCompareSearchParams(projectId, nextBaseline, nextCandidate), {
      state: location.state,
    });
  };

  if (!baselineTraceId || !candidateTraceId) {
    return (
      <ComparePageShell returnTo={returnTo}>
        <section className="rounded-md border border-[var(--c-amber-border)] bg-[var(--c-amber-faint)] p-5 text-[var(--c-amber-text)]">
          <h1 className="text-base font-semibold">Comparison needs two traces</h1>
          <p className="mt-1.5 text-[13px]">
            Open this page from a session with both a baseline and candidate trace selected.
          </p>
        </section>
      </ComparePageShell>
    );
  }

  if (comparisonQuery.isLoading) {
    return (
      <ComparePageShell returnTo={returnTo}>
        <div className="app-empty-state">Loading comparison...</div>
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
        <div className="app-empty-state">Comparison not found.</div>
      </ComparePageShell>
    );
  }

  const comparison = comparisonQuery.data;
  const traceOptions = buildTraceOptions(comparison, narrativeQuery.data?.traces ?? []);

  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
      <PageHeader
        eyebrow={<BackToSessionLink returnTo={returnTo} />}
        title={comparison.session.name ?? comparison.session.external_id}
        description={
          <span className="flex flex-wrap items-center gap-2 text-[12px]">
            <span>Compare two traces from this session</span>
            <span className="font-mono text-[var(--c-text-muted)]">
              {comparison.session.external_id}
            </span>
            <ReadOnlyBadge />
          </span>
        }
      />

      <CompareToolbar
        baselineTraceId={baselineTraceId}
        candidateTraceId={candidateTraceId}
        comparison={comparison}
        onSelectPair={selectPair}
        traceOptions={traceOptions}
      />

      <div className="flex flex-col gap-5 px-6 py-5">
        <section aria-label="Compared traces" className="grid gap-3 md:grid-cols-2">
          <CompareTraceCard
            currentCompareUrl={currentCompareUrl}
            label="Baseline"
            projectId={projectId}
            semanticCount={comparison.summary.total_semantic_baseline}
            spanCount={comparison.summary.total_spans_baseline}
            trace={comparison.baseline}
          />
          <CompareTraceCard
            currentCompareUrl={currentCompareUrl}
            label="Candidate"
            projectId={projectId}
            semanticCount={comparison.summary.total_semantic_candidate}
            spanCount={comparison.summary.total_spans_candidate}
            trace={comparison.candidate}
          />
        </section>

        <section aria-labelledby="step-comparison-heading">
          <div className="mb-2 flex flex-wrap items-baseline justify-between gap-2">
            <h2
              id="step-comparison-heading"
              className="text-[15px] font-semibold text-[var(--c-text-primary)]"
            >
              Step comparison
            </h2>
            <span className="text-[11.5px] text-[var(--c-text-muted)]">
              red is slower · green is faster
            </span>
          </div>
          <CompareStepTable
            comparison={comparison}
            currentCompareUrl={currentCompareUrl}
            projectId={projectId}
          />
        </section>
      </div>
    </div>
  );
}

function buildTraceOptions(
  comparison: SessionCompareResponse,
  narrativeTraces: Array<{
    id: string;
    name: string;
    status: CompareTraceHeader['status'];
    duration_ms?: number;
  }>
): TraceOption[] {
  const seen = new Set<string>();
  const options: TraceOption[] = [];
  const add = (trace: {
    id: string;
    name: string;
    status: CompareTraceHeader['status'];
    duration_ms?: number;
  }) => {
    const id = normalizeCompareTraceIdParam(trace.id);
    if (!id || seen.has(id)) {
      return;
    }
    seen.add(id);
    const duration =
      trace.status === 'RUNNING' ? 'running' : formatDerivedDuration(trace.duration_ms);
    options.push({
      id,
      label: `${shortId(id)} · ${trace.name} · ${duration}`,
      running: trace.status === 'RUNNING',
    });
  };

  narrativeTraces.forEach(add);
  add(comparison.baseline);
  add(comparison.candidate);
  return options;
}

function CompareToolbar({
  baselineTraceId,
  candidateTraceId,
  comparison,
  onSelectPair,
  traceOptions,
}: {
  baselineTraceId: string;
  candidateTraceId: string;
  comparison: SessionCompareResponse;
  onSelectPair: (baseline: string, candidate: string) => void;
  traceOptions: TraceOption[];
}) {
  const { summary } = comparison;
  const deltaMs = summary.duration_delta_ms;
  const percent = formatSignedPercent(deltaMs, comparison.baseline.duration_ms);
  const deltaTone =
    deltaMs > 0
      ? 'text-[var(--c-red-text)]'
      : deltaMs < 0
        ? 'text-[var(--c-green-text)]'
        : 'text-[var(--c-text-secondary)]';
  const usageUnverified =
    isUsageUnverified([
      comparison.baseline.total_tokens_in,
      comparison.baseline.total_tokens_out,
      comparison.baseline.total_cost_usd,
    ]) ||
    isUsageUnverified([
      comparison.candidate.total_tokens_in,
      comparison.candidate.total_tokens_out,
      comparison.candidate.total_cost_usd,
    ]);

  const pick = (role: 'baseline' | 'candidate', value: string) => {
    if (role === 'baseline') {
      // Picking the other role's trace swaps the pair instead of comparing a trace to itself.
      onSelectPair(value, value === candidateTraceId ? baselineTraceId : candidateTraceId);
      return;
    }
    onSelectPair(value === baselineTraceId ? candidateTraceId : baselineTraceId, value);
  };

  return (
    <div
      role="region"
      aria-label="Comparison toolbar"
      className="sticky top-0 z-20 flex flex-wrap items-center gap-x-4 gap-y-2 border-b border-[var(--c-border)] bg-[var(--c-surface)] px-6 py-2.5"
    >
      <div className="flex flex-wrap items-center gap-2">
        <TracePicker
          label="Baseline"
          onChange={(value) => pick('baseline', value)}
          options={traceOptions}
          value={baselineTraceId}
        />
        <ArrowRight aria-hidden="true" className="h-3.5 w-3.5 text-[var(--c-text-muted)]" />
        <TracePicker
          label="Candidate"
          onChange={(value) => pick('candidate', value)}
          options={traceOptions}
          value={candidateTraceId}
        />
        <Btn
          kind="ghost"
          size="sm"
          leadingIcon={ArrowLeftRight}
          onClick={() => onSelectPair(candidateTraceId, baselineTraceId)}
          aria-label="Swap baseline and candidate"
        >
          Swap
        </Btn>
      </div>

      <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-[12.5px]">
        <span
          className={`font-mono font-semibold tabular-nums ${deltaTone}`}
          title="Candidate duration minus baseline duration, as recorded"
        >
          {formatSignedMs(deltaMs)} total{percent ? ` (${percent})` : ''}
        </span>
        <span className="font-mono tabular-nums text-[var(--c-text-secondary)]">
          spans {summary.total_spans_baseline} → {summary.total_spans_candidate}
        </span>
        {usageUnverified ? (
          <UnverifiedPill title="Token or cost totals read 0 or are missing on at least one trace, so a usage delta is not shown.">
            usage not verified
          </UnverifiedPill>
        ) : (
          <span
            className="font-mono tabular-nums text-[var(--c-text-secondary)]"
            title="Candidate usage minus baseline usage, as recorded"
          >
            tokens {formatSignedNumber(summary.tokens_in_delta)} in /{' '}
            {formatSignedNumber(summary.tokens_out_delta)} out · cost{' '}
            {formatSignedCost(summary.cost_delta_usd)}
          </span>
        )}
      </div>
    </div>
  );
}

function TracePicker({
  label,
  onChange,
  options,
  value,
}: {
  label: 'Baseline' | 'Candidate';
  onChange: (value: string) => void;
  options: TraceOption[];
  value: string;
}) {
  return (
    <label className="flex items-center gap-1.5">
      <span className="text-[10.5px] font-semibold uppercase tracking-[0.06em] text-[var(--c-text-muted)]">
        {label}
      </span>
      <select
        aria-label={`${label} trace`}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        className="h-7 max-w-[15rem] truncate rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-2 font-mono text-[12px] text-[var(--c-text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--c-focus)]"
      >
        {options.map((option) => (
          <option key={option.id} value={option.id} disabled={option.running && option.id !== value}>
            {option.label}
          </option>
        ))}
      </select>
    </label>
  );
}

function BackToSessionLink({ returnTo }: { returnTo: string }) {
  return (
    <Link to={returnTo} className="inline-flex items-center gap-1 hover:text-[var(--c-text-primary)]">
      <ArrowLeft aria-hidden="true" className="h-3 w-3" />
      Back to Session
    </Link>
  );
}

function ComparePageShell({ children, returnTo }: { children: ReactNode; returnTo: string }) {
  return (
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
      <div className="border-b border-[var(--c-border)] px-6 py-3 text-[11px] font-medium text-[var(--c-text-muted)]">
        <BackToSessionLink returnTo={returnTo} />
      </div>
      <div className="flex flex-col gap-4 px-6 py-5">{children}</div>
    </div>
  );
}

function CompareTraceCard({
  currentCompareUrl,
  label,
  projectId,
  semanticCount,
  spanCount,
  trace,
}: {
  currentCompareUrl: string;
  label: 'Baseline' | 'Candidate';
  projectId?: string;
  semanticCount: number;
  spanCount: number;
  trace: CompareTraceHeader;
}) {
  return (
    <article className="rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-4 py-3">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-[10.5px] font-semibold uppercase tracking-[0.06em] text-[var(--c-text-muted)]">
            {label}
          </p>
          <div className="mt-1 flex flex-wrap items-center gap-2">
            <Link
              to={appendProjectToPath(`/traces/${trace.id}`, projectId)}
              state={{ returnTo: currentCompareUrl }}
              className="text-[14px] font-semibold text-[var(--c-accent-text)] hover:underline"
            >
              {trace.name}
            </Link>
            {trace.engine ? <EngineBadge projectionState={trace.engine.projection_state} /> : null}
          </div>
          <p className="mt-0.5 truncate font-mono text-[11.5px] text-[var(--c-text-muted)]">
            {trace.trace_id}
          </p>
          {trace.engine ? (
            <p className="mt-1 text-[11.5px] text-[var(--c-text-secondary)]">
              {trace.engine.definition_name}@{trace.engine.definition_version} ·{' '}
              {formatProjectionStateLabel(trace.engine.projection_state)}
            </p>
          ) : null}
        </div>
        <StatusDot status={trace.status} />
      </div>
      <dl className="mt-3 grid grid-cols-2 gap-x-4 gap-y-1 text-[12px] sm:grid-cols-4">
        <CardMetric label="Started">
          <span title={trace.started_at}>{formatExactTime(trace.started_at)}</span>
        </CardMetric>
        <CardMetric label="Duration">{formatDerivedDuration(trace.duration_ms)}</CardMetric>
        <CardMetric label="Errors">
          {trace.error_count != null ? `${trace.error_count} recorded` : 'Not recorded'}
        </CardMetric>
        <CardMetric label="Spans">
          {spanCount} · {semanticCount} events
        </CardMetric>
      </dl>
    </article>
  );
}

function CardMetric({ children, label }: { children: ReactNode; label: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-[10.5px] text-[var(--c-text-muted)]">{label}</dt>
      <dd className="truncate font-mono tabular-nums text-[var(--c-text-primary)]">{children}</dd>
    </div>
  );
}

function ComparisonTooLargePanel({
  error,
}: {
  error: ApiError & { detail: ComparisonTooLargeErrorDetail };
}) {
  return (
    <section className="rounded-md border border-[var(--c-amber-border)] bg-[var(--c-amber-faint)] p-5">
      <h1 className="text-base font-semibold text-[var(--c-amber-text)]">
        Comparison exceeds the v1 ceiling
      </h1>
      <p className="mt-1.5 text-[13px] text-[var(--c-text-primary)]">{error.message}</p>
      <dl className="mt-4 grid gap-3 text-[12.5px] sm:grid-cols-2 lg:grid-cols-4">
        <LimitMetric
          hint={`${error.detail.baseline_semantic_count} semantic events`}
          label="Baseline"
          value={`${error.detail.baseline_span_count} spans`}
        />
        <LimitMetric
          hint={`${error.detail.candidate_semantic_count} semantic events`}
          label="Candidate"
          value={`${error.detail.candidate_span_count} spans`}
        />
        <LimitMetric label="Max spans" value={String(error.detail.max_spans)} />
        <LimitMetric label="Max semantic events" value={String(error.detail.max_semantic_events)} />
      </dl>
      <HonestyNote className="mt-3" kind="unavailable">
        The server refuses comparisons above these limits, so no step diff is available.
      </HonestyNote>
    </section>
  );
}

function LimitMetric({ hint, label, value }: { hint?: string; label: string; value: string }) {
  return (
    <div className="rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-3 py-2">
      <dt className="text-[10.5px] font-semibold uppercase tracking-[0.06em] text-[var(--c-text-muted)]">
        {label}
      </dt>
      <dd className="mt-1 font-mono font-semibold text-[var(--c-text-primary)]">{value}</dd>
      {hint ? <p className="mt-0.5 text-[11.5px] text-[var(--c-text-secondary)]">{hint}</p> : null}
    </div>
  );
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
