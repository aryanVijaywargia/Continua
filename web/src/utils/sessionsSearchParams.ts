import { DEFAULT_PAGE_SIZE, normalizeUiPageSize } from './pagination';

export type SessionSortBy = 'created_at' | 'trace_count';
export type SortDirection = 'asc' | 'desc';

export interface FetchSessionsParams {
  project_id?: string;
  limit?: number;
  offset?: number;
  q?: string;
  user_id?: string;
  sort_by?: SessionSortBy;
  sort_dir?: SortDirection;
}

export interface SessionsSearchState {
  project_id?: string;
  limit: number;
  offset: number;
  q?: string;
  user_id?: string;
  sort_by?: SessionSortBy;
  sort_dir?: SortDirection;
}

type NormalizedSessionsParams = FetchSessionsParams & {
  offset: number;
};

type NormalizableSessionsParams = {
  [Key in keyof FetchSessionsParams]?: FetchSessionsParams[Key] | string;
} & {
  offset?: number | string;
};

const CANONICAL_PARAM_ORDER: Array<keyof FetchSessionsParams> = [
  'project_id',
  'limit',
  'offset',
  'q',
  'user_id',
  'sort_by',
  'sort_dir',
];

function normalizeOptionalText(value: string | null | undefined): string | undefined {
  const trimmed = value?.trim();
  return trimmed ? trimmed : undefined;
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

function normalizeSessionSortBy(value: string | null | undefined): SessionSortBy | undefined {
  return value === 'created_at' || value === 'trace_count' ? value : undefined;
}

function normalizeSortDirection(
  value: string | null | undefined
): SortDirection | undefined {
  return value === 'asc' || value === 'desc' ? value : undefined;
}

function normalizeUUID(value: string | null | undefined): string | undefined {
  const trimmed = value?.trim();
  if (!trimmed) {
    return undefined;
  }

  const uuidPattern =
    /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
  return uuidPattern.test(trimmed) ? trimmed.toLowerCase() : undefined;
}

function normalizeSessionsParams(
  params: NormalizableSessionsParams
): NormalizedSessionsParams {
  return {
    project_id: normalizeUUID(params.project_id),
    limit:
      params.limit === undefined
        ? undefined
        : normalizeUiPageSize(params.limit),
    offset: normalizeNonNegativeInteger(params.offset) ?? 0,
    q: normalizeOptionalText(params.q),
    user_id: normalizeOptionalText(params.user_id),
    sort_by: normalizeSessionSortBy(params.sort_by),
    sort_dir: normalizeSortDirection(params.sort_dir),
  };
}

function toQueryEntries(
  params: NormalizedSessionsParams,
  options: { includeDefaultLimit: boolean }
): Array<[keyof FetchSessionsParams, string]> {
  const entries: Array<[keyof FetchSessionsParams, string]> = [];

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
  if (params.q) {
    entries.push(['q', params.q]);
  }
  if (params.user_id) {
    entries.push(['user_id', params.user_id]);
  }
  if (params.sort_by) {
    entries.push(['sort_by', params.sort_by]);
  }
  if (params.sort_dir) {
    entries.push(['sort_dir', params.sort_dir]);
  }

  return entries.sort(
    ([left], [right]) =>
      CANONICAL_PARAM_ORDER.indexOf(left) - CANONICAL_PARAM_ORDER.indexOf(right)
  );
}

export function parseSessionsParams(searchParams: URLSearchParams): SessionsSearchState {
  const normalized = normalizeSessionsParams({
    project_id: searchParams.get('project_id') ?? undefined,
    limit: searchParams.get('limit') ?? undefined,
    offset: searchParams.get('offset') ?? undefined,
    q: searchParams.get('q') ?? undefined,
    user_id: searchParams.get('user_id') ?? undefined,
    sort_by: searchParams.get('sort_by') ?? undefined,
    sort_dir: searchParams.get('sort_dir') ?? undefined,
  });

  return {
    limit: normalized.limit ?? DEFAULT_PAGE_SIZE,
    project_id: normalized.project_id,
    offset: normalized.offset,
    q: normalized.q,
    user_id: normalized.user_id,
    sort_by: normalized.sort_by,
    sort_dir: normalized.sort_dir,
  };
}

export function serializeSessionsParams(state: SessionsSearchState): URLSearchParams {
  const normalized = normalizeSessionsParams(state);
  const params = new URLSearchParams();

  toQueryEntries(normalized, { includeDefaultLimit: false }).forEach(([key, value]) => {
    params.set(key, value);
  });

  return params;
}

export function buildSessionsQueryString(
  params: FetchSessionsParams | SessionsSearchState
): string {
  const normalized = normalizeSessionsParams(params);
  const query = new URLSearchParams();

  toQueryEntries(normalized, { includeDefaultLimit: normalized.limit !== undefined }).forEach(
    ([key, value]) => {
      query.set(key, value);
    }
  );

  return query.toString();
}
