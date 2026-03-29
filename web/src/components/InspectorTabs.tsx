import {
  useEffect,
  useState,
  type MutableRefObject,
  type ReactNode,
} from 'react';

export type InspectorTabId = 'details' | 'timeline' | 'reasoning' | 'state';

interface InspectorTabsProps {
  details: ReactNode;
  reasoning: ReactNode;
  timeline: ReactNode;
  state: ReactNode;
  stateCount: number;
  switchToDetailsRef?: MutableRefObject<(() => void) | null>;
}

export function InspectorTabs({
  details,
  reasoning,
  timeline,
  state,
  stateCount,
  switchToDetailsRef,
}: InspectorTabsProps) {
  const [activeTab, setActiveTab] = useState<InspectorTabId>('details');

  useEffect(() => {
    if (!switchToDetailsRef) {
      return;
    }

    const switchToDetails = () => setActiveTab('details');
    switchToDetailsRef.current = switchToDetails;

    return () => {
      if (switchToDetailsRef.current === switchToDetails) {
        switchToDetailsRef.current = null;
      }
    };
  }, [switchToDetailsRef]);

  return (
    <section className="flex h-full min-h-0 flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
      <div className="border-b border-slate-200 bg-slate-50 px-4 py-3 dark:border-slate-800 dark:bg-slate-950/70">
        <div className="flex items-center gap-2">
          <InspectorTabButton
            active={activeTab === 'details'}
            label="Details"
            onClick={() => setActiveTab('details')}
          />
          <InspectorTabButton
            active={activeTab === 'timeline'}
            label="Timeline"
            onClick={() => setActiveTab('timeline')}
          />
          <InspectorTabButton
            active={activeTab === 'reasoning'}
            label="Reasoning"
            onClick={() => setActiveTab('reasoning')}
          />
          <InspectorTabButton
            active={activeTab === 'state'}
            label="State"
            badgeCount={stateCount}
            onClick={() => setActiveTab('state')}
          />
        </div>
      </div>

      <div className="min-h-0 flex-1">
        <div
          className="h-full"
          style={{ display: activeTab === 'details' ? 'block' : 'none' }}
        >
          {details}
        </div>
        <div
          className="h-full"
          style={{ display: activeTab === 'timeline' ? 'block' : 'none' }}
        >
          {timeline}
        </div>
        <div
          className="h-full"
          style={{ display: activeTab === 'reasoning' ? 'block' : 'none' }}
        >
          {reasoning}
        </div>
        <div
          className="h-full"
          style={{ display: activeTab === 'state' ? 'block' : 'none' }}
        >
          {state}
        </div>
      </div>
    </section>
  );
}

function InspectorTabButton({
  active,
  label,
  badgeCount,
  onClick,
}: {
  active: boolean;
  label: string;
  badgeCount?: number;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      aria-label={label}
      className={`rounded-full px-3 py-1.5 text-sm font-medium transition focus:outline-none focus:ring-2 focus:ring-blue-200 ${
        active
          ? 'bg-slate-900 text-white dark:bg-slate-100 dark:text-slate-950'
          : 'bg-white text-slate-600 ring-1 ring-slate-200 hover:bg-slate-100 dark:bg-slate-900 dark:text-slate-300 dark:ring-slate-700 dark:hover:bg-slate-800'
      }`}
      aria-pressed={active}
      onClick={onClick}
    >
      <span>{label}</span>
      {badgeCount && badgeCount > 0 ? (
        <span
          aria-hidden="true"
          className={`ml-2 rounded-full px-2 py-0.5 text-xs font-semibold ${
            active
              ? 'bg-white/20 text-white dark:bg-slate-200/30 dark:text-slate-950'
              : 'bg-slate-900 text-white dark:bg-slate-100 dark:text-slate-950'
          }`}
        >
          {badgeCount}
        </span>
      ) : null}
    </button>
  );
}
