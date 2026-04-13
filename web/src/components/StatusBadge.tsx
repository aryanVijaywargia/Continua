interface StatusBadgeProps {
  status: 'RUNNING' | 'COMPLETED' | 'FAILED' | 'SCHEDULED' | 'STARTED';
}

/**
 * Color-coded status badge component.
 */
export function StatusBadge({ status }: StatusBadgeProps) {
  const colors = {
    RUNNING:
      'border border-sky-300/60 bg-sky-100/80 text-sky-900 dark:border-sky-400/25 dark:bg-sky-400/10 dark:text-sky-100',
    STARTED:
      'border border-sky-300/60 bg-sky-100/80 text-sky-900 dark:border-sky-400/25 dark:bg-sky-400/10 dark:text-sky-100',
    COMPLETED:
      'border border-emerald-300/60 bg-emerald-100/80 text-emerald-900 dark:border-emerald-400/20 dark:bg-emerald-400/10 dark:text-emerald-100',
    FAILED:
      'border border-red-300/60 bg-red-100/80 text-red-900 dark:border-red-400/25 dark:bg-red-400/10 dark:text-red-100',
    SCHEDULED:
      'border border-[var(--continua-border-strong)] bg-[var(--continua-surface-muted)] text-[var(--continua-text-secondary)]',
  };

  return (
    <span
      className={`inline-flex items-center rounded-full px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.14em] ${colors[status]}`}
    >
      {status}
    </span>
  );
}
