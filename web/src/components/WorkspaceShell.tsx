import type { ReactNode } from 'react';
import { Group, Panel, Separator } from 'react-resizable-panels';

export type MobileWorkspaceTabId =
  | 'details'
  | 'waterfall'
  | 'tree'
  | 'reasoning'
  | 'timeline'
  | 'state';

interface WorkspaceShellProps {
  isDesktop: boolean;
  treeRail: ReactNode;
  waterfall: ReactNode;
  inspector: ReactNode;
  mobileDetails: ReactNode;
  mobileReasoning: ReactNode;
  mobileTimeline: ReactNode;
  mobileState: ReactNode;
  activeMobileTab: MobileWorkspaceTabId;
  onMobileTabChange: (tab: MobileWorkspaceTabId) => void;
}

const MOBILE_TABS: Array<{ id: MobileWorkspaceTabId; label: string }> = [
  { id: 'details', label: 'Details' },
  { id: 'waterfall', label: 'Waterfall' },
  { id: 'tree', label: 'Tree' },
  { id: 'timeline', label: 'Timeline' },
  { id: 'reasoning', label: 'Reasoning' },
  { id: 'state', label: 'State' },
];
const USE_STATIC_DESKTOP_LAYOUT =
  typeof navigator !== 'undefined' && /\bjsdom\b/i.test(navigator.userAgent);

export function WorkspaceShell({
  isDesktop,
  treeRail,
  waterfall,
  inspector,
  mobileDetails,
  mobileReasoning,
  mobileTimeline,
  mobileState,
  activeMobileTab,
  onMobileTabChange,
}: WorkspaceShellProps) {
  if (isDesktop) {
    if (USE_STATIC_DESKTOP_LAYOUT) {
      return (
        <div className="grid min-h-0 flex-1 grid-cols-[minmax(18rem,28%)_minmax(0,1fr)] gap-4">
          <div className="min-h-0">{treeRail}</div>
          <div className="grid min-h-0 grid-rows-[minmax(14rem,48%)_minmax(16rem,52%)] gap-4">
            <div className="min-h-0">{waterfall}</div>
            <div className="min-h-0">{inspector}</div>
          </div>
        </div>
      );
    }

    return (
      <div className="min-h-0 flex-1">
        <Group orientation="horizontal" className="h-full">
          <Panel defaultSize={28} minSize={18}>
            <div className="h-full pr-2">{treeRail}</div>
          </Panel>
          <ResizeHandle />
          <Panel defaultSize={72} minSize={40}>
            <Group orientation="vertical" className="h-full">
              <Panel defaultSize={48} minSize={24}>
                <div className="h-full pl-2 pb-2">{waterfall}</div>
              </Panel>
              <ResizeHandle horizontal />
              <Panel defaultSize={52} minSize={24}>
                <div className="h-full pl-2 pt-2">{inspector}</div>
              </Panel>
            </Group>
          </Panel>
        </Group>
      </div>
    );
  }

  return (
    <section className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm dark:border-slate-800 dark:bg-slate-900">
      <div className="border-b border-slate-200 bg-slate-50 px-3 py-3 dark:border-slate-800 dark:bg-slate-950/70">
        <div className="flex flex-wrap items-center gap-2">
          {MOBILE_TABS.map((tab) => (
            <button
              key={tab.id}
              type="button"
              className={`rounded-full px-3 py-1.5 text-sm font-medium transition focus:outline-none focus:ring-2 focus:ring-blue-200 ${
                activeMobileTab === tab.id
                  ? 'bg-slate-900 text-white dark:bg-slate-100 dark:text-slate-950'
                  : 'bg-white text-slate-600 ring-1 ring-slate-200 hover:bg-slate-100 dark:bg-slate-900 dark:text-slate-300 dark:ring-slate-700 dark:hover:bg-slate-800'
              }`}
              aria-pressed={activeMobileTab === tab.id}
              onClick={() => onMobileTabChange(tab.id)}
            >
              {tab.label}
            </button>
          ))}
        </div>
      </div>

      <div className="min-h-0 flex-1">
        <div
          className="h-full"
          style={{ display: activeMobileTab === 'details' ? 'block' : 'none' }}
        >
          {mobileDetails}
        </div>
        <div
          className="h-full"
          style={{ display: activeMobileTab === 'waterfall' ? 'block' : 'none' }}
        >
          {waterfall}
        </div>
        <div
          className="h-full"
          style={{ display: activeMobileTab === 'tree' ? 'block' : 'none' }}
        >
          {treeRail}
        </div>
        <div
          className="h-full"
          style={{ display: activeMobileTab === 'timeline' ? 'block' : 'none' }}
        >
          {mobileTimeline}
        </div>
        <div
          className="h-full"
          style={{ display: activeMobileTab === 'reasoning' ? 'block' : 'none' }}
        >
          {mobileReasoning}
        </div>
        <div
          className="h-full"
          style={{ display: activeMobileTab === 'state' ? 'block' : 'none' }}
        >
          {mobileState}
        </div>
      </div>
    </section>
  );
}

function ResizeHandle({ horizontal = false }: { horizontal?: boolean }) {
  return (
    <Separator
      className={`group relative flex items-center justify-center ${
        horizontal ? 'h-3' : 'w-3'
      }`}
    >
      <div
        className={`rounded-full transition group-hover:opacity-90 ${
          horizontal ? 'h-1 w-10' : 'h-10 w-1'
        }`}
        style={{ backgroundColor: 'var(--continua-panel-separator)' }}
      />
    </Separator>
  );
}
