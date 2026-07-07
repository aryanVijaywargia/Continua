import type { TimelineEvent } from '../api/client';

export type LogLevel = 'info' | 'warn' | 'error' | 'debug';

export const LOG_LEVELS: LogLevel[] = ['info', 'warn', 'error', 'debug'];

export interface LogLine {
  id: string;
  timestamp: string;
  hms: string;
  level: LogLevel;
  source: string;
  message: string;
  spanId?: string;
}

export function deriveLogLevel(event: TimelineEvent): LogLevel {
  if (event.event_type === 'error' || event.event_type === 'exception' || event.event_type === 'span_failed') {
    return 'error';
  }
  if (event.level === 'error') return 'error';
  if (event.level === 'warning' || event.event_type === 'wait') return 'warn';
  if (event.level === 'debug') return 'debug';
  return 'info';
}

export function deriveLogSource(event: TimelineEvent): string {
  if (event.span_name) return event.span_name;
  if (event.event_type === 'snapshot_marker') return 'engine.checkpoint';
  if (event.event_type === 'state_change') return 'engine.state';
  if (event.event_type === 'decision') return 'engine.decision';
  return 'trace';
}

export function formatLogTimestamp(iso: string): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  const hh = String(date.getHours()).padStart(2, '0');
  const mm = String(date.getMinutes()).padStart(2, '0');
  const ss = String(date.getSeconds()).padStart(2, '0');
  const ms = String(date.getMilliseconds()).padStart(3, '0');
  return `${hh}:${mm}:${ss}.${ms}`;
}

/** Explicit events only — synthetic span lifecycle markers are not log lines. */
export function buildLogLines(events: TimelineEvent[]): LogLine[] {
  return events
    .filter((event) => event.source === 'explicit')
    .map((event) => ({
      id: event.id,
      timestamp: event.timestamp,
      hms: formatLogTimestamp(event.timestamp),
      level: deriveLogLevel(event),
      source: deriveLogSource(event),
      message: event.message ?? event.event_type.replace(/_/g, ' '),
      spanId: event.span_id,
    }));
}

export function formatLogLinesAsText(lines: LogLine[]): string {
  return lines
    .map(
      (line) =>
        `${line.hms} ${line.level.toUpperCase().padEnd(5)} ${line.source} — ${line.message}`
    )
    .join('\n');
}
