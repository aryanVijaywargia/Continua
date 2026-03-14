/**
 * Formatting utilities.
 */

/**
 * Format a duration in milliseconds to a human-readable string.
 */
export function formatDuration(ms: number | undefined | null): string {
  if (ms === undefined || ms === null) return '-';
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  if (ms < 3600000) return `${(ms / 60000).toFixed(1)}m`;
  return `${(ms / 3600000).toFixed(1)}h`;
}

/**
 * Format a token count with K/M suffix.
 */
export function formatTokens(count: number | undefined | null): string {
  if (count === undefined || count === null) return '-';
  if (count < 1000) return count.toString();
  if (count < 1000000) return `${(count / 1000).toFixed(1)}K`;
  return `${(count / 1000000).toFixed(1)}M`;
}

/**
 * Format a cost in USD.
 */
export function formatCost(amount: number | undefined | null): string {
  if (amount === undefined || amount === null) return '-';
  if (amount < 0.01) return `$${amount.toFixed(4)}`;
  return `$${amount.toFixed(2)}`;
}

/**
 * Format a date to relative time (e.g., "2m ago").
 */
export function formatRelativeTime(dateStr: string | undefined | null): string {
  if (!dateStr) return '-';
  const date = new Date(dateStr);
  const now = new Date();
  const diffMs = now.getTime() - date.getTime();
  const diffSec = Math.floor(diffMs / 1000);
  const diffMin = Math.floor(diffSec / 60);
  const diffHour = Math.floor(diffMin / 60);
  const diffDay = Math.floor(diffHour / 24);

  if (diffSec < 60) return 'just now';
  if (diffMin < 60) return `${diffMin}m ago`;
  if (diffHour < 24) return `${diffHour}h ago`;
  if (diffDay < 7) return `${diffDay}d ago`;
  return date.toLocaleDateString();
}

/**
 * Format a timestamp as an absolute ISO string.
 */
export function formatTimestamp(dateStr: string | undefined | null): string {
  if (!dateStr) return '-';
  return new Date(dateStr).toISOString();
}

/**
 * Calculate duration from start and end times.
 */
export function calculateDuration(
  startedAt: string | undefined,
  endedAt: string | undefined | null
): number | null {
  if (!startedAt) return null;
  const start = new Date(startedAt).getTime();
  const end = endedAt ? new Date(endedAt).getTime() : Date.now();
  return end - start;
}
