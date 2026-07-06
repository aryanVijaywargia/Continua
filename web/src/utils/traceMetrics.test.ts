import { describe, expect, it } from 'vitest';
import { createSpan } from '../test/traceFixtures';
import {
  bucketLatencies,
  buildSpanMetricSeries,
  orderSpansByStart,
} from './traceMetrics';

describe('bucketLatencies', () => {
  it('returns no buckets without samples', () => {
    expect(bucketLatencies([])).toEqual([]);
  });

  it('uses fine-grained edges for sub-second latencies', () => {
    const buckets = bucketLatencies([10, 30, 600]);
    expect(buckets.map((bucket) => bucket.range)).toEqual([
      '0–25ms',
      '25–50ms',
      '50–100ms',
      '100–200ms',
      '200–350ms',
      '350–500ms',
      '500–750ms',
      '750+ ms',
    ]);
    expect(buckets.map((bucket) => bucket.n)).toEqual([1, 1, 0, 0, 0, 0, 1, 0]);
  });

  it('widens edges and counts overflow for multi-second latencies', () => {
    const buckets = bucketLatencies([100, 4500, 9000]);
    expect(buckets[buckets.length - 1]).toEqual({ range: '5000+ ms', n: 1 });
    expect(buckets[0]).toEqual({ range: '0–500ms', n: 1 });
  });
});

describe('orderSpansByStart', () => {
  it('sorts a copy of the spans by start time', () => {
    const late = createSpan({ started_at: '2026-03-14T10:00:02.000Z' });
    const early = createSpan({ started_at: '2026-03-14T10:00:00.000Z' });
    const input = [late, early];

    const ordered = orderSpansByStart(input);
    expect(ordered.map((span) => span.id)).toEqual([early.id, late.id]);
    expect(input.map((span) => span.id)).toEqual([late.id, early.id]);
  });
});

describe('buildSpanMetricSeries', () => {
  it('accumulates tokens and span cost while tracking error rate', () => {
    const ok = createSpan({
      latency_ms: 100,
      tokens_in: 10,
      tokens_out: 5,
      cost_usd: 0.01,
    });
    const failed = createSpan({
      status: 'FAILED',
      latency_ms: 300,
      tokens_in: 20,
      tokens_out: 5,
      cost_usd: 0.02,
    });

    const series = buildSpanMetricSeries([ok, failed], null);
    expect(series.latency).toEqual([100, 300]);
    expect(series.cumulativeTokens).toEqual([15, 40]);
    expect(series.cumulativeCost).toEqual([0.01, 0.03]);
    expect(series.errorRate).toEqual([0, 50]);
  });

  it('prefers the trace cost series when it has points', () => {
    const span = createSpan({ cost_usd: 0.5 });
    const series = buildSpanMetricSeries([span], {
      partial: false,
      points: [
        { anchorMs: 0, cumulativeCostUsd: 0.1, incrementalCostUsd: 0.1 },
        { anchorMs: 100, cumulativeCostUsd: 0.2, incrementalCostUsd: 0.1 },
      ],
      totalCostUsd: 0.2,
    });
    expect(series.cumulativeCost).toEqual([0.1, 0.2]);
  });
});
