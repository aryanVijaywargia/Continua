import type { Span, TimelineEvent } from '../api/client';

export type TimelineRowType = 'engine' | 'span' | 'log';
export type TimelineRowLevel = 'info' | 'warn' | 'error';

export interface TimelineRow {
  id: string;
  offsetMs: number;
  type: TimelineRowType;
  level: TimelineRowLevel;
  spanId?: string;
  spanName: string;
  message: string;
  meta?: string;
  rawEvent: TimelineEvent;
}

export interface TimelinePhase {
  id: string;
  name: string;
  status: string;
  startMs: number;
  durationMs: number;
}

export function classifyTimelineRow(
  event: TimelineEvent,
  rootSpanId: string | undefined
): TimelineRowType {
  if (
    event.event_type === 'span_started' ||
    event.event_type === 'span_completed' ||
    event.event_type === 'span_failed'
  ) {
    return 'span';
  }
  if (
    event.event_type === 'snapshot_marker' ||
    event.event_type === 'state_change' ||
    event.event_type === 'decision' ||
    (event.source === 'synthetic' && event.span_id === rootSpanId)
  ) {
    return 'engine';
  }
  return 'log';
}

export function classifyTimelineLevel(event: TimelineEvent): TimelineRowLevel {
  if (event.event_type === 'error' || event.event_type === 'exception' || event.event_type === 'span_failed') {
    return 'error';
  }
  if (event.level === 'error') return 'error';
  if (event.level === 'warning' || event.event_type === 'wait') return 'warn';
  return 'info';
}

export function timelineRowMessage(event: TimelineEvent): string {
  if (event.message) return event.message;
  switch (event.event_type) {
    case 'span_started':
      return `Span started${event.span_name ? ` · ${event.span_name}` : ''}`;
    case 'span_completed':
      return `Span completed${event.span_name ? ` · ${event.span_name}` : ''}`;
    case 'span_failed':
      return `Span failed${event.span_name ? ` · ${event.span_name}` : ''}`;
    case 'snapshot_marker':
      return 'Checkpoint persisted';
    case 'state_change':
      return 'State change';
    case 'decision':
      return 'Decision recorded';
    case 'effect':
      return 'Effect recorded';
    case 'wait':
      return 'Awaiting signal';
    default:
      return event.event_type.replace(/_/g, ' ');
  }
}

export function timelineRowMeta(event: TimelineEvent): string | undefined {
  if (!event.payload) return undefined;
  const entries = Object.entries(event.payload).filter(
    ([key]) => key !== 'message' && key !== 'kind' && key !== 'level'
  );
  if (entries.length === 0) return undefined;
  const compact: string[] = [];
  for (const [key, value] of entries.slice(0, 3)) {
    if (value === null || value === undefined) continue;
    if (typeof value === 'object') continue;
    compact.push(`${key}=${String(value)}`);
  }
  return compact.length > 0 ? compact.join(' · ') : undefined;
}

export function formatOffset(ms: number): string {
  if (ms < 1000) return `+${ms}ms`;
  return `+${(ms / 1000).toFixed(2)}s`;
}

export function findRootSpanId(spans: Span[]): string | undefined {
  return spans.find((span) => !span.parent_span_id)?.id;
}

export function buildTimelineRows(
  events: TimelineEvent[],
  rootSpanId: string | undefined,
  traceStartMs: number
): TimelineRow[] {
  return events
    .map((event) => {
      const offsetMs = Math.max(new Date(event.timestamp).getTime() - traceStartMs, 0);
      return {
        id: event.id,
        offsetMs,
        type: classifyTimelineRow(event, rootSpanId),
        level: classifyTimelineLevel(event),
        spanId: event.span_id,
        spanName: event.span_name ?? event.span_id ?? '—',
        message: timelineRowMessage(event),
        meta: timelineRowMeta(event),
        rawEvent: event,
      };
    })
    .sort((a, b) => a.offsetMs - b.offsetMs);
}

export function buildTimelinePhases(
  spans: Span[],
  rootSpanId: string | undefined,
  traceStartMs: number,
  totalMs: number
): TimelinePhase[] {
  const topLevel = spans.filter(
    (span) => !span.parent_span_id || span.parent_span_id === rootSpanId
  );
  return topLevel
    .map((span) => {
      const startMs = new Date(span.started_at).getTime() - traceStartMs;
      const endMs = span.ended_at
        ? new Date(span.ended_at).getTime() - traceStartMs
        : totalMs;
      return {
        id: span.id,
        name: span.name,
        status: span.status,
        startMs: Math.max(startMs, 0),
        durationMs: Math.max(endMs - startMs, 0),
      };
    })
    .filter((phase) => phase.durationMs > 0);
}
