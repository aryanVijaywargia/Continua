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

export interface EffectDetails {
  effectKind: string;
  hasExternalSideEffect: boolean;
  effectId?: string;
  idempotent?: boolean;
  idempotencyKey?: string;
}

export interface WaitDetails {
  waitKind: string;
  phase: string;
  waitId?: string;
  resolution?: string;
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

export function getEffectDetails(event: TimelineEvent): EffectDetails | null {
  if (event.event_type !== 'effect' || !event.payload) {
    return null;
  }

  const effectKind = getNonEmptyString(event.payload, 'effect_kind');
  const hasExternalSideEffect = getBoolean(
    event.payload,
    'has_external_side_effect'
  );
  const effectId = getOptionalNonEmptyString(event.payload, 'effect_id');
  const idempotent = getOptionalBoolean(event.payload, 'idempotent');
  const idempotencyKey = getOptionalNonEmptyString(
    event.payload,
    'idempotency_key'
  );

  if (
    !effectKind ||
    hasExternalSideEffect === null ||
    effectId === null ||
    idempotent === null ||
    idempotencyKey === null
  ) {
    return null;
  }

  return {
    effectKind,
    hasExternalSideEffect,
    effectId,
    idempotent,
    idempotencyKey,
  };
}

export function getWaitDetails(event: TimelineEvent): WaitDetails | null {
  if (event.event_type !== 'wait' || !event.payload) {
    return null;
  }

  const waitKind = getNonEmptyString(event.payload, 'wait_kind');
  const phase = getNonEmptyString(event.payload, 'phase');
  const waitId = getOptionalNonEmptyString(event.payload, 'wait_id');
  const resolution = getOptionalNonEmptyString(event.payload, 'resolution');

  if (!waitKind || !phase || waitId === null || resolution === null) {
    return null;
  }

  return {
    waitKind,
    phase,
    waitId,
    resolution,
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

function getOptionalNonEmptyString(
  payload: Record<string, unknown>,
  key: string
): string | undefined | null {
  if (!Object.prototype.hasOwnProperty.call(payload, key)) {
    return undefined;
  }

  return getNonEmptyString(payload, key);
}

function getBoolean(payload: Record<string, unknown>, key: string): boolean | null {
  const value = payload[key];
  return typeof value === 'boolean' ? value : null;
}

function getOptionalBoolean(
  payload: Record<string, unknown>,
  key: string
): boolean | undefined | null {
  if (!Object.prototype.hasOwnProperty.call(payload, key)) {
    return undefined;
  }

  return getBoolean(payload, key);
}
