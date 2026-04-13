import { useEffect, useMemo, useState } from 'react';
import {
  Activity,
  Command,
  LayoutDashboard,
  Menu,
  MoonStar,
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
  const commands = useMemo(
    () => [
      {
        id: 'go-overview',
        title: 'Go to Overview',
        keywords: ['home', 'overview', 'dashboard'],
        action: () => navigate('/dashboard'),
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
    <div className="app-shell-enter min-h-screen bg-[var(--continua-app-bg)] text-[var(--continua-text-primary)]">
      {/* Top Navigation — matches the landing page nav style */}
      <nav className="fixed top-0 z-50 w-full border-b border-[var(--continua-border-soft)] bg-[var(--continua-shell-topbar)] backdrop-blur-xl">
        <div className="mx-auto flex h-16 max-w-7xl items-center justify-between px-6">
          {/* Brand */}
          <div className="flex items-center gap-8">
            <NavLink to="/" className="text-xl font-black tracking-tighter text-[var(--continua-text-primary)]">
              Continua
            </NavLink>

            {/* Desktop nav links */}
            <div className="hidden items-center gap-1 md:flex" role="navigation" aria-label="Primary">
              {NAV_ITEMS.map((item) => (
                <ShellNavLink key={item.path} item={item} />
              ))}
            </div>
          </div>

          {/* Right side controls */}
          <div className="flex items-center gap-3">
            <div className="hidden items-center gap-2 sm:flex">
              <span className="inline-flex items-center gap-2 rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-3 py-1.5 text-xs font-medium text-[var(--continua-text-secondary)]">
                <Command className="h-3.5 w-3.5 text-[var(--continua-accent)]" />
                <span>{hasApiKey ? 'Connected' : 'API key required'}</span>
              </span>

              <button
                type="button"
                onClick={toggleTheme}
                className="inline-flex items-center gap-2 rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] px-3 py-1.5 text-sm font-medium text-[var(--continua-text-secondary)] transition hover:border-[var(--continua-border-strong)] hover:text-[var(--continua-text-primary)] focus:outline-none"
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

            {/* Mobile menu button */}
            <button
              type="button"
              aria-label="Open navigation"
              className="inline-flex h-10 w-10 items-center justify-center rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] text-[var(--continua-text-secondary)] transition hover:text-[var(--continua-text-primary)] md:hidden"
              onClick={() => setMobileOpen(true)}
            >
              <Menu className="h-4 w-4" />
            </button>
          </div>
        </div>
      </nav>

      {/* Mobile drawer overlay */}
      {mobileOpen ? (
        <div className="app-overlay-enter fixed inset-0 z-[60] md:hidden">
          <button
            type="button"
            aria-label="Close navigation overlay"
            className="absolute inset-0 bg-[var(--continua-text-primary)]/50 backdrop-blur-sm"
            onClick={() => setMobileOpen(false)}
          />
          <aside className="app-drawer-enter relative flex h-full w-[19rem] max-w-[88vw] flex-col border-r border-[var(--continua-border-strong)] bg-[var(--continua-app-bg)] px-6 py-5 shadow-2xl">
            <div className="flex items-center justify-between">
              <span className="text-xl font-black tracking-tighter text-[var(--continua-text-primary)]">
                Continua
              </span>
              <button
                type="button"
                aria-label="Close navigation"
                className="inline-flex h-10 w-10 items-center justify-center rounded-full border border-[var(--continua-border-soft)] bg-[var(--continua-surface-elevated)] text-[var(--continua-text-secondary)] transition hover:text-[var(--continua-text-primary)]"
                onClick={() => setMobileOpen(false)}
              >
                <X className="h-4 w-4" />
              </button>
            </div>

            <nav className="mt-8 flex flex-1 flex-col gap-1" aria-label="Mobile primary">
              {NAV_ITEMS.map((item) => (
                <MobileNavLink key={item.path} item={item} />
              ))}
            </nav>

            <div className="mt-auto flex flex-col gap-3">
              <button
                type="button"
                onClick={toggleTheme}
                className="flex w-full items-center gap-3 rounded-full border border-[var(--continua-border-soft)] px-4 py-2.5 text-sm font-medium text-[var(--continua-text-secondary)] transition hover:text-[var(--continua-text-primary)]"
              >
                {resolvedTheme === 'dark' ? (
                  <MoonStar className="h-4 w-4 text-[var(--continua-accent)]" />
                ) : (
                  <SunMedium className="h-4 w-4 text-[var(--continua-accent)]" />
                )}
                <span className="capitalize">{resolvedTheme} mode</span>
              </button>
              <CommandPalette commands={commands} />
            </div>
          </aside>
        </div>
      ) : null}

      {/* Main content — full width, generous spacing, matching landing page feel */}
      <main className="mx-auto min-h-screen max-w-7xl px-6 pt-24 pb-16">
        <Outlet />
      </main>
    </div>
  );
}

function ShellNavLink({ item }: { item: NavItem }) {
  return (
    <NavLink
      to={item.path}
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

function MobileNavLink({ item }: { item: NavItem }) {
  const Icon = item.icon;

  return (
    <NavLink
      to={item.path}
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
