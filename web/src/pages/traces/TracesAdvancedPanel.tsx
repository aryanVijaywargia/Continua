import type { KeyboardEvent, ReactNode } from 'react';
import type { TracesFilterState } from '../../utils/tracesSearchParams';
import {
  ENGINE_PROJECTION_STATE_FILTER_VALUES,
  ENGINE_RUN_STATUS_FILTER_VALUES,
  formatEngineProjectionStateLabel,
  formatEngineRunStatusLabel,
} from '../../utils/tracesSearchParams';

type SetFilters = (patch: Partial<TracesFilterState>, mode: 'push' | 'replace') => void;

export interface DraftField {
  value: string;
  onChange: (value: string) => void;
  onKeyDown: (event: KeyboardEvent<HTMLInputElement>) => void;
}

const FIELD_CLASS =
  'h-7 w-full min-w-0 rounded border border-[var(--c-border)] bg-[var(--c-app-bg)] px-2 text-xs text-[var(--c-text-primary)] outline-none focus:ring-2 focus:ring-[var(--c-accent-faint)]';

function Field({ children, label }: { children: ReactNode; label: string }) {
  return (
    <div className="flex min-w-0 flex-col gap-1">
      <span className="text-[11px] font-medium text-[var(--c-text-muted)]">{label}</span>
      {children}
    </div>
  );
}

export function TracesAdvancedPanel({
  engineDefinitions,
  engineInstanceKey,
  filters,
  id,
  minDuration,
  setFilters,
  userId,
}: {
  engineDefinitions: string[];
  engineInstanceKey: DraftField;
  filters: TracesFilterState;
  id: string;
  minDuration: DraftField;
  setFilters: SetFilters;
  userId: DraftField;
}) {
  return (
    <div
      id={id}
      className="grid gap-x-6 gap-y-3 border-b border-[var(--c-border)] bg-[var(--c-surface-muted)] px-6 py-3 md:grid-cols-2"
    >
      <fieldset className="min-w-0">
        <legend className="mb-2 text-xs font-semibold text-[var(--c-text-primary)]">Trace</legend>
        <div className="grid grid-cols-2 gap-3">
          <Field label="User">
            <input
              aria-label="User ID"
              value={userId.value}
              onChange={(event) => userId.onChange(event.target.value)}
              onKeyDown={userId.onKeyDown}
              placeholder="usr_…"
              className={FIELD_CLASS}
            />
          </Field>
          <Field label="Minimum duration">
            <input
              aria-label="Min Duration (ms)"
              type="number"
              min="1"
              step="1"
              value={minDuration.value}
              onChange={(event) => minDuration.onChange(event.target.value)}
              onKeyDown={minDuration.onKeyDown}
              placeholder="ms"
              className={FIELD_CLASS}
            />
          </Field>
        </div>
        <label className="mt-2.5 flex cursor-pointer items-center gap-2 text-[12.5px] text-[var(--c-text-secondary)]">
          <input
            aria-label="Only show traces with errors"
            type="checkbox"
            checked={Boolean(filters.has_errors)}
            onChange={() =>
              setFilters({ has_errors: filters.has_errors ? undefined : true }, 'push')
            }
            className="h-[13px] w-[13px] accent-[var(--c-accent)]"
          />
          Only traces with failed steps
        </label>
      </fieldset>

      <fieldset className="min-w-0">
        <legend className="mb-2 text-xs font-semibold text-[var(--c-text-primary)]">
          Engine and projection
        </legend>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Definition">
            <select
              aria-label="Engine Definition"
              value={filters.engine_definition_name ?? ''}
              onChange={(event) =>
                setFilters(
                  { engine_definition_name: event.target.value || undefined },
                  'push'
                )
              }
              className={FIELD_CLASS}
            >
              <option value="">
                {engineDefinitions.length > 0 ? 'All definitions' : 'No engine definitions'}
              </option>
              {engineDefinitions.map((name) => (
                <option key={name} value={name}>
                  {name}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Instance">
            <input
              aria-label="Engine Instance Key"
              value={engineInstanceKey.value}
              onChange={(event) => engineInstanceKey.onChange(event.target.value)}
              onKeyDown={engineInstanceKey.onKeyDown}
              placeholder="Instance key"
              className={FIELD_CLASS}
            />
          </Field>
          <Field label="Run status">
            <select
              aria-label="Engine Status"
              value={filters.engine_run_status ?? ''}
              onChange={(event) =>
                setFilters(
                  {
                    engine_run_status: event.target.value
                      ? (event.target.value as (typeof ENGINE_RUN_STATUS_FILTER_VALUES)[number])
                      : undefined,
                  },
                  'push'
                )
              }
              className={FIELD_CLASS}
            >
              <option value="">All engine statuses</option>
              {ENGINE_RUN_STATUS_FILTER_VALUES.map((value) => (
                <option key={value} value={value}>
                  {formatEngineRunStatusLabel(value)}
                </option>
              ))}
            </select>
          </Field>
          <Field label="Projection">
            <select
              aria-label="Projection State"
              value={filters.engine_projection_state ?? ''}
              onChange={(event) =>
                setFilters(
                  {
                    engine_projection_state: event.target.value
                      ? (event.target
                          .value as (typeof ENGINE_PROJECTION_STATE_FILTER_VALUES)[number])
                      : undefined,
                  },
                  'push'
                )
              }
              className={FIELD_CLASS}
            >
              <option value="">All projection states</option>
              {ENGINE_PROJECTION_STATE_FILTER_VALUES.map((value) => (
                <option key={value} value={value}>
                  {formatEngineProjectionStateLabel(value)}
                </option>
              ))}
            </select>
          </Field>
        </div>
        <p className="mt-2 text-[11px] leading-4 text-[var(--c-text-muted)]">
          Advanced operator filter for inspecting projection health across engine traces.
        </p>
      </fieldset>
    </div>
  );
}
