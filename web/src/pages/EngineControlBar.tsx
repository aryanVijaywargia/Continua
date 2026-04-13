import { useMemo, useState, type ReactNode } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import {
  ApiError,
  cancelEngineRun,
  purgeEngineRun,
  repairEngineRun,
  resumeEngineRun,
  signalEngineRun,
  suspendEngineRun,
  terminateEngineRun,
  type EnginePurgeMode,
  type EngineRepairReason,
  type EngineRunStatus,
  type EngineRunSummary,
  type JsonValue,
} from '../api/client';

type EngineAction =
  | 'signal'
  | 'cancel'
  | 'suspend'
  | 'resume'
  | 'terminate'
  | 'purge'
  | 'repair';

type FeedbackTone = 'success' | 'info' | 'warning' | 'error';

interface FeedbackState {
  tone: FeedbackTone;
  title: string;
  message: string;
}

interface EngineControlBarProps {
  engine: EngineRunSummary;
  traceId: string;
}

const NON_TERMINAL_ENGINE_STATUSES: ReadonlySet<EngineRunStatus> = new Set([
  'QUEUED',
  'RUNNING',
  'WAITING',
  'SUSPENDED',
]);

const PURGEABLE_ENGINE_STATUSES: ReadonlySet<EngineRunStatus> = new Set([
  'COMPLETED',
  'FAILED',
  'CANCELLED',
  'TERMINATED',
  'CONTINUED_AS_NEW',
]);

const ACTION_LABELS: Record<EngineAction, string> = {
  signal: 'Signal',
  cancel: 'Cancel',
  suspend: 'Suspend',
  resume: 'Resume',
  terminate: 'Terminate',
  purge: 'Purge',
  repair: 'Repair',
};

export function EngineControlBar({
  engine,
  traceId,
}: EngineControlBarProps) {
  const queryClient = useQueryClient();
  const [pendingAction, setPendingAction] = useState<EngineAction | null>(null);
  const [feedback, setFeedback] = useState<FeedbackState | null>(null);
  const [isSignalModalOpen, setIsSignalModalOpen] = useState(false);
  const [signalName, setSignalName] = useState('');
  const [signalPayload, setSignalPayload] = useState('');
  const [confirmAction, setConfirmAction] = useState<
    'cancel' | 'terminate' | 'purge' | null
  >(null);
  const [purgeMode, setPurgeMode] =
    useState<EnginePurgeMode>('projection_only');

  const signalNameTrimmed = signalName.trim();
  const signalPayloadTrimmed = signalPayload.trim();
  const parsedSignalPayload = useMemo(() => {
    if (!signalPayloadTrimmed) {
      return { value: undefined as JsonValue | undefined, error: null };
    }

    try {
      return {
        value: JSON.parse(signalPayloadTrimmed) as JsonValue,
        error: null,
      };
    } catch {
      return {
        value: undefined,
        error: 'Payload must be valid JSON.',
      };
    }
  }, [signalPayloadTrimmed]);

  const isBusy = pendingAction !== null;
  const signalEnabled = NON_TERMINAL_ENGINE_STATUSES.has(engine.status);
  const cancelEnabled = NON_TERMINAL_ENGINE_STATUSES.has(engine.status);
  const suspendEnabled =
    engine.status === 'QUEUED' ||
    engine.status === 'RUNNING' ||
    engine.status === 'WAITING';
  const resumeEnabled = engine.status === 'SUSPENDED';
  const terminateEnabled = NON_TERMINAL_ENGINE_STATUSES.has(engine.status);
  const purgeEnabled = PURGEABLE_ENGINE_STATUSES.has(engine.status);
  const signalSubmitDisabled =
    isBusy || !signalNameTrimmed || parsedSignalPayload.error !== null;

  async function invalidateEngineQueries() {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['trace', traceId] }),
      queryClient.invalidateQueries({ queryKey: ['timeline', traceId] }),
      queryClient.invalidateQueries({ queryKey: ['spans', traceId] }),
      queryClient.invalidateQueries({
        queryKey: ['enginePendingWork', engine.run_id],
      }),
      queryClient.invalidateQueries({ queryKey: ['traces'] }),
    ]);
  }

  async function refreshConflictState() {
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['trace', traceId] }),
      queryClient.invalidateQueries({
        queryKey: ['enginePendingWork', engine.run_id],
      }),
    ]);
  }

  async function runAction(
    action: EngineAction,
    request: () => Promise<FeedbackState>
  ) {
    setPendingAction(action);

    try {
      const nextFeedback = await request();
      setFeedback(nextFeedback);
      await invalidateEngineQueries();
    } catch (error) {
      if (error instanceof ApiError && error.status === 409) {
        await refreshConflictState();
      }

      setFeedback({
        tone: 'error',
        title: `${ACTION_LABELS[action]} failed`,
        message: error instanceof Error ? error.message : 'Request failed.',
      });
    } finally {
      setPendingAction(null);
    }
  }

  async function handleSignalSubmit() {
    if (signalSubmitDisabled) {
      return;
    }

    await runAction('signal', async () => {
      const response = await signalEngineRun(engine.run_id, {
        signal_name: signalNameTrimmed,
        payload: parsedSignalPayload.value,
      });
      setIsSignalModalOpen(false);
      setSignalName('');
      setSignalPayload('');

      return response.accepted
        ? {
            tone: 'success',
            title: 'Signal accepted',
            message: response.wake_applied
              ? 'The signal was delivered and the engine run was woken.'
              : 'The signal was delivered to the engine run.',
          }
        : {
            tone: 'info',
            title: 'Signal recorded',
            message: 'The signal request completed without waking the run.',
          };
    });
  }

  async function handleConfirmedAction() {
    if (!confirmAction) {
      return;
    }

    if (confirmAction === 'cancel') {
      await runAction('cancel', async () => {
        const response = await cancelEngineRun(engine.run_id);
        setConfirmAction(null);

        return response.accepted
          ? {
              tone: 'success',
              title: 'Cancel requested',
              message: response.wake_applied
                ? 'Cancel was requested and the engine run was woken.'
                : 'Cancel was requested for the engine run.',
            }
          : {
              tone: 'info',
              title: 'Cancel already satisfied',
              message: 'The engine run was already in a state that did not change.',
            };
      });
      return;
    }

    if (confirmAction === 'terminate') {
      await runAction('terminate', async () => {
        await terminateEngineRun(engine.run_id);
        setConfirmAction(null);

        return {
          tone: 'success',
          title: 'Run terminated',
          message: 'The engine run was terminated.',
        };
      });
      return;
    }

    await runAction('purge', async () => {
      const response = await purgeEngineRun(engine.run_id, purgeMode);
      setConfirmAction(null);
      setPurgeMode('projection_only');

      return response.deleted
        ? {
            tone: 'success',
            title: 'Purge applied',
            message:
              purgeMode === 'full'
                ? 'Projection state and retained engine history were purged.'
                : 'Projection state was purged for this engine run.',
          }
        : {
            tone: 'info',
            title: 'Purge already satisfied',
            message:
              purgeMode === 'full'
                ? 'Retained history was already removed for this engine run.'
                : 'Projection state was already absent for this engine run.',
          };
    });
  }

  async function handleRepair() {
    await runAction('repair', async () => {
      const response = await repairEngineRun(engine.run_id);
      return repairFeedbackForReason(response.reason);
    });
  }

  async function handleSuspend() {
    await runAction('suspend', async () => {
      await suspendEngineRun(engine.run_id);
      return {
        tone: 'success',
        title: 'Run suspended',
        message: 'The engine run is now suspended.',
      };
    });
  }

  async function handleResume() {
    await runAction('resume', async () => {
      await resumeEngineRun(engine.run_id);
      return {
        tone: 'success',
        title: 'Run resumed',
        message: 'The engine run resumed execution.',
      };
    });
  }

  return (
    <section className="app-surface p-4 sm:p-5">
      <div className="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <div>
          <div className="app-overline">Engine controls</div>
          <h2 className="mt-2 text-xl font-black tight-headline text-[var(--continua-text-primary)]">
            Drive the engine run from the debugger
          </h2>
          <p className="mt-2 text-sm leading-6 text-[var(--continua-text-secondary)]">
            Send signals, request state transitions, repair projections, or
            purge retained engine data from the current trace.
          </p>
        </div>

        <div className="flex flex-wrap gap-2 xl:justify-end">
          <ActionButton
            action="signal"
            disabled={!signalEnabled || isBusy}
            isPending={pendingAction === 'signal'}
            onClick={() => setIsSignalModalOpen(true)}
          />
          <ActionButton
            action="cancel"
            disabled={!cancelEnabled || isBusy}
            isPending={pendingAction === 'cancel'}
            onClick={() => setConfirmAction('cancel')}
          />
          <ActionButton
            action="suspend"
            disabled={!suspendEnabled || isBusy}
            isPending={pendingAction === 'suspend'}
            onClick={() => void handleSuspend()}
          />
          <ActionButton
            action="resume"
            disabled={!resumeEnabled || isBusy}
            isPending={pendingAction === 'resume'}
            onClick={() => void handleResume()}
          />
          <ActionButton
            action="terminate"
            disabled={!terminateEnabled || isBusy}
            isPending={pendingAction === 'terminate'}
            onClick={() => setConfirmAction('terminate')}
          />
          <ActionButton
            action="purge"
            disabled={!purgeEnabled || isBusy}
            isPending={pendingAction === 'purge'}
            onClick={() => setConfirmAction('purge')}
          />
          <ActionButton
            action="repair"
            disabled={isBusy}
            isPending={pendingAction === 'repair'}
            onClick={() => void handleRepair()}
          />
        </div>
      </div>

      {isBusy ? (
        <div className="mt-4 rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-3 py-2 text-sm text-[var(--continua-text-secondary)]">
          Submitting {pendingAction ? ACTION_LABELS[pendingAction].toLowerCase() : 'request'}...
        </div>
      ) : null}

      {feedback ? (
        <EngineControlFeedback feedback={feedback} />
      ) : null}

      {isSignalModalOpen ? (
        <ModalShell
          title="Send signal"
          description="Signal names are required. Payload is optional JSON."
          onClose={() => {
            if (!isBusy) {
              setIsSignalModalOpen(false);
            }
          }}
        >
          <div className="space-y-4">
            <div>
              <label
                htmlFor="engine-signal-name"
                className="mb-1 block text-sm font-medium text-[var(--continua-text-secondary)]"
              >
                Signal name
              </label>
              <input
                id="engine-signal-name"
                type="text"
                value={signalName}
                onChange={(event) => setSignalName(event.target.value)}
                placeholder="e.g. approval_received"
                className="app-input"
              />
              {!signalNameTrimmed ? (
                <p className="mt-1 text-xs text-[var(--continua-error)]">
                  Signal name is required.
                </p>
              ) : null}
            </div>

            <div>
              <label
                htmlFor="engine-signal-payload"
                className="mb-1 block text-sm font-medium text-[var(--continua-text-secondary)]"
              >
                Payload (JSON)
              </label>
              <textarea
                id="engine-signal-payload"
                rows={6}
                value={signalPayload}
                onChange={(event) => setSignalPayload(event.target.value)}
                placeholder='{"approved": true}'
                className="app-input resize-y"
              />
              {parsedSignalPayload.error ? (
                <p className="mt-1 text-xs text-[var(--continua-error)]">
                  {parsedSignalPayload.error}
                </p>
              ) : (
                <p className="mt-1 text-xs text-[var(--continua-text-muted)]">
                  Leave blank to send the signal without a payload.
                </p>
              )}
            </div>

            <div className="flex flex-wrap justify-end gap-2">
              <button
                type="button"
                onClick={() => setIsSignalModalOpen(false)}
                disabled={isBusy}
                className="app-button-secondary"
              >
                Close
              </button>
              <button
                type="button"
                onClick={() => void handleSignalSubmit()}
                disabled={signalSubmitDisabled}
                className="app-button-primary disabled:cursor-not-allowed disabled:opacity-60"
              >
                {pendingAction === 'signal' ? 'Submitting...' : 'Send signal'}
              </button>
            </div>
          </div>
        </ModalShell>
      ) : null}

      {confirmAction ? (
        <ModalShell
          title={confirmDialogTitle(confirmAction)}
          description={confirmDialogDescription(confirmAction)}
          onClose={() => {
            if (!isBusy) {
              setConfirmAction(null);
              setPurgeMode('projection_only');
            }
          }}
        >
          <div className="space-y-4">
            {confirmAction === 'purge' ? (
              <fieldset>
                <legend className="text-sm font-medium text-[var(--continua-text-secondary)]">
                  Purge mode
                </legend>
                <div className="mt-3 space-y-3">
                  <label className="flex items-start gap-3 rounded-[1rem] border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] p-3">
                    <input
                      type="radio"
                      name="purge-mode"
                      value="projection_only"
                      checked={purgeMode === 'projection_only'}
                      onChange={() => setPurgeMode('projection_only')}
                      disabled={isBusy}
                    />
                    <span>
                      <span className="block text-sm font-semibold text-[var(--continua-text-primary)]">
                        Projection only
                      </span>
                      <span className="mt-1 block text-sm text-[var(--continua-text-secondary)]">
                        Remove derived projection state and keep retained engine
                        history.
                      </span>
                    </span>
                  </label>
                  <label className="flex items-start gap-3 rounded-[1rem] border border-amber-300/50 bg-amber-100/60 p-3 text-amber-950 dark:border-amber-300/20 dark:bg-amber-400/10 dark:text-amber-100">
                    <input
                      type="radio"
                      name="purge-mode"
                      value="full"
                      checked={purgeMode === 'full'}
                      onChange={() => setPurgeMode('full')}
                      disabled={isBusy}
                    />
                    <span>
                      <span className="block text-sm font-semibold">
                        Full purge
                      </span>
                      <span className="mt-1 block text-sm opacity-90">
                        Permanently delete retained engine history. This cannot
                        be recovered.
                      </span>
                    </span>
                  </label>
                </div>
              </fieldset>
            ) : null}

            <div className="flex flex-wrap justify-end gap-2">
              <button
                type="button"
                onClick={() => {
                  setConfirmAction(null);
                  setPurgeMode('projection_only');
                }}
                disabled={isBusy}
                className="app-button-secondary"
              >
                Close
              </button>
              <button
                type="button"
                onClick={() => void handleConfirmedAction()}
                disabled={isBusy}
                className="app-button-primary disabled:cursor-not-allowed disabled:opacity-60"
              >
                {confirmAction === pendingAction
                  ? 'Submitting...'
                  : confirmDialogConfirmLabel(confirmAction)}
              </button>
            </div>
          </div>
        </ModalShell>
      ) : null}
    </section>
  );
}

function ActionButton({
  action,
  disabled,
  isPending,
  onClick,
}: {
  action: EngineAction;
  disabled: boolean;
  isPending: boolean;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className="app-button-secondary disabled:cursor-not-allowed disabled:opacity-60"
    >
      {isPending ? 'Submitting...' : ACTION_LABELS[action]}
    </button>
  );
}

function EngineControlFeedback({
  feedback,
}: {
  feedback: FeedbackState;
}) {
  const toneClasses =
    feedback.tone === 'success'
      ? 'border-emerald-300/50 bg-emerald-100/70 text-emerald-950 dark:border-emerald-300/20 dark:bg-emerald-400/10 dark:text-emerald-100'
      : feedback.tone === 'warning'
        ? 'border-amber-300/50 bg-amber-100/70 text-amber-950 dark:border-amber-300/20 dark:bg-amber-400/10 dark:text-amber-100'
        : feedback.tone === 'info'
          ? 'border-sky-300/50 bg-sky-100/70 text-sky-950 dark:border-sky-300/20 dark:bg-sky-400/10 dark:text-sky-100'
          : 'border-red-300/50 bg-red-100/70 text-red-950 dark:border-red-300/20 dark:bg-red-400/10 dark:text-red-100';

  return (
    <div
      role={feedback.tone === 'error' ? 'alert' : 'status'}
      className={`mt-4 rounded-[1rem] border px-4 py-3 text-sm ${toneClasses}`}
    >
      <p className="font-semibold">{feedback.title}</p>
      <p className="mt-1">{feedback.message}</p>
    </div>
  );
}

function ModalShell({
  title,
  description,
  onClose,
  children,
}: {
  title: string;
  description: string;
  onClose: () => void;
  children: ReactNode;
}) {
  return (
    <div className="app-overlay-enter fixed inset-0 z-50 flex items-center justify-center bg-[#111318]/45 px-4 backdrop-blur-sm">
      <button
        type="button"
        aria-label={`Close ${title.toLowerCase()} dialog`}
        className="absolute inset-0"
        onClick={onClose}
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className="relative z-10 w-full max-w-xl rounded-[1rem] border border-[var(--continua-border-strong)] bg-[var(--continua-surface)] p-5 shadow-[var(--continua-shadow-soft)]"
      >
        <div className="mb-4">
          <div className="app-overline">Engine action</div>
          <h3 className="mt-2 text-xl font-black tight-headline text-[var(--continua-text-primary)]">
            {title}
          </h3>
          <p className="mt-2 text-sm leading-6 text-[var(--continua-text-secondary)]">
            {description}
          </p>
        </div>

        {children}
      </div>
    </div>
  );
}

function repairFeedbackForReason(reason: EngineRepairReason): FeedbackState {
  switch (reason) {
    case 'repair_requested':
      return {
        tone: 'success',
        title: 'Repair requested',
        message: 'Projection repair was requested for this engine run.',
      };
    case 'already_catching_up':
      return {
        tone: 'info',
        title: 'Already catching up',
        message: 'Projection repair is already in progress for this engine run.',
      };
    case 'already_up_to_date':
      return {
        tone: 'info',
        title: 'Already up to date',
        message: 'Projection state is already current for this engine run.',
      };
    case 'no_events_to_project':
      return {
        tone: 'info',
        title: 'No events to project',
        message: 'There are no retained engine events left to project.',
      };
    case 'history_expired':
      return {
        tone: 'warning',
        title: 'History expired',
        message:
          'The event journal has expired, so projection repair is not possible.',
      };
  }
}

function confirmDialogTitle(action: 'cancel' | 'terminate' | 'purge'): string {
  switch (action) {
    case 'cancel':
      return 'Cancel run';
    case 'terminate':
      return 'Terminate run';
    case 'purge':
      return 'Purge retained engine data';
  }
}

function confirmDialogDescription(
  action: 'cancel' | 'terminate' | 'purge'
): string {
  switch (action) {
    case 'cancel':
      return 'Confirm before requesting cancellation for this engine run.';
    case 'terminate':
      return 'Confirm before forcefully terminating this engine run.';
    case 'purge':
      return 'Choose how much retained engine state to purge before continuing.';
  }
}

function confirmDialogConfirmLabel(
  action: 'cancel' | 'terminate' | 'purge'
): string {
  switch (action) {
    case 'cancel':
      return 'Confirm cancel';
    case 'terminate':
      return 'Confirm terminate';
    case 'purge':
      return 'Confirm purge';
  }
}
