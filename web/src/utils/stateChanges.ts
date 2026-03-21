import type { TimelineEvent } from '../api/client';
import { getStateChangeDetails } from './eventSemantics';

export interface ExtractedStateChange {
  event: TimelineEvent;
  key: string;
  namespace?: string;
  oldValue: unknown;
  newValue: unknown;
}

export function extractStateChanges(
  events: TimelineEvent[]
): ExtractedStateChange[] {
  return events.flatMap((event) => {
    const details = getStateChangeDetails(event);
    if (!details) {
      return [];
    }

    return [
      {
        event,
        key: details.key,
        namespace: details.namespace,
        oldValue: details.oldValue,
        newValue: details.newValue,
      },
    ];
  });
}
