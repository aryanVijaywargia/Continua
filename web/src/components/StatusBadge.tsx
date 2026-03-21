interface StatusBadgeProps {
  status: 'RUNNING' | 'COMPLETED' | 'FAILED' | 'SCHEDULED' | 'STARTED';
}

/**
 * Color-coded status badge component.
 */
export function StatusBadge({ status }: StatusBadgeProps) {
  const colors = {
    RUNNING: 'bg-blue-100 text-blue-800 dark:bg-blue-500/15 dark:text-blue-200',
    STARTED: 'bg-blue-100 text-blue-800 dark:bg-blue-500/15 dark:text-blue-200',
    COMPLETED: 'bg-green-100 text-green-800 dark:bg-emerald-500/15 dark:text-emerald-200',
    FAILED: 'bg-red-100 text-red-800 dark:bg-red-500/15 dark:text-red-200',
    SCHEDULED: 'bg-gray-100 text-gray-800 dark:bg-slate-800 dark:text-slate-200',
  };

  return (
    <span
      className={`inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-medium ${colors[status]}`}
    >
      {status}
    </span>
  );
}
