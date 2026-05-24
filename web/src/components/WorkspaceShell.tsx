import { useState, type ReactNode } from 'react';

export type MobileWorkspaceTabId =
  | 'summary'
  | 'execution';

interface WorkspaceShellProps {
  isDesktop: boolean;
  treeRail: ReactNode;
  waterfall: ReactNode;
  inspector: ReactNode;
  mobileSummary: ReactNode;
  activeMobileTab: MobileWorkspaceTabId;
  onMobileTabChange: (tab: MobileWorkspaceTabId) => void;
}

const MOBILE_TABS: Array<{ id: MobileWorkspaceTabId; label: string }> = [
  { id: 'summary', label: 'Summary' },
  { id: 'execution', label: 'Execution' },
];
export function WorkspaceShell({
  isDesktop,
  treeRail,
  waterfall,
  inspector,
  mobileSummary,
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
    <section className="flex min-h-0 flex-1 flex-col overflow-hidden bg-[var(--c-app-bg)]">
      <nav
        aria-label="Mobile trace workspace"
        className="border-b border-[var(--c-border)] bg-[var(--c-app-bg)] px-4"
      >
        <div className="flex overflow-x-auto">
          {MOBILE_TABS.map((tab) => (
            <button
              key={tab.id}
              type="button"
              className={`-mb-px shrink-0 border-b-2 px-3.5 py-2 text-[13px] font-medium transition focus:outline-none focus:ring-2 focus:ring-[var(--c-accent-faint)] ${
                activeMobileTab === tab.id
                  ? 'border-[var(--c-accent)] text-[var(--c-text-primary)]'
                  : 'border-transparent text-[var(--c-text-secondary)] hover:text-[var(--c-text-primary)]'
              }`}
              aria-pressed={activeMobileTab === tab.id}
              onClick={() => onMobileTabChange(tab.id)}
            >
              {tab.label}
            </button>
          ))}
        </div>
      </nav>

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
    <div className="h-full flex-col" style={{ display: active ? 'flex' : 'none' }}>
      <div className="border-b border-[var(--c-border)] bg-[var(--c-app-bg)] px-4">
        <div className="flex overflow-x-auto">
          {(['waterfall', 'tree'] as const).map((nextMode) => (
            <button
              key={nextMode}
              type="button"
              aria-pressed={mode === nextMode}
              onClick={() => setMode(nextMode)}
              className={`-mb-px shrink-0 border-b-2 px-3.5 py-2 text-xs font-medium transition focus:outline-none focus:ring-2 focus:ring-[var(--c-accent-faint)] ${
                mode === nextMode
                  ? 'border-[var(--c-accent)] text-[var(--c-text-primary)]'
                  : 'border-transparent text-[var(--c-text-secondary)] hover:text-[var(--c-text-primary)]'
              }`}
            >
              {nextMode === 'waterfall' ? 'Waterfall' : 'Tree'}
            </button>
          ))}
        </div>
      </div>

      <div className="min-h-0 flex-1">
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
