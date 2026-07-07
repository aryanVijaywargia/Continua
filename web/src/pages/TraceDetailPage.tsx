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
import { ReasoningTab } from '../components/ReasoningTab';
import { useMediaQuery } from '../hooks/useMediaQuery';
import { buildTraceLineageChain, getReturnToDestination } from '../utils/traceLineage';
import { getProjectIdFromSearchParams } from '../utils/projectSearchParams';
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
import { TraceDetailsSurface } from './traceDetail/TraceDetailsSurface';
import { TraceContextDrawer, TraceContextSheet } from './traceDetail/TraceContextPanels';
import { TraceEngineSection } from './traceDetail/TraceEngineSection';
import { TraceLineageCard } from './traceDetail/TraceLineagePanels';
import { TraceLogsSection } from './traceDetail/TraceLogsSection';
import { TraceMetricsSection } from './traceDetail/TraceMetricsSection';
import { TraceReplaySection } from './traceDetail/TraceReplaySection';
import { TraceTimelineSection } from './traceDetail/TraceTimelineSection';
import { WorkspaceOverviewSection } from './traceDetail/WorkspaceOverviewSection';
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
const TRACE_CONTEXT_DRAWER_MEDIA_QUERY = '(min-width: 768px)';

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
 * state (active section, trace-context overlay); everything else comes from
 * the workspace context.
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
    projectId,
    reasoningEntries,
    returnTo,
    selectSpanAndShowDetails,
    spans,
    trace,
  } = useTraceDetailWorkspace();
  const isDesktop = useMediaQuery(DESKTOP_MEDIA_QUERY);
  const isContextDrawer = useMediaQuery(TRACE_CONTEXT_DRAWER_MEDIA_QUERY);
  const [activeSection, setActiveSection] =
    useState<TraceDetailSectionId>('overview');
  const [isTraceContextOpen, setIsTraceContextOpen] = useState(false);
  const replayPreviewEnabled = isReplayPreviewEnabled();

  useEffect(() => {
    if (activeSection === 'engine' && !trace.engine) {
      setActiveSection('overview');
    }
    if (activeSection === 'replay' && !replayPreviewEnabled) {
      setActiveSection('overview');
    }
  }, [activeSection, replayPreviewEnabled, trace.engine]);

  const timelineAuthError = isAuthError(timelineRawError);

  const mobileSummaryContent = (
    <div className="grid h-full gap-4 overflow-y-auto p-4">
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
      <TraceDetailsSurface />
      <ReasoningTab
        entries={reasoningEntries}
        onSelectSpan={selectSpanAndShowDetails}
      />
    </div>
  );

  const workspaceContent = (
    <WorkspaceOverviewSection
      isDesktop={isDesktop}
      mobileSummary={mobileSummaryContent}
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
        <TraceTimelineSection />
      </TraceSectionSurface>
    ) : activeSection === 'logs' ? (
      <TraceSectionSurface
        flush
        title="Logs"
        description="Explicit logs, errors, exceptions, decisions, effects, and waits recorded by the trace."
      >
        <TraceLogsSection />
      </TraceSectionSurface>
    ) : activeSection === 'metrics' ? (
      <TraceSectionSurface
        title="Metrics"
        description="Aggregate latency, token, cost, and state-change signals from the loaded spans."
      >
        <TraceMetricsSection />
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
      workspaceContent
    );

  return (
    <div className="relative flex min-h-0 flex-1 flex-col">
      <TraceDetailHeader
        isDesktop={isDesktop}
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
        onChange={setActiveSection}
        replayPreviewEnabled={replayPreviewEnabled}
        spanCount={spans.length}
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

      {isContextDrawer && isTraceContextOpen ? (
        <TraceContextDrawer
          childTraces={childTraces}
          childTracesLoading={childTracesLoading}
          hasLineageError={hasChildTracesError}
          onClose={() => setIsTraceContextOpen(false)}
        />
      ) : null}

      {!isContextDrawer && isTraceContextOpen ? (
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
