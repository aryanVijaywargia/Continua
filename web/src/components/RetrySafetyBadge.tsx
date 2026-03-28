import {
  getRetrySafetyLabel,
  type RetrySafetyClassification,
} from '../utils/retrySafety';

interface RetrySafetyBadgeProps {
  classification: RetrySafetyClassification;
  variant: 'compact' | 'full';
  'aria-label'?: string;
}

const BADGE_TONES: Record<RetrySafetyClassification, string> = {
  retryable:
    'border-emerald-200 bg-emerald-100 text-emerald-800 dark:border-emerald-500/40 dark:bg-emerald-500/15 dark:text-emerald-200',
  unsafe:
    'border-red-200 bg-red-100 text-red-800 dark:border-red-500/40 dark:bg-red-500/15 dark:text-red-200',
  unknown:
    'border-amber-200 bg-amber-100 text-amber-800 dark:border-amber-500/40 dark:bg-amber-500/15 dark:text-amber-200',
};

export function RetrySafetyBadge({
  classification,
  variant,
  'aria-label': ariaLabel,
}: RetrySafetyBadgeProps) {
  return (
    <span
      className={`inline-flex whitespace-nowrap rounded-full border font-semibold uppercase tracking-[0.16em] ${
        variant === 'compact' ? 'px-2 py-0.5 text-[10px]' : 'px-2.5 py-1 text-[11px]'
      } ${BADGE_TONES[classification]}`}
      aria-label={ariaLabel}
    >
      {getRetrySafetyLabel(classification)}
    </span>
  );
}
