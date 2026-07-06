import type { Span } from '../api/client';

export type ReplayStepStatus = 'replayed' | 'current' | 'pending';

export interface ReplayStep {
  id: string;
  name: string;
  status: ReplayStepStatus;
  mock: boolean;
  spanId?: string;
}

/**
 * Preview-only replay plan: spans before the first failure count as replayed,
 * the failed span is current, and everything after is pending. With
 * `mockFailures` the failed span onward is marked as mocked.
 */
export function buildReplaySteps(
  orderedSpans: Span[],
  failedSpan: Span | undefined,
  mockFailures: boolean
): ReplayStep[] {
  if (orderedSpans.length === 0) return [];
  const failedIndex = failedSpan
    ? orderedSpans.findIndex((span) => span.id === failedSpan.id)
    : -1;
  return orderedSpans.map((span, index) => {
    const isMock = mockFailures && failedIndex >= 0 && index >= failedIndex;
    const status: ReplayStepStatus =
      index < failedIndex || (failedIndex < 0 && span.status === 'COMPLETED')
        ? 'replayed'
        : index === failedIndex
          ? 'current'
          : 'pending';
    return {
      id: span.id,
      name: span.name,
      status,
      mock: isMock,
      spanId: span.span_id,
    };
  });
}
