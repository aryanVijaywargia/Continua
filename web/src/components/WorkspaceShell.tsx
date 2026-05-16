import { useState, type ReactNode } from 'react';

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
    return (
      <div className="grid h-full min-h-0 flex-1 grid-cols-[minmax(0,1fr)_minmax(22rem,28rem)]">
        <div className="min-h-0 min-w-0">{waterfall}</div>
        <div className="min-h-0 min-w-0 border-l border-[var(--c-border)]">{inspector}</div>
      </div>
    );
  }

  return (
    <section className="flex min-h-0 flex-1 flex-col overflow-hidden border border-[var(--c-border)] bg-[var(--c-surface)]">
      <div className="border-b border-[var(--c-border)] bg-[var(--c-surface-muted)] px-3 py-3">
        <div className="flex flex-wrap items-center gap-2">
          {MOBILE_TABS.map((tab) => (
            <button
              key={tab.id}
              type="button"
              className={`rounded-full px-3 py-1.5 text-sm font-medium transition focus:outline-none focus:ring-2 focus:ring-[var(--continua-accent-faint)] ${
                activeMobileTab === tab.id
                  ? 'border border-[var(--c-accent-border)] bg-[var(--c-accent-faint)] text-[var(--c-accent-text)]'
                  : 'border border-[var(--c-border)] bg-[var(--c-surface)] text-[var(--c-text-secondary)] hover:border-[var(--c-border-strong)] hover:text-[var(--c-text-primary)]'
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
      <div className="border-b border-[var(--c-border)] bg-[var(--c-surface-muted)] px-3 py-2">
        <div className="flex flex-wrap items-center gap-2">
          {(['waterfall', 'tree'] as const).map((nextMode) => (
            <button
              key={nextMode}
              type="button"
              aria-pressed={mode === nextMode}
              onClick={() => setMode(nextMode)}
              className={`rounded-full px-3 py-1.5 text-sm font-medium transition focus:outline-none focus:ring-2 focus:ring-[var(--continua-accent-faint)] ${
                mode === nextMode
                  ? 'border border-[var(--c-accent-border)] bg-[var(--c-accent-faint)] text-[var(--c-accent-text)]'
                  : 'border border-[var(--c-border)] bg-[var(--c-surface)] text-[var(--c-text-secondary)]'
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
