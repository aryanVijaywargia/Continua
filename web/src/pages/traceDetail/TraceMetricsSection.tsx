import { useMemo, type ReactNode } from 'react';
import { formatCost, formatDuration, formatTokens } from '../../utils/format';
import {
  bucketLatencies,
  buildSpanMetricSeries,
  orderSpansByStart,
} from '../../utils/traceMetrics';
import { useTraceDetailWorkspace } from './traceDetailWorkspaceContext';

type MetricTone = 'muted' | 'amber' | 'red' | 'green';

interface MetricCardSpec {
  label: string;
  value: string;
  delta?: string;
  tone: MetricTone;
  series: number[];
  anomaly?: boolean;
}

const METRIC_TONE_STROKE: Record<MetricTone, string> = {
  muted: 'var(--c-text-muted)',
  amber: 'var(--c-amber)',
  red: 'var(--c-red)',
  green: 'var(--c-green)',
};

const METRIC_TONE_TEXT: Record<MetricTone, string> = {
  muted: 'var(--c-text-muted)',
  amber: 'var(--c-amber-text)',
  red: 'var(--c-red-text)',
  green: 'var(--c-green-text)',
};

/** Aggregate latency, token, cost, and state-change signals for the trace. */
export function TraceMetricsSection() {
  const { events, spans, trace, traceCostSeries } = useTraceDetailWorkspace();

  const totalSpans = spans.length;
  const failedSpans = spans.filter((span) => span.status === 'FAILED').length;
  const runningSpans = spans.filter((span) => span.status === 'STARTED').length;
  const completedSpans = spans.filter((span) => span.status === 'COMPLETED').length;
  const totalLatencyMs = spans.reduce((sum, span) => sum + (span.latency_ms ?? 0), 0);
  const explicitEventCount = events.filter((event) => event.source === 'explicit').length;
  const errorEventCount = events.filter(
    (event) => event.event_type === 'error' || event.event_type === 'exception',
  ).length;

  const traceDuration = trace.ended_at
    ? new Date(trace.ended_at).getTime() - new Date(trace.started_at).getTime()
    : 0;
  const errorRate = totalSpans === 0 ? 0 : (failedSpans / totalSpans) * 100;
  const tokensIn = trace.total_tokens_in ?? 0;
  const tokensOut = trace.total_tokens_out ?? 0;
  const totalTokens = tokensIn + tokensOut;
  const totalCost = trace.total_cost_usd ?? 0;

  const orderedSpans = useMemo(() => orderSpansByStart(spans), [spans]);
  const series = useMemo(
    () => buildSpanMetricSeries(orderedSpans, traceCostSeries),
    [orderedSpans, traceCostSeries]
  );

  const cards: MetricCardSpec[] = [
    {
      label: 'Duration',
      value: formatDuration(traceDuration || totalLatencyMs),
      delta:
        traceDuration > 0 ? `${formatDuration(totalLatencyMs)} span time` : 'Trace still running',
      tone: trace.status === 'FAILED' ? 'red' : trace.status === 'RUNNING' ? 'amber' : 'muted',
      series: series.latency.length > 0 ? series.latency : [0],
      anomaly: trace.status === 'FAILED',
    },
    {
      label: 'Tokens',
      value: formatTokens(totalTokens),
      delta:
        totalTokens > 0
          ? `in ${formatTokens(tokensIn)} · out ${formatTokens(tokensOut)}`
          : 'No token telemetry',
      tone: 'muted',
      series: series.cumulativeTokens.length > 0 ? series.cumulativeTokens : [0],
    },
    {
      label: 'Cost',
      value: formatCost(totalCost),
      delta:
        series.cumulativeCost.length > 0
          ? `${series.cumulativeCost.length} sample(s)`
          : 'No cost telemetry',
      tone: 'muted',
      series: series.cumulativeCost.length > 0 ? series.cumulativeCost : [0],
    },
    {
      label: 'Error rate',
      value: `${Math.round(errorRate)}%`,
      delta:
        failedSpans > 0
          ? `${failedSpans} of ${totalSpans} spans failed`
          : 'All spans recorded clean',
      tone: failedSpans > 0 ? 'red' : 'green',
      series: series.errorRate.length > 0 ? series.errorRate : [0],
      anomaly: failedSpans > 0,
    },
  ];

  const latencyByRow = useMemo(() => {
    const totals = totalLatencyMs <= 0 ? 1 : totalLatencyMs;
    return [...spans]
      .sort((a, b) => (b.latency_ms ?? 0) - (a.latency_ms ?? 0))
      .slice(0, 6)
      .map((span) => {
        const ms = span.latency_ms ?? 0;
        const pct = (ms / totals) * 100;
        return {
          id: span.id,
          name: span.name,
          ms,
          pct,
          status: span.status,
        };
      });
  }, [spans, totalLatencyMs]);

  const distributionBuckets = useMemo(
    () => bucketLatencies(spans.map((span) => span.latency_ms ?? 0)),
    [spans],
  );
  const distributionMax = Math.max(...distributionBuckets.map((bucket) => bucket.n), 1);

  const tokenSegments = useMemo(() => {
    if (totalTokens === 0) return [];
    return [
      { label: 'Input', n: tokensIn, pct: (tokensIn / totalTokens) * 100, color: 'var(--c-blue)' },
      {
        label: 'Output',
        n: tokensOut,
        pct: (tokensOut / totalTokens) * 100,
        color: 'var(--c-accent)',
      },
    ];
  }, [tokensIn, tokensOut, totalTokens]);

  const spanKindCounts = useMemo(() => {
    const counts = new Map<string, number>();
    spans.forEach((span) => {
      counts.set(span.kind, (counts.get(span.kind) ?? 0) + 1);
    });
    return Array.from(counts.entries())
      .sort((a, b) => b[1] - a[1])
      .slice(0, 4)
      .map(([kind, count]) => `${kind.toLowerCase()} · ${count}`);
  }, [spans]);

  const spanSignals: Array<[string, string]> = [
    ['Spans', String(totalSpans)],
    ['Completed', String(completedSpans)],
    ['Running', String(runningSpans)],
    ['Failed', String(failedSpans)],
    ['Span latency', formatDuration(totalLatencyMs)],
    ['Avg span latency', totalSpans > 0 ? formatDuration(totalLatencyMs / totalSpans) : '—'],
    ['Span kinds', spanKindCounts.length > 0 ? spanKindCounts.join(', ') : '—'],
    ['Explicit events', `${explicitEventCount} · ${errorEventCount} error`],
  ];

  const engineSignals: Array<[string, string]> = trace.engine
    ? [
        ['Definition', trace.engine.definition_name ?? '—'],
        ['Version', trace.engine.definition_version ?? '—'],
        ['Run', trace.engine.run_id ?? '—'],
        ['Instance', trace.engine.instance_key ?? '—'],
        ['Run status', trace.engine.status ?? '—'],
        ['Projection', trace.engine.projection_state ?? '—'],
      ]
    : [['Engine signals', 'No engine attached to this trace.']];

  const statusToneFromSpan = (status: string) =>
    status === 'FAILED' ? 'var(--c-red)' : status === 'STARTED' ? 'var(--c-blue)' : 'var(--c-green)';

  return (
    <div className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
        {cards.map((card) => (
          <MetricCard key={card.label} card={card} />
        ))}
      </div>

      <MetricsSection title="Latency by span" hint="Top contributors to wall time">
        {latencyByRow.length === 0 ? (
          <p className="py-2 text-[11.5px] text-[var(--c-text-muted)]">
            No spans with measured latency yet.
          </p>
        ) : (
          <div className="flex flex-col">
            {latencyByRow.map((row, index) => (
              <div
                key={row.id}
                className="grid items-center gap-4 py-2"
                style={{
                  gridTemplateColumns: 'minmax(0, 320px) minmax(0, 1fr) 80px 60px',
                  borderBottom:
                    index < latencyByRow.length - 1 ? '1px solid var(--c-border-subtle)' : 'none',
                }}
              >
                <div className="flex items-center gap-2 truncate font-mono text-xs text-[var(--c-text-primary)]">
                  <span
                    className="inline-block h-1.5 w-1.5 shrink-0 rounded-full"
                    style={{ background: statusToneFromSpan(row.status) }}
                  />
                  <span className="truncate">{row.name}</span>
                </div>
                <div className="relative h-1.5 overflow-hidden rounded-full bg-[var(--c-surface-muted)]">
                  <div
                    className="absolute left-0 top-0 bottom-0 rounded-full"
                    style={{
                      width: `${Math.max(row.pct, 0.5)}%`,
                      background:
                        row.status === 'FAILED' ? 'var(--c-bar-failed)' : 'var(--c-bar-success)',
                    }}
                  />
                </div>
                <div className="text-right font-mono text-[11.5px] tabular-nums text-[var(--c-text-secondary)]">
                  {formatDuration(row.ms)}
                </div>
                <div className="text-right font-mono text-[11px] tabular-nums text-[var(--c-text-muted)]">
                  {Math.round(row.pct)}%
                </div>
              </div>
            ))}
          </div>
        )}
      </MetricsSection>

      <div className="grid gap-4 xl:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)]">
        <MetricsSection title="Latency distribution" hint="Spans bucketed by duration">
          {distributionBuckets.length === 0 ? (
            <p className="py-2 text-[11.5px] text-[var(--c-text-muted)]">
              No latency samples yet.
            </p>
          ) : (
            <>
              <div className="flex h-24 items-end gap-1">
                {distributionBuckets.map((bucket) => (
                  <div
                    key={bucket.range}
                    className="flex flex-1 flex-col items-center gap-1"
                  >
                    <div
                      className="w-full rounded-t-[2px]"
                      style={{
                        height: `${(bucket.n / distributionMax) * 100}%`,
                        minHeight: bucket.n > 0 ? 2 : 0,
                        background: 'var(--c-text-muted)',
                        opacity: bucket.n > 0 ? 0.55 : 0.18,
                      }}
                      title={`${bucket.range} · ${bucket.n} span(s)`}
                    />
                    <span className="font-mono text-[9.5px] text-[var(--c-text-muted)]">
                      {bucket.range}
                    </span>
                  </div>
                ))}
              </div>
              <div className="mt-3 flex justify-between border-t border-[var(--c-border-subtle)] pt-2 font-mono text-[11px] text-[var(--c-text-muted)]">
                <span>
                  min{' '}
                  <span className="text-[var(--c-text-secondary)]">
                    {formatDuration(Math.min(...spans.map((s) => s.latency_ms ?? 0)))}
                  </span>
                </span>
                <span>
                  median{' '}
                  <span className="text-[var(--c-text-secondary)]">
                    {totalSpans > 0
                      ? formatDuration(
                          [...spans]
                            .map((s) => s.latency_ms ?? 0)
                            .sort((a, b) => a - b)[Math.floor(totalSpans / 2)],
                        )
                      : '—'}
                  </span>
                </span>
                <span>
                  max{' '}
                  <span className="text-[var(--c-text-secondary)]">
                    {formatDuration(Math.max(...spans.map((s) => s.latency_ms ?? 0)))}
                  </span>
                </span>
              </div>
            </>
          )}
        </MetricsSection>

        <MetricsSection title="Token usage" hint="Input vs. output split">
          {tokenSegments.length === 0 ? (
            <p className="py-2 text-[11.5px] text-[var(--c-text-muted)]">
              No token telemetry recorded.
            </p>
          ) : (
            <>
              <div className="flex h-3 overflow-hidden rounded-full bg-[var(--c-surface-muted)]">
                {tokenSegments.map((segment) => (
                  <div
                    key={segment.label}
                    style={{ width: `${segment.pct}%`, background: segment.color }}
                  />
                ))}
              </div>
              <div className="mt-3 flex flex-col gap-1.5">
                {tokenSegments.map((segment) => (
                  <div
                    key={segment.label}
                    className="grid items-center gap-2 text-[11.5px]"
                    style={{ gridTemplateColumns: '12px minmax(0, 1fr) auto auto' }}
                  >
                    <span
                      className="inline-block h-2 w-2 rounded-sm"
                      style={{ background: segment.color }}
                    />
                    <span className="text-[var(--c-text-secondary)]">{segment.label}</span>
                    <span className="font-mono tabular-nums text-[var(--c-text-primary)]">
                      {formatTokens(segment.n)}
                    </span>
                    <span className="min-w-[32px] text-right font-mono tabular-nums text-[var(--c-text-muted)]">
                      {Math.round(segment.pct)}%
                    </span>
                  </div>
                ))}
              </div>
            </>
          )}
        </MetricsSection>
      </div>

      <div className="grid gap-4 xl:grid-cols-2">
        <MetricsSection title="Span signals">
          <MetricsKvGrid rows={spanSignals} />
        </MetricsSection>
        <MetricsSection title="Engine signals">
          <MetricsKvGrid rows={engineSignals} />
        </MetricsSection>
      </div>
    </div>
  );
}

function MetricCard({ card }: { card: MetricCardSpec }) {
  const max = Math.max(...card.series, 1);
  const points = card.series
    .map((value, index) => {
      const x = card.series.length === 1 ? 100 : (index / (card.series.length - 1)) * 100;
      const y = 24 - (value / max) * 22;
      return `${x.toFixed(2)},${y.toFixed(2)}`;
    })
    .join(' ');
  const lastX = 100;
  const lastY = 24 - (card.series[card.series.length - 1] / max) * 22;
  const stroke = METRIC_TONE_STROKE[card.tone];
  return (
    <div className="rounded-lg border border-[var(--c-border)] bg-[var(--c-surface)] px-3.5 py-3">
      <div className="flex items-center justify-between">
        <span className="text-[10.5px] font-semibold uppercase tracking-[0.05em] text-[var(--c-text-muted)]">
          {card.label}
        </span>
        {card.anomaly ? (
          <span
            aria-hidden="true"
            className="inline-block h-1.5 w-1.5 rounded-full"
            style={{ background: stroke }}
            title="Anomaly"
          />
        ) : null}
      </div>
      <div className="mt-1 text-[22px] font-semibold tabular-nums leading-tight tracking-[-0.015em] text-[var(--c-text-primary)]">
        {card.value}
      </div>
      {card.delta ? (
        <div className="mt-1 text-[11px]" style={{ color: METRIC_TONE_TEXT[card.tone] }}>
          {card.delta}
        </div>
      ) : null}
      <svg
        viewBox="0 0 100 24"
        preserveAspectRatio="none"
        className="mt-2 block h-7 w-full"
        aria-hidden="true"
      >
        <polyline
          points={points}
          fill="none"
          stroke={stroke}
          strokeWidth={1.2}
          vectorEffect="non-scaling-stroke"
        />
        {card.anomaly ? <circle cx={lastX} cy={lastY} r={2} fill={stroke} /> : null}
      </svg>
    </div>
  );
}

function MetricsSection({
  children,
  hint,
  title,
}: {
  children: ReactNode;
  hint?: string;
  title: string;
}) {
  return (
    <section>
      <div className="mb-2 flex items-baseline justify-between">
        <h3 className="text-[12.5px] font-semibold tracking-[-0.005em] text-[var(--c-text-primary)]">
          {title}
        </h3>
        {hint ? <span className="text-[11px] text-[var(--c-text-muted)]">{hint}</span> : null}
      </div>
      <div className="rounded-lg border border-[var(--c-border)] bg-[var(--c-surface)] px-3.5 py-3">
        {children}
      </div>
    </section>
  );
}

function MetricsKvGrid({ rows }: { rows: Array<[string, string]> }) {
  return (
    <div className="flex flex-col">
      {rows.map(([label, value], index) => (
        <div
          key={label}
          className="grid grid-cols-[160px_minmax(0,1fr)] gap-3 py-1.5 text-[11.5px]"
          style={{
            borderBottom:
              index < rows.length - 1 ? '1px solid var(--c-border-subtle)' : 'none',
          }}
        >
          <span className="text-[var(--c-text-muted)]">{label}</span>
          <span className="truncate font-mono tabular-nums text-[var(--c-text-primary)]">
            {value}
          </span>
        </div>
      ))}
    </div>
  );
}
