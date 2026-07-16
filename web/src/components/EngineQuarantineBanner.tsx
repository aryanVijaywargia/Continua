import type { EngineFailureSummary, EngineWaitState } from '../api/client';
import { describeEngineWaitState } from '../pages/engineWaitState';

export function EngineQuarantineBanner({
  failure,
  waitState,
}: {
  failure?: EngineFailureSummary;
  waitState?: EngineWaitState | null;
}) {
  const mismatchSummary =
    waitState?.kind === 'replay_mismatch'
      ? describeEngineWaitState(waitState)
      : null;

  return (
    <section className="rounded-[1rem] border border-[var(--c-red-border)] bg-[var(--c-red-faint)] px-4 py-3 text-[var(--c-text-primary)]">
      <h2 className="text-sm font-semibold text-[var(--c-red-text)]">
        Run quarantined
      </h2>
      {failure ? (
        <div className="mt-2">
          <div className="font-mono text-xs font-semibold text-[var(--c-red-text)]">
            {failure.error_code}
          </div>
          <p className="mt-1 text-sm leading-6">{failure.error_message}</p>
        </div>
      ) : null}
      {mismatchSummary ? (
        <p className="mt-2 font-mono text-xs leading-5 text-[var(--c-red-text)]">
          {mismatchSummary.detail}
        </p>
      ) : null}
      <p className="mt-2 text-sm leading-6">
        Recover by rolling back the workflow code to the version this run recorded, then use
        Resume to retry replay. Terminate remains available if the run should not continue.
      </p>
    </section>
  );
}
