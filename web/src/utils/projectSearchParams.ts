const PROJECT_ID_PATTERN =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export function normalizeProjectId(
  value: string | null | undefined
): string | undefined {
  const trimmed = value?.trim();
  if (!trimmed || !PROJECT_ID_PATTERN.test(trimmed)) {
    return undefined;
  }

  return trimmed.toLowerCase();
}

export function getProjectIdFromSearchParams(
  searchParams: URLSearchParams
): string | undefined {
  return normalizeProjectId(searchParams.get('project_id'));
}

export function buildProjectOnlySearch(projectId?: string): string {
  if (!projectId) {
    return '';
  }

  return `?project_id=${projectId}`;
}

export function buildProjectPath(pathname: string, projectId?: string): string {
  return `${pathname}${buildProjectOnlySearch(projectId)}`;
}

export function appendProjectToPath(path: string, projectId?: string): string {
  const url = new URL(path, 'http://localhost');
  if (projectId) {
    url.searchParams.set('project_id', projectId);
  }

  return `${url.pathname}${url.search}`;
}

export function mergeProjectIntoSearch(
  search: string | URLSearchParams,
  projectId?: string
): string {
  const params =
    search instanceof URLSearchParams ? new URLSearchParams(search) : new URLSearchParams(search);

  if (projectId) {
    params.set('project_id', projectId);
  } else {
    params.delete('project_id');
  }

  const query = params.toString();
  return query ? `?${query}` : '';
}
