import type { TimelineEvent } from '../api/client';

export interface StateChangeDetails {
  key: string;
  namespace?: string;
  oldValue: unknown;
  newValue: unknown;
}

export interface DecisionDetails {
  question: string;
  chosen: unknown;
  alternatives?: unknown[];
  reasoning?: string;
}

export function getStateChangeDetails(
  event: TimelineEvent
): StateChangeDetails | null {
  if (event.event_type !== 'state_change' || !event.payload) {
    return null;
  }

  const key = getNonEmptyString(event.payload, 'key');
  if (!key) {
    return null;
  }

  return {
    key,
    namespace: getNonEmptyString(event.payload, 'namespace') ?? undefined,
    oldValue: event.payload.old_value,
    newValue: event.payload.new_value,
  };
}

export function getDecisionDetails(event: TimelineEvent): DecisionDetails | null {
  if (event.event_type !== 'decision' || !event.payload) {
    return null;
  }

  const question = getNonEmptyString(event.payload, 'question');
  const hasChosen = Object.prototype.hasOwnProperty.call(event.payload, 'chosen');
  if (!question || !hasChosen) {
    return null;
  }

  return {
    question,
    chosen: event.payload.chosen,
    alternatives: Array.isArray(event.payload.alternatives)
      ? event.payload.alternatives
      : undefined,
    reasoning: getNonEmptyString(event.payload, 'reasoning') ?? undefined,
  };
}

export function formatInlineSemanticValue(value: unknown): string {
  if (value === undefined) {
    return 'unset';
  }
  if (typeof value === 'string') {
    return value;
  }
  if (typeof value === 'number' || typeof value === 'boolean' || value === null) {
    return String(value);
  }

  try {
    return JSON.stringify(value) ?? 'unserializable';
  } catch {
    return 'unserializable';
  }
}

export function isStructuredSemanticValue(value: unknown): boolean {
  return typeof value === 'object' && value !== null;
}

function getNonEmptyString(
  payload: Record<string, unknown>,
  key: string
): string | null {
  const value = payload[key];
  return typeof value === 'string' && value.length > 0 ? value : null;
}
