import { describe, expect, it } from 'vitest';
import { createSpan, createTimelineEvent } from '../test/traceFixtures';
import {
  buildReasoningEntries,
  buildTraceCostSeries,
} from './reasoning';

describe('buildReasoningEntries', () => {
  it('filters malformed decisions and falls back to the span lookup when the event name is absent', () => {
    const span = createSpan({
      span_id: 'reasoning-span',
      name: 'Fallback span name',
    });
    const validDecision = createTimelineEvent({
      id: 'valid-decision',
      span_id: span.span_id,
      event_type: 'decision',
      payload: {
        question: 'Pick a model?',
        chosen: 'gpt-5.4',
      },
    });
    const malformedDecision = createTimelineEvent({
      id: 'malformed-decision',
      span_id: span.span_id,
      event_type: 'decision',
      payload: {
        question: 'Missing chosen field',
      },
    });
    const messageEvent = createTimelineEvent({
      id: 'plain-message',
      span_id: span.span_id,
      event_type: 'message',
      message: 'not a decision',
    });

    expect(
      buildReasoningEntries([malformedDecision, validDecision, messageEvent], [span])
    ).toEqual([
      {
        event: validDecision,
        spanId: span.span_id,
        spanName: span.name,
        question: 'Pick a model?',
        chosen: 'gpt-5.4',
        alternatives: undefined,
        reasoning: undefined,
      },
    ]);
  });

  it('orders cross-span decision entries with the shared compareTimelineEvents comparator', () => {
    const alphaSpan = createSpan({
      span_id: 'alpha',
      name: 'Alpha span',
    });
    const betaSpan = createSpan({
      span_id: 'beta',
      name: 'Beta span',
    });
    const firstDecision = createTimelineEvent({
      id: 'decision-explicit-earlier-sequence',
      span_id: betaSpan.span_id,
      span_name: 'Beta event name',
      event_type: 'decision',
      timestamp: '2026-03-14T10:00:02.000Z',
      source: 'explicit',
      sequence: 1,
      payload: {
        question: 'First explicit decision?',
        chosen: 'beta',
      },
    });
    const secondDecision = createTimelineEvent({
      id: 'decision-explicit-later-sequence',
      span_id: alphaSpan.span_id,
      span_name: 'Alpha event name',
      event_type: 'decision',
      timestamp: '2026-03-14T10:00:02.000Z',
      source: 'explicit',
      sequence: 2,
      payload: {
        question: 'Second explicit decision?',
        chosen: 'alpha',
      },
    });
    const syntheticDecision = createTimelineEvent({
      id: 'decision-synthetic',
      span_id: alphaSpan.span_id,
      span_name: 'Synthetic span name',
      event_type: 'decision',
      timestamp: '2026-03-14T10:00:02.000Z',
      source: 'synthetic',
      payload: {
        question: 'Synthetic decision?',
        chosen: 'synthetic',
      },
    });
    const earlierDecision = createTimelineEvent({
      id: 'decision-earliest',
      span_id: alphaSpan.span_id,
      span_name: 'Alpha earliest',
      event_type: 'decision',
      timestamp: '2026-03-14T10:00:01.000Z',
      payload: {
        question: 'Earliest decision?',
        chosen: 'first',
      },
    });

    const entries = buildReasoningEntries(
      [syntheticDecision, secondDecision, earlierDecision, firstDecision],
      [alphaSpan, betaSpan]
    );

    expect(entries.map((entry) => entry.event.id)).toEqual([
      earlierDecision.id,
      firstDecision.id,
      secondDecision.id,
      syntheticDecision.id,
    ]);
  });
});

describe('buildTraceCostSeries', () => {
  it('builds a completed-trace series from terminal cost-bearing spans and ignores non-costed spans', () => {
    const completedSpan = createSpan({
      span_id: 'completed-cost',
      status: 'COMPLETED',
      started_at: '2026-03-14T10:00:00.000Z',
      ended_at: '2026-03-14T10:00:03.000Z',
      cost_usd: 0.02,
    });
    const failedSpan = createSpan({
      span_id: 'failed-cost',
      status: 'FAILED',
      started_at: '2026-03-14T10:00:01.000Z',
      ended_at: '2026-03-14T10:00:04.000Z',
      cost_usd: 0.03,
    });
    const zeroCostSpan = createSpan({
      span_id: 'zero-cost',
      status: 'COMPLETED',
      started_at: '2026-03-14T10:00:01.500Z',
      ended_at: '2026-03-14T10:00:02.000Z',
      cost_usd: 0,
    });
    const runningSpan = createSpan({
      span_id: 'running-cost',
      status: 'STARTED',
      started_at: '2026-03-14T10:00:02.000Z',
      cost_usd: 0.99,
    });

    expect(
      buildTraceCostSeries(
        [completedSpan, failedSpan, zeroCostSpan, runningSpan],
        'COMPLETED'
      )
    ).toEqual({
      partial: false,
      points: [
        {
          anchorMs: Date.parse('2026-03-14T10:00:03.000Z'),
          cumulativeCostUsd: 0.02,
          incrementalCostUsd: 0.02,
        },
        {
          anchorMs: Date.parse('2026-03-14T10:00:04.000Z'),
          cumulativeCostUsd: 0.05,
          incrementalCostUsd: 0.03,
        },
      ],
      totalCostUsd: 0.05,
    });
  });

  it('marks running traces as partial and aggregates tied terminal timestamps into a single step', () => {
    const firstCompletedSpan = createSpan({
      span_id: 'running-completed-a',
      status: 'COMPLETED',
      started_at: '2026-03-14T10:00:00.000Z',
      ended_at: '2026-03-14T10:00:05.000Z',
      cost_usd: 0.02,
    });
    const secondCompletedSpan = createSpan({
      span_id: 'running-completed-b',
      status: 'FAILED',
      started_at: '2026-03-14T10:00:02.000Z',
      ended_at: '2026-03-14T10:00:05.000Z',
      cost_usd: 0.03,
    });
    const stillRunningSpan = createSpan({
      span_id: 'running-live',
      status: 'STARTED',
      started_at: '2026-03-14T10:00:03.000Z',
      cost_usd: 0.5,
    });

    expect(
      buildTraceCostSeries(
        [firstCompletedSpan, secondCompletedSpan, stillRunningSpan],
        'RUNNING'
      )
    ).toEqual({
      partial: true,
      points: [
        {
          anchorMs: Date.parse('2026-03-14T10:00:05.000Z'),
          cumulativeCostUsd: 0.05,
          incrementalCostUsd: 0.05,
        },
      ],
      totalCostUsd: 0.05,
    });
  });

  it('uses started_at when a terminal span is missing ended_at', () => {
    const terminalWithoutEnd = createSpan({
      span_id: 'fallback-anchor',
      status: 'COMPLETED',
      started_at: '2026-03-14T10:00:07.000Z',
      ended_at: undefined,
      cost_usd: 0.04,
    });

    expect(buildTraceCostSeries([terminalWithoutEnd], 'COMPLETED')).toEqual({
      partial: false,
      points: [
        {
          anchorMs: Date.parse('2026-03-14T10:00:07.000Z'),
          cumulativeCostUsd: 0.04,
          incrementalCostUsd: 0.04,
        },
      ],
      totalCostUsd: 0.04,
    });
  });

  it('returns null for traces without any terminal non-zero cost data', () => {
    const zeroCostSpan = createSpan({
      span_id: 'zero-cost',
      status: 'COMPLETED',
      cost_usd: 0,
    });
    const undefinedCostSpan = createSpan({
      span_id: 'undefined-cost',
      status: 'FAILED',
      cost_usd: undefined,
    });
    const runningCostSpan = createSpan({
      span_id: 'ignored-running-cost',
      status: 'STARTED',
      cost_usd: 0.3,
    });

    expect(
      buildTraceCostSeries(
        [zeroCostSpan, undefinedCostSpan, runningCostSpan],
        'RUNNING'
      )
    ).toBeNull();
  });
});
