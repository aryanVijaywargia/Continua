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
    <section className="flex h-full min-h-0 flex-col overflow-hidden rounded-[1.5rem] border border-[var(--continua-border-strong)] bg-[var(--continua-surface)] shadow-[var(--continua-shadow-soft)]">
      <div className="border-b border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-4 py-3">
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
      className={`rounded-full px-3 py-1.5 text-sm font-medium transition focus:outline-none focus:ring-2 focus:ring-[var(--continua-accent-faint)] ${
        active
          ? 'border border-[var(--continua-accent)] bg-[var(--continua-accent-faint)] text-[var(--continua-accent)]'
          : 'border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] text-[var(--continua-text-secondary)] hover:border-[var(--continua-border-strong)] hover:text-[var(--continua-text-primary)]'
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
              ? 'bg-[var(--continua-accent)] text-[var(--continua-accent-contrast)]'
              : 'bg-[var(--continua-text-primary)] text-[var(--continua-surface-elevated)]'
          }`}
        >
          {badgeCount}
        </span>
      ) : null}
    </button>
  );
}
