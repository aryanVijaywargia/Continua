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
      <span className="inline-flex items-center gap-1 text-gray-300">
        <span>{label}</span>
        <span aria-hidden="true">{indicator}</span>
      </span>
    );
  }

  return (
    <button
      type="button"
      onClick={onClick}
      className="inline-flex items-center gap-1 text-gray-500 transition hover:text-gray-700 focus:outline-none focus:ring-2 focus:ring-blue-200"
    >
      <span>{label}</span>
      <span aria-hidden="true">{indicator}</span>
    </button>
  );
}
