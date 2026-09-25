import { keepPreviousData, useQuery } from '@tanstack/react-query';
import { useCallback, useEffect, useState, type ReactNode } from 'react';
import { Link, useLocation, useParams, useSearchParams } from 'react-router-dom';
import { ArrowLeft, ArrowRight, Download, GitCompare, Zap } from 'lucide-react';
import { AuthErrorBanner } from '../components/AuthErrorBanner';
import { PaginationControls } from '../components/PaginationControls';
import {
  DerivedTag,
  HonestyNote,
  ReadOnlyBadge,
  TruncatedNotice,
} from '../components/DataState';
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
  formatDerivedDuration,
  formatExactTime,
  formatTokens,
} from '../utils/format';
import { summarizeTimelineEvent } from '../utils/timeline';
import {
  buildCompareSearchParams,
  normalizeCompareTraceIdParam,
} from './sessionCompareUtils';
import { downloadJsonFile } from '../utils/downloadJson';
import { SessionDetailsDrawer } from './session/SessionDetailsDrawer';
import { isUsageUnverified, shortId } from './session/sessionDisplay';

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

type SessionView = 'journey' | 'table';

interface SessionDisplayName {
  name: string;
  source: 'recorded' | 'trace' | 'external_id';
}

/** The recorded name, else the oldest trace name, else the external ID. */
function resolveSessionDisplayName(
  session: Session,
  narrativeTraces: SessionNarrativeTrace[]
): SessionDisplayName {
  if (session.name) {
    return { name: session.name, source: 'recorded' };
  }
  const firstTrace = narrativeTraces[0];
  if (firstTrace?.name) {
    return { name: firstTrace.name, source: 'trace' };
  }
  return { name: session.external_id, source: 'external_id' };
}

function deriveSessionStatus(summary?: SessionNarrativeSummary): string | undefined {
  if (!summary || summary.total_trace_count === 0) {
    return undefined;
  }
  if (summary.running_trace_count > 0) {
    return 'RUNNING';
  }
  return summary.failed_trace_count > 0 ? 'FAILED' : 'COMPLETED';
}

function deriveSessionDurationMs(summary?: SessionNarrativeSummary): number | undefined {
  if (!summary?.started_at || !summary.last_activity_at) {
    return undefined;
  }
  const duration =
    new Date(summary.last_activity_at).getTime() - new Date(summary.started_at).getTime();
  return Number.isFinite(duration) && duration >= 0 ? duration : undefined;
}

type HeaderCompareTarget =
  | { kind: 'link'; baselineId: string; candidateId: string; title: string }
  | { kind: 'disabled'; reason: string };

/**
 * The header Compare action. It opens the selected pair when one is ready,
 * otherwise the two most recent finished traces. It states why it is off.
 */
function resolveHeaderCompareTarget({
  canOpenSelectedPair,
  narrativeLoaded,
  narrativeTraces,
  selectedBaselineId,
  selectedCandidateId,
  totalTraceCount,
}: {
  canOpenSelectedPair: boolean;
  narrativeLoaded: boolean;
  narrativeTraces: SessionNarrativeTrace[];
  selectedBaselineId?: string;
  selectedCandidateId?: string;
  totalTraceCount: number;
}): HeaderCompareTarget {
  if (canOpenSelectedPair && selectedBaselineId && selectedCandidateId) {
    return {
      kind: 'link',
      baselineId: selectedBaselineId,
      candidateId: selectedCandidateId,
      title: 'Compare the selected baseline and candidate traces',
    };
  }
  if (!narrativeLoaded) {
    return { kind: 'disabled', reason: 'Loading session traces' };
  }
  if (totalTraceCount < 2) {
    return {
      kind: 'disabled',
      reason:
        totalTraceCount === 1
          ? 'Needs two traces — this session has one'
          : 'Needs two traces — this session has none',
    };
  }
  const finished = narrativeTraces.filter((trace) => isTerminalTraceStatus(trace.status));
  if (finished.length < 2) {
    return { kind: 'disabled', reason: 'Needs two finished traces' };
  }
  const baseline = finished[finished.length - 2];
  const candidate = finished[finished.length - 1];
  return {
    kind: 'link',
    baselineId: baseline.id,
    candidateId: candidate.id,
    title: `Compare the two most recent finished traces: ${baseline.name} → ${candidate.name}`,
  };
}

export function SessionDetailPage() {
  const { id } = useParams<{ id: string }>();

  if (!id) {
    return (
      <div className="flex min-h-full items-center justify-center">
        <div className="text-[var(--c-red-text)]">Session ID is required</div>
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
  const [view, setView] = useState<SessionView>('journey');
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
      <div className="flex min-h-full items-center justify-center">
        <div className="text-[var(--c-text-muted)]">Loading session...</div>
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
        <div className="text-[var(--c-text-muted)]">Session not found</div>
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
  const runningTraceCount = summary?.running_trace_count ?? 0;
  const displayName = resolveSessionDisplayName(session, narrativeTraces);
  const sessionStatus = deriveSessionStatus(summary);
  const sessionDurationMs = deriveSessionDurationMs(summary);
  const totalErrors = narrativeTraces.reduce((sum, trace) => sum + (trace.error_count ?? 0), 0);
  const usageUnverified = isUsageUnverified([
    summary?.total_tokens_in,
    summary?.total_tokens_out,
    summary?.total_cost_usd,
  ]);
  const headerCompare = resolveHeaderCompareTarget({
    canOpenSelectedPair: canOpenComparison,
    narrativeLoaded: Boolean(narrative),
    narrativeTraces,
    selectedBaselineId: selectedBaseline?.id,
    selectedCandidateId: selectedCandidate?.id,
    totalTraceCount: totalNarrativeTraces,
  });
  const headerCompareSearch =
    headerCompare.kind === 'link'
      ? buildCompareSearchParams(
          filters.project_id,
          headerCompare.baselineId,
          headerCompare.candidateId
        ).toString()
      : '';

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <PageHeader
        eyebrow={
          <Link
            to={returnTo}
            className="inline-flex items-center gap-1 hover:text-[var(--c-text-primary)]"
          >
            <ArrowLeft className="h-3 w-3" />
            Sessions
          </Link>
        }
        title={displayName.name}
        description={
          <span className="flex flex-wrap items-center gap-2 text-[12px]">
            {displayName.source === 'trace' ? (
              <DerivedTag
                label="name derived from its trace"
                title="The session has no recorded name. This is the name of its first trace."
              />
            ) : null}
            {displayName.source === 'external_id' ? (
              <DerivedTag
                label="no name · external ID"
                title="The session has no recorded name and no trace names yet."
              />
            ) : null}
            {sessionStatus ? <StatusDot status={sessionStatus} /> : null}
            <span className="font-mono text-[var(--c-text-secondary)]" title={session.id}>
              {shortId(session.id)}
            </span>
            <ReadOnlyBadge />
          </span>
        }
        actions={
          <>
            <Btn kind="secondary" leadingIcon={Download} size="sm" onClick={handleExportSession}>
              Export
            </Btn>
            {headerCompare.kind === 'link' ? (
              <Link
                to={`/sessions/${sessionId}/compare${headerCompareSearch ? `?${headerCompareSearch}` : ''}`}
                state={{ returnTo: currentSessionDetailUrl }}
                title={headerCompare.title}
                className="inline-flex h-7 items-center gap-1.5 rounded-md border border-transparent bg-[var(--c-text-primary)] px-2.5 text-xs font-semibold text-[var(--c-app-bg)]"
              >
                <GitCompare className="h-3.5 w-3.5" />
                Compare
              </Link>
            ) : (
              <span className="flex flex-col items-end gap-0.5">
                <Btn
                  disabled
                  kind="secondary"
                  leadingIcon={GitCompare}
                  size="sm"
                  title={headerCompare.reason}
                >
                  Compare
                </Btn>
                <span className="text-[11px] text-[var(--c-text-muted)]">
                  {headerCompare.reason}
                </span>
              </span>
            )}
          </>
        }
      />

      <section
        aria-label="Session summary"
        className="grid grid-cols-2 border-b border-[var(--c-border)] bg-[var(--c-surface)] sm:grid-cols-3 lg:grid-cols-5"
      >
        <SummaryCell
          label="Traces"
          value={String(totalNarrativeTraces)}
          hint={runningTraceCount > 0 ? `${runningTraceCount} running` : undefined}
        />
        <SummaryCell
          label="Duration"
          value={formatDerivedDuration(sessionDurationMs)}
          tag={
            sessionDurationMs != null ? (
              <DerivedTag title="From the first trace start to the last recorded activity." />
            ) : undefined
          }
        />
        <SummaryCell
          label="Errors"
          value={String(totalErrors)}
          suffix="recorded"
          tone={totalErrors > 0 ? 'red' : undefined}
          hint={
            summary?.truncated
              ? `across the ${summary.returned_trace_count} loaded traces`
              : undefined
          }
        />
        {usageUnverified ? (
          <SummaryCell
            label="Usage"
            value="Not verified"
            tone="amber"
            hint="token and cost fields read 0"
          />
        ) : (
          <SummaryCell
            label="Usage"
            value={`${formatTokens(
              (summary?.total_tokens_in ?? 0) + (summary?.total_tokens_out ?? 0)
            )} tok`}
            hint={`${formatCost(summary?.total_cost_usd)} recorded cost`}
          />
        )}
        <SummaryCell
          label="User"
          value={session.user_id ?? 'Not recorded'}
          mono={Boolean(session.user_id)}
          tone={session.user_id ? undefined : 'amber'}
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

      <div className="flex min-h-0 flex-1 flex-col lg:flex-row">
        <div className="min-w-0 flex-1 overflow-y-auto">
          <div className="flex flex-wrap items-center justify-between gap-3 px-6 pb-3 pt-4">
            <div className="flex items-center gap-3">
              <div className="flex gap-0.5 rounded-md border border-[var(--c-border)] bg-[var(--c-surface-muted)] p-0.5">
                <ViewToggleButton active={view === 'journey'} onClick={() => setView('journey')}>
                  Journey
                </ViewToggleButton>
                <ViewToggleButton active={view === 'table'} onClick={() => setView('table')}>
                  Table
                </ViewToggleButton>
              </div>
              <span className="text-[11.5px] text-[var(--c-text-muted)]">
                same collection, two densities
              </span>
            </div>
            {view === 'journey' ? (
              <div className="flex gap-0.5 rounded-md border border-[var(--c-border)] bg-[var(--c-surface-muted)] p-0.5">
                {[
                  { id: 'all', label: 'All' },
                  { id: 'completed', label: 'OK' },
                  { id: 'failed', label: 'Failed' },
                ].map((filter) => (
                  <ViewToggleButton
                    key={filter.id}
                    active={journeyStatusFilter === filter.id}
                    onClick={() =>
                      setJourneyStatusFilter(filter.id as typeof journeyStatusFilter)
                    }
                  >
                    {filter.label}
                  </ViewToggleButton>
                ))}
              </div>
            ) : null}
          </div>

          {narrativeQuery.error ? (
            <div className="mx-6 mb-3 rounded-md border border-[var(--c-red-border)] bg-[var(--c-red-faint)] px-3 py-2 text-sm text-[var(--c-red-text)]">
              Error loading narrative: {queryErrorMessage(narrativeQuery.error)}
            </div>
          ) : null}

          {view === 'journey' ? (
            <div className="px-6 pb-6">
              <div className="mb-3 flex items-baseline gap-2">
                <h2 className="text-[13px] font-semibold text-[var(--c-text-primary)]">
                  Session journey
                </h2>
                <span className="text-[11.5px] text-[var(--c-text-muted)]">
                  oldest first{narrativeQuery.isFetching ? ' · updating' : ''}
                </span>
              </div>
              {summary?.truncated ? (
                <div className="mb-3">
                  <TruncatedNotice>
                    Showing the first {summary.returned_trace_count} of{' '}
                    {summary.total_trace_count} traces. Table view lists every trace.
                  </TruncatedNotice>
                </div>
              ) : null}
              {narrativeQuery.isPending && !narrative ? (
                <div className="app-empty-state">Loading session journey...</div>
              ) : visibleJourneyTraces.length === 0 ? (
                <div className="app-empty-state">No traces match this filter.</div>
              ) : (
                <SessionJourney
                  assignCompareRole={assignCompareRole}
                  compare={compare}
                  isOnlyTrace={narrativeTraces.length === 1}
                  projectId={filters.project_id}
                  returnTo={currentSessionDetailUrl}
                  setComparePair={setComparePair}
                  traces={visibleJourneyTraces}
                />
              )}
              {narrative ? <LineageNote summary={narrative.summary} /> : null}
            </div>
          ) : (
            <div className="flex min-h-0 flex-col">
              <h2 className="px-6 pb-3 text-[13px] font-semibold text-[var(--c-text-primary)]">
                Session traces
              </h2>
              {tracesQuery.error ? (
                isAuthError(tracesQuery.error) ? (
                  <AuthErrorBanner message={queryErrorMessage(tracesQuery.error)} />
                ) : (
                  <div className="mx-6 mb-3 rounded-md border border-[var(--c-red-border)] bg-[var(--c-red-faint)] px-3 py-2 text-sm text-[var(--c-red-text)]">
                    Error loading traces: {queryErrorMessage(tracesQuery.error)}
                  </div>
                )
              ) : null}

              {tracesQuery.isPending && !tracesQuery.data ? (
                <div className="app-empty-state">Loading traces...</div>
              ) : traces.length === 0 ? (
                <div className="app-empty-state">
                  <h3 className="text-base font-semibold text-[var(--c-text-primary)]">
                    No traces in this session
                  </h3>
                  <p className="mt-2">Traces will appear here as they are ingested.</p>
                </div>
              ) : (
                <>
                  <DataTable>
                    <colgroup>
                      <col className="w-[34%]" />
                      <col className="w-[110px]" />
                      <col className="w-[100px]" />
                      <col className="w-[90px]" />
                      <col className="w-[90px]" />
                      <col className="w-[70px]" />
                      <col className="w-[120px]" />
                      <col className="w-[130px]" />
                    </colgroup>
                    <thead>
                      <tr>
                        <Th className="pl-6">Trace</Th>
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
                        <Th align="right" className="pr-6">
                          Compare
                        </Th>
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
                  <HonestyNote className="px-6 pb-4" kind="unverified">
                    Tokens and cost show “—” where a trace reports 0 or nothing; the source cannot
                    prove those are measured zeros.
                  </HonestyNote>
                </>
              )}
            </div>
          )}
        </div>

        <SessionDetailsDrawer
          session={session}
          summary={summary}
          usageUnverified={usageUnverified}
        />
      </div>
    </div>
  );
}

function ViewToggleButton({
  active,
  children,
  onClick,
}: {
  active: boolean;
  children: ReactNode;
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

function SummaryCell({
  hint,
  label,
  mono = false,
  suffix,
  tag,
  tone,
  value,
}: {
  hint?: string;
  label: string;
  mono?: boolean;
  suffix?: string;
  tag?: ReactNode;
  tone?: 'amber' | 'red';
  value: string;
}) {
  const toneClass =
    tone === 'red'
      ? 'text-[var(--c-red-text)]'
      : tone === 'amber'
        ? 'text-[var(--c-amber-text)]'
        : 'text-[var(--c-text-primary)]';

  return (
    <div
      className={`min-w-0 border-b border-r border-[var(--c-border)] px-4 py-3 lg:border-b-0 ${
        tone === 'amber' ? 'bg-[var(--c-amber-faint)]' : ''
      }`}
    >
      <div className="mb-1 text-[10.5px] font-semibold uppercase tracking-[0.05em] text-[var(--c-text-muted)]">
        {label}
      </div>
      <div className="flex min-w-0 items-baseline gap-1.5">
        <span
          className={`truncate text-[17px] font-semibold leading-tight tabular-nums ${toneClass} ${
            mono ? 'font-mono text-[14px]' : ''
          }`}
          title={value}
        >
          {value}
        </span>
        {suffix ? <span className="text-[11.5px] text-[var(--c-text-muted)]">{suffix}</span> : null}
        {tag}
      </div>
      {hint ? <div className="mt-1 text-[11px] text-[var(--c-text-muted)]">{hint}</div> : null}
    </div>
  );
}

function LineageNote({ summary }: { summary: SessionNarrativeSummary }) {
  if (summary.total_trace_count <= 1) {
    return (
      <HonestyNote className="mt-4" kind="not-captured">
        No trace relationships to show — a single-trace session has nothing to link or infer.
      </HonestyNote>
    );
  }

  return (
    <HonestyNote className="mt-4" kind={summary.inferred_link_count > 0 ? 'derived' : 'recorded'}>
      {summary.explicit_link_count} explicit {summary.explicit_link_count === 1 ? 'link' : 'links'}{' '}
      recorded · {summary.inferred_link_count} inferred from timing ·{' '}
      {summary.unlinked_trace_count} unlinked
    </HonestyNote>
  );
}

function SessionJourney({
  assignCompareRole,
  compare,
  isOnlyTrace,
  projectId,
  returnTo,
  setComparePair,
  traces,
}: {
  assignCompareRole: (role: CompareRole, traceId: string, mode?: HistoryMode) => void;
  compare: SessionDetailCompareState;
  isOnlyTrace: boolean;
  projectId?: string;
  returnTo: string;
  setComparePair: (baselineTraceId: string, candidateTraceId: string, mode?: HistoryMode) => void;
  traces: SessionNarrativeTrace[];
}) {
  const traceByExternalId = new Map(traces.map((trace) => [trace.trace_id, trace] as const));

  return (
    <ol className="flex flex-col gap-2.5">
      {traces.map((trace, index) => (
        <li key={trace.id}>
          <JourneyCard
            assignCompareRole={assignCompareRole}
            compare={compare}
            index={index + 1}
            isOnlyTrace={isOnlyTrace}
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
        </li>
      ))}
    </ol>
  );
}

function JourneyCard({
  assignCompareRole,
  compare,
  index,
  isOnlyTrace,
  parentTrace,
  projectId,
  returnTo,
  setComparePair,
  trace,
}: {
  assignCompareRole: (role: CompareRole, traceId: string, mode?: HistoryMode) => void;
  compare: SessionDetailCompareState;
  index: number;
  isOnlyTrace: boolean;
  parentTrace?: SessionNarrativeTrace;
  projectId?: string;
  returnTo: string;
  setComparePair: (baselineTraceId: string, candidateTraceId: string, mode?: HistoryMode) => void;
  trace: SessionNarrativeTrace;
}) {
  const traceHref = appendProjectToPath(`/traces/${trace.id}`, projectId);
  const isSelectable = isTerminalTraceStatus(trace.status);
  const canCompareToParent =
    parentTrace && isSelectable && isTerminalTraceStatus(parentTrace.status);
  const usageVerified = !isUsageUnverified([
    trace.total_tokens_in,
    trace.total_tokens_out,
    trace.total_cost_usd,
  ]);
  const events = trace.semantic_events.filter((event) => Boolean(event.event_type));
  const firstEvent = events[0];
  const latestEvent = events.length > 1 ? events[events.length - 1] : undefined;
  const nodeTone =
    trace.status === 'FAILED'
      ? 'var(--c-red)'
      : trace.status === 'RUNNING'
        ? 'var(--c-blue)'
        : 'var(--c-green)';

  return (
    <article className="rounded-lg border border-[var(--c-border)] bg-[var(--c-surface)] px-4 py-3">
      <div className="flex flex-wrap items-center gap-x-2.5 gap-y-1.5">
        <span
          className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full border font-mono text-[10.5px] font-semibold"
          style={{ borderColor: nodeTone, color: nodeTone }}
        >
          {index}
        </span>
        <Link
          to={traceHref}
          state={{ returnTo }}
          className="min-w-0 truncate font-mono text-[13px] font-semibold text-[var(--c-text-primary)] hover:text-[var(--c-accent-text)]"
        >
          {trace.name}
        </Link>
        <StatusDot status={trace.status} />
        {isOnlyTrace ? <Chip tone="muted">only trace</Chip> : null}
        {!isOnlyTrace && trace.lineage.type !== 'unlinked' ? (
          <Chip tone={trace.lineage.type === 'explicit' ? 'accent' : 'muted'}>
            {trace.lineage.type === 'explicit' ? 'explicit link' : 'inferred link'}
          </Chip>
        ) : null}
        {trace.error_count ? (
          <Chip tone="error">
            {trace.error_count} {trace.error_count === 1 ? 'error' : 'errors'}
          </Chip>
        ) : null}
        <span className="ml-auto flex items-center gap-3 font-mono text-[11.5px] tabular-nums text-[var(--c-text-muted)]">
          <span className="text-[var(--c-text-secondary)]">
            {formatDerivedDuration(trace.duration_ms)}
          </span>
          <span title={trace.started_at}>{formatExactTime(trace.started_at)}</span>
        </span>
      </div>

      {usageVerified ? (
        <div className="mt-1.5 pl-[34px] font-mono text-[11.5px] tabular-nums text-[var(--c-text-muted)]">
          {formatTokens((trace.total_tokens_in ?? 0) + (trace.total_tokens_out ?? 0))} tok ·{' '}
          {formatCost(trace.total_cost_usd)}
        </div>
      ) : null}

      <div className="mt-2.5 pl-[34px]">
        {firstEvent ? (
          <div className="grid gap-2 md:grid-cols-2">
            <EventSummary
              label={latestEvent ? 'First event' : 'Recorded event'}
              eventType={firstEvent.event_type}
              text={summarizeTimelineEvent(firstEvent)}
            />
            {latestEvent ? (
              <EventSummary
                label="Latest event"
                eventType={latestEvent.event_type}
                text={summarizeTimelineEvent(latestEvent)}
              />
            ) : null}
          </div>
        ) : (
          <HonestyNote kind="not-captured">
            No semantic events were recorded for this trace.
          </HonestyNote>
        )}
      </div>

      <div className="mt-3 flex flex-wrap items-center gap-1.5 pl-[34px]">
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
        <Link
          to={traceHref}
          state={{ returnTo }}
          className="ml-auto inline-flex items-center gap-1 text-[12px] font-medium text-[var(--c-accent-text)] hover:underline"
        >
          Open trace
          <ArrowRight className="h-3 w-3" />
        </Link>
      </div>
    </article>
  );
}

function EventSummary({
  eventType,
  label,
  text,
}: {
  eventType: string;
  label: string;
  text: string;
}) {
  return (
    <div className="min-w-0 rounded-md border border-[var(--c-border)] bg-[var(--c-app-bg)] px-2.5 py-2">
      <div className="mb-0.5 flex items-center gap-1.5 text-[10px] font-semibold uppercase tracking-[0.06em] text-[var(--c-text-muted)]">
        {label}
        <span className="font-mono normal-case tracking-normal">· {eventType}</span>
      </div>
      <p className="text-[12.5px] leading-5 text-[var(--c-text-secondary)]">{text}</p>
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
    <section
      aria-label="Compare selection"
      className="sticky top-0 z-20 border-b border-[var(--c-border)] bg-[var(--c-surface)] px-6 py-3"
    >
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
      aria-pressed={isSelected}
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
  const usageVerified = !isUsageUnverified([
    trace.total_tokens_in,
    trace.total_tokens_out,
    trace.total_cost_usd,
  ]);
  const isSelectable = isTerminalTraceStatus(trace.status);

  return (
    <Tr>
      <Td className="pl-6">
        <div className="flex min-w-0 flex-col gap-1">
          <Link
            to={appendProjectToPath(`/traces/${trace.id}`, projectId)}
            state={{ returnTo }}
            className="truncate font-mono text-[12.5px] font-medium text-[var(--c-text-primary)] hover:text-[var(--c-accent-text)]"
          >
            {trace.name}
          </Link>
          <div className="flex min-w-0 items-center gap-1.5">
            <span className="truncate font-mono text-[10.5px] text-[var(--c-text-muted)]" title={trace.id}>
              {shortId(trace.id)}
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
        {formatDerivedDuration(duration)}
      </Td>
      <Td align="right" mono dim={!usageVerified}>
        {usageVerified ? formatTokens(totalTokens) : '—'}
      </Td>
      <Td align="right" mono dim={!usageVerified}>
        {usageVerified ? formatCost(trace.total_cost_usd) : '—'}
      </Td>
      <Td align="right" mono className={(trace.error_count ?? 0) > 0 ? 'text-[var(--c-red-text)]' : ''}>
        {trace.error_count ?? 0}
      </Td>
      <Td align="right" dim>
        <span title={trace.started_at}>{formatExactTime(trace.started_at)}</span>
      </Td>
      <Td className="pr-6">
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
