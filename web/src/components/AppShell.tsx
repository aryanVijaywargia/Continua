import { useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import {
  Activity,
  ChevronDown,
  ChevronRight,
  FolderKanban,
  LayoutDashboard,
  Menu,
  Moon,
  MoreHorizontal,
  Settings2,
  Sun,
  Waypoints,
  Workflow,
  X,
  type LucideIcon,
} from 'lucide-react';
import { Link, NavLink, Outlet, useLocation, useNavigate } from 'react-router-dom';
import {
  clearApiKey,
  fetchProjects,
  getFallbackProjectApiKey,
  getApiKey,
  getKnownProjectApiKey,
  isAuthError,
  LOCAL_API_KEY_CHANGED_EVENT,
  rememberProjectApiKey,
  setApiKey,
  setSelectedProjectIdProvider,
} from '../api/client';
import { useOperatorAuth, useRuntimeAuth } from '../auth/runtime';
import { useTheme } from '../hooks/useTheme';
import { CommandPalette } from './CommandPalette';
import {
  buildProjectPath,
  getProjectIdFromSearchParams,
} from '../utils/projectSearchParams';

interface NavItem {
  path: string;
  label: string;
  icon: LucideIcon;
}

const RUN_LOCALLY_DOCS_URL =
  'https://www.continua.in/docs/guides/installation';

const NAV_ITEMS: NavItem[] = [
  { path: '/dashboard', label: 'Overview', icon: LayoutDashboard },
  { path: '/traces', label: 'Traces', icon: Activity },
  { path: '/engine/runs', label: 'Engine Runs', icon: Workflow },
  { path: '/sessions', label: 'Sessions', icon: Waypoints },
  { path: '/projects', label: 'Projects', icon: FolderKanban },
  { path: '/settings', label: 'Settings', icon: Settings2 },
];

export function AppShell() {
  const location = useLocation();
  const navigate = useNavigate();
  const runtimeAuth = useRuntimeAuth();
  const { isAuthenticated, user } = useOperatorAuth();
  const { resolvedTheme, toggleTheme } = useTheme();
  const isPublicDemo = runtimeAuth.public_demo_enabled === true;
  const publicDemoLabel = runtimeAuth.public_demo_label ?? 'Sample data';
  const isProjectsRoute = location.pathname === '/projects';
  const canAttemptProjectBootstrap =
    runtimeAuth.status === 'ready' &&
    !runtimeAuth.enabled &&
    !isAuthenticated &&
    isProjectsRoute;
  const [mobileOpen, setMobileOpen] = useState(false);
  const [lastProjectId, setLastProjectId] = useState<string | undefined>();
  const [localAuthVersion, setLocalAuthVersion] = useState(0);
  const currentProjectId = getProjectIdFromSearchParams(
    new URLSearchParams(location.search)
  );
  const projectsQuery = useQuery({
    queryKey: ['projects', localAuthVersion],
    queryFn: fetchProjects,
    enabled: !isPublicDemo && (isAuthenticated || canAttemptProjectBootstrap),
    retry: false,
  });
  const projects = useMemo(
    () => projectsQuery.data?.projects ?? [],
    [projectsQuery.data?.projects]
  );
  const effectiveProjectId = currentProjectId ?? lastProjectId;
  const selectedProject =
    projects.find((project) => project.id === effectiveProjectId) ??
    projects[0] ??
    null;
  const selectedProjectId = selectedProject?.id;

  useEffect(() => {
    setMobileOpen(false);
  }, [location.pathname, location.search]);

  const queryClient = useQueryClient();
  useEffect(() => {
    const handleLocalAuthChange = () => {
      setLastProjectId(undefined);
      setLocalAuthVersion((version) => version + 1);
      // Cascade the auth change to every cache entry that scopes data by the
      // current API key so page-level queries (e.g. ProjectsPage's separate
      // ['projects'] query) refetch under the new auth instead of staying
      // stuck on a stale 401.
      queryClient.invalidateQueries({ queryKey: ['projects'] });
    };

    window.addEventListener(LOCAL_API_KEY_CHANGED_EVENT, handleLocalAuthChange);
    return () => {
      window.removeEventListener(LOCAL_API_KEY_CHANGED_EVENT, handleLocalAuthChange);
    };
  }, [queryClient]);

  const staleKeyClearedFor = useRef<number | null>(null);
  const isLocalMode = !isPublicDemo && !runtimeAuth.enabled;
  useEffect(() => {
    if (!isLocalMode || !projectsQuery.error) {
      return;
    }
    if (!isAuthError(projectsQuery.error)) {
      return;
    }
    if (staleKeyClearedFor.current === localAuthVersion) {
      return;
    }
    if (!getApiKey()) {
      return;
    }
    staleKeyClearedFor.current = localAuthVersion;
    const currentLocalKey = getApiKey();
    const fallbackKey = getFallbackProjectApiKey(currentLocalKey);
    if (fallbackKey) {
      setApiKey(fallbackKey);
      return;
    }
    clearApiKey();
  }, [
    isLocalMode,
    localAuthVersion,
    projectsQuery.error,
  ]);

  useEffect(() => {
    if (selectedProjectId) {
      setLastProjectId(selectedProjectId);
    }
  }, [selectedProjectId]);

  useEffect(() => {
    if (!isLocalMode || !projectsQuery.isSuccess) {
      return;
    }

    const currentLocalKey = getApiKey();
    if (!currentLocalKey) {
      return;
    }

    const authenticatedProjectId =
      projectsQuery.data.authenticated_project_id ??
      (projects.length === 1 ? projects[0]?.id : undefined);
    if (
      !authenticatedProjectId ||
      getKnownProjectApiKey(authenticatedProjectId) === currentLocalKey
    ) {
      return;
    }

    // The API identifies the project that owns the active local key. The
    // single-project fallback covers older servers that do not expose that
    // metadata yet.
    rememberProjectApiKey(authenticatedProjectId, currentLocalKey);
  }, [isLocalMode, projects, projectsQuery.data, projectsQuery.isSuccess]);

  useEffect(() => {
    if (isPublicDemo) {
      setSelectedProjectIdProvider(null);
      return () => setSelectedProjectIdProvider(null);
    }

    setSelectedProjectIdProvider(() => selectedProjectId ?? null);
    return () => setSelectedProjectIdProvider(null);
  }, [isPublicDemo, selectedProjectId]);

  useEffect(() => {
    if (
      isPublicDemo ||
      !projectsQuery.isSuccess ||
      !selectedProjectId ||
      projects.length === 0 ||
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

  useEffect(() => {
    if (
      isPublicDemo ||
      !projectsQuery.isSuccess ||
      projects.length !== 0 ||
      isProjectsRoute
    ) {
      return;
    }

    navigate('/projects', { replace: true });
  }, [
    isProjectsRoute,
    isPublicDemo,
    navigate,
    projects.length,
    projectsQuery.isSuccess,
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
      {
        id: 'go-projects',
        title: 'Go to Projects',
        keywords: ['navigate', 'projects', 'keys'],
        action: () => navigate(buildProjectPath('/projects', selectedProjectId)),
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
  const projectsReady = isPublicDemo
    ? true
    : projectsQuery.isSuccess &&
      (projects.length === 0 || selectedProject !== null);
  const projectsErrorMessage =
    projectsQuery.error instanceof Error
      ? projectsQuery.error.message
      : 'Failed to load projects.';

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

  const content = isPublicDemo ? (
    <Outlet />
  ) : canAttemptProjectBootstrap ? (
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
      actions={
        <>
          <button
            type="button"
            className="app-button-primary"
            onClick={() => {
              void projectsQuery.refetch();
            }}
          >
            Retry
          </button>
          <Link to="/settings" className="app-button-secondary">
            Open settings
          </Link>
        </>
      }
    />
  ) : projectsQuery.isPending || !projectsReady ? (
    <ShellStateCard
      title="Loading projects"
      message="Resolving the project list for this operator session."
    />
  ) : projects.length === 0 && isProjectsRoute ? (
    <Outlet />
  ) : projects.length === 0 ? (
    <ShellStateCard
      title="No projects available"
      message="Create a project to get an API key and start loading the debugger workspace."
    />
  ) : (
    <Outlet />
  );

  return (
    <div className="app-shell-enter min-h-screen bg-[var(--c-app-bg)] text-[var(--c-text-primary)]">
      <ConsoleSidebar
        email={user?.email}
        isPublicDemo={isPublicDemo}
        navItems={visibleNavItems}
        projectId={selectedProjectId}
        projects={projects}
        projectsReady={projectsReady}
        projectsQueryPending={projectsQuery.isPending}
        selectedProjectId={selectedProject?.id}
        selectedProjectName={selectedProject?.name}
        onProjectChange={handleProjectChange}
      />

      <div className="min-h-screen md:ml-56">
        <ConsoleTopBar
          breadcrumbs={buildBreadcrumbs(location.pathname)}
          commands={commands}
          resolvedTheme={resolvedTheme}
          toggleTheme={toggleTheme}
          onMobileOpen={() => setMobileOpen(true)}
        />
        <main className="flex min-h-[calc(100vh-44px)] flex-col">
          {isPublicDemo ? (
            <PublicDemoBanner label={publicDemoLabel} docsHref={RUN_LOCALLY_DOCS_URL} />
          ) : null}
          {content}
        </main>
      </div>

      {mobileOpen ? (
        <div className="app-overlay-enter fixed inset-0 z-[70] md:hidden">
          <button
            type="button"
            aria-label="Close navigation"
            className="absolute inset-0 bg-slate-950/40 backdrop-blur-sm"
            onClick={() => setMobileOpen(false)}
          />
          <aside className="app-drawer-enter relative flex h-full w-[19rem] max-w-[88vw] flex-col border-r border-[var(--c-border)] bg-[var(--c-sidebar-bg)]">
            <div className="flex items-center justify-between border-b border-[var(--c-border)] p-4">
              <BrandBlock />
              <button
                type="button"
                aria-label="Close navigation"
                className="inline-flex h-8 w-8 items-center justify-center rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] text-[var(--c-text-secondary)]"
                onClick={() => setMobileOpen(false)}
              >
                <X className="h-4 w-4" />
              </button>
            </div>
            <SidebarProjectSwitcher
              isPublicDemo={isPublicDemo}
              label="Project"
              projects={projects}
              projectsReady={projectsReady}
              projectsQueryPending={projectsQuery.isPending}
              selectedProjectId={selectedProject?.id}
              selectedProjectName={selectedProject?.name}
              onProjectChange={handleProjectChange}
            />
            <div className="border-b border-[var(--c-border)] px-3 py-3">
              <CommandPalette commands={commands} />
            </div>
            <SidebarNav
              label="Mobile primary"
              isPublicDemo={isPublicDemo}
              items={visibleNavItems}
              projectId={selectedProjectId}
            />
            <SidebarFooter email={user?.email} />
          </aside>
        </div>
      ) : null}
    </div>
  );
}

function ConsoleSidebar({
  email,
  isPublicDemo,
  navItems,
  onProjectChange,
  projectId,
  projects,
  projectsQueryPending,
  projectsReady,
  selectedProjectId,
  selectedProjectName,
}: {
  email?: string;
  isPublicDemo: boolean;
  navItems: NavItem[];
  onProjectChange: (projectId: string) => void;
  projectId?: string;
  projects: Array<{ id: string; name: string }>;
  projectsQueryPending: boolean;
  projectsReady: boolean;
  selectedProjectId?: string;
  selectedProjectName?: string;
}) {
  return (
    <aside className="fixed inset-y-0 left-0 z-50 hidden w-56 flex-col border-r border-[var(--c-border)] bg-[var(--c-sidebar-bg)] md:flex">
      <div className="border-b border-[var(--c-border)] px-4 py-3.5">
        <BrandBlock />
      </div>
      <SidebarProjectSwitcher
        isPublicDemo={isPublicDemo}
        label="Active project"
        projects={projects}
        projectsReady={projectsReady}
        projectsQueryPending={projectsQueryPending}
        selectedProjectId={selectedProjectId}
        selectedProjectName={selectedProjectName}
        onProjectChange={onProjectChange}
      />
      <SidebarNav
        label="Primary"
        isPublicDemo={isPublicDemo}
        items={navItems}
        projectId={projectId}
      />
      <SidebarFooter email={email} />
    </aside>
  );
}

function BrandBlock() {
  return (
    <Link
      to="/"
      aria-label="Go to landing page"
      className="flex min-w-0 items-center gap-2.5 rounded-md outline-none transition-opacity hover:opacity-80 focus-visible:ring-2 focus-visible:ring-[var(--c-focus)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--c-sidebar-bg)]"
    >
      <img
        alt=""
        className="h-[22px] w-[22px] shrink-0"
        src="/logo.svg"
      />
      <div className="min-w-0 leading-none">
        <div className="truncate text-[13px] font-bold text-[var(--c-text-primary)]">
          Continua
        </div>
        <div className="mt-1 font-mono text-[10.5px] text-[var(--c-text-muted)]">
          local · alpha
        </div>
      </div>
    </Link>
  );
}

function SidebarProjectSwitcher({
  isPublicDemo,
  label = 'Active project',
  onProjectChange,
  projects,
  projectsQueryPending,
  projectsReady,
  selectedProjectId,
  selectedProjectName,
}: {
  isPublicDemo: boolean;
  label?: string;
  onProjectChange: (projectId: string) => void;
  projects: Array<{ id: string; name: string }>;
  projectsQueryPending: boolean;
  projectsReady: boolean;
  selectedProjectId?: string;
  selectedProjectName?: string;
}) {
  if (isPublicDemo) {
    return (
      <div className="m-3 rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-2.5 py-2 text-[13px] font-medium text-[var(--c-text-primary)]">
        Sample data
      </div>
    );
  }

  return (
    <label className="relative m-3 block">
      <span className="sr-only">{label}</span>
      <select
        value={selectedProjectId ?? ''}
        onChange={(event) => onProjectChange(event.target.value)}
        disabled={!projectsReady || projects.length === 0}
        className="h-8 w-full appearance-none truncate rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-9 pr-8 text-[13px] font-medium text-[var(--c-text-primary)] outline-none"
      >
        {projects.length === 0 ? (
          <option value="">
            {projectsQueryPending ? 'Loading projects...' : 'No projects'}
          </option>
        ) : null}
        {projects.map((project) => (
          <option key={project.id} value={project.id}>
            {project.name}
          </option>
        ))}
      </select>
      <span className="absolute left-2 top-2 h-4 w-4 rounded-[3px] bg-gradient-to-br from-[#0075d6] to-[#79b4f5]" />
      <ChevronDown className="pointer-events-none absolute right-2 top-2 h-4 w-4 text-[var(--c-text-muted)]" />
      {!selectedProjectName && projectsQueryPending ? null : null}
    </label>
  );
}

function SidebarNav({
  isPublicDemo,
  items,
  label,
  projectId,
}: {
  isPublicDemo: boolean;
  items: NavItem[];
  label: string;
  projectId?: string;
}) {
  const primaryItems = items.filter((item) => item.path !== '/settings');
  const projectItem = items.find((item) => item.path === '/projects');
  const settingsItem = items.find((item) => item.path === '/settings');
  const mainItems = primaryItems.filter((item) => item.path !== '/projects');

  return (
    <nav
      aria-label={label}
      className="flex flex-1 flex-col gap-3 overflow-y-auto px-2 pb-4"
    >
      <div>
        {mainItems.map((item) => (
          <SidebarNavLink key={item.path} item={item} projectId={projectId} />
        ))}
      </div>

      {!isPublicDemo && settingsItem ? (
        <div>
          <div className="px-2.5 py-1.5 text-[10.5px] font-semibold uppercase tracking-[0.08em] text-[var(--c-text-muted)]">
            Workspace
          </div>
          {projectItem ? (
            <SidebarNavLink item={projectItem} projectId={projectId} />
          ) : null}
          <SidebarNavLink item={settingsItem} projectId={projectId} />
        </div>
      ) : null}
    </nav>
  );
}

function SidebarNavLink({
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
        `mb-0.5 flex items-center gap-2.5 rounded-[5px] px-2.5 py-1.5 text-[13px] font-medium transition ${
          isActive
            ? 'bg-[var(--c-nav-active-bg)] font-semibold text-[var(--c-text-primary)]'
            : 'text-[var(--c-text-secondary)] hover:bg-[var(--c-nav-hover-bg)] hover:text-[var(--c-text-primary)]'
        }`
      }
    >
      <Icon className="h-4 w-4 text-[var(--c-text-muted)]" />
      {item.label}
    </NavLink>
  );
}

function SidebarFooter({ email }: { email?: string }) {
  const initials = (email ?? 'AV')
    .split('@')[0]
    .split(/[._-]/)
    .map((part) => part[0])
    .join('')
    .slice(0, 2)
    .toUpperCase();

  return (
    <div className="flex items-center justify-between gap-2 border-t border-[var(--c-border)] p-3">
      <div className="flex min-w-0 items-center gap-2">
        <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full border border-[var(--c-border)] bg-[var(--c-surface-elevated)] text-[10px] font-bold text-[var(--c-text-secondary)]">
          {initials || 'AV'}
        </div>
        <div className="truncate text-xs text-[var(--c-text-secondary)]">
          {email ?? 'aryan@continua.dev'}
        </div>
      </div>
      <MoreHorizontal className="h-4 w-4 shrink-0 text-[var(--c-text-muted)]" />
    </div>
  );
}

function ConsoleTopBar({
  breadcrumbs,
  commands,
  onMobileOpen,
  resolvedTheme,
  toggleTheme,
}: {
  breadcrumbs: string[];
  commands: Array<{ id: string; title: string; keywords: string[]; action: () => void }>;
  onMobileOpen: () => void;
  resolvedTheme: 'light' | 'dark';
  toggleTheme: () => void;
}) {
  return (
    <header className="sticky top-0 z-40 flex h-11 items-center justify-between border-b border-[var(--c-border)] bg-[var(--c-app-bg)] px-3 md:px-4">
      <div className="flex min-w-0 items-center gap-1.5 text-[13px] text-[var(--c-text-secondary)]">
        <button
          type="button"
          className="mr-1 inline-flex h-7 w-7 items-center justify-center rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] md:hidden"
          aria-label="Open navigation"
          onClick={onMobileOpen}
        >
          <Menu className="h-4 w-4" />
        </button>
        {breadcrumbs.map((breadcrumb, index) => (
          <span key={`${breadcrumb}-${index}`} className="contents">
            {index > 0 ? (
              <ChevronRight className="h-3.5 w-3.5 text-[var(--c-text-muted)]" />
            ) : null}
            <span
              className={`truncate ${
                index === breadcrumbs.length - 1
                  ? 'font-semibold text-[var(--c-text-primary)]'
                  : 'font-medium text-[var(--c-text-secondary)]'
              }`}
            >
              {breadcrumb}
            </span>
          </span>
        ))}
      </div>

      <div className="flex items-center gap-1.5">
        <div className="hidden sm:block">
          <CommandPalette commands={commands} />
        </div>
        <button
          type="button"
          title="Toggle theme"
          aria-label="Toggle theme"
          className="flex h-7 w-7 items-center justify-center rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] text-[var(--c-text-secondary)]"
          onClick={toggleTheme}
        >
          {resolvedTheme === 'dark' ? <Sun className="h-3.5 w-3.5" /> : <Moon className="h-3.5 w-3.5" />}
        </button>
      </div>
    </header>
  );
}

function PublicDemoBanner({
  docsHref,
  label,
}: {
  docsHref: string;
  label: string;
}) {
  return (
    <section className="flex flex-col gap-3 border-b border-[var(--c-border)] bg-[var(--c-accent-faint)] px-6 py-3 sm:flex-row sm:items-center sm:justify-between">
      <div>
        <div className="text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--c-accent-text)]">
          {label}
        </div>
        <p className="mt-1 text-[13px] leading-5 text-[var(--c-text-secondary)]">
          Read-only demo: sample traces are hosted here for exploration. Run Continua locally to inspect your own traces.
        </p>
      </div>
      <a
        href={docsHref}
        target="_blank"
        rel="noreferrer"
        className="app-button-secondary whitespace-nowrap"
      >
        Run locally
      </a>
    </section>
  );
}

function ShellStateCard({
  actions,
  message,
  title,
}: {
  actions?: ReactNode;
  message: string;
  title: string;
}) {
  return (
    <section className="m-6 max-w-3xl rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] p-6">
      <div className="text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--c-text-muted)]">
        Operator workspace
      </div>
      <h1 className="mt-3 text-2xl font-bold text-[var(--c-text-primary)]">
        {title}
      </h1>
      <p className="mt-3 text-sm leading-6 text-[var(--c-text-secondary)]">
        {message}
      </p>
      {actions ? <div className="mt-5 flex flex-wrap gap-2">{actions}</div> : null}
    </section>
  );
}

function buildBreadcrumbs(pathname: string): string[] {
  const segments = pathname.split('/').filter(Boolean);
  if (segments.length === 0 || segments[0] === 'dashboard') {
    return ['Overview'];
  }
  if (segments[0] === 'traces') {
    return segments[1] ? ['Traces', segments[1]] : ['Traces'];
  }
  if (segments[0] === 'engine') {
    return ['Engine Runs'];
  }
  if (segments[0] === 'tools' && segments[1] === 'engine-projections') {
    return ['Settings', 'Engine projection repair'];
  }
  if (segments[0] === 'sessions') {
    if (segments[2] === 'compare') {
      return ['Sessions', segments[1] ?? 'Session', 'Compare'];
    }
    return segments[1] ? ['Sessions', segments[1]] : ['Sessions'];
  }
  if (segments[0] === 'settings') {
    return ['Settings'];
  }
  if (segments[0] === 'projects') {
    return ['Projects'];
  }
  return [segments[0] ?? 'Overview'];
}
