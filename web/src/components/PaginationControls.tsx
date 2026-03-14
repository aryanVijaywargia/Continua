interface PaginationControlsProps {
  offset: number;
  pageSize: number;
  total: number;
  onOffsetChange: (offset: number) => void;
}

export function PaginationControls({
  offset,
  pageSize,
  total,
  onOffsetChange,
}: PaginationControlsProps) {
  const hasNextPage = offset + pageSize < total;
  const hasPrevPage = offset > 0;
  const showingFrom = total === 0 ? 0 : offset + 1;
  const showingTo = Math.min(offset + pageSize, total);

  return (
    <div className="mt-4 flex items-center justify-between">
      <button
        type="button"
        aria-label="Previous page"
        onClick={() => onOffsetChange(Math.max(0, offset - pageSize))}
        disabled={!hasPrevPage}
        className={`px-4 py-2 rounded-lg ${
          hasPrevPage
            ? 'bg-blue-600 text-white hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-200'
            : 'bg-gray-200 text-gray-400 cursor-not-allowed'
        }`}
      >
        Previous
      </button>
      <span className="text-sm text-gray-600">
        Showing {showingFrom} - {showingTo} of {total}
      </span>
      <button
        type="button"
        aria-label="Next page"
        onClick={() => onOffsetChange(offset + pageSize)}
        disabled={!hasNextPage}
        className={`px-4 py-2 rounded-lg ${
          hasNextPage
            ? 'bg-blue-600 text-white hover:bg-blue-700 focus:outline-none focus:ring-2 focus:ring-blue-200'
            : 'bg-gray-200 text-gray-400 cursor-not-allowed'
        }`}
      >
        Next
      </button>
    </div>
  );
}
