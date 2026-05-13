import type { EngineProjectionState } from '../api/client';
import { formatProjectionStateLabel } from './engineProjectionState';

const toneByProjectionState: Record<EngineProjectionState, string> = {
  up_to_date:
    'border-[var(--c-green-border)] bg-[var(--c-green-faint)] text-[var(--c-green-text)]',
  catching_up:
    'border-[var(--c-amber-border)] bg-[var(--c-amber-faint)] text-[var(--c-amber-text)]',
  summary_only:
    'border-[var(--c-border)] bg-[var(--c-surface)] text-[var(--c-text-secondary)]',
  journal_expired:
    'border-[var(--c-red-border)] bg-[var(--c-red-faint)] text-[var(--c-red-text)]',
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
      className={`inline-flex h-5 items-center rounded border px-1.5 text-[11px] font-medium normal-case tracking-normal ${toneByProjectionState[projectionState]}`}
    >
      {showProjectionState ? `Engine · ${formatProjectionStateLabel(projectionState)}` : 'Engine'}
    </span>
  );
}
