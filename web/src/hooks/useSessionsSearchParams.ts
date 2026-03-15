import { useCallback, useEffect } from 'react';
import { useSearchParams } from 'react-router-dom';
import {
  parseSessionsParams,
  serializeSessionsParams,
  type SessionsSearchState,
} from '../utils/sessionsSearchParams';

type HistoryMode = 'push' | 'replace';

function shouldResetOffset(
  filters: SessionsSearchState,
  updates: Partial<SessionsSearchState>
): boolean {
  return Object.entries(updates).some(
    ([key, value]) =>
      key !== 'offset' &&
      filters[key as keyof SessionsSearchState] !== value
  );
}

function stripSortWhenSearchActive(
  filters: SessionsSearchState
): SessionsSearchState {
  if (!filters.q) {
    return filters;
  }

  return {
    ...filters,
    sort_by: undefined,
    sort_dir: undefined,
  };
}

export function useSessionsSearchParams() {
  const [searchParams, setSearchParams] = useSearchParams();
  const filters = stripSortWhenSearchActive(parseSessionsParams(searchParams));
  const canonicalSearch = serializeSessionsParams(filters).toString();

  useEffect(() => {
    if (searchParams.toString() === canonicalSearch) {
      return;
    }

    setSearchParams(new URLSearchParams(canonicalSearch), { replace: true });
  }, [canonicalSearch, searchParams, setSearchParams]);

  const setFilters = useCallback(
    (updates: Partial<SessionsSearchState>, mode: HistoryMode = 'push') => {
      const next = {
        ...filters,
        ...updates,
      };

      const resetNext = shouldResetOffset(filters, updates)
        ? { ...next, offset: 0 }
        : next;
      const normalizedNext = stripSortWhenSearchActive(resetNext);

      setSearchParams(serializeSessionsParams(normalizedNext), {
        replace: mode === 'replace',
      });
    },
    [filters, setSearchParams]
  );

  const clearAll = useCallback(() => {
    setSearchParams(new URLSearchParams(), { replace: false });
  }, [setSearchParams]);

  return {
    filters,
    setFilters,
    clearAll,
  };
}
