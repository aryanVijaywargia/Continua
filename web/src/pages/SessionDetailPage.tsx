import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { useCallback, useEffect, useState, type ReactNode } from 'react';
import { Link, useLocation, useParams, useSearchParams } from 'react-router-dom';
import {
  Download,
  ExternalLink,
  MoreHorizontal,
  RefreshCw,
  User,
  Zap,
  type LucideIcon,
} from 'lucide-react';
import { AuthErrorBanner } from '../components/AuthErrorBanner';
import { CopyButton } from '../components/CopyButton';
import { PaginationControls } from '../components/PaginationControls';
import {
  Btn,
  Chip,
  DataTable,
  PageHeader,
  StatusDot,
  Td,
  Th,
  Tr,
} from '../components/DebuggerKit';
import {
  ApiError,
  fetchSession,
  fetchTrace,
  fetchSessionNarrative,
  fetchTraces,
  isAuthError,
  type Session,
  type SessionNarrativeSummary,
  type SessionNarrativeTrace,
  type TraceDetail,
  type Trace,
} from '../api/client';
import { DEFAULT_PAGE_SIZE, getLastValidOffset } from '../utils/pagination';
import {
  buildCanonicalQueryString,
  parseTracesParams,
  serializeTracesParams,
} from '../utils/tracesSearchParams';
import { appendProjectToPath } from '../utils/projectSearchParams';
import {
  calculateDuration,
  formatCost,
  formatDuration,
  formatRelativeTime,
  formatTokens,
} from '../utils/format';
import { summarizeTimelineEvent } from '../utils/timeline';
import {
  buildCompareSearchParams,
  normalizeCompareTraceIdParam,
} from './sessionCompareUtils';
import { downloadJsonFile } from '../utils/downloadJson';

type HistoryMode = 'push' | 'replace';
type CompareRole = 'baseline' | 'candidate';

interface SessionDetailCompareState {
  baseline_trace_id?: string;
  candidate_trace_id?: string;
}

interface CompareSelectedTrace {
  id: string;
  name: string;
  status: 'RUNNING' | 'COMPLETED' | 'FAILED';
  trace_id?: string;
  user_id?: string;
  started_at: string;
  ended_at?: string;
  duration_ms?: number;
  total_cost_usd?: number;
  total_tokens_in?: number;
  total_tokens_out?: number;
  session_id?: string;
}

interface SessionTraceTableState {
  project_id?: string;
  limit: number;
  offset: number;
  sort_by?: 'started_at';
  sort_dir?: 'asc' | 'desc';
}

function shouldResetOffset(
  currentState: SessionTraceTableState,
  updates: Partial<SessionTraceTableState>
): boolean {
  return Object.entries(updates).some(
    ([key, value]) =>
      key !== 'offset' && currentState[key as keyof SessionTraceTableState] !== value
  );
}

function getSessionTraceTableState(searchParams: URLSearchParams) {
  const parsed = parseTracesParams(searchParams);

  return {
    project_id: parsed.project_id,
    limit: parsed.limit,
    offset: parsed.offset,
    sort_by: parsed.sort_by,
    sort_dir: parsed.sort_dir,
  } satisfies SessionTraceTableState;
}

function canonicalizeCompareState(state: SessionDetailCompareState): SessionDetailCompareState {
  const baselineTraceId = normalizeCompareTraceIdParam(state.baseline_trace_id);
  const candidateTraceId = normalizeCompareTraceIdParam(state.candidate_trace_id);

  return {
    baseline_trace_id: baselineTraceId,
    candidate_trace_id:
      candidateTraceId && candidateTraceId !== baselineTraceId
        ? candidateTraceId
        : undefined,
  };
}

function getSessionCompareState(searchParams: URLSearchParams): SessionDetailCompareState {
  return canonicalizeCompareState({
    baseline_trace_id: searchParams.get('baseline_trace_id') ?? undefined,
    candidate_trace_id: searchParams.get('candidate_trace_id') ?? undefined,
  });
}

function serializeSessionDetailSearchParams(
  filters: SessionTraceTableState,
  compare: SessionDetailCompareState
): URLSearchParams {
  const params = serializeTracesParams(filters);
  buildCompareSearchParams(
    filters.project_id,
    compare.baseline_trace_id,
    compare.candidate_trace_id
  ).forEach(
    (value, key) => {
      params.set(key, value);
    }
  );
  return params;
}

function useSessionDetailSearchParams() {
  const [searchParams, setSearchParams] = useSearchParams();
  const filters = getSessionTraceTableState(searchParams);
  const compare = getSessionCompareState(searchParams);
  const canonicalSearch = serializeSessionDetailSearchParams(filters, compare).toString();

  useEffect(() => {
    if (searchParams.toString() === canonicalSearch) {
      return;
    }

    setSearchParams(new URLSearchParams(canonicalSearch), { replace: true });
  }, [canonicalSearch, searchParams, setSearchParams]);

  const setFilters = useCallback(
    (
      updates: Partial<SessionTraceTableState>,
      mode: HistoryMode = 'push'
    ) => {
      const next = {
        ...filters,
        ...updates,
      };

      const normalizedNext = shouldResetOffset(filters, updates)
        ? { ...next, offset: 0 }
        : next;

      setSearchParams(serializeSessionDetailSearchParams(normalizedNext, compare), {
        replace: mode === 'replace',
      });
    },
    [compare, filters, setSearchParams]
  );

  const replaceCompare = useCallback(
    (nextCompare: SessionDetailCompareState, mode: HistoryMode = 'push') => {
      setSearchParams(
        serializeSessionDetailSearchParams(filters, canonicalizeCompareState(nextCompare)),
        {
          replace: mode === 'replace',
        }
      );
    },
    [filters, setSearchParams]
  );

  const assignCompareRole = useCallback(
    (role: CompareRole, traceId: string, mode: HistoryMode = 'push') => {
      const nextCompare = { ...compare };

      if (role === 'baseline') {
        nextCompare.baseline_trace_id = traceId;
        if (nextCompare.candidate_trace_id === traceId) {
          nextCompare.candidate_trace_id = undefined;
        }
      } else {
        nextCompare.candidate_trace_id = traceId;
        if (nextCompare.baseline_trace_id === traceId) {
          nextCompare.baseline_trace_id = undefined;
        }
      }

      replaceCompare(nextCompare, mode);
    },
    [compare, replaceCompare]
  );

  const clearCompareRole = useCallback(
    (role: CompareRole, mode: HistoryMode = 'push') => {
      replaceCompare(
        {
          ...compare,
          [role === 'baseline' ? 'baseline_trace_id' : 'candidate_trace_id']: undefined,
        },
        mode
      );
    },
    [compare, replaceCompare]
  );

  const clearCompare = useCallback(
    (mode: HistoryMode = 'push') => {
      replaceCompare({}, mode);
    },
    [replaceCompare]
  );

  const swapCompare = useCallback(
    (mode: HistoryMode = 'push') => {
      replaceCompare(
        {
          baseline_trace_id: compare.candidate_trace_id,
          candidate_trace_id: compare.baseline_trace_id,
        },
        mode
      );
    },
    [compare, replaceCompare]
  );

  const setComparePair = useCallback(
    (
      baselineTraceId: string,
      candidateTraceId: string,
      mode: HistoryMode = 'push'
    ) => {
      replaceCompare(
        {
          baseline_trace_id: baselineTraceId,
          candidate_trace_id: candidateTraceId,
        },
        mode
      );
    },
    [replaceCompare]
  );

  return {
    filters,
    compare,
    setFilters,
    assignCompareRole,
    setComparePair,
    clearCompareRole,
    clearCompare,
    swapCompare,
  };
}

function getSessionsReturnToDestination(state: unknown): string {
  if (
    typeof state !== 'object' ||
    state === null ||
    !('returnTo' in state) ||
    typeof state.returnTo !== 'string'
  ) {
    return '/sessions';
  }

  const { returnTo } = state;
  return returnTo === '/sessions' || returnTo.startsWith('/sessions?')
    ? returnTo
    : '/sessions';
}

function queryErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Unknown error';
}

function isTerminalTraceStatus(status: CompareSelectedTrace['status']): boolean {
  return status === 'COMPLETED' || status === 'FAILED';
}

function narrativeTraceToCompareSelectedTrace(trace: SessionNarrativeTrace): CompareSelectedTrace {
  return {
    id: trace.id,
    name: trace.name,
    status: trace.status,
    trace_id: trace.trace_id,
    user_id: trace.user_id,
    started_at: trace.started_at,
    ended_at: trace.ended_at,
    duration_ms: trace.duration_ms,
    total_cost_usd: trace.total_cost_usd,
    total_tokens_in: trace.total_tokens_in,
    total_tokens_out: trace.total_tokens_out,
  };
}

function listTraceToCompareSelectedTrace(trace: Trace): CompareSelectedTrace {
  return {
    id: trace.id,
    name: trace.name,
    status: trace.status,
    started_at: trace.started_at,
    ended_at: trace.ended_at,
    total_cost_usd: trace.total_cost_usd,
    total_tokens_in: trace.total_tokens_in,
    total_tokens_out: trace.total_tokens_out,
    session_id: trace.session_id,
  };
}

function detailTraceToCompareSelectedTrace(trace: TraceDetail): CompareSelectedTrace {
  return {
    id: trace.id,
    name: trace.name,
    status: trace.status,
    trace_id: trace.trace_id,
    user_id: trace.user_id,
    started_at: trace.started_at,
    ended_at: trace.ended_at,
    total_cost_usd: trace.total_cost_usd,
    total_tokens_in: trace.total_tokens_in,
    total_tokens_out: trace.total_tokens_out,
    session_id: trace.session_id,
  };
}

export function SessionDetailPage() {
  const { id } = useParams<{ id: string }>();

  if (!id) {
    return (
      <div className="min-h-screen bg-[var(--continua-app-bg)] flex items-center justify-center">
        <div className="text-red-600">Session ID is required</div>
      </div>
    );
  }

  return <SessionDetailContent sessionId={id} />;
}

function SessionDetailContent({ sessionId }: { sessionId: string }) {
  const location = useLocation();
  const {
    filters,
    compare,
    setFilters,
    assignCompareRole,
    setComparePair,
    clearCompareRole,
    clearCompare,
    swapCompare,
  } = useSessionDetailSearchParams();
  const returnTo = getSessionsReturnToDestination(location.state);
  const [activeTab, setActiveTab] = useState<
    'journey' | 'traces' | 'context' | 'feedback'
  >('journey');
  const [journeyStatusFilter, setJourneyStatusFilter] = useState<
    'all' | 'completed' | 'failed'
  >('all');
  const currentSessionDetailSearch = serializeSessionDetailSearchParams(filters, compare).toString();
  const currentSessionDetailUrl = `${location.pathname}${currentSessionDetailSearch ? `?${currentSessionDetailSearch}` : ''}`;
  const projectQueryKey = filters.project_id ?? null;

  const sessionQuery = useQuery({
    queryKey: ['session', sessionId, projectQueryKey],
    queryFn: () => fetchSession(sessionId, filters.project_id),
  });
  const narrativeQuery = useQuery({
    queryKey: ['session-narrative', sessionId, projectQueryKey],
    queryFn: () => fetchSessionNarrative(sessionId, filters.project_id),
    refetchInterval: (query) =>
      (query.state.data?.summary.running_trace_count ?? 0) > 0 ? 30_000 : false,
  });

  const traceQueryParams = {
    project_id: filters.project_id,
    session_id: sessionId,
    limit: filters.limit,
    offset: filters.offset,
    sort_by: filters.sort_by,
    sort_dir: filters.sort_dir,
  };
  const tracesQuery = useQuery({
    queryKey: ['session-traces', sessionId, buildCanonicalQueryString(traceQueryParams)],
    queryFn: () => fetchTraces(traceQueryParams),
    placeholderData: keepPreviousData,
  });
  const traces = tracesQuery.data?.traces ?? [];
  const total = tracesQuery.data?.total ?? 0;

  const selectedBaselineFromNarrative = narrativeQuery.data?.traces.find(
    (trace) => trace.id === compare.baseline_trace_id
  );
  const selectedCandidateFromNarrative = narrativeQuery.data?.traces.find(
    (trace) => trace.id === compare.candidate_trace_id
  );
  const selectedBaselineFromTable = traces.find((trace) => trace.id === compare.baseline_trace_id);
  const selectedCandidateFromTable = traces.find((trace) => trace.id === compare.candidate_trace_id);

  const selectedBaselineLoaded =
    selectedBaselineFromNarrative
      ? narrativeTraceToCompareSelectedTrace(selectedBaselineFromNarrative)
      : selectedBaselineFromTable
        ? listTraceToCompareSelectedTrace(selectedBaselineFromTable)
        : undefined;
  const selectedCandidateLoaded =
    selectedCandidateFromNarrative
      ? narrativeTraceToCompareSelectedTrace(selectedCandidateFromNarrative)
      : selectedCandidateFromTable
        ? listTraceToCompareSelectedTrace(selectedCandidateFromTable)
        : undefined;

  const baselineLookupQuery = useQuery({
    queryKey: ['trace', compare.baseline_trace_id, projectQueryKey],
    queryFn: () => fetchTrace(compare.baseline_trace_id!, filters.project_id),
    enabled: Boolean(compare.baseline_trace_id && !selectedBaselineLoaded),
  });
  const candidateLookupQuery = useQuery({
    queryKey: ['trace', compare.candidate_trace_id, projectQueryKey],
    queryFn: () => fetchTrace(compare.candidate_trace_id!, filters.project_id),
    enabled: Boolean(compare.candidate_trace_id && !selectedCandidateLoaded),
  });

  useEffect(() => {
    if (!compare.baseline_trace_id || selectedBaselineLoaded) {
      return;
    }

    if (baselineLookupQuery.data && baselineLookupQuery.data.session_id !== sessionId) {
      clearCompareRole('baseline', 'replace');
      return;
    }

    if (
      baselineLookupQuery.error instanceof ApiError &&
      baselineLookupQuery.error.status === 404
    ) {
      clearCompareRole('baseline', 'replace');
    }
  }, [
    baselineLookupQuery.data,
    baselineLookupQuery.error,
    clearCompareRole,
    compare.baseline_trace_id,
    selectedBaselineLoaded,
    sessionId,
  ]);

  useEffect(() => {
    if (!compare.candidate_trace_id || selectedCandidateLoaded) {
      return;
    }

    if (candidateLookupQuery.data && candidateLookupQuery.data.session_id !== sessionId) {
      clearCompareRole('candidate', 'replace');
      return;
    }

    if (
      candidateLookupQuery.error instanceof ApiError &&
      candidateLookupQuery.error.status === 404
    ) {
      clearCompareRole('candidate', 'replace');
    }
  }, [
    candidateLookupQuery.data,
    candidateLookupQuery.error,
    clearCompareRole,
    compare.candidate_trace_id,
    selectedCandidateLoaded,
    sessionId,
  ]);

  const selectedBaseline =
    selectedBaselineLoaded ??
    (baselineLookupQuery.data && baselineLookupQuery.data.session_id === sessionId
      ? detailTraceToCompareSelectedTrace(baselineLookupQuery.data)
      : undefined);
  const selectedCandidate =
    selectedCandidateLoaded ??
    (candidateLookupQuery.data && candidateLookupQuery.data.session_id === sessionId
      ? detailTraceToCompareSelectedTrace(candidateLookupQuery.data)
      : undefined);

  const isBaselineLookupPending = Boolean(compare.baseline_trace_id && !selectedBaselineLoaded) && baselineLookupQuery.isPending;
  const isCandidateLookupPending = Boolean(compare.candidate_trace_id && !selectedCandidateLoaded) && candidateLookupQuery.isPending;
  const compareSelectionVisible = Boolean(compare.baseline_trace_id || compare.candidate_trace_id);
  let canOpenComparison = false;
  if (selectedBaseline && selectedCandidate && !isBaselineLookupPending && !isCandidateLookupPending) {
    canOpenComparison =
      isTerminalTraceStatus(selectedBaseline.status) &&
      isTerminalTraceStatus(selectedCandidate.status);
  }
  const handleExportSession = useCallback(() => {
    downloadJsonFile(`continua-session-${sessionId}.json`, {
      exported_at: new Date().toISOString(),
      source: currentSessionDetailUrl,
      session: sessionQuery.data ?? null,
      narrative: narrativeQuery.data ?? null,
      traces: tracesQuery.data?.traces ?? [],
      trace_page: {
        total: tracesQuery.data?.total ?? 0,
        limit: filters.limit,
        offset: filters.offset,
        sort_by: filters.sort_by,
        sort_dir: filters.sort_dir,
      },
      compare,
    });
  }, [
    compare,
    currentSessionDetailUrl,
    filters.limit,
    filters.offset,
    filters.sort_by,
    filters.sort_dir,
    narrativeQuery.data,
    sessionId,
    sessionQuery.data,
    tracesQuery.data,
  ]);

  useEffect(() => {
    if (traces.length !== 0 || total === 0 || filters.offset === 0) {
      return;
    }

    const lastValidOffset = getLastValidOffset(total, filters.limit ?? DEFAULT_PAGE_SIZE);
    if (lastValidOffset !== filters.offset) {
      setFilters({ offset: lastValidOffset }, 'replace');
    }
  }, [filters.limit, filters.offset, setFilters, total, traces.length]);

  const handleStartedSortToggle = useCallback(() => {
    setFilters(
      {
        sort_by: 'started_at',
        sort_dir:
          filters.sort_by === 'started_at' && filters.sort_dir === 'asc'
            ? 'desc'
            : 'asc',
      },
      'push'
    );
  }, [filters.sort_by, filters.sort_dir, setFilters]);

  if (sessionQuery.isLoading) {
    return (
      <div className="min-h-screen bg-[var(--continua-app-bg)] flex items-center justify-center">
        <div className="text-[var(--continua-text-muted)]">Loading session...</div>
      </div>
    );
  }

  if (sessionQuery.error) {
    return (
      <div className="app-page max-w-4xl">
          {isAuthError(sessionQuery.error) ? (
            <AuthErrorBanner message={queryErrorMessage(sessionQuery.error)} />
          ) : (
            <div className="app-alert-error">
              Error loading session: {queryErrorMessage(sessionQuery.error)}
            </div>
          )}
      </div>
    );
  }

  if (!sessionQuery.data) {
    return (
      <div className="flex min-h-full items-center justify-center">
        <div className="text-[var(--continua-text-muted)]">Session not found</div>
      </div>
    );
  }

  const session = sessionQuery.data;
  const narrative = narrativeQuery.data;
  const narrativeTraces = [...(narrative?.traces ?? [])].sort(
    (a, b) => new Date(a.started_at).getTime() - new Date(b.started_at).getTime()
  );
  const visibleJourneyTraces =
    journeyStatusFilter === 'all'
      ? narrativeTraces
      : narrativeTraces.filter(
          (trace) => trace.status === journeyStatusFilter.toUpperCase()
        );
  const summary = narrative?.summary;
  const totalNarrativeTraces = summary?.total_trace_count ?? session.trace_count ?? total;
  const completedTraceCount = summary?.completed_trace_count ?? 0;
  const failedTraceCount = summary?.failed_trace_count ?? 0;
  const runningTraceCount = summary?.running_trace_count ?? 0;
  const successRate = totalNarrativeTraces
    ? Math.round((completedTraceCount / totalNarrativeTraces) * 100)
    : 0;
  const totalNarrativeTokens =
    (summary?.total_tokens_in ?? 0) + (summary?.total_tokens_out ?? 0);
  const durationValues = narrativeTraces
    .map((trace) => trace.duration_ms)
    .filter((value): value is number => typeof value === 'number');
  const averageDuration =
    durationValues.length > 0
      ? durationValues.reduce((sum, value) => sum + value, 0) / durationValues.length
      : undefined;
  const maxDuration =
    durationValues.length > 0 ? Math.max(...durationValues) : undefined;
  const totalErrors =
    narrativeTraces.reduce((sum, trace) => sum + (trace.error_count ?? 0), 0) ??
    0;

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <PageHeader
        actions={
          <>
            <Btn kind="secondary" leadingIcon={RefreshCw} size="sm" disabled>
              Replay session
            </Btn>
            <Btn
              kind="secondary"
              leadingIcon={Download}
              size="sm"
              onClick={handleExportSession}
            >
              Export
            </Btn>
            <Btn
              aria-label="Session actions"
              kind="secondary"
              leadingIcon={MoreHorizontal}
              size="sm"
            />
          </>
        }
        description={
          <span className="flex flex-wrap items-center gap-2 text-[12px]">
            <span className="font-mono text-[var(--c-text-secondary)]">
              {session.external_id}
            </span>
            <span className="h-1 w-1 rounded-full bg-[var(--c-text-muted)]" />
            <span>
              User{' '}
              <span className="font-mono text-[var(--c-text-secondary)]">
                {session.user_id || '-'}
              </span>
            </span>
            <span className="h-1 w-1 rounded-full bg-[var(--c-text-muted)]" />
            <span>Started {formatRelativeTime(session.created_at)}</span>
            {summary?.last_activity_at ? (
              <>
                <span className="h-1 w-1 rounded-full bg-[var(--c-text-muted)]" />
                <span>Last active {formatRelativeTime(summary.last_activity_at)}</span>
              </>
            ) : null}
          </span>
        }
        eyebrow={
          <span className="flex flex-wrap items-center gap-2">
            <Link to={returnTo} className="hover:text-[var(--c-text-primary)]">
              Sessions
            </Link>
            <span className="text-[var(--c-text-muted)]">/</span>
            <span className="font-mono">{session.id}</span>
            <CopyButton
              aria-label="Copy session ID"
              value={session.id}
              idleLabel="Copy"
              successLabel="Copied"
              className="!rounded-md !border-[var(--c-border)] !bg-[var(--c-surface)] !px-2 !py-0.5 !text-[11px] !text-[var(--c-text-secondary)]"
            />
          </span>
        }
        title={session.name || session.external_id}
      />

      <section className="grid border-b border-[var(--c-border)] bg-[var(--c-surface)] sm:grid-cols-2 lg:grid-cols-3 2xl:grid-cols-6">
        <SessionStatCard
          hint={`${completedTraceCount} ok · ${failedTraceCount} failed${
            runningTraceCount ? ` · ${runningTraceCount} live` : ''
          }`}
          label="Traces"
          value={String(totalNarrativeTraces)}
        />
        <SessionStatCard
          hint={successRate === 100 ? 'all clear' : `${failedTraceCount} failed`}
          label="Success rate"
          tone={successRate < 90 ? 'red' : successRate < 100 ? 'amber' : 'green'}
          value={`${successRate}%`}
        />
        <SessionStatCard
          hint={
            totalNarrativeTraces && totalNarrativeTokens
              ? `~${formatTokens(totalNarrativeTokens / totalNarrativeTraces)} avg / trace`
              : undefined
          }
          label="Total tokens"
          value={totalNarrativeTokens ? formatTokens(totalNarrativeTokens) : '-'}
        />
        <SessionStatCard
          hint={
            totalNarrativeTraces && summary?.total_cost_usd
              ? `${formatCost(summary.total_cost_usd / totalNarrativeTraces)} avg`
              : undefined
          }
          label="Total cost"
          value={formatCost(summary?.total_cost_usd)}
        />
        <SessionStatCard
          hint={maxDuration ? `peak ${formatDuration(maxDuration)}` : undefined}
          label="Avg duration"
          value={averageDuration ? formatDuration(averageDuration) : '-'}
        />
        <SessionStatCard
          hint={totalErrors > 0 ? 'across spans' : 'none'}
          label="Errors"
          tone={totalErrors > 0 ? 'red' : 'muted'}
          value={String(totalErrors)}
        />
      </section>

      {compareSelectionVisible ? (
        <CompareBar
          baseline={selectedBaseline}
          candidate={selectedCandidate}
          canOpenComparison={canOpenComparison}
          clearCompare={clearCompare}
          currentSessionDetailUrl={currentSessionDetailUrl}
          isBaselineLookupPending={isBaselineLookupPending}
          isCandidateLookupPending={isCandidateLookupPending}
          projectId={filters.project_id}
          sessionId={sessionId}
          swapCompare={swapCompare}
        />
      ) : null}

      <div className="flex border-b border-[var(--c-border)] px-6">
        {[
          { id: 'journey', label: 'Journey', count: totalNarrativeTraces },
          { id: 'traces', label: 'Traces', count: total },
          { id: 'context', label: 'Context' },
          { id: 'feedback', label: 'Feedback', count: getFeedbackItems(session).length },
        ].map((tab) => (
          <button
            key={tab.id}
            type="button"
            onClick={() => setActiveTab(tab.id as typeof activeTab)}
            className={`-mb-px border-b-2 px-3.5 py-2 text-[13px] font-medium ${
              activeTab === tab.id
                ? 'border-[var(--c-accent)] text-[var(--c-text-primary)]'
                : 'border-transparent text-[var(--c-text-secondary)]'
            }`}
          >
            {tab.label}
            {tab.count != null ? (
              <span className="ml-1.5 font-mono text-[11px] tabular-nums text-[var(--c-text-muted)]">
                {tab.count}
              </span>
            ) : null}
          </button>
        ))}
      </div>

      {narrativeQuery.error ? (
        <div className="border-b border-[var(--c-red-border)] bg-[var(--c-red-faint)] px-6 py-3 text-sm text-[var(--c-red-text)]">
          Error loading narrative: {queryErrorMessage(narrativeQuery.error)}
        </div>
      ) : null}

      {activeTab === 'journey' ? (
        <div className="flex min-h-0 flex-1">
          <div className="min-w-0 flex-1 overflow-y-auto px-6 py-5">
            <div className="mb-4 flex items-start justify-between gap-4">
              <div>
                <h2 className="text-[13px] font-semibold text-[var(--c-text-primary)]">
                  Session journey
                </h2>
                <p className="mt-0.5 text-[11.5px] text-[var(--c-text-muted)]">
                  Oldest first · click any step to open trace
                  {narrativeQuery.isFetching ? ' · updating' : ''}
                </p>
              </div>
              <div className="flex gap-1 rounded-md border border-[var(--c-border)] bg-[var(--c-surface-muted)] p-0.5">
                {[
                  { id: 'all', label: 'All' },
                  { id: 'completed', label: 'OK' },
                  { id: 'failed', label: 'Failed' },
                ].map((filter) => (
                  <button
                    key={filter.id}
                    type="button"
                    onClick={() =>
                      setJourneyStatusFilter(filter.id as typeof journeyStatusFilter)
                    }
                    className={`rounded px-2.5 py-1 text-[11.5px] font-medium ${
                      journeyStatusFilter === filter.id
                        ? 'border border-[var(--c-border)] bg-[var(--c-surface)] text-[var(--c-text-primary)]'
                        : 'border border-transparent text-[var(--c-text-secondary)]'
                    }`}
                  >
                    {filter.label}
                  </button>
                ))}
              </div>
            </div>

            {narrativeQuery.isPending && !narrative ? (
              <div className="app-empty-state">Loading session journey...</div>
            ) : visibleJourneyTraces.length === 0 ? (
              <div className="app-empty-state">No traces match this filter.</div>
            ) : (
              <SessionJourney
                assignCompareRole={assignCompareRole}
                compare={compare}
                projectId={filters.project_id}
                returnTo={currentSessionDetailUrl}
                setComparePair={setComparePair}
                traces={visibleJourneyTraces}
              />
            )}
          </div>

          <aside className="hidden w-80 shrink-0 overflow-y-auto border-l border-[var(--c-border)] lg:block">
            <SessionSidePanel
              completedTraceCount={completedTraceCount}
              failedTraceCount={failedTraceCount}
              runningTraceCount={runningTraceCount}
              session={session}
              summary={summary}
              totalTokens={totalNarrativeTokens}
              traces={narrativeTraces}
            />
          </aside>
        </div>
      ) : null}

      {activeTab === 'traces' ? (
        <div className="flex min-h-0 flex-1 flex-col">
          {tracesQuery.error ? (
            isAuthError(tracesQuery.error) ? (
              <AuthErrorBanner message={queryErrorMessage(tracesQuery.error)} />
            ) : (
              <div className="border-b border-[var(--c-red-border)] bg-[var(--c-red-faint)] px-6 py-3 text-sm text-[var(--c-red-text)]">
                Error loading traces: {queryErrorMessage(tracesQuery.error)}
              </div>
            )
          ) : null}

          {tracesQuery.isPending && !tracesQuery.data ? (
            <div className="app-empty-state">Loading traces...</div>
          ) : traces.length === 0 ? (
            <div className="app-empty-state">
              <h2 className="text-base font-semibold text-[var(--c-text-primary)]">
                No traces in this session
              </h2>
              <p className="mt-2">Traces will appear here as they are ingested.</p>
            </div>
          ) : (
            <>
              <DataTable>
                <colgroup>
                  <col className="w-[34%]" />
                  <col className="w-[110px]" />
                  <col className="w-[110px]" />
                  <col className="w-[90px]" />
                  <col className="w-[90px]" />
                  <col className="w-[70px]" />
                  <col className="w-[130px]" />
                  <col className="w-[150px]" />
                </colgroup>
                <thead>
                  <tr>
                    <Th>Trace</Th>
                    <Th>Status</Th>
                    <Th align="right">Duration</Th>
                    <Th align="right">Tokens</Th>
                    <Th align="right">Cost</Th>
                    <Th align="right">Errors</Th>
                    <Th
                      align="right"
                      sortable
                      sortActive={filters.sort_by === 'started_at'}
                      sortDir={filters.sort_dir}
                      onSort={handleStartedSortToggle}
                    >
                      Started
                    </Th>
                    <Th align="right">Compare</Th>
                  </tr>
                </thead>
                <tbody>
                  {traces.map((trace) => (
                    <SessionTraceRow
                      assignCompareRole={assignCompareRole}
                      compare={compare}
                      key={trace.id}
                      projectId={filters.project_id}
                      returnTo={currentSessionDetailUrl}
                      trace={trace}
                    />
                  ))}
                </tbody>
              </DataTable>
              <div className="border-t border-[var(--c-border)] px-6 py-2">
                <PaginationControls
                  offset={filters.offset}
                  pageSize={filters.limit ?? DEFAULT_PAGE_SIZE}
                  total={total}
                  currentItemCount={traces.length}
                  onOffsetChange={(offset) => setFilters({ offset }, 'push')}
                  onPageSizeChange={(limit) => setFilters({ limit }, 'push')}
                  onRepairOffset={(offset) => setFilters({ offset }, 'replace')}
                />
              </div>
            </>
          )}
        </div>
      ) : null}

      {activeTab === 'context' ? (
        <SessionContextPanel session={session} summary={summary} />
      ) : null}

      {activeTab === 'feedback' ? (
        <SessionFeedbackPanel session={session} traces={narrativeTraces} />
      ) : null}
    </div>
  );
}

function SessionJourney({
  assignCompareRole,
  compare,
  projectId,
  returnTo,
  setComparePair,
  traces,
}: {
  assignCompareRole: (role: CompareRole, traceId: string, mode?: HistoryMode) => void;
  compare: SessionDetailCompareState;
  projectId?: string;
  returnTo: string;
  setComparePair: (baselineTraceId: string, candidateTraceId: string, mode?: HistoryMode) => void;
  traces: SessionNarrativeTrace[];
}) {
  const traceByExternalId = new Map(
    traces.map((trace) => [trace.trace_id, trace] as const)
  );

  return (
    <div className="relative">
      <div className="absolute bottom-2 left-[14px] top-2 w-px bg-[var(--c-border)]" />
      <div className="flex flex-col gap-1.5">
        {traces.map((trace, index) => (
          <JourneyStep
            assignCompareRole={assignCompareRole}
            compare={compare}
            index={index + 1}
            key={trace.id}
            parentTrace={
              trace.lineage.parent_trace_id
                ? traceByExternalId.get(trace.lineage.parent_trace_id)
                : undefined
            }
            projectId={projectId}
            returnTo={returnTo}
            setComparePair={setComparePair}
            trace={trace}
          />
        ))}
      </div>
    </div>
  );
}

function JourneyStep({
  assignCompareRole,
  compare,
  index,
  parentTrace,
  projectId,
  returnTo,
  setComparePair,
  trace,
}: {
  assignCompareRole: (role: CompareRole, traceId: string, mode?: HistoryMode) => void;
  compare: SessionDetailCompareState;
  index: number;
  parentTrace?: SessionNarrativeTrace;
  projectId?: string;
  returnTo: string;
  setComparePair: (baselineTraceId: string, candidateTraceId: string, mode?: HistoryMode) => void;
  trace: SessionNarrativeTrace;
}) {
  const duration = formatDuration(trace.duration_ms);
  const totalTokens = (trace.total_tokens_in ?? 0) + (trace.total_tokens_out ?? 0);
  const semanticSnippet = getSemanticSnippet(trace);
  const isSelectable = isTerminalTraceStatus(trace.status);
  const canCompareToParent =
    parentTrace && isSelectable && isTerminalTraceStatus(parentTrace.status);
  const nodeTone =
    trace.status === 'FAILED'
      ? 'var(--c-red)'
      : trace.status === 'RUNNING'
        ? 'var(--c-blue)'
        : 'var(--c-green)';

  return (
    <div className="grid grid-cols-[30px_minmax(0,1fr)] gap-3 rounded-md py-2 pr-3 hover:bg-[var(--c-row-hover-bg)]">
      <div className="relative flex justify-center pt-0.5">
        <div
          className="relative z-[1] flex h-7 w-7 items-center justify-center rounded-full border bg-[var(--c-surface)] font-mono text-[10.5px] font-semibold"
          style={{ borderColor: nodeTone, color: nodeTone }}
        >
          {index}
          {trace.status === 'RUNNING' ? (
            <span
              className="absolute -inset-1 rounded-full"
              style={{
                animation: 'continua-pulse 1.6s ease-out infinite',
                border: `1.5px solid ${nodeTone}`,
                opacity: 0.35,
              }}
            />
          ) : null}
        </div>
      </div>
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2">
          <Link
            to={appendProjectToPath(`/traces/${trace.id}`, projectId)}
            state={{ returnTo }}
            className="font-mono text-[13px] font-semibold text-[var(--c-text-primary)] hover:text-[var(--c-accent-text)]"
          >
            {trace.name}
          </Link>
          <StatusDot status={trace.status} />
          <Chip tone="muted">{formatLineageLabel(trace.lineage.type)}</Chip>
          <span className="ml-auto font-mono text-[11px] tabular-nums text-[var(--c-text-muted)]">
            {formatRelativeTime(trace.started_at)}
          </span>
        </div>
        <div className="mt-1 flex flex-wrap items-center gap-3 font-mono text-[11.5px] tabular-nums text-[var(--c-text-muted)]">
          <span>
            dur <span className="font-medium text-[var(--c-text-secondary)]">{duration}</span>
          </span>
          <span>
            tok{' '}
            <span className="font-medium text-[var(--c-text-secondary)]">
              {totalTokens > 0 ? formatTokens(totalTokens) : '-'}
            </span>
          </span>
          <span>
            cost{' '}
            <span className="font-medium text-[var(--c-text-secondary)]">
              {formatCost(trace.total_cost_usd)}
            </span>
          </span>
          {trace.error_count ? (
            <span className="text-[var(--c-red-text)]">err {trace.error_count}</span>
          ) : null}
        </div>
        {semanticSnippet ? (
          <div className="mt-2 rounded border border-[var(--c-border)] bg-[var(--c-surface)] px-2.5 py-1.5 text-[12px] leading-5 text-[var(--c-text-secondary)]">
            {semanticSnippet}
          </div>
        ) : null}
        <div className="mt-2 flex flex-wrap gap-1.5">
          <CompareRoleButton
            disabled={!isSelectable}
            isSelected={compare.baseline_trace_id === trace.id}
            label="Set baseline"
            onClick={() => assignCompareRole('baseline', trace.id)}
            title={!isSelectable ? 'Trace must complete before it can be compared' : undefined}
          />
          <CompareRoleButton
            disabled={!isSelectable}
            isSelected={compare.candidate_trace_id === trace.id}
            label="Set candidate"
            onClick={() => assignCompareRole('candidate', trace.id)}
            title={!isSelectable ? 'Trace must complete before it can be compared' : undefined}
          />
          {canCompareToParent ? (
            <CompareRoleButton
              disabled={false}
              isSelected={false}
              label="Compare to parent"
              onClick={() => setComparePair(parentTrace.id, trace.id)}
            />
          ) : null}
        </div>
      </div>
    </div>
  );
}

function SessionStatCard({
  hint,
  label,
  tone,
  value,
}: {
  hint?: string;
  label: string;
  tone?: 'amber' | 'green' | 'muted' | 'red';
  value: string;
}) {
  const toneClass =
    tone === 'red'
      ? 'text-[var(--c-red-text)]'
      : tone === 'amber'
        ? 'text-[var(--c-amber-text)]'
        : tone === 'green'
          ? 'text-[var(--c-green-text)]'
          : 'text-[var(--c-text-primary)]';

  return (
    <div className="border-r border-[var(--c-border)] px-4 py-3">
      <div className="mb-1.5 text-[10.5px] font-semibold uppercase tracking-[0.05em] text-[var(--c-text-muted)]">
        {label}
      </div>
      <div className={`text-lg font-semibold leading-tight tracking-[-0.01em] tabular-nums ${toneClass}`}>
        {value}
      </div>
      {hint ? (
        <div className="mt-1 text-[11px] text-[var(--c-text-muted)]">{hint}</div>
      ) : null}
    </div>
  );
}

function SessionSidePanel({
  completedTraceCount,
  failedTraceCount,
  runningTraceCount,
  session,
  summary,
  totalTokens,
  traces,
}: {
  completedTraceCount: number;
  failedTraceCount: number;
  runningTraceCount: number;
  session: Session;
  summary?: SessionNarrativeSummary;
  totalTokens: number;
  traces: SessionNarrativeTrace[];
}) {
  return (
    <div className="flex flex-col gap-5 px-5 py-4">
      <SidePanelSection title="Identity">
        <SidePanelRow label="Session ID" mono value={session.id} />
        <SidePanelRow label="External" mono value={session.external_id} />
        <SidePanelRow label="User" mono value={session.user_id || '-'} />
      </SidePanelSection>
      <SidePanelSection title="Engagement">
        <SidePanelRow label="Started" value={formatRelativeTime(session.created_at)} />
        <SidePanelRow
          label="Last active"
          value={formatRelativeTime(summary?.last_activity_at)}
        />
        <SidePanelRow label="Touchpoints" mono value={String(summary?.total_trace_count ?? session.trace_count ?? 0)} />
      </SidePanelSection>
      <SidePanelSection title="Trace status">
        <UsageBar
          segments={[
            { color: 'var(--c-green)', label: 'OK', value: completedTraceCount },
            { color: 'var(--c-blue)', label: 'Live', value: runningTraceCount },
            { color: 'var(--c-red)', label: 'Failed', value: failedTraceCount },
          ]}
        />
      </SidePanelSection>
      <SidePanelSection title="Usage">
        <SidePanelRow label="Tokens" mono value={formatTokens(totalTokens)} />
        <SidePanelRow label="Cost" mono value={formatCost(summary?.total_cost_usd)} />
      </SidePanelSection>
      <SidePanelSection title="Latency by trace">
        <LatencyBars traces={traces} />
      </SidePanelSection>
      <SidePanelSection title="Linked">
        <SidePanelLink icon={User} label="User profile" sub={session.user_id || 'No user ID'} />
        <SidePanelLink icon={Zap} label="Trace lineage" sub={`${summary?.explicit_link_count ?? 0} explicit links`} />
        <SidePanelLink icon={ExternalLink} label="Workspace" sub="Current project" />
      </SidePanelSection>
    </div>
  );
}

function SessionContextPanel({
  session,
  summary,
}: {
  session: Session;
  summary?: SessionNarrativeSummary;
}) {
  return (
    <div className="max-w-3xl px-6 py-5">
      <ContextSection title="User and session">
        <ContextRow label="Session ID" mono value={session.id} />
        <ContextRow label="External ID" mono value={session.external_id} />
        <ContextRow label="User ID" mono value={session.user_id || '-'} />
        <ContextRow label="Created" value={formatRelativeTime(session.created_at)} />
        <ContextRow
          label="Last activity"
          value={formatRelativeTime(summary?.last_activity_at)}
        />
      </ContextSection>
      <ContextSection title="Metadata">
        {session.metadata && Object.keys(session.metadata).length > 0 ? (
          Object.entries(session.metadata).map(([key, value]) => (
            <ContextRow
              key={key}
              label={key}
              mono={typeof value !== 'object'}
              value={
                typeof value === 'string'
                  ? value
                  : JSON.stringify(value, null, 2)
              }
            />
          ))
        ) : (
          <div className="text-[12.5px] text-[var(--c-text-muted)]">
            No session metadata.
          </div>
        )}
      </ContextSection>
    </div>
  );
}

interface FeedbackItem {
  score?: string | number;
  text: string;
  trace?: string;
  user?: string;
  when?: string;
}

function SessionFeedbackPanel({
  session,
  traces,
}: {
  session: Session;
  traces: SessionNarrativeTrace[];
}) {
  const feedbackItems = getFeedbackItems(session);

  if (feedbackItems.length === 0) {
    return (
      <div className="max-w-3xl px-6 py-5">
        <ContextSection title="Feedback">
          <div className="rounded-md border border-dashed border-[var(--c-border)] bg-[var(--c-surface)] px-4 py-8 text-center text-[12.5px] text-[var(--c-text-muted)]">
            No feedback has been attached to this session.
          </div>
        </ContextSection>
        <ContextSection title="Trace signals">
          <div className="flex flex-col gap-2">
            {traces.slice(0, 6).map((trace) => (
              <div
                key={trace.id}
                className="flex items-center justify-between gap-3 rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-3 py-2 text-[12.5px]"
              >
                <span className="min-w-0 truncate font-mono font-medium text-[var(--c-text-primary)]">
                  {trace.name}
                </span>
                <StatusDot status={trace.status} />
              </div>
            ))}
          </div>
        </ContextSection>
      </div>
    );
  }

  return (
    <div className="max-w-3xl px-6 py-5">
      <ContextSection title="Feedback">
        <div className="flex flex-col gap-2.5">
          {feedbackItems.map((item, index) => (
            <div
              key={`${item.text}-${index}`}
              className="rounded-lg border border-[var(--c-border)] bg-[var(--c-surface)] px-3.5 py-3"
            >
              <div className="mb-1.5 flex items-center gap-2 text-xs">
                {item.score != null ? (
                  <Chip tone={isPositiveFeedback(item.score) ? 'success' : 'error'}>
                    {String(item.score)}
                  </Chip>
                ) : null}
                <span className="font-mono text-[var(--c-text-secondary)]">
                  {item.user || session.user_id || 'anonymous'}
                </span>
                {item.when ? (
                  <>
                    <span className="text-[var(--c-text-muted)]">/</span>
                    <span className="text-[var(--c-text-muted)]">{item.when}</span>
                  </>
                ) : null}
                {item.trace ? (
                  <span className="ml-auto truncate font-mono text-[11px] text-[var(--c-accent-text)]">
                    {item.trace}
                  </span>
                ) : null}
              </div>
              <div className="text-[12.5px] leading-5 text-[var(--c-text-primary)]">
                {item.text}
              </div>
            </div>
          ))}
        </div>
      </ContextSection>
    </div>
  );
}

function getFeedbackItems(session: Session): FeedbackItem[] {
  const metadata = session.metadata;
  if (!metadata) {
    return [];
  }

  const rawFeedback =
    metadata.feedback ??
    metadata.feedback_items ??
    metadata.ratings ??
    metadata.rating;

  if (Array.isArray(rawFeedback)) {
    return rawFeedback
      .map(normalizeFeedbackItem)
      .filter((item): item is FeedbackItem => item !== null);
  }

  const normalizedSingle = normalizeFeedbackItem(rawFeedback);
  if (normalizedSingle) {
    return [normalizedSingle];
  }

  if (typeof metadata.feedback_text === 'string') {
    return [
      {
        score:
          typeof metadata.feedback_score === 'string' ||
          typeof metadata.feedback_score === 'number'
            ? metadata.feedback_score
            : undefined,
        text: metadata.feedback_text,
      },
    ];
  }

  return [];
}

function normalizeFeedbackItem(value: unknown): FeedbackItem | null {
  if (typeof value === 'string') {
    return { text: value };
  }

  if (!value || typeof value !== 'object') {
    return null;
  }

  const record = value as Record<string, unknown>;
  const rawText = record.text ?? record.comment ?? record.message ?? record.feedback;
  if (typeof rawText !== 'string' || rawText.trim() === '') {
    return null;
  }

  return {
    score:
      typeof record.score === 'string' || typeof record.score === 'number'
        ? record.score
        : typeof record.rating === 'string' || typeof record.rating === 'number'
          ? record.rating
          : undefined,
    text: rawText,
    trace: typeof record.trace === 'string' ? record.trace : undefined,
    user: typeof record.user === 'string' ? record.user : undefined,
    when: typeof record.when === 'string' ? record.when : undefined,
  };
}

function isPositiveFeedback(score: string | number): boolean {
  if (typeof score === 'number') {
    return score >= 0;
  }
  return !/negative|bad|fail|thumbs\s*down|down/i.test(score);
}

function ContextSection({
  children,
  title,
}: {
  children: ReactNode;
  title: string;
}) {
  return (
    <section className="mb-6 border-b border-[var(--c-border)] pb-6">
      <div className="mb-3 text-[10.5px] font-semibold uppercase tracking-[0.06em] text-[var(--c-text-muted)]">
        {title}
      </div>
      <div className="flex flex-col gap-1">{children}</div>
    </section>
  );
}

function ContextRow({
  label,
  mono = false,
  value,
}: {
  label: string;
  mono?: boolean;
  value: string;
}) {
  return (
    <div className="grid grid-cols-[160px_minmax(0,1fr)] items-baseline gap-4 border-b border-[var(--c-border-subtle)] py-1.5 text-[12.5px]">
      <span className="font-medium text-[var(--c-text-secondary)]">{label}</span>
      <span
        className={`min-w-0 whitespace-pre-wrap break-words text-[var(--c-text-primary)] ${
          mono ? 'font-mono' : ''
        }`}
      >
        {value}
      </span>
    </div>
  );
}

function SidePanelSection({
  children,
  title,
}: {
  children: ReactNode;
  title: string;
}) {
  return (
    <section>
      <div className="mb-2.5 text-[10.5px] font-semibold uppercase tracking-[0.06em] text-[var(--c-text-muted)]">
        {title}
      </div>
      <div className="flex flex-col gap-1.5">{children}</div>
    </section>
  );
}

function SidePanelRow({
  label,
  mono = false,
  value,
}: {
  label: string;
  mono?: boolean;
  value: string;
}) {
  return (
    <div className="grid grid-cols-[90px_minmax(0,1fr)] gap-2 text-[11.5px]">
      <span className="text-[var(--c-text-muted)]">{label}</span>
      <span className={`truncate text-[var(--c-text-primary)] ${mono ? 'font-mono' : ''}`}>
        {value}
      </span>
    </div>
  );
}

function UsageBar({
  segments,
}: {
  segments: Array<{ color: string; label: string; value: number }>;
}) {
  const total = Math.max(
    segments.reduce((sum, segment) => sum + segment.value, 0),
    1
  );

  return (
    <div>
      <div className="flex h-2 overflow-hidden rounded-full bg-[var(--c-surface-muted)]">
        {segments.map((segment) => (
          <div
            key={segment.label}
            style={{
              background: segment.color,
              width: `${(segment.value / total) * 100}%`,
            }}
          />
        ))}
      </div>
      <div className="mt-2 flex justify-between gap-2 text-[11.5px]">
        {segments.map((segment) => (
          <span key={segment.label} className="inline-flex items-center gap-1.5">
            <span
              className="h-1.5 w-1.5 rounded-sm"
              style={{ background: segment.color }}
            />
            <span className="text-[var(--c-text-secondary)]">{segment.label}</span>
            <span className="font-mono text-[var(--c-text-muted)]">
              {segment.value}
            </span>
          </span>
        ))}
      </div>
    </div>
  );
}

function LatencyBars({ traces }: { traces: SessionNarrativeTrace[] }) {
  const durations = traces
    .map((trace) => trace.duration_ms)
    .filter((duration): duration is number => typeof duration === 'number' && duration > 0);
  const maxDuration = Math.max(...durations, 1);
  const minDuration = durations.length > 0 ? Math.min(...durations) : undefined;
  const p95Duration = percentile(durations, 0.95);

  if (durations.length === 0) {
    return (
      <div className="text-[11.5px] text-[var(--c-text-muted)]">
        No completed trace latency yet.
      </div>
    );
  }

  return (
    <div>
      <div className="flex h-9 items-end gap-[3px]">
        {durations.slice(0, 18).map((duration, index) => (
          <div
            key={`${duration}-${index}`}
            className="min-h-0.5 flex-1 rounded-[1px]"
            style={{
              background:
                duration > maxDuration * 0.7
                  ? 'var(--c-amber)'
                  : 'var(--c-accent)',
              height: `${Math.max(6, (duration / maxDuration) * 100)}%`,
              opacity: 0.85,
            }}
            title={formatDuration(duration)}
          />
        ))}
      </div>
      <div className="mt-1.5 flex justify-between font-mono text-[10.5px] text-[var(--c-text-muted)]">
        <span>min {formatDuration(minDuration)}</span>
        <span>p95 {formatDuration(p95Duration)}</span>
        <span>max {formatDuration(maxDuration)}</span>
      </div>
    </div>
  );
}

function percentile(values: number[], p: number): number | undefined {
  if (values.length === 0) {
    return undefined;
  }
  const sorted = [...values].sort((a, b) => a - b);
  const index = Math.min(sorted.length - 1, Math.ceil(sorted.length * p) - 1);
  return sorted[index];
}

function SidePanelLink({
  icon: Icon,
  label,
  sub,
}: {
  icon: LucideIcon;
  label: string;
  sub: string;
}) {
  return (
    <div className="grid grid-cols-[20px_minmax(0,1fr)_12px] items-center gap-2.5 rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-2.5 py-2">
      <Icon className="h-3.5 w-3.5 text-[var(--c-text-secondary)]" />
      <div className="min-w-0">
        <div className="truncate text-xs font-medium text-[var(--c-text-primary)]">
          {label}
        </div>
        <div className="truncate font-mono text-[11px] text-[var(--c-text-muted)]">
          {sub}
        </div>
      </div>
      <ExternalLink className="h-3 w-3 text-[var(--c-text-muted)]" />
    </div>
  );
}

function CompareBar({
  baseline,
  candidate,
  canOpenComparison,
  clearCompare,
  currentSessionDetailUrl,
  isBaselineLookupPending,
  isCandidateLookupPending,
  projectId,
  sessionId,
  swapCompare,
}: {
  baseline?: CompareSelectedTrace;
  candidate?: CompareSelectedTrace;
  canOpenComparison: boolean;
  clearCompare: (mode?: HistoryMode) => void;
  currentSessionDetailUrl: string;
  isBaselineLookupPending: boolean;
  isCandidateLookupPending: boolean;
  projectId?: string;
  sessionId: string;
  swapCompare: (mode?: HistoryMode) => void;
}) {
  const compareSearch = buildCompareSearchParams(
    projectId,
    baseline?.id,
    candidate?.id
  ).toString();
  const compareHref = compareSearch
    ? `/sessions/${sessionId}/compare?${compareSearch}`
    : `/sessions/${sessionId}/compare`;
  const hasRunningSelection =
    (baseline && !isTerminalTraceStatus(baseline.status)) ||
    (candidate && !isTerminalTraceStatus(candidate.status));

  return (
    <section className="sticky top-11 z-20 border-b border-[var(--c-border)] bg-[var(--c-surface)] px-6 py-3">
      <div className="flex flex-col gap-3 xl:flex-row xl:items-end xl:justify-between">
        <div className="grid gap-2 sm:grid-cols-2 xl:w-[40rem]">
          <CompareSelectionCard
            isPending={isBaselineLookupPending}
            label="Baseline"
            trace={baseline}
          />
          <CompareSelectionCard
            isPending={isCandidateLookupPending}
            label="Candidate"
            trace={candidate}
          />
        </div>

        <div className="flex flex-wrap gap-1.5">
          <Btn kind="ghost" size="sm" type="button" onClick={() => swapCompare()}>
            Swap
          </Btn>
          <Btn kind="ghost" size="sm" type="button" onClick={() => clearCompare()}>
            Clear
          </Btn>
          {canOpenComparison ? (
            <Link
              to={compareHref}
              state={{ returnTo: currentSessionDetailUrl }}
              className="inline-flex h-7 items-center justify-center rounded-md border border-transparent bg-[var(--c-text-primary)] px-2.5 text-xs font-semibold text-[var(--c-app-bg)]"
            >
              Open comparison
            </Link>
          ) : (
            <Btn
              disabled
              kind="secondary"
              size="sm"
              title={
                hasRunningSelection
                  ? 'Both selected traces must be terminal before comparison can open'
                  : 'Both selections must resolve before comparison can open'
              }
            >
              Open comparison
            </Btn>
          )}
        </div>
      </div>

      {(isBaselineLookupPending || isCandidateLookupPending || hasRunningSelection) ? (
        <p className="mt-2 text-[12.5px] text-[var(--c-text-secondary)]">
          {isBaselineLookupPending || isCandidateLookupPending
            ? 'Resolving selected trace details...'
            : 'Running traces stay visible here, but comparison remains disabled until they finish.'}
        </p>
      ) : null}
    </section>
  );
}

function CompareSelectionCard({
  label,
  trace,
  isPending,
}: {
  label: string;
  trace?: CompareSelectedTrace;
  isPending: boolean;
}) {
  return (
    <div className="rounded-md border border-[var(--c-border)] bg-[var(--c-app-bg)] p-3">
      <p className="text-[10.5px] font-semibold uppercase tracking-[0.06em] text-[var(--c-text-muted)]">
        {label}
      </p>
      {isPending ? (
        <p className="mt-1.5 text-[12.5px] text-[var(--c-text-secondary)]">Loading trace...</p>
      ) : trace ? (
        <div className="mt-1.5">
          <div className="flex flex-wrap items-center gap-2">
            <p className="font-mono text-[12.5px] font-semibold text-[var(--c-text-primary)]">{trace.name}</p>
            <StatusDot status={trace.status} />
          </div>
          {trace.trace_id ? (
            <p className="mt-1 font-mono text-[11px] text-[var(--c-text-muted)]">{trace.trace_id}</p>
          ) : null}
        </div>
      ) : (
        <p className="mt-1.5 text-[12.5px] text-[var(--c-text-secondary)]">No trace selected</p>
      )}
    </div>
  );
}

function CompareRoleButton({
  label,
  disabled,
  isSelected,
  onClick,
  title,
}: {
  label: string;
  disabled: boolean;
  isSelected: boolean;
  onClick: () => void;
  title?: string;
}) {
  return (
    <button
      type="button"
      disabled={disabled}
      onClick={onClick}
      title={title}
      className={`rounded border px-2 py-1 text-[11.5px] font-medium transition ${
        disabled
          ? 'cursor-not-allowed border-[var(--c-border)] text-[var(--c-text-muted)]'
          : isSelected
            ? 'border-[var(--c-accent-border)] bg-[var(--c-accent-faint)] text-[var(--c-accent-text)]'
            : 'border-[var(--c-border)] bg-[var(--c-surface)] text-[var(--c-text-secondary)] hover:border-[var(--c-border-strong)] hover:text-[var(--c-text-primary)]'
      }`}
    >
      {label}
    </button>
  );
}

function formatLineageLabel(type: SessionNarrativeTrace['lineage']['type']): string {
  switch (type) {
    case 'explicit':
      return 'Explicit';
    case 'inferred':
      return 'Inferred';
    default:
      return 'Unlinked';
  }
}

function getSemanticSnippet(trace: SessionNarrativeTrace): string | null {
  const latestEvent = trace.semantic_events.at(-1);
  if (!latestEvent?.event_type) {
    return null;
  }

  return summarizeTimelineEvent(latestEvent);
}

function SessionTraceRow({
  compare,
  assignCompareRole,
  projectId,
  trace,
  returnTo,
}: {
  compare: SessionDetailCompareState;
  assignCompareRole: (role: CompareRole, traceId: string, mode?: HistoryMode) => void;
  projectId?: string;
  trace: Trace;
  returnTo: string;
}) {
  const duration = calculateDuration(trace.started_at, trace.ended_at);
  const totalTokens = (trace.total_tokens_in ?? 0) + (trace.total_tokens_out ?? 0);
  const isSelectable = isTerminalTraceStatus(trace.status);

  return (
    <Tr>
      <Td>
        <div className="flex min-w-0 flex-col gap-1">
          <Link
            to={appendProjectToPath(`/traces/${trace.id}`, projectId)}
            state={{ returnTo }}
            className="truncate font-mono text-[12.5px] font-medium text-[var(--c-text-primary)] hover:text-[var(--c-accent-text)]"
          >
            {trace.name}
          </Link>
          <div className="flex min-w-0 items-center gap-1.5">
            <span className="truncate font-mono text-[10.5px] text-[var(--c-text-muted)]">
              {trace.id}
            </span>
            {trace.engine ? (
              <Chip icon={Zap}>{trace.engine.definition_name}</Chip>
            ) : null}
          </div>
        </div>
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
      <Td align="right" mono className={(trace.error_count ?? 0) > 0 ? 'text-[var(--c-red-text)]' : ''}>
        {trace.error_count ?? 0}
      </Td>
      <Td align="right" dim>
        {formatRelativeTime(trace.started_at)}
      </Td>
      <Td className="w-[150px]">
        <div className="flex justify-end gap-1">
          <CompareRoleButton
            disabled={!isSelectable}
            isSelected={compare.baseline_trace_id === trace.id}
            label="Base"
            onClick={() => assignCompareRole('baseline', trace.id)}
            title={!isSelectable ? 'Trace must complete before it can be compared' : undefined}
          />
          <CompareRoleButton
            disabled={!isSelectable}
            isSelected={compare.candidate_trace_id === trace.id}
            label="Cand"
            onClick={() => assignCompareRole('candidate', trace.id)}
            title={!isSelectable ? 'Trace must complete before it can be compared' : undefined}
          />
        </div>
      </Td>
    </Tr>
  );
}
