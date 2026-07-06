import {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from 'react';
import { useLocation } from 'react-router-dom';
import type {
  Span,
  TimelineEvent,
  TimelineTraceStatus,
  TraceDetail,
} from '../../api/client';
import type { MobileWorkspaceTabId } from '../../components/WorkspaceShell';
import { useFailedSpanAutoSelect } from '../../hooks/useFailedSpanAutoSelect';
import { useSpanExpansion } from '../../hooks/useSpanExpansion';
import { useTraceDetailSearchParams } from '../../hooks/useTraceDetailSearchParams';
import { downloadJsonFile } from '../../utils/downloadJson';
import {
  buildBreadcrumbPath,
  buildFailureAnalysis,
  buildSpanIndex,
} from '../../utils/failureAnalysis';
import {
  buildReasoningEntries,
  buildTraceCostSeries,
} from '../../utils/reasoning';
import type { RetrySafetyAssessment } from '../../utils/retrySafety';
import {
  buildSpanTree,
  collectExpandableSpanIds,
  getAncestorIds,
} from '../../utils/spanTree';
import { serializeSpanParam } from '../../utils/traceDetailSearchParams';
import { computeOpenWaits } from '../../utils/waitStallAnalysis';
import { useRetrySafetyAnalysis } from '../useRetrySafetyAnalysis';
import { useWaitStallAnalysis } from '../useWaitStallAnalysis';
import {
  TraceDetailWorkspaceContext,
  type PendingWorkState,
  type TraceDetailWorkspaceValue,
} from './traceDetailWorkspaceContext';

const EMPTY_RETRY_SAFETY_ASSESSMENTS = new Map<string, RetrySafetyAssessment>();

interface TraceDetailWorkspaceProviderProps {
  traceId: string;
  trace: TraceDetail;
  spans: Span[];
  events: TimelineEvent[];
  timelineStatus: TimelineTraceStatus | null;
  hasTimelineSnapshot: boolean;
  /** True once the spans query has succeeded for this trace. */
  isSpanDataReady: boolean;
  pendingWork: PendingWorkState;
  projectId: string | undefined;
  returnTo: string;
  children: ReactNode;
}

/**
 * Owns the trace-detail workspace: derived analyses, URL-derived span
 * selection, and the expansion open set. The page fetches data and hands it
 * in; sections consume {@link TraceDetailWorkspaceContext} instead of prop
 * threading.
 */
export function TraceDetailWorkspaceProvider({
  traceId,
  trace,
  spans,
  events,
  timelineStatus,
  hasTimelineSnapshot,
  isSpanDataReady,
  pendingWork,
  projectId,
  returnTo,
  children,
}: TraceDetailWorkspaceProviderProps) {
  const location = useLocation();
  const { spanParam, setSpanParam } = useTraceDetailSearchParams();
  const [activeMobileTab, setActiveMobileTab] =
    useState<MobileWorkspaceTabId>('summary');

  const spanIndex = useMemo(() => buildSpanIndex(spans), [spans]);
  const failureAnalysis = useMemo(
    () => buildFailureAnalysis(spans, events, spanIndex),
    [events, spanIndex, spans]
  );
  const spanTree = useMemo(() => buildSpanTree(spans), [spans]);
  const expandableSpanIds = useMemo(
    () => collectExpandableSpanIds(spanTree),
    [spanTree]
  );
  const reasoningEntries = useMemo(
    () => buildReasoningEntries(events, spans),
    [events, spans]
  );
  const traceCostSeries = useMemo(
    () => buildTraceCostSeries(spans, timelineStatus),
    [spans, timelineStatus]
  );
  const openWaits = useMemo(() => computeOpenWaits(events), [events]);

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
    if (!isSpanDataReady) {
      return;
    }
    if (spanParam !== null && !spanIndex.has(spanParam)) {
      setSpanParam(null);
    }
  }, [isSpanDataReady, spanParam, spanIndex, setSpanParam]);

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

  const selectSpanAndShowDetails = useCallback(
    (spanId: string) => {
      setSpanParam(spanId);
      setActiveMobileTab('summary');
    },
    [setSpanParam]
  );

  useFailedSpanAutoSelect({
    traceId,
    isReady: isSpanDataReady,
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
    traceStartedAt: trace.started_at,
    spans,
    events,
    hasTimelineSnapshot,
  });
  const retrySafetyAnalysisRaw = useRetrySafetyAnalysis(spans, events);
  const showRetrySafety = timelineStatus === 'FAILED';
  const retrySafetyAnalysis = showRetrySafety ? retrySafetyAnalysisRaw : null;
  const visibleRetrySafetyAssessments = showRetrySafety
    ? retrySafetyAnalysisRaw.spanAssessments
    : EMPTY_RETRY_SAFETY_ASSESSMENTS;

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

  const exportTrace = useCallback(() => {
    downloadJsonFile(`continua-trace-${traceId}.json`, {
      exported_at: new Date().toISOString(),
      trace,
      spans,
      timeline: events,
      selected_span_id: selectedSpanId,
    });
  }, [events, selectedSpanId, spans, trace, traceId]);

  const value = useMemo<TraceDetailWorkspaceValue>(
    () => ({
      traceId,
      trace,
      spans,
      events,
      timelineStatus,
      pendingWork,
      projectId,
      returnTo,
      spanIndex,
      spanTree,
      expandableSpanIds,
      failureAnalysis,
      reasoningEntries,
      traceCostSeries,
      openWaits,
      waitStallAssessment,
      retrySafetyAnalysis,
      visibleRetrySafetyAssessments,
      selectedSpanId,
      selectedSpan,
      selectedBreadcrumbPath,
      revealPath,
      expandedSpanIds,
      toggleExpandedSpan,
      expandAll,
      collapseAll,
      setExact,
      selectSpan,
      selectSpanAndShowDetails,
      buildCopyTraceUrl,
      exportTrace,
      activeMobileTab,
      setActiveMobileTab,
    }),
    [
      traceId,
      trace,
      spans,
      events,
      timelineStatus,
      pendingWork,
      projectId,
      returnTo,
      spanIndex,
      spanTree,
      expandableSpanIds,
      failureAnalysis,
      reasoningEntries,
      traceCostSeries,
      openWaits,
      waitStallAssessment,
      retrySafetyAnalysis,
      visibleRetrySafetyAssessments,
      selectedSpanId,
      selectedSpan,
      selectedBreadcrumbPath,
      revealPath,
      expandedSpanIds,
      toggleExpandedSpan,
      expandAll,
      collapseAll,
      setExact,
      selectSpan,
      selectSpanAndShowDetails,
      buildCopyTraceUrl,
      exportTrace,
      activeMobileTab,
    ]
  );

  return (
    <TraceDetailWorkspaceContext.Provider value={value}>
      {children}
    </TraceDetailWorkspaceContext.Provider>
  );
}
