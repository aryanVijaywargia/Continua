import { Link } from 'react-router-dom';

interface AuthErrorBannerProps {
  message?: string;
}

export function AuthErrorBanner({
  message = 'Your API key is missing, expired, or invalid.',
}: AuthErrorBannerProps) {
  return (
    <div
      role="alert"
      className="rounded-[1rem] border border-amber-200 bg-amber-50/80 px-4 py-3 text-amber-950 shadow-sm dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-100"
    >
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <p className="app-overline text-amber-800 dark:text-amber-200">
            Authentication required
          </p>
          <p className="mt-1 text-sm">{message}</p>
        </div>
        <Link
          to="/settings"
          className="inline-flex items-center justify-center rounded-full border border-amber-300 bg-[var(--continua-surface-elevated)] px-4 py-2 text-sm font-bold text-amber-900 transition hover:bg-amber-100 focus:outline-none focus:ring-2 focus:ring-amber-300 dark:border-amber-400/40 dark:text-amber-100 dark:hover:bg-[var(--continua-surface-muted)]"
        >
          Go to Settings
        </Link>
      </div>
    </div>
  );
}
