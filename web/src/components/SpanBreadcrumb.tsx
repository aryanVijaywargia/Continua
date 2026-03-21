import type { BreadcrumbSegment } from '../utils/failureAnalysis';

interface SpanBreadcrumbProps {
  path: BreadcrumbSegment[];
  onSelectSpan?: (spanId: string) => void;
  ariaLabel?: string;
  className?: string;
}

export function SpanBreadcrumb({
  path,
  onSelectSpan,
  ariaLabel = 'Span breadcrumb',
  className = '',
}: SpanBreadcrumbProps) {
  if (path.length === 0) {
    return null;
  }

  const lastIndex = path.length - 1;

  return (
    <nav
      aria-label={ariaLabel}
      className={`overflow-x-auto ${className}`.trim()}
    >
      <ol className="flex min-w-0 flex-wrap items-center gap-2 text-xs text-slate-500 dark:text-slate-400">
        {path.map((segment, index) => {
          const isCurrent = index === lastIndex;
          const isInteractive = !isCurrent && Boolean(onSelectSpan);

          return (
            <li key={segment.spanId} className="flex min-w-0 items-center gap-2">
              {isInteractive ? (
                <button
                  type="button"
                  className="truncate rounded-full border border-slate-200 bg-white px-2.5 py-1 font-medium text-slate-700 transition hover:bg-slate-100 focus:outline-none focus:ring-2 focus:ring-blue-200 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200 dark:hover:bg-slate-800"
                  aria-label={`Select ancestor span ${segment.name}`}
                  onClick={() => onSelectSpan?.(segment.spanId)}
                >
                  {segment.name}
                </button>
              ) : (
                <span
                  className={`truncate rounded-full px-2.5 py-1 ${
                    isCurrent
                      ? 'bg-slate-900 text-white dark:bg-slate-100 dark:text-slate-950'
                      : 'bg-slate-100 font-medium text-slate-700 dark:bg-slate-800 dark:text-slate-200'
                  }`}
                  aria-current={isCurrent ? 'page' : undefined}
                >
                  {segment.name}
                </span>
              )}

              {!isCurrent && <span aria-hidden="true">{'>'}</span>}
            </li>
          );
        })}
      </ol>
    </nav>
  );
}
