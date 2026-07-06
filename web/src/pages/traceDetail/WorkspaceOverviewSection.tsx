import { useEffect, useState, type ReactNode } from 'react';
import type { Span, TimelineEvent } from '../../api/client';
import { ExecutionWaterfall } from '../../components/ExecutionWaterfall';
import { TreeRail } from '../../components/TreeRail';
import { TruncationBanner } from '../../components/TruncationBanner';
import { WorkspaceShell } from '../../components/WorkspaceShell';
import { formatDuration, formatTimestamp } from '../../utils/format';
import { deriveVisibleRows } from '../../utils/spanTree';
import {
  CompactPayloadInspector,
  InspectorEmptyState,
} from './CompactPayloadInspector';
import { useTraceDetailWorkspace } from './traceDetailWorkspaceContext';

/**
 * The overview workspace: tree rail + execution waterfall + span inspector.
 * Everything except layout inputs comes from the workspace context, so this
 * section no longer forwards two dozen props into the shell.
 */
export function WorkspaceOverviewSection({
  isDesktop,
  mobileSummary,
}: {
  isDesktop: boolean;
  mobileSummary: ReactNode;
}) {
  const {
    activeMobileTab,
    collapseAll,
    events,
    expandAll,
    expandableSpanIds,
    expandedSpanIds,
    failureAnalysis,
    revealPath,
    selectSpan,
    selectSpanAndShowDetails,
    selectedSpan,
    selectedSpanId,
    setActiveMobileTab,
    setExact,
    spanIndex,
    spanTree,
    spans,
    toggleExpandedSpan,
    trace,
    traceCostSeries,
    visibleRetrySafetyAssessments,
  } = useTraceDetailWorkspace();

  const [visibleRows, setVisibleRows] = useState(() =>
    deriveVisibleRows(spanTree, expandedSpanIds)
  );

  useEffect(() => {
    setVisibleRows(deriveVisibleRows(spanTree, expandedSpanIds));
  }, [expandedSpanIds, spanTree]);

  return (
    <WorkspaceShell
      isDesktop={isDesktop}
      treeRail={
        <TreeRail
          expandableSpanIds={expandableSpanIds}
          expandedSpanIds={expandedSpanIds}
          failedSpanIds={failureAnalysis.failedSpanIds}
          inlineErrorPreviews={failureAnalysis.inlineErrorPreviews}
          onSelectSpan={selectSpan}
          onToggleExpand={toggleExpandedSpan}
          onVisibleRowsChange={setVisibleRows}
          primaryAncestorPath={failureAnalysis.primaryAncestorPath}
          revealPath={revealPath}
          selectedSpanId={selectedSpanId}
          expandAll={expandAll}
          collapseAll={collapseAll}
          setExact={setExact}
          spanIndex={spanIndex}
          spanTree={spanTree}
          spans={spans}
          spanAssessments={visibleRetrySafetyAssessments}
        />
      }
      waterfall={
        <ExecutionWaterfall
          events={events}
          rows={visibleRows}
          selectedSpanId={selectedSpanId}
          onSelectSpanAndShowDetails={selectSpanAndShowDetails}
          revealTarget={selectedSpanId}
          spans={spans}
          costSeries={traceCostSeries}
          spanAssessments={visibleRetrySafetyAssessments}
          traceEndedAt={trace.ended_at}
          traceStartedAt={trace.started_at}
        />
      }
      inspector={
        <SpanInspectorPanel
          events={events}
          selectedSpan={selectedSpan}
        />
      }
      mobileSummary={mobileSummary}
      activeMobileTab={activeMobileTab}
      onMobileTabChange={setActiveMobileTab}
    />
  );
}

function SpanInspectorPanel({
  events,
  selectedSpan,
}: {
  events: TimelineEvent[];
  selectedSpan: Span | null;
}) {
  const [activeTab, setActiveTab] = useState<'input' | 'output' | 'attributes' | 'events'>('input');

  if (!selectedSpan) {
    return (
      <section className="flex h-full items-center justify-center bg-[var(--c-app-bg)] text-sm text-[var(--c-text-muted)]">
        Select a span to inspect payloads.
      </section>
    );
  }

  const spanEvents = events.filter((event) => event.span_id === selectedSpan.span_id);
  const tabs = [
    { id: 'input' as const, label: 'Input' },
    { id: 'output' as const, label: 'Output' },
    { id: 'attributes' as const, label: 'Attributes' },
    { id: 'events' as const, label: 'Events', count: spanEvents.length },
  ];

  return (
    <section className="flex h-full min-h-0 flex-col overflow-hidden bg-[var(--c-app-bg)]">
      <div className="border-b border-[var(--c-border)] px-4 py-3">
        <div className="flex items-center gap-2">
          <span
            className="h-1.5 w-1.5 rounded-full"
            style={{
              background:
                selectedSpan.status === 'FAILED'
                  ? 'var(--c-red)'
                  : selectedSpan.status === 'STARTED'
                    ? 'var(--c-blue)'
                    : 'var(--c-green)',
            }}
          />
          <h2 className="min-w-0 truncate font-mono text-[13px] font-semibold text-[var(--c-text-primary)]">
            {selectedSpan.name}
          </h2>
        </div>
        <div className="mt-2 flex gap-4 text-[11.5px] text-[var(--c-text-muted)]">
          <span className="font-mono">{formatDuration(selectedSpan.latency_ms)}</span>
          <span>kind: {selectedSpan.kind.toLowerCase()}</span>
          <span>{selectedSpan.status.toLowerCase()}</span>
        </div>
      </div>

      <div className="flex border-b border-[var(--c-border)] px-3">
        {tabs.map((tab) => (
          <button
            key={tab.id}
            type="button"
            aria-pressed={activeTab === tab.id}
            onClick={() => setActiveTab(tab.id)}
            className={`-mb-px border-b-2 px-3 py-2 text-xs font-medium ${
              activeTab === tab.id
                ? 'border-[var(--c-accent)] text-[var(--c-text-primary)]'
                : 'border-transparent text-[var(--c-text-secondary)] hover:text-[var(--c-text-primary)]'
            }`}
          >
            {tab.label}
            {tab.count != null ? (
              <span className="ml-1 font-mono text-[10.5px] text-[var(--c-text-muted)]">
                {tab.count}
              </span>
            ) : null}
          </button>
        ))}
      </div>

      <div className="min-h-0 flex-1 overflow-auto p-3">
        {activeTab === 'input' ? (
          selectedSpan.input !== undefined ? (
            <>
              <TruncationBanner
                title="Input payload"
                truncated={selectedSpan.input_truncated}
                originalSizeBytes={selectedSpan.input_original_size_bytes}
                reason={selectedSpan.input_truncation_reason}
              />
              <CompactPayloadInspector value={selectedSpan.input} />
            </>
          ) : (
            <InspectorEmptyState>No input payload recorded.</InspectorEmptyState>
          )
        ) : null}

        {activeTab === 'output' ? (
          selectedSpan.output !== undefined ? (
            <>
              <TruncationBanner
                title="Output payload"
                truncated={selectedSpan.output_truncated}
                originalSizeBytes={selectedSpan.output_original_size_bytes}
                reason={selectedSpan.output_truncation_reason}
              />
              <CompactPayloadInspector value={selectedSpan.output} />
            </>
          ) : selectedSpan.error_message ? (
            <pre className="whitespace-pre-wrap rounded border border-[var(--c-red-border)] bg-[var(--c-red-faint)] p-3 font-mono text-xs leading-6 text-[var(--c-red-text)]">
              {selectedSpan.error_message}
            </pre>
          ) : (
            <InspectorEmptyState>No output payload recorded.</InspectorEmptyState>
          )
        ) : null}

        {activeTab === 'attributes' ? (
          <div className="divide-y divide-[var(--c-border-subtle)] text-xs">
            {[
              ['span.id', selectedSpan.id],
              ['span.span_id', selectedSpan.span_id],
              ['trace.id', selectedSpan.trace_id],
              ['span.kind', selectedSpan.kind.toLowerCase()],
              ['span.status', selectedSpan.status.toLowerCase()],
              ['span.started_at', formatTimestamp(selectedSpan.started_at)],
              ['span.ended_at', selectedSpan.ended_at ? formatTimestamp(selectedSpan.ended_at) : '—'],
              ['span.duration', formatDuration(selectedSpan.latency_ms)],
              ['model', selectedSpan.model ?? '—'],
              ['provider', selectedSpan.provider ?? '—'],
            ].map(([key, value]) => (
              <div key={key} className="grid grid-cols-[8.5rem_minmax(0,1fr)] gap-3 py-2">
                <span className="font-mono text-[var(--c-text-muted)]">{key}</span>
                <span className="min-w-0 truncate font-mono text-[var(--c-text-primary)]">{value}</span>
              </div>
            ))}
            {selectedSpan.metadata && Object.keys(selectedSpan.metadata).length > 0 ? (
              <div className="py-3">
                <div className="mb-2 font-mono text-[var(--c-text-muted)]">metadata</div>
                <CompactPayloadInspector value={selectedSpan.metadata} />
              </div>
            ) : null}
          </div>
        ) : null}

        {activeTab === 'events' ? (
          spanEvents.length > 0 ? (
            <div className="divide-y divide-[var(--c-border-subtle)]">
              {spanEvents.map((event) => (
                <div key={event.id} className="py-2 text-xs">
                  <div className="flex items-center justify-between gap-3">
                    <span className="font-mono text-[var(--c-text-primary)]">{event.event_type}</span>
                    <span className="font-mono text-[var(--c-text-muted)]">
                      {formatTimestamp(event.timestamp)}
                    </span>
                  </div>
                  {event.payload !== undefined ? (
                    <div className="mt-2">
                      <CompactPayloadInspector value={event.payload} />
                    </div>
                  ) : null}
                </div>
              ))}
            </div>
          ) : (
            <InspectorEmptyState>No events recorded for this span.</InspectorEmptyState>
          )
        ) : null}
      </div>
    </section>
  );
}
