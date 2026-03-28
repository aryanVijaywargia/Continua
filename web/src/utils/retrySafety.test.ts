import { beforeEach, describe, expect, it } from 'vitest';
import {
  createSpan,
  createTimelineEvent,
  resetTestEntityCounter,
} from '../test/traceFixtures';
import {
  assessSpanRetrySafety,
  assessTraceRetrySafety,
  classifyEffectEvent,
  getAccessibleSummary,
  getReasonExplanation,
  type RetrySafetyAssessment,
  type RetrySafetyReason,
} from './retrySafety';

beforeEach(() => {
  resetTestEntityCounter();
});

describe('classifyEffectEvent', () => {
  it('returns null for non-effect events', () => {
    expect(
      classifyEffectEvent(
        createTimelineEvent({
          event_type: 'message',
          message: 'non-effect',
        })
      )
    ).toBeNull();
  });

  it.each([
    {
      name: 'read_only_effect',
      payload: {
        effect_kind: 'model_call',
        has_external_side_effect: false,
      },
      expected: {
        classification: 'retryable',
        reason: 'read_only_effect',
      },
    },
    {
      name: 'mutating_non_idempotent',
      payload: {
        effect_kind: 'api_call',
        has_external_side_effect: true,
        idempotent: false,
      },
      expected: {
        classification: 'unsafe',
        reason: 'mutating_non_idempotent',
      },
    },
    {
      name: 'mutating_idempotent_with_key',
      payload: {
        effect_kind: 'api_call',
        has_external_side_effect: true,
        idempotent: true,
        idempotency_key: 'key-1',
      },
      expected: {
        classification: 'retryable',
        reason: 'mutating_idempotent_with_key',
      },
    },
    {
      name: 'mutating_missing_idempotent',
      payload: {
        effect_kind: 'tool_call',
        has_external_side_effect: true,
      },
      expected: {
        classification: 'unknown',
        reason: 'mutating_missing_idempotent',
      },
    },
    {
      name: 'mutating_idempotent_missing_key',
      payload: {
        effect_kind: 'tool_call',
        has_external_side_effect: true,
        idempotent: true,
      },
      expected: {
        classification: 'unknown',
        reason: 'mutating_idempotent_missing_key',
      },
    },
  ])('classifies $name', ({ payload, expected }) => {
    expect(
      classifyEffectEvent(
        createTimelineEvent({
          id: `event-${expected.reason}`,
          span_id: 'failed-span',
          span_name: 'Failed span',
          event_type: 'effect',
          payload,
        })
      )
    ).toMatchObject({
      ...expected,
      decisiveEventId: `event-${expected.reason}`,
      decisiveSpanId: 'failed-span',
      decisiveSpanName: 'Failed span',
    });
  });

  it('classifies malformed effect payloads as unknown', () => {
    expect(
      classifyEffectEvent(
        createTimelineEvent({
          id: 'bad-effect',
          event_type: 'effect',
          payload: {
            effect_kind: 'api_call',
          },
        })
      )
    ).toEqual({
      classification: 'unknown',
      reason: 'malformed_effect_payload',
      decisiveEventId: 'bad-effect',
      decisiveSpanId: undefined,
      decisiveSpanName: undefined,
    });
  });
});

describe('assessSpanRetrySafety', () => {
  it('returns retryable for a failed span with a single retryable effect', () => {
    const failedSpan = createSpan({
      span_id: 'failed-span',
      name: 'Failed span',
      status: 'FAILED',
    });

    const assessment = assessSpanRetrySafety(failedSpan, [
      createTimelineEvent({
        id: 'retryable-effect',
        span_id: failedSpan.span_id,
        event_type: 'effect',
        payload: {
          effect_kind: 'model_call',
          has_external_side_effect: false,
        },
      }),
    ]);

    expect(assessment).toMatchObject({
      classification: 'retryable',
      reason: 'read_only_effect',
      decisiveSpanId: failedSpan.span_id,
      decisiveSpanName: failedSpan.name,
      decisiveEventId: 'retryable-effect',
    });
  });

  it('uses unsafe over retryable when a failed span has mixed effects', () => {
    const failedSpan = createSpan({
      span_id: 'failed-span',
      name: 'Failed span',
      status: 'FAILED',
    });

    const assessment = assessSpanRetrySafety(failedSpan, [
      createTimelineEvent({
        id: 'retryable-effect',
        span_id: failedSpan.span_id,
        timestamp: '2026-03-14T10:00:00.000Z',
        event_type: 'effect',
        payload: {
          effect_kind: 'model_call',
          has_external_side_effect: false,
        },
      }),
      createTimelineEvent({
        id: 'unsafe-effect',
        span_id: failedSpan.span_id,
        timestamp: '2026-03-14T10:00:01.000Z',
        event_type: 'effect',
        payload: {
          effect_kind: 'api_call',
          has_external_side_effect: true,
          idempotent: false,
        },
      }),
    ]);

    expect(assessment?.classification).toBe('unsafe');
    expect(assessment?.reason).toBe('mutating_non_idempotent');
    expect(assessment?.decisiveEventId).toBe('unsafe-effect');
  });

  it('uses unknown over retryable when an effect is missing idempotency metadata', () => {
    const failedSpan = createSpan({
      span_id: 'failed-span',
      name: 'Failed span',
      status: 'FAILED',
    });

    const assessment = assessSpanRetrySafety(failedSpan, [
      createTimelineEvent({
        id: 'retryable-effect',
        span_id: failedSpan.span_id,
        timestamp: '2026-03-14T10:00:00.000Z',
        event_type: 'effect',
        payload: {
          effect_kind: 'model_call',
          has_external_side_effect: false,
        },
      }),
      createTimelineEvent({
        id: 'unknown-effect',
        span_id: failedSpan.span_id,
        timestamp: '2026-03-14T10:00:01.000Z',
        event_type: 'effect',
        payload: {
          effect_kind: 'tool_call',
          has_external_side_effect: true,
        },
      }),
    ]);

    expect(assessment?.classification).toBe('unknown');
    expect(assessment?.reason).toBe('mutating_missing_idempotent');
    expect(assessment?.decisiveEventId).toBe('unknown-effect');
  });

  it('returns unknown with no_effect_events when a failed span has no effect events', () => {
    const failedSpan = createSpan({
      span_id: 'failed-span',
      name: 'Failed span',
      status: 'FAILED',
    });

    expect(
      assessSpanRetrySafety(failedSpan, [
        createTimelineEvent({
          span_id: failedSpan.span_id,
          event_type: 'message',
          message: 'no effect',
        }),
      ])
    ).toEqual({
      classification: 'unknown',
      reason: 'no_effect_events',
      decisiveSpanId: failedSpan.span_id,
      decisiveSpanName: failedSpan.name,
    });
  });

  it('returns null for non-failed spans', () => {
    expect(
      assessSpanRetrySafety(
        createSpan({
          span_id: 'completed-span',
          status: 'COMPLETED',
        }),
        [
          createTimelineEvent({
            span_id: 'completed-span',
            event_type: 'effect',
            payload: {
              effect_kind: 'api_call',
              has_external_side_effect: true,
              idempotent: false,
            },
          }),
        ]
      )
    ).toBeNull();
  });

  it('selects the latest event within the winning class as decisive evidence', () => {
    const failedSpan = createSpan({
      span_id: 'failed-span',
      name: 'Failed span',
      status: 'FAILED',
    });

    const assessment = assessSpanRetrySafety(failedSpan, [
      createTimelineEvent({
        id: 'unsafe-effect-earlier',
        span_id: failedSpan.span_id,
        timestamp: '2026-03-14T10:00:00.000Z',
        event_type: 'effect',
        payload: {
          effect_kind: 'api_call',
          has_external_side_effect: true,
          idempotent: false,
        },
      }),
      createTimelineEvent({
        id: 'unsafe-effect-later',
        span_id: failedSpan.span_id,
        timestamp: '2026-03-14T10:00:01.000Z',
        event_type: 'effect',
        payload: {
          effect_kind: 'tool_call',
          has_external_side_effect: true,
          idempotent: false,
        },
      }),
    ]);

    expect(assessment?.decisiveEventId).toBe('unsafe-effect-later');
    expect(assessment?.effectKind).toBe('tool_call');
  });
});

describe('assessTraceRetrySafety', () => {
  it('uses a single failed span assessment as the trace assessment', () => {
    expect(
      assessTraceRetrySafety([
        createAssessment({
          classification: 'retryable',
          reason: 'read_only_effect',
          decisiveSpanId: 'failed-span',
          decisiveSpanName: 'Failed span',
        }),
      ])
    ).toMatchObject({
      classification: 'retryable',
      reason: 'read_only_effect',
      decisiveSpanId: 'failed-span',
    });
  });

  it('uses precedence across multiple failed spans', () => {
    expect(
      assessTraceRetrySafety([
        createAssessment({
          classification: 'retryable',
          reason: 'read_only_effect',
          decisiveSpanId: 'primary-failed',
          decisiveSpanName: 'Primary failed',
        }),
        createAssessment({
          classification: 'unsafe',
          reason: 'mutating_non_idempotent',
          decisiveSpanId: 'secondary-failed',
          decisiveSpanName: 'Secondary failed',
        }),
      ])
    ).toMatchObject({
      classification: 'unsafe',
      decisiveSpanId: 'secondary-failed',
      decisiveSpanName: 'Secondary failed',
    });
  });

  it('can point to a decisive span that differs from the primary failed span', () => {
    const traceAssessment = assessTraceRetrySafety([
      createAssessment({
        classification: 'retryable',
        reason: 'read_only_effect',
        decisiveSpanId: 'primary-failed',
        decisiveSpanName: 'Primary failed',
      }),
      createAssessment({
        classification: 'unsafe',
        reason: 'mutating_non_idempotent',
        decisiveSpanId: 'secondary-failed',
        decisiveSpanName: 'Secondary failed',
      }),
    ]);

    expect(traceAssessment.decisiveSpanId).toBe('secondary-failed');
    expect(traceAssessment.decisiveSpanName).toBe('Secondary failed');
  });

  it('returns unknown when there are no assessable failed spans', () => {
    expect(assessTraceRetrySafety([])).toEqual({
      classification: 'unknown',
      reason: 'no_effect_events',
    });
  });
});

describe('retry safety explanations', () => {
  it.each([
    [
      'read_only_effect',
      'Retry would repeat a read-only effect with no recorded external mutation.',
    ],
    [
      'mutating_non_idempotent',
      'Recorded effect mutates external state and is explicitly non-idempotent.',
    ],
    [
      'mutating_idempotent_with_key',
      'Recorded effect mutates external state but is marked idempotent and includes an idempotency key.',
    ],
    [
      'no_effect_events',
      'No effect events were recorded for this failed span.',
    ],
    [
      'malformed_effect_payload',
      'An effect event was recorded, but its retry-safety fields were malformed or incomplete.',
    ],
    [
      'mutating_missing_idempotent',
      'An effect may mutate external state, but no idempotency flag was recorded.',
    ],
    [
      'mutating_idempotent_missing_key',
      'An effect is marked idempotent, but no idempotency key was recorded.',
    ],
  ] satisfies Array<[RetrySafetyReason, string]>)(
    'maps %s to the fixed explanation text',
    (reason, explanation) => {
      expect(getReasonExplanation(reason)).toBe(explanation);
    }
  );

  it('builds the accessible summary from the classification', () => {
    expect(getAccessibleSummary('unsafe')).toBe(
      'Retry safety advisory: unsafe. Inferred from effect metadata.'
    );
  });
});

function createAssessment(
  overrides: Partial<RetrySafetyAssessment> = {}
): RetrySafetyAssessment {
  return {
    classification: overrides.classification ?? 'retryable',
    reason: overrides.reason ?? 'read_only_effect',
    decisiveSpanId: overrides.decisiveSpanId ?? 'failed-span',
    decisiveSpanName: overrides.decisiveSpanName ?? 'Failed span',
    decisiveEventId: overrides.decisiveEventId,
    effectKind: overrides.effectKind,
    hasExternalSideEffect: overrides.hasExternalSideEffect,
    idempotent: overrides.idempotent,
    idempotencyKey: overrides.idempotencyKey,
  };
}
