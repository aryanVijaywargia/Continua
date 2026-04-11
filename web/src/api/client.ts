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
const API_KEY_EVENT_NAME = 'continua:api-key-change';
const ENGINE_PREVIEW_HEADER = 'X-Continua-Engine-Preview';
const ENGINE_PREVIEW_HEADER_VALUE = '1';

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
  dispatchApiKeyChange();
}

/**
 * Clear the stored API key.
 */
export function clearApiKey(): void {
  localStorage.removeItem(API_KEY_STORAGE_KEY);
  dispatchApiKeyChange();
}

/**
 * Custom error class for API errors.
 */
export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
    public detail?: unknown
  ) {
    super(message);
    this.name = 'ApiError';
  }
}

export function isAuthError(error: unknown): error is ApiError {
  return error instanceof ApiError && error.status === 401;
}

export interface ComparisonTooLargeErrorDetail {
  baseline_span_count: number;
  candidate_span_count: number;
  baseline_semantic_count: number;
  candidate_semantic_count: number;
  max_spans: number;
  max_semantic_events: number;
}

export function isComparisonTooLargeError(
  error: unknown
): error is ApiError & { detail: ComparisonTooLargeErrorDetail } {
  return (
    error instanceof ApiError &&
    error.status === 422 &&
    error.code === 'comparison_too_large' &&
    typeof error.detail === 'object' &&
    error.detail !== null
  );
}

function dispatchApiKeyChange() {
  if (typeof window === 'undefined') {
    return;
  }

  window.dispatchEvent(new Event(API_KEY_EVENT_NAME));
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
    throw new ApiError(
      response.status,
      error.code || 'error',
      error.message || 'Request failed',
      error.detail
    );
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

export type EngineProjectionState =
  | 'up_to_date'
  | 'catching_up'
  | 'summary_only'
  | 'journal_expired';

export type EngineRunStatus =
  | 'QUEUED'
  | 'RUNNING'
  | 'WAITING'
  | 'SUSPENDED'
  | 'COMPLETED'
  | 'FAILED'
  | 'CANCELLED'
  | 'TERMINATED'
  | 'CONTINUED_AS_NEW';

export type EngineRepairReason =
  | 'already_up_to_date'
  | 'history_expired'
  | 'no_events_to_project'
  | 'repair_requested'
  | 'already_catching_up';

export type EnginePurgeMode = 'projection_only' | 'full';

export interface EngineTraceInfo {
  run_id: string;
  definition_name: string;
  definition_version: string;
  projection_state: EngineProjectionState;
}

export interface EngineWaitState {
  kind?: string;
  activity_key?: string;
  activity_type?: string;
  due_at?: string;
  signal_name?: string;
  timer_key?: string;
  [key: string]: unknown;
}

export interface EnginePendingWork {
  pending_activity_tasks: number;
  pending_inbox_items: number;
}

export interface EnginePendingActivityItem {
  task_id: string;
  activity_key: string;
  activity_type: string;
  status: string;
  available_at: string;
  attempt_count: number;
}

export interface EnginePendingTimerItem {
  inbox_id: string;
  timer_key: string;
  status: string;
  available_at: string;
}

export interface EnginePendingSignalItem {
  inbox_id: string;
  signal_name: string;
  status: string;
  available_at: string;
}

export interface EnginePendingWorkResponse {
  run_id: string;
  current_wait: EngineWaitState | null;
  activities: EnginePendingActivityItem[];
  timers: EnginePendingTimerItem[];
  signals: EnginePendingSignalItem[];
  pending_activity_tasks: number;
  pending_inbox_items: number;
}

export interface EngineFailureSummary {
  error_code: string;
  error_message: string;
  status: string;
}

export interface EngineRunSummary {
  run_id: string;
  instance_key: string;
  continued_from_run_id?: string;
  continued_to_run_id?: string;
  continued_from_trace_id?: string;
  continued_to_trace_id?: string;
  definition_name: string;
  definition_version: string;
  projection_state: EngineProjectionState;
  status: EngineRunStatus;
  created_at: string;
  updated_at: string;
  completed_at?: string;
  custom_status?: Record<string, unknown>;
  pending_work: EnginePendingWork;
  result?: JsonValue;
  failure?: EngineFailureSummary;
  wait_state?: EngineWaitState;
}

export interface EngineRunResponse extends EngineRunSummary {
  instance_id: string;
}

export interface EngineRunResultResponse {
  run_id: string;
  continued_from_run_id?: string;
  continued_to_run_id?: string;
  continued_from_trace_id?: string;
  continued_to_trace_id?: string;
  status: EngineRunStatus;
  result: JsonValue | null;
  failure?: EngineFailureSummary;
}

export interface EngineControlResponse {
  run_id: string;
  instance_key: string;
  accepted: boolean;
  wake_applied: boolean;
}

export interface EngineSignalRunRequest {
  signal_name: string;
  payload?: JsonValue;
}

export interface EnginePurgeResponse {
  run_id: string;
  mode: EnginePurgeMode;
  projection_state: EngineProjectionState;
  deleted: boolean;
}

export interface EngineRepairResponse {
  run_id: string;
  accepted: boolean;
  reason: EngineRepairReason;
  projection_state: EngineProjectionState;
}

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
  engine?: EngineTraceInfo;
}

export interface TraceDetail extends Trace {
  trace_id?: string;
  user_id?: string;
  tags?: string[];
  environment?: string;
  release?: string;
  input?: JsonValue;
  output?: JsonValue;
  engine?: EngineRunSummary;
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
    | 'snapshot_marker'
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
  engine?: {
    projection_state: EngineProjectionState;
  };
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

export interface SessionNarrativeLineage {
  type: 'explicit' | 'inferred' | 'unlinked';
  parent_trace_id?: string;
  trigger_span_id?: string;
  link_kind?: string;
}

export interface SessionNarrativeSummary {
  total_trace_count: number;
  returned_trace_count: number;
  truncated: boolean;
  running_trace_count: number;
  completed_trace_count: number;
  failed_trace_count: number;
  total_cost_usd: number;
  total_tokens_in: number;
  total_tokens_out: number;
  started_at: string | null;
  last_activity_at: string | null;
  explicit_link_count: number;
  inferred_link_count: number;
  unlinked_trace_count: number;
}

export interface SessionNarrativeTrace {
  id: string;
  trace_id: string;
  name: string;
  status: 'RUNNING' | 'COMPLETED' | 'FAILED';
  user_id?: string;
  started_at: string;
  ended_at?: string;
  duration_ms?: number;
  error_count?: number;
  total_cost_usd?: number;
  total_tokens_in?: number;
  total_tokens_out?: number;
  latest_activity_at: string;
  semantic_events: TimelineEvent[];
  lineage: SessionNarrativeLineage;
}

export interface SessionNarrative {
  summary: SessionNarrativeSummary;
  traces: SessionNarrativeTrace[];
}

export interface CompareTraceHeader {
  id: string;
  trace_id: string;
  name: string;
  status: 'RUNNING' | 'COMPLETED' | 'FAILED';
  engine?: EngineTraceInfo;
  user_id?: string;
  started_at: string;
  ended_at?: string;
  duration_ms?: number;
  error_count?: number;
  total_cost_usd?: number;
  total_tokens_in?: number;
  total_tokens_out?: number;
}

export interface CompareSessionHeader {
  id: string;
  external_id: string;
  name?: string;
}

export interface CompareSummary {
  total_spans_baseline: number;
  total_spans_candidate: number;
  matched_spans: number;
  unmatched_baseline_spans: number;
  unmatched_candidate_spans: number;
  heuristic_matches: number;
  duration_delta_ms: number;
  tokens_in_delta: number;
  tokens_out_delta: number;
  cost_delta_usd: number;
  total_semantic_baseline: number;
  total_semantic_candidate: number;
}

export interface CompareSpanSummary {
  id: string;
  span_id: string;
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
}

export interface CompareSemanticSummary {
  id: string;
  span_id?: string;
  span_name?: string;
  event_type: 'decision' | 'effect' | 'wait';
  timestamp: string;
  message?: string;
  payload?: Record<string, unknown>;
}

export interface SemanticDiffGroup {
  event_type: 'decision' | 'effect' | 'wait';
  diff_status: 'unchanged' | 'changed' | 'baseline_only' | 'candidate_only';
  match_source?: 'stable_id' | 'heuristic';
  match_reason?: string;
  changed_fields: string[];
  baseline_event: CompareSemanticSummary | null;
  candidate_event: CompareSemanticSummary | null;
}

export interface SpanDiffRow {
  diff_status: 'unchanged' | 'changed' | 'baseline_only' | 'candidate_only';
  match_source?: 'stable_id' | 'heuristic';
  match_reason?: string;
  changed_fields: string[];
  baseline_span: CompareSpanSummary | null;
  candidate_span: CompareSpanSummary | null;
  semantic_groups: SemanticDiffGroup[];
  depth: number;
}

export interface SessionCompareResponse {
  session: CompareSessionHeader;
  baseline: CompareTraceHeader;
  candidate: CompareTraceHeader;
  summary: CompareSummary;
  span_diffs: SpanDiffRow[];
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

/**
 * Fetch a session narrative by session ID.
 */
export async function fetchSessionNarrative(id: string): Promise<SessionNarrative> {
  return fetchAPI<SessionNarrative>(`/api/sessions/${id}/narrative`);
}

export async function fetchSessionComparison(
  sessionId: string,
  baselineTraceId: string,
  candidateTraceId: string
): Promise<SessionCompareResponse> {
  const params = new URLSearchParams({
    baseline_trace_id: baselineTraceId,
    candidate_trace_id: candidateTraceId,
  });

  return fetchAPI<SessionCompareResponse>(`/api/sessions/${sessionId}/compare?${params.toString()}`);
}

function withEnginePreviewHeader(options: RequestInit = {}): RequestInit {
  return {
    ...options,
    headers: {
      [ENGINE_PREVIEW_HEADER]: ENGINE_PREVIEW_HEADER_VALUE,
      ...options.headers,
    },
  };
}

function withJsonBody(body?: unknown): RequestInit {
  if (body === undefined) {
    return { method: 'POST' };
  }

  return {
    method: 'POST',
    body: JSON.stringify(body),
  };
}

export async function signalEngineRun(
  runId: string,
  request: EngineSignalRunRequest
): Promise<EngineControlResponse> {
  return fetchAPI<EngineControlResponse>(
    `/v1/engine/runs/${runId}/signal`,
    withEnginePreviewHeader(withJsonBody(request))
  );
}

export async function cancelEngineRun(runId: string): Promise<EngineControlResponse> {
  return fetchAPI<EngineControlResponse>(
    `/v1/engine/runs/${runId}/cancel`,
    withEnginePreviewHeader(withJsonBody())
  );
}

export async function suspendEngineRun(runId: string): Promise<EngineRunResponse> {
  return fetchAPI<EngineRunResponse>(
    `/v1/engine/runs/${runId}/suspend`,
    withEnginePreviewHeader(withJsonBody())
  );
}

export async function resumeEngineRun(runId: string): Promise<EngineRunResponse> {
  return fetchAPI<EngineRunResponse>(
    `/v1/engine/runs/${runId}/resume`,
    withEnginePreviewHeader(withJsonBody())
  );
}

export async function terminateEngineRun(runId: string): Promise<EngineRunResultResponse> {
  return fetchAPI<EngineRunResultResponse>(
    `/v1/engine/runs/${runId}/terminate`,
    withEnginePreviewHeader(withJsonBody())
  );
}

export async function purgeEngineRun(
  runId: string,
  mode: EnginePurgeMode
): Promise<EnginePurgeResponse> {
  return fetchAPI<EnginePurgeResponse>(
    `/v1/engine/runs/${runId}/purge`,
    withEnginePreviewHeader(withJsonBody({ mode }))
  );
}

export async function repairEngineRun(runId: string): Promise<EngineRepairResponse> {
  return fetchAPI<EngineRepairResponse>(
    `/v1/engine/runs/${runId}/repair`,
    withEnginePreviewHeader(withJsonBody())
  );
}

export async function fetchEnginePendingWork(
  runId: string
): Promise<EnginePendingWorkResponse> {
  return fetchAPI<EnginePendingWorkResponse>(`/v1/engine/runs/${runId}/pending-work`);
}
