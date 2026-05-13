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
    <section className="flex h-full min-h-0 flex-col overflow-hidden bg-[var(--c-app-bg)]">
      <div className="border-b border-[var(--c-border)] px-3">
        <div className="flex items-center gap-0">
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
      className={`-mb-px border-b-2 px-3 py-2 text-xs font-medium transition focus:outline-none focus:ring-2 focus:ring-[var(--c-accent-faint)] ${
        active
          ? 'border-[var(--c-accent)] text-[var(--c-text-primary)]'
          : 'border-transparent text-[var(--c-text-secondary)] hover:text-[var(--c-text-primary)]'
      }`}
      aria-pressed={active}
      onClick={onClick}
    >
      <span>{label}</span>
      {badgeCount && badgeCount > 0 ? (
        <span
          aria-hidden="true"
          className={`ml-1 font-mono text-[10.5px] ${
            active
              ? 'text-[var(--c-text-muted)]'
              : 'text-[var(--c-text-muted)]'
          }`}
        >
          {badgeCount}
        </span>
      ) : null}
    </button>
  );
}
