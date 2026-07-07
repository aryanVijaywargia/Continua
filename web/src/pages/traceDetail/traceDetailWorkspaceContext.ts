import { createContext, useContext } from 'react';
import type {
  EnginePendingWorkResponse,
  Span,
  TimelineEvent,
  TimelineTraceStatus,
  TraceDetail,
} from '../../api/client';
import type { MobileWorkspaceTabId } from '../../components/WorkspaceShell';
import type {
  BreadcrumbSegment,
  FailureAnalysis,
} from '../../utils/failureAnalysis';
import type { DecisionTraceEntry, TraceCostSeries } from '../../utils/reasoning';
import type { RetrySafetyAssessment } from '../../utils/retrySafety';
import type { SpanTreeNode } from '../../utils/spanTree';
import type { OpenWait, WaitStallAssessment } from '../../utils/waitStallAnalysis';
import type { RetrySafetyAnalysis } from '../useRetrySafetyAnalysis';

/** Pending-work query state, fetched once by the page and shared here. */
export interface PendingWorkState {
  data: EnginePendingWorkResponse | undefined;
  isLoading: boolean;
  isError: boolean;
  errorMessage: string;
}

/**
 * The one provider seam for the trace-detail workspace: loaded trace data,
 * the analyses derived from it, URL-derived span selection, the
 * expansion-reducer-owned open set, and the workspace actions. Sections read
 * from this context instead of having a dozen props threaded through
 * intermediate components. The URL stays the single source of truth for
 * selection — `selectedSpanId`/`selectedSpan` are derived values, never a
 * second copy of state (see CONTEXT.md "Selected span").
 */
export interface TraceDetailWorkspaceValue {
  // Loaded data (the page owns fetching; the provider owns everything derived).
  traceId: string;
  trace: TraceDetail;
  spans: Span[];
  events: TimelineEvent[];
  timelineStatus: TimelineTraceStatus | null;
  pendingWork: PendingWorkState;

  // Navigation context shared across the page's sections.
  projectId: string | undefined;
  returnTo: string;

  // Derived indices and analyses.
  spanIndex: ReadonlyMap<string, Span>;
  spanTree: SpanTreeNode[];
  expandableSpanIds: ReadonlySet<string>;
  failureAnalysis: FailureAnalysis;
  reasoningEntries: DecisionTraceEntry[];
  traceCostSeries: TraceCostSeries | null;
  openWaits: OpenWait[];
  waitStallAssessment: WaitStallAssessment | null;
  /** Non-null only while the trace is FAILED (retry safety is failure-scoped). */
  retrySafetyAnalysis: RetrySafetyAnalysis | null;
  visibleRetrySafetyAssessments: ReadonlyMap<string, RetrySafetyAssessment>;

  // Selection, derived from the `span` URL param.
  selectedSpanId: string | null;
  selectedSpan: Span | null;
  selectedBreadcrumbPath: BreadcrumbSegment[];
  /** Pure derivation: ancestors(selected) ∪ {selected}. See CONTEXT.md "Reveal". */
  revealPath: ReadonlySet<string>;

  // Expansion, owned by the pure expansionReducer via useSpanExpansion.
  expandedSpanIds: ReadonlySet<string>;
  toggleExpandedSpan: (spanId: string) => void;
  expandAll: () => void;
  collapseAll: () => void;
  setExact: (expanded: ReadonlySet<string>) => void;

  // Actions.
  selectSpan: (spanId: string) => void;
  /** Selects and, on mobile, switches the workspace to the summary tab. */
  selectSpanAndShowDetails: (spanId: string) => void;
  buildCopyTraceUrl: () => string;
  exportTrace: () => void;

  // Mobile workspace tab.
  activeMobileTab: MobileWorkspaceTabId;
  setActiveMobileTab: (tab: MobileWorkspaceTabId) => void;
}

export const TraceDetailWorkspaceContext =
  createContext<TraceDetailWorkspaceValue | null>(null);

export function useTraceDetailWorkspace(): TraceDetailWorkspaceValue {
  const context = useContext(TraceDetailWorkspaceContext);
  if (!context) {
    throw new Error(
      'useTraceDetailWorkspace must be used within a TraceDetailWorkspaceProvider'
    );
  }

  return context;
}
