interface StatusBadgeProps {
  status: 'RUNNING' | 'COMPLETED' | 'FAILED' | 'SCHEDULED' | 'STARTED';
}

/**
 * Color-coded status badge component.
 */
export function StatusBadge({ status }: StatusBadgeProps) {
  const colors = {
    RUNNING: 'bg-blue-100 text-blue-800',
    STARTED: 'bg-blue-100 text-blue-800',
    COMPLETED: 'bg-green-100 text-green-800',
    FAILED: 'bg-red-100 text-red-800',
    SCHEDULED: 'bg-gray-100 text-gray-800',
  };

  return (
    <span
      className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${colors[status]}`}
    >
      {status}
    </span>
  );
}
