import { useState, type ReactNode } from 'react';
import { Link, useParams } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import {
  fetchSpans,
  fetchTrace,
  Span,
} from '../api/client';
import { JsonViewer } from '../components/JsonViewer';
import { SpanDetail } from '../components/SpanDetail';
import { SpanTree } from '../components/SpanTree';
import { StatusBadge } from '../components/StatusBadge';
import { Timeline } from '../components/Timeline';
import { useRequireApiKey } from '../hooks/useRequireApiKey';
import { useTraceTimeline } from './useTraceTimeline';
import {
  calculateDuration,
  formatCost,
  formatDuration,
  formatTokens,
} from '../utils/format';

/**
 * Trace detail page with span tree, detail panel, and merged event timeline.
 */
export function TraceDetailPage() {
  const { id } = useParams<{ id: string }>();
  const { hasApiKey, prompt } = useRequireApiKey();
  const [selectedSpan, setSelectedSpan] = useState<Span | null>(null);

  if (!hasApiKey) {
    return prompt;
  }

  if (!id) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-gray-50">
        <div className="text-red-600">Trace ID is required</div>
      </div>
    );
  }

  return (
    <TraceDetailContent
      traceId={id}
      selectedSpan={selectedSpan}
      onSelectSpan={setSelectedSpan}
    />
  );
}

interface TraceDetailContentProps {
  traceId: string;
  selectedSpan: Span | null;
  onSelectSpan: (span: Span | null) => void;
}

function TraceDetailContent({
  traceId,
  selectedSpan,
  onSelectSpan,
}: TraceDetailContentProps) {
  const traceQuery = useQuery({
    queryKey: ['trace', traceId],
    queryFn: () => fetchTrace(traceId),
  });

  const spansQuery = useQuery({
    queryKey: ['spans', traceId],
    queryFn: () => fetchSpans(traceId),
  });
  const timeline = useTraceTimeline(traceId);

  if (traceQuery.isLoading || spansQuery.isLoading) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-gray-50">
        <div className="text-gray-500">Loading trace...</div>
      </div>
    );
  }

  if (traceQuery.error) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-gray-50">
        <div className="text-red-600">
          Error loading trace:{' '}
          {traceQuery.error instanceof Error
            ? traceQuery.error.message
            : 'Unknown error'}
        </div>
      </div>
    );
  }

  if (spansQuery.error) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-gray-50">
        <div className="text-red-600">
          Error loading spans:{' '}
          {spansQuery.error instanceof Error
            ? spansQuery.error.message
            : 'Unknown error'}
        </div>
      </div>
    );
  }

  const trace = traceQuery.data;
  const spans = spansQuery.data?.spans ?? [];

  if (!trace) {
    return (
      <div className="flex min-h-screen items-center justify-center bg-gray-50">
        <div className="text-red-600">Trace not found</div>
      </div>
    );
  }

  const timelineStatus = timeline.traceStatus ?? trace.status;
  const duration = calculateDuration(trace.started_at, trace.ended_at);
  const totalTokens =
    (trace.total_tokens_in ?? 0) + (trace.total_tokens_out ?? 0);

  return (
    <div className="flex min-h-screen flex-col bg-gray-50">
      <header className="border-b bg-white px-6 py-4">
        <div className="flex items-center gap-4">
          <Link to="/traces" className="text-gray-500 hover:text-gray-700">
            ← Traces
          </Link>
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="truncate text-xl font-semibold text-gray-900">
                {trace.name}
              </h1>
              <StatusBadge status={timelineStatus} />
            </div>
            <div className="mt-2 flex flex-wrap items-center gap-4 text-sm text-gray-500">
              <span>{formatDuration(duration)}</span>
              <span>{formatTokens(totalTokens)} tokens</span>
              <span>{formatCost(trace.total_cost_usd)}</span>
              {trace.error_count && trace.error_count > 0 && (
                <span className="text-red-600">{trace.error_count} errors</span>
              )}
            </div>
          </div>
        </div>
      </header>

      <div className="flex-1 overflow-y-auto p-4">
        <div className="mx-auto flex max-w-7xl flex-col gap-4">
          <section className="overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm">
            <div className="border-b border-gray-200 bg-gray-50 px-4 py-3">
              <h2 className="text-sm font-semibold uppercase tracking-[0.2em] text-gray-600">
                Trace Context
              </h2>
            </div>
            <div className="space-y-6 p-4">
              <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
                <TraceContextField
                  label="ID"
                  value={renderContextText(trace.id, true)}
                />
                <TraceContextField
                  label="External Trace ID"
                  value={renderContextText(trace.trace_id, true)}
                />
                <TraceContextField
                  label="Session UUID"
                  value={trace.session_id ? (
                    <Link
                      to={`/sessions/${trace.session_id}`}
                      className="font-mono text-xs text-blue-600 hover:text-blue-800"
                    >
                      {trace.session_id}
                    </Link>
                  ) : (
                    renderContextText(undefined)
                  )}
                />
                <TraceContextField
                  label="User ID"
                  value={renderContextText(trace.user_id)}
                />
                <TraceContextField
                  label="Environment"
                  value={renderContextText(trace.environment)}
                />
                <TraceContextField
                  label="Release"
                  value={renderContextText(trace.release)}
                />
                <TraceContextField
                  label="Tags"
                  className="md:col-span-2 xl:col-span-3"
                  value={trace.tags && trace.tags.length > 0 ? (
                    <div className="flex flex-wrap gap-2">
                      {trace.tags.map((tag) => (
                        <span
                          key={tag}
                          className="rounded-full border border-gray-200 bg-white px-3 py-1 font-mono text-xs text-gray-700"
                        >
                          {tag}
                        </span>
                      ))}
                    </div>
                  ) : (
                    renderContextText(undefined)
                  )}
                />
              </div>

              {(trace.input !== undefined || trace.output !== undefined) && (
                <div className="grid gap-4 xl:grid-cols-2">
                  {trace.input !== undefined && (
                    <TracePayloadPanel title="Input" data={trace.input} />
                  )}
                  {trace.output !== undefined && (
                    <TracePayloadPanel title="Output" data={trace.output} />
                  )}
                </div>
              )}
            </div>
          </section>

          <div className="grid gap-4 xl:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
            <section className="overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm">
              <div className="border-b border-gray-200 bg-gray-50 px-4 py-3">
                <h2 className="text-sm font-semibold uppercase tracking-[0.2em] text-gray-600">
                  Spans ({spans.length})
                </h2>
              </div>
              <div className="h-[32rem] overflow-y-auto">
                <SpanTree
                  spans={spans}
                  selectedSpanId={selectedSpan?.id ?? null}
                  onSelectSpan={onSelectSpan}
                />
              </div>
            </section>

            <section className="overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm">
              <div className="border-b border-gray-200 bg-gray-50 px-4 py-3">
                <h2 className="text-sm font-semibold uppercase tracking-[0.2em] text-gray-600">
                  {selectedSpan ? selectedSpan.name : 'Span Details'}
                </h2>
              </div>
              <div className="h-[32rem]">
                <SpanDetail span={selectedSpan} />
              </div>
            </section>
          </div>

          <Timeline
            events={timeline.events}
            traceStatus={timelineStatus}
            isLive={timeline.isLive}
            isLoading={timeline.isLoading}
            error={timeline.error}
            selectedSpanId={selectedSpan?.span_id ?? null}
            onSelectSpan={(spanId) => {
              const span = spans.find((candidate) => candidate.span_id === spanId) ?? null;
              if (span) {
                onSelectSpan(span);
              }
            }}
          />
        </div>
      </div>
    </div>
  );
}

interface TraceContextFieldProps {
  label: string;
  value: ReactNode;
  className?: string;
}

function TraceContextField({ label, value, className = '' }: TraceContextFieldProps) {
  return (
    <div className={`rounded-lg border border-gray-200 bg-gray-50 p-4 ${className}`.trim()}>
      <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-gray-500">
        {label}
      </div>
      <div className="mt-2 text-sm text-gray-900">{value}</div>
    </div>
  );
}

function TracePayloadPanel({ title, data }: { title: string; data: unknown }) {
  return (
    <div className="rounded-lg border border-gray-200 bg-gray-50 p-4">
      <h3 className="mb-2 text-sm font-medium text-gray-700">{title}</h3>
      <JsonViewer data={data} className="max-h-80 overflow-y-auto bg-white" />
    </div>
  );
}

function renderContextText(value: string | undefined, monospace = false) {
  if (value === undefined) {
    return <span className="text-sm text-gray-400">-</span>;
  }

  return (
    <span className={monospace ? 'font-mono text-xs text-gray-900' : 'text-sm text-gray-900'}>
      {value}
    </span>
  );
}
