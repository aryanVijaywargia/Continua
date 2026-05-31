import {
  useCallback,
  useEffect,
  useRef,
  useState,
  useMemo,
  type ReactNode,
} from 'react';
import { Link, useLocation, useParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import {
  fetchTraces,
  fetchSpans,
  fetchTrace,
  fetchEngineRunHistory,
  fetchEngineRunResult,
  isAuthError,
  type EngineRunStatus,
  type EngineHistoryEvent,
  type Span,
  type Trace,
  type TimelineEvent,
  type TraceDetail,
  type EnginePendingWorkResponse,
} from '../api/client';
import { AuthErrorBanner } from '../components/AuthErrorBanner';
import { Btn, Chip, StatusDot } from '../components/DebuggerKit';
import { CopyButton } from '../components/CopyButton';
import { EngineProjectionBanner } from '../components/EngineProjectionBanner';
import { ExecutionWaterfall } from '../components/ExecutionWaterfall';
import { FailureSummary } from '../components/FailureSummary';
import { ReasoningTab } from '../components/ReasoningTab';
import { SpanDetail } from '../components/SpanDetail';
import { StatusBadge } from '../components/StatusBadge';
import { TreeRail } from '../components/TreeRail';
import { TruncationBanner } from '../components/TruncationBanner';
import {
  MobileWorkspaceTabId,
  WorkspaceShell,
} from '../components/WorkspaceShell';
import { useMediaQuery } from '../hooks/useMediaQuery';
import { useTraceDetailSearchParams } from '../hooks/useTraceDetailSearchParams';
import { useSpanExpansion } from '../hooks/useSpanExpansion';
import { useFailedSpanAutoSelect } from '../hooks/useFailedSpanAutoSelect';
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
  getAncestorIds,
  type SpanTreeNode,
} from '../utils/spanTree';
import { downloadJsonFile } from '../utils/downloadJson';
import {
  ChevronDown,
  ChevronRight,
  Copy as CopyIcon,
  Download,
  ExternalLink,
  Info,
  Pause,
  Play,
  RotateCcw,
  Search,
  SkipBack,
  SkipForward,
  Zap,
} from 'lucide-react';

const EMPTY_SPANS: Span[] = [];
const EMPTY_TRACES: Trace[] = [];
const EMPTY_RETRY_SAFETY_ASSESSMENTS = new Map<string, RetrySafetyAssessment>();
const DESKTOP_MEDIA_QUERY = '(min-width: 1024px)';
const TRACE_CONTEXT_DRAWER_MEDIA_QUERY = '(min-width: 768px)';
const MAX_LINEAGE_ANCESTOR_DEPTH = 64;
const CHILD_TRACES_PAGE_SIZE = 20;
type TraceDetailSectionId = 'overview' | 'timeline' | 'logs' | 'metrics' | 'engine' | 'replay';

function isReplayPreviewEnabled(): boolean {
  return import.meta.env.VITE_CONTINUA_REPLAY_PREVIEW === '1';
}

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
  const isContextDrawer = useMediaQuery(TRACE_CONTEXT_DRAWER_MEDIA_QUERY);
  const location = useLocation();
  const currentProjectId = getProjectIdFromSearchParams(
    new URLSearchParams(location.search)
  );
  const projectQueryKey = currentProjectId ?? null;
  const { spanParam, setSpanParam } = useTraceDetailSearchParams();
  const [activeMobileTab, setActiveMobileTab] =
    useState<MobileWorkspaceTabId>('summary');
  const [activeSection, setActiveSection] =
    useState<TraceDetailSectionId>('overview');
  const [isTraceContextOpen, setIsTraceContextOpen] = useState(false);
  const replayPreviewEnabled = isReplayPreviewEnabled();
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
    toggleExpandedSpan,
    revealAncestors,
    expandAll,
    collapseAll,
    setExact,
  } = useSpanExpansion(expandableSpanIds);

  // Selection derives from the URL — the single source of truth (see CONTEXT.md)
  // — but only when the param resolves to a real span. An unresolved param is
  // not a selection: it must not drive copy/export/replay or block auto-select.
  const selectedSpanId =
    spanParam !== null && spanIndex.has(spanParam) ? spanParam : null;
  const selectedSpan = selectedSpanId
    ? spanIndex.get(selectedSpanId) ?? null
    : null;

  // Clear a stale/invalid ?span= once span data has loaded, so the URL never
  // advertises a span that does not exist in this trace.
  useEffect(() => {
    if (!spansQuery.isSuccess) {
      return;
    }
    if (spanParam !== null && !spanIndex.has(spanParam)) {
      setSpanParam(null);
    }
  }, [spansQuery.isSuccess, spanParam, spanIndex, setSpanParam]);

  // Reveal is pure derivation from the selected span; no re-center counter.
  const revealPath = useMemo(() => {
    if (!selectedSpanId) {
      return new Set<string>();
    }
    return new Set([
      ...getAncestorIds(selectedSpanId, spanIndex),
      selectedSpanId,
    ]);
  }, [selectedSpanId, spanIndex]);

  // Expanding the selected span's ancestors is a side effect of *selection
  // change* — not of every span-data refresh. Polling rebuilds spanIndex by
  // identity each tick; without this guard the effect would re-fire and reopen
  // ancestors the operator manually collapsed. Track the last span we revealed
  // for and only act when it actually changes.
  const lastRevealedSpanId = useRef<string | null>(null);
  useEffect(() => {
    if (!selectedSpanId) {
      lastRevealedSpanId.current = null;
      return;
    }
    if (lastRevealedSpanId.current === selectedSpanId) {
      return;
    }
    lastRevealedSpanId.current = selectedSpanId;
    revealAncestors(getAncestorIds(selectedSpanId, spanIndex));
  }, [revealAncestors, selectedSpanId, spanIndex]);

  const selectSpan = useCallback(
    (spanId: string) => {
      setSpanParam(spanId);
    },
    [setSpanParam]
  );

  useFailedSpanAutoSelect({
    traceId,
    isReady: spansQuery.isSuccess,
    traceStatus: timelineStatus,
    spanParam,
    selectedSpanId,
    primaryFailedSpanId:
      failureAnalysis.summary.primaryFailedSpan?.span_id ?? null,
    setSpanParam,
  });
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

  useEffect(() => {
    if (activeSection === 'engine' && !trace?.engine) {
      setActiveSection('overview');
    }
    if (activeSection === 'replay' && !replayPreviewEnabled) {
      setActiveSection('overview');
    }
  }, [activeSection, replayPreviewEnabled, trace?.engine]);

  const handleSelectSpan = useCallback((spanId: string) => {
    selectSpan(spanId);
  }, [selectSpan]);

  const handleSelectSpanAndShowDetails = useCallback((spanId: string) => {
    selectSpan(spanId);
    setActiveMobileTab('summary');
  }, [selectSpan]);

  const buildCopyTraceUrl = useCallback(() => {
    const searchParams = serializeSpanParam(
      new URLSearchParams(location.search),
      selectedSpanId
    );
    const nextSearch = searchParams.toString();

    return new URL(
      `${location.pathname}${nextSearch ? `?${nextSearch}` : ''}`,
      window.location.origin
    ).toString();
  }, [location.pathname, location.search, selectedSpanId]);
  const handleExportTrace = useCallback(() => {
    downloadJsonFile(`continua-trace-${traceId}.json`, {
      exported_at: new Date().toISOString(),
      trace,
      spans,
      timeline: timeline.events,
      selected_span_id: selectedSpanId,
    });
  }, [selectedSpanId, spans, timeline.events, trace, traceId]);

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

  const reasoningContent = (
    <ReasoningTab
      entries={reasoningEntries}
      onSelectSpan={handleSelectSpanAndShowDetails}
    />
  );
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

  const workspaceContent = (
    <TraceWorkspace
      activeMobileTab={activeMobileTab}
      events={timeline.events}
      expandableSpanIds={expandableSpanIds}
      expandedSpanIds={expandedSpanIds}
      failedSpanIds={failureAnalysis.failedSpanIds}
      inlineErrorPreviews={failureAnalysis.inlineErrorPreviews}
      isDesktop={isDesktop}
      mobileSummaryContent={mobileSummaryContent}
      onMobileTabChange={setActiveMobileTab}
      onSelectSpan={handleSelectSpan}
      onSelectSpanAndShowDetails={handleSelectSpanAndShowDetails}
      onToggleExpand={toggleExpandedSpan}
      primaryAncestorPath={failureAnalysis.primaryAncestorPath}
      revealPath={revealPath}
      revealTarget={selectedSpanId}
      selectedSpanId={selectedSpanId}
      expandAll={expandAll}
      collapseAll={collapseAll}
      setExact={setExact}
      spanAssessments={visibleRetrySafetyAssessments}
      spanIndex={spanIndex}
      spanTree={spanTree}
      spans={spans}
      traceCostSeries={traceCostSeries}
      traceEndedAt={trace.ended_at}
      traceStartedAt={trace.started_at}
    />
  );

  const sectionContent =
    activeSection === 'overview' ? (
      workspaceContent
    ) : activeSection === 'timeline' ? (
      <TraceSectionSurface
        flush
        title="Timeline"
        description="Chronological trace events with span selection preserved."
      >
        <TraceTimelinePanel
          events={timeline.events}
          onSelectSpan={handleSelectSpanAndShowDetails}
          spanIndex={spanIndex}
          spans={spans}
          traceEndedAt={trace.ended_at}
          traceStartedAt={trace.started_at}
        />
      </TraceSectionSurface>
    ) : activeSection === 'logs' ? (
      <TraceSectionSurface
        flush
        title="Logs"
        description="Explicit logs, errors, exceptions, decisions, effects, and waits recorded by the trace."
      >
        <TraceLogsPanel
          events={timeline.events}
          onSelectSpan={handleSelectSpanAndShowDetails}
          spanIndex={spanIndex}
        />
      </TraceSectionSurface>
    ) : activeSection === 'metrics' ? (
      <TraceSectionSurface
        title="Metrics"
        description="Aggregate latency, token, cost, and state-change signals from the loaded spans."
      >
        <TraceMetricsPanel
          events={timeline.events}
          spans={spans}
          trace={trace}
          traceCostSeries={traceCostSeries}
        />
      </TraceSectionSurface>
    ) : activeSection === 'engine' ? (
      <TraceSectionSurface
        flush
        title="Engine state"
        description="Current engine projection, wait state, and queued work for this trace."
      >
        <TraceEngineStatePanel
          engine={trace.engine}
          events={timeline.events}
          errorMessage={queryErrorMessage(pendingWorkQuery.error)}
          isError={pendingWorkQuery.isError}
          isLoading={pendingWorkQuery.isLoading}
          onSelectSpan={handleSelectSpanAndShowDetails}
          openWaits={openWaits}
          pendingWork={pendingWorkQuery.data}
          runningStateAssessment={waitStallAssessment}
          spanIndex={spanIndex}
          traceId={traceId}
        />
      </TraceSectionSurface>
    ) : activeSection === 'replay' && replayPreviewEnabled ? (
      <TraceSectionSurface
        flush
        title="Replay"
        description="Replay readiness and export actions for this trace."
      >
        <TraceReplayPanel
          events={timeline.events}
          onExportTrace={handleExportTrace}
          selectedSpanId={selectedSpanId}
          spans={spans}
          trace={trace}
        />
      </TraceSectionSurface>
    ) : (
      workspaceContent
    );

  return (
    <div className="relative flex min-h-0 flex-1 flex-col">
      <header className="border-b border-[var(--c-border)] bg-[var(--c-app-bg)] px-6 py-3.5">
        <div className="flex flex-col gap-5 xl:flex-row xl:items-start xl:justify-between">
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2 text-xs text-[var(--c-text-muted)]">
              <Link
                to={returnTo}
                aria-label={
                  returnTo.startsWith('/sessions/')
                    ? '← Session'
                    : returnTo.startsWith('/engine/runs')
                      ? '← Engine Runs'
                      : '← Traces'
                }
                className="inline-flex items-center gap-1 font-medium text-[var(--c-text-secondary)] transition hover:text-[var(--c-accent-text)]"
              >
                ‹ {returnTo.startsWith('/sessions/')
                  ? 'Session'
                  : returnTo.startsWith('/engine/runs')
                    ? 'Engine Runs'
                    : 'Traces'}
              </Link>
              <span aria-hidden="true">›</span>
              <span className="font-mono">{trace.trace_id ?? trace.id}</span>
              <CopyButton
                aria-label="Copy Trace URL"
                getValue={buildCopyTraceUrl}
                idleLabel=""
                successLabel=""
                className="h-5 min-w-5 border-0 bg-transparent px-1 text-[var(--c-text-muted)] shadow-none hover:text-[var(--c-text-primary)]"
              />
            </div>

            <div className="mt-2 flex flex-wrap items-center gap-2.5">
              <h1 className="truncate font-mono text-lg font-semibold tracking-[-0.01em] text-[var(--c-text-primary)]">
                {trace.name}
              </h1>
              <StatusDot status={timelineStatus ?? trace.status} />
              {trace.engine ? (
                <Chip icon={Zap}>
                  {trace.engine.definition_name}
                </Chip>
              ) : null}
              {trace.error_count && trace.error_count > 0 ? (
                <Chip tone="error">{trace.error_count} error{trace.error_count === 1 ? '' : 's'}</Chip>
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
              <div className="mt-3 flex flex-wrap gap-2 text-sm">
                {trace.engine.continued_from_trace_id ? (
                  <Link
                    to={appendProjectToPath(
                      `/traces/${trace.engine.continued_from_trace_id}`,
                      currentProjectId
                    )}
                    state={{ returnTo }}
                    className="inline-flex items-center rounded border border-[var(--c-border)] bg-[var(--c-surface-muted)] px-2.5 py-1 text-xs font-medium text-[var(--c-text-secondary)] transition hover:border-[var(--c-border-strong)] hover:text-[var(--c-accent-text)]"
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
                    className="inline-flex items-center rounded border border-[var(--c-border)] bg-[var(--c-surface-muted)] px-2.5 py-1 text-xs font-medium text-[var(--c-text-secondary)] transition hover:border-[var(--c-border-strong)] hover:text-[var(--c-accent-text)]"
                  >
                    Next run →
                  </Link>
                ) : null}
              </div>
            ) : null}

            {trace.engine?.status === 'WAITING' ? (
              <EngineWaitStateSummary engine={trace.engine} />
            ) : null}

            <div className="mt-3 flex flex-wrap gap-7">
              <TraceHeaderMetric label="Duration" value={formatDuration(duration)} />
              <TraceHeaderMetric label="Spans" value={String(spans.length)} />
              <TraceHeaderMetric label="Tokens" value={formatTokens(totalTokens)} />
              <TraceHeaderMetric label="Cost" value={formatCost(trace.total_cost_usd)} />
              <TraceHeaderMetric label="Started" value={formatRelativeTime(trace.started_at)} />
              <TraceHeaderMetric label="User" value={trace.user_id ?? '—'} mono />
            </div>
          </div>

          <div className="flex flex-wrap gap-1.5 xl:justify-end">
            {replayPreviewEnabled ? (
              <Btn
                kind="secondary"
                leadingIcon={RotateCcw}
                size="sm"
                type="button"
                onClick={() => setActiveSection('replay')}
              >
                Replay
              </Btn>
            ) : null}
            <Btn kind="secondary" leadingIcon={Download} size="sm" type="button" onClick={handleExportTrace}>
              Export JSON
            </Btn>
            <Btn
              kind="secondary"
              leadingIcon={ExternalLink}
              size="sm"
              type="button"
              disabled
              title="Coming soon"
            >
              Open in workspace
            </Btn>
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

      <TraceDetailTabs
        activeSection={activeSection}
        eventCount={timeline.events.length}
        hasEngine={Boolean(trace.engine)}
        onChange={setActiveSection}
        replayPreviewEnabled={replayPreviewEnabled}
        spanCount={spans.length}
      />

      {timelineAuthError ? (
        <AuthErrorBanner message={queryErrorMessage(timeline.rawError)} />
      ) : null}

      {trace.engine?.failure?.error_code === 'definition_version_mismatch' ? (
        <DefinitionVersionMismatchBanner />
      ) : null}
      <div className="flex min-h-0 flex-1 flex-col">
        {sectionContent}
      </div>

      {isContextDrawer && isTraceContextOpen ? (
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

      {!isContextDrawer && isTraceContextOpen ? (
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
    <section className="mt-3 rounded-md border border-[var(--c-blue-border)] bg-[var(--c-blue-faint)] px-3 py-2 text-[var(--c-blue-text)]">
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="text-sm font-semibold">{summary.heading}</h2>
        <span className="text-[11px] font-medium uppercase tracking-[0.08em] opacity-75">
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
  events: TimelineEvent[];
  expandableSpanIds: ReadonlySet<string>;
  expandedSpanIds: ReadonlySet<string>;
  failedSpanIds: ReadonlySet<string>;
  inlineErrorPreviews: ReadonlyMap<string, string>;
  isDesktop: boolean;
  mobileSummaryContent: ReactNode;
  onMobileTabChange: (tab: MobileWorkspaceTabId) => void;
  onSelectSpan: (spanId: string) => void;
  onSelectSpanAndShowDetails: (spanId: string) => void;
  onToggleExpand: (spanId: string) => void;
  primaryAncestorPath: ReadonlySet<string>;
  revealPath: ReadonlySet<string>;
  revealTarget: string | null;
  selectedSpanId: string | null;
  expandAll: () => void;
  collapseAll: () => void;
  setExact: (expanded: ReadonlySet<string>) => void;
  spanAssessments: ReadonlyMap<string, RetrySafetyAssessment>;
  spanIndex: ReadonlyMap<string, Span>;
  spanTree: SpanTreeNode[];
  spans: Span[];
  traceCostSeries: TraceCostSeries | null;
  traceEndedAt?: string;
  traceStartedAt?: string;
}

function TraceWorkspace({
  activeMobileTab,
  events,
  expandableSpanIds,
  expandedSpanIds,
  failedSpanIds,
  inlineErrorPreviews,
  isDesktop,
  mobileSummaryContent,
  onMobileTabChange,
  onSelectSpan,
  onSelectSpanAndShowDetails,
  onToggleExpand,
  primaryAncestorPath,
  revealPath,
  revealTarget,
  selectedSpanId,
  expandAll,
  collapseAll,
  setExact,
  spanAssessments,
  spanIndex,
  spanTree,
  spans,
  traceCostSeries,
  traceEndedAt,
  traceStartedAt,
}: TraceWorkspaceProps) {
  const [visibleRows, setVisibleRows] = useState(() =>
    deriveVisibleRows(spanTree, expandedSpanIds)
  );
  const selectedSpan = selectedSpanId ? spanIndex.get(selectedSpanId) ?? null : null;

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
          revealPath={revealPath}
          selectedSpanId={selectedSpanId}
          expandAll={expandAll}
          collapseAll={collapseAll}
          setExact={setExact}
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
          spans={spans}
          costSeries={traceCostSeries}
          spanAssessments={spanAssessments}
          traceEndedAt={traceEndedAt}
          traceStartedAt={traceStartedAt}
        />
      }
      inspector={
        <SpanInspectorPanel
          events={events}
          selectedSpan={selectedSpan}
        />
      }
      mobileSummary={mobileSummaryContent}
      activeMobileTab={activeMobileTab}
      onMobileTabChange={onMobileTabChange}
    />
  );
}

function SpanInspectorPanel({
  events,
  selectedSpan,
}: {
  events: TimelineEvent[];
  selectedSpan: Span | null;
}) {
  const [activeTab, setActiveTab] = useState<'input' | 'output' | 'attributes' | 'events'>('input');

  if (!selectedSpan) {
    return (
      <section className="flex h-full items-center justify-center bg-[var(--c-app-bg)] text-sm text-[var(--c-text-muted)]">
        Select a span to inspect payloads.
      </section>
    );
  }

  const spanEvents = events.filter((event) => event.span_id === selectedSpan.span_id);
  const tabs = [
    { id: 'input' as const, label: 'Input' },
    { id: 'output' as const, label: 'Output' },
    { id: 'attributes' as const, label: 'Attributes' },
    { id: 'events' as const, label: 'Events', count: spanEvents.length },
  ];

  return (
    <section className="flex h-full min-h-0 flex-col overflow-hidden bg-[var(--c-app-bg)]">
      <div className="border-b border-[var(--c-border)] px-4 py-3">
        <div className="flex items-center gap-2">
          <span
            className="h-1.5 w-1.5 rounded-full"
            style={{
              background:
                selectedSpan.status === 'FAILED'
                  ? 'var(--c-red)'
                  : selectedSpan.status === 'STARTED'
                    ? 'var(--c-blue)'
                    : 'var(--c-green)',
            }}
          />
          <h2 className="min-w-0 truncate font-mono text-[13px] font-semibold text-[var(--c-text-primary)]">
            {selectedSpan.name}
          </h2>
        </div>
        <div className="mt-2 flex gap-4 text-[11.5px] text-[var(--c-text-muted)]">
          <span className="font-mono">{formatDuration(selectedSpan.latency_ms)}</span>
          <span>kind: {selectedSpan.kind.toLowerCase()}</span>
          <span>{selectedSpan.status.toLowerCase()}</span>
        </div>
      </div>

      <div className="flex border-b border-[var(--c-border)] px-3">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            type="button"
            aria-pressed={activeTab === tab.id}
            onClick={() => setActiveTab(tab.id)}
            className={`-mb-px border-b-2 px-3 py-2 text-xs font-medium ${
              activeTab === tab.id
                ? 'border-[var(--c-accent)] text-[var(--c-text-primary)]'
                : 'border-transparent text-[var(--c-text-secondary)] hover:text-[var(--c-text-primary)]'
            }`}
          >
            {tab.label}
            {tab.count != null ? (
              <span className="ml-1 font-mono text-[10.5px] text-[var(--c-text-muted)]">
                {tab.count}
              </span>
            ) : null}
          </button>
        ))}
      </div>

      <div className="min-h-0 flex-1 overflow-auto p-3">
        {activeTab === 'input' ? (
          selectedSpan.input !== undefined ? (
            <>
              <TruncationBanner
                title="Input payload"
                truncated={selectedSpan.input_truncated}
                originalSizeBytes={selectedSpan.input_original_size_bytes}
                reason={selectedSpan.input_truncation_reason}
              />
              <CompactPayloadInspector value={selectedSpan.input} />
            </>
          ) : (
            <InspectorEmptyState>No input payload recorded.</InspectorEmptyState>
          )
        ) : null}

        {activeTab === 'output' ? (
          selectedSpan.output !== undefined ? (
            <>
              <TruncationBanner
                title="Output payload"
                truncated={selectedSpan.output_truncated}
                originalSizeBytes={selectedSpan.output_original_size_bytes}
                reason={selectedSpan.output_truncation_reason}
              />
              <CompactPayloadInspector value={selectedSpan.output} />
            </>
          ) : selectedSpan.error_message ? (
            <pre className="whitespace-pre-wrap rounded border border-[var(--c-red-border)] bg-[var(--c-red-faint)] p-3 font-mono text-xs leading-6 text-[var(--c-red-text)]">
              {selectedSpan.error_message}
            </pre>
          ) : (
            <InspectorEmptyState>No output payload recorded.</InspectorEmptyState>
          )
        ) : null}

        {activeTab === 'attributes' ? (
          <div className="divide-y divide-[var(--c-border-subtle)] text-xs">
            {[
              ['span.id', selectedSpan.id],
              ['span.span_id', selectedSpan.span_id],
              ['trace.id', selectedSpan.trace_id],
              ['span.kind', selectedSpan.kind.toLowerCase()],
              ['span.status', selectedSpan.status.toLowerCase()],
              ['span.started_at', formatTimestamp(selectedSpan.started_at)],
              ['span.ended_at', selectedSpan.ended_at ? formatTimestamp(selectedSpan.ended_at) : '—'],
              ['span.duration', formatDuration(selectedSpan.latency_ms)],
              ['model', selectedSpan.model ?? '—'],
              ['provider', selectedSpan.provider ?? '—'],
            ].map(([key, value]) => (
              <div key={key} className="grid grid-cols-[8.5rem_minmax(0,1fr)] gap-3 py-2">
                <span className="font-mono text-[var(--c-text-muted)]">{key}</span>
                <span className="min-w-0 truncate font-mono text-[var(--c-text-primary)]">{value}</span>
              </div>
            ))}
            {selectedSpan.metadata && Object.keys(selectedSpan.metadata).length > 0 ? (
              <div className="py-3">
                <div className="mb-2 font-mono text-[var(--c-text-muted)]">metadata</div>
                <CompactPayloadInspector value={selectedSpan.metadata} />
              </div>
            ) : null}
          </div>
        ) : null}

        {activeTab === 'events' ? (
          spanEvents.length > 0 ? (
            <div className="divide-y divide-[var(--c-border-subtle)]">
              {spanEvents.map((event) => (
                <div key={event.id} className="py-2 text-xs">
                  <div className="flex items-center justify-between gap-3">
                    <span className="font-mono text-[var(--c-text-primary)]">{event.event_type}</span>
                    <span className="font-mono text-[var(--c-text-muted)]">
                      {formatTimestamp(event.timestamp)}
                    </span>
                  </div>
                  {event.payload !== undefined ? (
                    <div className="mt-2">
                      <CompactPayloadInspector value={event.payload} />
                    </div>
                  ) : null}
                </div>
              ))}
            </div>
          ) : (
            <InspectorEmptyState>No events recorded for this span.</InspectorEmptyState>
          )
        ) : null}
      </div>
    </section>
  );
}

function InspectorEmptyState({ children }: { children: ReactNode }) {
  return (
    <div className="rounded border border-dashed border-[var(--c-border)] bg-[var(--c-surface-muted)] px-4 py-6 text-sm text-[var(--c-text-muted)]">
      {children}
    </div>
  );
}

function CompactPayloadInspector({
  depth = 0,
  isLast = true,
  name,
  value,
}: {
  depth?: number;
  isLast?: boolean;
  name?: string | number;
  value: unknown;
}) {
  const [open, setOpen] = useState(depth < 2);
  const isObject = value !== null && typeof value === 'object';

  if (!isObject) {
    return (
      <div className="flex font-mono text-xs leading-6">
        {name !== undefined ? (
          <>
            <span className="text-[var(--c-text-secondary)]">"{String(name)}"</span>
            <span className="text-[var(--c-text-muted)]">: </span>
          </>
        ) : null}
        <PayloadPrimitive value={value} />
        {!isLast ? <span className="text-[var(--c-text-muted)]">,</span> : null}
      </div>
    );
  }

  const isArray = Array.isArray(value);
  const entries = isArray
    ? (value as unknown[]).map((entry, index) => [index, entry] as const)
    : Object.entries(value as Record<string, unknown>);
  const openToken = isArray ? '[' : '{';
  const closeToken = isArray ? ']' : '}';

  return (
    <div className="font-mono text-xs leading-6">
      <div className="flex items-center gap-1">
        <button
          type="button"
          aria-label={`${open ? 'Collapse' : 'Expand'} ${name ?? 'payload'}`}
          className="flex h-4 w-4 items-center justify-center text-[var(--c-text-muted)] hover:text-[var(--c-text-primary)]"
          onClick={() => setOpen((current) => !current)}
        >
          {open ? <ChevronDown className="h-3 w-3" /> : <ChevronRight className="h-3 w-3" />}
        </button>
        {name !== undefined ? (
          <>
            <span className="text-[var(--c-text-secondary)]">"{String(name)}"</span>
            <span className="text-[var(--c-text-muted)]">: </span>
          </>
        ) : null}
        <span className="text-[var(--c-text-secondary)]">{openToken}</span>
        {!open ? (
          <>
            <span className="mx-1 text-[var(--c-text-muted)]">
              {entries.length} item{entries.length === 1 ? '' : 's'}
            </span>
            <span className="text-[var(--c-text-secondary)]">{closeToken}</span>
            {!isLast ? <span className="text-[var(--c-text-muted)]">,</span> : null}
          </>
        ) : null}
      </div>
      {open ? (
        <>
          <div className="ml-[5px] border-l border-[var(--c-border-subtle)] pl-4">
            {entries.map(([entryName, entryValue], index) => (
              <CompactPayloadInspector
                key={String(entryName)}
                depth={depth + 1}
                isLast={index === entries.length - 1}
                name={isArray ? undefined : entryName}
                value={entryValue}
              />
            ))}
          </div>
          <div className="pl-1 text-[var(--c-text-secondary)]">
            {closeToken}
            {!isLast ? <span className="text-[var(--c-text-muted)]">,</span> : null}
          </div>
        </>
      ) : null}
    </div>
  );
}

function PayloadPrimitive({ value }: { value: unknown }) {
  if (value === null) {
    return <span className="text-[var(--c-text-muted)]">null</span>;
  }
  if (value === undefined) {
    return <span className="text-[var(--c-text-muted)]">undefined</span>;
  }
  if (typeof value === 'string') {
    return <span className="break-all text-[var(--c-amber-text)]">"{value}"</span>;
  }
  if (typeof value === 'number' || typeof value === 'boolean') {
    return <span className="text-[var(--c-blue-text)]">{String(value)}</span>;
  }
  return <span className="text-[var(--c-text-primary)]">{String(value)}</span>;
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
    returnTo === '/engine/runs' ||
    returnTo.startsWith('/engine/runs?') ||
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
    <div className="flex h-full min-h-0 flex-col overflow-y-auto bg-[var(--c-app-bg)]">
      <div className="space-y-3 p-3">
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

        <div className="min-h-[22rem] overflow-hidden border-t border-[var(--c-border)] bg-[var(--c-app-bg)]">
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

function TraceDetailTabs({
  activeSection,
  eventCount,
  hasEngine,
  onChange,
  replayPreviewEnabled,
  spanCount,
}: {
  activeSection: TraceDetailSectionId;
  eventCount: number;
  hasEngine: boolean;
  onChange: (section: TraceDetailSectionId) => void;
  replayPreviewEnabled: boolean;
  spanCount: number;
}) {
  const tabs: Array<{ id: TraceDetailSectionId; label: string; count?: number }> = [
    { id: 'overview', label: 'Overview' },
    { id: 'timeline', label: 'Timeline', count: spanCount },
    { id: 'logs', label: 'Logs', count: eventCount },
    { id: 'metrics', label: 'Metrics' },
    ...(hasEngine ? [{ id: 'engine' as const, label: 'Engine state' }] : []),
    ...(replayPreviewEnabled ? [{ id: 'replay' as const, label: 'Replay' }] : []),
  ];

  return (
    <nav
      aria-label="Trace detail sections"
      className="flex border-b border-[var(--c-border)] bg-[var(--c-app-bg)] px-6"
    >
      {tabs.map((tab) => (
        <button
          key={tab.id}
          type="button"
          aria-pressed={activeSection === tab.id}
          onClick={() => onChange(tab.id)}
          className={`-mb-px border-b-2 px-3.5 py-2 text-[13px] font-medium ${
            activeSection === tab.id
              ? 'border-[var(--c-accent)] text-[var(--c-text-primary)]'
              : 'border-transparent text-[var(--c-text-secondary)] hover:text-[var(--c-text-primary)]'
          }`}
        >
          {tab.label}
          {tab.count != null ? (
            <span className="ml-1.5 font-mono text-[11px] text-[var(--c-text-muted)]">
              {tab.count}
            </span>
          ) : null}
        </button>
      ))}
    </nav>
  );
}

function TraceSectionSurface({
  children,
  description,
  flush = false,
  title,
}: {
  children: ReactNode;
  description: string;
  flush?: boolean;
  title: string;
}) {
  return (
    <section className="flex h-full min-h-0 flex-col overflow-hidden bg-[var(--c-app-bg)]">
      <div className="border-b border-[var(--c-border)] px-6 py-3">
        <h2 className="text-[11px] font-semibold uppercase tracking-[0.16em] text-[var(--c-text-muted)]">
          {title}
        </h2>
        <p className="mt-1 text-sm text-[var(--c-text-secondary)]">{description}</p>
      </div>
      <div className={flush ? 'flex min-h-0 flex-1 flex-col overflow-hidden' : 'min-h-0 flex-1 overflow-auto p-4'}>
        {children}
      </div>
    </section>
  );
}

type TimelineRowType = 'engine' | 'span' | 'log';
type TimelineRowLevel = 'info' | 'warn' | 'error';

interface TimelineRow {
  id: string;
  offsetMs: number;
  type: TimelineRowType;
  level: TimelineRowLevel;
  spanId?: string;
  spanName: string;
  message: string;
  meta?: string;
  rawEvent: TimelineEvent;
}

const TIMELINE_FILTERS: Array<{ id: TimelineRowType | 'all'; label: string }> = [
  { id: 'all', label: 'All' },
  { id: 'engine', label: 'Engine' },
  { id: 'span', label: 'Spans' },
  { id: 'log', label: 'Logs' },
];

function classifyTimelineRow(event: TimelineEvent, rootSpanId: string | undefined): TimelineRowType {
  if (
    event.event_type === 'span_started' ||
    event.event_type === 'span_completed' ||
    event.event_type === 'span_failed'
  ) {
    return 'span';
  }
  if (
    event.event_type === 'snapshot_marker' ||
    event.event_type === 'state_change' ||
    event.event_type === 'decision' ||
    (event.source === 'synthetic' && event.span_id === rootSpanId)
  ) {
    return 'engine';
  }
  return 'log';
}

function classifyTimelineLevel(event: TimelineEvent): TimelineRowLevel {
  if (event.event_type === 'error' || event.event_type === 'exception' || event.event_type === 'span_failed') {
    return 'error';
  }
  if (event.level === 'error') return 'error';
  if (event.level === 'warning' || event.event_type === 'wait') return 'warn';
  return 'info';
}

function timelineRowMessage(event: TimelineEvent): string {
  if (event.message) return event.message;
  switch (event.event_type) {
    case 'span_started':
      return `Span started${event.span_name ? ` · ${event.span_name}` : ''}`;
    case 'span_completed':
      return `Span completed${event.span_name ? ` · ${event.span_name}` : ''}`;
    case 'span_failed':
      return `Span failed${event.span_name ? ` · ${event.span_name}` : ''}`;
    case 'snapshot_marker':
      return 'Checkpoint persisted';
    case 'state_change':
      return 'State change';
    case 'decision':
      return 'Decision recorded';
    case 'effect':
      return 'Effect recorded';
    case 'wait':
      return 'Awaiting signal';
    default:
      return event.event_type.replace(/_/g, ' ');
  }
}

function timelineRowMeta(event: TimelineEvent): string | undefined {
  if (!event.payload) return undefined;
  const entries = Object.entries(event.payload).filter(
    ([key]) => key !== 'message' && key !== 'kind' && key !== 'level',
  );
  if (entries.length === 0) return undefined;
  const compact: string[] = [];
  for (const [key, value] of entries.slice(0, 3)) {
    if (value === null || value === undefined) continue;
    if (typeof value === 'object') continue;
    compact.push(`${key}=${String(value)}`);
  }
  return compact.length > 0 ? compact.join(' · ') : undefined;
}

function formatOffset(ms: number): string {
  if (ms < 1000) return `+${ms}ms`;
  return `+${(ms / 1000).toFixed(2)}s`;
}

function TimelinePhaseRibbon({
  phases,
  totalMs,
}: {
  phases: Array<{ id: string; name: string; status: string; startMs: number; durationMs: number }>;
  totalMs: number;
}) {
  if (totalMs <= 0 || phases.length === 0) return null;
  return (
    <>
      {phases.map((phase) => {
        const left = (phase.startMs / totalMs) * 100;
        const width = Math.max((phase.durationMs / totalMs) * 100, 0.4);
        const tone =
          phase.status === 'FAILED'
            ? { bg: 'rgba(239,68,68,0.16)', border: 'var(--c-bar-failed)' }
            : phase.status === 'STARTED'
              ? { bg: 'rgba(59,130,246,0.16)', border: 'var(--c-bar-running)' }
              : { bg: 'rgba(16,185,129,0.18)', border: 'var(--c-bar-success)' };
        return (
          <div
            key={phase.id}
            className="absolute top-1 bottom-1 pointer-events-auto"
            style={{
              left: `${left}%`,
              width: `${width}%`,
              background: tone.bg,
              borderLeft: `2px solid ${tone.border}`,
            }}
            title={`${phase.name} · ${formatDuration(phase.durationMs)}`}
          />
        );
      })}
    </>
  );
}

function TraceTimelinePanel({
  events,
  onSelectSpan,
  spanIndex,
  spans,
  traceEndedAt,
  traceStartedAt,
}: {
  events: TimelineEvent[];
  onSelectSpan: (spanId: string) => void;
  spanIndex: ReadonlyMap<string, Span>;
  spans: Span[];
  traceEndedAt?: string;
  traceStartedAt: string;
}) {
  const [zoom, setZoom] = useState(1);
  const [filter, setFilter] = useState<TimelineRowType | 'all'>('all');

  const traceStartMs = useMemo(() => new Date(traceStartedAt).getTime(), [traceStartedAt]);
  const traceEndMs = useMemo(
    () => (traceEndedAt ? new Date(traceEndedAt).getTime() : Date.now()),
    [traceEndedAt],
  );
  const totalMs = Math.max(traceEndMs - traceStartMs, 1);

  const rootSpanId = useMemo(() => {
    const root = spans.find((span) => !span.parent_span_id);
    return root?.id;
  }, [spans]);

  const phases = useMemo(() => {
    const topLevel = spans.filter((span) => !span.parent_span_id || span.parent_span_id === rootSpanId);
    return topLevel
      .map((span) => {
        const startMs = new Date(span.started_at).getTime() - traceStartMs;
        const endMs = span.ended_at ? new Date(span.ended_at).getTime() - traceStartMs : totalMs;
        return {
          id: span.id,
          name: span.name,
          status: span.status,
          startMs: Math.max(startMs, 0),
          durationMs: Math.max(endMs - startMs, 0),
        };
      })
      .filter((phase) => phase.durationMs > 0);
  }, [rootSpanId, spans, totalMs, traceStartMs]);

  const rows = useMemo<TimelineRow[]>(() => {
    return events
      .map((event) => {
        const offsetMs = Math.max(new Date(event.timestamp).getTime() - traceStartMs, 0);
        return {
          id: event.id,
          offsetMs,
          type: classifyTimelineRow(event, rootSpanId),
          level: classifyTimelineLevel(event),
          spanId: event.span_id,
          spanName: event.span_name ?? event.span_id ?? '—',
          message: timelineRowMessage(event),
          meta: timelineRowMeta(event),
          rawEvent: event,
        };
      })
      .sort((a, b) => a.offsetMs - b.offsetMs);
  }, [events, rootSpanId, traceStartMs]);

  const counts = useMemo(() => {
    const counts: Record<TimelineRowType, number> = { engine: 0, span: 0, log: 0 };
    rows.forEach((row) => {
      counts[row.type] += 1;
    });
    return counts;
  }, [rows]);

  const filteredRows = filter === 'all' ? rows : rows.filter((row) => row.type === filter);

  const filterCount = (id: TimelineRowType | 'all') =>
    id === 'all' ? rows.length : counts[id];

  const typeColor: Record<TimelineRowType, string> = {
    engine: 'var(--c-accent-text)',
    span: 'var(--c-text-secondary)',
    log: 'var(--c-text-muted)',
  };
  const levelColor: Record<TimelineRowLevel, string> = {
    info: 'var(--c-text-muted)',
    warn: 'var(--c-amber-text)',
    error: 'var(--c-red-text)',
  };

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex items-center justify-between border-b border-[var(--c-border-subtle)] px-4 py-2.5">
        <div className="flex gap-1 rounded-md border border-[var(--c-border)] bg-[var(--c-surface-muted)] p-0.5">
          {TIMELINE_FILTERS.map((option) => {
            const active = filter === option.id;
            return (
              <button
                key={option.id}
                type="button"
                onClick={() => setFilter(option.id)}
                className={`inline-flex items-center gap-1.5 rounded px-2.5 py-1 text-[11.5px] font-medium transition ${
                  active
                    ? 'border border-[var(--c-border)] bg-[var(--c-surface)] text-[var(--c-text-primary)]'
                    : 'border border-transparent text-[var(--c-text-secondary)] hover:text-[var(--c-text-primary)]'
                }`}
              >
                {option.label}
                <span className="font-mono text-[10.5px] text-[var(--c-text-muted)]">
                  {filterCount(option.id)}
                </span>
              </button>
            );
          })}
        </div>
        <label className="flex items-center gap-2.5 text-[11.5px] text-[var(--c-text-muted)]">
          <span>Zoom</span>
          <input
            type="range"
            min={1}
            max={4}
            step={0.5}
            value={zoom}
            onChange={(event) => setZoom(parseFloat(event.target.value))}
            className="h-1 w-24 cursor-pointer accent-[var(--c-accent)]"
            aria-label="Zoom"
          />
          <span className="min-w-[32px] font-mono">{zoom}×</span>
        </label>
      </div>

      <div className="border-b border-[var(--c-border-subtle)] px-4 py-3">
        <div className="overflow-x-auto">
          <div style={{ width: `${zoom * 100}%`, minWidth: '100%' }}>
            <div className="relative h-8 overflow-hidden rounded border border-[var(--c-border)] bg-[var(--c-surface-muted)]">
              <TimelinePhaseRibbon phases={phases} totalMs={totalMs} />
              {rows.map((row) => (
                <div
                  key={row.id}
                  className="absolute top-0 bottom-0"
                  style={{
                    left: `${(row.offsetMs / totalMs) * 100}%`,
                    width: 1.5,
                    background:
                      row.level === 'error'
                        ? 'var(--c-red)'
                        : row.level === 'warn'
                          ? 'var(--c-amber)'
                          : typeColor[row.type],
                    opacity: 0.7,
                  }}
                />
              ))}
              <div
                className="pointer-events-none absolute inset-0 grid"
                style={{ gridTemplateColumns: 'repeat(4, 1fr)' }}
              >
                <span className="border-r border-dashed border-[var(--c-bar-tick)]" />
                <span className="border-r border-dashed border-[var(--c-bar-tick)]" />
                <span className="border-r border-dashed border-[var(--c-bar-tick)]" />
                <span />
              </div>
            </div>
          </div>
        </div>
        <div className="mt-1 flex justify-between font-mono text-[10.5px] text-[var(--c-text-muted)]">
          <span>0</span>
          <span>{formatDuration(totalMs * 0.25)}</span>
          <span>{formatDuration(totalMs * 0.5)}</span>
          <span>{formatDuration(totalMs * 0.75)}</span>
          <span>{formatDuration(totalMs)}</span>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto">
        <div
          className="sticky top-0 z-[1] grid border-b border-[var(--c-border)] bg-[var(--c-table-head-bg)] px-4 py-1.5 text-[10px] font-semibold uppercase tracking-[0.05em] text-[var(--c-text-muted)]"
          style={{ gridTemplateColumns: '90px 70px 70px 220px minmax(0, 1fr)' }}
        >
          <span>Offset</span>
          <span>Type</span>
          <span>Level</span>
          <span>Span</span>
          <span>Message</span>
        </div>

        {filteredRows.length === 0 ? (
          <InspectorEmptyState>
            No {filter === 'all' ? '' : `${filter} `}events recorded for this trace.
          </InspectorEmptyState>
        ) : (
          filteredRows.map((row) => {
            const canSelectSpan = Boolean(row.spanId && spanIndex.has(row.spanId));
            return (
              <button
                key={row.id}
                type="button"
                disabled={!canSelectSpan}
                onClick={canSelectSpan ? () => onSelectSpan(row.spanId!) : undefined}
                className={`grid w-full items-baseline gap-0 border-b border-[var(--c-border-subtle)] px-4 py-1.5 text-left text-xs transition ${
                  canSelectSpan ? 'cursor-pointer hover:bg-[var(--c-row-hover-bg)]' : 'cursor-default'
                }`}
                style={{ gridTemplateColumns: '90px 70px 70px 220px minmax(0, 1fr)' }}
              >
                <span className="font-mono tabular-nums text-[var(--c-text-muted)]">
                  {formatOffset(row.offsetMs)}
                </span>
                <span
                  className="font-mono text-[11px] font-medium"
                  style={{ color: typeColor[row.type] }}
                >
                  {row.type}
                </span>
                <span
                  className="font-mono text-[10.5px] font-semibold uppercase"
                  style={{ color: levelColor[row.level] }}
                >
                  {row.level}
                </span>
                <span className="truncate font-mono text-[var(--c-text-secondary)]">
                  {row.spanName}
                </span>
                <span className="min-w-0 text-[var(--c-text-primary)]">
                  {row.message}
                  {row.meta ? (
                    <span className="ml-2.5 font-mono text-[11px] text-[var(--c-text-muted)]">
                      {row.meta}
                    </span>
                  ) : null}
                </span>
              </button>
            );
          })
        )}
      </div>
    </div>
  );
}

type LogLevel = 'info' | 'warn' | 'error' | 'debug';

const LOG_LEVELS: LogLevel[] = ['info', 'warn', 'error', 'debug'];

interface LogLine {
  id: string;
  timestamp: string;
  hms: string;
  level: LogLevel;
  source: string;
  message: string;
  spanId?: string;
}

function deriveLogLevel(event: TimelineEvent): LogLevel {
  if (event.event_type === 'error' || event.event_type === 'exception' || event.event_type === 'span_failed') {
    return 'error';
  }
  if (event.level === 'error') return 'error';
  if (event.level === 'warning' || event.event_type === 'wait') return 'warn';
  if (event.level === 'debug') return 'debug';
  return 'info';
}

function deriveLogSource(event: TimelineEvent): string {
  if (event.span_name) return event.span_name;
  if (event.event_type === 'snapshot_marker') return 'engine.checkpoint';
  if (event.event_type === 'state_change') return 'engine.state';
  if (event.event_type === 'decision') return 'engine.decision';
  return 'trace';
}

function formatLogTimestamp(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  const hh = String(date.getHours()).padStart(2, '0');
  const mm = String(date.getMinutes()).padStart(2, '0');
  const ss = String(date.getSeconds()).padStart(2, '0');
  const ms = String(date.getMilliseconds()).padStart(3, '0');
  return `${hh}:${mm}:${ss}.${ms}`;
}

function downloadTextFile(filename: string, text: string) {
  const blob = new Blob([text], { type: 'text/plain;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = filename;
  document.body.appendChild(anchor);
  anchor.click();
  document.body.removeChild(anchor);
  URL.revokeObjectURL(url);
}

function TraceLogsPanel({
  events,
  onSelectSpan,
  spanIndex,
}: {
  events: TimelineEvent[];
  onSelectSpan: (spanId: string) => void;
  spanIndex: ReadonlyMap<string, Span>;
}) {
  const [search, setSearch] = useState('');
  const [activeLevels, setActiveLevels] = useState<Record<LogLevel, boolean>>({
    info: true,
    warn: true,
    error: true,
    debug: false,
  });
  const [tail, setTail] = useState(true);
  const [copied, setCopied] = useState(false);

  const lines = useMemo<LogLine[]>(() => {
    return events
      .filter((event) => event.source === 'explicit')
      .map((event) => ({
        id: event.id,
        timestamp: event.timestamp,
        hms: formatLogTimestamp(event.timestamp),
        level: deriveLogLevel(event),
        source: deriveLogSource(event),
        message: event.message ?? event.event_type.replace(/_/g, ' '),
        spanId: event.span_id,
      }));
  }, [events]);

  const filtered = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return lines.filter((line) => {
      if (!activeLevels[line.level]) return false;
      if (!needle) return true;
      return (
        line.message.toLowerCase().includes(needle) ||
        line.source.toLowerCase().includes(needle)
      );
    });
  }, [activeLevels, lines, search]);

  const levelCount = useMemo(() => {
    const out: Record<LogLevel, number> = { info: 0, warn: 0, error: 0, debug: 0 };
    lines.forEach((line) => {
      out[line.level] += 1;
    });
    return out;
  }, [lines]);

  const levelTextColor: Record<LogLevel, string> = {
    info: 'var(--c-text-muted)',
    warn: 'var(--c-amber-text)',
    error: 'var(--c-red-text)',
    debug: 'var(--c-text-muted)',
  };

  const buildPlainText = () =>
    filtered
      .map((line) => `${line.hms} ${line.level.toUpperCase().padEnd(5)} ${line.source} — ${line.message}`)
      .join('\n');

  const handleDownload = () => {
    if (filtered.length === 0) return;
    downloadTextFile('trace-logs.log', buildPlainText());
  };

  const handleCopy = async () => {
    if (filtered.length === 0) return;
    try {
      await navigator.clipboard.writeText(buildPlainText());
      setCopied(true);
      setTimeout(() => setCopied(false), 1400);
    } catch {
      // ignore — clipboard might not be available
    }
  };

  if (lines.length === 0) {
    return (
      <div className="flex min-h-0 flex-1 items-center justify-center p-6">
        <InspectorEmptyState>No explicit log events recorded for this trace.</InspectorEmptyState>
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex flex-wrap items-center gap-3 border-b border-[var(--c-border-subtle)] px-4 py-2.5">
        <div className="relative w-full max-w-[320px] sm:w-auto sm:flex-1">
          <Search className="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-[var(--c-text-muted)]" />
          <input
            aria-label="Filter logs"
            type="search"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
            placeholder="Filter logs…"
            className="h-7 w-full rounded border border-[var(--c-border)] bg-[var(--c-surface)] py-1 pl-7 pr-2 text-xs text-[var(--c-text-primary)] outline-none focus:border-[var(--c-border-strong)]"
          />
        </div>

        <div className="flex flex-wrap gap-1">
          {LOG_LEVELS.map((level) => {
            const active = activeLevels[level];
            return (
              <button
                key={level}
                type="button"
                onClick={() =>
                  setActiveLevels((prev) => ({ ...prev, [level]: !prev[level] }))
                }
                aria-pressed={active}
                className={`inline-flex items-center gap-1.5 rounded border px-2 py-1 font-mono text-[10.5px] font-semibold uppercase tracking-[0.04em] transition ${
                  active
                    ? 'border-[var(--c-border-strong)] bg-[var(--c-surface)]'
                    : 'border-[var(--c-border)] bg-[var(--c-surface-muted)] opacity-60'
                }`}
                style={{ color: levelTextColor[level] }}
              >
                {level}
                <span className="text-[10px] text-[var(--c-text-muted)]">
                  {levelCount[level]}
                </span>
              </button>
            );
          })}
        </div>

        <div className="ml-auto flex items-center gap-3">
          <label className="inline-flex cursor-pointer items-center gap-1.5 text-[11.5px] text-[var(--c-text-secondary)]">
            <input
              type="checkbox"
              checked={tail}
              onChange={(event) => setTail(event.target.checked)}
              className="h-3 w-3 cursor-pointer accent-[var(--c-accent)]"
            />
            Tail
          </label>
          <button
            type="button"
            onClick={handleDownload}
            disabled={filtered.length === 0}
            className="inline-flex items-center gap-1.5 rounded border border-[var(--c-border)] bg-[var(--c-surface)] px-2 py-1 text-[11.5px] font-medium text-[var(--c-text-secondary)] transition hover:text-[var(--c-text-primary)] disabled:opacity-50"
          >
            <Download className="h-3.5 w-3.5" />
            Download
          </button>
          <button
            type="button"
            onClick={handleCopy}
            disabled={filtered.length === 0}
            className="inline-flex items-center gap-1.5 rounded border border-[var(--c-border)] bg-[var(--c-surface)] px-2 py-1 text-[11.5px] font-medium text-[var(--c-text-secondary)] transition hover:text-[var(--c-text-primary)] disabled:opacity-50"
          >
            <CopyIcon className="h-3.5 w-3.5" />
            {copied ? 'Copied' : 'Copy'}
          </button>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-y-auto bg-[var(--c-app-bg)] font-mono text-xs leading-[1.55]">
        {filtered.length === 0 ? (
          <div className="p-6 text-[var(--c-text-muted)]">No log lines match the current filters.</div>
        ) : (
          filtered.map((line, index) => {
            const canSelect = Boolean(line.spanId && spanIndex.has(line.spanId));
            const rowBg =
              line.level === 'error'
                ? 'rgba(239,68,68,0.05)'
                : line.level === 'warn'
                  ? 'rgba(245,158,11,0.04)'
                  : 'transparent';
            const railColor =
              line.level === 'error'
                ? 'var(--c-red)'
                : line.level === 'warn'
                  ? 'var(--c-amber)'
                  : 'transparent';
            return (
              <div
                key={line.id}
                className="grid items-baseline gap-3 py-1 pr-4"
                style={{
                  gridTemplateColumns: '50px 130px 60px 200px minmax(0, 1fr)',
                  background: rowBg,
                  borderLeft: `2px solid ${railColor}`,
                }}
              >
                <span className="select-none pl-3 text-right text-[var(--c-text-muted)]">
                  {index + 1}
                </span>
                <span className="text-[var(--c-text-muted)]">{line.hms}</span>
                <span
                  className="text-[10.5px] font-semibold uppercase"
                  style={{ color: levelTextColor[line.level] }}
                >
                  {line.level}
                </span>
                {canSelect ? (
                  <button
                    type="button"
                    onClick={() => onSelectSpan(line.spanId!)}
                    className="truncate text-left text-[var(--c-text-secondary)] hover:text-[var(--c-accent-text)]"
                  >
                    {line.source}
                  </button>
                ) : (
                  <span className="truncate text-[var(--c-text-secondary)]">{line.source}</span>
                )}
                <span className="whitespace-pre-wrap text-[var(--c-text-primary)]">
                  {line.message}
                </span>
              </div>
            );
          })
        )}
        {tail ? (
          <div
            className="grid gap-3 py-1 pr-4"
            style={{ gridTemplateColumns: '50px minmax(0, 1fr)' }}
          >
            <span className="pl-3 text-right text-[var(--c-text-muted)]">—</span>
            <span className="inline-flex items-center gap-2 italic text-[var(--c-text-muted)]">
              <span
                className="inline-block h-1.5 w-1.5 rounded-full bg-[var(--c-blue)]"
                style={{ animation: 'continua-pulse 1.6s ease-out infinite' }}
              />
              Tailing — stream open
            </span>
          </div>
        ) : null}
      </div>
    </div>
  );
}

type MetricTone = 'muted' | 'amber' | 'red' | 'green';

interface MetricCardSpec {
  label: string;
  value: string;
  delta?: string;
  tone: MetricTone;
  series: number[];
  anomaly?: boolean;
}

const METRIC_TONE_STROKE: Record<MetricTone, string> = {
  muted: 'var(--c-text-muted)',
  amber: 'var(--c-amber)',
  red: 'var(--c-red)',
  green: 'var(--c-green)',
};

const METRIC_TONE_TEXT: Record<MetricTone, string> = {
  muted: 'var(--c-text-muted)',
  amber: 'var(--c-amber-text)',
  red: 'var(--c-red-text)',
  green: 'var(--c-green-text)',
};

function MetricCard({ card }: { card: MetricCardSpec }) {
  const max = Math.max(...card.series, 1);
  const points = card.series
    .map((value, index) => {
      const x = card.series.length === 1 ? 100 : (index / (card.series.length - 1)) * 100;
      const y = 24 - (value / max) * 22;
      return `${x.toFixed(2)},${y.toFixed(2)}`;
    })
    .join(' ');
  const lastX = 100;
  const lastY = 24 - (card.series[card.series.length - 1] / max) * 22;
  const stroke = METRIC_TONE_STROKE[card.tone];
  return (
    <div className="rounded-lg border border-[var(--c-border)] bg-[var(--c-surface)] px-3.5 py-3">
      <div className="flex items-center justify-between">
        <span className="text-[10.5px] font-semibold uppercase tracking-[0.05em] text-[var(--c-text-muted)]">
          {card.label}
        </span>
        {card.anomaly ? (
          <span
            aria-hidden="true"
            className="inline-block h-1.5 w-1.5 rounded-full"
            style={{ background: stroke }}
            title="Anomaly"
          />
        ) : null}
      </div>
      <div className="mt-1 text-[22px] font-semibold tabular-nums leading-tight tracking-[-0.015em] text-[var(--c-text-primary)]">
        {card.value}
      </div>
      {card.delta ? (
        <div className="mt-1 text-[11px]" style={{ color: METRIC_TONE_TEXT[card.tone] }}>
          {card.delta}
        </div>
      ) : null}
      <svg
        viewBox="0 0 100 24"
        preserveAspectRatio="none"
        className="mt-2 block h-7 w-full"
        aria-hidden="true"
      >
        <polyline
          points={points}
          fill="none"
          stroke={stroke}
          strokeWidth={1.2}
          vectorEffect="non-scaling-stroke"
        />
        {card.anomaly ? <circle cx={lastX} cy={lastY} r={2} fill={stroke} /> : null}
      </svg>
    </div>
  );
}

function MetricsSection({
  children,
  hint,
  title,
}: {
  children: ReactNode;
  hint?: string;
  title: string;
}) {
  return (
    <section>
      <div className="mb-2 flex items-baseline justify-between">
        <h3 className="text-[12.5px] font-semibold tracking-[-0.005em] text-[var(--c-text-primary)]">
          {title}
        </h3>
        {hint ? <span className="text-[11px] text-[var(--c-text-muted)]">{hint}</span> : null}
      </div>
      <div className="rounded-lg border border-[var(--c-border)] bg-[var(--c-surface)] px-3.5 py-3">
        {children}
      </div>
    </section>
  );
}

function MetricsKvGrid({ rows }: { rows: Array<[string, string]> }) {
  return (
    <div className="flex flex-col">
      {rows.map(([label, value], index) => (
        <div
          key={label}
          className="grid grid-cols-[160px_minmax(0,1fr)] gap-3 py-1.5 text-[11.5px]"
          style={{
            borderBottom:
              index < rows.length - 1 ? '1px solid var(--c-border-subtle)' : 'none',
          }}
        >
          <span className="text-[var(--c-text-muted)]">{label}</span>
          <span className="truncate font-mono tabular-nums text-[var(--c-text-primary)]">
            {value}
          </span>
        </div>
      ))}
    </div>
  );
}

function bucketLatencies(latencies: number[]): Array<{ range: string; n: number }> {
  if (latencies.length === 0) return [];
  const max = Math.max(...latencies);
  const ceiling = max <= 0 ? 1 : max;
  const edges =
    ceiling >= 4000
      ? [0, 500, 1000, 1500, 2000, 3000, 4000, 5000, Number.POSITIVE_INFINITY]
      : ceiling >= 1000
        ? [0, 100, 250, 500, 750, 1000, 1500, 2000, Number.POSITIVE_INFINITY]
        : [0, 25, 50, 100, 200, 350, 500, 750, Number.POSITIVE_INFINITY];
  const counts = new Array(edges.length - 1).fill(0);
  latencies.forEach((latency) => {
    for (let i = 0; i < edges.length - 1; i += 1) {
      if (latency >= edges[i] && latency < edges[i + 1]) {
        counts[i] += 1;
        return;
      }
    }
  });
  return counts.map((n, i) => {
    const lower = edges[i];
    const upper = edges[i + 1];
    const range =
      upper === Number.POSITIVE_INFINITY
        ? `${lower}+ ms`
        : upper >= 1000
          ? `${(lower / 1000).toFixed(0)}–${(upper / 1000).toFixed(0)}s`
          : `${lower}–${upper}ms`;
    return { range, n };
  });
}

function TraceMetricsPanel({
  events,
  spans,
  trace,
  traceCostSeries,
}: {
  events: TimelineEvent[];
  spans: Span[];
  trace: TraceDetail;
  traceCostSeries: TraceCostSeries | null;
}) {
  const totalSpans = spans.length;
  const failedSpans = spans.filter((span) => span.status === 'FAILED').length;
  const runningSpans = spans.filter((span) => span.status === 'STARTED').length;
  const completedSpans = spans.filter((span) => span.status === 'COMPLETED').length;
  const totalLatencyMs = spans.reduce((sum, span) => sum + (span.latency_ms ?? 0), 0);
  const explicitEventCount = events.filter((event) => event.source === 'explicit').length;
  const errorEventCount = events.filter(
    (event) => event.event_type === 'error' || event.event_type === 'exception',
  ).length;

  const traceDuration = trace.ended_at
    ? new Date(trace.ended_at).getTime() - new Date(trace.started_at).getTime()
    : 0;
  const errorRate = totalSpans === 0 ? 0 : (failedSpans / totalSpans) * 100;
  const tokensIn = trace.total_tokens_in ?? 0;
  const tokensOut = trace.total_tokens_out ?? 0;
  const totalTokens = tokensIn + tokensOut;
  const totalCost = trace.total_cost_usd ?? 0;

  const orderedSpans = useMemo(
    () =>
      [...spans].sort(
        (a, b) => new Date(a.started_at).getTime() - new Date(b.started_at).getTime(),
      ),
    [spans],
  );

  const latencySeries = orderedSpans.map((span) => span.latency_ms ?? 0);
  const cumulativeTokens: number[] = [];
  let runningTokens = 0;
  orderedSpans.forEach((span) => {
    runningTokens += (span.tokens_in ?? 0) + (span.tokens_out ?? 0);
    cumulativeTokens.push(runningTokens);
  });
  const cumulativeCost: number[] = [];
  let runningCost = 0;
  if (traceCostSeries && traceCostSeries.points.length > 0) {
    traceCostSeries.points.forEach((point) => {
      runningCost = point.cumulativeCostUsd;
      cumulativeCost.push(runningCost);
    });
  } else {
    orderedSpans.forEach((span) => {
      runningCost += span.cost_usd ?? 0;
      cumulativeCost.push(runningCost);
    });
  }
  const errorRateSeries = orderedSpans.map((_span, index) => {
    const failedSoFar = orderedSpans
      .slice(0, index + 1)
      .filter((s) => s.status === 'FAILED').length;
    return ((failedSoFar / (index + 1)) * 100) | 0;
  });

  const cards: MetricCardSpec[] = [
    {
      label: 'Duration',
      value: formatDuration(traceDuration || totalLatencyMs),
      delta:
        traceDuration > 0 ? `${formatDuration(totalLatencyMs)} span time` : 'Trace still running',
      tone: trace.status === 'FAILED' ? 'red' : trace.status === 'RUNNING' ? 'amber' : 'muted',
      series: latencySeries.length > 0 ? latencySeries : [0],
      anomaly: trace.status === 'FAILED',
    },
    {
      label: 'Tokens',
      value: formatTokens(totalTokens),
      delta:
        totalTokens > 0
          ? `in ${formatTokens(tokensIn)} · out ${formatTokens(tokensOut)}`
          : 'No token telemetry',
      tone: 'muted',
      series: cumulativeTokens.length > 0 ? cumulativeTokens : [0],
    },
    {
      label: 'Cost',
      value: formatCost(totalCost),
      delta:
        cumulativeCost.length > 0 ? `${cumulativeCost.length} sample(s)` : 'No cost telemetry',
      tone: 'muted',
      series: cumulativeCost.length > 0 ? cumulativeCost : [0],
    },
    {
      label: 'Error rate',
      value: `${Math.round(errorRate)}%`,
      delta:
        failedSpans > 0
          ? `${failedSpans} of ${totalSpans} spans failed`
          : 'All spans recorded clean',
      tone: failedSpans > 0 ? 'red' : 'green',
      series: errorRateSeries.length > 0 ? errorRateSeries : [0],
      anomaly: failedSpans > 0,
    },
  ];

  const latencyByRow = useMemo(() => {
    const totals = totalLatencyMs <= 0 ? 1 : totalLatencyMs;
    return [...spans]
      .sort((a, b) => (b.latency_ms ?? 0) - (a.latency_ms ?? 0))
      .slice(0, 6)
      .map((span) => {
        const ms = span.latency_ms ?? 0;
        const pct = (ms / totals) * 100;
        return {
          id: span.id,
          name: span.name,
          ms,
          pct,
          status: span.status,
        };
      });
  }, [spans, totalLatencyMs]);

  const distributionBuckets = useMemo(
    () => bucketLatencies(spans.map((span) => span.latency_ms ?? 0)),
    [spans],
  );
  const distributionMax = Math.max(...distributionBuckets.map((bucket) => bucket.n), 1);

  const tokenSegments = useMemo(() => {
    if (totalTokens === 0) return [];
    return [
      { label: 'Input', n: tokensIn, pct: (tokensIn / totalTokens) * 100, color: 'var(--c-blue)' },
      {
        label: 'Output',
        n: tokensOut,
        pct: (tokensOut / totalTokens) * 100,
        color: 'var(--c-accent)',
      },
    ];
  }, [tokensIn, tokensOut, totalTokens]);

  const spanKindCounts = useMemo(() => {
    const counts = new Map<string, number>();
    spans.forEach((span) => {
      counts.set(span.kind, (counts.get(span.kind) ?? 0) + 1);
    });
    return Array.from(counts.entries())
      .sort((a, b) => b[1] - a[1])
      .slice(0, 4)
      .map(([kind, count]) => `${kind.toLowerCase()} · ${count}`);
  }, [spans]);

  const spanSignals: Array<[string, string]> = [
    ['Spans', String(totalSpans)],
    ['Completed', String(completedSpans)],
    ['Running', String(runningSpans)],
    ['Failed', String(failedSpans)],
    ['Span latency', formatDuration(totalLatencyMs)],
    ['Avg span latency', totalSpans > 0 ? formatDuration(totalLatencyMs / totalSpans) : '—'],
    ['Span kinds', spanKindCounts.length > 0 ? spanKindCounts.join(', ') : '—'],
    ['Explicit events', `${explicitEventCount} · ${errorEventCount} error`],
  ];

  const engineSignals: Array<[string, string]> = trace.engine
    ? [
        ['Definition', trace.engine.definition_name ?? '—'],
        ['Version', trace.engine.definition_version ?? '—'],
        ['Run', trace.engine.run_id ?? '—'],
        ['Instance', trace.engine.instance_key ?? '—'],
        ['Run status', trace.engine.status ?? '—'],
        ['Projection', trace.engine.projection_state ?? '—'],
      ]
    : [['Engine signals', 'No engine attached to this trace.']];

  const statusToneFromSpan = (status: string) =>
    status === 'FAILED' ? 'var(--c-red)' : status === 'STARTED' ? 'var(--c-blue)' : 'var(--c-green)';

  return (
    <div className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {cards.map((card) => (
          <MetricCard key={card.label} card={card} />
        ))}
      </div>

      <MetricsSection title="Latency by span" hint="Top contributors to wall time">
        {latencyByRow.length === 0 ? (
          <p className="py-2 text-[11.5px] text-[var(--c-text-muted)]">
            No spans with measured latency yet.
          </p>
        ) : (
          <div className="flex flex-col">
            {latencyByRow.map((row, index) => (
              <div
                key={row.id}
                className="grid items-center gap-4 py-2"
                style={{
                  gridTemplateColumns: 'minmax(0, 320px) minmax(0, 1fr) 80px 60px',
                  borderBottom:
                    index < latencyByRow.length - 1 ? '1px solid var(--c-border-subtle)' : 'none',
                }}
              >
                <div className="flex items-center gap-2 truncate font-mono text-xs text-[var(--c-text-primary)]">
                  <span
                    className="inline-block h-1.5 w-1.5 shrink-0 rounded-full"
                    style={{ background: statusToneFromSpan(row.status) }}
                  />
                  <span className="truncate">{row.name}</span>
                </div>
                <div className="relative h-1.5 overflow-hidden rounded-full bg-[var(--c-surface-muted)]">
                  <div
                    className="absolute left-0 top-0 bottom-0 rounded-full"
                    style={{
                      width: `${Math.max(row.pct, 0.5)}%`,
                      background:
                        row.status === 'FAILED' ? 'var(--c-bar-failed)' : 'var(--c-bar-success)',
                    }}
                  />
                </div>
                <div className="text-right font-mono text-[11.5px] tabular-nums text-[var(--c-text-secondary)]">
                  {formatDuration(row.ms)}
                </div>
                <div className="text-right font-mono text-[11px] tabular-nums text-[var(--c-text-muted)]">
                  {Math.round(row.pct)}%
                </div>
              </div>
            ))}
          </div>
        )}
      </MetricsSection>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
        <MetricsSection title="Latency distribution" hint="Spans bucketed by duration">
          {distributionBuckets.length === 0 ? (
            <p className="py-2 text-[11.5px] text-[var(--c-text-muted)]">
              No latency samples yet.
            </p>
          ) : (
            <>
              <div className="flex h-24 items-end gap-1">
                {distributionBuckets.map((bucket) => (
                  <div
                    key={bucket.range}
                    className="flex flex-1 flex-col items-center gap-1"
                  >
                    <div
                      className="w-full rounded-t-[2px]"
                      style={{
                        height: `${(bucket.n / distributionMax) * 100}%`,
                        minHeight: bucket.n > 0 ? 2 : 0,
                        background: 'var(--c-text-muted)',
                        opacity: bucket.n > 0 ? 0.55 : 0.18,
                      }}
                      title={`${bucket.range} · ${bucket.n} span(s)`}
                    />
                    <span className="font-mono text-[9.5px] text-[var(--c-text-muted)]">
                      {bucket.range}
                    </span>
                  </div>
                ))}
              </div>
              <div className="mt-3 flex justify-between border-t border-[var(--c-border-subtle)] pt-2 font-mono text-[11px] text-[var(--c-text-muted)]">
                <span>
                  min{' '}
                  <span className="text-[var(--c-text-secondary)]">
                    {formatDuration(Math.min(...spans.map((s) => s.latency_ms ?? 0)))}
                  </span>
                </span>
                <span>
                  median{' '}
                  <span className="text-[var(--c-text-secondary)]">
                    {totalSpans > 0
                      ? formatDuration(
                          [...spans]
                            .map((s) => s.latency_ms ?? 0)
                            .sort((a, b) => a - b)[Math.floor(totalSpans / 2)],
                        )
                      : '—'}
                  </span>
                </span>
                <span>
                  max{' '}
                  <span className="text-[var(--c-text-secondary)]">
                    {formatDuration(Math.max(...spans.map((s) => s.latency_ms ?? 0)))}
                  </span>
                </span>
              </div>
            </>
          )}
        </MetricsSection>

        <MetricsSection title="Token usage" hint="Input vs. output split">
          {tokenSegments.length === 0 ? (
            <p className="py-2 text-[11.5px] text-[var(--c-text-muted)]">
              No token telemetry recorded.
            </p>
          ) : (
            <>
              <div className="flex h-3 overflow-hidden rounded-full bg-[var(--c-surface-muted)]">
                {tokenSegments.map((segment) => (
                  <div
                    key={segment.label}
                    style={{ width: `${segment.pct}%`, background: segment.color }}
                  />
                ))}
              </div>
              <div className="mt-3 flex flex-col gap-1.5">
                {tokenSegments.map((segment) => (
                  <div
                    key={segment.label}
                    className="grid items-center gap-2 text-[11.5px]"
                    style={{ gridTemplateColumns: '12px minmax(0, 1fr) auto auto' }}
                  >
                    <span
                      className="inline-block h-2 w-2 rounded-sm"
                      style={{ background: segment.color }}
                    />
                    <span className="text-[var(--c-text-secondary)]">{segment.label}</span>
                    <span className="font-mono tabular-nums text-[var(--c-text-primary)]">
                      {formatTokens(segment.n)}
                    </span>
                    <span className="min-w-[32px] text-right font-mono tabular-nums text-[var(--c-text-muted)]">
                      {Math.round(segment.pct)}%
                    </span>
                  </div>
                ))}
              </div>
            </>
          )}
        </MetricsSection>
      </div>

      <div className="grid gap-4 xl:grid-cols-2">
        <MetricsSection title="Span signals">
          <MetricsKvGrid rows={spanSignals} />
        </MetricsSection>
        <MetricsSection title="Engine signals">
          <MetricsKvGrid rows={engineSignals} />
        </MetricsSection>
      </div>
    </div>
  );
}

type EngineStatePane = 'overview' | 'pending' | 'history' | 'result';

interface StateMachineStep {
  id: string;
  label: string;
  done: boolean;
  current: boolean;
  warn?: boolean;
  error?: boolean;
}

function buildEngineStateMachine(status: EngineRunStatus): StateMachineStep[] {
  const isFailed = status === 'FAILED' || status === 'CANCELLED' || status === 'TERMINATED';
  const isClosed = status === 'COMPLETED' || isFailed || status === 'CONTINUED_AS_NEW';
  const isWaiting = status === 'WAITING' || status === 'SUSPENDED';
  return [
    { id: 'created', label: 'Created', done: true, current: false },
    {
      id: 'running',
      label: 'Running',
      done: status !== 'QUEUED',
      current: status === 'RUNNING',
    },
    {
      id: 'waiting',
      label: 'Waiting',
      done: isClosed || isWaiting,
      current: isWaiting,
      warn: isWaiting,
    },
    {
      id: 'closed',
      label: isFailed ? 'Failed' : isClosed ? 'Closed' : 'Closing',
      done: isClosed,
      current: isClosed,
      error: isFailed,
    },
  ];
}

function StateMachine({ steps }: { steps: StateMachineStep[] }) {
  return (
    <div className="flex items-center justify-between">
      {steps.map((step, index) => {
        const ringColor = step.error
          ? 'var(--c-red)'
          : step.warn
            ? 'var(--c-amber)'
            : step.done
              ? 'var(--c-text-secondary)'
              : 'var(--c-border)';
        const fillColor = step.error
          ? 'var(--c-red-faint)'
          : step.warn
            ? 'var(--c-amber-faint)'
            : step.done
              ? 'var(--c-surface-muted)'
              : 'var(--c-surface)';
        const textColor = step.error
          ? 'var(--c-red-text)'
          : step.warn
            ? 'var(--c-amber-text)'
            : step.done
              ? 'var(--c-text-primary)'
              : 'var(--c-text-muted)';
        const labelColor = step.error ? 'var(--c-red-text)' : 'var(--c-text-secondary)';
        return (
          <div key={step.id} className="flex flex-1 items-center last:flex-initial">
            <div className="flex shrink-0 flex-col items-center gap-2">
              <div
                className="flex h-7 w-7 items-center justify-center rounded-full font-mono text-[11px] font-semibold"
                style={{
                  border: `1.5px solid ${ringColor}`,
                  background: fillColor,
                  color: textColor,
                  boxShadow: step.current && step.error ? '0 0 0 4px rgba(239,68,68,0.18)' : 'none',
                }}
              >
                {step.error ? '!' : step.done ? '✓' : index + 1}
              </div>
              <span
                className="whitespace-nowrap text-[11px]"
                style={{ color: labelColor, fontWeight: step.current ? 600 : 500 }}
              >
                {step.label}
              </span>
            </div>
            {index < steps.length - 1 ? (
              <div
                className="-mb-5 h-px flex-1"
                style={{
                  background: steps[index + 1].done ? 'var(--c-text-muted)' : 'var(--c-border)',
                }}
              />
            ) : null}
          </div>
        );
      })}
    </div>
  );
}

function EngineKvSmall({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex flex-col gap-px">
      <span className="text-[10px] font-semibold uppercase tracking-[0.05em] text-[var(--c-text-muted)]">
        {label}
      </span>
      <span className="truncate font-mono text-xs text-[var(--c-text-primary)]">{value}</span>
    </div>
  );
}

const ENGINE_PANES: Array<{ id: EngineStatePane; label: string }> = [
  { id: 'overview', label: 'Overview' },
  { id: 'pending', label: 'Pending' },
  { id: 'history', label: 'Engine history' },
  { id: 'result', label: 'Result' },
];

function TraceEngineStatePanel({
  engine,
  errorMessage,
  events,
  isError,
  isLoading,
  onSelectSpan,
  openWaits,
  pendingWork,
  runningStateAssessment,
  spanIndex,
  traceId,
}: {
  engine: TraceDetail['engine'];
  errorMessage: string;
  events: TimelineEvent[];
  isError: boolean;
  isLoading: boolean;
  onSelectSpan: (spanId: string) => void;
  openWaits: OpenWait[];
  pendingWork: EnginePendingWorkResponse | undefined;
  runningStateAssessment: WaitStallAssessment | null;
  spanIndex: ReadonlyMap<string, Span>;
  traceId: string;
}) {
  const [pane, setPane] = useState<EngineStatePane>('overview');
  const [selectedEventId, setSelectedEventId] = useState<string | null>(null);

  if (!engine) {
    return <InspectorEmptyState>This trace is not backed by an engine run.</InspectorEmptyState>;
  }

  const journalEvents = events.filter((event) => {
    return (
      event.event_type === 'snapshot_marker' ||
      event.event_type === 'state_change' ||
      event.event_type === 'decision' ||
      event.event_type === 'effect' ||
      event.event_type === 'wait' ||
      event.event_type === 'span_failed'
    );
  });

  const checkpointCount = events.filter((event) => event.event_type === 'snapshot_marker').length;
  const stateChangeCount = events.filter((event) => event.event_type === 'state_change').length;
  const decisionCount = events.filter((event) => event.event_type === 'decision').length;
  const effectCount = events.filter((event) => event.event_type === 'effect').length;
  const waitCount = events.filter((event) => event.event_type === 'wait').length;
  const pendingCount =
    (pendingWork?.activities.length ?? 0) +
    (pendingWork?.timers.length ?? 0) +
    (pendingWork?.signals.length ?? 0);
  const hasFailure = Boolean(engine.failure);
  const waitState = pendingWork?.current_wait ?? engine.wait_state ?? null;
  const hasWait = Boolean(waitState && Object.keys(waitState).length > 0);

  const paneCounts: Record<EngineStatePane, number> = {
    overview: journalEvents.length,
    pending: pendingCount,
    history: 0,
    result: isTerminalEngineStatus(engine.status) || hasFailure ? 1 : 0,
  };

  const selectedEvent = selectedEventId
    ? events.find((event) => event.id === selectedEventId) ?? null
    : null;

  const stateMachine = buildEngineStateMachine(engine.status);

  const formatRelative = (iso?: string) => {
    if (!iso) return '—';
    const ts = new Date(iso).getTime();
    if (Number.isNaN(ts)) return '—';
    return formatTimestamp(iso);
  };

  return (
    <div className="flex min-h-0 flex-1">
      <div className="flex min-w-0 flex-1 flex-col overflow-y-auto px-5 py-4">
        <div className="rounded-lg border border-[var(--c-border)] bg-[var(--c-surface)] px-4 py-3">
          <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
            <div className="flex items-center gap-2">
              <Zap className="h-3.5 w-3.5 text-[var(--c-accent-text)]" />
              <span className="font-mono text-[13.5px] font-semibold text-[var(--c-text-primary)]">
                {engine.run_id}
              </span>
              <Chip tone={hasFailure ? 'error' : engine.status === 'RUNNING' ? 'accent' : 'muted'}>
                {engine.status}
              </Chip>
            </div>
          </div>
          <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
            <EngineKvSmall label="Definition" value={engine.definition_name ?? '—'} />
            <EngineKvSmall label="Version" value={engine.definition_version ?? '—'} />
            <EngineKvSmall label="Instance" value={engine.instance_key ?? '—'} />
            <EngineKvSmall label="Projection" value={engine.projection_state ?? '—'} />
            <EngineKvSmall
              label="Parent run"
              value={engine.parent_run_id ?? '—'}
            />
            <EngineKvSmall label="Created" value={formatRelative(engine.created_at)} />
            <EngineKvSmall label="Updated" value={formatRelative(engine.updated_at)} />
            <EngineKvSmall label="Closed" value={formatRelative(engine.completed_at)} />
          </div>
        </div>

        <div className="mt-4 mb-2 flex items-baseline justify-between">
          <h3 className="text-[12.5px] font-semibold text-[var(--c-text-primary)]">State machine</h3>
          <span className="text-[11px] text-[var(--c-text-muted)]">Workflow lifecycle</span>
        </div>
        <div className="rounded-lg border border-[var(--c-border)] bg-[var(--c-surface)] px-4 py-5">
          <StateMachine steps={stateMachine} />
        </div>

        <div className="mt-4 space-y-3">
          <EngineControlBar engine={engine} traceId={traceId} />
          <EngineProjectionBanner projectionState={engine.projection_state} />
        </div>

        <div className="mt-4 flex border-b border-[var(--c-border)]">
          {ENGINE_PANES.map((tab) => {
            const active = pane === tab.id;
            return (
              <button
                key={tab.id}
                type="button"
                onClick={() => setPane(tab.id)}
                aria-pressed={active}
                className={`-mb-px flex items-center gap-1.5 px-3.5 py-2 text-[12.5px] transition ${
                  active
                    ? 'border-b-2 border-[var(--c-accent)] font-semibold text-[var(--c-text-primary)]'
                    : 'border-b-2 border-transparent font-medium text-[var(--c-text-secondary)] hover:text-[var(--c-text-primary)]'
                }`}
              >
                {tab.label}
                <span className="font-mono text-[10.5px] text-[var(--c-text-muted)]">
                  {paneCounts[tab.id]}
                </span>
              </button>
            );
          })}
        </div>

        <div className="mt-3">
          {pane === 'overview' ? (
            <EngineOverviewPane
              checkpointCount={checkpointCount}
              decisionCount={decisionCount}
              effectCount={effectCount}
              engine={engine}
              events={journalEvents}
              hasWait={hasWait}
              onSelectEvent={(event) => {
                setSelectedEventId(event.id);
                if (event.span_id && spanIndex.has(event.span_id)) {
                  onSelectSpan(event.span_id);
                }
              }}
              runningStateAssessment={runningStateAssessment}
              openWaits={openWaits}
              selectedEventId={selectedEventId}
              spanIndex={spanIndex}
              stateChangeCount={stateChangeCount}
              waitCount={waitCount}
              waitState={waitState}
              onSelectSpan={onSelectSpan}
            />
          ) : pane === 'pending' ? (
            isLoading ? (
              <EngineEmptyCard>Loading pending work…</EngineEmptyCard>
            ) : isError ? (
              <EngineEmptyCard>{errorMessage}</EngineEmptyCard>
            ) : (
              <EnginePendingWorkPanel
                data={pendingWork}
                isError={isError}
                isLoading={isLoading}
                errorMessage={errorMessage}
              />
            )
          ) : pane === 'history' ? (
            <EngineHistoryPane runId={engine.run_id} />
          ) : pane === 'result' ? (
            <EngineResultPane runId={engine.run_id} status={engine.status} />
          ) : null}
        </div>

        {stateChangeCount > 0 || checkpointCount > 0 ? (
          <p className="mt-3 text-[11px] text-[var(--c-text-muted)]">
            {checkpointCount} checkpoint(s) · {stateChangeCount} state change(s) recorded.
          </p>
        ) : null}
      </div>

      <aside className="hidden w-[360px] shrink-0 flex-col border-l border-[var(--c-border)] xl:flex">
        <div className="border-b border-[var(--c-border)] px-4 py-3">
          <div className="text-[10.5px] font-semibold uppercase tracking-[0.05em] text-[var(--c-text-muted)]">
            {selectedEvent ? 'Selected event' : 'Engine projection'}
          </div>
          {selectedEvent ? (
            <>
              <div className="mt-1 font-mono text-[13px] font-semibold text-[var(--c-text-primary)]">
                {selectedEvent.event_type}
              </div>
              <div className="mt-1 font-mono text-[11px] text-[var(--c-text-muted)]">
                seq={selectedEvent.sequence ?? '—'} · {formatTimestamp(selectedEvent.timestamp)}
              </div>
            </>
          ) : (
            <div className="mt-1 font-mono text-[11px] text-[var(--c-text-muted)]">
              {engine.run_id}
            </div>
          )}
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto px-3 py-3">
          {selectedEvent ? (
            <CompactPayloadInspector value={selectedEvent.payload ?? {}} />
          ) : (
            <CompactPayloadInspector value={engine} />
          )}
        </div>
      </aside>
    </div>
  );
}

function EngineOverviewPane({
  checkpointCount,
  decisionCount,
  effectCount,
  engine,
  events,
  hasWait,
  onSelectEvent,
  onSelectSpan,
  openWaits,
  runningStateAssessment,
  selectedEventId,
  spanIndex,
  stateChangeCount,
  waitCount,
  waitState,
}: {
  checkpointCount: number;
  decisionCount: number;
  effectCount: number;
  engine: NonNullable<TraceDetail['engine']>;
  events: TimelineEvent[];
  hasWait: boolean;
  onSelectEvent: (event: TimelineEvent) => void;
  onSelectSpan: (spanId: string) => void;
  openWaits: OpenWait[];
  runningStateAssessment: WaitStallAssessment | null;
  selectedEventId: string | null;
  spanIndex: ReadonlyMap<string, Span>;
  stateChangeCount: number;
  waitCount: number;
  waitState: EnginePendingWorkResponse['current_wait'] | NonNullable<TraceDetail['engine']>['wait_state'] | null;
}) {
  return (
    <div className="space-y-4">
      {hasWait && runningStateAssessment ? (
        <RunningStatePanel
          assessment={runningStateAssessment}
          events={events}
          openWaits={openWaits}
          spanIndex={spanIndex}
          onSelectSpan={onSelectSpan}
        />
      ) : hasWait && waitState ? (
        <div className="rounded-lg border border-[var(--c-border)] bg-[var(--c-surface)] p-3.5">
          <div className="text-[10.5px] font-semibold uppercase tracking-[0.05em] text-[var(--c-text-muted)]">
            Current wait
          </div>
          <div className="mt-3 rounded border border-[var(--c-border-subtle)] bg-[var(--c-app-bg)] p-3">
            <CompactPayloadInspector value={waitState} />
          </div>
        </div>
      ) : (
        <EngineEmptyCard>No wait state for this run.</EngineEmptyCard>
      )}

      {engine.failure ? (
        <div className="rounded-lg border border-[var(--c-red-border)] bg-[var(--c-red-faint)] p-3.5">
          <div className="text-[10.5px] font-semibold uppercase tracking-[0.05em] text-[var(--c-red-text)]">
            Failure
          </div>
          <div className="mt-2 font-mono text-xs text-[var(--c-red-text)]">
            {engine.failure.error_code}
          </div>
          <p className="mt-2 text-sm text-[var(--c-text-primary)]">
            {engine.failure.error_message}
          </p>
        </div>
      ) : null}

      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
        <EngineKvSmall label="Run" value={engine.run_id} />
        <EngineKvSmall label="Instance" value={engine.instance_key ?? '—'} />
        <EngineKvSmall label="Definition" value={engine.definition_name ?? '—'} />
        <EngineKvSmall label="Version" value={engine.definition_version ?? '—'} />
        <EngineKvSmall label="Updated" value={formatTimestamp(engine.updated_at)} />
      </div>

      <div className="grid gap-3 sm:grid-cols-5">
        <JournalCount label="State changes" value={stateChangeCount} />
        <JournalCount label="Decisions" value={decisionCount} />
        <JournalCount label="Effects" value={effectCount} />
        <JournalCount label="Waits" value={waitCount} />
        <JournalCount label="Snapshots" value={checkpointCount} />
      </div>

      {events.length === 0 ? (
        <EngineEmptyCard>No projected journal summary is available for this run.</EngineEmptyCard>
      ) : (
        <ProjectedJournalSummary
          events={events}
          onSelectEvent={onSelectEvent}
          selectedEventId={selectedEventId}
          spanIndex={spanIndex}
        />
      )}
    </div>
  );
}

function JournalCount({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-lg border border-[var(--c-border)] bg-[var(--c-surface)] px-3 py-2">
      <div className="text-[10.5px] font-semibold uppercase tracking-[0.05em] text-[var(--c-text-muted)]">
        {label}
      </div>
      <div className="mt-1 font-mono text-lg font-semibold text-[var(--c-text-primary)]">
        {value}
      </div>
    </div>
  );
}

function ProjectedJournalSummary({
  events,
  onSelectEvent,
  selectedEventId,
}: {
  events: TimelineEvent[];
  onSelectEvent: (event: TimelineEvent) => void;
  selectedEventId: string | null;
  spanIndex: ReadonlyMap<string, Span>;
}) {
  return (
    <div className="overflow-hidden rounded-lg border border-[var(--c-border)] bg-[var(--c-surface)]">
      <div
        className="grid border-b border-[var(--c-border)] bg-[var(--c-table-head-bg)] px-3.5 py-1.5 text-[10px] font-semibold uppercase tracking-[0.05em] text-[var(--c-text-muted)]"
        style={{ gridTemplateColumns: '50px 110px 200px minmax(0,1fr)' }}
      >
        <span>Seq</span>
        <span>Time</span>
        <span>Event</span>
        <span>Detail</span>
      </div>
      {events.slice(0, 12).map((event, index) => {
        const isCheckpoint = event.event_type === 'snapshot_marker';
        const isError = event.event_type === 'span_failed';
        return (
          <button
            key={event.id}
            type="button"
            onClick={() => onSelectEvent(event)}
            className={`grid w-full items-baseline gap-0 px-3.5 py-1.5 text-left font-mono text-[11.5px] transition ${
              selectedEventId === event.id
                ? 'bg-[var(--c-row-selected-bg)]'
                : 'hover:bg-[var(--c-row-hover-bg)]'
            }`}
            style={{
              gridTemplateColumns: '50px 110px 200px minmax(0,1fr)',
              borderBottom:
                index < Math.min(events.length, 12) - 1
                  ? '1px solid var(--c-border-subtle)'
                  : 'none',
            }}
          >
            <span className="tabular-nums text-[var(--c-text-muted)]">
              {event.sequence ?? index + 1}
            </span>
            <span className="text-[var(--c-text-muted)]">
              {formatTimestamp(event.timestamp)}
            </span>
            <span
              className="truncate font-semibold"
              style={{
                color: isError
                  ? 'var(--c-red-text)'
                  : isCheckpoint
                    ? 'var(--c-accent-text)'
                    : 'var(--c-text-primary)',
              }}
            >
              {event.event_type}
            </span>
            <span className="truncate text-[var(--c-text-secondary)]">
              {event.message ?? event.span_name ?? '—'}
            </span>
          </button>
        );
      })}
    </div>
  );
}

function EngineHistoryPane({ runId }: { runId: string }) {
  const [after, setAfter] = useState<number | undefined>();
  const [events, setEvents] = useState<EngineHistoryEvent[]>([]);
  const loadedPageKeys = useRef(new Set<string>());
  const historyQuery = useQuery({
    queryKey: ['engineRunHistory', runId, after ?? null],
    queryFn: () => fetchEngineRunHistory(runId, { after, limit: 50 }),
  });

  useEffect(() => {
    if (!historyQuery.data) {
      return;
    }
    const pageKey = after === undefined ? 'initial' : String(after);
    if (loadedPageKeys.current.has(pageKey)) {
      return;
    }
    loadedPageKeys.current.add(pageKey);
    setEvents((current) => [...current, ...historyQuery.data.events]);
  }, [after, historyQuery.data]);

  if (historyQuery.isLoading && events.length === 0) {
    return <EngineEmptyCard>Loading engine history…</EngineEmptyCard>;
  }
  if (historyQuery.isError) {
    return <EngineEmptyCard>Engine history is temporarily unavailable.</EngineEmptyCard>;
  }
  if (historyQuery.data?.expired) {
    return <EngineEmptyCard>History expired. Retained history for this run has been purged.</EngineEmptyCard>;
  }
  if (events.length === 0) {
    return <EngineEmptyCard>No retained engine history events.</EngineEmptyCard>;
  }

  return (
    <div className="space-y-3">
      <div className="overflow-hidden rounded-lg border border-[var(--c-border)] bg-[var(--c-surface)]">
        {events.map((event, index) => (
          <div
            key={`${event.id}-${index}`}
            className="grid gap-3 border-b border-[var(--c-border-subtle)] px-3.5 py-2 last:border-b-0"
            style={{ gridTemplateColumns: '70px 150px minmax(0,1fr)' }}
          >
            <span className="font-mono text-xs text-[var(--c-text-muted)]">
              {event.sequence_no}
            </span>
            <span className="font-mono text-xs text-[var(--c-text-secondary)]">
              {formatTimestamp(event.created_at)}
            </span>
            <div className="min-w-0">
              <div className="font-mono text-xs font-semibold text-[var(--c-text-primary)]">
                {event.event_type}
              </div>
              {event.payload ? (
                <div className="mt-2 rounded border border-[var(--c-border-subtle)] bg-[var(--c-app-bg)] p-2">
                  <CompactPayloadInspector value={event.payload} />
                </div>
              ) : null}
            </div>
          </div>
        ))}
      </div>
      {historyQuery.data?.has_more && historyQuery.data.next_after !== undefined ? (
        <Btn
          kind="secondary"
          type="button"
          disabled={historyQuery.isFetching}
          onClick={() => setAfter(historyQuery.data?.next_after)}
        >
          {historyQuery.isFetching ? 'Loading…' : 'Load more'}
        </Btn>
      ) : null}
    </div>
  );
}

function EngineResultPane({
  runId,
  status,
}: {
  runId: string;
  status: EngineRunStatus;
}) {
  const terminal = isTerminalEngineStatus(status);
  const resultQuery = useQuery({
    queryKey: ['engineRunResult', runId],
    queryFn: () => fetchEngineRunResult(runId),
    enabled: terminal,
  });

  if (!terminal) {
    return <EngineEmptyCard>Result is not available until the run reaches a terminal state.</EngineEmptyCard>;
  }
  if (resultQuery.isLoading) {
    return <EngineEmptyCard>Loading engine result…</EngineEmptyCard>;
  }
  if (resultQuery.isError || !resultQuery.data) {
    return <EngineEmptyCard>Engine result is temporarily unavailable.</EngineEmptyCard>;
  }

  const result = resultQuery.data;
  if (result.status === 'COMPLETED') {
    return (
      <div className="rounded-lg border border-[var(--c-border)] bg-[var(--c-surface)] p-3.5">
        <div className="text-[10.5px] font-semibold uppercase tracking-[0.05em] text-[var(--c-text-muted)]">
          Completed result
        </div>
        <div className="mt-3 rounded border border-[var(--c-border-subtle)] bg-[var(--c-app-bg)] p-3">
          <CompactPayloadInspector value={result.result} />
        </div>
      </div>
    );
  }

  return (
    <div className="rounded-lg border border-[var(--c-red-border)] bg-[var(--c-red-faint)] p-3.5">
      <div className="text-[10.5px] font-semibold uppercase tracking-[0.05em] text-[var(--c-red-text)]">
        {result.status} result shell
      </div>
      {result.failure ? (
        <>
          <div className="mt-2 font-mono text-xs text-[var(--c-red-text)]">
            {result.failure.error_code}
          </div>
          <p className="mt-2 text-sm text-[var(--c-text-primary)]">
            {result.failure.error_message}
          </p>
        </>
      ) : (
        <p className="mt-2 text-sm text-[var(--c-text-primary)]">
          Workflow result payload is null for this terminal shell.
        </p>
      )}
    </div>
  );
}

function isTerminalEngineStatus(status: EngineRunStatus): boolean {
  return (
    status === 'COMPLETED' ||
    status === 'FAILED' ||
    status === 'CANCELLED' ||
    status === 'TERMINATED' ||
    status === 'CONTINUED_AS_NEW'
  );
}

function EngineEmptyCard({ children }: { children: ReactNode }) {
  return (
    <div className="rounded-lg border border-dashed border-[var(--c-border)] bg-[var(--c-surface)] px-4 py-6 text-center text-[12.5px] text-[var(--c-text-muted)]">
      {children}
    </div>
  );
}

type ReplayMode = 'from-start' | 'from-cp';
type ReplayStepStatus = 'replayed' | 'current' | 'pending';

interface ReplayStep {
  id: string;
  name: string;
  status: ReplayStepStatus;
  mock: boolean;
  spanId?: string;
}

function ReplayGroup({ children, title }: { children: ReactNode; title: string }) {
  return (
    <div className="mb-4">
      <div className="mb-1.5 text-[10.5px] font-semibold uppercase tracking-[0.05em] text-[var(--c-text-muted)]">
        {title}
      </div>
      {children}
    </div>
  );
}

function ReplayRadioRow({
  checked,
  disabled = false,
  hint,
  label,
  onChange,
}: {
  checked: boolean;
  disabled?: boolean;
  hint: string;
  label: string;
  onChange: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onChange}
      disabled={disabled}
      aria-pressed={checked}
      className={`mb-1.5 flex w-full items-start gap-2.5 rounded border px-2.5 py-2 text-left transition disabled:cursor-not-allowed disabled:opacity-60 ${
        checked
          ? 'border-[var(--c-accent)] bg-[var(--c-row-selected-bg)]'
          : 'border-[var(--c-border)] bg-transparent hover:border-[var(--c-border-strong)]'
      }`}
    >
      <span
        className="mt-0.5 inline-flex h-3.5 w-3.5 shrink-0 items-center justify-center rounded-full"
        style={{
          border: `1.5px solid ${checked ? 'var(--c-accent)' : 'var(--c-border-strong)'}`,
        }}
      >
        {checked ? (
          <span className="h-1.5 w-1.5 rounded-full bg-[var(--c-accent)]" />
        ) : null}
      </span>
      <span>
        <span className="block text-xs font-medium text-[var(--c-text-primary)]">{label}</span>
        <span className="mt-0.5 block text-[11px] text-[var(--c-text-muted)]">{hint}</span>
      </span>
    </button>
  );
}

function ReplayToggleRow({
  checked,
  disabled = false,
  hint,
  label,
  onChange,
}: {
  checked: boolean;
  disabled?: boolean;
  hint: string;
  label: string;
  onChange: (next: boolean) => void;
}) {
  return (
    <label
      className={`flex items-start gap-2.5 py-1.5 ${
        disabled ? 'cursor-not-allowed opacity-60' : 'cursor-pointer'
      }`}
    >
      <span
        className="relative mt-0.5 inline-block h-4 w-7 shrink-0 rounded-full transition-colors"
        style={{
          background: checked ? 'var(--c-accent)' : 'var(--c-border-strong)',
        }}
      >
        <span
          className="absolute top-0.5 h-3 w-3 rounded-full bg-white shadow-sm transition-[left]"
          style={{ left: checked ? '14px' : '2px' }}
        />
      </span>
      <input
        type="checkbox"
        className="sr-only"
        checked={checked}
        disabled={disabled}
        onChange={(event) => onChange(event.target.checked)}
      />
      <span>
        <span className="block text-xs font-medium text-[var(--c-text-primary)]">{label}</span>
        <span className="mt-0.5 block text-[11px] text-[var(--c-text-muted)]">{hint}</span>
      </span>
    </label>
  );
}

function ReplayIconButton({
  disabled = false,
  icon: Icon,
  label,
  onClick,
}: {
  disabled?: boolean;
  icon: typeof Play;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      onClick={onClick}
      disabled={disabled}
      className="inline-flex h-7 w-7 items-center justify-center rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] text-[var(--c-text-secondary)] transition hover:text-[var(--c-text-primary)] disabled:cursor-not-allowed disabled:opacity-50"
    >
      <Icon className="h-3.5 w-3.5" />
    </button>
  );
}

function ReplayComingSoonBanner() {
  return (
    <div className="border-b border-[var(--c-amber-border)] bg-[var(--c-amber-faint)] px-5 py-3 text-[var(--c-amber-text)]">
      <div className="flex items-start gap-2.5">
        <Info className="mt-0.5 h-4 w-4 shrink-0" />
        <div>
          <div className="text-[12.5px] font-semibold text-[var(--c-text-primary)]">
            Replay is coming soon
          </div>
          <p className="mt-1 max-w-3xl text-[12px] leading-5 text-[var(--c-text-secondary)]">
            This preview shows the planned debugger workflow only. Runtime replay,
            sandboxed execution, checkpoints, mocked activities, and apply-as-new-run
            promotion are not connected in this checkout yet.
          </p>
        </div>
      </div>
    </div>
  );
}

function ReplayStepDot({ status }: { status: ReplayStepStatus }) {
  const color =
    status === 'replayed'
      ? 'var(--c-green)'
      : status === 'current'
        ? 'var(--c-blue)'
        : 'var(--c-border-strong)';
  return (
    <span
      className="inline-block h-2 w-2 rounded-full"
      style={{
        background: color,
        boxShadow: status === 'current' ? '0 0 0 3px rgba(38,124,255,0.18)' : 'none',
      }}
    />
  );
}

function ReplayDiffBlock({
  after,
  before,
  title,
}: {
  after: string[];
  before: string[];
  title: string;
}) {
  return (
    <div className="mb-3.5">
      <div className="mb-1.5 font-mono text-[11.5px] font-semibold text-[var(--c-text-secondary)]">
        {title}
      </div>
      <div className="overflow-hidden rounded border border-[var(--c-border)] font-mono text-[11.5px] leading-[1.65]">
        {before.map((line, index) => (
          <div
            key={`b-${index}`}
            className="px-3 py-0.5"
            style={{
              background: 'rgba(239,68,68,0.06)',
              color: 'var(--c-red-text)',
            }}
          >
            {line}
          </div>
        ))}
        {after.map((line, index) => (
          <div
            key={`a-${index}`}
            className="px-3 py-0.5"
            style={{
              background: 'rgba(16,185,129,0.07)',
              color: 'var(--c-green-text)',
            }}
          >
            {line}
          </div>
        ))}
      </div>
    </div>
  );
}

function TraceReplayPanel({
  events,
  onExportTrace,
  selectedSpanId,
  spans,
  trace,
}: {
  events: TimelineEvent[];
  onExportTrace: () => void;
  selectedSpanId: string | null;
  spans: Span[];
  trace: TraceDetail;
}) {
  const checkpoints = useMemo(
    () => events.filter((event) => event.event_type === 'snapshot_marker'),
    [events],
  );
  const failedSpan = useMemo(
    () => spans.find((span) => span.status === 'FAILED'),
    [spans],
  );
  const orderedSpans = useMemo(
    () =>
      [...spans].sort(
        (a, b) => new Date(a.started_at).getTime() - new Date(b.started_at).getTime(),
      ),
    [spans],
  );

  const [mode, setMode] = useState<ReplayMode>(checkpoints.length > 0 ? 'from-cp' : 'from-start');
  const [selectedCheckpoint, setSelectedCheckpoint] = useState<string | undefined>(
    checkpoints[checkpoints.length - 1]?.id,
  );
  const [overrides, setOverrides] = useState({
    mockFailures: trace.status === 'FAILED',
    skipBackoff: false,
    frozenClock: true,
  });
  const [running, setRunning] = useState(false);

  const steps = useMemo<ReplayStep[]>(() => {
    if (orderedSpans.length === 0) return [];
    const failedIndex = failedSpan
      ? orderedSpans.findIndex((span) => span.id === failedSpan.id)
      : -1;
    return orderedSpans.map((span, index) => {
      const isMock = overrides.mockFailures && failedIndex >= 0 && index >= failedIndex;
      const status: ReplayStepStatus =
        index < failedIndex || (failedIndex < 0 && span.status === 'COMPLETED')
          ? 'replayed'
          : index === failedIndex
            ? 'current'
            : 'pending';
      return {
        id: span.id,
        name: span.name,
        status,
        mock: isMock,
        spanId: span.span_id,
      };
    });
  }, [failedSpan, orderedSpans, overrides.mockFailures]);

  const stepIdx = Math.max(
    steps.findIndex((step) => step.status === 'current'),
    0,
  );
  const progress = steps.length > 0 ? Math.round(((stepIdx + 1) / steps.length) * 100) : 0;

  const failureMessage = failedSpan?.error_message ?? 'Activity failed';
  const replayControlsDisabled = true;

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <ReplayComingSoonBanner />
      <div className="flex min-h-0 flex-1">
      <aside className="hidden w-[320px] shrink-0 flex-col border-r border-[var(--c-border)] overflow-y-auto px-4 py-4 lg:flex">
        <ReplayGroup title="Replay source">
          <ReplayRadioRow
            checked={mode === 'from-start'}
            disabled={replayControlsDisabled}
            hint="Re-run full workflow history"
            label="From beginning"
            onChange={() => setMode('from-start')}
          />
          <ReplayRadioRow
            checked={mode === 'from-cp'}
            disabled={replayControlsDisabled || checkpoints.length === 0}
            hint={
              checkpoints.length > 0
                ? 'Resume from saved state'
                : 'No checkpoints recorded yet'
            }
            label="From checkpoint"
            onChange={() => checkpoints.length > 0 && setMode('from-cp')}
          />
          {mode === 'from-cp' && checkpoints.length > 0 ? (
            <select
              value={selectedCheckpoint ?? ''}
              disabled={replayControlsDisabled}
              onChange={(event) => setSelectedCheckpoint(event.target.value)}
              className="mt-1.5 w-full rounded border border-[var(--c-border)] bg-[var(--c-surface)] px-2.5 py-1.5 font-mono text-xs text-[var(--c-text-primary)] outline-none disabled:cursor-not-allowed disabled:opacity-60"
            >
              {checkpoints.map((checkpoint, index) => (
                <option key={checkpoint.id} value={checkpoint.id}>
                  cp-{index + 1} · {formatTimestamp(checkpoint.timestamp)}
                </option>
              ))}
            </select>
          ) : null}
        </ReplayGroup>

        <ReplayGroup title="Determinism overrides">
          <ReplayToggleRow
            checked={overrides.mockFailures}
            disabled={replayControlsDisabled}
            hint="Replace failing call with success response"
            label="Mock failed activities"
            onChange={(next) => setOverrides((prev) => ({ ...prev, mockFailures: next }))}
          />
          <ReplayToggleRow
            checked={overrides.skipBackoff}
            disabled={replayControlsDisabled}
            hint="Collapse retry backoff timers"
            label="Skip retry backoff"
            onChange={(next) => setOverrides((prev) => ({ ...prev, skipBackoff: next }))}
          />
          <ReplayToggleRow
            checked={overrides.frozenClock}
            disabled={replayControlsDisabled}
            hint="Pin time to original run"
            label="Frozen clock"
            onChange={(next) => setOverrides((prev) => ({ ...prev, frozenClock: next }))}
          />
        </ReplayGroup>

        <ReplayGroup title="Mocked response">
          <div className="rounded border border-[var(--c-border)] bg-[var(--c-surface)] p-2.5 font-mono text-[11.5px] leading-[1.55] text-[var(--c-text-primary)]">
            <div className="text-[var(--c-text-muted)]">
              {failedSpan ? `// ${failedSpan.name}` : '// no failure to mock'}
            </div>
            <div>{'{'}</div>
            <div className="pl-3">
              <span className="text-[var(--c-blue-text)]">"status"</span>:{' '}
              <span className="text-[var(--c-green-text)]">"succeeded"</span>,
            </div>
            <div className="pl-3">
              <span className="text-[var(--c-blue-text)]">"latency_ms"</span>:{' '}
              <span className="text-[var(--c-green-text)]">12</span>
            </div>
            <div>{'}'}</div>
          </div>
        </ReplayGroup>

        <div className="mt-2 flex flex-wrap gap-2">
          <Btn
            kind="primary"
            leadingIcon={running ? Pause : Play}
            size="sm"
            type="button"
            disabled={replayControlsDisabled}
            onClick={() => setRunning((prev) => !prev)}
          >
            {running ? 'Pause' : 'Run replay'}
          </Btn>
          <Btn
            kind="secondary"
            leadingIcon={RotateCcw}
            size="sm"
            type="button"
            disabled={replayControlsDisabled}
            onClick={() => setRunning(false)}
          >
            Reset
          </Btn>
          <Btn
            kind="secondary"
            leadingIcon={Download}
            size="sm"
            type="button"
            onClick={onExportTrace}
          >
            Export
          </Btn>
        </div>
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <div
          className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--c-border-subtle)] px-5 py-3"
          style={{ background: running ? 'rgba(38,124,255,0.06)' : 'var(--c-surface-muted)' }}
        >
          <div className="flex items-center gap-2.5">
            <span
              className="inline-block h-2 w-2 rounded-full"
              style={{
                background: running ? 'var(--c-blue)' : 'var(--c-text-muted)',
                animation: running ? 'continua-pulse 1.6s ease-out infinite' : 'none',
              }}
            />
            <span className="text-[12.5px] font-medium text-[var(--c-text-primary)]">
              {running
                ? overrides.mockFailures
                  ? 'Replaying with mocked failures'
                  : 'Replaying original run'
                : 'Replay preview only'}
            </span>
            <span className="font-mono text-[11.5px] text-[var(--c-text-muted)]">
              · step {steps.length === 0 ? 0 : stepIdx + 1} of {steps.length}
            </span>
          </div>
          <div className="flex items-center gap-1">
            <ReplayIconButton
              disabled={replayControlsDisabled}
              icon={SkipBack}
              label="Previous step"
              onClick={() => setRunning(false)}
            />
            <ReplayIconButton
              disabled={replayControlsDisabled}
              icon={running ? Pause : Play}
              label={running ? 'Pause' : 'Play'}
              onClick={() => setRunning((prev) => !prev)}
            />
            <ReplayIconButton
              disabled={replayControlsDisabled}
              icon={SkipForward}
              label="Next step"
              onClick={() => setRunning(false)}
            />
          </div>
        </div>

        <div className="border-b border-[var(--c-border-subtle)] px-5 py-3">
          <div className="relative h-1 bg-[var(--c-surface-muted)]">
            <div
              className="absolute inset-y-0 left-0 bg-[var(--c-accent)]"
              style={{ width: `${progress}%` }}
            />
            {checkpoints.map((checkpoint, index) => {
              const ts = new Date(checkpoint.timestamp).getTime();
              const start = new Date(trace.started_at).getTime();
              const end = trace.ended_at ? new Date(trace.ended_at).getTime() : ts;
              const total = Math.max(end - start, 1);
              const pct = Math.min(Math.max(((ts - start) / total) * 100, 0), 100);
              const active = checkpoint.id === selectedCheckpoint;
              return (
                <div
                  key={checkpoint.id}
                  className="absolute -top-1 -bottom-1 w-0.5"
                  style={{
                    left: `${pct}%`,
                    background: active ? 'var(--c-accent)' : 'var(--c-text-muted)',
                    opacity: active ? 1 : 0.4,
                  }}
                  title={`cp-${index + 1}`}
                />
              );
            })}
          </div>
          <div className="mt-1.5 flex justify-between font-mono text-[10.5px] text-[var(--c-text-muted)]">
            <span>start</span>
            {checkpoints.length > 0 ? (
              <span style={{ color: 'var(--c-accent)' }}>
                {checkpoints.length} checkpoint{checkpoints.length === 1 ? '' : 's'}
              </span>
            ) : null}
            <span>end</span>
          </div>
        </div>

        <div className="flex min-h-0 flex-1">
          <div className="hidden w-[320px] shrink-0 overflow-y-auto border-r border-[var(--c-border)] md:block">
            {steps.length === 0 ? (
              <div className="px-4 py-6 text-[12.5px] text-[var(--c-text-muted)]">
                No spans available to step through yet.
              </div>
            ) : (
              steps.map((step) => {
                const active = stepIdx === steps.findIndex((s) => s.id === step.id);
                return (
                  <div
                    key={step.id}
                    className="grid items-center gap-2 px-3.5 py-2 text-xs transition"
                    style={{
                      gridTemplateColumns: '24px minmax(0,1fr) auto',
                      borderBottom: '1px solid var(--c-border-subtle)',
                      borderLeft: `2px solid ${active ? 'var(--c-accent)' : 'transparent'}`,
                      background: active ? 'var(--c-row-selected-bg)' : 'transparent',
                    }}
                  >
                    <span className="text-right font-mono text-[10.5px] text-[var(--c-text-muted)]">
                      {steps.findIndex((s) => s.id === step.id) + 1}
                    </span>
                    <span
                      className="truncate font-mono"
                      style={{
                        color:
                          step.status === 'replayed'
                            ? 'var(--c-text-secondary)'
                            : step.status === 'current'
                              ? 'var(--c-text-primary)'
                              : 'var(--c-text-muted)',
                        fontWeight: step.status === 'current' ? 600 : 500,
                      }}
                    >
                      {step.name}
                    </span>
                    <span className="inline-flex items-center gap-1">
                      {step.mock ? (
                        <span className="rounded border border-[var(--c-amber-border)] bg-[var(--c-amber-faint)] px-1 font-mono text-[9px] uppercase text-[var(--c-amber-text)]">
                          MOCK
                        </span>
                      ) : null}
                      <ReplayStepDot status={step.status} />
                    </span>
                  </div>
                );
              })
            )}
          </div>

          <div className="min-w-0 flex-1 overflow-y-auto px-5 py-4">
            <div className="mb-3">
              <div className="text-[12.5px] font-semibold text-[var(--c-text-primary)]">
                Divergence from original run
              </div>
              <div className="mt-1 text-[11.5px] text-[var(--c-text-muted)]">
                {selectedSpanId
                  ? `Showing changes around span ${selectedSpanId}.`
                  : 'Showing changes if the failed step is mocked to succeed.'}
              </div>
            </div>

            {failedSpan ? (
              <ReplayDiffBlock
                after={[
                  '+ status: "succeeded"',
                  '+ duration_ms: 12 (mocked)',
                  '+ attempts: 1',
                ]}
                before={[
                  '- status: "failed"',
                  `- error: ${JSON.stringify(failureMessage)}`,
                  `- duration_ms: ${failedSpan.latency_ms ?? 0}`,
                ]}
                title={failedSpan.name}
              />
            ) : (
              <div className="rounded border border-dashed border-[var(--c-border)] px-4 py-6 text-center text-[12.5px] text-[var(--c-text-muted)]">
                No failure detected on this run — mocked replay would mirror the original outcome.
              </div>
            )}

            {failedSpan ? (
              <ReplayDiffBlock
                after={[
                  '+ trace.status: "completed"',
                  '+ projected_close_time: shortened',
                ]}
                before={[
                  `- trace.status: ${JSON.stringify(trace.status.toLowerCase())}`,
                  `- error_count: ${trace.error_count ?? 0}`,
                ]}
                title="Trace outcome"
              />
            ) : null}

            <div className="mt-4 flex items-start gap-2 rounded border border-[var(--c-border)] bg-[var(--c-surface-muted)] px-3 py-2.5 text-[11.5px] text-[var(--c-text-secondary)]">
              <Info className="mt-0.5 h-3.5 w-3.5 shrink-0 text-[var(--c-text-muted)]" />
              <span>
                Planned replay will run in a sandboxed worker with buffered state writes
                before promotion as a new run.
                <Btn
                  kind="ghost"
                  leadingIcon={ExternalLink}
                  size="sm"
                  type="button"
                  className="ml-2"
                  onClick={onExportTrace}
                >
                  Export trace
                </Btn>
              </span>
            </div>
          </div>
        </div>
      </div>
      </div>
    </div>
  );
}

function TraceHeaderMetric({
  label,
  value,
  danger = false,
  mono = false,
}: {
  label: string;
  value: string;
  danger?: boolean;
  mono?: boolean;
}) {
  return (
    <div className="flex flex-col gap-0.5">
      <div className="text-[10.5px] font-medium uppercase tracking-[0.04em] text-[var(--c-text-muted)]">
        {label}
      </div>
      <div
        className={`text-[13px] font-medium tabular-nums ${mono ? 'font-mono' : ''} ${
          danger ? 'text-[var(--c-red-text)]' : 'text-[var(--c-text-primary)]'
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
    <div className="app-overlay-enter fixed inset-0 z-50 hidden bg-[#111318]/40 backdrop-blur-sm md:block">
      <button
        type="button"
        aria-label="Close trace context drawer"
        className="absolute inset-0"
        onClick={onClose}
      />
      <aside
        className="app-drawer-enter absolute inset-y-4 right-4 w-[36rem] max-w-[calc(100vw-2rem)] overflow-y-auto"
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
    <div className="app-overlay-enter fixed inset-0 z-50 flex items-end bg-[#111318]/50 backdrop-blur-sm md:hidden">
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
    <section className="overflow-hidden rounded-md border border-[var(--c-border)] bg-[var(--c-app-bg)] shadow-[0_14px_36px_rgba(15,23,42,0.14)]">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-[var(--c-border)] bg-[var(--c-app-bg)] px-4 py-3">
        <button
          type="button"
          className="flex items-center gap-3 text-left"
          aria-expanded={open}
          onClick={onToggle}
        >
          <span className="text-xs font-semibold uppercase tracking-[0.18em] text-[var(--c-text-primary)]">
            Trace Context
          </span>
          <span className="text-xs font-medium text-[var(--c-text-muted)]">
            {open ? 'Hide' : 'Show'}
          </span>
        </button>
        <CopyButton
          aria-label="Copy Trace URL"
          className="shrink-0 rounded-md border-[var(--c-border)] bg-[var(--c-surface)] text-[var(--c-text-secondary)]"
          getValue={buildCopyTraceUrl}
          idleLabel="Copy Trace URL"
          successLabel="Copied URL"
        />
      </div>

      {open ? (
        <div className="space-y-5 p-4">
          <div className="overflow-hidden rounded-md border border-[var(--c-border)]">
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
                  className="inline-flex min-w-0 flex-col text-left text-[var(--c-accent-text)] hover:opacity-80"
                >
                  <span className="truncate text-xs font-medium text-[var(--c-text-primary)]">
                    {trace.session_external_id ?? trace.session_id}
                  </span>
                  <span className="truncate font-mono text-[11px] text-[var(--c-text-muted)]">
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
              value={trace.tags && trace.tags.length > 0 ? (
                <div className="flex flex-wrap gap-2">
                  {trace.tags.map((tag) => (
                    <span
                      key={tag}
                      className="rounded border border-[var(--c-border)] bg-[var(--c-surface)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--c-text-secondary)]"
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
            <div className="space-y-4">
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
  copyValue,
  copyButtonLabel,
}: {
  label: string;
  value: ReactNode;
  copyValue?: string;
  copyButtonLabel?: string;
}) {
  return (
    <div className="grid grid-cols-[9rem_minmax(0,1fr)] gap-4 border-b border-[var(--c-border-subtle)] px-3 py-2.5 last:border-b-0">
      <div className="text-[11px] font-semibold uppercase tracking-[0.14em] text-[var(--c-text-muted)]">
        {label}
      </div>
      <div className="flex min-w-0 items-start justify-between gap-3">
        <div className="min-w-0 flex-1 text-xs text-[var(--c-text-primary)]">{value}</div>
        {copyValue && copyButtonLabel ? (
          <CopyButton
            aria-label={copyButtonLabel}
            className="h-6 shrink-0 rounded-md border-[var(--c-border)] bg-[var(--c-surface)] px-2 text-[11px] text-[var(--c-text-secondary)]"
            value={copyValue}
          />
        ) : null}
      </div>
    </div>
  );
}

function TracePayloadPanel({ title, data }: { title: string; data: unknown }) {
  return (
    <section className="overflow-hidden rounded-md border border-[var(--c-border)] bg-[var(--c-app-bg)]">
      <div className="border-b border-[var(--c-border)] bg-[var(--c-table-head-bg)] px-3 py-2">
        <h3 className="text-xs font-semibold text-[var(--c-text-primary)]">{title}</h3>
      </div>
      <div className="max-h-80 overflow-auto p-3">
        <CompactPayloadInspector value={data} />
      </div>
    </section>
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
    return <span className="text-xs text-[var(--c-text-muted)]">-</span>;
  }

  return (
    <span
      className={
        monospace
          ? 'font-mono text-xs text-[var(--c-text-primary)]'
          : 'text-xs text-[var(--c-text-primary)]'
      }
    >
      {value}
    </span>
  );
}
