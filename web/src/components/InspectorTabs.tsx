import {
  useEffect,
  useState,
  type MutableRefObject,
  type ReactNode,
} from 'react';

export type InspectorTabId = 'details' | 'timeline';

interface InspectorTabsProps {
  details: ReactNode;
  timeline: ReactNode;
  switchToDetailsRef?: MutableRefObject<(() => void) | null>;
}

export function InspectorTabs({
  details,
  timeline,
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
    <section className="flex h-full min-h-0 flex-col overflow-hidden rounded-2xl border border-gray-200 bg-white shadow-sm">
      <div className="border-b border-gray-200 bg-gray-50 px-4 py-3">
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
      </div>
    </section>
  );
}

function InspectorTabButton({
  active,
  label,
  onClick,
}: {
  active: boolean;
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      className={`rounded-full px-3 py-1.5 text-sm font-medium transition focus:outline-none focus:ring-2 focus:ring-blue-200 ${
        active
          ? 'bg-gray-900 text-white'
          : 'bg-white text-gray-600 ring-1 ring-gray-200 hover:bg-gray-100'
      }`}
      aria-pressed={active}
      onClick={onClick}
    >
      {label}
    </button>
  );
}
