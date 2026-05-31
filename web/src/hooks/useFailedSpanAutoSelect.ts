import { useEffect, useRef } from 'react';
import type { TimelineTraceStatus } from '../api/client';

interface UseFailedSpanAutoSelectOptions {
  traceId: string;
  isReady: boolean;
  traceStatus: TimelineTraceStatus | null;
  /** Raw `?span=` value, which may reference a span that does not exist. */
  spanParam: string | null;
  /** Validated selection: non-null only when the param resolves to a real span. */
  selectedSpanId: string | null;
  primaryFailedSpanId: string | null;
  setSpanParam: (spanId: string | null) => void;
}

/**
 * One-shot latch: on a FAILED trace with no span selected, open the primary
 * failed span once per trace load by writing the URL. The latch is keyed by
 * traceId, so a fresh trace re-fires but closing the panel within a load does
 * not. It records only *whether the system has acted* — not which span is
 * selected — so it is not a second source of truth for selection.
 * See CONTEXT.md ("Failed-span auto-select").
 */
export function useFailedSpanAutoSelect({
  traceId,
  isReady,
  traceStatus,
  spanParam,
  selectedSpanId,
  primaryFailedSpanId,
  setSpanParam,
}: UseFailedSpanAutoSelectOptions) {
  const autoSelectedForTrace = useRef<string | null>(null);

  useEffect(() => {
    if (autoSelectedForTrace.current === traceId) {
      return;
    }
    // Only decide once span data is loaded and the trace is known-failed.
    if (!isReady || traceStatus !== 'FAILED') {
      return;
    }

    // A valid explicit selection was present on load: the operator/sharer
    // already chose a span, so auto-open must not fire for this load. Mark the
    // trace handled so a later clear does not trigger auto-open.
    if (selectedSpanId !== null) {
      autoSelectedForTrace.current = traceId;
      return;
    }

    // The URL has a span param that does not resolve to a real span. It is being
    // cleared elsewhere this commit; wait for the cleared state rather than
    // latching on a phantom or racing the clear.
    if (spanParam !== null) {
      return;
    }

    // Fresh failed trace with no explicit span: open the primary failed span
    // once. If there is no identifiable failed span yet, stay unlatched and wait.
    if (primaryFailedSpanId) {
      autoSelectedForTrace.current = traceId;
      setSpanParam(primaryFailedSpanId);
    }
  }, [
    traceId,
    isReady,
    traceStatus,
    spanParam,
    selectedSpanId,
    primaryFailedSpanId,
    setSpanParam,
  ]);
}
