import { useEffect } from 'react';
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
    ? 'rounded-lg bg-blue-600 px-3 py-2 text-sm font-medium text-white transition hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-200'
    : 'cursor-not-allowed rounded-lg bg-gray-200 px-3 py-2 text-sm font-medium text-gray-400';
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
    <div className="mt-4 flex flex-col gap-3 rounded-xl border border-gray-200 bg-white px-4 py-3 shadow-sm md:flex-row md:items-center md:justify-between">
      <div className="flex flex-wrap items-center gap-2">
        <button
          type="button"
          aria-label="First page"
          onClick={() => onOffsetChange(0)}
          disabled={!hasPreviousPage}
          className={buttonClassName(hasPreviousPage)}
        >
          First
        </button>
        <button
          type="button"
          aria-label="Previous page"
          onClick={() => onOffsetChange(Math.max(0, currentOffset - pageSize))}
          disabled={!hasPreviousPage}
          className={buttonClassName(hasPreviousPage)}
        >
          Previous
        </button>
        <button
          type="button"
          aria-label="Next page"
          onClick={() => onOffsetChange(currentOffset + pageSize)}
          disabled={!hasNextPage}
          className={buttonClassName(hasNextPage)}
        >
          Next
        </button>
        <button
          type="button"
          aria-label="Last page"
          onClick={() => onOffsetChange(lastValidOffset)}
          disabled={!hasNextPage}
          className={buttonClassName(hasNextPage)}
        >
          Last
        </button>
      </div>

      <div className="flex flex-col gap-2 text-sm text-gray-600 md:items-end">
        <span>
          Showing {showingFrom}-{showingTo} of {total}
        </span>
        <div className="flex flex-wrap items-center gap-3">
          <span>Page {currentPage} of {totalPages}</span>
          <label className="flex items-center gap-2">
            <span>Rows</span>
            <select
              aria-label="Rows per page"
              value={pageSize}
              onChange={(event) => onPageSizeChange(Number(event.target.value) || DEFAULT_PAGE_SIZE)}
              className="rounded-lg border border-gray-300 bg-white px-2 py-1.5 text-sm text-gray-900 shadow-sm transition focus:border-blue-500 focus:outline-none focus:ring-2 focus:ring-blue-200"
            >
              {pageSizeOptions.map((option) => (
                <option key={option} value={option}>
                  {option}
                </option>
              ))}
            </select>
          </label>
        </div>
      </div>
    </div>
  );
}
