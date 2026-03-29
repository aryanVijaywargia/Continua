import { describe, expect, it } from 'vitest';
import type { Span, TimelineEvent } from '../api/client';
import {
  createSpan as createSpanFixture,
  createTimelineEvent,
} from '../test/traceFixtures';
import {
  classifyRunningTrace,
  computeOpenWaits,
} from './waitStallAnalysis';

const RUNNING_STARTED_AT = '2026-03-14T10:00:00.000Z';
const STALE_NOW = new Date('2026-03-14T10:20:00.000Z');
const ACTIVE_NOW = new Date('2026-03-14T10:04:00.000Z');

describe('computeOpenWaits', () => {
  it('keeps a single entered wait open', () => {
    const openWaits = computeOpenWaits([
      createWaitEvent({
        id: 'entered',
        timestamp: '2026-03-14T10:00:01.000Z',
        payload: {
          wait_kind: 'model_response',
          phase: 'entered',
          wait_id: 'wait-1',
        },
      }),
    ]);

    expect(openWaits).toHaveLength(1);
    expect(openWaits[0]?.event.id).toBe('entered');
  });

  it('closes matching entered and resolved waits', () => {
    const openWaits = computeOpenWaits([
      createWaitEvent({
        id: 'entered',
        timestamp: '2026-03-14T10:00:01.000Z',
        payload: {
          wait_kind: 'tool_call',
          phase: 'entered',
          wait_id: 'wait-1',
        },
      }),
      createWaitEvent({
        id: 'resolved',
        timestamp: '2026-03-14T10:00:02.000Z',
        payload: {
          wait_kind: 'tool_call',
          phase: 'resolved',
          wait_id: 'wait-1',
        },
      }),
    ]);

    expect(openWaits).toEqual([]);
  });

  it('self-heals when resolved appears before entered in the input order', () => {
    const openWaits = computeOpenWaits([
      createWaitEvent({
        id: 'resolved',
        timestamp: '2026-03-14T10:00:02.000Z',
        payload: {
          wait_kind: 'tool_call',
          phase: 'resolved',
          wait_id: 'wait-1',
        },
      }),
      createWaitEvent({
        id: 'entered',
        timestamp: '2026-03-14T10:00:01.000Z',
        payload: {
          wait_kind: 'tool_call',
          phase: 'entered',
          wait_id: 'wait-1',
        },
      }),
    ]);

    expect(openWaits).toEqual([]);
  });

  it('never lets a wait without wait_id close a different wait', () => {
    const openWaits = computeOpenWaits([
      createWaitEvent({
        id: 'entered',
        timestamp: '2026-03-14T10:00:01.000Z',
        payload: {
          wait_kind: 'model_response',
          phase: 'entered',
          wait_id: 'wait-1',
        },
      }),
      createWaitEvent({
        id: 'resolved-without-id',
        timestamp: '2026-03-14T10:00:02.000Z',
        payload: {
          wait_kind: 'model_response',
          phase: 'resolved',
        },
      }),
    ]);

    expect(openWaits.map((openWait) => openWait.event.id)).toEqual(['entered']);
  });

  it('closes duplicate wait ids in FIFO order', () => {
    const openWaits = computeOpenWaits([
      createWaitEvent({
        id: 'entered-1',
        timestamp: '2026-03-14T10:00:01.000Z',
        payload: {
          wait_kind: 'model_response',
          phase: 'entered',
          wait_id: 'wait-1',
        },
      }),
      createWaitEvent({
        id: 'entered-2',
        timestamp: '2026-03-14T10:00:02.000Z',
        payload: {
          wait_kind: 'model_response',
          phase: 'entered',
          wait_id: 'wait-1',
        },
      }),
      createWaitEvent({
        id: 'resolved-1',
        timestamp: '2026-03-14T10:00:03.000Z',
        payload: {
          wait_kind: 'model_response',
          phase: 'resolved',
          wait_id: 'wait-1',
        },
      }),
    ]);

    expect(openWaits.map((openWait) => openWait.event.id)).toEqual(['entered-2']);
  });

  it('closes both entered waits when two resolves arrive', () => {
    const openWaits = computeOpenWaits([
      createWaitEvent({
        id: 'entered-1',
        timestamp: '2026-03-14T10:00:01.000Z',
        payload: {
          wait_kind: 'tool_call',
          phase: 'entered',
          wait_id: 'wait-1',
        },
      }),
      createWaitEvent({
        id: 'entered-2',
        timestamp: '2026-03-14T10:00:02.000Z',
        payload: {
          wait_kind: 'tool_call',
          phase: 'entered',
          wait_id: 'wait-1',
        },
      }),
      createWaitEvent({
        id: 'resolved-1',
        timestamp: '2026-03-14T10:00:03.000Z',
        payload: {
          wait_kind: 'tool_call',
          phase: 'resolved',
          wait_id: 'wait-1',
        },
      }),
      createWaitEvent({
        id: 'resolved-2',
        timestamp: '2026-03-14T10:00:04.000Z',
        payload: {
          wait_kind: 'tool_call',
          phase: 'resolved',
          wait_id: 'wait-1',
        },
      }),
    ]);

    expect(openWaits).toEqual([]);
  });

  it('sorts internally before pairing', () => {
    const openWaits = computeOpenWaits([
      createWaitEvent({
        id: 'resolved',
        timestamp: '2026-03-14T10:00:02.000Z',
        payload: {
          wait_kind: 'tool_call',
          phase: 'resolved',
          wait_id: 'wait-1',
        },
      }),
      createWaitEvent({
        id: 'entered',
        timestamp: '2026-03-14T10:00:01.000Z',
        payload: {
          wait_kind: 'tool_call',
          phase: 'entered',
          wait_id: 'wait-1',
        },
      }),
    ]);

    expect(openWaits).toEqual([]);
  });

  it('ignores unsupported phases during pairing', () => {
    const openWaits = computeOpenWaits([
      createWaitEvent({
        id: 'entered',
        timestamp: '2026-03-14T10:00:01.000Z',
        payload: {
          wait_kind: 'tool_call',
          phase: 'entered',
          wait_id: 'wait-1',
        },
      }),
      createWaitEvent({
        id: 'unsupported',
        timestamp: '2026-03-14T10:00:02.000Z',
        payload: {
          wait_kind: 'tool_call',
          phase: 'suspended',
          wait_id: 'wait-1',
        },
      }),
    ]);

    expect(openWaits.map((openWait) => openWait.event.id)).toEqual(['entered']);
  });
});

describe('classifyRunningTrace', () => {
  it('classifies an open wait as a declared wait and records decisive evidence', () => {
    const span = createSpan({
      span_id: 'waiting-span',
      name: 'Waiting span',
      status: 'STARTED',
      started_at: '2026-03-14T10:00:01.000Z',
    });
    const assessment = classifyRunningTrace({
      traceStatus: 'RUNNING',
      traceStartedAt: RUNNING_STARTED_AT,
      spans: [span],
      events: [
        createWaitEvent({
          id: 'open-wait',
          span_id: span.span_id,
          span_name: span.name,
          timestamp: '2026-03-14T10:01:00.000Z',
          payload: {
            wait_kind: 'model_response',
            phase: 'entered',
            wait_id: 'wait-1',
          },
        }),
      ],
      now: ACTIVE_NOW,
    });

    expect(assessment).toMatchObject({
      classification: 'declared_wait',
      basis: 'declared',
      reason: 'open_declared_wait',
      decisiveEventId: 'open-wait',
      decisiveSpanId: span.span_id,
      decisiveSpanName: span.name,
    });
  });

  it('waits for model spans when no declared wait is open', () => {
    const earlierOpenModel = createSpan({
      span_id: 'llm-1',
      name: 'First LLM span',
      kind: 'LLM',
      status: 'STARTED',
      started_at: '2026-03-14T10:01:00.000Z',
    });
    const laterOpenModel = createSpan({
      span_id: 'llm-2',
      name: 'Latest LLM span',
      kind: 'LLM',
      status: 'STARTED',
      started_at: '2026-03-14T10:02:00.000Z',
    });

    const assessment = classifyRunningTrace({
      traceStatus: 'RUNNING',
      traceStartedAt: RUNNING_STARTED_AT,
      spans: [earlierOpenModel, laterOpenModel],
      events: [],
      now: ACTIVE_NOW,
    });

    expect(assessment).toMatchObject({
      classification: 'waiting_on_model',
      basis: 'inferred',
      reason: 'open_model_span',
      decisiveSpanId: 'llm-2',
      decisiveSpanName: 'Latest LLM span',
    });
  });

  it('waits for tool spans when no declared wait or model span is open', () => {
    const openTool = createSpan({
      span_id: 'tool-1',
      name: 'Tool span',
      kind: 'TOOL',
      status: 'STARTED',
      started_at: '2026-03-14T10:02:00.000Z',
    });

    const assessment = classifyRunningTrace({
      traceStatus: 'RUNNING',
      traceStartedAt: RUNNING_STARTED_AT,
      spans: [openTool],
      events: [],
      now: ACTIVE_NOW,
    });

    expect(assessment).toMatchObject({
      classification: 'waiting_on_tool',
      basis: 'inferred',
      reason: 'open_tool_span',
      decisiveSpanId: 'tool-1',
      decisiveSpanName: 'Tool span',
    });
  });

  it('treats open generic spans as actively executing and preserves decisive evidence', () => {
    const openGeneric = createSpan({
      span_id: 'agent-1',
      name: 'Agent span',
      kind: 'AGENT',
      status: 'STARTED',
      started_at: '2026-03-14T10:01:30.000Z',
    });

    const assessment = classifyRunningTrace({
      traceStatus: 'RUNNING',
      traceStartedAt: RUNNING_STARTED_AT,
      spans: [openGeneric],
      events: [],
      now: STALE_NOW,
    });

    expect(assessment).toMatchObject({
      classification: 'actively_executing',
      basis: 'heuristic',
      reason: 'open_generic_span',
      decisiveSpanId: 'agent-1',
      decisiveSpanName: 'Agent span',
    });
  });

  it('treats recent execution without open spans as actively executing', () => {
    const completedSpan = createSpan({
      span_id: 'completed-work',
      name: 'Completed work',
      status: 'COMPLETED',
      started_at: '2026-03-14T10:01:00.000Z',
      ended_at: '2026-03-14T10:02:00.000Z',
    });

    const assessment = classifyRunningTrace({
      traceStatus: 'RUNNING',
      traceStartedAt: RUNNING_STARTED_AT,
      spans: [completedSpan],
      events: [],
      now: ACTIVE_NOW,
    });

    expect(assessment).toMatchObject({
      classification: 'actively_executing',
      basis: 'heuristic',
      reason: 'recent_activity_without_open_span',
    });
    expect(assessment?.decisiveSpanId).toBeUndefined();
    expect(assessment?.decisiveSpanName).toBeUndefined();
  });

  it('returns unknown when there is no execution evidence', () => {
    const assessment = classifyRunningTrace({
      traceStatus: 'RUNNING',
      traceStartedAt: RUNNING_STARTED_AT,
      spans: [],
      events: [],
      now: ACTIVE_NOW,
    });

    expect(assessment).toMatchObject({
      classification: 'unknown',
      basis: 'heuristic',
      reason: 'insufficient_running_evidence',
    });
    expect(assessment?.decisiveSpanId).toBeUndefined();
    expect(assessment?.decisiveSpanName).toBeUndefined();
    expect(assessment?.decisiveEventId).toBeUndefined();
  });

  it('returns unknown for scheduled-only traces', () => {
    const scheduledSpan = createSpan({
      span_id: 'scheduled',
      name: 'Scheduled span',
      status: 'SCHEDULED',
      kind: 'LLM',
    });

    const assessment = classifyRunningTrace({
      traceStatus: 'RUNNING',
      traceStartedAt: RUNNING_STARTED_AT,
      spans: [scheduledSpan],
      events: [],
      now: ACTIVE_NOW,
    });

    expect(assessment?.classification).toBe('unknown');
  });

  it('uses the stale heuristic when no stronger signal exists', () => {
    const assessment = classifyRunningTrace({
      traceStatus: 'RUNNING',
      traceStartedAt: RUNNING_STARTED_AT,
      spans: [],
      events: [],
      now: STALE_NOW,
    });

    expect(assessment).toMatchObject({
      classification: 'possibly_stalled',
      basis: 'heuristic',
      reason: 'stale_without_stronger_signal',
    });
    expect(assessment?.decisiveSpanId).toBeUndefined();
    expect(assessment?.decisiveSpanName).toBeUndefined();
    expect(assessment?.decisiveEventId).toBeUndefined();
  });

  it('applies the declared-wait > model > tool precedence', () => {
    const assessment = classifyRunningTrace({
      traceStatus: 'RUNNING',
      traceStartedAt: RUNNING_STARTED_AT,
      spans: [
        createSpan({
          span_id: 'tool-1',
          name: 'Tool span',
          kind: 'TOOL',
          status: 'STARTED',
          started_at: '2026-03-14T10:01:00.000Z',
        }),
        createSpan({
          span_id: 'llm-1',
          name: 'Model span',
          kind: 'LLM',
          status: 'STARTED',
          started_at: '2026-03-14T10:02:00.000Z',
        }),
      ],
      events: [
        createWaitEvent({
          id: 'wait-1',
          timestamp: '2026-03-14T10:03:00.000Z',
          payload: {
            wait_kind: 'model_response',
            phase: 'entered',
            wait_id: 'wait-1',
          },
        }),
      ],
      now: ACTIVE_NOW,
    });

    expect(assessment?.classification).toBe('declared_wait');
  });

  it('prefers model spans over tool spans when both are open', () => {
    const assessment = classifyRunningTrace({
      traceStatus: 'RUNNING',
      traceStartedAt: RUNNING_STARTED_AT,
      spans: [
        createSpan({
          span_id: 'tool-1',
          name: 'Tool span',
          kind: 'TOOL',
          status: 'STARTED',
          started_at: '2026-03-14T10:01:00.000Z',
        }),
        createSpan({
          span_id: 'llm-1',
          name: 'Model span',
          kind: 'LLM',
          status: 'STARTED',
          started_at: '2026-03-14T10:02:00.000Z',
        }),
      ],
      events: [],
      now: ACTIVE_NOW,
    });

    expect(assessment?.classification).toBe('waiting_on_model');
  });

  it('keeps generic open spans above the stale heuristic', () => {
    const assessment = classifyRunningTrace({
      traceStatus: 'RUNNING',
      traceStartedAt: RUNNING_STARTED_AT,
      spans: [
        createSpan({
          span_id: 'generic-1',
          name: 'Generic span',
          kind: 'CUSTOM',
          status: 'STARTED',
          started_at: '2026-03-14T10:01:00.000Z',
        }),
      ],
      events: [],
      now: STALE_NOW,
    });

    expect(assessment?.classification).toBe('actively_executing');
    expect(assessment?.reason).toBe('open_generic_span');
  });

  it('lets the stale heuristic beat unknown', () => {
    const assessment = classifyRunningTrace({
      traceStatus: 'RUNNING',
      traceStartedAt: RUNNING_STARTED_AT,
      spans: [],
      events: [],
      now: STALE_NOW,
    });

    expect(assessment?.classification).toBe('possibly_stalled');
  });

  it('breaks equal started_at ties by the later input span', () => {
    const firstSpan = createSpan({
      span_id: 'tool-1',
      name: 'Earlier input',
      kind: 'TOOL',
      status: 'STARTED',
      started_at: '2026-03-14T10:02:00.000Z',
    });
    const secondSpan = createSpan({
      span_id: 'tool-2',
      name: 'Later input',
      kind: 'TOOL',
      status: 'STARTED',
      started_at: '2026-03-14T10:02:00.000Z',
    });

    const assessment = classifyRunningTrace({
      traceStatus: 'RUNNING',
      traceStartedAt: RUNNING_STARTED_AT,
      spans: [firstSpan, secondSpan],
      events: [],
      now: ACTIVE_NOW,
    });

    expect(assessment).toMatchObject({
      classification: 'waiting_on_tool',
      decisiveSpanId: 'tool-2',
      decisiveSpanName: 'Later input',
    });
  });

  it.each(['COMPLETED', 'FAILED'] as const)(
    'returns null for %s traces',
    (traceStatus) => {
      expect(
        classifyRunningTrace({
          traceStatus,
          traceStartedAt: RUNNING_STARTED_AT,
          spans: [],
          events: [],
          now: ACTIVE_NOW,
        })
      ).toBeNull();
    }
  );
});

function createWaitEvent(overrides: Partial<TimelineEvent> = {}): TimelineEvent {
  return createTimelineEvent({
    event_type: 'wait',
    source: 'explicit',
    ...overrides,
  });
}

function createSpan(overrides: Partial<Span> = {}): Span {
  return createSpanFixture(overrides);
}
