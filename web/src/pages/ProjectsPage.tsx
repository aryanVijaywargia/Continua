import { useEffect, useState, type ReactNode } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useNavigate } from 'react-router-dom';
import {
  ApiError,
  clearApiKey,
  createProject,
  deleteProject,
  fetchProjects,
  forgetProjectApiKey,
  getFallbackProjectApiKey,
  getApiKey,
  getKnownProjectApiKey,
  isAuthError,
  renameProject,
  rememberProjectApiKey,
  rotateProjectApiKey,
  setApiKey,
  type Project,
  type ProjectWithKey,
} from '../api/client';
import { PageHeader } from '../components/DebuggerKit';
import { CopyButton } from '../components/CopyButton';
import { buildProjectPath } from '../utils/projectSearchParams';

const PROJECTS_QUERY_KEY = ['projects'] as const;

export function ProjectsPage() {
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: PROJECTS_QUERY_KEY,
    queryFn: fetchProjects,
  });

  const [createOpen, setCreateOpen] = useState(false);
  const [renameTarget, setRenameTarget] = useState<Project | null>(null);
  const [rotateTarget, setRotateTarget] = useState<Project | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Project | null>(null);
  const [revealedKey, setRevealedKey] = useState<ProjectWithKey | null>(null);
  const [postRevealPath, setPostRevealPath] = useState<string | null>(null);
  const [activateRevealedKey, setActivateRevealedKey] = useState(false);

  const invalidateProjects = () => {
    queryClient.invalidateQueries({ queryKey: PROJECTS_QUERY_KEY });
  };

  useEffect(() => {
    const authenticatedProjectId = data?.authenticated_project_id;
    const currentLocalKey = getApiKey();
    if (!authenticatedProjectId || !currentLocalKey) {
      return;
    }
    rememberProjectApiKey(authenticatedProjectId, currentLocalKey);
  }, [data?.authenticated_project_id]);

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <PageHeader
        title="Projects"
        description="Create and manage projects used to isolate trace data. Each project has its own API key."
        actions={
          <button
            type="button"
            className="app-button-primary"
            onClick={() => setCreateOpen(true)}
          >
            Create project
          </button>
        }
      />

      <div className="max-w-5xl p-6">
        {isLoading ? (
          <p className="text-sm text-[var(--c-text-secondary)]">Loading projects…</p>
        ) : isError ? (
          <ErrorPanel
            error={error}
            onRetry={() => {
              refetch();
            }}
          />
        ) : data && data.projects.length > 0 ? (
          <ProjectsTable
            projects={data.projects}
            onRename={setRenameTarget}
            onRotate={setRotateTarget}
            onDelete={setDeleteTarget}
          />
        ) : !getApiKey() ? (
          <FirstRunHero onCreate={() => setCreateOpen(true)} />
        ) : (
          <EmptyState onCreate={() => setCreateOpen(true)} />
        )}
      </div>

      {createOpen ? (
        <CreateProjectDialog
          onClose={() => setCreateOpen(false)}
          onCreated={(project) => {
            const isFirstProject = !getApiKey() && (data?.projects.length ?? 0) === 0;
            if (isFirstProject) {
              setPostRevealPath(buildProjectPath('/dashboard', project.id));
            } else {
              invalidateProjects();
            }
            setCreateOpen(false);
            setRevealedKey(project);
          }}
        />
      ) : null}

      {renameTarget ? (
        <RenameProjectDialog
          project={renameTarget}
          onClose={() => setRenameTarget(null)}
          onRenamed={() => {
            invalidateProjects();
            setRenameTarget(null);
          }}
        />
      ) : null}

      {rotateTarget ? (
        <RotateKeyDialog
          project={rotateTarget}
          onClose={() => setRotateTarget(null)}
          onRotated={(project) => {
            setRotateTarget(null);
            setActivateRevealedKey(true);
            setRevealedKey(project);
          }}
        />
      ) : null}

      {deleteTarget ? (
        <DeleteProjectDialog
          project={deleteTarget}
          onClose={() => setDeleteTarget(null)}
          onDeleted={() => {
            const deletedProjectKey = getKnownProjectApiKey(deleteTarget.id);
            const deletedCurrentLocalKey =
              deleteTarget.id === data?.authenticated_project_id ||
              (deletedProjectKey !== null && deletedProjectKey === getApiKey());
            forgetProjectApiKey(deleteTarget.id);
            if (deletedCurrentLocalKey) {
              const fallbackKey = getFallbackProjectApiKey();
              if (fallbackKey) {
                setApiKey(fallbackKey);
              } else {
                clearApiKey();
              }
            }
            invalidateProjects();
            setDeleteTarget(null);
          }}
        />
      ) : null}

      {revealedKey ? (
        <RevealKeyDialog
          project={revealedKey}
          onClose={() => {
            const nextPath = postRevealPath;
            const revealedProject = revealedKey;
            const shouldActivateRevealedKey = activateRevealedKey;
            setRevealedKey(null);
            setPostRevealPath(null);
            setActivateRevealedKey(false);
            if (revealedProject) {
              rememberProjectApiKey(revealedProject.id, revealedProject.api_key);
            }
            if (
              revealedProject &&
              (shouldActivateRevealedKey || !getApiKey())
            ) {
              setApiKey(revealedProject.api_key);
            }
            invalidateProjects();
            if (nextPath) {
              navigate(nextPath, { replace: true });
            }
          }}
        />
      ) : null}
    </div>
  );
}

function ProjectsTable({
  projects,
  onRename,
  onRotate,
  onDelete,
}: {
  projects: Project[];
  onRename: (project: Project) => void;
  onRotate: (project: Project) => void;
  onDelete: (project: Project) => void;
}) {
  return (
    <div className="overflow-hidden rounded-md border border-[var(--c-border)] bg-[var(--c-surface)]">
      <table className="min-w-full divide-y divide-[var(--c-border)]">
        <thead className="bg-[var(--c-app-bg)]">
          <tr>
            <th className="px-4 py-2 text-left text-xs font-semibold uppercase tracking-wide text-[var(--c-text-muted)]">
              Name
            </th>
            <th className="px-4 py-2 text-left text-xs font-semibold uppercase tracking-wide text-[var(--c-text-muted)]">
              Project ID
            </th>
            <th className="px-4 py-2 text-left text-xs font-semibold uppercase tracking-wide text-[var(--c-text-muted)]">
              Created
            </th>
            <th className="px-4 py-2 text-right text-xs font-semibold uppercase tracking-wide text-[var(--c-text-muted)]">
              Actions
            </th>
          </tr>
        </thead>
        <tbody className="divide-y divide-[var(--c-border)]">
          {projects.map((project) => {
            return (
              <tr key={project.id} data-testid={`project-row-${project.id}`}>
                <td className="px-4 py-3 text-sm font-medium text-[var(--c-text-primary)]">
                  {project.name}
                </td>
                <td className="px-4 py-3 font-mono text-xs text-[var(--c-text-secondary)]">
                  {project.id}
                </td>
                <td className="px-4 py-3 text-sm text-[var(--c-text-secondary)]">
                  {new Date(project.created_at).toLocaleString()}
                </td>
                <td className="px-4 py-3 text-right">
                  <div className="inline-flex flex-wrap justify-end gap-2">
                    <button
                      type="button"
                      className="app-button-secondary"
                      onClick={() => onRename(project)}
                    >
                      Rename
                    </button>
                    <button
                      type="button"
                      className="app-button-secondary"
                      onClick={() => onRotate(project)}
                    >
                      Rotate key
                    </button>
                    <button
                      type="button"
                      className="app-button-secondary"
                      onClick={() => onDelete(project)}
                    >
                      Delete
                    </button>
                  </div>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function EmptyState({ onCreate }: { onCreate: () => void }) {
  return (
    <div className="app-surface-muted p-8 text-center">
      <p className="text-sm text-[var(--c-text-secondary)]">
        No projects yet. Create one to get an API key for your SDK.
      </p>
      <button type="button" className="app-button-primary mt-4" onClick={onCreate}>
        Create project
      </button>
    </div>
  );
}

const QUICKSTART_DOCS_URL = 'https://www.continua.in/docs/guides/quickstart';

function FirstRunHero({ onCreate }: { onCreate: () => void }) {
  return (
    <div
      data-testid="projects-first-run-hero"
      className="app-surface-muted p-10"
    >
      <p className="text-[11px] font-semibold uppercase tracking-[0.12em] text-[var(--c-text-muted)]">
        Welcome to Continua
      </p>
      <h2 className="mt-3 text-xl font-semibold text-[var(--c-text-primary)]">
        Create your first project to mint an API key
      </h2>
      <p className="mt-3 max-w-2xl text-sm leading-6 text-[var(--c-text-secondary)]">
        Projects isolate trace data and own the API key your SDK uses to ingest
        runs. We'll show the new key exactly once after creation — copy it then
        and store it somewhere safe.
      </p>
      <div className="mt-6 flex flex-wrap items-center gap-3">
        <button type="button" className="app-button-primary" onClick={onCreate}>
          Create project
        </button>
        <a
          href={QUICKSTART_DOCS_URL}
          target="_blank"
          rel="noreferrer"
          className="text-sm font-medium text-[var(--c-accent-text)] hover:underline"
        >
          Read the quickstart →
        </a>
      </div>
    </div>
  );
}

function ErrorPanel({
  error,
  onRetry,
}: {
  error: unknown;
  onRetry: () => void;
}) {
  const message =
    error instanceof ApiError
      ? error.message
      : error instanceof Error
        ? error.message
        : 'Failed to load projects';

  return (
    <div className="rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] p-4">
      <p className="text-sm text-[var(--c-text-primary)]">{message}</p>
      <button type="button" className="app-button-secondary mt-3" onClick={onRetry}>
        Retry
      </button>
    </div>
  );
}

function CreateProjectDialog({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: (project: ProjectWithKey) => void;
}) {
  const [name, setName] = useState('');
  const mutation = useMutation({
    mutationFn: (n: string) => createProject(n),
    onSuccess: onCreated,
  });

  const trimmed = name.trim();
  const submitDisabled = trimmed === '' || mutation.isPending;

  return (
    <DialogShell title="Create project" onClose={onClose}>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          if (submitDisabled) return;
          mutation.mutate(trimmed);
        }}
      >
        <label
          htmlFor="create-project-name"
          className="mb-2 block text-sm font-semibold text-[var(--c-text-primary)]"
        >
          Project name
        </label>
        <input
          id="create-project-name"
          autoFocus
          className="app-input w-full"
          placeholder="e.g. chatbot-v2"
          maxLength={100}
          value={name}
          onChange={(event) => setName(event.target.value)}
        />

        <MutationError error={mutation.error} />

        <DialogActions
          onClose={onClose}
          submitLabel={mutation.isPending ? 'Creating…' : 'Create project'}
          submitDisabled={submitDisabled}
        />
      </form>
    </DialogShell>
  );
}

function RenameProjectDialog({
  project,
  onClose,
  onRenamed,
}: {
  project: Project;
  onClose: () => void;
  onRenamed: (project: Project) => void;
}) {
  const [name, setName] = useState(project.name);
  const mutation = useMutation({
    mutationFn: (n: string) => renameProject(project.id, n),
    onSuccess: onRenamed,
  });

  const trimmed = name.trim();
  const submitDisabled =
    trimmed === '' || trimmed === project.name || mutation.isPending;

  return (
    <DialogShell title={`Rename "${project.name}"`} onClose={onClose}>
      <form
        onSubmit={(event) => {
          event.preventDefault();
          if (submitDisabled) return;
          mutation.mutate(trimmed);
        }}
      >
        <label
          htmlFor="rename-project-name"
          className="mb-2 block text-sm font-semibold text-[var(--c-text-primary)]"
        >
          New name
        </label>
        <input
          id="rename-project-name"
          autoFocus
          className="app-input w-full"
          maxLength={100}
          value={name}
          onChange={(event) => setName(event.target.value)}
        />

        <MutationError error={mutation.error} />

        <DialogActions
          onClose={onClose}
          submitLabel={mutation.isPending ? 'Saving…' : 'Save'}
          submitDisabled={submitDisabled}
        />
      </form>
    </DialogShell>
  );
}

function RotateKeyDialog({
  project,
  onClose,
  onRotated,
}: {
  project: Project;
  onClose: () => void;
  onRotated: (project: ProjectWithKey) => void;
}) {
  const mutation = useMutation({
    mutationFn: () => rotateProjectApiKey(project.id),
    onSuccess: onRotated,
  });

  return (
    <DialogShell title={`Rotate key for "${project.name}"`} onClose={onClose}>
      <p className="text-sm text-[var(--c-text-secondary)]">
        A new API key will be generated. The current key stops working immediately,
        and the new key will be shown to you exactly once.
      </p>

      <MutationError error={mutation.error} />

      <DialogActions
        onClose={onClose}
        submitLabel={mutation.isPending ? 'Rotating…' : 'Rotate key'}
        submitDisabled={mutation.isPending}
        onSubmit={() => mutation.mutate()}
      />
    </DialogShell>
  );
}

function DeleteProjectDialog({
  project,
  onClose,
  onDeleted,
}: {
  project: Project;
  onClose: () => void;
  onDeleted: () => void;
}) {
  const [confirm, setConfirm] = useState('');
  const [retriedWithFallback, setRetriedWithFallback] = useState(false);
  const mutation = useMutation({
    mutationFn: () => deleteProject(project.id),
    onSuccess: onDeleted,
  });

  useEffect(() => {
    if (retriedWithFallback || !isAuthError(mutation.error)) {
      return;
    }

    const fallbackKey = getFallbackProjectApiKey(getApiKey());
    if (!fallbackKey) {
      return;
    }

    setRetriedWithFallback(true);
    setApiKey(fallbackKey);
    mutation.reset();
    mutation.mutate();
  }, [mutation, mutation.error, mutation.mutate, mutation.reset, retriedWithFallback]);

  const canSubmit = confirm === project.name && !mutation.isPending;

  return (
    <DialogShell title={`Delete "${project.name}"`} onClose={onClose}>
      <p className="text-sm text-[var(--c-text-secondary)]">
        This permanently deletes the project and all its traces, sessions, spans,
        and payloads. This cannot be undone.
      </p>
      <label
        htmlFor="delete-project-confirm"
        className="mt-4 mb-2 block text-sm font-semibold text-[var(--c-text-primary)]"
      >
        Type the project name to confirm
      </label>
      <input
        id="delete-project-confirm"
        autoFocus
        className="app-input w-full"
        value={confirm}
        onChange={(event) => setConfirm(event.target.value)}
        placeholder={project.name}
      />

      <MutationError error={mutation.error} />

      <DialogActions
        onClose={onClose}
        submitLabel={mutation.isPending ? 'Deleting…' : 'Delete project'}
        submitDisabled={!canSubmit}
        submitTone="danger"
        onSubmit={() => mutation.mutate()}
      />
    </DialogShell>
  );
}

function RevealKeyDialog({
  project,
  onClose,
}: {
  project: ProjectWithKey;
  onClose: () => void;
}) {
  return (
    <DialogShell title={`API key for "${project.name}"`} onClose={onClose}>
      <div className="rounded-md border border-[var(--c-amber-border)] bg-[var(--c-amber-faint)] p-3 text-sm text-[var(--c-text-primary)]">
        Copy this key now. It will not be shown again. If you lose it, rotate the key
        to issue a new one.
      </div>

      <div className="mt-4">
        <label className="mb-2 block text-xs font-semibold uppercase tracking-wide text-[var(--c-text-muted)]">
          API key
        </label>
        <div className="flex items-center gap-2">
          <code
            data-testid="revealed-api-key"
            className="flex-1 break-all rounded-md border border-[var(--c-border)] bg-[var(--c-app-bg)] px-3 py-2 font-mono text-xs text-[var(--c-text-primary)]"
          >
            {project.api_key}
          </code>
          <CopyButton value={project.api_key} idleLabel="Copy" />
        </div>
      </div>

      <div className="mt-4">
        <label className="mb-2 block text-xs font-semibold uppercase tracking-wide text-[var(--c-text-muted)]">
          Python SDK snippet
        </label>
        <pre className="overflow-x-auto rounded-md border border-[var(--c-border)] bg-[var(--c-app-bg)] p-3 font-mono text-xs text-[var(--c-text-primary)]">
{`from continua import Continua

Continua.init(
    api_key="${project.api_key}",
    endpoint="${typeof window !== 'undefined' ? window.location.origin : 'http://localhost:8080'}",
)`}
        </pre>
      </div>

      <DialogActions onClose={onClose} closeLabel="Done" />
    </DialogShell>
  );
}

function DialogShell({
  title,
  onClose,
  children,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
}) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/45 px-4 backdrop-blur-sm">
      <button
        type="button"
        aria-label={`Close ${title} dialog`}
        className="absolute inset-0"
        onClick={onClose}
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-label={title}
        className="relative z-10 w-full max-w-xl rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] p-5 shadow-xl"
      >
        <h3 className="mb-4 text-lg font-semibold text-[var(--c-text-primary)]">
          {title}
        </h3>
        {children}
      </div>
    </div>
  );
}

function DialogActions({
  onClose,
  closeLabel = 'Cancel',
  submitLabel,
  submitDisabled,
  submitTone = 'primary',
  onSubmit,
}: {
  onClose: () => void;
  closeLabel?: string;
  submitLabel?: string;
  submitDisabled?: boolean;
  submitTone?: 'primary' | 'danger';
  onSubmit?: () => void;
}) {
  return (
    <div className="mt-5 flex justify-end gap-2">
      <button type="button" className="app-button-secondary" onClick={onClose}>
        {closeLabel}
      </button>
      {submitLabel ? (
        <button
          type={onSubmit ? 'button' : 'submit'}
          className={
            submitTone === 'danger' ? 'app-button-primary' : 'app-button-primary'
          }
          disabled={submitDisabled}
          onClick={onSubmit}
          data-tone={submitTone}
        >
          {submitLabel}
        </button>
      ) : null}
    </div>
  );
}

function MutationError({ error }: { error: unknown }) {
  if (!error) return null;
  const message =
    error instanceof ApiError
      ? error.message
      : error instanceof Error
        ? error.message
        : 'Request failed';
  return (
    <p
      role="alert"
      className="mt-3 rounded-md border border-[var(--c-red-border)] bg-[var(--c-red-faint)] px-3 py-2 text-sm text-[var(--c-text-primary)]"
    >
      {message}
    </p>
  );
}
