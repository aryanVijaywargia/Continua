import type { Span, TimelineEvent } from '../api/client';
import { getEffectDetails } from './eventSemantics';
import { compareTimelineEvents } from './timeline';

export type RetrySafetyClassification = 'retryable' | 'unsafe' | 'unknown';

export type RetrySafetyReason =
  | 'read_only_effect'
  | 'mutating_non_idempotent'
  | 'mutating_idempotent_with_key'
  | 'no_effect_events'
  | 'malformed_effect_payload'
  | 'mutating_missing_idempotent'
  | 'mutating_idempotent_missing_key';

export interface RetrySafetyAssessment {
  classification: RetrySafetyClassification;
  reason: RetrySafetyReason;
  decisiveEventId?: string;
  decisiveSpanId?: string;
  decisiveSpanName?: string;
  effectKind?: string;
  hasExternalSideEffect?: boolean;
  idempotent?: boolean;
  idempotencyKey?: string;
}

const CLASSIFICATION_PRIORITY: Record<RetrySafetyClassification, number> = {
  retryable: 0,
  unknown: 1,
  unsafe: 2,
};

const CLASSIFICATION_LABELS: Record<RetrySafetyClassification, string> = {
  retryable: 'Retryable',
  unsafe: 'Unsafe',
  unknown: 'Unknown',
};

const REASON_EXPLANATIONS: Record<RetrySafetyReason, string> = {
  read_only_effect:
    'Retry would repeat a read-only effect with no recorded external mutation.',
  mutating_non_idempotent:
    'Recorded effect mutates external state and is explicitly non-idempotent.',
  mutating_idempotent_with_key:
    'Recorded effect mutates external state but is marked idempotent and includes an idempotency key.',
  no_effect_events: 'No effect events were recorded for this failed span.',
  malformed_effect_payload:
    'An effect event was recorded, but its retry-safety fields were malformed or incomplete.',
  mutating_missing_idempotent:
    'An effect may mutate external state, but no idempotency flag was recorded.',
  mutating_idempotent_missing_key:
    'An effect is marked idempotent, but no idempotency key was recorded.',
};

export function classifyEffectEvent(
  event: TimelineEvent
): RetrySafetyAssessment | null {
  if (event.event_type !== 'effect') {
    return null;
  }

  const effectDetails = getEffectDetails(event);
  const eventEvidence = {
    decisiveEventId: event.id,
    decisiveSpanId: event.span_id ?? undefined,
    decisiveSpanName: event.span_name ?? undefined,
  };

  if (!effectDetails) {
    return {
      classification: 'unknown',
      reason: 'malformed_effect_payload',
      ...eventEvidence,
    };
  }

  const effectEvidence = {
    ...eventEvidence,
    effectKind: effectDetails.effectKind,
    hasExternalSideEffect: effectDetails.hasExternalSideEffect,
    idempotent: effectDetails.idempotent,
    idempotencyKey: effectDetails.idempotencyKey,
  };

  if (!effectDetails.hasExternalSideEffect) {
    return {
      classification: 'retryable',
      reason: 'read_only_effect',
      ...effectEvidence,
    };
  }

  if (effectDetails.idempotent === false) {
    return {
      classification: 'unsafe',
      reason: 'mutating_non_idempotent',
      ...effectEvidence,
    };
  }

  if (effectDetails.idempotent === undefined) {
    return {
      classification: 'unknown',
      reason: 'mutating_missing_idempotent',
      ...effectEvidence,
    };
  }

  if (effectDetails.idempotencyKey === undefined) {
    return {
      classification: 'unknown',
      reason: 'mutating_idempotent_missing_key',
      ...effectEvidence,
    };
  }

  return {
    classification: 'retryable',
    reason: 'mutating_idempotent_with_key',
    ...effectEvidence,
  };
}

export function assessSpanRetrySafety(
  span: Span,
  events: TimelineEvent[]
): RetrySafetyAssessment | null {
  if (span.status !== 'FAILED') {
    return null;
  }

  const effectEvents = events
    .filter((event) => event.span_id === span.span_id && event.event_type === 'effect')
    .sort(compareTimelineEvents);

  if (effectEvents.length === 0) {
    return {
      classification: 'unknown',
      reason: 'no_effect_events',
      decisiveSpanId: span.span_id,
      decisiveSpanName: span.name,
    };
  }

  let decisiveAssessment: RetrySafetyAssessment | null = null;

  for (const event of effectEvents) {
    const eventAssessment = classifyEffectEvent(event);
    if (!eventAssessment) {
      continue;
    }

    decisiveAssessment = chooseDecisiveAssessment(
      decisiveAssessment,
      eventAssessment
    );
  }

  if (!decisiveAssessment) {
    return {
      classification: 'unknown',
      reason: 'no_effect_events',
      decisiveSpanId: span.span_id,
      decisiveSpanName: span.name,
    };
  }

  return {
    ...decisiveAssessment,
    decisiveSpanId: span.span_id,
    decisiveSpanName: span.name,
  };
}

export function assessTraceRetrySafety(
  spanAssessments: Iterable<RetrySafetyAssessment>
): RetrySafetyAssessment {
  let decisiveAssessment: RetrySafetyAssessment | null = null;

  for (const assessment of spanAssessments) {
    decisiveAssessment = chooseDecisiveAssessment(
      decisiveAssessment,
      assessment
    );
  }

  return (
    decisiveAssessment ?? {
      classification: 'unknown',
      reason: 'no_effect_events',
    }
  );
}

export function getReasonExplanation(reason: RetrySafetyReason): string {
  return REASON_EXPLANATIONS[reason];
}

export function getAccessibleSummary(
  classification: RetrySafetyClassification
): string {
  return `Retry safety advisory: ${classification}. Inferred from effect metadata.`;
}

export function getRetrySafetyLabel(
  classification: RetrySafetyClassification
): string {
  return CLASSIFICATION_LABELS[classification];
}

function chooseDecisiveAssessment(
  current: RetrySafetyAssessment | null,
  candidate: RetrySafetyAssessment
): RetrySafetyAssessment {
  if (!current) {
    return candidate;
  }

  if (
    CLASSIFICATION_PRIORITY[candidate.classification] >=
    CLASSIFICATION_PRIORITY[current.classification]
  ) {
    return candidate;
  }

  return current;
}
