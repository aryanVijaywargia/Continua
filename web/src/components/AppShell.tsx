import { useQuery } from '@tanstack/react-query';
import { useEffect, useMemo, useState } from 'react';
import {
  Activity,
  Command,
  LayoutDashboard,
  Menu,
  Settings2,
  Waypoints,
  X,
} from 'lucide-react';
import { NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom';
import {
  fetchProjects,
  LOCAL_API_KEY_CHANGED_EVENT,
  setSelectedProjectIdProvider,
} from '../api/client';
import { useOperatorAuth, useRuntimeAuth } from '../auth/runtime';
import { useMediaQuery } from '../hooks/useMediaQuery';
import { CommandPalette } from './CommandPalette';
import {
  buildProjectPath,
  getProjectIdFromSearchParams,
} from '../utils/projectSearchParams';

interface NavItem {
  path: string;
  label: string;
  detail: string;
  icon: typeof LayoutDashboard;
}

const RUN_LOCALLY_DOCS_URL =
  'https://github.com/aryanVijaywargia/Continua/blob/main/docs/guides/run-locally.md';

const NAV_ITEMS: NavItem[] = [
  {
    path: '/dashboard',
    label: 'Overview',
    detail: 'Snapshot of active traces and sessions',
    icon: LayoutDashboard,
  },
  {
    path: '/traces',
    label: 'Traces',
    detail: 'Triage individual runs and failures',
    icon: Activity,
  },
  {
    path: '/sessions',
    label: 'Sessions',
    detail: 'Follow user workflows across traces',
    icon: Waypoints,
  },
  {
    path: '/settings',
    label: 'Settings',
    detail: 'Theme and operator session controls',
    icon: Settings2,
  },
];

export function AppShell() {
  const location = useLocation();
  const navigate = useNavigate();
  const runtimeAuth = useRuntimeAuth();
  const { isAuthenticated, user } = useOperatorAuth();
  const isPublicDemo = runtimeAuth.public_demo_enabled === true;
  const publicDemoLabel = runtimeAuth.public_demo_label ?? 'Sample data';
  const isPrimaryNavVisible = useMediaQuery('(min-width: 768px)');
  const [mobileOpen, setMobileOpen] = useState(false);
  const [lastProjectId, setLastProjectId] = useState<string | undefined>();
  const [localAuthVersion, setLocalAuthVersion] = useState(0);
  const currentProjectId = getProjectIdFromSearchParams(
    new URLSearchParams(location.search)
  );
  const projectsQuery = useQuery({
    queryKey: ['projects', localAuthVersion],
    queryFn: fetchProjects,
    enabled: isAuthenticated && !isPublicDemo,
  });
  const projects = projectsQuery.data?.projects ?? [];
  const effectiveProjectId = currentProjectId ?? lastProjectId;
  const selectedProject =
    projects.find((project) => project.id === effectiveProjectId) ??
    projects[0] ??
    null;
  const selectedProjectId = selectedProject?.id;

  useEffect(() => {
    setMobileOpen(false);
  }, [location.pathname, location.search]);

  useEffect(() => {
    const handleLocalAuthChange = () => {
      setLastProjectId(undefined);
      setLocalAuthVersion((version) => version + 1);
    };

    window.addEventListener(LOCAL_API_KEY_CHANGED_EVENT, handleLocalAuthChange);
    return () => {
      window.removeEventListener(LOCAL_API_KEY_CHANGED_EVENT, handleLocalAuthChange);
    };
  }, []);

  useEffect(() => {
    if (selectedProjectId) {
      setLastProjectId(selectedProjectId);
    }
  }, [selectedProjectId]);

  useEffect(() => {
    if (isPublicDemo) {
      setSelectedProjectIdProvider(null);
      return () => {
        setSelectedProjectIdProvider(null);
      };
    }

    setSelectedProjectIdProvider(() => selectedProjectId ?? null);

    return () => {
      setSelectedProjectIdProvider(null);
    };
  }, [isPublicDemo, selectedProjectId]);

  useEffect(() => {
    if (
      isPublicDemo ||
      !projectsQuery.isSuccess ||
      projects.length === 0 ||
      !selectedProjectId ||
      currentProjectId === selectedProjectId
    ) {
      return;
    }

    const nextParams = new URLSearchParams(location.search);
    nextParams.set('project_id', selectedProjectId);

    navigate(
      {
        pathname: location.pathname,
        search: `?${nextParams.toString()}`,
      },
      { replace: true }
    );
  }, [
    currentProjectId,
    isPublicDemo,
    location.pathname,
    location.search,
    navigate,
    projects.length,
    projectsQuery.isSuccess,
    selectedProjectId,
  ]);

  const visibleNavItems = useMemo(
    () => NAV_ITEMS.filter((item) => !(isPublicDemo && item.path === '/settings')),
    [isPublicDemo]
  );

  const commands = useMemo(() => {
    const baseCommands = [
      {
        id: 'go-overview',
        title: 'Go to Overview',
        keywords: ['home', 'overview', 'dashboard'],
        action: () => navigate(buildProjectPath('/dashboard', selectedProjectId)),
      },
      {
        id: 'go-traces',
        title: 'Go to Traces',
        keywords: ['navigate', 'traces'],
        action: () => navigate(buildProjectPath('/traces', selectedProjectId)),
      },
      {
        id: 'go-sessions',
        title: 'Go to Sessions',
        keywords: ['navigate', 'sessions'],
        action: () => navigate(buildProjectPath('/sessions', selectedProjectId)),
      },
    ];

    if (isPublicDemo) {
      return baseCommands;
    }

    return [
      ...baseCommands,
      {
        id: 'go-settings',
        title: 'Go to Settings',
        keywords: ['navigate', 'settings', 'theme', 'auth'],
        action: () => navigate(buildProjectPath('/settings', selectedProjectId)),
      },
    ];
  }, [isPublicDemo, navigate, selectedProjectId]);

  const handleProjectChange = (projectId: string) => {
    const nextParams = new URLSearchParams(location.search);
    nextParams.set('project_id', projectId);

    navigate(
      {
        pathname: location.pathname,
        search: `?${nextParams.toString()}`,
      },
      { replace: false }
    );
  };

  const projectsReady = isPublicDemo
    ? true
    : projectsQuery.isSuccess &&
      (projects.length === 0 ||
        (selectedProject !== null && currentProjectId === selectedProject.id));
  const projectsErrorMessage =
    projectsQuery.error instanceof Error
      ? projectsQuery.error.message
      : 'Failed to load projects.';

  return (
    <div className="app-shell-enter min-h-screen bg-[var(--continua-app-bg)] text-[var(--continua-text-primary)]">
      <nav className="fixed top-0 z-50 w-full border-b border-[var(--continua-border-soft)] bg-[var(--continua-shell-topbar)] backdrop-blur-xl">
        <div className="mx-auto flex h-16 max-w-7xl items-center justify-between gap-4 px-6">
          <div className="flex items-center gap-8">
            <NavLink
              to="/"
              className="text-xl font-black tracking-tighter text-[var(--continua-text-primary)]"
            >
              Continua
            </NavLink>

            <div
              className="hidden items-center gap-1 md:flex"
              role="navigation"
              aria-label="Primary"
            >
              {visibleNavItems.map((item) => (
                <ShellNavLink
                  key={item.path}
                  item={item}
                  projectId={selectedProject?.id}
                />
              ))}
            </div>
          </div>

          <div className="flex items-center gap-2 md:gap-3">
            {!isPublicDemo ? (
              <div className="hidden items-center gap-2 md:flex">
                <label className="sr-only" htmlFor="project-switcher">
                  Active project
                </label>
                <select
                  id="project-switcher"
                  value={selectedProject?.id ?? ''}
                  onChange={(event) => handleProjectChange(event.target.value)}
                  disabled={!projectsReady || projects.length === 0}
                  className="app-input min-w-[11rem] py-2 text-sm lg:min-w-[13rem]"
                >
                  {projects.length === 0 ? (
                    <option value="">
                      {projectsQuery.isPending ? 'Loading projects...' : 'No projects'}
                    </option>
                  ) : null}
                  {projects.map((project) => (
                    <option key={project.id} value={project.id}>
                      {project.name}
                    </option>
                  ))}
                </select>
              </div>
            ) : null}

            <div className="hidden items-center gap-2 xl:flex">
              {!isPublicDemo ? (
                <span className="inline-flex items-center gap-2 rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-3 py-1.5 text-xs font-medium text-[var(--continua-text-secondary)]">
                  <Command className="h-3.5 w-3.5 text-[var(--continua-accent)]" />
                  <span>{user?.email ?? 'Signed in'}</span>
                </span>
              ) : null}

              <CommandPalette commands={commands} />
            </div>

            <button
              type="button"
              aria-label={
                !isPublicDemo && isPrimaryNavVisible
                  ? 'Open operator tools'
                  : 'Open navigation'
              }
              className="inline-flex h-10 items-center justify-center gap-2 rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-3 text-[var(--continua-text-secondary)] transition hover:text-[var(--continua-text-primary)] xl:hidden"
              onClick={() => setMobileOpen(true)}
            >
              <Menu className="h-4 w-4" />
              {!isPublicDemo && isPrimaryNavVisible ? (
                <span className="text-sm font-medium">Tools</span>
              ) : null}
            </button>
          </div>
        </div>
      </nav>

      {mobileOpen ? (
        <div className="app-overlay-enter fixed inset-0 z-[60] xl:hidden">
          <button
            type="button"
            aria-label={
              !isPublicDemo && isPrimaryNavVisible
                ? 'Close operator tools'
                : 'Close navigation'
            }
            className="absolute inset-0 bg-[var(--continua-text-primary)]/50 backdrop-blur-sm"
            onClick={() => setMobileOpen(false)}
          />
          <aside className="app-drawer-enter relative flex h-full w-[19rem] max-w-[88vw] flex-col border-r border-[var(--continua-border-strong)] bg-[var(--continua-app-bg)] px-6 py-5 shadow-2xl">
            <div className="flex items-center justify-between">
              <span className="text-xl font-black tracking-tighter text-[var(--continua-text-primary)]">
                {!isPublicDemo && isPrimaryNavVisible ? 'Operator tools' : 'Continua'}
              </span>
              <button
                type="button"
                aria-label={
                  !isPublicDemo && isPrimaryNavVisible
                    ? 'Close operator tools'
                    : 'Close navigation'
                }
                className="inline-flex h-10 w-10 items-center justify-center rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] text-[var(--continua-text-secondary)] transition hover:text-[var(--continua-text-primary)]"
                onClick={() => setMobileOpen(false)}
              >
                <X className="h-4 w-4" />
              </button>
            </div>

            {!isPublicDemo && isPrimaryNavVisible ? (
              <div className="mt-6 flex flex-1 flex-col gap-4">
                <div className="rounded-2xl border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-4 py-4">
                  <p className="text-xs font-semibold uppercase tracking-[0.16em] text-[var(--continua-text-muted)]">
                    Signed in
                  </p>
                  <p className="mt-2 text-sm font-semibold text-[var(--continua-text-primary)]">
                    {user?.email ?? 'Signed in'}
                  </p>
                  <p className="mt-3 text-sm leading-relaxed text-[var(--continua-text-secondary)]">
                    Use the top bar to switch projects and move between views.
                    Open the command palette here for fast jumps and keyboard shortcuts.
                  </p>
                </div>

                <div className="rounded-2xl border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-4 py-4">
                  <p className="text-xs font-semibold uppercase tracking-[0.16em] text-[var(--continua-text-muted)]">
                    Active project
                  </p>
                  <p className="mt-2 text-sm font-semibold text-[var(--continua-text-primary)]">
                    {selectedProject?.name ??
                      (projectsQuery.isPending ? 'Loading project...' : 'No project selected')}
                  </p>
                </div>

                <div className="mt-auto">
                  <CommandPalette commands={commands} />
                </div>
              </div>
            ) : (
              <>
                {!isPublicDemo ? (
                  <div className="mt-6">
                    <label
                      className="mb-1 block text-xs font-semibold uppercase tracking-[0.16em] text-[var(--continua-text-muted)]"
                      htmlFor="mobile-project-switcher"
                    >
                      Project
                    </label>
                    <select
                      id="mobile-project-switcher"
                      value={selectedProject?.id ?? ''}
                      onChange={(event) => handleProjectChange(event.target.value)}
                      disabled={!projectsReady || projects.length === 0}
                      className="app-input w-full py-2 text-sm"
                    >
                      {projects.length === 0 ? (
                        <option value="">
                          {projectsQuery.isPending ? 'Loading projects...' : 'No projects'}
                        </option>
                      ) : null}
                      {projects.map((project) => (
                        <option key={project.id} value={project.id}>
                          {project.name}
                        </option>
                      ))}
                    </select>
                  </div>
                ) : (
                  <div className="mt-6 rounded-2xl border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-4 py-4">
                    <p className="text-xs font-semibold uppercase tracking-[0.16em] text-[var(--continua-text-muted)]">
                      {publicDemoLabel}
                    </p>
                    <p className="mt-2 text-sm leading-relaxed text-[var(--continua-text-secondary)]">
                      Read-only sample traces are available here. Run Continua locally to inspect your own data.
                    </p>
                  </div>
                )}

                <nav className="mt-8 flex flex-1 flex-col gap-1" aria-label="Mobile primary">
                  {visibleNavItems.map((item) => (
                    <MobileNavLink
                      key={item.path}
                      item={item}
                      projectId={selectedProject?.id}
                    />
                  ))}
                </nav>

                <div className="mt-auto space-y-3">
                  {!isPublicDemo && user?.email ? (
                    <div className="rounded-2xl border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-4 py-3 text-sm text-[var(--continua-text-secondary)]">
                      {user.email}
                    </div>
                  ) : null}
                  <CommandPalette commands={commands} />
                </div>
              </>
            )}
          </aside>
        </div>
      ) : null}

      <main className="mx-auto min-h-screen max-w-7xl px-6 pt-24 pb-16">
        {isPublicDemo ? (
          <PublicDemoBanner
            label={publicDemoLabel}
            docsHref={RUN_LOCALLY_DOCS_URL}
          />
        ) : null}

        {isPublicDemo ? (
          <Outlet />
        ) : !isAuthenticated ? (
          <ShellStateCard
            title="Local API key required"
            message="Enter a project API key to load the local debugger workspace."
          />
        ) : projectsQuery.isError ? (
          <ShellStateCard
            title="Project loading failed"
            message={projectsErrorMessage}
          />
        ) : projectsQuery.isPending || !projectsReady ? (
          <ShellStateCard
            title="Loading projects"
            message="Resolving the project list for this operator session."
          />
        ) : projects.length === 0 ? (
          <ShellStateCard
            title="No projects available"
            message="Create or ingest into a project first, then reload the debugger."
          />
        ) : (
          <Outlet />
        )}
      </main>
    </div>
  );
}

function PublicDemoBanner({
  label,
  docsHref,
}: {
  label: string;
  docsHref: string;
}) {
  return (
    <section className="mb-6 flex flex-col gap-3 rounded-2xl border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <div className="text-xs font-semibold uppercase tracking-[0.16em] text-[var(--continua-text-muted)]">
          {label}
        </div>
        <p className="mt-2 text-sm leading-6 text-[var(--continua-text-secondary)]">
          Read-only demo: sample traces are hosted here for exploration. Run Continua locally to inspect your own traces.
        </p>
      </div>
      <a
        href={docsHref}
        target="_blank"
        rel="noreferrer"
        className="app-button-secondary whitespace-nowrap"
      >
        Run locally with your own traces
      </a>
    </section>
  );
}

function ShellStateCard({
  title,
  message,
}: {
  title: string;
  message: string;
}) {
  return (
    <section className="app-surface max-w-3xl p-8">
      <div className="app-overline">Operator workspace</div>
      <h1 className="mt-3 text-3xl font-black tight-headline text-[var(--continua-text-primary)]">
        {title}
      </h1>
      <p className="mt-4 text-sm leading-7 text-[var(--continua-text-secondary)] sm:text-base">
        {message}
      </p>
    </section>
  );
}

function ShellNavLink({
  item,
  projectId,
}: {
  item: NavItem;
  projectId?: string;
}) {
  return (
    <NavLink
      to={buildProjectPath(item.path, projectId)}
      end={item.path === '/dashboard'}
      className={({ isActive }) =>
        `rounded-full px-4 py-2 text-sm font-bold tracking-tight transition ${
          isActive
            ? 'border-b-2 border-[var(--continua-text-primary)] text-[var(--continua-text-primary)]'
            : 'text-[var(--continua-text-muted)] hover:text-[var(--continua-text-primary)]'
        }`
      }
    >
      {item.label}
    </NavLink>
  );
}

function MobileNavLink({
  item,
  projectId,
}: {
  item: NavItem;
  projectId?: string;
}) {
  const Icon = item.icon;

  return (
    <NavLink
      to={buildProjectPath(item.path, projectId)}
      end={item.path === '/dashboard'}
      className={({ isActive }) =>
        `flex items-center gap-3 rounded-xl px-4 py-3 text-sm font-bold transition ${
          isActive
            ? 'bg-[var(--continua-surface-elevated)] text-[var(--continua-text-primary)]'
            : 'text-[var(--continua-text-muted)] hover:bg-[var(--continua-surface-muted)] hover:text-[var(--continua-text-primary)]'
        }`
      }
    >
      <Icon className="h-4 w-4" />
      <span>{item.label}</span>
    </NavLink>
  );
}
