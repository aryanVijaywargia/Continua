import { useCallback, useEffect } from 'react';
import { useSearchParams } from 'react-router-dom';
import {
  clearChip as clearTracesChip,
  parseTracesParams,
  serializeTracesParams,
  type ChipKey,
  type TracesFilterState,
} from '../utils/tracesSearchParams';

type HistoryMode = 'push' | 'replace';

function shouldResetOffset(
  filters: TracesFilterState,
  updates: Partial<TracesFilterState>
): boolean {
  return Object.entries(updates).some(
    ([key, value]) =>
      key !== 'offset' &&
      filters[key as keyof TracesFilterState] !== value
  );
}

export function useTracesSearchParams() {
  const [searchParams, setSearchParams] = useSearchParams();
  const filters = parseTracesParams(searchParams);
  const canonicalSearch = serializeTracesParams(filters).toString();

  useEffect(() => {
    if (searchParams.toString() === canonicalSearch) {
      return;
    }

    setSearchParams(new URLSearchParams(canonicalSearch), { replace: true });
  }, [canonicalSearch, searchParams, setSearchParams]);

  const setFilters = useCallback(
    (updates: Partial<TracesFilterState>, mode: HistoryMode = 'push') => {
      const next = {
        ...filters,
        ...updates,
      };

      const normalizedNext = shouldResetOffset(filters, updates)
        ? { ...next, offset: 0 }
        : next;

      setSearchParams(serializeTracesParams(normalizedNext), {
        replace: mode === 'replace',
      });
    },
    [filters, setSearchParams]
  );

  const clearAll = useCallback(() => {
    setSearchParams(new URLSearchParams(), { replace: false });
  }, [setSearchParams]);

  const clearChip = useCallback(
    (key: ChipKey, mode: HistoryMode = 'push') => {
      setSearchParams(serializeTracesParams(clearTracesChip(filters, key)), {
        replace: mode === 'replace',
      });
    },
    [filters, setSearchParams]
  );

  return {
    filters,
    setFilters,
    clearAll,
    clearChip,
  };
}
