import type { operations } from '@continua/contracts/generated/typescript/api';

import { DEFAULT_PAGE_SIZE, normalizeUiPageSize } from './pagination';

export type TraceStatusFilter = 'running' | 'completed' | 'failed';
export type TraceSortBy = 'started_at';
export type SortDirection = 'asc' | 'desc';

type ListTracesQuery = NonNullable<
  operations['listTraces']['parameters']['query']
>;

export type EngineRunStatusFilter = NonNullable<
  ListTracesQuery['engine_run_status']
>;
export type EngineProjectionStateFilter = NonNullable<
  ListTracesQuery['engine_projection_state']
>;

function objectKeys<T extends object>(value: T): Array<Extract<keyof T, string>> {
  return Object.keys(value) as Array<Extract<keyof T, string>>;
}

// OpenAPI generation emits type unions, not runtime enum arrays. These maps are
// the runtime option source, while `satisfies` keeps them exhaustive.
const ENGINE_RUN_STATUS_LABELS = {
  queued: 'Queued',
  running: 'Running',
  waiting: 'Waiting',
  suspended: 'Suspended',
  completed: 'Completed',
  failed: 'Failed',
  cancelled: 'Cancelled',
  terminated: 'Terminated',
  continued_as_new: 'Continued as new',
} satisfies Record<EngineRunStatusFilter, string>;

export const ENGINE_RUN_STATUS_FILTER_VALUES = objectKeys(ENGINE_RUN_STATUS_LABELS);

const ENGINE_PROJECTION_STATE_LABELS = {
  up_to_date: 'Up to date',
  catching_up: 'Catching up',
  summary_only: 'Summary only',
  journal_expired: 'Journal expired',
} satisfies Record<EngineProjectionStateFilter, string>;

export const ENGINE_PROJECTION_STATE_FILTER_VALUES = objectKeys(
  ENGINE_PROJECTION_STATE_LABELS
);

export function formatEngineRunStatusLabel(value: EngineRunStatusFilter): string {
  return ENGINE_RUN_STATUS_LABELS[value];
}

export function formatEngineProjectionStateLabel(
  value: EngineProjectionStateFilter
): string {
  return ENGINE_PROJECTION_STATE_LABELS[value];
}

export interface FetchTracesParams {
  project_id?: string;
  limit?: number;
  offset?: number;
  session_id?: string;
  q?: string;
  sort_by?: TraceSortBy;
  sort_dir?: SortDirection;
  status?: TraceStatusFilter;
  start_time_from?: string;
  start_time_to?: string;
  user_id?: string;
  engine_run_id?: string;
  engine_instance_key?: string;
  engine_definition_name?: string;
  engine_definition_version?: string;
  engine_run_status?: EngineRunStatusFilter;
  engine_parent_run_id?: string;
  engine_root_run_id?: string;
  engine_child_key?: string;
  engine_child_depth?: number;
  engine_projection_state?: EngineProjectionStateFilter;
  has_errors?: boolean;
  min_duration_ms?: number;
}

export interface TracesFilterState {
  project_id?: string;
  limit: number;
  offset: number;
  session_id?: string;
  q?: string;
  sort_by?: TraceSortBy;
  sort_dir?: SortDirection;
  status?: TraceStatusFilter;
  start_time_from?: string;
  start_time_to?: string;
  user_id?: string;
  engine_run_id?: string;
  engine_instance_key?: string;
  engine_definition_name?: string;
  engine_definition_version?: string;
  engine_run_status?: EngineRunStatusFilter;
  engine_parent_run_id?: string;
  engine_root_run_id?: string;
  engine_child_key?: string;
  engine_child_depth?: number;
  engine_projection_state?: EngineProjectionStateFilter;
  has_errors?: boolean;
  min_duration_ms?: number;
}

export type ChipKey = Exclude<keyof TracesFilterState, 'offset'>;

export interface Chip {
  key: ChipKey;
  label: string;
  value?: string;
}

interface NormalizedTracesParams extends FetchTracesParams {
  offset: number;
}

type NormalizableTracesParams = {
  [Key in keyof FetchTracesParams]?: FetchTracesParams[Key] | string;
} & {
  offset?: number | string;
};

const VALID_STATUSES = new Set<TraceStatusFilter>(['running', 'completed', 'failed']);
const VALID_ENGINE_RUN_STATUSES = new Set<EngineRunStatusFilter>(
  ENGINE_RUN_STATUS_FILTER_VALUES
);
const VALID_ENGINE_PROJECTION_STATES = new Set<EngineProjectionStateFilter>(
  ENGINE_PROJECTION_STATE_FILTER_VALUES
);
const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const DATE_INPUT_PATTERN = /^\d{4}-\d{2}-\d{2}$/;
const ISO_DATE_TIME_PATTERN =
  /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/;

const CANONICAL_PARAM_ORDER: Array<keyof FetchTracesParams> = [
  'project_id',
  'limit',
  'offset',
  'session_id',
  'q',
  'sort_by',
  'sort_dir',
  'status',
  'start_time_from',
  'start_time_to',
  'user_id',
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
  'has_errors',
  'min_duration_ms',
];

function normalizeOptionalText(value: string | null | undefined): string | undefined {
  const trimmed = value?.trim();
  return trimmed ? trimmed : undefined;
}

function normalizeStatus(value: string | null | undefined): TraceStatusFilter | undefined {
  const candidate = value?.trim().toLowerCase();
  if (!candidate) {
    return undefined;
  }

  const normalized = candidate === 'error' ? 'failed' : candidate;
  return VALID_STATUSES.has(normalized as TraceStatusFilter)
    ? (normalized as TraceStatusFilter)
    : undefined;
}

function normalizeEngineRunStatus(
  value: string | null | undefined
): EngineRunStatusFilter | undefined {
  const candidate = value?.trim().toLowerCase();
  if (!candidate) {
    return undefined;
  }

  return VALID_ENGINE_RUN_STATUSES.has(candidate as EngineRunStatusFilter)
    ? (candidate as EngineRunStatusFilter)
    : undefined;
}

function normalizeEngineProjectionState(
  value: string | null | undefined
): EngineProjectionStateFilter | undefined {
  const candidate = value?.trim().toLowerCase();
  if (!candidate) {
    return undefined;
  }

  return VALID_ENGINE_PROJECTION_STATES.has(
    candidate as EngineProjectionStateFilter
  )
    ? (candidate as EngineProjectionStateFilter)
    : undefined;
}

function normalizeTraceSortBy(value: string | null | undefined): TraceSortBy | undefined {
  return value?.trim() === 'started_at' ? 'started_at' : undefined;
}

function normalizeSortDirection(
  value: string | null | undefined
): SortDirection | undefined {
  if (value === 'asc' || value === 'desc') {
    return value;
  }

  return undefined;
}

function normalizeUUID(value: string | null | undefined): string | undefined {
  const trimmed = value?.trim();
  if (!trimmed || !UUID_PATTERN.test(trimmed)) {
    return undefined;
  }

  return trimmed.toLowerCase();
}

function normalizeBooleanTrue(value: boolean | string | undefined): boolean | undefined {
  return value === true || value === 'true' ? true : undefined;
}

function normalizeNonNegativeInteger(value: number | string | undefined): number | undefined {
  if (value === undefined) {
    return undefined;
  }

  const parsed = typeof value === 'number' ? value : Number(value);
  if (!Number.isInteger(parsed) || parsed < 0) {
    return undefined;
  }

  return parsed;
}

function normalizePositiveInteger(value: number | string | undefined): number | undefined {
  const parsed = normalizeNonNegativeInteger(value);
  if (parsed === undefined || parsed === 0) {
    return undefined;
  }

  return parsed;
}

function normalizeISODateTime(value: string | null | undefined): string | undefined {
  const trimmed = value?.trim();
  if (!trimmed || !ISO_DATE_TIME_PATTERN.test(trimmed)) {
    return undefined;
  }

  const timestamp = new Date(trimmed);
  if (Number.isNaN(timestamp.getTime())) {
    return undefined;
  }

  return timestamp.toISOString();
}

function normalizeTracesParams(
  params: NormalizableTracesParams
): NormalizedTracesParams {
  return {
    project_id: normalizeUUID(params.project_id),
    limit:
      params.limit === undefined
        ? undefined
        : normalizeUiPageSize(params.limit),
    offset: normalizeNonNegativeInteger(params.offset) ?? 0,
    session_id: normalizeUUID(params.session_id),
    q: normalizeOptionalText(params.q),
    sort_by: normalizeTraceSortBy(params.sort_by),
    sort_dir: normalizeSortDirection(params.sort_dir),
    status: normalizeStatus(params.status),
    start_time_from: normalizeISODateTime(params.start_time_from),
    start_time_to: normalizeISODateTime(params.start_time_to),
    user_id: normalizeOptionalText(params.user_id),
    engine_run_id: normalizeUUID(params.engine_run_id),
    engine_instance_key: normalizeOptionalText(params.engine_instance_key),
    engine_definition_name: normalizeOptionalText(params.engine_definition_name),
    engine_definition_version: normalizeOptionalText(
      params.engine_definition_version
    ),
    engine_run_status: normalizeEngineRunStatus(params.engine_run_status),
    engine_parent_run_id: normalizeUUID(params.engine_parent_run_id),
    engine_root_run_id: normalizeUUID(params.engine_root_run_id),
    engine_child_key: normalizeOptionalText(params.engine_child_key),
    engine_child_depth: normalizeNonNegativeInteger(params.engine_child_depth),
    engine_projection_state: normalizeEngineProjectionState(
      params.engine_projection_state
    ),
    has_errors: normalizeBooleanTrue(params.has_errors),
    min_duration_ms: normalizePositiveInteger(params.min_duration_ms),
  };
}

function toQueryEntries(
  params: NormalizedTracesParams,
  options: { includeDefaultLimit: boolean }
): Array<[keyof FetchTracesParams, string]> {
  const entries: Array<[keyof FetchTracesParams, string]> = [];

  if (
    params.project_id
  ) {
    entries.push(['project_id', params.project_id]);
  }
  if (
    params.limit !== undefined &&
    (options.includeDefaultLimit || params.limit !== DEFAULT_PAGE_SIZE)
  ) {
    entries.push(['limit', String(params.limit)]);
  }
  if (params.offset > 0) {
    entries.push(['offset', String(params.offset)]);
  }
  if (params.session_id) {
    entries.push(['session_id', params.session_id]);
  }
  if (params.q) {
    entries.push(['q', params.q]);
  }
  if (params.sort_by) {
    entries.push(['sort_by', params.sort_by]);
  }
  if (params.sort_dir) {
    entries.push(['sort_dir', params.sort_dir]);
  }
  if (params.status) {
    entries.push(['status', params.status]);
  }
  if (params.start_time_from) {
    entries.push(['start_time_from', params.start_time_from]);
  }
  if (params.start_time_to) {
    entries.push(['start_time_to', params.start_time_to]);
  }
  if (params.user_id) {
    entries.push(['user_id', params.user_id]);
  }
  if (params.engine_run_id) {
    entries.push(['engine_run_id', params.engine_run_id]);
  }
  if (params.engine_instance_key) {
    entries.push(['engine_instance_key', params.engine_instance_key]);
  }
  if (params.engine_definition_name) {
    entries.push(['engine_definition_name', params.engine_definition_name]);
  }
  if (params.engine_definition_version) {
    entries.push(['engine_definition_version', params.engine_definition_version]);
  }
  if (params.engine_run_status) {
    entries.push(['engine_run_status', params.engine_run_status]);
  }
  if (params.engine_parent_run_id) {
    entries.push(['engine_parent_run_id', params.engine_parent_run_id]);
  }
  if (params.engine_root_run_id) {
    entries.push(['engine_root_run_id', params.engine_root_run_id]);
  }
  if (params.engine_child_key) {
    entries.push(['engine_child_key', params.engine_child_key]);
  }
  if (params.engine_child_depth !== undefined) {
    entries.push(['engine_child_depth', String(params.engine_child_depth)]);
  }
  if (params.engine_projection_state) {
    entries.push(['engine_projection_state', params.engine_projection_state]);
  }
  if (params.has_errors) {
    entries.push(['has_errors', 'true']);
  }
  if (params.min_duration_ms !== undefined) {
    entries.push(['min_duration_ms', String(params.min_duration_ms)]);
  }

  return entries.sort(
    ([left], [right]) =>
      CANONICAL_PARAM_ORDER.indexOf(left) - CANONICAL_PARAM_ORDER.indexOf(right)
  );
}

function resetOffset(state: TracesFilterState): TracesFilterState {
  return { ...state, offset: 0 };
}

function parseLocalDate(date: string): Date | null {
  if (!DATE_INPUT_PATTERN.test(date)) {
    return null;
  }

  const [yearString, monthString, dayString] = date.split('-');
  const year = Number(yearString);
  const month = Number(monthString);
  const day = Number(dayString);
  const parsed = new Date(year, month - 1, day);

  if (
    parsed.getFullYear() !== year ||
    parsed.getMonth() !== month - 1 ||
    parsed.getDate() !== day
  ) {
    return null;
  }

  return parsed;
}

export function parseTracesParams(searchParams: URLSearchParams): TracesFilterState {
  const normalized = normalizeTracesParams({
    project_id: searchParams.get('project_id') ?? undefined,
    limit: searchParams.get('limit') ?? undefined,
    offset: searchParams.get('offset') ?? undefined,
    session_id: searchParams.get('session_id') ?? undefined,
    q: searchParams.get('q') ?? undefined,
    sort_by: searchParams.get('sort_by') ?? undefined,
    sort_dir: searchParams.get('sort_dir') ?? undefined,
    status: searchParams.get('status') ?? undefined,
    start_time_from: searchParams.get('start_time_from') ?? undefined,
    start_time_to: searchParams.get('start_time_to') ?? undefined,
    user_id: searchParams.get('user_id') ?? undefined,
    engine_run_id: searchParams.get('engine_run_id') ?? undefined,
    engine_instance_key: searchParams.get('engine_instance_key') ?? undefined,
    engine_definition_name:
      searchParams.get('engine_definition_name') ?? undefined,
    engine_definition_version:
      searchParams.get('engine_definition_version') ?? undefined,
    engine_run_status: searchParams.get('engine_run_status') ?? undefined,
    engine_parent_run_id:
      searchParams.get('engine_parent_run_id') ?? undefined,
    engine_root_run_id: searchParams.get('engine_root_run_id') ?? undefined,
    engine_child_key: searchParams.get('engine_child_key') ?? undefined,
    engine_child_depth: searchParams.get('engine_child_depth') ?? undefined,
    engine_projection_state:
      searchParams.get('engine_projection_state') ?? undefined,
    has_errors: searchParams.get('has_errors') ?? undefined,
    min_duration_ms: searchParams.get('min_duration_ms') ?? undefined,
  });

  return {
    limit: normalized.limit ?? DEFAULT_PAGE_SIZE,
    project_id: normalized.project_id,
    offset: normalized.offset,
    session_id: normalized.session_id,
    q: normalized.q,
    sort_by: normalized.sort_by,
    sort_dir: normalized.sort_dir,
    status: normalized.status,
    start_time_from: normalized.start_time_from,
    start_time_to: normalized.start_time_to,
    user_id: normalized.user_id,
    engine_run_id: normalized.engine_run_id,
    engine_instance_key: normalized.engine_instance_key,
    engine_definition_name: normalized.engine_definition_name,
    engine_definition_version: normalized.engine_definition_version,
    engine_run_status: normalized.engine_run_status,
    engine_parent_run_id: normalized.engine_parent_run_id,
    engine_root_run_id: normalized.engine_root_run_id,
    engine_child_key: normalized.engine_child_key,
    engine_child_depth: normalized.engine_child_depth,
    engine_projection_state: normalized.engine_projection_state,
    has_errors: normalized.has_errors,
    min_duration_ms: normalized.min_duration_ms,
  };
}

export function serializeTracesParams(state: TracesFilterState): URLSearchParams {
  const normalized = normalizeTracesParams(state);
  const params = new URLSearchParams();

  toQueryEntries(normalized, { includeDefaultLimit: false }).forEach(([key, value]) => {
    params.set(key, value);
  });

  return params;
}

export function buildCanonicalQueryString(
  params: FetchTracesParams | TracesFilterState
): string {
  const normalized = normalizeTracesParams(params);
  const query = new URLSearchParams();

  toQueryEntries(normalized, { includeDefaultLimit: normalized.limit !== undefined }).forEach(([key, value]) => {
    query.set(key, value);
  });

  return query.toString();
}

export function localDateToISOStart(date: string): string {
  const parsed = parseLocalDate(date);
  if (!parsed) {
    throw new Error(`Invalid local date: ${date}`);
  }

  parsed.setHours(0, 0, 0, 0);
  return parsed.toISOString();
}

export function localDateToISOEnd(date: string): string {
  const parsed = parseLocalDate(date);
  if (!parsed) {
    throw new Error(`Invalid local date: ${date}`);
  }

  parsed.setHours(23, 59, 59, 999);
  return parsed.toISOString();
}

export function isoToLocalDateInputValue(value?: string): string {
  if (!value) {
    return '';
  }

  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return '';
  }

  const year = parsed.getFullYear();
  const month = String(parsed.getMonth() + 1).padStart(2, '0');
  const day = String(parsed.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

export function deriveActiveChips(state: TracesFilterState): Chip[] {
  const normalized = normalizeTracesParams(state);
  const chips: Chip[] = [];

  if (normalized.q) {
    chips.push({ key: 'q', label: 'Search', value: normalized.q });
  }
  if (normalized.status) {
    chips.push({ key: 'status', label: 'Status', value: normalized.status });
  }
  if (normalized.start_time_from) {
    chips.push({
      key: 'start_time_from',
      label: 'From',
      value: isoToLocalDateInputValue(normalized.start_time_from),
    });
  }
  if (normalized.start_time_to) {
    chips.push({
      key: 'start_time_to',
      label: 'To',
      value: isoToLocalDateInputValue(normalized.start_time_to),
    });
  }
  if (normalized.user_id) {
    chips.push({ key: 'user_id', label: 'User', value: normalized.user_id });
  }
  if (normalized.engine_run_id) {
    chips.push({
      key: 'engine_run_id',
      label: 'Engine run',
      value: normalized.engine_run_id,
    });
  }
  if (normalized.engine_instance_key) {
    chips.push({
      key: 'engine_instance_key',
      label: 'Engine instance',
      value: normalized.engine_instance_key,
    });
  }
  if (normalized.engine_definition_name) {
    chips.push({
      key: 'engine_definition_name',
      label: 'Engine definition',
      value: normalized.engine_definition_name,
    });
  }
  if (normalized.engine_definition_version) {
    chips.push({
      key: 'engine_definition_version',
      label: 'Engine version',
      value: normalized.engine_definition_version,
    });
  }
  if (normalized.engine_run_status) {
    chips.push({
      key: 'engine_run_status',
      label: 'Engine status',
      value: formatEngineRunStatusLabel(normalized.engine_run_status),
    });
  }
  if (normalized.engine_parent_run_id) {
    chips.push({
      key: 'engine_parent_run_id',
      label: 'Parent run',
      value: normalized.engine_parent_run_id,
    });
  }
  if (normalized.engine_root_run_id) {
    chips.push({
      key: 'engine_root_run_id',
      label: 'Root run',
      value: normalized.engine_root_run_id,
    });
  }
  if (normalized.engine_child_key) {
    chips.push({
      key: 'engine_child_key',
      label: 'Child key',
      value: normalized.engine_child_key,
    });
  }
  if (normalized.engine_child_depth !== undefined) {
    chips.push({
      key: 'engine_child_depth',
      label: 'Child depth',
      value: String(normalized.engine_child_depth),
    });
  }
  if (normalized.engine_projection_state) {
    chips.push({
      key: 'engine_projection_state',
      label: 'Projection state',
      value: formatEngineProjectionStateLabel(
        normalized.engine_projection_state
      ),
    });
  }
  if (normalized.has_errors) {
    chips.push({ key: 'has_errors', label: 'Errors', value: 'Only traces with errors' });
  }
  if (normalized.min_duration_ms !== undefined) {
    chips.push({
      key: 'min_duration_ms',
      label: 'Min duration',
      value: `${normalized.min_duration_ms} ms`,
    });
  }
  if (normalized.session_id) {
    chips.push({ key: 'session_id', label: 'Session', value: normalized.session_id });
  }

  return chips;
}

export function clearChip(
  state: TracesFilterState,
  chipKey: ChipKey
): TracesFilterState {
  const normalized = parseTracesParams(serializeTracesParams(state));

  switch (chipKey) {
    case 'q':
      return resetOffset({ ...normalized, q: undefined });
    case 'status':
      return resetOffset({ ...normalized, status: undefined });
    case 'start_time_from':
      return resetOffset({ ...normalized, start_time_from: undefined });
    case 'start_time_to':
      return resetOffset({ ...normalized, start_time_to: undefined });
    case 'user_id':
      return resetOffset({ ...normalized, user_id: undefined });
    case 'engine_run_id':
      return resetOffset({ ...normalized, engine_run_id: undefined });
    case 'engine_instance_key':
      return resetOffset({ ...normalized, engine_instance_key: undefined });
    case 'engine_definition_name':
      return resetOffset({ ...normalized, engine_definition_name: undefined });
    case 'engine_definition_version':
      return resetOffset({
        ...normalized,
        engine_definition_version: undefined,
      });
    case 'engine_run_status':
      return resetOffset({ ...normalized, engine_run_status: undefined });
    case 'engine_parent_run_id':
      return resetOffset({ ...normalized, engine_parent_run_id: undefined });
    case 'engine_root_run_id':
      return resetOffset({ ...normalized, engine_root_run_id: undefined });
    case 'engine_child_key':
      return resetOffset({ ...normalized, engine_child_key: undefined });
    case 'engine_child_depth':
      return resetOffset({ ...normalized, engine_child_depth: undefined });
    case 'engine_projection_state':
      return resetOffset({
        ...normalized,
        engine_projection_state: undefined,
      });
    case 'has_errors':
      return resetOffset({ ...normalized, has_errors: undefined });
    case 'min_duration_ms':
      return resetOffset({ ...normalized, min_duration_ms: undefined });
    case 'session_id':
      return resetOffset({ ...normalized, session_id: undefined });
    default:
      return normalized;
  }
}
