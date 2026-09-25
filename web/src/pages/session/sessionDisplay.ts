/**
 * Display helpers shared by the sessions list, session detail, and session
 * compare pages. They format recorded values; they never invent missing ones.
 */

/** First eight characters of an ID, the way the console abbreviates IDs. */
export function shortId(id: string | null | undefined): string {
  if (!id) {
    return '—';
  }
  return id.slice(0, 8);
}

/** Local wall-clock time with milliseconds, e.g. "02:05:24.889". */
export function formatClockTime(dateStr: string | null | undefined): string {
  if (!dateStr) {
    return '—';
  }
  const date = new Date(dateStr);
  if (Number.isNaN(date.getTime())) {
    return '—';
  }
  const pad = (value: number, width = 2) => String(value).padStart(width, '0');
  return `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}.${pad(
    date.getMilliseconds(),
    3
  )}`;
}

/** Signed millisecond delta, e.g. "+261 ms", "-1.25s", "0 ms". */
export function formatSignedMs(deltaMs: number): string {
  if (!Number.isFinite(deltaMs) || deltaMs === 0) {
    return '0 ms';
  }
  const sign = deltaMs > 0 ? '+' : '-';
  const abs = Math.abs(deltaMs);
  if (abs < 1000) {
    return `${sign}${Math.round(abs)} ms`;
  }
  return `${sign}${(abs / 1000).toFixed(2)}s`;
}

/** Signed percentage of a delta against its base, or null when the base is not usable. */
export function formatSignedPercent(deltaMs: number, baseMs: number | null | undefined): string | null {
  if (baseMs == null || !Number.isFinite(baseMs) || baseMs <= 0 || !Number.isFinite(deltaMs)) {
    return null;
  }
  const percent = (deltaMs / baseMs) * 100;
  const rounded = Math.round(percent * 10) / 10;
  if (rounded === 0) {
    return '0%';
  }
  return `${rounded > 0 ? '+' : ''}${rounded.toFixed(1)}%`;
}

/**
 * True when every usage value is missing or zero. A zero token or cost
 * field can be a default, so the console cannot treat it as a measured zero.
 */
export function isUsageUnverified(values: Array<number | null | undefined>): boolean {
  return values.every((value) => value == null || value === 0);
}
