import { FailureSummary } from '../../components/FailureSummary';
import { SpanDetail } from '../../components/SpanDetail';
import { EnginePendingWorkPanel } from '../EnginePendingWorkPanel';
import { RunningStatePanel } from './RunningStatePanel';
import { useTraceDetailWorkspace } from './traceDetailWorkspaceContext';

/**
 * Failure-first summary surface: pending engine work, failure summary,
 * running-state assessment, and the selected span's detail card. Rendered
 * inside the mobile summary tab.
 */
export function TraceDetailsSurface() {
  const {
    events,
    failureAnalysis,
    openWaits,
    pendingWork,
    retrySafetyAnalysis,
    selectSpanAndShowDetails,
    selectedBreadcrumbPath,
    selectedSpan,
    spanIndex,
    timelineStatus,
    trace,
    waitStallAssessment,
  } = useTraceDetailWorkspace();

  return (
    <div className="flex h-full min-h-0 flex-col overflow-y-auto bg-[var(--c-app-bg)]">
      <div className="space-y-3 p-3">
        {trace.engine ? (
          <EnginePendingWorkPanel
            data={pendingWork.data}
            isError={pendingWork.isError}
            isLoading={pendingWork.isLoading}
            errorMessage={pendingWork.errorMessage}
          />
        ) : null}

        {timelineStatus === 'FAILED' ? (
          <FailureSummary
            summary={failureAnalysis.summary}
            onJumpToPrimaryFailedSpan={selectSpanAndShowDetails}
            traceRetrySafety={retrySafetyAnalysis?.traceAssessment ?? null}
          />
        ) : null}

        {waitStallAssessment ? (
          <RunningStatePanel
            assessment={waitStallAssessment}
            events={events}
            openWaits={openWaits}
            spanIndex={spanIndex}
            onSelectSpan={selectSpanAndShowDetails}
          />
        ) : null}

        <div className="min-h-[22rem] overflow-hidden border-t border-[var(--c-border)] bg-[var(--c-app-bg)]">
          <SpanDetail
            span={selectedSpan}
            breadcrumbPath={selectedBreadcrumbPath}
            onSelectSpan={selectSpanAndShowDetails}
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
