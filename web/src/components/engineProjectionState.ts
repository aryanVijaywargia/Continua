import type { EngineProjectionState } from '../api/client';

export function formatProjectionStateLabel(state: EngineProjectionState): string {
  switch (state) {
    case 'catching_up':
      return 'Catching up';
    case 'summary_only':
      return 'Summary only';
    case 'journal_expired':
      return 'Journal expired';
    default:
      return 'Up to date';
  }
}
