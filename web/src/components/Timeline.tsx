import { useState } from 'react';
import { TimelineEvent, TimelineTraceStatus } from '../api/client';
import { JsonViewer } from './JsonViewer';
import {
  isTimelineErrorEvent,
  summarizeTimelineEvent,
} from '../utils/timeline';

interface TimelineProps {
  events: TimelineEvent[];
  traceStatus: TimelineTraceStatus | null;
  isLive: boolean;
  isLoading?: boolean;
  error?: string | null;
  selectedSpanId?: string | null;
  onSelectSpan: (spanId: string) => void;
}

/**
 * Chronological timeline view for trace events and synthetic lifecycle markers.
 */
export function Timeline({
  events,
  traceStatus,
  isLive,
  isLoading = false,
  error = null,
  selectedSpanId = null,
  onSelectSpan,
}: TimelineProps) {
  return (
    <section className="overflow-hidden rounded-xl border border-gray-200 bg-white shadow-sm">
      <div className="flex flex-wrap items-center justify-between gap-3 border-b border-gray-200 bg-gray-50 px-4 py-3">
        <div>
          <h2 className="text-sm font-semibold uppercase tracking-[0.2em] text-gray-600">
            Timeline
          </h2>
          <p className="mt-1 text-sm text-gray-500">
            Chronological trace events with lifecycle markers and payload inspection.
          </p>
        </div>
        <div className="flex items-center gap-2 rounded-full border border-gray-200 bg-white px-3 py-1 text-xs font-medium text-gray-600">
          <span
            className={`h-2.5 w-2.5 rounded-full ${
              isLive ? 'animate-pulse bg-emerald-500' : traceStatus === 'FAILED' ? 'bg-red-500' : 'bg-gray-400'
            }`}
          />
          <span>{timelineStatusLabel(traceStatus, isLive)}</span>
        </div>
      </div>

      {isLoading && events.length === 0 ? (
        <div className="px-4 py-12 text-center text-sm text-gray-500">
          Loading timeline...
        </div>
      ) : error && events.length === 0 ? (
        <div className="px-4 py-12 text-center text-sm text-red-600">{error}</div>
      ) : events.length === 0 ? (
        <div className="px-4 py-12 text-center text-sm text-gray-500">
          No timeline events recorded for this trace yet.
        </div>
      ) : (
        <div className="divide-y divide-gray-100">
          {events.map((event) => (
            <TimelineRow
              key={event.id}
              event={event}
              isSelectedSpan={selectedSpanId === event.span_id}
              onSelectSpan={onSelectSpan}
            />
          ))}
        </div>
      )}
    </section>
  );
}

interface TimelineRowProps {
  event: TimelineEvent;
  isSelectedSpan: boolean;
  onSelectSpan: (spanId: string) => void;
}

function TimelineRow({ event, isSelectedSpan, onSelectSpan }: TimelineRowProps) {
  const [isExpanded, setIsExpanded] = useState(false);
  const hasDetails = Boolean(event.message || event.payload);
  const hasNavigableSpan = Boolean(event.span_id && event.span_name);
  const isError = isTimelineErrorEvent(event);
  const rowAccent = isError
    ? 'border-red-200 bg-red-50/70'
    : event.source === 'synthetic'
      ? 'border-amber-200 bg-amber-50/70'
      : 'border-gray-200 bg-white';

  return (
    <div className="p-4">
      <div className={`rounded-2xl border ${rowAccent} p-4 transition-colors`}>
        <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
          <div className="flex min-w-0 gap-4">
            <div className="min-w-28 rounded-xl bg-gray-900 px-3 py-2 text-xs font-medium uppercase tracking-[0.18em] text-white">
              <div>{formatTimelineTime(event.timestamp)}</div>
              <div className="mt-1 text-[10px] tracking-[0.25em] text-gray-300">
                {event.source}
              </div>
            </div>

            <div className="min-w-0">
              <div className="flex flex-wrap items-center gap-2">
                <span
                  className={`rounded-full px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.16em] ${
                    isError
                      ? 'bg-red-100 text-red-700'
                      : event.source === 'synthetic'
                        ? 'bg-amber-100 text-amber-700'
                        : 'bg-blue-100 text-blue-700'
                  }`}
                >
                  {formatEventType(event.event_type)}
                </span>
                {event.level && (
                  <span className="rounded-full border border-gray-200 bg-white px-2.5 py-1 text-[11px] font-medium uppercase tracking-[0.16em] text-gray-600">
                    {event.level}
                  </span>
                )}
              </div>

              <p className="mt-3 text-sm font-medium leading-6 text-gray-900">
                {summarizeTimelineEvent(event)}
              </p>

              {(event.span_name || event.span_id) && (
                <div className="mt-3 flex flex-wrap items-center gap-2 text-xs text-gray-500">
                  <span className="uppercase tracking-[0.18em] text-gray-400">
                    span
                  </span>
                  {hasNavigableSpan && event.span_id ? (
                    <button
                      type="button"
                      className={`rounded-full px-3 py-1 font-medium transition ${
                        isSelectedSpan
                          ? 'bg-gray-900 text-white'
                          : 'bg-white text-gray-700 ring-1 ring-gray-200 hover:bg-gray-100'
                      }`}
                      onClick={() => onSelectSpan(event.span_id!)}
                    >
                      {event.span_name}
                    </button>
                  ) : (
                    <span className="rounded-full bg-white px-3 py-1 font-mono text-gray-600 ring-1 ring-gray-200">
                      {event.span_name ?? event.span_id}
                    </span>
                  )}
                </div>
              )}
            </div>
          </div>

          {hasDetails && (
            <button
              type="button"
              className="rounded-full border border-gray-200 bg-white px-3 py-1.5 text-xs font-medium text-gray-600 transition hover:bg-gray-100"
              onClick={() => setIsExpanded((expanded) => !expanded)}
            >
              {isExpanded ? 'Hide details' : 'Show details'}
            </button>
          )}
        </div>

        {isExpanded && hasDetails && (
          <div className="mt-4 space-y-3 border-t border-gray-200 pt-4">
            {event.message && (
              <div>
                <div className="mb-1 text-xs font-semibold uppercase tracking-[0.18em] text-gray-500">
                  Message
                </div>
                <div className="rounded-xl border border-gray-200 bg-white px-3 py-2 text-sm text-gray-700">
                  {event.message}
                </div>
              </div>
            )}
            {event.payload && (
              <div>
                <div className="mb-1 text-xs font-semibold uppercase tracking-[0.18em] text-gray-500">
                  Payload
                </div>
                <JsonViewer data={event.payload} className="max-h-80 overflow-y-auto" />
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

function formatTimelineTime(timestamp: string): string {
  return new Date(timestamp).toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  });
}

function formatEventType(eventType: TimelineEvent['event_type']): string {
  return eventType.replace(/_/g, ' ');
}

function timelineStatusLabel(
  traceStatus: TimelineTraceStatus | null,
  isLive: boolean
): string {
  if (isLive) {
    return 'LIVE / polling every 3s';
  }
  if (traceStatus === 'FAILED') {
    return 'FAILED';
  }
  if (traceStatus === 'COMPLETED') {
    return 'COMPLETED';
  }
  return 'IDLE';
}
