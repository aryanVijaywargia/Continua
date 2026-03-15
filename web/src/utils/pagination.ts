export const PAGE_SIZE_OPTIONS = [20, 50, 100] as const;
export const DEFAULT_PAGE_SIZE = PAGE_SIZE_OPTIONS[0];

export function normalizeUiPageSize(
  value: number | string | undefined,
  fallback: number = DEFAULT_PAGE_SIZE
): number {
  if (value === undefined) {
    return fallback;
  }

  const parsed = typeof value === 'number' ? value : Number(value);
  if (!Number.isInteger(parsed) || parsed <= 0) {
    return fallback;
  }

  let normalized: number = fallback;
  for (const option of PAGE_SIZE_OPTIONS) {
    if (parsed >= option) {
      normalized = option;
    }
  }

  return normalized;
}

export function getLastValidOffset(total: number, pageSize: number): number {
  if (total <= 0) {
    return 0;
  }

  return Math.floor((total - 1) / pageSize) * pageSize;
}
