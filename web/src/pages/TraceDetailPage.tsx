import {
  useCallback,
  useEffect,
  useRef,
  useState,
  useMemo,
  type Dispatch,
  type MutableRefObject,
  type ReactNode,
  type SetStateAction,
} from 'react';
import { Link, useLocation, useParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import {
  fetchTraces,
  fetchSpans,
  fetchTrace,
  isAuthError,
  type Span,
  type Trace,
  type TimelineEvent,
  type TraceDetail,
} from '../api/client';
import { AuthErrorBanner } from '../components/AuthErrorBanner';
import { CopyButton } from '../components/CopyButton';
import { EngineBadge } from '../components/EngineBadge';
import { EngineProjectionBanner } from '../components/EngineProjectionBanner';
import { ExecutionWaterfall } from '../components/ExecutionWaterfall';
import { FailureSummary } from '../components/FailureSummary';
import { InspectorTabs } from '../components/InspectorTabs';
import { JsonViewer } from '../components/JsonViewer';
import { ReasoningTab } from '../components/ReasoningTab';
import { SpanDetail } from '../components/SpanDetail';
import { StateDiffViewer } from '../components/StateDiffViewer';
import { StatusBadge } from '../components/StatusBadge';
import { TreeRail } from '../components/TreeRail';
import { Timeline } from '../components/Timeline';
import {
  MobileWorkspaceTabId,
  WorkspaceShell,
} from '../components/WorkspaceShell';
import { useMediaQuery } from '../hooks/useMediaQuery';
import { useTraceDetailSearchParams } from '../hooks/useTraceDetailSearchParams';
import { useWorkspaceState } from '../hooks/useWorkspaceState';
import type { RetrySafetyAssessment } from '../utils/retrySafety';
import { EngineControlBar } from './EngineControlBar';
import { EnginePendingWorkPanel } from './EnginePendingWorkPanel';
import { describeEngineWaitState } from './engineWaitState';
import {
  TIMELINE_POLL_INTERVAL_MS,
  useTraceTimeline,
} from './useTraceTimeline';
import { useEnginePendingWork } from './useEnginePendingWork';
import { useRetrySafetyAnalysis } from './useRetrySafetyAnalysis';
import { useWaitStallAnalysis } from './useWaitStallAnalysis';
import {
  calculateDuration,
  formatCost,
  formatDuration,
  formatRelativeTime,
  formatTimestamp,
  formatTokens,
} from '../utils/format';
import {
  buildBreadcrumbPath,
  buildFailureAnalysis,
  buildSpanIndex,
} from '../utils/failureAnalysis';
import { getWaitDetails } from '../utils/eventSemantics';
import { extractStateChanges } from '../utils/stateChanges';
import {
  buildReasoningEntries,
  buildTraceCostSeries,
  type TraceCostSeries,
} from '../utils/reasoning';
import { serializeSpanParam } from '../utils/traceDetailSearchParams';
import {
  appendProjectToPath,
  getProjectIdFromSearchParams,
} from '../utils/projectSearchParams';
import {
  computeOpenWaits,
  type OpenWait,
  type WaitStallAssessment,
} from '../utils/waitStallAnalysis';
import {
  buildSpanTree,
  collectExpandableSpanIds,
  deriveVisibleRows,
  type SpanTreeNode,
} from '../utils/spanTree';

const EMPTY_SPANS: Span[] = [];
const EMPTY_TRACES: Trace[] = [];
const EMPTY_RETRY_SAFETY_ASSESSMENTS = new Map<string, RetrySafetyAssessment>();
const DESKTOP_MEDIA_QUERY = '(min-width: 1024px)';
const MAX_LINEAGE_ANCESTOR_DEPTH = 64;
const CHILD_TRACES_PAGE_SIZE = 20;

function queryErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Unknown error';
}

export function TraceDetailPage() {
  const { id } = useParams<{ id: string }>();

  if (!id) {
    return <TraceDetailEmptyState>Trace ID is required.</TraceDetailEmptyState>;
  }

  return (
    <TraceDetailContent
      key={id}
      traceId={id}
    />
  );
}

interface TraceDetailContentProps {
  traceId: string;
}

function TraceDetailContent({ traceId }: TraceDetailContentProps) {
  const isDesktop = useMediaQuery(DESKTOP_MEDIA_QUERY);
  const location = useLocation();
  const currentProjectId = getProjectIdFromSearchParams(
    new URLSearchParams(location.search)
  );
  const projectQueryKey = currentProjectId ?? null;
  const { spanParam, setSpanParam } = useTraceDetailSearchParams();
  const [activeMobileTab, setActiveMobileTab] =
    useState<MobileWorkspaceTabId>('summary');
  const [isTraceContextOpen, setIsTraceContextOpen] = useState(false);
  const switchToInspectorDetailsRef = useRef<(() => void) | null>(null);
  const traceQuery = useQuery({
    queryKey: ['trace', traceId, projectQueryKey],
    queryFn: () => fetchTrace(traceId, currentProjectId),
  });
  const timeline = useTraceTimeline(traceId, currentProjectId);
  const liveTraceStatus = timeline.traceStatus ?? traceQuery.data?.status ?? null;
  const spansQuery = useQuery({
    queryKey: ['spans', traceId, projectQueryKey],
    queryFn: () => fetchSpans(traceId, currentProjectId),
    refetchInterval:
      liveTraceStatus === 'RUNNING' ? TIMELINE_POLL_INTERVAL_MS : false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  const trace = traceQuery.data ?? null;
  const lineageAncestorsQuery = useQuery({
    queryKey: [
      'trace-lineage-ancestors',
      traceId,
      projectQueryKey,
      trace?.engine?.run_id ?? null,
      trace?.engine?.parent_run_id ?? null,
    ],
    queryFn: () => fetchTraceLineageAncestors(trace!, currentProjectId),
    enabled: Boolean(trace?.engine?.parent_run_id),
  });
  const childTracesQuery = useQuery({
    queryKey: [
      'trace-lineage-children',
      traceId,
      projectQueryKey,
      trace?.engine?.run_id ?? null,
    ],
    queryFn: () => fetchDirectChildTraces(trace!.engine!.run_id, currentProjectId),
    enabled: Boolean(trace?.engine?.run_id),
  });
  const pendingWorkQuery = useEnginePendingWork(
    trace?.engine?.run_id,
    trace?.engine?.status
  );
  const lineageAncestors = lineageAncestorsQuery.data ?? EMPTY_TRACES;
  const lineageChain = useMemo(
    () => buildTraceLineageChain(trace, lineageAncestors),
    [lineageAncestors, trace]
  );
  const childTraces = childTracesQuery.data ?? EMPTY_TRACES;
  const spans = spansQuery.data?.spans ?? EMPTY_SPANS;
  const timelineAuthError = isAuthError(timeline.rawError);
  const timelineStatus = trace ? timeline.traceStatus ?? trace.status : timeline.traceStatus;
  const duration = trace ? calculateDuration(trace.started_at, trace.ended_at) : null;
  const totalTokens = trace
    ? (trace.total_tokens_in ?? 0) + (trace.total_tokens_out ?? 0)
    : 0;
  const stateChanges = useMemo(
    () => extractStateChanges(timeline.events),
    [timeline.events]
  );
  const reasoningEntries = useMemo(
    () => buildReasoningEntries(timeline.events, spans),
    [spans, timeline.events]
  );
  const returnTo = getReturnToDestination(location.state);
  const spanIndex = useMemo(() => buildSpanIndex(spans), [spans]);
  const failureAnalysis = useMemo(
    () => buildFailureAnalysis(spans, timeline.events, spanIndex),
    [spanIndex, spans, timeline.events]
  );
  const spanTree = useMemo(() => buildSpanTree(spans), [spans]);
  const expandableSpanIds = useMemo(
    () => collectExpandableSpanIds(spanTree),
    [spanTree]
  );
  const {
    expandedSpanIds,
    revealPath,
    revealVersion,
    selectSpan,
    selectedSpanExternalId,
    setExpandedSpanIds,
    toggleExpandedSpan,
    waterfallRevealTarget,
  } = useWorkspaceState({
    isSpanDataReady: spansQuery.isSuccess,
    spanIndex,
    spanParam,
    setSpanParam,
    timelineStatus,
    primaryFailedSpanId: failureAnalysis.summary.primaryFailedSpan?.span_id ?? null,
    expandableSpanIds,
  });
  const selectedSpan = selectedSpanExternalId
    ? spanIndex.get(selectedSpanExternalId) ?? null
    : null;
  const selectedBreadcrumbPath = useMemo(
    () => buildBreadcrumbPath(selectedSpan, spanIndex),
    [selectedSpan, spanIndex]
  );
  const waitStallAssessment = useWaitStallAnalysis({
    traceStatus: timelineStatus,
    traceStartedAt: trace?.started_at,
    spans,
    events: timeline.events,
    hasTimelineSnapshot: timeline.hasSnapshot,
  });
  const openWaits = useMemo(() => computeOpenWaits(timeline.events), [timeline.events]);
  const retrySafetyAnalysis = useRetrySafetyAnalysis(spans, timeline.events);
  const showRetrySafety = timelineStatus === 'FAILED';
  const visibleRetrySafetyAssessments = showRetrySafety
    ? retrySafetyAnalysis.spanAssessments
    : EMPTY_RETRY_SAFETY_ASSESSMENTS;
  const traceCostSeries = useMemo(
    () => buildTraceCostSeries(spans, timelineStatus),
    [spans, timelineStatus]
  );

  const handleSelectSpan = useCallback((spanId: string) => {
    selectSpan(spanId);
  }, [selectSpan]);

  const handleSelectSpanAndShowDetails = useCallback((spanId: string) => {
    selectSpan(spanId);
    switchToInspectorDetailsRef.current?.();
    setActiveMobileTab('summary');
  }, [selectSpan]);

  const buildCopyTraceUrl = useCallback(() => {
    const searchParams = serializeSpanParam(
      new URLSearchParams(location.search),
      selectedSpanExternalId
    );
    const nextSearch = searchParams.toString();

    return new URL(
      `${location.pathname}${nextSearch ? `?${nextSearch}` : ''}`,
      window.location.origin
    ).toString();
  }, [location.pathname, location.search, selectedSpanExternalId]);

  if (traceQuery.isLoading || spansQuery.isLoading) {
    return <TraceDetailEmptyState>Loading trace...</TraceDetailEmptyState>;
  }

  if (traceQuery.error) {
    return isAuthError(traceQuery.error) ? (
      <div className="app-page">
        <AuthErrorBanner message={queryErrorMessage(traceQuery.error)} />
      </div>
    ) : (
      <TraceDetailErrorState>
        Error loading trace: {queryErrorMessage(traceQuery.error)}
      </TraceDetailErrorState>
    );
  }

  if (spansQuery.error) {
    return isAuthError(spansQuery.error) ? (
      <div className="app-page">
        <AuthErrorBanner message={queryErrorMessage(spansQuery.error)} />
      </div>
    ) : (
      <TraceDetailErrorState>
        Error loading spans: {queryErrorMessage(spansQuery.error)}
      </TraceDetailErrorState>
    );
  }

  if (!trace) {
    return <TraceDetailEmptyState>Trace not found.</TraceDetailEmptyState>;
  }

  const detailsContent = (
    <TraceDetailsSurface
      pendingWorkContent={
        trace.engine ? (
          <EnginePendingWorkPanel
            data={pendingWorkQuery.data}
            isError={pendingWorkQuery.isError}
            isLoading={pendingWorkQuery.isLoading}
            errorMessage={queryErrorMessage(pendingWorkQuery.error)}
          />
        ) : null
      }
      runningStateAssessment={waitStallAssessment}
      timelineStatus={timelineStatus}
      failureAnalysis={failureAnalysis}
      retrySafetyAnalysis={showRetrySafety ? retrySafetyAnalysis : null}
      onSelectSpan={handleSelectSpanAndShowDetails}
      openWaits={openWaits}
      selectedBreadcrumbPath={selectedBreadcrumbPath}
      selectedSpan={selectedSpan}
      spanIndex={spanIndex}
      events={timeline.events}
    />
  );

  const timelineContent = (
    <Timeline
      events={timeline.events}
      traceStatus={timelineStatus}
      isLive={timeline.isLive}
      isLoading={timeline.isLoading}
      error={timelineAuthError ? null : timeline.error}
      selectedSpanId={selectedSpanExternalId}
      onSelectSpan={handleSelectSpanAndShowDetails}
      spanIndex={spanIndex}
    />
  );
  const reasoningContent = (
    <ReasoningTab
      entries={reasoningEntries}
      onSelectSpan={handleSelectSpanAndShowDetails}
    />
  );
  const stateContent = <StateDiffViewer changes={stateChanges} />;
  const mobileSummaryContent = (
    <div className="grid h-full gap-4 overflow-y-auto p-4">
      <TraceLineageCard
        childTraces={childTraces}
        childTracesLoading={childTracesQuery.isLoading}
        hasChildTracesError={childTracesQuery.isError}
        lineageChain={lineageChain}
        lineageLoading={lineageAncestorsQuery.isLoading}
        projectId={currentProjectId}
        returnTo={returnTo}
        showLineageSummary
        showEmptyChildren={Boolean(trace.engine?.parent_run_id)}
        trace={trace}
      />
      {detailsContent}
      {reasoningContent}
    </div>
  );

  return (
    <div className="relative flex min-h-full flex-col gap-4">
      <header className="app-surface p-5 sm:p-6">
        <div className="flex flex-col gap-5 xl:flex-row xl:items-start xl:justify-between">
          <div className="min-w-0 flex-1">
            <Link
              to={returnTo}
              className="inline-flex items-center text-sm font-medium text-[var(--continua-accent)] transition hover:opacity-80"
            >
              {returnTo.startsWith('/sessions/') ? '← Session' : '← Traces'}
            </Link>

            <div className="mt-4 flex flex-wrap items-center gap-3">
              <h1 className="truncate text-3xl font-black tight-headline text-[var(--continua-text-primary)]">
                {trace.name}
              </h1>
              {trace.engine ? (
                <EngineBadge projectionState={trace.engine.projection_state} />
              ) : null}
              <StatusBadge status={timelineStatus ?? trace.status} />
            </div>

            <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-[var(--continua-text-muted)]">
              <span className="rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-2.5 py-1 font-mono">
                {trace.trace_id ?? trace.id}
              </span>
              {trace.session_external_id ? (
                <span className="rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-2.5 py-1">
                  {trace.session_external_id}
                </span>
              ) : null}
              {trace.engine ? (
                <span className="rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-2.5 py-1">
                  {trace.engine.definition_name}@{trace.engine.definition_version}
                </span>
              ) : null}
            </div>

            {isDesktop ? (
              <TraceLineageBreadcrumb
                chain={lineageChain}
                isLoading={lineageAncestorsQuery.isLoading}
                projectId={currentProjectId}
                returnTo={returnTo}
                trace={trace}
              />
            ) : null}

            {trace.engine?.continued_from_trace_id ||
            trace.engine?.continued_to_trace_id ? (
              <div className="mt-4 flex flex-wrap gap-2 text-sm">
                {trace.engine.continued_from_trace_id ? (
                  <Link
                    to={appendProjectToPath(
                      `/traces/${trace.engine.continued_from_trace_id}`,
                      currentProjectId
                    )}
                    state={{ returnTo }}
                    className="inline-flex items-center rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-3 py-1.5 font-medium text-[var(--continua-text-secondary)] transition hover:border-[var(--continua-border-strong)] hover:text-[var(--continua-accent)]"
                  >
                    ← Previous run
                  </Link>
                ) : null}
                {trace.engine.continued_to_trace_id ? (
                  <Link
                    to={appendProjectToPath(
                      `/traces/${trace.engine.continued_to_trace_id}`,
                      currentProjectId
                    )}
                    state={{ returnTo }}
                    className="inline-flex items-center rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-3 py-1.5 font-medium text-[var(--continua-text-secondary)] transition hover:border-[var(--continua-border-strong)] hover:text-[var(--continua-accent)]"
                  >
                    Next run →
                  </Link>
                ) : null}
              </div>
            ) : null}

            {trace.engine?.status === 'WAITING' ? (
              <EngineWaitStateSummary engine={trace.engine} />
            ) : null}

            <div className="mt-5 grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              <TraceHeaderMetric label="Duration" value={formatDuration(duration)} />
              <TraceHeaderMetric label="Tokens" value={formatTokens(totalTokens)} />
              <TraceHeaderMetric label="Cost" value={formatCost(trace.total_cost_usd)} />
              <TraceHeaderMetric
                label="Errors"
                value={trace.error_count && trace.error_count > 0 ? String(trace.error_count) : '0'}
                danger={Boolean(trace.error_count && trace.error_count > 0)}
              />
            </div>
          </div>

          <div className="flex flex-wrap gap-2 xl:justify-end">
            <CopyButton
              aria-label="Copy Trace URL"
              getValue={buildCopyTraceUrl}
              idleLabel="Copy trace URL"
              successLabel="Copied URL"
              className="border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] text-[var(--continua-text-secondary)] hover:border-[var(--continua-border-strong)] hover:bg-[var(--continua-surface-muted)] hover:text-[var(--continua-text-primary)]"
            />
            <button
              type="button"
              aria-expanded={isTraceContextOpen}
              aria-label="Trace Context"
              onClick={() => setIsTraceContextOpen((open) => !open)}
              className="app-button-secondary"
            >
              {isTraceContextOpen ? 'Hide context' : 'Trace context'}
            </button>
          </div>
        </div>
      </header>

      {timelineAuthError ? (
        <AuthErrorBanner message={queryErrorMessage(timeline.rawError)} />
      ) : null}

      <EngineProjectionBanner projectionState={trace.engine?.projection_state} />
      {trace.engine?.failure?.error_code === 'definition_version_mismatch' ? (
        <DefinitionVersionMismatchBanner />
      ) : null}
      {trace.engine ? (
        <EngineControlBar
          engine={trace.engine}
          traceId={traceId}
        />
      ) : null}

      <div className="min-h-[42rem] flex-1">
        <TraceWorkspace
          activeMobileTab={activeMobileTab}
          detailsContent={detailsContent}
          events={timeline.events}
          expandableSpanIds={expandableSpanIds}
          expandedSpanIds={expandedSpanIds}
          failedSpanIds={failureAnalysis.failedSpanIds}
          inlineErrorPreviews={failureAnalysis.inlineErrorPreviews}
          inspectorSwitchToDetailsRef={switchToInspectorDetailsRef}
          isDesktop={isDesktop}
          mobileSummaryContent={mobileSummaryContent}
          onMobileTabChange={setActiveMobileTab}
          onSelectSpan={handleSelectSpan}
          onSelectSpanAndShowDetails={handleSelectSpanAndShowDetails}
          onToggleExpand={toggleExpandedSpan}
          primaryAncestorPath={failureAnalysis.primaryAncestorPath}
          revealKey={revealVersion}
          revealPath={revealPath}
          revealTarget={waterfallRevealTarget}
          reasoningContent={reasoningContent}
          selectedSpanId={selectedSpanExternalId}
          setExpandedSpanIds={setExpandedSpanIds}
          spanAssessments={visibleRetrySafetyAssessments}
          spanIndex={spanIndex}
          spanTree={spanTree}
          spans={spans}
          stateChangeCount={stateChanges.length}
          stateContent={stateContent}
          traceCostSeries={traceCostSeries}
          timelineContent={timelineContent}
          traceEndedAt={trace.ended_at}
          traceStartedAt={trace.started_at}
        />
      </div>

      {isDesktop && isTraceContextOpen ? (
        <TraceContextDrawer
          buildCopyTraceUrl={buildCopyTraceUrl}
          childTraces={childTraces}
          childTracesLoading={childTracesQuery.isLoading}
          hasLineageError={childTracesQuery.isError}
          onClose={() => setIsTraceContextOpen(false)}
          projectId={currentProjectId}
          returnTo={returnTo}
          trace={trace}
        />
      ) : null}

      {!isDesktop && isTraceContextOpen ? (
        <TraceContextSheet
          buildCopyTraceUrl={buildCopyTraceUrl}
          childTraces={childTraces}
          childTracesLoading={childTracesQuery.isLoading}
          hasLineageError={childTracesQuery.isError}
          onClose={() => setIsTraceContextOpen(false)}
          projectId={currentProjectId}
          returnTo={returnTo}
          trace={trace}
        />
      ) : null}
    </div>
  );
}

function EngineWaitStateSummary({
  engine,
}: {
  engine: NonNullable<TraceDetail['engine']>;
}) {
  const summary = describeEngineWaitState(engine.wait_state);
  if (!summary) {
    return null;
  }

  return (
    <section className="mt-4 rounded-[1rem] border border-sky-300/40 bg-sky-50/80 px-4 py-3 text-sky-900 dark:border-sky-400/20 dark:bg-sky-400/10 dark:text-sky-100">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="text-sm font-semibold">{summary.heading}</h2>
        <span className="text-[11px] font-semibold uppercase tracking-[0.12em] opacity-75">
          {engine.pending_work.pending_activity_tasks} tasks · {engine.pending_work.pending_inbox_items} inbox
        </span>
      </div>
      <p className="mt-1 text-sm leading-6">{summary.detail}</p>
    </section>
  );
}

function TraceLineageBreadcrumb({
  chain,
  isLoading,
  projectId,
  returnTo,
  trace,
}: {
  chain: Trace[];
  isLoading: boolean;
  projectId?: string;
  returnTo: string;
  trace: TraceDetail;
}) {
  if (!trace.engine?.parent_run_id) {
    return null;
  }

  if (!isLoading && chain.length <= 1) {
    return null;
  }

  return (
    <section className="mt-4">
      <p className="text-[11px] font-semibold uppercase tracking-[0.18em] text-[var(--continua-text-muted)]">
        Lineage
      </p>
      <nav
        aria-label="Trace lineage"
        className="mt-2 flex flex-wrap items-center gap-2 text-sm"
      >
        {chain.length > 1 ? (
          chain.map((lineageTrace, index) => {
            const isCurrent = lineageTrace.id === trace.id;
            return (
              <div
                key={lineageTrace.engine?.run_id ?? lineageTrace.id}
                className="contents"
              >
                {index > 0 ? (
                  <span className="text-[var(--continua-text-muted)]">›</span>
                ) : null}
                {isCurrent ? (
                  <span className="rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-3 py-1.5 font-medium text-[var(--continua-text-primary)]">
                    {lineageTrace.name}
                  </span>
                ) : (
                  <Link
                    to={appendProjectToPath(`/traces/${lineageTrace.id}`, projectId)}
                    state={{ returnTo }}
                    className="rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-3 py-1.5 font-medium text-[var(--continua-text-secondary)] transition hover:border-[var(--continua-border-strong)] hover:text-[var(--continua-accent)]"
                  >
                    {lineageTrace.name}
                  </Link>
                )}
              </div>
            );
          })
        ) : (
          <span className="rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-3 py-1.5 text-[var(--continua-text-muted)]">
            Loading lineage...
          </span>
        )}
      </nav>
    </section>
  );
}

async function fetchTraceLineageAncestors(
  trace: TraceDetail,
  projectId?: string
): Promise<Trace[]> {
  const engine = trace.engine;
  if (!engine?.run_id || !engine.parent_run_id) {
    return [];
  }

  const ancestors: Trace[] = [];
  const visitedRunIds = new Set<string>([engine.run_id]);
  let parentRunId: string | undefined = engine.parent_run_id;

  while (parentRunId) {
    if (ancestors.length >= MAX_LINEAGE_ANCESTOR_DEPTH) {
      return [];
    }
    if (visitedRunIds.has(parentRunId)) {
      return [];
    }
    visitedRunIds.add(parentRunId);

    const page = await fetchTraces({
      project_id: projectId,
      engine_run_id: parentRunId,
      limit: 1,
    });
    const parentTrace = page.traces[0];
    if (!parentTrace?.engine?.run_id) {
      return [];
    }

    ancestors.push(parentTrace);
    parentRunId = parentTrace.engine.parent_run_id;
  }

  return ancestors.reverse();
}

async function fetchDirectChildTraces(
  parentRunId: string,
  projectId?: string
): Promise<Trace[]> {
  const traces: Trace[] = [];
  let offset = 0;

  for (;;) {
    const page = await fetchTraces({
      project_id: projectId,
      engine_parent_run_id: parentRunId,
      limit: CHILD_TRACES_PAGE_SIZE,
      offset,
    });

    traces.push(...page.traces);
    if (
      page.traces.length === 0 ||
      traces.length >= page.total ||
      page.traces.length < CHILD_TRACES_PAGE_SIZE
    ) {
      return traces;
    }

    offset += page.traces.length;
  }
}

function buildTraceLineageChain(
  trace: TraceDetail | null,
  lineageAncestors: Trace[]
): Trace[] {
  if (!trace?.engine?.run_id) {
    return [];
  }

  const visitedRunIds = new Set<string>();
  return [...lineageAncestors, trace].filter((lineageTrace) => {
    const runId = lineageTrace.engine?.run_id;
    if (!runId || visitedRunIds.has(runId)) {
      return false;
    }
    visitedRunIds.add(runId);
    return true;
  });
}

function TraceLineageCard({
  childTraces,
  childTracesLoading,
  framed = true,
  hasChildTracesError,
  lineageChain,
  lineageLoading,
  projectId,
  returnTo,
  showEmptyChildren = false,
  showLineageSummary = false,
  trace,
}: {
  childTraces: Trace[];
  childTracesLoading: boolean;
  framed?: boolean;
  hasChildTracesError: boolean;
  lineageChain: Trace[];
  lineageLoading: boolean;
  projectId?: string;
  returnTo: string;
  showEmptyChildren?: boolean;
  showLineageSummary?: boolean;
  trace: TraceDetail;
}) {
  const hasParent = Boolean(trace.engine?.parent_run_id);
  const hasRenderableLineage =
    showLineageSummary && hasParent && (lineageLoading || lineageChain.length > 1);
  const shouldRender =
    hasRenderableLineage ||
    childTracesLoading ||
    hasChildTracesError ||
    childTraces.length > 0 ||
    showEmptyChildren;

  if (!trace.engine || !shouldRender) {
    return null;
  }

  return (
    <section
      className={
        framed
          ? 'overflow-hidden rounded-[1rem] border border-[var(--continua-border-strong)] bg-[var(--continua-surface)] shadow-[var(--continua-shadow-soft)]'
          : 'space-y-4'
      }
    >
      <div
        className={
          framed
            ? 'border-b border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-4 py-3'
            : ''
        }
      >
        <h2 className="text-sm font-semibold uppercase tracking-[0.18em] text-[var(--continua-text-secondary)]">
          Child Workflows
        </h2>
      </div>
      <div className={framed ? 'space-y-4 p-4' : 'space-y-4'}>
        {showLineageSummary ? (
          <TraceLineageSummaryBreadcrumb
            chain={lineageChain}
            isLoading={lineageLoading}
            projectId={projectId}
            returnTo={returnTo}
            trace={trace}
          />
        ) : null}
        <ChildWorkflowSection
          childTraces={childTraces}
          isError={hasChildTracesError}
          isLoading={childTracesLoading}
          projectId={projectId}
          returnTo={returnTo}
          showEmptyState={showEmptyChildren}
        />
      </div>
    </section>
  );
}

function TraceLineageSummaryBreadcrumb({
  chain,
  isLoading,
  projectId,
  returnTo,
  trace,
}: {
  chain: Trace[];
  isLoading: boolean;
  projectId?: string;
  returnTo: string;
  trace: TraceDetail;
}) {
  if (!trace.engine?.parent_run_id) {
    return null;
  }

  if (!isLoading && chain.length <= 1) {
    return null;
  }

  return (
    <div className="space-y-2">
      <p className="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--continua-text-muted)]">
        Lineage
      </p>
      {chain.length > 1 ? (
        <nav
          aria-label="Trace lineage summary"
          className="flex flex-wrap items-center gap-2 text-sm"
        >
          {chain.map((lineageTrace, index) => {
            const isCurrent = lineageTrace.id === trace.id;
            return (
              <div
                key={lineageTrace.engine?.run_id ?? lineageTrace.id}
                className="contents"
              >
                {index > 0 ? (
                  <span className="text-[var(--continua-text-muted)]">›</span>
                ) : null}
                {isCurrent ? (
                  <span className="rounded-lg border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-3 py-2 font-medium text-[var(--continua-text-primary)]">
                    {lineageTrace.name}
                  </span>
                ) : (
                  <Link
                    to={appendProjectToPath(`/traces/${lineageTrace.id}`, projectId)}
                    state={{ returnTo }}
                    className="inline-flex max-w-full items-center rounded-lg border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-3 py-2 font-medium text-[var(--continua-accent)] transition hover:border-[var(--continua-border-strong)] hover:opacity-80"
                  >
                    <span className="truncate">{lineageTrace.name}</span>
                  </Link>
                )}
              </div>
            );
          })}
        </nav>
      ) : isLoading ? (
        <p className="text-sm text-[var(--continua-text-muted)]">
          Loading lineage...
        </p>
      ) : null}
    </div>
  );
}

function ChildWorkflowSection({
  childTraces,
  isError,
  isLoading,
  projectId,
  returnTo,
  showEmptyState,
}: {
  childTraces: Trace[];
  isError: boolean;
  isLoading: boolean;
  projectId?: string;
  returnTo: string;
  showEmptyState: boolean;
}) {
  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-3">
        <p className="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--continua-text-muted)]">
          Direct Children
        </p>
        {!isLoading && !isError ? (
          <span className="text-xs text-[var(--continua-text-muted)]">
            {childTraces.length}
          </span>
        ) : null}
      </div>

      {isLoading ? (
        <p className="text-sm text-[var(--continua-text-muted)]">
          Loading child workflows...
        </p>
      ) : null}

      {isError ? (
        <p className="text-sm text-[var(--continua-text-muted)]">
          Child workflow lineage is temporarily unavailable.
        </p>
      ) : null}

      {!isLoading && !isError && childTraces.length === 0 && showEmptyState ? (
        <p className="text-sm text-[var(--continua-text-muted)]">
          No direct child workflows yet.
        </p>
      ) : null}

      {!isLoading && !isError && childTraces.length > 0 ? (
        <div className="space-y-3">
          {childTraces.map((childTrace) => (
            <Link
              key={childTrace.id}
              to={appendProjectToPath(`/traces/${childTrace.id}`, projectId)}
              state={{ returnTo }}
              className="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-3 py-3 transition hover:border-[var(--continua-border-strong)] hover:bg-[var(--continua-surface-elevated)] hover:opacity-90 focus:outline-none focus:ring-2 focus:ring-[var(--continua-accent)]"
            >
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-mono text-xs text-[var(--continua-text-muted)]">
                    {childTrace.engine?.child_key ?? 'child'}
                  </span>
                  <StatusBadge status={childTrace.status} />
                </div>
                <p className="mt-2 truncate text-sm font-medium text-[var(--continua-text-primary)]">
                  {childTrace.name}
                </p>
                {childTrace.engine ? (
                  <p className="mt-1 text-xs text-[var(--continua-text-muted)]">
                    {childTrace.engine.definition_name}@
                    {childTrace.engine.definition_version}
                  </p>
                ) : null}
              </div>
              <span
                className="inline-flex items-center rounded-lg border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-3 py-2 text-sm font-medium text-[var(--continua-accent)] transition hover:border-[var(--continua-border-strong)] hover:opacity-80"
              >
                Open trace
              </span>
            </Link>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function DefinitionVersionMismatchBanner() {
  return (
    <section className="rounded-[1rem] border border-amber-300/50 bg-amber-100/70 px-4 py-3 text-sm text-amber-950 shadow-[var(--continua-shadow-soft)] dark:border-amber-300/20 dark:bg-amber-400/10 dark:text-amber-100">
      <p className="font-semibold">Definition version mismatch</p>
      <p className="mt-1">
        This run failed because the engine definition version could not be
        matched during activation.
      </p>
    </section>
  );
}

interface TraceWorkspaceProps {
  activeMobileTab: MobileWorkspaceTabId;
  detailsContent: ReactNode;
  events: TimelineEvent[];
  expandableSpanIds: ReadonlySet<string>;
  expandedSpanIds: ReadonlySet<string>;
  failedSpanIds: ReadonlySet<string>;
  inlineErrorPreviews: ReadonlyMap<string, string>;
  inspectorSwitchToDetailsRef: MutableRefObject<(() => void) | null>;
  isDesktop: boolean;
  mobileSummaryContent: ReactNode;
  onMobileTabChange: (tab: MobileWorkspaceTabId) => void;
  onSelectSpan: (spanId: string) => void;
  onSelectSpanAndShowDetails: (spanId: string) => void;
  onToggleExpand: (spanId: string) => void;
  primaryAncestorPath: ReadonlySet<string>;
  revealKey: number;
  revealPath: ReadonlySet<string>;
  revealTarget: string | null;
  reasoningContent: ReactNode;
  selectedSpanId: string | null;
  setExpandedSpanIds: Dispatch<SetStateAction<Set<string>>>;
  spanAssessments: ReadonlyMap<string, RetrySafetyAssessment>;
  spanIndex: ReadonlyMap<string, Span>;
  spanTree: SpanTreeNode[];
  spans: Span[];
  stateChangeCount: number;
  stateContent: ReactNode;
  traceCostSeries: TraceCostSeries | null;
  timelineContent: ReactNode;
  traceEndedAt?: string;
  traceStartedAt?: string;
}

function TraceWorkspace({
  activeMobileTab,
  detailsContent,
  events,
  expandableSpanIds,
  expandedSpanIds,
  failedSpanIds,
  inlineErrorPreviews,
  inspectorSwitchToDetailsRef,
  isDesktop,
  mobileSummaryContent,
  onMobileTabChange,
  onSelectSpan,
  onSelectSpanAndShowDetails,
  onToggleExpand,
  primaryAncestorPath,
  revealKey,
  revealPath,
  revealTarget,
  reasoningContent,
  selectedSpanId,
  setExpandedSpanIds,
  spanAssessments,
  spanIndex,
  spanTree,
  spans,
  stateChangeCount,
  stateContent,
  traceCostSeries,
  timelineContent,
  traceEndedAt,
  traceStartedAt,
}: TraceWorkspaceProps) {
  const [visibleRows, setVisibleRows] = useState(() =>
    deriveVisibleRows(spanTree, expandedSpanIds)
  );

  useEffect(() => {
    setVisibleRows(deriveVisibleRows(spanTree, expandedSpanIds));
  }, [expandedSpanIds, spanTree]);

  return (
    <WorkspaceShell
      isDesktop={isDesktop}
      treeRail={
        <TreeRail
          expandableSpanIds={expandableSpanIds}
          expandedSpanIds={expandedSpanIds}
          failedSpanIds={failedSpanIds}
          inlineErrorPreviews={inlineErrorPreviews}
          onSelectSpan={onSelectSpan}
          onToggleExpand={onToggleExpand}
          onVisibleRowsChange={setVisibleRows}
          primaryAncestorPath={primaryAncestorPath}
          revealKey={revealKey}
          revealPath={revealPath}
          selectedSpanId={selectedSpanId}
          setExpandedSpanIds={setExpandedSpanIds}
          spanIndex={spanIndex}
          spanTree={spanTree}
          spans={spans}
          spanAssessments={spanAssessments}
        />
      }
      waterfall={
        <ExecutionWaterfall
          events={events}
          rows={visibleRows}
          selectedSpanId={selectedSpanId}
          onSelectSpanAndShowDetails={onSelectSpanAndShowDetails}
          revealTarget={revealTarget}
          revealVersion={revealKey}
          spans={spans}
          costSeries={traceCostSeries}
          spanAssessments={spanAssessments}
          traceEndedAt={traceEndedAt}
          traceStartedAt={traceStartedAt}
        />
      }
      inspector={
        <InspectorTabs
          details={detailsContent}
          reasoning={reasoningContent}
          timeline={timelineContent}
          state={stateContent}
          stateCount={stateChangeCount}
          switchToDetailsRef={inspectorSwitchToDetailsRef}
        />
      }
      mobileSummary={mobileSummaryContent}
      mobileTimeline={timelineContent}
      mobileState={stateContent}
      activeMobileTab={activeMobileTab}
      onMobileTabChange={onMobileTabChange}
    />
  );
}

function getReturnToDestination(state: unknown): string {
  if (
    typeof state !== 'object' ||
    state === null ||
    !('returnTo' in state) ||
    typeof state.returnTo !== 'string'
  ) {
    return '/traces';
  }

  const { returnTo } = state;
  return returnTo === '/traces' ||
    returnTo.startsWith('/traces?') ||
    returnTo.startsWith('/sessions/')
    ? returnTo
    : '/traces';
}

function TraceDetailsSurface({
  pendingWorkContent,
  runningStateAssessment,
  timelineStatus,
  failureAnalysis,
  retrySafetyAnalysis,
  onSelectSpan,
  openWaits,
  selectedBreadcrumbPath,
  selectedSpan,
  spanIndex,
  events,
}: {
  pendingWorkContent: ReactNode;
  runningStateAssessment: WaitStallAssessment | null;
  timelineStatus: 'RUNNING' | 'COMPLETED' | 'FAILED' | null;
  failureAnalysis: ReturnType<typeof buildFailureAnalysis>;
  retrySafetyAnalysis: ReturnType<typeof useRetrySafetyAnalysis> | null;
  onSelectSpan: (spanId: string) => void;
  openWaits: OpenWait[];
  selectedBreadcrumbPath: ReturnType<typeof buildBreadcrumbPath>;
  selectedSpan: Span | null;
  spanIndex: ReadonlyMap<string, Span>;
  events: TimelineEvent[];
}) {
  return (
    <div className="flex h-full min-h-0 flex-col overflow-y-auto bg-transparent p-4">
      <div className="space-y-4">
        {pendingWorkContent}

        {timelineStatus === 'FAILED' ? (
          <FailureSummary
            summary={failureAnalysis.summary}
            onJumpToPrimaryFailedSpan={onSelectSpan}
            traceRetrySafety={retrySafetyAnalysis?.traceAssessment ?? null}
          />
        ) : null}

        {runningStateAssessment ? (
          <RunningStatePanel
            assessment={runningStateAssessment}
            events={events}
            openWaits={openWaits}
            spanIndex={spanIndex}
            onSelectSpan={onSelectSpan}
          />
        ) : null}

        <div className="min-h-[22rem] overflow-hidden rounded-[1rem] border border-[var(--continua-border-strong)] bg-[var(--continua-surface)] shadow-[var(--continua-shadow-soft)]">
          <SpanDetail
            span={selectedSpan}
            breadcrumbPath={selectedBreadcrumbPath}
            onSelectSpan={onSelectSpan}
            spanIndex={spanIndex}
            events={events}
            retrySafety={
              selectedSpan?.status === 'FAILED' && retrySafetyAnalysis
                ? retrySafetyAnalysis.spanAssessments.get(selectedSpan.span_id) ?? null
                : null
            }
          />
        </div>
      </div>
    </div>
  );
}

function TraceHeaderMetric({
  label,
  value,
  danger = false,
}: {
  label: string;
  value: string;
  danger?: boolean;
}) {
  return (
    <div className="app-surface-muted px-4 py-3">
      <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-[var(--continua-text-muted)]">
        {label}
      </div>
      <div
        className={`mt-2 text-xl font-black tight-headline ${
          danger ? 'text-red-600 dark:text-red-300' : 'text-[var(--continua-text-primary)]'
        }`}
      >
        {value}
      </div>
    </div>
  );
}

function TraceContextDrawer({
  buildCopyTraceUrl,
  childTraces,
  childTracesLoading,
  hasLineageError,
  onClose,
  projectId,
  returnTo,
  trace,
}: {
  buildCopyTraceUrl: () => string;
  childTraces: Trace[];
  childTracesLoading: boolean;
  hasLineageError: boolean;
  onClose: () => void;
  projectId?: string;
  returnTo: string;
  trace: TraceDetail;
}) {
  return (
    <div className="app-overlay-enter fixed inset-0 z-50 hidden bg-[#111318]/40 backdrop-blur-sm lg:block">
      <button
        type="button"
        aria-label="Close trace context drawer"
        className="absolute inset-0"
        onClick={onClose}
      />
      <aside
        className="app-drawer-enter absolute inset-y-4 right-4 w-[32rem] max-w-[calc(100vw-2rem)] overflow-y-auto"
        role="dialog"
        aria-modal="true"
        aria-label="Trace context"
      >
        <TraceContextSection
          buildCopyTraceUrl={buildCopyTraceUrl}
          childTraces={childTraces}
          childTracesLoading={childTracesLoading}
          hasLineageError={hasLineageError}
          onToggle={onClose}
          open
          projectId={projectId}
          returnTo={returnTo}
          showLineage
          trace={trace}
        />
      </aside>
    </div>
  );
}

function TraceContextSheet({
  buildCopyTraceUrl,
  childTraces,
  childTracesLoading,
  hasLineageError,
  onClose,
  projectId,
  returnTo,
  trace,
}: {
  buildCopyTraceUrl: () => string;
  childTraces: Trace[];
  childTracesLoading: boolean;
  hasLineageError: boolean;
  onClose: () => void;
  projectId?: string;
  returnTo: string;
  trace: TraceDetail;
}) {
  return (
    <div className="app-overlay-enter fixed inset-0 z-50 flex items-end bg-[#111318]/50 backdrop-blur-sm lg:hidden">
      <button
        type="button"
        aria-label="Close trace context sheet"
        className="absolute inset-0"
        onClick={onClose}
      />
      <aside
        className="app-sheet-enter relative z-10 max-h-[82vh] w-full overflow-y-auto px-3 pb-3"
        role="dialog"
        aria-modal="true"
        aria-label="Trace context"
      >
        <TraceContextSection
          buildCopyTraceUrl={buildCopyTraceUrl}
          childTraces={childTraces}
          childTracesLoading={childTracesLoading}
          hasLineageError={hasLineageError}
          onToggle={onClose}
          open
          projectId={projectId}
          returnTo={returnTo}
          showLineage={false}
          trace={trace}
        />
      </aside>
    </div>
  );
}

function TraceContextSection({
  buildCopyTraceUrl,
  childTraces,
  childTracesLoading,
  hasLineageError,
  onToggle,
  open,
  projectId,
  returnTo,
  showLineage,
  trace,
}: {
  buildCopyTraceUrl: () => string;
  childTraces: Trace[];
  childTracesLoading: boolean;
  hasLineageError: boolean;
  onToggle: () => void;
  open: boolean;
  projectId?: string;
  returnTo: string;
  showLineage: boolean;
  trace: TraceDetail;
}) {
  return (
    <section className="overflow-hidden rounded-[1rem] border border-[var(--continua-border-strong)] bg-[var(--continua-surface)] shadow-[var(--continua-shadow-soft)]">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-4 py-3">
        <button
          type="button"
          className="flex items-center gap-3 text-left"
          aria-expanded={open}
          onClick={onToggle}
        >
          <span className="text-sm font-semibold uppercase tracking-[0.2em] text-[var(--continua-text-secondary)]">
            Trace Context
          </span>
          <span className="rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-2 py-1 text-xs font-medium text-[var(--continua-text-muted)]">
            {open ? 'Hide' : 'Show'}
          </span>
        </button>
        <CopyButton
          aria-label="Copy Trace URL"
          className="shrink-0"
          getValue={buildCopyTraceUrl}
          idleLabel="Copy Trace URL"
          successLabel="Copied URL"
        />
      </div>

      {open ? (
        <div className="space-y-6 p-4">
          <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
            <TraceContextField
              label="ID"
              value={renderContextText(trace.id, true)}
              copyValue={trace.id}
              copyButtonLabel="Copy trace UUID"
            />
            <TraceContextField
              label="External Trace ID"
              value={renderContextText(trace.trace_id, true)}
              copyValue={trace.trace_id}
              copyButtonLabel="Copy external trace ID"
            />
            <TraceContextField
              label="Session"
              value={trace.session_id ? (
                <Link
                  to={appendProjectToPath(`/sessions/${trace.session_id}`, projectId)}
                  className="inline-flex flex-col text-left text-[var(--continua-accent)] hover:opacity-80"
                >
                  <span className="text-sm font-medium text-[var(--continua-text-primary)]">
                    {trace.session_external_id ?? trace.session_id}
                  </span>
                  <span className="font-mono text-xs text-[var(--continua-text-muted)]">
                    {trace.session_id}
                  </span>
                </Link>
              ) : (
                renderContextText(undefined)
              )}
              copyValue={trace.session_id}
              copyButtonLabel="Copy session UUID"
            />
            <TraceContextField
              label="User ID"
              value={renderContextText(trace.user_id)}
            />
            <TraceContextField
              label="Environment"
              value={renderContextText(trace.environment)}
            />
            <TraceContextField
              label="Release"
              value={renderContextText(trace.release)}
            />
            <TraceContextField
              label="Tags"
              className="md:col-span-2 xl:col-span-3"
              value={trace.tags && trace.tags.length > 0 ? (
                <div className="flex flex-wrap gap-2">
                  {trace.tags.map((tag) => (
                    <span
                      key={tag}
                      className="rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-3 py-1 font-mono text-xs text-[var(--continua-text-secondary)]"
                    >
                      {tag}
                    </span>
                  ))}
                </div>
              ) : (
                renderContextText(undefined)
              )}
            />
          </div>

          {showLineage && trace.engine ? (
            <TraceLineageCard
              childTraces={childTraces}
              childTracesLoading={childTracesLoading}
              framed={false}
              hasChildTracesError={hasLineageError}
              lineageChain={EMPTY_TRACES}
              lineageLoading={false}
              projectId={projectId}
              returnTo={returnTo}
              trace={trace}
            />
          ) : null}

          {(trace.input !== undefined || trace.output !== undefined) ? (
            <div className="grid gap-4 xl:grid-cols-2">
              {trace.input !== undefined ? (
                <TracePayloadPanel title="Input" data={trace.input} />
              ) : null}
              {trace.output !== undefined ? (
                <TracePayloadPanel title="Output" data={trace.output} />
              ) : null}
            </div>
          ) : null}
        </div>
      ) : null}
    </section>
  );
}

function TraceContextField({
  label,
  value,
  className = '',
  copyValue,
  copyButtonLabel,
}: {
  label: string;
  value: ReactNode;
  className?: string;
  copyValue?: string;
  copyButtonLabel?: string;
}) {
  return (
    <div className={`rounded-xl border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] p-4 ${className}`.trim()}>
      <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-[var(--continua-text-muted)]">
        {label}
      </div>
      <div className="mt-2 flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1 text-sm text-[var(--continua-text-primary)]">{value}</div>
        {copyValue && copyButtonLabel ? (
          <CopyButton
            aria-label={copyButtonLabel}
            className="shrink-0"
            value={copyValue}
          />
        ) : null}
      </div>
    </div>
  );
}

function TracePayloadPanel({ title, data }: { title: string; data: unknown }) {
  return (
    <div className="rounded-xl border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] p-4">
      <h3 className="mb-2 text-sm font-medium text-[var(--continua-text-secondary)]">{title}</h3>
      <JsonViewer data={data} className="max-h-80 overflow-y-auto bg-[var(--continua-surface-elevated)]" />
    </div>
  );
}

function TraceDetailEmptyState({ children }: { children: ReactNode }) {
  return (
    <div className="app-page">
      <div className="app-empty-state mt-0 flex min-h-[24rem] items-center justify-center px-6">
        {children}
      </div>
    </div>
  );
}

function TraceDetailErrorState({ children }: { children: ReactNode }) {
  return (
    <div className="app-page">
      <div className="app-alert-error">{children}</div>
    </div>
  );
}

function RunningStatePanel({
  assessment,
  events,
  openWaits,
  spanIndex,
  onSelectSpan,
}: {
  assessment: WaitStallAssessment;
  events: TimelineEvent[];
  openWaits: OpenWait[];
  spanIndex: ReadonlyMap<string, Span>;
  onSelectSpan: (spanId: string) => void;
}) {
  const waitKind = resolveDeclaredWaitKind(assessment, events);
  const panelTone = getRunningStatePanelTone(assessment.classification);
  const summary = getRunningStateSummary(assessment);
  const orderedOpenWaits = [...openWaits].reverse();

  return (
    <section className={`rounded-xl border px-4 py-4 shadow-sm ${panelTone}`}>
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-[11px] font-semibold uppercase tracking-[0.18em]">
            Running state
          </div>
          <div className="mt-2 flex flex-wrap items-center gap-2">
            <h2 className="text-base font-bold">{summary.label}</h2>
            <span className="rounded-full border border-current/15 px-2.5 py-1 text-[11px] font-medium uppercase tracking-[0.14em]">
              {formatRunningStateBasis(assessment.basis)}
            </span>
          </div>
          <p className="mt-2 max-w-3xl text-sm leading-6">{summary.copy}</p>
        </div>

        {assessment.decisiveSpanId ? (
          <button
            type="button"
            className="rounded-full border border-current/20 bg-[var(--continua-surface-elevated)]/70 px-3 py-1.5 text-xs font-medium transition hover:bg-[var(--continua-surface-elevated)]"
            onClick={() => onSelectSpan(assessment.decisiveSpanId!)}
          >
            Jump to {assessment.decisiveSpanName ?? assessment.decisiveSpanId}
          </button>
        ) : null}
      </div>

      <div className="mt-4 flex flex-wrap gap-4 text-xs">
        {waitKind ? (
          <div>
            <div className="font-semibold uppercase tracking-[0.16em] opacity-75">
              Current wait
            </div>
            <div className="mt-1 text-sm font-medium">{waitKind}</div>
          </div>
        ) : null}
        {assessment.latestActivityAt ? (
          <div>
            <div className="font-semibold uppercase tracking-[0.16em] opacity-75">
              Latest activity
            </div>
            <div className="mt-1 text-sm font-medium">
              {formatTimestamp(assessment.latestActivityAt)} (
              {formatRelativeTime(assessment.latestActivityAt)})
            </div>
          </div>
        ) : null}
        {assessment.runtimeMs !== null ? (
          <div>
            <div className="font-semibold uppercase tracking-[0.16em] opacity-75">
              Runtime
            </div>
            <div className="mt-1 text-sm font-medium">
              {formatDuration(assessment.runtimeMs)}
            </div>
          </div>
        ) : null}
        {assessment.inactivityMs !== null ? (
          <div>
            <div className="font-semibold uppercase tracking-[0.16em] opacity-75">
              Inactivity
            </div>
            <div className="mt-1 text-sm font-medium">
              {formatDuration(assessment.inactivityMs)}
            </div>
          </div>
        ) : null}
      </div>

      {orderedOpenWaits.length > 0 ? (
        <div className="mt-4 border-t border-current/15 pt-4">
          <div className="text-[11px] font-semibold uppercase tracking-[0.18em] opacity-75">
            Open waits
          </div>
          <div className="mt-3 space-y-3">
            {orderedOpenWaits.map((openWait) => (
              <OpenWaitRow
                key={openWait.event.id}
                openWait={openWait}
                spanIndex={spanIndex}
                onSelectSpan={onSelectSpan}
              />
            ))}
          </div>
        </div>
      ) : null}
    </section>
  );
}

function OpenWaitRow({
  openWait,
  spanIndex,
  onSelectSpan,
}: {
  openWait: OpenWait;
  spanIndex: ReadonlyMap<string, Span>;
  onSelectSpan: (spanId: string) => void;
}) {
  const waitTitle =
    openWait.details.waitKind === 'human_approval' ? 'Approval gate' : 'Wait gate';
  const openDurationMs = calculateDuration(openWait.event.timestamp, undefined);
  const hasNavigableSpan = Boolean(
    openWait.event.span_id && spanIndex.has(openWait.event.span_id)
  );

  return (
    <div className="rounded-lg border border-current/15 bg-[var(--continua-surface)]/60 p-3">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-sm font-semibold">{waitTitle}</div>
          <div className="mt-1 flex flex-wrap items-center gap-2 text-xs">
            <span className="rounded-full border border-current/15 px-2.5 py-1 font-medium">
              {openWait.details.waitKind}
            </span>
            {openWait.details.waitId ? (
              <span className="rounded-full border border-current/15 px-2.5 py-1 font-mono">
                {openWait.details.waitId}
              </span>
            ) : null}
          </div>
        </div>

        {hasNavigableSpan ? (
          <button
            type="button"
            className="rounded-full border border-current/20 bg-[var(--continua-surface-elevated)]/70 px-3 py-1.5 text-xs font-medium transition hover:bg-[var(--continua-surface-elevated)]"
            onClick={() => onSelectSpan(openWait.event.span_id!)}
          >
            Jump to {openWait.event.span_name ?? openWait.event.span_id}
          </button>
        ) : null}
      </div>

      <div className="mt-3 flex flex-wrap gap-4 text-xs">
        <div>
          <div className="font-semibold uppercase tracking-[0.16em] opacity-75">
            Entered
          </div>
          <div className="mt-1 text-sm font-medium">
            {formatTimestamp(openWait.event.timestamp)}
          </div>
        </div>
        <div>
          <div className="font-semibold uppercase tracking-[0.16em] opacity-75">
            Open duration
          </div>
          <div className="mt-1 text-sm font-medium">
            {formatDuration(openDurationMs)}
          </div>
        </div>
      </div>

      {openWait.event.message ? (
        <p className="mt-3 text-sm leading-6 opacity-90">{openWait.event.message}</p>
      ) : null}
    </div>
  );
}

function getRunningStateSummary(assessment: WaitStallAssessment): {
  label: string;
  copy: string;
} {
  switch (assessment.classification) {
    case 'declared_wait':
      return {
        label: 'Declared wait',
        copy: 'Execution declared a wait and has not yet recorded a matching resolution.',
      };
    case 'waiting_on_model':
      return {
        label: 'Waiting on model',
        copy: 'Execution appears to be waiting on an in-flight model span.',
      };
    case 'waiting_on_tool':
      return {
        label: 'Waiting on tool',
        copy: 'Execution appears to be waiting on an in-flight tool span.',
      };
    case 'actively_executing':
      return assessment.reason === 'recent_activity_without_open_span'
        ? {
            label: 'Actively executing',
            copy: 'Recent activity suggests execution is still progressing between spans.',
          }
        : {
            label: 'Actively executing',
            copy: 'A running span suggests execution is still actively progressing.',
          };
    case 'possibly_stalled':
      return {
        label: 'Possibly stalled',
        copy: 'Execution is still marked running, but recent activity is sparse.',
      };
    case 'unknown':
      return {
        label: 'Unknown',
        copy: 'The debugger cannot yet explain where it is waiting.',
      };
  }
}

function formatRunningStateBasis(basis: WaitStallAssessment['basis']): string {
  switch (basis) {
    case 'declared':
      return 'Declared';
    case 'inferred':
      return 'Inferred';
    case 'heuristic':
      return 'Heuristic';
  }
}

function getRunningStatePanelTone(
  classification: WaitStallAssessment['classification']
): string {
  switch (classification) {
    case 'declared_wait':
    case 'waiting_on_model':
    case 'waiting_on_tool':
      return 'border-sky-200 bg-sky-50 text-sky-950 dark:border-sky-500/30 dark:bg-sky-500/10 dark:text-sky-100';
    case 'actively_executing':
      return 'border-emerald-200 bg-emerald-50 text-emerald-950 dark:border-emerald-500/30 dark:bg-emerald-500/10 dark:text-emerald-100';
    case 'possibly_stalled':
      return 'border-amber-200 bg-amber-50 text-amber-950 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-100';
    case 'unknown':
      return 'border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] text-[var(--continua-text-primary)]';
  }
}

function resolveDeclaredWaitKind(
  assessment: WaitStallAssessment,
  events: TimelineEvent[]
): string | null {
  if (assessment.classification !== 'declared_wait' || !assessment.decisiveEventId) {
    return null;
  }

  const decisiveEvent = events.find((event) => event.id === assessment.decisiveEventId);
  return decisiveEvent ? getWaitDetails(decisiveEvent)?.waitKind ?? null : null;
}

function renderContextText(value: string | undefined, monospace = false) {
  if (value === undefined) {
    return <span className="text-sm text-[var(--continua-text-muted)]">-</span>;
  }

  return (
    <span
      className={
        monospace
          ? 'font-mono text-xs text-[var(--continua-text-primary)]'
          : 'text-sm text-[var(--continua-text-primary)]'
      }
    >
      {value}
    </span>
  );
}
