import { FailureSummary } from '../../components/FailureSummary';
import { EnginePendingWorkPanel } from '../EnginePendingWorkPanel';
import { RunningStatePanel } from './RunningStatePanel';
import { useTraceDetailWorkspace } from './traceDetailWorkspaceContext';

/**
 * Failure-first trace state: pending engine work, the failure summary, and
 * the running-state assessment. Renders nothing when none of them applies.
 */
export function TraceStatePanels() {
  const {
    events,
    failureAnalysis,
    openWaits,
    pendingWork,
    retrySafetyAnalysis,
    selectSpanAndShowDetails,
    spanIndex,
    timelineStatus,
    trace,
    waitStallAssessment,
  } = useTraceDetailWorkspace();

  const showFailure = timelineStatus === 'FAILED';
  if (!trace.engine && !showFailure && !waitStallAssessment) {
    return null;
  }

  return (
    <div className="space-y-3">
      {trace.engine ? (
        <EnginePendingWorkPanel
          data={pendingWork.data}
          isError={pendingWork.isError}
          isLoading={pendingWork.isLoading}
          errorMessage={pendingWork.errorMessage}
        />
      ) : null}

      {showFailure ? (
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
    </div>
  );
}
