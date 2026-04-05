import type { EngineProjectionState } from '../api/client';
import { formatProjectionStateLabel } from './engineProjectionState';

type ProjectionBannerTone = {
  className: string;
  title: string;
  message: string;
};

const bannerToneByState: Partial<Record<EngineProjectionState, ProjectionBannerTone>> = {
  catching_up: {
    className:
      'border-amber-300/40 bg-amber-50/80 text-amber-900 dark:border-amber-400/20 dark:bg-amber-400/10 dark:text-amber-100',
    title: 'Projection catching up',
    message: 'Projected spans and events may lag behind the latest engine state for a short time.',
  },
  summary_only: {
    className:
      'border-stone-300/40 bg-stone-50/80 text-stone-900 dark:border-stone-400/20 dark:bg-stone-400/10 dark:text-stone-100',
    title: 'Summary retained',
    message: 'Detailed span and event rows were cleaned up. Summary state is still available.',
  },
  journal_expired: {
    className:
      'border-rose-300/40 bg-rose-50/80 text-rose-900 dark:border-rose-400/20 dark:bg-rose-400/10 dark:text-rose-100',
    title: 'Journal expired',
    message: 'Detailed engine history is no longer available for this trace.',
  },
};

export function EngineProjectionBanner({
  projectionState,
}: {
  projectionState?: EngineProjectionState;
}) {
  if (!projectionState || projectionState === 'up_to_date') {
    return null;
  }

  const banner = bannerToneByState[projectionState];
  if (!banner) {
    return null;
  }

  return (
    <section className={`rounded-[1.25rem] border px-4 py-3 ${banner.className}`}>
      <div className="flex flex-wrap items-center gap-2">
        <h2 className="text-sm font-semibold">{banner.title}</h2>
        <span className="text-[11px] font-semibold uppercase tracking-[0.12em] opacity-75">
          {formatProjectionStateLabel(projectionState)}
        </span>
      </div>
      <p className="mt-1 text-sm leading-6">{banner.message}</p>
    </section>
  );
}
