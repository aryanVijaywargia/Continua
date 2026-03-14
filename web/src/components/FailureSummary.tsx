import type { FailureSummary as FailureSummaryData } from '../utils/failureAnalysis';
import { formatTimestamp } from '../utils/format';
import { SpanBreadcrumb } from './SpanBreadcrumb';

interface FailureSummaryProps {
  summary: FailureSummaryData;
  onJumpToPrimaryFailedSpan: (spanId: string) => void;
}

export function FailureSummary({
  summary,
  onJumpToPrimaryFailedSpan,
}: FailureSummaryProps) {
  const primaryFailedSpan = summary.primaryFailedSpan;

  return (
    <section className="overflow-hidden rounded-xl border border-red-200 bg-white shadow-sm">
      <div className="border-b border-red-200 bg-red-50 px-4 py-3">
        <h2 className="text-sm font-semibold uppercase tracking-[0.2em] text-red-700">
          Failure Summary
        </h2>
        <p className="mt-1 text-sm text-red-700/80">
          Failure-first guidance for this trace.
        </p>
      </div>

      <div className="space-y-4 p-4">
        {primaryFailedSpan ? (
          <>
            <div className="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <h3 className="text-base font-semibold text-gray-900">
                    {primaryFailedSpan.name}
                  </h3>
                  <span className="rounded-full bg-gray-900 px-2.5 py-1 text-[11px] font-semibold uppercase tracking-[0.16em] text-white">
                    {primaryFailedSpan.kind}
                  </span>
                </div>

                {summary.errorPreview ? (
                  <p className="mt-2 text-sm text-gray-700">{summary.errorPreview}</p>
                ) : (
                  <p className="mt-2 text-sm text-gray-500">
                    No inline error preview was available for the primary failed span.
                  </p>
                )}
              </div>

              <button
                type="button"
                className="inline-flex shrink-0 items-center justify-center rounded-full bg-red-600 px-4 py-2 text-sm font-medium text-white transition hover:bg-red-700 focus:outline-none focus:ring-2 focus:ring-red-200"
                aria-label={`Jump to failed span ${primaryFailedSpan.name}`}
                onClick={() => onJumpToPrimaryFailedSpan(primaryFailedSpan.span_id)}
              >
                Jump to failed span
              </button>
            </div>

            <div className="grid gap-3 md:grid-cols-3">
              <SummaryStat
                label="Failed spans"
                value={String(summary.failedSpanCount)}
              />
              <SummaryStat
                label="Error events"
                value={String(summary.errorEventCount)}
              />
              <SummaryStat
                label="Failure timestamp"
                value={formatTimestamp(summary.failureTimestamp)}
              />
            </div>

            <div>
              <div className="mb-2 text-[11px] font-semibold uppercase tracking-[0.18em] text-gray-500">
                Failure path
              </div>
              <SpanBreadcrumb
                path={summary.breadcrumbPath}
                ariaLabel="Failure path"
              />
            </div>
          </>
        ) : (
          <>
            <p className="text-sm text-gray-700">
              This trace is marked as failed, but no failed span could be identified
              from the current span data.
            </p>
            <div className="grid gap-3 md:grid-cols-2">
              <SummaryStat
                label="Failed spans"
                value={String(summary.failedSpanCount)}
              />
              <SummaryStat
                label="Error events"
                value={String(summary.errorEventCount)}
              />
            </div>
          </>
        )}
      </div>
    </section>
  );
}

function SummaryStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-gray-200 bg-gray-50 p-3">
      <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-gray-500">
        {label}
      </div>
      <div className="mt-2 text-sm font-medium text-gray-900">{value}</div>
    </div>
  );
}
