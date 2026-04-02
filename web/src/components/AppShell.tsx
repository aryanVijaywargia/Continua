import { useEffect, useMemo, useState } from 'react';
import {
  Activity,
  Command,
  LayoutDashboard,
  Menu,
  MoonStar,
  PanelLeftClose,
  Settings2,
  SunMedium,
  Waypoints,
  X,
} from 'lucide-react';
import {
  NavLink,
  Outlet,
  useLocation,
  useNavigate,
} from 'react-router-dom';
import { CommandPalette } from './CommandPalette';
import { getApiKey } from '../api/client';
import { useTheme } from '../hooks/useTheme';

interface NavItem {
  path: string;
  label: string;
  detail: string;
  icon: typeof LayoutDashboard;
}

const NAV_ITEMS: NavItem[] = [
  {
    path: '/',
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
    detail: 'Theme and local API-key controls',
    icon: Settings2,
  },
];

export function AppShell() {
  const location = useLocation();
  const navigate = useNavigate();
  const { resolvedTheme, toggleTheme } = useTheme();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [hasApiKey, setHasApiKey] = useState(() => Boolean(getApiKey()));
  const activeItem = useMemo(
    () =>
      NAV_ITEMS.find((item) =>
        item.path === '/'
          ? location.pathname === '/'
          : location.pathname.startsWith(item.path)
      ) ?? NAV_ITEMS[0],
    [location.pathname]
  );
  const commands = useMemo(
    () => [
      {
        id: 'go-overview',
        title: 'Go to Overview',
        keywords: ['home', 'overview', 'dashboard'],
        action: () => navigate('/'),
      },
      {
        id: 'go-traces',
        title: 'Go to Traces',
        keywords: ['navigate', 'traces'],
        action: () => navigate('/traces'),
      },
      {
        id: 'go-sessions',
        title: 'Go to Sessions',
        keywords: ['navigate', 'sessions'],
        action: () => navigate('/sessions'),
      },
      {
        id: 'go-settings',
        title: 'Go to Settings',
        keywords: ['navigate', 'settings', 'theme', 'api key'],
        action: () => navigate('/settings'),
      },
      {
        id: 'toggle-theme',
        title: 'Toggle Theme',
        keywords: ['appearance', 'theme', 'dark', 'light'],
        action: () => toggleTheme(),
      },
    ],
    [navigate, toggleTheme]
  );

  useEffect(() => {
    if (mobileOpen) {
      setMobileOpen(false);
    }
  }, [location.pathname, location.search, mobileOpen]);

  useEffect(() => {
    const syncApiKeyState = () => setHasApiKey(Boolean(getApiKey()));

    window.addEventListener('storage', syncApiKeyState);
    window.addEventListener('continua:api-key-change', syncApiKeyState);

    return () => {
      window.removeEventListener('storage', syncApiKeyState);
      window.removeEventListener('continua:api-key-change', syncApiKeyState);
    };
  }, []);

  return (
    <div className="app-shell-enter min-h-screen bg-transparent text-[var(--continua-text-primary)]">
      <div className="flex min-h-screen">
        <aside className="hidden w-[17rem] shrink-0 border-r border-[var(--continua-border-strong)] bg-[var(--continua-shell-rail)] px-4 py-5 lg:flex lg:flex-col">
          <ShellBrand />
          <nav className="mt-8 flex flex-1 flex-col gap-2" aria-label="Primary">
            {NAV_ITEMS.map((item) => (
              <ShellNavLink key={item.path} item={item} />
            ))}
          </nav>
          <div className="rounded-[1.25rem] border border-[var(--continua-border-soft)] bg-[var(--continua-surface-muted)] p-4 shadow-[var(--continua-shadow-soft)]">
            <div className="text-[11px] font-semibold uppercase tracking-[0.22em] text-[var(--continua-text-muted)]">
              Investigation model
            </div>
            <p className="mt-3 text-sm leading-6 text-[var(--continua-text-secondary)]">
              Triage recent work, then move directly into a trace or session workspace
              without leaving the shared shell.
            </p>
          </div>
        </aside>

        {mobileOpen ? (
          <div className="app-overlay-enter fixed inset-0 z-50 lg:hidden">
            <button
              type="button"
              aria-label="Close navigation overlay"
              className="absolute inset-0 bg-slate-950/45 backdrop-blur-sm"
              onClick={() => setMobileOpen(false)}
            />
            <aside className="app-drawer-enter relative flex h-full w-[19rem] max-w-[88vw] flex-col border-r border-[var(--continua-border-strong)] bg-[var(--continua-shell-rail)] px-4 py-5 shadow-2xl">
              <div className="flex items-center justify-between">
                <ShellBrand />
                <button
                  type="button"
                  aria-label="Close navigation"
                  className="inline-flex h-10 w-10 items-center justify-center rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] text-[var(--continua-text-secondary)] transition hover:border-[var(--continua-border-strong)] hover:text-[var(--continua-text-primary)]"
                  onClick={() => setMobileOpen(false)}
                >
                  <X className="h-4 w-4" />
                </button>
              </div>

              <nav className="mt-8 flex flex-1 flex-col gap-2" aria-label="Mobile primary">
                {NAV_ITEMS.map((item) => (
                  <ShellNavLink key={item.path} item={item} />
                ))}
              </nav>
            </aside>
          </div>
        ) : null}

        <div className="flex min-h-screen min-w-0 flex-1 flex-col">
          <header className="sticky top-0 z-40 border-b border-[var(--continua-border-soft)] bg-[var(--continua-shell-topbar)] px-4 py-3 backdrop-blur-xl sm:px-6 lg:px-8">
            <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div className="flex min-w-0 items-center gap-3">
                <button
                  type="button"
                  aria-label="Open navigation"
                  className="inline-flex h-10 w-10 items-center justify-center rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] text-[var(--continua-text-secondary)] transition hover:border-[var(--continua-border-strong)] hover:text-[var(--continua-text-primary)] lg:hidden"
                  onClick={() => setMobileOpen(true)}
                >
                  <Menu className="h-4 w-4" />
                </button>

                <div className="min-w-0">
                  <div className="text-[11px] font-semibold uppercase tracking-[0.24em] text-[var(--continua-text-muted)]">
                    {activeItem.label}
                  </div>
                  <div className="mt-1 truncate text-sm text-[var(--continua-text-secondary)]">
                    {activeItem.detail}
                  </div>
                </div>
              </div>

              <div className="flex flex-wrap items-center gap-2">
                <div className="inline-flex items-center gap-2 rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-3 py-2 text-xs font-medium text-[var(--continua-text-secondary)] shadow-[var(--continua-shadow-soft)]">
                  <Command className="h-3.5 w-3.5 text-[var(--continua-accent)]" />
                  <span>{hasApiKey ? 'API key connected' : 'API key required'}</span>
                </div>

                <button
                  type="button"
                  onClick={toggleTheme}
                  className="inline-flex items-center gap-2 rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-3 py-2 text-sm font-medium text-[var(--continua-text-secondary)] shadow-[var(--continua-shadow-soft)] transition hover:border-[var(--continua-border-strong)] hover:text-[var(--continua-text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--continua-accent-faint)]"
                  aria-label="Toggle theme"
                >
                  {resolvedTheme === 'dark' ? (
                    <MoonStar className="h-4 w-4 text-[var(--continua-accent)]" />
                  ) : (
                    <SunMedium className="h-4 w-4 text-[var(--continua-accent)]" />
                  )}
                  <span className="capitalize">{resolvedTheme}</span>
                </button>

                <CommandPalette commands={commands} />
              </div>
            </div>
          </header>

          <main className="min-w-0 flex-1 overflow-x-hidden px-4 py-5 sm:px-6 lg:px-8">
            <Outlet />
          </main>
        </div>
      </div>
    </div>
  );
}

function ShellBrand() {
  return (
    <div className="rounded-[1.5rem] border border-[var(--continua-border-strong)] bg-[var(--continua-surface-elevated)] p-4 shadow-[var(--continua-shadow-soft)]">
      <div className="flex items-center gap-3">
        <div className="inline-flex h-11 w-11 items-center justify-center rounded-[1rem] bg-[var(--continua-accent-strong)] text-[var(--continua-accent-contrast)] shadow-[var(--continua-shadow-soft)]">
          <PanelLeftClose className="h-5 w-5" />
        </div>
        <div className="min-w-0">
          <div className="truncate text-lg font-semibold tracking-[-0.02em] text-[var(--continua-text-primary)]">
            Continua
          </div>
          <div className="mt-1 text-xs font-medium uppercase tracking-[0.18em] text-[var(--continua-text-muted)]">
            Operator Console
          </div>
        </div>
      </div>
    </div>
  );
}

function ShellNavLink({ item }: { item: NavItem }) {
  const Icon = item.icon;

  return (
    <NavLink
      to={item.path}
      end={item.path === '/'}
      className={({ isActive }) =>
        `group flex items-start gap-3 rounded-[1.15rem] border px-3 py-3 transition ${
          isActive
            ? 'border-[var(--continua-border-strong)] bg-[var(--continua-surface-elevated)] text-[var(--continua-text-primary)] shadow-[var(--continua-shadow-soft)]'
            : 'border-transparent bg-transparent text-[var(--continua-text-secondary)] hover:border-[var(--continua-border-soft)] hover:bg-[var(--continua-surface-muted)] hover:text-[var(--continua-text-primary)]'
        }`
      }
    >
      {({ isActive }) => (
        <>
          <span
            className={`mt-0.5 inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-full ${
              isActive
                ? 'bg-[var(--continua-accent-faint)] text-[var(--continua-accent)]'
                : 'bg-[var(--continua-surface-muted)] text-[var(--continua-text-muted)] group-hover:text-[var(--continua-accent)]'
            }`}
          >
            <Icon className="h-4 w-4" />
          </span>
          <span className="min-w-0">
            <span className="block text-sm font-semibold">{item.label}</span>
            <span className="mt-1 block text-xs leading-5 text-[var(--continua-text-muted)]">
              {item.detail}
            </span>
          </span>
        </>
      )}
    </NavLink>
  );
}
