import { useCallback, useEffect } from 'react';
import { useSearchParams } from 'react-router-dom';
import {
  parseSpanParam,
  serializeSpanParam,
} from '../utils/traceDetailSearchParams';

export function useTraceDetailSearchParams() {
  const [searchParams, setSearchParams] = useSearchParams();
  const rawSpanParam = searchParams.get('span');
  const spanParam = parseSpanParam(searchParams);

  useEffect(() => {
    if (rawSpanParam !== '') {
      return;
    }

    setSearchParams(serializeSpanParam(searchParams, null), { replace: true });
  }, [rawSpanParam, searchParams, setSearchParams]);

  const setSpanParam = useCallback(
    (spanId: string | null) => {
      setSearchParams(serializeSpanParam(searchParams, spanId), { replace: true });
    },
    [searchParams, setSearchParams]
  );

  return {
    spanParam,
    setSpanParam,
  };
}
