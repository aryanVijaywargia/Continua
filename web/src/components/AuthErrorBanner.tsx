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
      className="rounded-xl border border-amber-200 bg-amber-50 px-4 py-3 text-amber-950 shadow-sm dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-100"
    >
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <p className="text-sm font-semibold uppercase tracking-[0.16em] text-amber-800 dark:text-amber-200">
            Authentication required
          </p>
          <p className="mt-1 text-sm">{message}</p>
        </div>
        <Link
          to="/settings"
          className="inline-flex items-center justify-center rounded-lg border border-amber-300 bg-white px-3 py-2 text-sm font-medium text-amber-900 transition hover:bg-amber-100 focus:outline-none focus:ring-2 focus:ring-amber-300 dark:border-amber-400/40 dark:bg-slate-950 dark:text-amber-100 dark:hover:bg-slate-900"
        >
          Go to Settings
        </Link>
      </div>
    </div>
  );
}
