import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
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
import { useTraceTimeline } from './useTraceTimeline';
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
  evaluateStaleTraceSignal,
  type StaleTraceSignal,
} from '../utils/failureAnalysis';
import { extractStateChanges } from '../utils/stateChanges';
import { serializeSpanParam } from '../utils/traceDetailSearchParams';
import {
  buildSpanTree,
  collectExpandableSpanIds,
  deriveVisibleRows,
  type SpanTreeNode,
} from '../utils/spanTree';

const EMPTY_SPANS: Span[] = [];
const EMPTY_STALE_TRACE_SIGNAL: StaleTraceSignal = {
  shouldDisplay: false,
  latestActivityAt: null,
  runtimeMs: null,
  inactivityMs: null,
};
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
  const spansQuery = useQuery({
    queryKey: ['spans', traceId],
    queryFn: () => fetchSpans(traceId),
  });
  const timeline = useTraceTimeline(traceId);
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
  const staleTraceSignal = useMemo(
    () =>
      timeline.hasSnapshot
        ? evaluateStaleTraceSignal({
            traceStatus: timelineStatus,
            traceStartedAt: trace?.started_at,
            spans,
            events: timeline.events,
          })
        : EMPTY_STALE_TRACE_SIGNAL,
    [spans, timeline.events, timeline.hasSnapshot, timelineStatus, trace?.started_at]
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
      staleTraceSignal={staleTraceSignal}
      timelineStatus={timelineStatus}
      failureAnalysis={failureAnalysis}
      onSelectSpan={handleSelectSpanAndShowDetails}
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
            selectedSpanId={selectedSpanExternalId}
            setExpandedSpanIds={setExpandedSpanIds}
            spanIndex={spanIndex}
            spanTree={spanTree}
            spans={spans}
            stateChangeCount={stateChanges.length}
            stateContent={stateContent}
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
  selectedSpanId: string | null;
  setExpandedSpanIds: Dispatch<SetStateAction<Set<string>>>;
  spanIndex: ReadonlyMap<string, Span>;
  spanTree: SpanTreeNode[];
  spans: Span[];
  stateChangeCount: number;
  stateContent: ReactNode;
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
  selectedSpanId,
  setExpandedSpanIds,
  spanIndex,
  spanTree,
  spans,
  stateChangeCount,
  stateContent,
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
          traceEndedAt={traceEndedAt}
          traceStartedAt={traceStartedAt}
        />
      }
      inspector={
        <InspectorTabs
          details={detailsContent}
          timeline={timelineContent}
          state={stateContent}
          stateCount={stateChangeCount}
          switchToDetailsRef={inspectorSwitchToDetailsRef}
        />
      }
      mobileDetails={detailsContent}
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
  staleTraceSignal,
  timelineStatus,
  failureAnalysis,
  onSelectSpan,
  selectedBreadcrumbPath,
  selectedSpan,
  spanIndex,
  events,
  traceContext,
}: {
  staleTraceSignal: StaleTraceSignal;
  timelineStatus: 'RUNNING' | 'COMPLETED' | 'FAILED' | null;
  failureAnalysis: ReturnType<typeof buildFailureAnalysis>;
  onSelectSpan: (spanId: string) => void;
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
          />
        ) : null}

        {staleTraceSignal.shouldDisplay ? (
          <StaleTraceSignalPanel staleTraceSignal={staleTraceSignal} />
        ) : null}

        <div className="min-h-[22rem] overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
          <SpanDetail
            span={selectedSpan}
            breadcrumbPath={selectedBreadcrumbPath}
            onSelectSpan={onSelectSpan}
            spanIndex={spanIndex}
            events={events}
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

function StaleTraceSignalPanel({
  staleTraceSignal,
}: {
  staleTraceSignal: StaleTraceSignal;
}) {
  return (
    <section className="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-900 shadow-sm">
      <div className="text-[11px] font-semibold uppercase tracking-[0.18em] text-amber-800">
        Experimental stale trace signal
      </div>
      <p className="mt-2">
        Still marked running. Recent activity is sparse, so this trace may be stale
        or incomplete.
      </p>
      {staleTraceSignal.latestActivityAt ? (
        <p className="mt-2 text-xs text-amber-800">
          Latest activity: {formatTimestamp(staleTraceSignal.latestActivityAt)} (
          {formatRelativeTime(staleTraceSignal.latestActivityAt)})
        </p>
      ) : null}
    </section>
  );
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
