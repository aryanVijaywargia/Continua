interface SortableHeaderProps {
  label: string;
  isActive: boolean;
  isAscending: boolean;
  isDisabled?: boolean;
  onClick: () => void;
}

function getIndicator(isActive: boolean, isAscending: boolean): string {
  if (!isActive) {
    return '↕';
  }

  return isAscending ? '↑' : '↓';
}

export function SortableHeader({
  label,
  isActive,
  isAscending,
  isDisabled = false,
  onClick,
}: SortableHeaderProps) {
  const indicator = getIndicator(isActive, isAscending);

  if (isDisabled) {
    return (
      <span className="inline-flex items-center gap-1 text-[var(--continua-text-muted)] opacity-55">
        <span>{label}</span>
        <span aria-hidden="true">{indicator}</span>
      </span>
    );
  }

  return (
    <button
      type="button"
      onClick={onClick}
      className="inline-flex items-center gap-1 rounded-full px-2 py-1 text-[var(--continua-text-secondary)] transition hover:bg-[var(--continua-surface-muted)] hover:text-[var(--continua-text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--continua-accent-faint)]"
    >
      <span>{label}</span>
      <span aria-hidden="true">{indicator}</span>
    </button>
  );
}
