import { TimelineEvent } from '../api/client';
import {
  formatInlineSemanticValue,
  getDecisionDetails,
  getEffectDetails,
  getSnapshotMarkerDetails,
  getStateChangeDetails,
  getWaitDetails,
} from './eventSemantics';

/**
 * Merge timeline pages and polling updates without duplicating already-rendered events.
 */
export function mergeTimelineEvents(
  existing: TimelineEvent[],
  incoming: TimelineEvent[]
): TimelineEvent[] {
  const merged = new Map<string, TimelineEvent>();

  for (const event of existing) {
    merged.set(event.id, event);
  }
  for (const event of incoming) {
    merged.set(event.id, event);
  }

  return Array.from(merged.values()).sort(compareTimelineEvents);
}

export function compareTimelineEvents(a: TimelineEvent, b: TimelineEvent): number {
  const timestampDelta =
    new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime();
  if (timestampDelta !== 0) {
    return timestampDelta;
  }

  const sourceDelta = timelineSourceRank(a.source) - timelineSourceRank(b.source);
  if (sourceDelta !== 0) {
    return sourceDelta;
  }

  if (a.source === 'explicit' && b.source === 'explicit') {
    const sequenceDelta = compareNullableSequence(a.sequence, b.sequence);
    if (sequenceDelta !== 0) {
      return sequenceDelta;
    }
  }

  const phaseDelta = timelinePhaseRank(a) - timelinePhaseRank(b);
  if (phaseDelta !== 0) {
    return phaseDelta;
  }

  return a.id.localeCompare(b.id);
}

export function isTimelineErrorEvent(event: TimelineEvent): boolean {
  return (
    event.event_type === 'error' ||
    event.event_type === 'exception' ||
    event.event_type === 'span_failed' ||
    event.level === 'error'
  );
}

export function summarizeTimelineEvent(event: TimelineEvent): string {
  switch (event.event_type) {
    case 'state_change': {
      const details = getStateChangeDetails(event);
      if (details) {
        return `${details.key}: ${formatInlineSemanticValue(
          details.oldValue
        )} → ${formatInlineSemanticValue(details.newValue)}`;
      }
      return event.message ?? 'state change';
    }
    case 'decision': {
      const details = getDecisionDetails(event);
      if (details) {
        return `${details.question} → ${formatInlineSemanticValue(details.chosen)}`;
      }
      return event.message ?? 'decision';
    }
    case 'effect': {
      const details = getEffectDetails(event);
      if (details) {
        return `${details.effectKind} (${
          details.hasExternalSideEffect ? 'mutating' : 'read-only'
        })`;
      }
      return event.message ?? 'effect';
    }
    case 'snapshot_marker': {
      const details = getSnapshotMarkerDetails(event);
      if (details) {
        return details.label;
      }
      return event.message ?? 'snapshot marker';
    }
    case 'span_started':
      return `${event.span_name ?? event.span_id ?? 'Span'} started`;
    case 'span_completed':
      return `${event.span_name ?? event.span_id ?? 'Span'} completed`;
    case 'span_failed':
      return `${event.span_name ?? event.span_id ?? 'Span'} failed`;
    case 'metric':
      return formatMetricSummary(event);
    case 'wait':
      return formatWaitSummary(event);
    default:
      if (event.message) {
        return event.message;
      }
      return event.event_type.replace(/_/g, ' ');
  }
}

export function formatTimelineTime(timestamp: string): string {
  return new Date(timestamp).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  });
}

function formatMetricSummary(event: TimelineEvent): string {
  const metricName =
    typeof event.payload?.metric_name === 'string' ? event.payload.metric_name : null;
  const metricValue =
    typeof event.payload?.metric_value === 'number'
      ? event.payload.metric_value
      : null;

  if (metricName && metricValue !== null) {
    return `${metricName}: ${metricValue}`;
  }

  return 'Metric recorded';
}

function formatWaitSummary(event: TimelineEvent): string {
  const waitDetails = getWaitDetails(event);
  if (waitDetails) {
    if (waitDetails.phase === 'entered') {
      return `Entered wait: ${waitDetails.waitKind}`;
    }

    if (waitDetails.phase === 'resolved') {
      return waitDetails.resolution
        ? `Resolved wait: ${waitDetails.waitKind} → ${waitDetails.resolution}`
        : `Resolved wait: ${waitDetails.waitKind}`;
    }

    return `${capitalizeWaitPhase(waitDetails.phase)} wait`;
  }

  const rawPhase =
    typeof event.payload?.phase === 'string' && event.payload.phase.length > 0
      ? event.payload.phase
      : null;
  if (rawPhase) {
    return `${capitalizeWaitPhase(rawPhase)} wait`;
  }

  if (event.message) {
    return event.message;
  }

  return event.event_type.replace(/_/g, ' ');
}

function capitalizeWaitPhase(phase: string): string {
  return phase.charAt(0).toUpperCase() + phase.slice(1);
}

function timelineSourceRank(source: TimelineEvent['source']): number {
  return source === 'explicit' ? 0 : 1;
}

function timelinePhaseRank(event: TimelineEvent): number {
  switch (event.event_type) {
    case 'span_started':
      return 0;
    case 'span_completed':
    case 'span_failed':
      return 1;
    default:
      return 0;
  }
}

function compareNullableSequence(
  a: TimelineEvent['sequence'],
  b: TimelineEvent['sequence']
): number {
  if (a === undefined && b === undefined) {
    return 0;
  }
  if (a === undefined) {
    return 1;
  }
  if (b === undefined) {
    return -1;
  }
  return a - b;
}
