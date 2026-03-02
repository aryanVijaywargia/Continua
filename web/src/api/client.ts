/**
 * API client for the Continua API.
 * Uses fetch with API key header from localStorage.
 */

const API_KEY_STORAGE_KEY = 'continua_api_key';

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
export interface Trace {
  id: string;
  session_id?: string;
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
  input?: Record<string, unknown>;
  output?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

export interface SpanList {
  spans: Span[];
}

export interface Session {
  id: string;
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
 * Fetch traces with pagination.
 */
export async function fetchTraces(limit = 20, offset = 0): Promise<TraceList> {
  return fetchAPI<TraceList>(`/api/traces?limit=${limit}&offset=${offset}`);
}

/**
 * Fetch a single trace by ID.
 */
export async function fetchTrace(id: string): Promise<Trace> {
  return fetchAPI<Trace>(`/api/traces/${id}`);
}

/**
 * Fetch spans for a trace.
 */
export async function fetchSpans(traceId: string): Promise<SpanList> {
  return fetchAPI<SpanList>(`/api/traces/${traceId}/spans`);
}

/**
 * Fetch sessions with pagination.
 */
export async function fetchSessions(limit = 20, offset = 0): Promise<SessionList> {
  return fetchAPI<SessionList>(`/api/sessions?limit=${limit}&offset=${offset}`);
}

/**
 * Fetch a single session by ID.
 */
export async function fetchSession(id: string): Promise<Session> {
  return fetchAPI<Session>(`/api/sessions/${id}`);
}

/**
 * Fetch traces filtered by session ID.
 */
export async function fetchTracesBySession(
  sessionId: string,
  limit = 20,
  offset = 0
): Promise<TraceList> {
  return fetchAPI<TraceList>(
    `/api/traces?session_id=${sessionId}&limit=${limit}&offset=${offset}`
  );
}
