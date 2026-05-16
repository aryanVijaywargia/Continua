interface StatusBadgeProps {
  status: 'RUNNING' | 'COMPLETED' | 'FAILED' | 'SCHEDULED' | 'STARTED';
}

/**
 * Color-coded status badge component.
 */
export function StatusBadge({ status }: StatusBadgeProps) {
  const colors = {
    RUNNING:
      'border border-[var(--c-blue-border)] bg-[var(--c-blue-faint)] text-[var(--c-blue-text)]',
    STARTED:
      'border border-[var(--c-blue-border)] bg-[var(--c-blue-faint)] text-[var(--c-blue-text)]',
    COMPLETED:
      'border border-[var(--c-green-border)] bg-[var(--c-green-faint)] text-[var(--c-green-text)]',
    FAILED:
      'border border-[var(--c-red-border)] bg-[var(--c-red-faint)] text-[var(--c-red-text)]',
    SCHEDULED:
      'border border-[var(--c-border)] bg-[var(--c-surface-muted)] text-[var(--c-text-secondary)]',
  };

  return (
    <span
      className={`inline-flex items-center rounded-full px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.14em] ${colors[status]}`}
    >
      {status}
    </span>
  );
}
