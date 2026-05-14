import { useState, type ReactNode } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import {
  ApiError,
  createProject,
  deleteProject,
  fetchProjects,
  renameProject,
  rotateProjectApiKey,
  type Project,
  type ProjectWithKey,
} from '../api/client';
import { PageHeader } from '../components/DebuggerKit';
import { CopyButton } from '../components/CopyButton';

const PROJECTS_QUERY_KEY = ['projects'] as const;
const DEFAULT_PROJECT_ID = '00000000-0000-0000-0000-000000000001';

export function ProjectsPage() {
  const queryClient = useQueryClient();
  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: PROJECTS_QUERY_KEY,
    queryFn: fetchProjects,
  });

  const [createOpen, setCreateOpen] = useState(false);
  const [renameTarget, setRenameTarget] = useState<Project | null>(null);
  const [rotateTarget, setRotateTarget] = useState<Project | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Project | null>(null);
  const [revealedKey, setRevealedKey] = useState<ProjectWithKey | null>(null);

  const invalidateProjects = () => {
    queryClient.invalidateQueries({ queryKey: PROJECTS_QUERY_KEY });
  };

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
        ) : (
          <EmptyState onCreate={() => setCreateOpen(true)} />
        )}
      </div>

      {createOpen ? (
        <CreateProjectDialog
          onClose={() => setCreateOpen(false)}
          onCreated={(project) => {
            invalidateProjects();
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
            invalidateProjects();
            setRotateTarget(null);
            setRevealedKey(project);
          }}
        />
      ) : null}

      {deleteTarget ? (
        <DeleteProjectDialog
          project={deleteTarget}
          onClose={() => setDeleteTarget(null)}
          onDeleted={() => {
            invalidateProjects();
            setDeleteTarget(null);
          }}
        />
      ) : null}

      {revealedKey ? (
        <RevealKeyDialog
          project={revealedKey}
          onClose={() => setRevealedKey(null)}
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
            const isDefault = project.id === DEFAULT_PROJECT_ID;
            return (
              <tr key={project.id} data-testid={`project-row-${project.id}`}>
                <td className="px-4 py-3 text-sm font-medium text-[var(--c-text-primary)]">
                  {project.name}
                  {isDefault ? (
                    <span className="ml-2 rounded-full border border-[var(--c-border)] px-2 py-0.5 text-[10px] uppercase tracking-wide text-[var(--c-text-muted)]">
                      Default
                    </span>
                  ) : null}
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
                      disabled={isDefault}
                      title={isDefault ? 'The seeded default project cannot be deleted' : undefined}
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
  const mutation = useMutation({
    mutationFn: () => deleteProject(project.id),
    onSuccess: onDeleted,
  });

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
      <div className="rounded-md border border-amber-500/40 bg-amber-500/10 p-3 text-sm text-[var(--c-text-primary)]">
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
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-[#111318]/45 px-4 backdrop-blur-sm">
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
        className="relative z-10 w-full max-w-xl rounded-[1rem] border border-[var(--continua-border-strong)] bg-[var(--continua-surface)] p-5 shadow-[var(--continua-shadow-soft)]"
      >
        <h3 className="mb-4 text-xl font-black tight-headline text-[var(--continua-text-primary)]">
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
      className="mt-3 rounded-md border border-red-500/40 bg-red-500/10 px-3 py-2 text-sm text-[var(--c-text-primary)]"
    >
      {message}
    </p>
  );
}
