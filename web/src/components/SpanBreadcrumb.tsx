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
      <ol className="flex min-w-0 flex-wrap items-center gap-2 text-xs text-[var(--c-text-muted)]">
        {path.map((segment, index) => {
          const isCurrent = index === lastIndex;
          const isInteractive = !isCurrent && Boolean(onSelectSpan);

          return (
            <li key={segment.spanId} className="flex min-w-0 items-center gap-2">
              {isInteractive ? (
                <button
                  type="button"
                  className="truncate rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-2.5 py-1 font-medium text-[var(--c-text-secondary)] transition hover:border-[var(--c-border-strong)] hover:text-[var(--c-text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--c-accent-faint)]"
                  aria-label={`Select ancestor span ${segment.name}`}
                  onClick={() => onSelectSpan?.(segment.spanId)}
                >
                  {segment.name}
                </button>
              ) : (
                <span
                  className={`truncate rounded-full px-2.5 py-1 ${
                    isCurrent
                      ? 'bg-[var(--c-accent-faint)] text-[var(--c-accent-text)] ring-1 ring-[var(--c-accent-border)]'
                      : 'bg-[var(--c-surface-muted)] font-medium text-[var(--c-text-secondary)]'
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
