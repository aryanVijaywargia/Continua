import type { EngineProjectionState } from '../api/client';
import { formatProjectionStateLabel } from './engineProjectionState';

const toneByProjectionState: Record<EngineProjectionState, string> = {
  up_to_date:
    'border-emerald-300/60 bg-emerald-100/80 text-emerald-900 dark:border-emerald-400/25 dark:bg-emerald-400/10 dark:text-emerald-100',
  catching_up:
    'border-amber-300/60 bg-amber-100/80 text-amber-900 dark:border-amber-400/25 dark:bg-amber-400/10 dark:text-amber-100',
  summary_only:
    'border-stone-300/60 bg-stone-100/80 text-stone-900 dark:border-stone-400/25 dark:bg-stone-400/10 dark:text-stone-100',
  journal_expired:
    'border-rose-300/60 bg-rose-100/80 text-rose-900 dark:border-rose-400/25 dark:bg-rose-400/10 dark:text-rose-100',
};

export function EngineBadge({
  projectionState = 'up_to_date',
  showProjectionState = false,
}: {
  projectionState?: EngineProjectionState;
  showProjectionState?: boolean;
}) {
  return (
    <span
      className={`inline-flex items-center rounded-full border px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.12em] ${toneByProjectionState[projectionState]}`}
    >
      {showProjectionState ? `Engine · ${formatProjectionStateLabel(projectionState)}` : 'Engine'}
    </span>
  );
}
