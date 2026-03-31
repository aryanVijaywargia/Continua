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
  fetchSpans,
  fetchTrace,
  isAuthError,
  type Span,
  type TimelineEvent,
  type TraceDetail,
} from '../api/client';
import { AuthErrorBanner } from '../components/AuthErrorBanner';
import { CopyButton } from '../components/CopyButton';
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
import { useRequireApiKey } from '../hooks/useRequireApiKey';
import { useTraceDetailSearchParams } from '../hooks/useTraceDetailSearchParams';
import { useWorkspaceState } from '../hooks/useWorkspaceState';
import type { RetrySafetyAssessment } from '../utils/retrySafety';
import {
  TIMELINE_POLL_INTERVAL_MS,
  useTraceTimeline,
} from './useTraceTimeline';
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
const EMPTY_RETRY_SAFETY_ASSESSMENTS = new Map<string, RetrySafetyAssessment>();
const DESKTOP_MEDIA_QUERY = '(min-width: 1024px)';

function queryErrorMessage(error: unknown): string {
  return error instanceof Error ? error.message : 'Unknown error';
}

export function TraceDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { hasApiKey, prompt } = useRequireApiKey();

  if (!hasApiKey) {
    return prompt;
  }

  if (!id) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-slate-50 dark:bg-slate-950">
        <div className="text-red-600">Trace ID is required</div>
      </div>
    );
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
  const { spanParam, setSpanParam } = useTraceDetailSearchParams();
  const [activeMobileTab, setActiveMobileTab] =
    useState<MobileWorkspaceTabId>('details');
  const [isTraceContextOpen, setIsTraceContextOpen] = useState(false);
  const switchToInspectorDetailsRef = useRef<(() => void) | null>(null);
  const traceQuery = useQuery({
    queryKey: ['trace', traceId],
    queryFn: () => fetchTrace(traceId),
  });
  const timeline = useTraceTimeline(traceId);
  const liveTraceStatus = timeline.traceStatus ?? traceQuery.data?.status ?? null;
  const spansQuery = useQuery({
    queryKey: ['spans', traceId],
    queryFn: () => fetchSpans(traceId),
    refetchInterval:
      liveTraceStatus === 'RUNNING' ? TIMELINE_POLL_INTERVAL_MS : false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  const trace = traceQuery.data ?? null;
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
    setActiveMobileTab('details');
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
    return (
      <div className="flex min-h-screen items-center justify-center bg-slate-50 dark:bg-slate-950">
        <div className="text-slate-500 dark:text-slate-400">Loading trace...</div>
      </div>
    );
  }

  if (traceQuery.error) {
    return (
      <div className="min-h-screen bg-slate-50 dark:bg-slate-950">
        <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6 lg:px-8">
          {isAuthError(traceQuery.error) ? (
            <AuthErrorBanner message={queryErrorMessage(traceQuery.error)} />
          ) : (
            <div className="rounded-xl border border-red-200 bg-red-50 p-4 text-red-700 dark:border-red-500/40 dark:bg-red-500/10 dark:text-red-200">
              Error loading trace: {queryErrorMessage(traceQuery.error)}
            </div>
          )}
        </div>
      </div>
    );
  }

  if (spansQuery.error) {
    return (
      <div className="min-h-screen bg-slate-50 dark:bg-slate-950">
        <div className="mx-auto max-w-4xl px-4 py-8 sm:px-6 lg:px-8">
          {isAuthError(spansQuery.error) ? (
            <AuthErrorBanner message={queryErrorMessage(spansQuery.error)} />
          ) : (
            <div className="rounded-xl border border-red-200 bg-red-50 p-4 text-red-700 dark:border-red-500/40 dark:bg-red-500/10 dark:text-red-200">
              Error loading spans: {queryErrorMessage(spansQuery.error)}
            </div>
          )}
        </div>
      </div>
    );
  }

  if (!trace) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-slate-50 dark:bg-slate-950">
        <div className="text-red-600">Trace not found</div>
      </div>
    );
  }

  const detailsContent = (
    <TraceDetailsSurface
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
      traceContext={isDesktop ? null : (
        <TraceContextSection
          buildCopyTraceUrl={buildCopyTraceUrl}
          onToggle={() => setIsTraceContextOpen((open) => !open)}
          open={isTraceContextOpen}
          trace={trace}
        />
      )}
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

  return (
    <div className="flex min-h-screen flex-col bg-slate-50 dark:bg-slate-950">
      <header className="border-b border-slate-200 bg-white px-6 py-4 dark:border-slate-800 dark:bg-slate-900">
        <div className="flex items-center gap-4">
          <Link to={returnTo} className="text-slate-500 hover:text-slate-700 dark:text-slate-400 dark:hover:text-slate-200">
            {returnTo.startsWith('/sessions/') ? '← Session' : '← Traces'}
          </Link>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="truncate text-xl font-semibold text-slate-900 dark:text-slate-100">
                {trace.name}
              </h1>
              <StatusBadge status={timelineStatus ?? trace.status} />
            </div>
            <div className="mt-2 flex flex-wrap items-center gap-4 text-sm text-slate-500 dark:text-slate-400">
              <span>{formatDuration(duration)}</span>
              <span>{formatTokens(totalTokens)} tokens</span>
              <span>{formatCost(trace.total_cost_usd)}</span>
              {trace.error_count && trace.error_count > 0 ? (
                <span className="text-red-600 dark:text-red-300">{trace.error_count} errors</span>
              ) : null}
            </div>
          </div>
        </div>
      </header>

      <div className="min-h-0 flex-1 overflow-hidden p-4">
        <div className="mx-auto flex h-full max-w-[96rem] flex-col gap-4">
          {timelineAuthError ? (
            <AuthErrorBanner message={queryErrorMessage(timeline.rawError)} />
          ) : null}

          {isDesktop ? (
            <TraceContextSection
              buildCopyTraceUrl={buildCopyTraceUrl}
              onToggle={() => setIsTraceContextOpen((open) => !open)}
              open={isTraceContextOpen}
              trace={trace}
            />
          ) : null}

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
      </div>
    </div>
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
      mobileDetails={detailsContent}
      mobileReasoning={reasoningContent}
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
  traceContext,
}: {
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
  traceContext: ReactNode;
}) {
  return (
    <div className="flex h-full min-h-0 flex-col overflow-y-auto bg-slate-50 p-4 dark:bg-slate-950">
      <div className="space-y-4">
        {traceContext}

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

        <div className="min-h-[22rem] overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
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

function TraceContextSection({
  buildCopyTraceUrl,
  onToggle,
  open,
  trace,
}: {
  buildCopyTraceUrl: () => string;
  onToggle: () => void;
  open: boolean;
  trace: TraceDetail;
}) {
  return (
    <section className="overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-950/70">
        <button
          type="button"
          className="flex items-center gap-3 text-left"
          aria-expanded={open}
          onClick={onToggle}
        >
          <span className="text-sm font-semibold uppercase tracking-[0.2em] text-slate-600 dark:text-slate-300">
            Trace Context
          </span>
          <span className="rounded-full bg-white px-2 py-1 text-xs font-medium text-slate-500 ring-1 ring-slate-200 dark:bg-slate-900 dark:text-slate-300 dark:ring-slate-700">
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
                  to={`/sessions/${trace.session_id}`}
                  className="inline-flex flex-col text-left text-blue-600 hover:text-blue-800 dark:text-sky-400 dark:hover:text-sky-300"
                >
                  <span className="text-sm font-medium text-slate-900 dark:text-slate-100">
                    {trace.session_external_id ?? trace.session_id}
                  </span>
                  <span className="font-mono text-xs text-slate-500 dark:text-slate-400">
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
                      className="rounded-full border border-slate-200 bg-white px-3 py-1 font-mono text-xs text-slate-700 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200"
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
    <div className={`rounded-lg border border-slate-200 bg-slate-50 p-4 dark:border-slate-800 dark:bg-slate-950/70 ${className}`.trim()}>
      <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-slate-500 dark:text-slate-400">
        {label}
      </div>
      <div className="mt-2 flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1 text-sm text-slate-900 dark:text-slate-100">{value}</div>
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
    <div className="rounded-lg border border-slate-200 bg-slate-50 p-4 dark:border-slate-800 dark:bg-slate-950/70">
      <h3 className="mb-2 text-sm font-medium text-slate-700 dark:text-slate-200">{title}</h3>
      <JsonViewer data={data} className="max-h-80 overflow-y-auto bg-white dark:bg-slate-900" />
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
            <h2 className="text-base font-semibold">{summary.label}</h2>
            <span className="rounded-full border border-current/15 px-2.5 py-1 text-[11px] font-medium uppercase tracking-[0.14em]">
              {formatRunningStateBasis(assessment.basis)}
            </span>
          </div>
          <p className="mt-2 max-w-3xl text-sm leading-6">{summary.copy}</p>
        </div>

        {assessment.decisiveSpanId ? (
          <button
            type="button"
            className="rounded-full border border-current/20 bg-white/70 px-3 py-1.5 text-xs font-medium transition hover:bg-white dark:bg-slate-950/40 dark:hover:bg-slate-950/70"
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
    <div className="rounded-lg border border-current/15 bg-white/60 p-3 dark:bg-slate-950/30">
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
            className="rounded-full border border-current/20 bg-white/70 px-3 py-1.5 text-xs font-medium transition hover:bg-white dark:bg-slate-950/40 dark:hover:bg-slate-950/70"
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
      return 'border-slate-200 bg-slate-100 text-slate-900 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-100';
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
    return <span className="text-sm text-slate-400 dark:text-slate-500">-</span>;
  }

  return (
    <span
      className={
        monospace
          ? 'font-mono text-xs text-slate-900 dark:text-slate-100'
          : 'text-sm text-slate-900 dark:text-slate-100'
      }
    >
      {value}
    </span>
  );
}
