import { DEFAULT_PAGE_SIZE, normalizeUiPageSize } from './pagination';

export type TraceStatusFilter = 'running' | 'completed' | 'failed';
export type TraceSortBy = 'started_at';
export type SortDirection = 'asc' | 'desc';

export interface FetchTracesParams {
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
  has_errors?: boolean;
  min_duration_ms?: number;
}

export interface TracesFilterState {
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
const UUID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const DATE_INPUT_PATTERN = /^\d{4}-\d{2}-\d{2}$/;
const ISO_DATE_TIME_PATTERN =
  /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/;

const CANONICAL_PARAM_ORDER: Array<keyof FetchTracesParams> = [
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
    has_errors: searchParams.get('has_errors') ?? undefined,
    min_duration_ms: searchParams.get('min_duration_ms') ?? undefined,
  });

  return {
    limit: normalized.limit ?? DEFAULT_PAGE_SIZE,
    offset: normalized.offset,
    session_id: normalized.session_id,
    q: normalized.q,
    sort_by: normalized.sort_by,
    sort_dir: normalized.sort_dir,
    status: normalized.status,
    start_time_from: normalized.start_time_from,
    start_time_to: normalized.start_time_to,
    user_id: normalized.user_id,
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
