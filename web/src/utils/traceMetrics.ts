import type { Span } from '../api/client';
import type { TraceCostSeries } from './reasoning';

export interface LatencyBucket {
  range: string;
  n: number;
}

export function bucketLatencies(latencies: number[]): LatencyBucket[] {
  if (latencies.length === 0) return [];
  const max = Math.max(...latencies);
  const ceiling = max <= 0 ? 1 : max;
  const edges =
    ceiling >= 4000
      ? [0, 500, 1000, 1500, 2000, 3000, 4000, 5000, Number.POSITIVE_INFINITY]
      : ceiling >= 1000
        ? [0, 100, 250, 500, 750, 1000, 1500, 2000, Number.POSITIVE_INFINITY]
        : [0, 25, 50, 100, 200, 350, 500, 750, Number.POSITIVE_INFINITY];
  const counts = new Array(edges.length - 1).fill(0);
  latencies.forEach((latency) => {
    for (let i = 0; i < edges.length - 1; i += 1) {
      if (latency >= edges[i] && latency < edges[i + 1]) {
        counts[i] += 1;
        return;
      }
    }
  });
  return counts.map((n, i) => {
    const lower = edges[i];
    const upper = edges[i + 1];
    const range =
      upper === Number.POSITIVE_INFINITY
        ? `${lower}+ ms`
        : upper >= 1000
          ? `${(lower / 1000).toFixed(0)}–${(upper / 1000).toFixed(0)}s`
          : `${lower}–${upper}ms`;
    return { range, n };
  });
}

export function orderSpansByStart(spans: Span[]): Span[] {
  return [...spans].sort(
    (a, b) => new Date(a.started_at).getTime() - new Date(b.started_at).getTime()
  );
}

export interface SpanMetricSeries {
  latency: number[];
  cumulativeTokens: number[];
  cumulativeCost: number[];
  errorRate: number[];
}

/**
 * Per-span sparkline series in span-start order. Cost prefers the trace cost
 * series (which knows about missing telemetry) and falls back to summing
 * span-level cost.
 */
export function buildSpanMetricSeries(
  orderedSpans: Span[],
  traceCostSeries: TraceCostSeries | null
): SpanMetricSeries {
  const latency = orderedSpans.map((span) => span.latency_ms ?? 0);

  const cumulativeTokens: number[] = [];
  let runningTokens = 0;
  orderedSpans.forEach((span) => {
    runningTokens += (span.tokens_in ?? 0) + (span.tokens_out ?? 0);
    cumulativeTokens.push(runningTokens);
  });

  const cumulativeCost: number[] = [];
  let runningCost = 0;
  if (traceCostSeries && traceCostSeries.points.length > 0) {
    traceCostSeries.points.forEach((point) => {
      runningCost = point.cumulativeCostUsd;
      cumulativeCost.push(runningCost);
    });
  } else {
    orderedSpans.forEach((span) => {
      runningCost += span.cost_usd ?? 0;
      cumulativeCost.push(runningCost);
    });
  }

  const errorRate = orderedSpans.map((_span, index) => {
    const failedSoFar = orderedSpans
      .slice(0, index + 1)
      .filter((s) => s.status === 'FAILED').length;
    return ((failedSoFar / (index + 1)) * 100) | 0;
  });

  return { latency, cumulativeTokens, cumulativeCost, errorRate };
}
