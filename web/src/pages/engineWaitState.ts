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
    default:
      return {
        heading: 'Waiting on engine state',
        detail: waitState.kind,
      };
  }
}
