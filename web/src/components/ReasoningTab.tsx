import { formatInlineSemanticValue } from '../utils/eventSemantics';
import { type DecisionTraceEntry } from '../utils/reasoning';
import { formatTimelineTime } from '../utils/timeline';
import { DecisionValuePill } from './DecisionValuePill';

interface ReasoningTabProps {
  entries: DecisionTraceEntry[];
  onSelectSpan: (spanId: string) => void;
}

export function ReasoningTab({ entries, onSelectSpan }: ReasoningTabProps) {
  return (
    <section className="overflow-hidden rounded-md border border-[var(--c-border)] bg-[var(--c-surface)]">
      <div className="border-b border-[var(--c-border)] bg-[var(--c-surface-muted)] px-4 py-3">
        <h2 className="text-xs font-semibold uppercase tracking-[0.08em] text-[var(--c-text-muted)]">
          Reasoning
        </h2>
        <p className="mt-1 text-sm text-[var(--c-text-muted)]">
          Chronological decision events across the full trace.
        </p>
      </div>

      {entries.length === 0 ? (
        <div className="px-4 py-12 text-center text-sm text-[var(--c-text-muted)]">
          No reasoning decisions recorded for this trace.
        </div>
      ) : (
        <div className="divide-y divide-[var(--c-border-subtle)]">
          {entries.map((entry) => {
            const isNavigable = Boolean(entry.spanId);

            return (
              <button
                key={entry.event.id}
                type="button"
                disabled={!isNavigable}
                className={`flex w-full gap-4 p-4 text-left transition ${
                  isNavigable
                    ? 'hover:bg-[var(--c-row-hover-bg)] focus:outline-none focus:ring-2 focus:ring-[var(--c-accent-faint)]'
                    : 'cursor-default opacity-80'
                }`}
                onClick={() => {
                  if (entry.spanId) {
                    onSelectSpan(entry.spanId);
                  }
                }}
                onKeyDown={(event) => {
                  if (
                    !entry.spanId ||
                    (event.key !== ' ' &&
                      event.key !== 'Space' &&
                      event.key !== 'Spacebar')
                  ) {
                    return;
                  }

                  event.preventDefault();
                  onSelectSpan(entry.spanId);
                }}
              >
                <div className="min-w-28 rounded-md border border-[var(--c-border)] bg-[var(--c-table-head-bg)] px-3 py-2 text-xs font-medium uppercase tracking-[0.08em] text-[var(--c-text-secondary)]">
                  <div>{formatTimelineTime(entry.event.timestamp)}</div>
                  <div className="mt-1 truncate text-[10px] text-[var(--c-text-muted)]">
                    {entry.spanName}
                  </div>
                </div>

                <div className="min-w-0 flex-1">
                  <div className="text-sm font-semibold leading-6 text-[var(--c-text-primary)]">
                    {entry.question}
                  </div>
                  <div className="mt-2 flex flex-wrap items-center gap-2 text-sm text-[var(--c-text-secondary)]">
                    <span>Chosen</span>
                    <DecisionValuePill tone="accent">
                      {formatInlineSemanticValue(entry.chosen)}
                    </DecisionValuePill>
                  </div>
                  {entry.reasoning ? (
                    <p className="mt-2 text-sm text-[var(--c-text-secondary)]">
                      {entry.reasoning}
                    </p>
                  ) : null}
                  {entry.alternatives && entry.alternatives.length > 0 ? (
                    <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-[var(--c-text-muted)]">
                      <span>Alternatives</span>
                      {entry.alternatives.map((alternative, index) => (
                        <DecisionValuePill
                          key={`${entry.event.id}-alternative-${index}`}
                        >
                          {formatInlineSemanticValue(alternative)}
                        </DecisionValuePill>
                      ))}
                    </div>
                  ) : null}
                </div>
              </button>
            );
          })}
        </div>
      )}
    </section>
  );
}
