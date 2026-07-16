import { type EngineWaitState } from '../api/client';
import { formatTimestamp } from '../utils/format';

export interface EngineWaitStateSummary {
  heading: string;
  detail: string;
}

export function describeEngineWaitState(
  waitState?: EngineWaitState | null
): EngineWaitStateSummary | null {
  if (!waitState?.kind) {
    return null;
  }

  switch (waitState.kind) {
    case 'activity':
      return {
        heading: 'Waiting on activity',
        detail: waitState.activity_type
          ? `${waitState.activity_type}${waitState.activity_key ? ` · ${waitState.activity_key}` : ''}`
          : (waitState.activity_key ?? 'Activity work'),
      };
    case 'timer':
      return {
        heading: 'Waiting on timer',
        detail: waitState.due_at
          ? `Scheduled for ${formatTimestamp(waitState.due_at)}`
          : (waitState.timer_key ?? 'Timer wait'),
      };
    case 'signal':
      return {
        heading: 'Waiting on signal',
        detail: waitState.signal_name ?? 'Signal receipt',
      };
    case 'child_workflow':
      return {
        heading: 'Waiting on child workflow',
        detail: waitState.child_key ?? 'Child workflow completion',
      };
    case 'replay_mismatch': {
      const expectedType =
        typeof waitState.expected_type === 'string' ? waitState.expected_type : null;
      const expectedKey =
        typeof waitState.expected_key === 'string' ? waitState.expected_key : null;
      const actualType =
        typeof waitState.actual_type === 'string' ? waitState.actual_type : null;
      const actualKey =
        typeof waitState.actual_key === 'string' ? waitState.actual_key : null;
      const detail = typeof waitState.detail === 'string' ? waitState.detail : null;

      return {
        heading: 'Replay mismatch',
        detail:
          expectedType && expectedKey && actualType && actualKey
            ? `expected ${expectedType} · ${expectedKey}, got ${actualType} · ${actualKey}`
            : (detail ?? 'Replay produced a different event'),
      };
    }
    case 'engine_invariant':
      return {
        heading: 'Engine invariant',
        detail:
          typeof waitState.detail === 'string'
            ? waitState.detail
            : 'Engine state invariant failed',
      };
    default:
      return {
        heading: 'Waiting on engine state',
        detail: waitState.kind,
      };
  }
}
