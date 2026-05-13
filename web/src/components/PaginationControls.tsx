import { useEffect } from 'react';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import {
  DEFAULT_PAGE_SIZE,
  PAGE_SIZE_OPTIONS,
  getLastValidOffset,
} from '../utils/pagination';

interface PaginationControlsProps {
  offset: number;
  pageSize: number;
  total: number;
  currentItemCount?: number;
  onOffsetChange: (offset: number) => void;
  onPageSizeChange: (pageSize: number) => void;
  onRepairOffset?: (offset: number) => void;
  pageSizeOptions?: readonly number[];
}

function buttonClassName(enabled: boolean): string {
  return enabled
    ? 'inline-flex h-7 min-w-7 items-center justify-center rounded border border-transparent px-2 text-[var(--c-text-secondary)] hover:border-[var(--c-border)] hover:bg-[var(--c-surface)]'
    : 'inline-flex h-7 min-w-7 cursor-not-allowed items-center justify-center rounded border border-transparent px-2 text-[var(--c-text-muted)] opacity-50';
}

export function PaginationControls({
  offset,
  pageSize,
  total,
  currentItemCount,
  onOffsetChange,
  onPageSizeChange,
  onRepairOffset = onOffsetChange,
  pageSizeOptions = PAGE_SIZE_OPTIONS,
}: PaginationControlsProps) {
  const lastValidOffset = getLastValidOffset(total, pageSize);
  const currentOffset = Math.min(offset, lastValidOffset);
  const totalPages = Math.max(1, Math.ceil(total / pageSize));
  const currentPage = total === 0 ? 1 : Math.floor(currentOffset / pageSize) + 1;
  const showingFrom = total === 0 ? 0 : currentOffset + 1;
  const showingTo = total === 0 ? 0 : Math.min(currentOffset + pageSize, total);
  const hasPreviousPage = currentOffset > 0;
  const hasNextPage = currentOffset + pageSize < total;

  useEffect(() => {
    if (
      currentItemCount === undefined ||
      currentItemCount > 0 ||
      offset <= lastValidOffset
    ) {
      return;
    }

    onRepairOffset(lastValidOffset);
  }, [currentItemCount, lastValidOffset, offset, onRepairOffset]);

  return (
    <div className="flex flex-col gap-3 bg-[var(--c-app-bg)] text-xs text-[var(--c-text-muted)] md:flex-row md:items-center md:justify-between">
      <div className="font-medium">
        <span>
          Showing {showingFrom}-{showingTo} of {total}
        </span>
      </div>

      <div className="flex flex-wrap items-center gap-2 md:justify-end">
        <label className="flex items-center gap-2">
          <span>Rows per page</span>
          <select
            aria-label="Rows per page"
            value={pageSize}
            onChange={(event) => onPageSizeChange(Number(event.target.value) || DEFAULT_PAGE_SIZE)}
            className="h-7 rounded border border-[var(--c-border)] bg-[var(--c-app-bg)] px-2 text-[11.5px] text-[var(--c-text-primary)] outline-none"
          >
            {pageSizeOptions.map((option) => (
              <option key={option} value={option}>
                {option}
              </option>
            ))}
          </select>
        </label>
        <div className="flex items-center gap-1">
          <button
            type="button"
            aria-label="Previous page"
            onClick={() => onOffsetChange(Math.max(0, currentOffset - pageSize))}
            disabled={!hasPreviousPage}
            className={buttonClassName(hasPreviousPage)}
          >
            <ChevronLeft className="h-3.5 w-3.5" />
          </button>
          <span>Page {currentPage} of {totalPages}</span>
          <button
            type="button"
            aria-label="Next page"
            onClick={() => onOffsetChange(currentOffset + pageSize)}
            disabled={!hasNextPage}
            className={buttonClassName(hasNextPage)}
          >
            <ChevronRight className="h-3.5 w-3.5" />
          </button>
        </div>
      </div>
    </div>
  );
}
