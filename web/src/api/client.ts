/**
 * API client for the Continua API.
 * Uses fetch with API key header from localStorage.
 */

import {
  buildCanonicalQueryString,
  type FetchTracesParams,
} from '../utils/tracesSearchParams';
import {
  buildSessionsQueryString,
  type FetchSessionsParams,
} from '../utils/sessionsSearchParams';

const API_KEY_STORAGE_KEY = 'continua_api_key';

export type { FetchTracesParams } from '../utils/tracesSearchParams';
export type {
  FetchSessionsParams,
  SessionSortBy,
  SortDirection,
} from '../utils/sessionsSearchParams';

/**
 * Get the stored API key.
 */
export function getApiKey(): string | null {
  return localStorage.getItem(API_KEY_STORAGE_KEY);
}

/**
 * Set the API key.
 */
export function setApiKey(key: string): void {
  localStorage.setItem(API_KEY_STORAGE_KEY, key);
}

/**
 * Clear the stored API key.
 */
export function clearApiKey(): void {
  localStorage.removeItem(API_KEY_STORAGE_KEY);
}

/**
 * Custom error class for API errors.
 */
export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

export function isAuthError(error: unknown): error is ApiError {
  return error instanceof ApiError && error.status === 401;
}

/**
 * Fetch wrapper with API key authentication.
 */
export async function fetchAPI<T>(
  path: string,
  options: RequestInit = {}
): Promise<T> {
  const apiKey = getApiKey();
  if (!apiKey) {
    throw new ApiError(401, 'missing_api_key', 'API key not configured');
  }

  const response = await fetch(path, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      'X-API-Key': apiKey,
      ...options.headers,
    },
  });

  if (!response.ok) {
    if (response.status === 401) {
      throw new ApiError(401, 'unauthorized', 'Invalid or missing API key');
    }
    if (response.status === 404) {
      throw new ApiError(404, 'not_found', 'Resource not found');
    }
    const error = await response.json().catch(() => ({ message: 'Unknown error' }));
    throw new ApiError(response.status, error.code || 'error', error.message || 'Request failed');
  }

  return response.json();
}

/**
 * Trace types from the API.
 */
export type JsonValue =
  | string
  | number
  | boolean
  | null
  | JsonValue[]
  | { [key: string]: JsonValue };

export interface Trace {
  id: string;
  session_id?: string;
  session_external_id?: string;
  name: string;
  status: 'RUNNING' | 'COMPLETED' | 'FAILED';
  started_at: string;
  ended_at?: string;
  total_tokens_in?: number;
  total_tokens_out?: number;
  total_cost_usd?: number;
  error_count?: number;
  metadata?: Record<string, unknown>;
}

export interface TraceDetail extends Trace {
  trace_id?: string;
  user_id?: string;
  tags?: string[];
  environment?: string;
  release?: string;
  input?: JsonValue;
  output?: JsonValue;
}

export interface TraceList {
  traces: Trace[];
  total: number;
}

export interface Span {
  id: string;
  trace_id: string;
  span_id: string; // External span ID for tree building
  parent_span_id?: string;
  name: string;
  kind: 'LLM' | 'TOOL' | 'CHAIN' | 'AGENT' | 'CUSTOM';
  status: 'SCHEDULED' | 'STARTED' | 'COMPLETED' | 'FAILED';
  started_at: string;
  ended_at?: string;
  tokens_in?: number;
  tokens_out?: number;
  cost_usd?: number;
  latency_ms?: number;
  error_message?: string;
  model?: string;
  provider?: string;
  input?: JsonValue;
  input_truncated?: boolean;
  input_original_size_bytes?: number;
  input_truncation_reason?: string;
  output?: JsonValue;
  output_truncated?: boolean;
  output_original_size_bytes?: number;
  output_truncation_reason?: string;
  metadata?: Record<string, unknown>;
}

export interface SpanList {
  spans: Span[];
}

export type TimelineTraceStatus = Trace['status'];

export interface TimelineEvent {
  id: string;
  trace_id: string;
  span_id?: string;
  span_name?: string;
  event_type:
    | 'log'
    | 'error'
    | 'exception'
    | 'message'
    | 'metric'
    | 'custom'
    | 'state_change'
    | 'decision'
    | 'effect'
    | 'wait'
    | 'span_started'
    | 'span_completed'
    | 'span_failed';
  timestamp: string;
  source: 'explicit' | 'synthetic';
  level?: 'debug' | 'info' | 'warning' | 'error';
  sequence?: number;
  message?: string;
  payload?: Record<string, unknown>;
}

export interface TimelineResponse {
  events: TimelineEvent[];
  trace_status: TimelineTraceStatus;
  has_more: boolean;
  next_cursor?: string;
  poll_cursor?: string;
}

export interface Session {
  id: string;
  external_id: string;
  name?: string;
  user_id?: string;
  trace_count?: number;
  metadata?: Record<string, unknown>;
  created_at: string;
}

export interface SessionList {
  sessions: Session[];
  total: number;
}

/**
 * Fetch traces with filters and pagination.
 */
export async function fetchTraces(params: FetchTracesParams = {}): Promise<TraceList> {
  const query = buildCanonicalQueryString(params);
  const path = query ? `/api/traces?${query}` : '/api/traces';
  return fetchAPI<TraceList>(path);
}

/**
 * Fetch a single trace by ID.
 */
export async function fetchTrace(id: string): Promise<TraceDetail> {
  return fetchAPI<TraceDetail>(`/api/traces/${id}`);
}

/**
 * Fetch spans for a trace.
 */
export async function fetchSpans(traceId: string): Promise<SpanList> {
  return fetchAPI<SpanList>(`/api/traces/${traceId}/spans`);
}

export interface FetchTimelineEventsOptions {
  after?: string;
  limit?: number;
}

/**
 * Fetch timeline events for a trace.
 */
export async function fetchTimelineEvents(
  traceId: string,
  options: FetchTimelineEventsOptions = {}
): Promise<TimelineResponse> {
  const params = new URLSearchParams();

  if (options.after) {
    params.set('after', options.after);
  }
  if (options.limit !== undefined) {
    params.set('limit', String(options.limit));
  }

  const query = params.size > 0 ? `?${params.toString()}` : '';
  return fetchAPI<TimelineResponse>(`/api/traces/${traceId}/events${query}`);
}

/**
 * Fetch sessions with filters and pagination.
 */
export async function fetchSessions(
  params: FetchSessionsParams = {}
): Promise<SessionList> {
  const query = buildSessionsQueryString(params);
  const path = query ? `/api/sessions?${query}` : '/api/sessions';
  return fetchAPI<SessionList>(path);
}

/**
 * Fetch a single session by ID.
 */
export async function fetchSession(id: string): Promise<Session> {
  return fetchAPI<Session>(`/api/sessions/${id}`);
}
