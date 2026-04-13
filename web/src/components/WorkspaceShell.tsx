import { useState, type ReactNode } from 'react';
import { Group, Panel, Separator } from 'react-resizable-panels';

export type MobileWorkspaceTabId =
  | 'summary'
  | 'execution'
  | 'timeline'
  | 'state';

interface WorkspaceShellProps {
  isDesktop: boolean;
  treeRail: ReactNode;
  waterfall: ReactNode;
  inspector: ReactNode;
  mobileSummary: ReactNode;
  mobileTimeline: ReactNode;
  mobileState: ReactNode;
  activeMobileTab: MobileWorkspaceTabId;
  onMobileTabChange: (tab: MobileWorkspaceTabId) => void;
}

const MOBILE_TABS: Array<{ id: MobileWorkspaceTabId; label: string }> = [
  { id: 'summary', label: 'Summary' },
  { id: 'execution', label: 'Execution' },
  { id: 'timeline', label: 'Timeline' },
  { id: 'state', label: 'State' },
];
const USE_STATIC_DESKTOP_LAYOUT =
  typeof navigator !== 'undefined' && /\bjsdom\b/i.test(navigator.userAgent);

export function WorkspaceShell({
  isDesktop,
  treeRail,
  waterfall,
  inspector,
  mobileSummary,
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
    <section className="flex min-h-0 flex-1 flex-col overflow-hidden rounded-[1rem] border border-[var(--continua-border-strong)] bg-[var(--continua-surface)] shadow-[var(--continua-shadow-soft)]">
      <div className="border-b border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-3 py-3">
        <div className="flex flex-wrap items-center gap-2">
          {MOBILE_TABS.map((tab) => (
            <button
              key={tab.id}
              type="button"
              className={`rounded-full px-3 py-1.5 text-sm font-medium transition focus:outline-none focus:ring-2 focus:ring-[var(--continua-accent-faint)] ${
                activeMobileTab === tab.id
                  ? 'border border-[var(--continua-accent)] bg-[var(--continua-accent-faint)] text-[var(--continua-accent)]'
                  : 'border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] text-[var(--continua-text-secondary)] hover:border-[var(--continua-border-strong)] hover:text-[var(--continua-text-primary)]'
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
          style={{ display: activeMobileTab === 'summary' ? 'block' : 'none' }}
        >
          {mobileSummary}
        </div>
        <MobileExecutionPane
          active={activeMobileTab === 'execution'}
          treeRail={treeRail}
          waterfall={waterfall}
        />
        <div
          className="h-full"
          style={{ display: activeMobileTab === 'timeline' ? 'block' : 'none' }}
        >
          {mobileTimeline}
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

function MobileExecutionPane({
  active,
  treeRail,
  waterfall,
}: {
  active: boolean;
  treeRail: ReactNode;
  waterfall: ReactNode;
}) {
  const [mode, setMode] = useState<'waterfall' | 'tree'>('waterfall');

  return (
    <div className="h-full" style={{ display: active ? 'block' : 'none' }}>
      <div className="border-b border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] px-3 py-2">
        <div className="flex flex-wrap items-center gap-2">
          {(['waterfall', 'tree'] as const).map((nextMode) => (
            <button
              key={nextMode}
              type="button"
              aria-pressed={mode === nextMode}
              onClick={() => setMode(nextMode)}
              className={`rounded-full px-3 py-1.5 text-sm font-medium transition focus:outline-none focus:ring-2 focus:ring-[var(--continua-accent-faint)] ${
                mode === nextMode
                  ? 'border border-[var(--continua-accent)] bg-[var(--continua-accent-faint)] text-[var(--continua-accent)]'
                  : 'border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] text-[var(--continua-text-secondary)]'
              }`}
            >
              {nextMode === 'waterfall' ? 'Waterfall' : 'Tree'}
            </button>
          ))}
        </div>
      </div>

      <div className="h-[calc(100%-3.5rem)]">
        <div className="h-full" style={{ display: mode === 'waterfall' ? 'block' : 'none' }}>
          {waterfall}
        </div>
        <div className="h-full" style={{ display: mode === 'tree' ? 'block' : 'none' }}>
          {treeRail}
        </div>
      </div>
    </div>
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
