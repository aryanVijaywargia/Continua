import { Link, useLocation, useNavigate } from 'react-router-dom';
import { CommandPalette } from './CommandPalette';
import { useTheme } from '../hooks/useTheme';

const navItems = [
  { path: '/traces', label: 'Traces' },
  { path: '/sessions', label: 'Sessions' },
  { path: '/projects', label: 'Projects' },
  { path: '/settings', label: 'Settings' },
];

/**
 * Main navigation component.
 */
export function Navigation() {
  const location = useLocation();
  const navigate = useNavigate();
  const { resolvedTheme, toggleTheme } = useTheme();
  const commands = [
    {
      id: 'go-traces',
      title: 'Go to Traces',
      keywords: ['navigate', 'traces', 'home'],
      action: () => navigate('/traces'),
    },
    {
      id: 'go-sessions',
      title: 'Go to Sessions',
      keywords: ['navigate', 'sessions'],
      action: () => navigate('/sessions'),
    },
    {
      id: 'go-projects',
      title: 'Go to Projects',
      keywords: ['navigate', 'projects', 'api key', 'create project'],
      action: () => navigate('/projects'),
    },
    {
      id: 'go-settings',
      title: 'Go to Settings',
      keywords: ['navigate', 'settings', 'api key', 'theme'],
      action: () => navigate('/settings'),
    },
    {
      id: 'toggle-theme',
      title: 'Toggle Theme',
      keywords: ['theme', 'dark', 'light', 'appearance'],
      action: () => toggleTheme(),
    },
  ];

  return (
    <nav className="border-b border-slate-200 bg-white/95 backdrop-blur dark:border-slate-800 dark:bg-slate-950/95">
      <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
        <div className="flex min-h-16 flex-col gap-3 py-3 lg:h-16 lg:flex-row lg:items-center lg:justify-between lg:py-0">
          <div className="flex min-w-0 items-center">
            <Link
              to="/dashboard"
              className="flex items-center text-xl font-bold text-slate-900 dark:text-slate-100"
            >
              Continua
            </Link>
            <div className="ml-6 flex flex-wrap items-center gap-2 lg:ml-10 lg:gap-4">
              {navItems.map((item) => {
                const isActive = location.pathname.startsWith(item.path);
                return (
                  <Link
                    key={item.path}
                    to={item.path}
                    className={`rounded-md px-3 py-2 text-sm font-medium transition ${
                      isActive
                        ? 'bg-slate-900 text-white dark:bg-slate-100 dark:text-slate-950'
                        : 'text-slate-500 hover:bg-slate-100 hover:text-slate-900 dark:text-slate-400 dark:hover:bg-slate-900 dark:hover:text-slate-100'
                    }`}
                  >
                    {item.label}
                  </Link>
                );
              })}
            </div>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <button
              type="button"
              onClick={toggleTheme}
              className="inline-flex items-center gap-2 rounded-full border border-slate-300 bg-white px-3 py-1.5 text-sm font-medium text-slate-600 transition hover:bg-slate-100 focus:outline-none focus:ring-2 focus:ring-blue-200 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200 dark:hover:bg-slate-800"
              aria-label="Toggle theme"
            >
              <span>Theme</span>
              <span className="rounded-full bg-slate-100 px-2 py-0.5 text-xs text-slate-500 dark:bg-slate-800 dark:text-slate-300">
                {resolvedTheme}
              </span>
            </button>
            <CommandPalette commands={commands} />
          </div>
        </div>
      </div>
    </nav>
  );
}
