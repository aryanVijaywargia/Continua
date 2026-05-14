import { formatCost } from '../utils/format';
import type { TraceCostSeries } from '../utils/reasoning';
import type { WaterfallWindow } from '../utils/waterfallTime';

interface CostStripProps {
  series: TraceCostSeries | null;
  window: WaterfallWindow;
}

const CHART_BOTTOM_Y = 24;
const CHART_HEIGHT = 28;
const CHART_TOP_Y = 4;
const CHART_WIDTH = 100;

export function CostStrip({ series, window }: CostStripProps) {
  if (!series) {
    return null;
  }

  const plottedPoints = series.points.map((point) => {
    const leftPercent = clamp(
      ((point.anchorMs - window.startMs) / window.durationMs) * 100,
      0,
      100
    );
    const x = (leftPercent / 100) * CHART_WIDTH;
    const y =
      CHART_BOTTOM_Y -
      (point.cumulativeCostUsd / Math.max(series.totalCostUsd, Number.EPSILON)) *
        (CHART_BOTTOM_Y - CHART_TOP_Y);

    return {
      ...point,
      leftPercent,
      x,
      y,
    };
  });
  const path = buildStepPath(plottedPoints);
  const lastPoint = plottedPoints[plottedPoints.length - 1];
  const labelLeftPercent = clamp(lastPoint.leftPercent + 1, 8, 82);

  return (
    <div className="grid grid-cols-[minmax(12rem,16rem)_minmax(0,1fr)_5rem] border-b border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)]">
      <div className="px-6 py-3 text-xs font-semibold uppercase tracking-[0.18em] text-[var(--continua-text-muted)]">
        Cumulative cost
      </div>
      <div className="relative border-l border-[var(--continua-border-soft)] px-4 py-3">
        <svg
          aria-label="Cumulative cost chart"
          className="h-12 w-full overflow-visible"
          viewBox={`0 0 ${CHART_WIDTH} ${CHART_HEIGHT}`}
          preserveAspectRatio="none"
        >
          <path
            d={`M 0 ${CHART_BOTTOM_Y} L ${CHART_WIDTH} ${CHART_BOTTOM_Y}`}
            fill="none"
            stroke="var(--continua-waterfall-tick-color)"
            strokeWidth="1"
            vectorEffect="non-scaling-stroke"
          />
          <path
            d={path}
            fill="none"
            stroke="var(--continua-waterfall-success-selected-border)"
            strokeWidth="1.8"
            vectorEffect="non-scaling-stroke"
          />
          <circle
            cx={lastPoint.x}
            cy={lastPoint.y}
            r="2.6"
            fill="var(--continua-waterfall-success-selected-border)"
          />
        </svg>

        <div
          className="pointer-events-none absolute top-1/2 -translate-y-1/2"
          style={{ left: `${labelLeftPercent}%` }}
        >
          <div className="inline-flex items-center gap-2 rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-3 py-1 text-xs font-medium text-[var(--continua-text-secondary)] shadow-sm">
            <span>{formatCost(series.totalCostUsd)}</span>
            {series.partial ? (
              <span className="rounded-full bg-amber-100 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-[0.16em] text-amber-800 dark:bg-amber-500/20 dark:text-amber-200">
                Partial
              </span>
            ) : null}
          </div>
        </div>
      </div>
      <div aria-hidden="true" />
    </div>
  );
}

function buildStepPath(
  points: Array<{ x: number; y: number }>
): string {
  let currentY = CHART_BOTTOM_Y;
  let path = `M 0 ${CHART_BOTTOM_Y}`;

  for (const point of points) {
    path += ` L ${point.x} ${currentY} L ${point.x} ${point.y}`;
    currentY = point.y;
  }

  return `${path} L ${CHART_WIDTH} ${currentY}`;
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max);
}
