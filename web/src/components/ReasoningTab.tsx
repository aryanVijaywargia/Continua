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
    <section className="overflow-hidden rounded-xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
      <div className="border-b border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-950/70">
        <h2 className="text-sm font-semibold uppercase tracking-[0.2em] text-slate-600 dark:text-slate-300">
          Reasoning
        </h2>
        <p className="mt-1 text-sm text-slate-500 dark:text-slate-400">
          Chronological decision events across the full trace.
        </p>
      </div>

      {entries.length === 0 ? (
        <div className="px-4 py-12 text-center text-sm text-slate-500 dark:text-slate-400">
          No reasoning decisions recorded for this trace.
        </div>
      ) : (
        <div className="divide-y divide-slate-100 dark:divide-slate-800">
          {entries.map((entry) => {
            const isNavigable = Boolean(entry.spanId);

            return (
              <button
                key={entry.event.id}
                type="button"
                disabled={!isNavigable}
                className={`flex w-full gap-4 p-4 text-left transition ${
                  isNavigable
                    ? 'hover:bg-slate-50 focus:outline-none focus:ring-2 focus:ring-blue-200 dark:hover:bg-slate-800/70'
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
                <div className="min-w-28 rounded-xl bg-slate-900 px-3 py-2 text-xs font-medium uppercase tracking-[0.18em] text-white dark:bg-slate-100 dark:text-slate-950">
                  <div>{formatTimelineTime(entry.event.timestamp)}</div>
                  <div className="mt-1 text-[10px] tracking-[0.25em] text-slate-300 dark:text-slate-600">
                    {entry.spanName}
                  </div>
                </div>

                <div className="min-w-0 flex-1">
                  <div className="text-sm font-semibold leading-6 text-slate-900 dark:text-slate-100">
                    {entry.question}
                  </div>
                  <div className="mt-2 flex flex-wrap items-center gap-2 text-sm text-slate-600 dark:text-slate-300">
                    <span>Chosen</span>
                    <DecisionValuePill tone="accent">
                      {formatInlineSemanticValue(entry.chosen)}
                    </DecisionValuePill>
                  </div>
                  {entry.reasoning ? (
                    <p className="mt-2 text-sm text-slate-600 dark:text-slate-300">
                      {entry.reasoning}
                    </p>
                  ) : null}
                  {entry.alternatives && entry.alternatives.length > 0 ? (
                    <div className="mt-2 flex flex-wrap items-center gap-2 text-xs text-slate-500 dark:text-slate-400">
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
