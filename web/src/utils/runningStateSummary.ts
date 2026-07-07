import type { TimelineEvent } from '../api/client';
import { getWaitDetails } from './eventSemantics';
import type { WaitStallAssessment } from './waitStallAnalysis';

export interface RunningStateSummary {
  label: string;
  copy: string;
}

export function getRunningStateSummary(
  assessment: WaitStallAssessment
): RunningStateSummary {
  switch (assessment.classification) {
    case 'declared_wait':
      return {
        label: 'Declared wait',
        copy: 'Execution declared a wait and has not yet recorded a matching resolution.',
      };
    case 'waiting_on_model':
      return {
        label: 'Waiting on model',
        copy: 'Execution appears to be waiting on an in-flight model span.',
      };
    case 'waiting_on_tool':
      return {
        label: 'Waiting on tool',
        copy: 'Execution appears to be waiting on an in-flight tool span.',
      };
    case 'actively_executing':
      return assessment.reason === 'recent_activity_without_open_span'
        ? {
            label: 'Actively executing',
            copy: 'Recent activity suggests execution is still progressing between spans.',
          }
        : {
            label: 'Actively executing',
            copy: 'A running span suggests execution is still actively progressing.',
          };
    case 'possibly_stalled':
      return {
        label: 'Possibly stalled',
        copy: 'Execution is still marked running, but recent activity is sparse.',
      };
    case 'unknown':
      return {
        label: 'Unknown',
        copy: 'The debugger cannot yet explain where it is waiting.',
      };
  }
}

export function formatRunningStateBasis(basis: WaitStallAssessment['basis']): string {
  switch (basis) {
    case 'declared':
      return 'Declared';
    case 'inferred':
      return 'Inferred';
    case 'heuristic':
      return 'Heuristic';
  }
}

export function getRunningStatePanelTone(
  classification: WaitStallAssessment['classification']
): string {
  switch (classification) {
    case 'declared_wait':
    case 'waiting_on_model':
    case 'waiting_on_tool':
      return 'border-sky-200 bg-sky-50 text-sky-950 dark:border-sky-500/30 dark:bg-sky-500/10 dark:text-sky-100';
    case 'actively_executing':
      return 'border-emerald-200 bg-emerald-50 text-emerald-950 dark:border-emerald-500/30 dark:bg-emerald-500/10 dark:text-emerald-100';
    case 'possibly_stalled':
      return 'border-amber-200 bg-amber-50 text-amber-950 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-100';
    case 'unknown':
      return 'border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] text-[var(--continua-text-primary)]';
  }
}

export function resolveDeclaredWaitKind(
  assessment: WaitStallAssessment,
  events: TimelineEvent[]
): string | null {
  if (assessment.classification !== 'declared_wait' || !assessment.decisiveEventId) {
    return null;
  }

  const decisiveEvent = events.find((event) => event.id === assessment.decisiveEventId);
  return decisiveEvent ? getWaitDetails(decisiveEvent)?.waitKind ?? null : null;
}
