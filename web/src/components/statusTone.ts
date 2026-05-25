export const STATUS_TONE = {
  QUEUED: {
    dot: 'var(--c-muted)',
    text: 'var(--c-muted-text)',
    label: 'Queued',
  },
  RUNNING: {
    dot: 'var(--c-blue)',
    text: 'var(--c-blue-text)',
    label: 'Running',
  },
  WAITING: {
    dot: 'var(--c-amber)',
    text: 'var(--c-amber-text)',
    label: 'Waiting',
  },
  SUSPENDED: {
    dot: 'var(--c-amber)',
    text: 'var(--c-amber-text)',
    label: 'Suspended',
  },
  STARTED: {
    dot: 'var(--c-blue)',
    text: 'var(--c-blue-text)',
    label: 'Started',
  },
  COMPLETED: {
    dot: 'var(--c-green)',
    text: 'var(--c-green-text)',
    label: 'Completed',
  },
  FAILED: {
    dot: 'var(--c-red)',
    text: 'var(--c-red-text)',
    label: 'Failed',
  },
  CANCELLED: {
    dot: 'var(--c-red)',
    text: 'var(--c-red-text)',
    label: 'Cancelled',
  },
  TERMINATED: {
    dot: 'var(--c-red)',
    text: 'var(--c-red-text)',
    label: 'Terminated',
  },
  CONTINUED_AS_NEW: {
    dot: 'var(--c-green)',
    text: 'var(--c-green-text)',
    label: 'Continued as new',
  },
  SCHEDULED: {
    dot: 'var(--c-muted)',
    text: 'var(--c-muted-text)',
    label: 'Scheduled',
  },
} as const;

export type ConsoleStatus = keyof typeof STATUS_TONE;

export function normalizeStatus(status?: string | null): ConsoleStatus {
  const upper = status?.toUpperCase();
  if (upper && upper in STATUS_TONE) {
    return upper as ConsoleStatus;
  }
  return 'SCHEDULED';
}
