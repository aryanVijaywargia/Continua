import { describe, expect, it } from 'vitest';
import type { Span, TimelineEvent } from '../api/client';
import {
  buildBreadcrumbPath,
  buildFailureAnalysis,
  buildFailureSummary,
  buildSpanIndex,
  evaluateStaleTraceSignal,
  findPrimaryFailedSpan,
  getInlineErrorPreview,
  STALE_INACTIVITY_THRESHOLD_MS,
  STALE_RUNTIME_THRESHOLD_MS,
} from './failureAnalysis';

function createSpan(overrides: Partial<Span> = {}): Span {
  const spanId = overrides.span_id ?? `span-${Math.random().toString(36).slice(2)}`;

  return {
    id: overrides.id ?? `uuid-${spanId}`,
    trace_id: overrides.trace_id ?? 'trace-1',
    span_id: spanId,
    name: overrides.name ?? spanId,
    kind: overrides.kind ?? 'CHAIN',
    status: overrides.status ?? 'COMPLETED',
    started_at: overrides.started_at ?? '2026-03-14T10:00:00.000Z',
    ended_at: overrides.ended_at,
    tokens_in: overrides.tokens_in,
    tokens_out: overrides.tokens_out,
    cost_usd: overrides.cost_usd,
    latency_ms: overrides.latency_ms,
    error_message: overrides.error_message,
    model: overrides.model,
    provider: overrides.provider,
    input: overrides.input,
    input_truncated: overrides.input_truncated,
    input_original_size_bytes: overrides.input_original_size_bytes,
    input_truncation_reason: overrides.input_truncation_reason,
    output: overrides.output,
    output_truncated: overrides.output_truncated,
    output_original_size_bytes: overrides.output_original_size_bytes,
    output_truncation_reason: overrides.output_truncation_reason,
    metadata: overrides.metadata,
    parent_span_id: overrides.parent_span_id,
  };
}

function createEvent(overrides: Partial<TimelineEvent> = {}): TimelineEvent {
  return {
    id: overrides.id ?? `event-${Math.random().toString(36).slice(2)}`,
    trace_id: overrides.trace_id ?? 'trace-1',
    event_type: overrides.event_type ?? 'message',
    timestamp: overrides.timestamp ?? '2026-03-14T10:00:00.000Z',
    source: overrides.source ?? 'explicit',
    span_id: overrides.span_id,
    span_name: overrides.span_name,
    level: overrides.level,
    sequence: overrides.sequence,
    message: overrides.message,
    payload: overrides.payload,
  };
}

describe('failureAnalysis', () => {
  it('builds a span index keyed by external span id', () => {
    const root = createSpan({ span_id: 'root', name: 'Root' });
    const child = createSpan({ span_id: 'child', name: 'Child' });

    const spanIndex = buildSpanIndex([root, child]);

    expect(spanIndex.get('root')).toBe(root);
    expect(spanIndex.get('child')).toBe(child);
  });

  it('finds the primary failed span using deterministic tie-breaking', () => {
    const spans = [
      createSpan({
        span_id: 'late-missing-end',
        status: 'FAILED',
        started_at: '2026-03-14T10:04:00.000Z',
      }),
      createSpan({
        span_id: 'same-end-later-start',
        status: 'FAILED',
        started_at: '2026-03-14T10:03:00.000Z',
        ended_at: '2026-03-14T10:05:00.000Z',
      }),
      createSpan({
        span_id: 'same-end-earlier-start',
        status: 'FAILED',
        started_at: '2026-03-14T10:02:00.000Z',
        ended_at: '2026-03-14T10:05:00.000Z',
      }),
      createSpan({
        span_id: 'earliest-end',
        status: 'FAILED',
        started_at: '2026-03-14T10:01:00.000Z',
        ended_at: '2026-03-14T10:04:00.000Z',
      }),
    ];

    expect(findPrimaryFailedSpan(spans)?.span_id).toBe('earliest-end');
  });

  it('falls back to original array order when ended_at and started_at are identical', () => {
    const first = createSpan({
      span_id: 'first',
      status: 'FAILED',
      started_at: '2026-03-14T10:00:00.000Z',
      ended_at: '2026-03-14T10:01:00.000Z',
    });
    const second = createSpan({
      span_id: 'second',
      status: 'FAILED',
      started_at: '2026-03-14T10:00:00.000Z',
      ended_at: '2026-03-14T10:01:00.000Z',
    });

    expect(findPrimaryFailedSpan([first, second])).toBe(first);
  });

  it('builds a breadcrumb path for normal, cyclic, and broken parent chains', () => {
    const root = createSpan({ span_id: 'root', name: 'Root' });
    const child = createSpan({
      span_id: 'child',
      name: 'Child',
      parent_span_id: 'root',
    });
    const grandchild = createSpan({
      span_id: 'grandchild',
      name: 'Grandchild',
      parent_span_id: 'child',
    });
    const broken = createSpan({
      span_id: 'broken',
      name: 'Broken',
      parent_span_id: 'missing',
    });
    const cycleA = createSpan({
      span_id: 'cycle-a',
      name: 'Cycle A',
      parent_span_id: 'cycle-b',
    });
    const cycleB = createSpan({
      span_id: 'cycle-b',
      name: 'Cycle B',
      parent_span_id: 'cycle-a',
    });
    const spanIndex = buildSpanIndex([
      root,
      child,
      grandchild,
      broken,
      cycleA,
      cycleB,
    ]);

    expect(buildBreadcrumbPath(grandchild, spanIndex)).toEqual([
      { spanId: 'root', name: 'Root' },
      { spanId: 'child', name: 'Child' },
      { spanId: 'grandchild', name: 'Grandchild' },
    ]);
    expect(buildBreadcrumbPath(broken, spanIndex)).toEqual([
      { spanId: 'broken', name: 'Broken' },
    ]);
    expect(buildBreadcrumbPath(cycleA, spanIndex)).toEqual([
      { spanId: 'cycle-b', name: 'Cycle B' },
      { spanId: 'cycle-a', name: 'Cycle A' },
    ]);
  });

  it('prefers span.error_message for inline previews and truncates the first line', () => {
    const span = createSpan({
      span_id: 'failed',
      status: 'FAILED',
      error_message:
        '   First line that is intentionally made very long to exceed the one hundred and twenty character preview limit by a noticeable margin.\nSecond line   ',
    });

    expect(getInlineErrorPreview(span, [])).toBe(
      'First line that is intentionally made very long to exceed the one hundred and twenty character preview limit by a not...'
    );
  });

  it('falls back to the first matching error or exception timeline message', () => {
    const span = createSpan({
      span_id: 'failed',
      status: 'FAILED',
    });
    const events = [
      createEvent({
        span_id: 'failed',
        event_type: 'span_failed',
        message: 'Synthetic failure event',
      }),
      createEvent({
        span_id: 'failed',
        event_type: 'error',
        message: '\n  Real failure details  \nMore context',
      }),
      createEvent({
        span_id: 'failed',
        event_type: 'exception',
        message: 'Later exception',
      }),
    ];

    expect(getInlineErrorPreview(span, events)).toBe('Real failure details');
  });

  it('builds a failure summary and failure analysis with consistent counts and previews', () => {
    const root = createSpan({
      span_id: 'root',
      name: 'Root',
      status: 'COMPLETED',
      ended_at: '2026-03-14T10:01:00.000Z',
    });
    const failed = createSpan({
      span_id: 'failed',
      name: 'Failed child',
      parent_span_id: 'root',
      status: 'FAILED',
      started_at: '2026-03-14T10:01:00.000Z',
      ended_at: '2026-03-14T10:02:00.000Z',
    });
    const laterFailed = createSpan({
      span_id: 'later-failed',
      name: 'Later failed child',
      parent_span_id: 'root',
      status: 'FAILED',
      started_at: '2026-03-14T10:02:30.000Z',
      ended_at: '2026-03-14T10:03:00.000Z',
      error_message: 'Later failure',
    });
    const events = [
      createEvent({
        span_id: 'failed',
        span_name: 'Failed child',
        event_type: 'error',
        timestamp: '2026-03-14T10:02:00.000Z',
        message: 'The first failure',
      }),
      createEvent({
        span_id: 'failed',
        event_type: 'message',
        timestamp: '2026-03-14T10:02:01.000Z',
        message: 'Not an error',
      }),
      createEvent({
        event_type: 'log',
        level: 'error',
        timestamp: '2026-03-14T10:02:02.000Z',
        message: 'Top-level error log',
      }),
      createEvent({
        span_id: 'later-failed',
        event_type: 'span_failed',
        timestamp: '2026-03-14T10:03:00.000Z',
        message: 'Later failed synthetic',
      }),
    ];

    const summary = buildFailureSummary({
      spans: [root, failed, laterFailed],
      events,
    });
    const analysis = buildFailureAnalysis([root, failed, laterFailed], events);

    expect(summary).toMatchObject({
      primaryFailedSpan: failed,
      failedSpanCount: 2,
      errorEventCount: 3,
      errorPreview: 'The first failure',
      failureTimestamp: '2026-03-14T10:02:00.000Z',
    });
    expect(summary.breadcrumbPath).toEqual([
      { spanId: 'root', name: 'Root' },
      { spanId: 'failed', name: 'Failed child' },
    ]);
    expect(analysis.failedSpanIds).toEqual(new Set(['failed', 'later-failed']));
    expect(analysis.inlineErrorPreviews.get('failed')).toBe('The first failure');
    expect(analysis.inlineErrorPreviews.get('later-failed')).toBe('Later failure');
    expect(analysis.primaryAncestorPath).toEqual(new Set(['root', 'failed']));
  });

  it('returns an empty failure summary when no spans are failed', () => {
    const summary = buildFailureSummary({
      spans: [
        createSpan({
          span_id: 'completed',
          status: 'COMPLETED',
        }),
      ],
      events: [],
    });

    expect(summary).toEqual({
      primaryFailedSpan: null,
      failedSpanCount: 0,
      errorEventCount: 0,
      errorPreview: null,
      failureTimestamp: null,
      breadcrumbPath: [],
    });
  });

  it('shows the stale trace signal only when both thresholds are exceeded', () => {
    const now = new Date('2026-03-14T10:30:00.000Z');
    const staleSignal = evaluateStaleTraceSignal({
      traceStatus: 'RUNNING',
      traceStartedAt: '2026-03-14T10:00:00.000Z',
      spans: [
        createSpan({
          span_id: 'active-span',
          started_at: '2026-03-14T10:05:00.000Z',
          ended_at: '2026-03-14T10:20:00.000Z',
        }),
      ],
      events: [
        createEvent({
          event_type: 'message',
          timestamp: '2026-03-14T10:22:00.000Z',
        }),
      ],
      now,
    });

    expect(staleSignal.shouldDisplay).toBe(true);
    expect(staleSignal.latestActivityAt).toBe('2026-03-14T10:22:00.000Z');
    expect(staleSignal.runtimeMs).toBeGreaterThanOrEqual(STALE_RUNTIME_THRESHOLD_MS);
    expect(staleSignal.inactivityMs).toBeGreaterThanOrEqual(
      STALE_INACTIVITY_THRESHOLD_MS
    );

    expect(
      evaluateStaleTraceSignal({
        traceStatus: 'RUNNING',
        traceStartedAt: '2026-03-14T10:20:00.000Z',
        spans: [],
        events: [],
        now,
      }).shouldDisplay
    ).toBe(false);

    expect(
      evaluateStaleTraceSignal({
        traceStatus: 'COMPLETED',
        traceStartedAt: '2026-03-14T10:00:00.000Z',
        spans: [],
        events: [],
        now,
      }).shouldDisplay
    ).toBe(false);
  });
});
