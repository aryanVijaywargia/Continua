import type { TracesFilterState } from '../../utils/tracesSearchParams';

/**
 * Exact local start time with seconds, e.g. "Aug 15, 02:05:34". The traces
 * list needs seconds so runs that start minutes apart stay distinguishable.
 */
export function formatExactTimeWithSeconds(dateStr: string | undefined | null): string {
  if (!dateStr) return '—';
  const date = new Date(dateStr);
  if (Number.isNaN(date.getTime())) return '—';
  return date.toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  });
}

export function shortTraceId(id: string): string {
  return id.slice(0, 8);
}

/** Filter keys that live behind the Advanced control. */
export const ADVANCED_FILTER_KEYS = [
  'user_id',
  'min_duration_ms',
  'has_errors',
  'engine_run_id',
  'engine_instance_key',
  'engine_definition_name',
  'engine_definition_version',
  'engine_run_status',
  'engine_parent_run_id',
  'engine_root_run_id',
  'engine_child_key',
  'engine_child_depth',
  'engine_projection_state',
  'engine_only',
] as const satisfies ReadonlyArray<keyof TracesFilterState>;

export function countAdvancedFilters(filters: TracesFilterState): number {
  return ADVANCED_FILTER_KEYS.filter((key) => {
    const value = filters[key];
    return value !== undefined && value !== '' && value !== false;
  }).length;
}
