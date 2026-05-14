import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { fetchProjects, getApiKey } from '../api/client';
import { useRuntimeAuth } from '../auth/runtime';

const DISMISS_KEY = 'continua_default_project_nudge_dismissed';
const DEFAULT_PROJECT_ID = '00000000-0000-0000-0000-000000000001';
const SEEDED_DEFAULT_KEY = 'default';

function isDismissed(): boolean {
  if (typeof window === 'undefined') return true;
  try {
    return window.localStorage.getItem(DISMISS_KEY) === '1';
  } catch {
    return false;
  }
}

function persistDismissal(): void {
  if (typeof window === 'undefined') return;
  try {
    window.localStorage.setItem(DISMISS_KEY, '1');
  } catch {
    // ignore — banner just won't persist
  }
}

export function DefaultProjectBanner() {
  const [dismissed, setDismissed] = useState(() => isDismissed());
  const runtimeAuth = useRuntimeAuth();
  // The nudge is only meaningful in the local-self-host path: Auth0 mode has its
  // own onboarding, and public demo mode has no concept of project ownership.
  const isLocalApiKeyMode =
    runtimeAuth.status === 'ready' &&
    !runtimeAuth.enabled &&
    runtimeAuth.public_demo_enabled !== true;
  const { data } = useQuery({
    queryKey: ['projects'],
    queryFn: fetchProjects,
    staleTime: 30_000,
    enabled: !dismissed && isLocalApiKeyMode,
  });

  useEffect(() => {
    if (dismissed) {
      persistDismissal();
    }
  }, [dismissed]);

  if (dismissed) return null;
  if (!isLocalApiKeyMode) return null;
  if (!data) return null;

  const onlyDefault =
    data.projects.length === 1 && data.projects[0]?.id === DEFAULT_PROJECT_ID;
  if (!onlyDefault) return null;

  // And only while the user is actually still on the seeded `default` API key.
  if (getApiKey() !== SEEDED_DEFAULT_KEY) return null;

  return (
    <div
      role="status"
      data-testid="default-project-banner"
      className="flex items-center justify-between gap-3 border-b border-[var(--c-border)] bg-amber-500/10 px-6 py-2 text-sm text-[var(--c-text-primary)]"
    >
      <span>
        You&apos;re using the seeded <code className="font-mono">default</code>{' '}
        project. Create a real project to start organizing your work.
      </span>
      <div className="flex items-center gap-2">
        <Link to="/projects" className="app-button-primary">
          Create project
        </Link>
        <button
          type="button"
          className="app-button-secondary"
          onClick={() => setDismissed(true)}
        >
          Dismiss
        </button>
      </div>
    </div>
  );
}
