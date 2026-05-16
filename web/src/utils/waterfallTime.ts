import type { Span, TimelineEvent } from '../api/client';

const DEFAULT_TICK_COUNT = 5;

export interface WaterfallWindow {
  startMs: number;
  endMs: number;
  durationMs: number;
}

export interface WaterfallBarLayout {
  durationMs: number;
  isRunning: boolean;
  leftPercent: number;
  widthPercent: number;
}

export interface WaterfallTick {
  label: string;
  leftPercent: number;
}

interface DeriveWaterfallWindowOptions {
  traceStartedAt?: string;
  traceEndedAt?: string | null;
  spans: Span[];
  events: TimelineEvent[];
}

export function deriveWaterfallWindow({
  traceStartedAt,
  traceEndedAt,
  spans,
  events,
}: DeriveWaterfallWindowOptions): WaterfallWindow | null {
  const traceStartMs = parseTimestamp(traceStartedAt);
  let earliestSpanStartMs: number | null = null;
  let latestSpanTimestampMs: number | null = null;

  for (const span of spans) {
    earliestSpanStartMs = updateBoundary(
      earliestSpanStartMs,
      parseTimestamp(span.started_at),
      Math.min
    );
    latestSpanTimestampMs = updateBoundary(
      latestSpanTimestampMs,
      parseTimestamp(span.started_at),
      Math.max
    );
    latestSpanTimestampMs = updateBoundary(
      latestSpanTimestampMs,
      parseTimestamp(span.ended_at),
      Math.max
    );
  }

  let earliestEventMs: number | null = null;
  let latestEventMs: number | null = null;

  for (const event of events) {
    const eventTimestampMs = parseTimestamp(event.timestamp);
    earliestEventMs = updateBoundary(earliestEventMs, eventTimestampMs, Math.min);
    latestEventMs = updateBoundary(latestEventMs, eventTimestampMs, Math.max);
  }

  const startMs = firstDefinedNumber(
    traceStartMs,
    earliestSpanStartMs,
    earliestEventMs
  );

  if (startMs === null) {
    return null;
  }

  const traceEndMs = parseTimestamp(traceEndedAt);
  const endMs =
    traceEndMs ??
    Math.max(
      latestSpanTimestampMs ?? startMs,
      latestEventMs ?? startMs,
      startMs
    );
  if (endMs === null) {
    return null;
  }
  const normalizedEndMs = endMs > startMs ? endMs : startMs + 1;

  return {
    startMs,
    endMs: normalizedEndMs,
    durationMs: normalizedEndMs - startMs,
  };
}

export function getWaterfallBarLayout(
  span: Span,
  window: WaterfallWindow
): WaterfallBarLayout {
  const spanStartMs = parseTimestamp(span.started_at) ?? window.startMs;
  const spanEndMs = parseTimestamp(span.ended_at) ?? window.endMs;
  const clampedStartMs = clamp(spanStartMs, window.startMs, window.endMs);
  const clampedEndMs = clamp(spanEndMs, clampedStartMs, window.endMs);
  const durationMs = Math.max(0, clampedEndMs - clampedStartMs);

  return {
    durationMs,
    isRunning: !span.ended_at,
    leftPercent: ((clampedStartMs - window.startMs) / window.durationMs) * 100,
    widthPercent: (durationMs / window.durationMs) * 100,
  };
}

export function buildWaterfallTicks(
  window: WaterfallWindow,
  tickCount = DEFAULT_TICK_COUNT
): WaterfallTick[] {
  if (tickCount <= 1) {
    return [
      {
        label: formatWaterfallOffset(0),
        leftPercent: 0,
      },
    ];
  }

  const ticks = Array.from({ length: tickCount }, (_, index) => {
    const leftPercent = (index / (tickCount - 1)) * 100;
    const offsetMs = window.durationMs * (leftPercent / 100);

    return {
      label: formatWaterfallOffset(offsetMs),
      leftPercent,
    };
  });

  const dedupedTicks = new Map<string, WaterfallTick>();
  const lastTickLabel = ticks[ticks.length - 1]?.label;
  for (const tick of ticks) {
    if (!dedupedTicks.has(tick.label) || tick.label === lastTickLabel) {
      dedupedTicks.set(tick.label, tick);
    }
  }

  return Array.from(dedupedTicks.values());
}

function parseTimestamp(value: string | undefined | null) {
  if (!value) {
    return null;
  }

  const parsed = Date.parse(value);
  return Number.isNaN(parsed) ? null : parsed;
}

function firstDefinedNumber(...values: Array<number | null>) {
  for (const value of values) {
    if (value !== null) {
      return value;
    }
  }

  return null;
}

function clamp(value: number, min: number, max: number) {
  return Math.min(Math.max(value, min), max);
}

function updateBoundary(
  current: number | null,
  next: number | null,
  reducer: (left: number, right: number) => number
) {
  if (next === null) {
    return current;
  }

  if (current === null) {
    return next;
  }

  return reducer(current, next);
}

function formatWaterfallOffset(offsetMs: number) {
  if (offsetMs < 1000) {
    return `${Math.round(offsetMs)}ms`;
  }

  if (offsetMs < 60_000) {
    return `${(offsetMs / 1000).toFixed(1)}s`;
  }

  if (offsetMs < 3_600_000) {
    return `${(offsetMs / 60_000).toFixed(1)}m`;
  }

  return `${(offsetMs / 3_600_000).toFixed(1)}h`;
}
