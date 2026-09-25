import type { Span, TraceDetail } from '../../api/client';
import type { WaterfallWindow } from '../../utils/waterfallTime';

/** Steps shorter than this are "fast"; timestamp rounding makes them unreliable. */
export const SLOW_STEP_THRESHOLD_MS = 10;

export interface StepDuration {
  /** Duration in milliseconds, or null when nothing was recorded. */
  ms: number | null;
  /** True when the value is computed from timestamps, not recorded. */
  derived: boolean;
  /** True when the step has not ended yet. */
  running: boolean;
}

function parseMs(value: string | undefined | null): number | null {
  if (!value) {
    return null;
  }
  const parsed = new Date(value).getTime();
  return Number.isFinite(parsed) ? parsed : null;
}

function isLiveStatus(status: Span['status']): boolean {
  return status === 'STARTED' || status === 'SCHEDULED';
}

/**
 * The duration of one step. A recorded `latency_ms` wins; otherwise the value
 * is derived from the recorded start and end timestamps. A step that has not
 * ended yet is measured up to `nowMs`.
 */
export function getStepDuration(span: Span, nowMs: number): StepDuration {
  if (span.latency_ms != null && Number.isFinite(span.latency_ms)) {
    return { ms: span.latency_ms, derived: false, running: false };
  }

  const startMs = parseMs(span.started_at);
  const endMs = parseMs(span.ended_at);
  if (startMs !== null && endMs !== null) {
    return { ms: Math.max(0, endMs - startMs), derived: true, running: false };
  }
  if (startMs !== null && isLiveStatus(span.status)) {
    return { ms: Math.max(0, nowMs - startMs), derived: true, running: true };
  }

  return { ms: null, derived: false, running: false };
}

export function isSlowStep(duration: StepDuration): boolean {
  return duration.ms === null || duration.ms >= SLOW_STEP_THRESHOLD_MS;
}

export interface StepBarLayout {
  leftPercent: number;
  widthPercent: number;
}

/** Positions one step's bar on the trace range. */
export function getStepBarLayout(
  span: Span,
  duration: StepDuration,
  window: WaterfallWindow
): StepBarLayout {
  const rawStartMs = parseMs(span.started_at) ?? window.startMs;
  const rawEndMs =
    parseMs(span.ended_at) ??
    (duration.ms !== null ? rawStartMs + duration.ms : rawStartMs);
  const startMs = Math.min(Math.max(rawStartMs, window.startMs), window.endMs);
  const endMs = Math.min(Math.max(rawEndMs, startMs), window.endMs);

  return {
    leftPercent: ((startMs - window.startMs) / window.durationMs) * 100,
    widthPercent: ((endMs - startMs) / window.durationMs) * 100,
  };
}

/** Formats a step's share of the whole trace, e.g. "44%" or "<1%". */
export function formatShareOfTrace(ms: number | null, traceMs: number | null): string | null {
  if (ms === null || traceMs === null || traceMs <= 0) {
    return null;
  }
  const share = (ms / traceMs) * 100;
  if (share < 1) {
    return '<1%';
  }
  return `${Math.min(100, Math.round(share))}%`;
}

/**
 * True when the trace carries real token or cost usage. Zero totals are the
 * ingest default, so they do not prove that usage was measured.
 */
export function hasRecordedUsage(trace: TraceDetail): boolean {
  const tokens = (trace.total_tokens_in ?? 0) + (trace.total_tokens_out ?? 0);
  const cost = trace.total_cost_usd ?? 0;
  return tokens > 0 || cost > 0;
}

/** Exact time with seconds and milliseconds, e.g. "Mar 14, 2026, 10:00:00.120 UTC". */
export function formatPreciseTime(value: string | undefined | null): string {
  const ms = parseMs(value);
  if (ms === null) {
    return '—';
  }
  return new Date(ms).toLocaleString('en-US', {
    year: 'numeric',
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    fractionalSecondDigits: 3,
    hour12: false,
    timeZoneName: 'short',
  });
}

export interface ReadableField {
  key: string;
  value: string;
  tone: 'text' | 'literal' | 'muted';
}

export interface ReadablePayload {
  /** Set when the payload is one plain string. */
  text: string | null;
  fields: ReadableField[];
  /** Fields that were not shown because the payload is too large. */
  hiddenCount: number;
}

const MAX_READABLE_FIELDS = 80;
const MAX_READABLE_DEPTH = 4;
const MAX_INLINE_LIST = 6;

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function isPrimitive(value: unknown): boolean {
  return value === null || ['string', 'number', 'boolean'].includes(typeof value);
}

function primitiveField(key: string, value: unknown): ReadableField {
  if (value === null) {
    return { key, value: 'null', tone: 'muted' };
  }
  if (typeof value === 'string') {
    return value.length === 0
      ? { key, value: 'empty text', tone: 'muted' }
      : { key, value, tone: 'text' };
  }
  return { key, value: String(value), tone: 'literal' };
}

/**
 * Flattens a JSON payload into readable key/value rows, e.g.
 * `messages[0].role → user`. Short lists of plain values stay on one row.
 */
export function toReadablePayload(value: unknown): ReadablePayload {
  if (typeof value === 'string') {
    return { text: value, fields: [], hiddenCount: 0 };
  }

  const fields: ReadableField[] = [];
  let hiddenCount = 0;

  const push = (field: ReadableField) => {
    if (fields.length >= MAX_READABLE_FIELDS) {
      hiddenCount += 1;
      return;
    }
    fields.push(field);
  };

  const visit = (key: string, current: unknown, depth: number) => {
    if (isPrimitive(current)) {
      push(primitiveField(key, current));
      return;
    }

    if (Array.isArray(current)) {
      if (current.length === 0) {
        push({ key, value: 'empty list', tone: 'muted' });
        return;
      }
      if (current.length <= MAX_INLINE_LIST && current.every(isPrimitive)) {
        push({ key, value: current.map((item) => String(item)).join(', '), tone: 'text' });
        return;
      }
      if (depth >= MAX_READABLE_DEPTH) {
        push({ key, value: `${current.length} items`, tone: 'muted' });
        return;
      }
      current.forEach((item, index) => visit(`${key}[${index}]`, item, depth + 1));
      return;
    }

    if (isPlainObject(current)) {
      const entries = Object.entries(current);
      if (entries.length === 0) {
        push({ key: key || 'value', value: 'empty object', tone: 'muted' });
        return;
      }
      if (depth >= MAX_READABLE_DEPTH) {
        push({ key, value: `${entries.length} fields`, tone: 'muted' });
        return;
      }
      for (const [childKey, childValue] of entries) {
        visit(key ? `${key}.${childKey}` : childKey, childValue, depth + 1);
      }
      return;
    }

    push({ key, value: String(current), tone: 'muted' });
  };

  if (isPrimitive(value) || Array.isArray(value)) {
    visit('value', value, 0);
  } else {
    visit('', value, 0);
  }

  return { text: null, fields, hiddenCount };
}
