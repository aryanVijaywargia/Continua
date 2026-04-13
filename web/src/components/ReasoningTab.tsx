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
    <section className="overflow-hidden rounded-xl border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] shadow-sm">
      <div className="border-b border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-4 py-3">
        <h2 className="text-sm font-semibold uppercase tracking-[0.2em] text-[var(--continua-text-secondary)]">
          Reasoning
        </h2>
        <p className="mt-1 text-sm text-[var(--continua-text-muted)]">
          Chronological decision events across the full trace.
        </p>
      </div>

      {entries.length === 0 ? (
        <div className="px-4 py-12 text-center text-sm text-[var(--continua-text-muted)]">
          No reasoning decisions recorded for this trace.
        </div>
      ) : (
        <div className="divide-y divide-[var(--continua-border-soft)]">
          {entries.map((entry) => {
            const isNavigable = Boolean(entry.spanId);

            return (
              <button
                key={entry.event.id}
                type="button"
                disabled={!isNavigable}
                className={`flex w-full gap-4 p-4 text-left transition ${
                  isNavigable
                    ? 'hover:bg-[var(--continua-surface-muted)] focus:outline-none focus:ring-2 focus:ring-[var(--continua-accent)]'
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
                <div className="min-w-28 rounded-xl bg-[var(--continua-text-primary)] px-3 py-2 text-xs font-medium uppercase tracking-[0.18em] text-[var(--continua-surface-elevated)]">
                  <div>{formatTimelineTime(entry.event.timestamp)}</div>
                  <div className="mt-1 text-[10px] tracking-[0.25em] opacity-50">
                    {entry.spanName}
                  </div>
                </div>

                <div className="min-w-0 flex-1">
                  <div className="text-sm font-semibold leading-6 text-[var(--continua-text-primary)]">
                    {entry.question}
                  </div>
                  <div className="mt-2 flex flex-wrap items-center gap-2 text-sm text-[var(--continua-text-secondary)]">
                    <span>Chosen</span>
                    <DecisionValuePill tone="accent">
                      {formatInlineSemanticValue(entry.chosen)}
                    </DecisionValuePill>
                  </div>
                  {entry.reasoning ? (
                    <p className="mt-2 text-sm text-[var(--continua-text-secondary)]">
                      {entry.reasoning}
                    </p>
                  ) : null}
                  {entry.alternatives && entry.alternatives.length > 0 ? (
                    <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-[var(--continua-text-muted)]">
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
