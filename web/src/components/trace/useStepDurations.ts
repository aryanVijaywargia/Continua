import { useMemo } from 'react';
import type { Span } from '../../api/client';
import { getStepDuration, type StepDuration } from './traceSteps';

export interface StepDurations {
  byId: ReadonlyMap<string, StepDuration>;
  /** True when at least one duration is computed from timestamps. */
  anyDerived: boolean;
  /** True when every known duration is computed from timestamps. */
  allDerived: boolean;
}

/**
 * Durations for every step, keyed by `span_id`. Running steps are measured up
 * to the time the span list last changed, so live polling moves them forward.
 */
export function useStepDurations(spans: Span[]): StepDurations {
  return useMemo(() => {
    const nowMs = Date.now();
    const byId = new Map<string, StepDuration>();
    let derivedCount = 0;
    let knownCount = 0;
    for (const span of spans) {
      const duration = getStepDuration(span, nowMs);
      byId.set(span.span_id, duration);
      if (duration.ms !== null) {
        knownCount += 1;
        if (duration.derived) {
          derivedCount += 1;
        }
      }
    }
    return {
      byId,
      anyDerived: derivedCount > 0,
      allDerived: knownCount > 0 && derivedCount === knownCount,
    };
  }, [spans]);
}
