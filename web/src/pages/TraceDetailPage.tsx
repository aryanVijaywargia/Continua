import { useEffect, useMemo, useState } from 'react';
import { useLocation, useParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import {
  fetchSpans,
  fetchTrace,
  isAuthError,
  type Span,
  type Trace,
} from '../api/client';
import { AuthErrorBanner } from '../components/AuthErrorBanner';
import { ExecutionWaterfall } from '../components/ExecutionWaterfall';
import { useStepDurations } from '../components/trace/useStepDurations';
import { ReasoningTab } from '../components/ReasoningTab';
import { useMediaQuery } from '../hooks/useMediaQuery';
import { buildTraceLineageChain, getReturnToDestination } from '../utils/traceLineage';
import { getProjectIdFromSearchParams } from '../utils/projectSearchParams';
import { deriveVisibleRows } from '../utils/spanTree';
import {
  TIMELINE_POLL_INTERVAL_MS,
  useTraceTimeline,
} from './useTraceTimeline';
import { useEnginePendingWork } from './useEnginePendingWork';
import {
  DefinitionVersionMismatchBanner,
  TraceDetailEmptyState,
  TraceDetailErrorState,
  TraceDetailTabs,
  TraceSectionSurface,
  type TraceDetailSectionId,
} from './traceDetail/TraceDetailChrome';
import { TraceDetailHeader } from './traceDetail/TraceDetailHeader';
import { TraceDetailWorkspaceProvider } from './traceDetail/TraceDetailWorkspaceProvider';
import { TraceContextDrawer, TraceContextSheet } from './traceDetail/TraceContextPanels';
import { TraceEngineSection } from './traceDetail/TraceEngineSection';
import { TraceLineageCard } from './traceDetail/TraceLineagePanels';
import { TraceLogsSection } from './traceDetail/TraceLogsSection';
import { TraceMetricsSection } from './traceDetail/TraceMetricsSection';
import { TraceReplaySection } from './traceDetail/TraceReplaySection';
import { TraceTimelineSection } from './traceDetail/TraceTimelineSection';
import { MobileStepList } from './traceDetail/MobileStepList';
import { TraceInspector } from './traceDetail/TraceInspector';
import { TraceStatePanels } from './traceDetail/TraceStatePanels';
import {
  fetchDirectChildTraces,
  fetchTraceLineageAncestors,
} from './traceDetail/lineageQueries';
import { queryErrorMessage } from './traceDetail/queryError';
import {
  useTraceDetailWorkspace,
  type PendingWorkState,
} from './traceDetail/traceDetailWorkspaceContext';

const EMPTY_SPANS: Span[] = [];
const EMPTY_TRACES: Trace[] = [];
const DESKTOP_MEDIA_QUERY = '(min-width: 1024px)';
const MEDIUM_MEDIA_QUERY = '(min-width: 768px)';

function isReplayPreviewEnabled(): boolean {
  return import.meta.env.VITE_CONTINUA_REPLAY_PREVIEW === '1';
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

/**
 * Data-fetching orchestration: queries, the polling timeline, and the
 * workspace provider. Layout and section switching live in
 * {@link TraceDetailBody} beneath the provider.
 */
function TraceDetailContent({ traceId }: TraceDetailContentProps) {
  const location = useLocation();
  const currentProjectId = getProjectIdFromSearchParams(
    new URLSearchParams(location.search)
  );
  const projectQueryKey = currentProjectId ?? null;
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
  const timelineStatus = trace ? timeline.traceStatus ?? trace.status : timeline.traceStatus;
  const returnTo = getReturnToDestination(location.state);

  const pendingWork = useMemo<PendingWorkState>(
    () => ({
      data: pendingWorkQuery.data,
      isLoading: pendingWorkQuery.isLoading,
      isError: pendingWorkQuery.isError,
      errorMessage: queryErrorMessage(pendingWorkQuery.error),
    }),
    [
      pendingWorkQuery.data,
      pendingWorkQuery.error,
      pendingWorkQuery.isError,
      pendingWorkQuery.isLoading,
    ]
  );

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

  return (
    <TraceDetailWorkspaceProvider
      traceId={traceId}
      trace={trace}
      spans={spans}
      events={timeline.events}
      timelineStatus={timelineStatus}
      hasTimelineSnapshot={timeline.hasSnapshot}
      isSpanDataReady={spansQuery.isSuccess}
      pendingWork={pendingWork}
      projectId={currentProjectId}
      returnTo={returnTo}
    >
      <TraceDetailBody
        childTraces={childTraces}
        childTracesLoading={childTracesQuery.isLoading}
        hasChildTracesError={childTracesQuery.isError}
        lineageChain={lineageChain}
        lineageLoading={lineageAncestorsQuery.isLoading}
        timelineRawError={timeline.rawError}
      />
    </TraceDetailWorkspaceProvider>
  );
}

/**
 * Layout and section switching for a loaded trace. Owns only page-local UI
 * state (active section, events view, narrow step view, trace-context
 * overlay); everything else comes from the workspace context.
 */
function TraceDetailBody({
  childTraces,
  childTracesLoading,
  hasChildTracesError,
  lineageChain,
  lineageLoading,
  timelineRawError,
}: {
  childTraces: Trace[];
  childTracesLoading: boolean;
  hasChildTracesError: boolean;
  lineageChain: Trace[];
  lineageLoading: boolean;
  timelineRawError: unknown;
}) {
  const {
    events,
    expandedSpanIds,
    projectId,
    reasoningEntries,
    returnTo,
    selectSpan,
    selectSpanAndShowDetails,
    selectedSpanId,
    spanTree,
    spans,
    toggleExpandedSpan,
    trace,
    visibleRetrySafetyAssessments,
  } = useTraceDetailWorkspace();
  const isDesktop = useMediaQuery(DESKTOP_MEDIA_QUERY);
  const isMediumUp = useMediaQuery(MEDIUM_MEDIA_QUERY);
  const isNarrow = !isMediumUp;
  const [activeSection, setActiveSection] =
    useState<TraceDetailSectionId>('execution');
  const [eventsView, setEventsView] = useState<'timeline' | 'logs'>('timeline');
  const [isNarrowStepOpen, setIsNarrowStepOpen] = useState(false);
  const [isTraceContextOpen, setIsTraceContextOpen] = useState(false);
  const replayPreviewEnabled = isReplayPreviewEnabled();
  const durations = useStepDurations(spans);
  const visibleRows = useMemo(
    () => deriveVisibleRows(spanTree, expandedSpanIds),
    [expandedSpanIds, spanTree]
  );

  useEffect(() => {
    if (activeSection === 'engine' && !trace.engine) {
      setActiveSection('execution');
    }
    if (activeSection === 'replay' && !replayPreviewEnabled) {
      setActiveSection('execution');
    }
  }, [activeSection, replayPreviewEnabled, trace.engine]);

  const timelineAuthError = isAuthError(timelineRawError);

  const executionTable = (
    <ExecutionWaterfall
      events={events}
      rows={visibleRows}
      spans={spans}
      durations={durations}
      selectedSpanId={selectedSpanId}
      onSelectSpan={selectSpan}
      revealTarget={selectedSpanId}
      expandedSpanIds={expandedSpanIds}
      onToggleExpand={toggleExpandedSpan}
      spanAssessments={visibleRetrySafetyAssessments}
      traceEndedAt={trace.ended_at}
      traceStartedAt={trace.started_at}
    />
  );

  const executionContent = isNarrow ? (
    isNarrowStepOpen && selectedSpanId ? (
      <TraceInspector durations={durations} onBack={() => setIsNarrowStepOpen(false)} />
    ) : (
      <MobileStepList
        durations={durations}
        onOpenStep={(spanId) => {
          selectSpan(spanId);
          setIsNarrowStepOpen(true);
        }}
      />
    )
  ) : isDesktop ? (
    <div className="grid min-h-0 flex-1 grid-cols-[minmax(0,1fr)_minmax(22rem,26rem)]">
      <div className="min-h-0 min-w-0">{executionTable}</div>
      <div className="min-h-0 border-l border-[var(--c-border)]">
        <TraceInspector durations={durations} />
      </div>
    </div>
  ) : (
    <div className="grid min-h-0 flex-1 grid-rows-[minmax(18rem,1fr)_minmax(20rem,1fr)]">
      <div className="min-h-0 min-w-0">{executionTable}</div>
      <div className="min-h-0 border-t border-[var(--c-border)]">
        <TraceInspector durations={durations} />
      </div>
    </div>
  );

  const eventsViewToggle = (
    <div className="flex gap-1 border-b border-[var(--c-border)] px-4 py-2 md:px-6">
      {(
        [
          ['timeline', 'Timeline view'],
          ['logs', 'Logs and errors'],
        ] as const
      ).map(([id, label]) => (
        <button
          key={id}
          type="button"
          aria-pressed={eventsView === id}
          onClick={() => setEventsView(id)}
          className={`h-7 rounded-md border px-2.5 text-xs font-medium ${
            eventsView === id
              ? 'border-[var(--c-accent-border)] bg-[var(--c-accent-faint)] text-[var(--c-accent-text)]'
              : 'border-[var(--c-border)] text-[var(--c-text-secondary)] hover:text-[var(--c-text-primary)]'
          }`}
        >
          {label}
        </button>
      ))}
    </div>
  );

  const sectionContent =
    activeSection === 'events' ? (
      <div className="flex min-h-0 flex-1 flex-col">
        {eventsViewToggle}
        {eventsView === 'timeline' ? (
          <TraceSectionSurface
            flush
            title="Timeline"
            description="Chronological trace events with step selection preserved."
          >
            <TraceTimelineSection />
          </TraceSectionSurface>
        ) : (
          <TraceSectionSurface
            flush
            title="Logs"
            description="Explicit logs, errors, exceptions, decisions, effects, and waits recorded by the trace."
          >
            <TraceLogsSection />
          </TraceSectionSurface>
        )}
      </div>
    ) : activeSection === 'metrics' ? (
      <TraceSectionSurface
        title="Metrics"
        description="Trace state, lineage, reasoning, and aggregate latency, token, cost, and state-change signals."
      >
        <div className="grid gap-4">
          <TraceStatePanels />
          <TraceLineageCard
            childTraces={childTraces}
            childTracesLoading={childTracesLoading}
            hasChildTracesError={hasChildTracesError}
            lineageChain={lineageChain}
            lineageLoading={lineageLoading}
            projectId={projectId}
            returnTo={returnTo}
            showLineageSummary
            showEmptyChildren={Boolean(trace.engine?.parent_run_id)}
            trace={trace}
          />
          <TraceMetricsSection />
          <ReasoningTab
            entries={reasoningEntries}
            onSelectSpan={(spanId) => {
              selectSpanAndShowDetails(spanId);
              setActiveSection('execution');
            }}
          />
        </div>
      </TraceSectionSurface>
    ) : activeSection === 'engine' ? (
      <TraceSectionSurface
        flush
        title="Engine state"
        description="Current engine projection, wait state, and queued work for this trace."
      >
        <TraceEngineSection />
      </TraceSectionSurface>
    ) : activeSection === 'replay' && replayPreviewEnabled ? (
      <TraceSectionSurface
        flush
        title="Replay"
        description="Replay readiness and export actions for this trace."
      >
        <TraceReplaySection />
      </TraceSectionSurface>
    ) : (
      executionContent
    );

  return (
    <div className="relative flex min-h-0 flex-1 flex-col">
      <TraceDetailHeader
        isDesktop={isDesktop}
        isNarrow={isNarrow}
        isTraceContextOpen={isTraceContextOpen}
        lineageChain={lineageChain}
        lineageLoading={lineageLoading}
        onShowReplay={() => setActiveSection('replay')}
        onToggleTraceContext={() => setIsTraceContextOpen((open) => !open)}
        replayPreviewEnabled={replayPreviewEnabled}
      />

      <TraceDetailTabs
        activeSection={activeSection}
        eventCount={events.length}
        hasEngine={Boolean(trace.engine)}
        isNarrow={isNarrow}
        onChange={setActiveSection}
        replayPreviewEnabled={replayPreviewEnabled}
      />

      {timelineAuthError ? (
        <AuthErrorBanner message={queryErrorMessage(timelineRawError)} />
      ) : null}

      {trace.engine?.failure?.error_code === 'definition_version_mismatch' ? (
        <DefinitionVersionMismatchBanner />
      ) : null}
      <div className="flex min-h-0 flex-1 flex-col">
        {sectionContent}
      </div>

      {isMediumUp && isTraceContextOpen ? (
        <TraceContextDrawer
          childTraces={childTraces}
          childTracesLoading={childTracesLoading}
          hasLineageError={hasChildTracesError}
          onClose={() => setIsTraceContextOpen(false)}
        />
      ) : null}

      {!isMediumUp && isTraceContextOpen ? (
        <TraceContextSheet
          childTraces={childTraces}
          childTracesLoading={childTracesLoading}
          hasLineageError={hasChildTracesError}
          onClose={() => setIsTraceContextOpen(false)}
        />
      ) : null}
    </div>
  );
}
