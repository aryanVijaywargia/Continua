import type {
  ButtonHTMLAttributes,
  HTMLAttributes,
  InputHTMLAttributes,
  ReactNode,
} from 'react';
import {
  ArrowDown,
  ArrowUp,
  ChevronDown,
  ChevronRight,
  Search,
  X,
  type LucideIcon,
} from 'lucide-react';
import { normalizeStatus, STATUS_TONE } from './statusTone';

export function StatusDot({
  status,
  withLabel = true,
  pulse,
}: {
  status?: string | null;
  withLabel?: boolean;
  pulse?: boolean;
}) {
  const normalizedStatus = normalizeStatus(status);
  const tone = STATUS_TONE[normalizedStatus];
  const isLive =
    pulse ?? (normalizedStatus === 'RUNNING' || normalizedStatus === 'STARTED');

  return (
    <span
      className="inline-flex items-center gap-[7px] whitespace-nowrap text-xs font-medium"
      style={{ color: tone.text }}
    >
      <span className="relative inline-block h-[7px] w-[7px]">
        <span
          className="absolute inset-0 rounded-full"
          style={{ background: tone.dot }}
        />
        {isLive ? (
          <span
            className="absolute -inset-1 rounded-full"
            style={{
              animation: 'continua-pulse 1.6s ease-out infinite',
              background: tone.dot,
              opacity: 0.25,
            }}
          />
        ) : null}
      </span>
      {withLabel ? tone.label : null}
    </span>
  );
}

export function Mono({
  children,
  className = '',
  dim = false,
}: {
  children: ReactNode;
  className?: string;
  dim?: boolean;
}) {
  return (
    <span
      className={`font-mono text-xs tabular-nums ${className}`}
      style={{ color: dim ? 'var(--c-text-muted)' : 'var(--c-text-secondary)' }}
    >
      {children}
    </span>
  );
}

export function Kbd({ children }: { children: ReactNode }) {
  return (
    <kbd className="rounded-[3px] border border-[var(--c-border)] bg-[var(--c-kbd-bg)] px-1.5 py-px font-mono text-[10.5px] font-semibold text-[var(--c-text-muted)]">
      {children}
    </kbd>
  );
}

type ButtonKind = 'primary' | 'secondary' | 'ghost' | 'accent';
type ButtonSize = 'sm' | 'md' | 'lg';

const buttonKindClasses: Record<ButtonKind, string> = {
  primary: 'border-transparent bg-[var(--c-text-primary)] text-[var(--c-app-bg)] font-semibold',
  secondary: 'border-[var(--c-border)] bg-[var(--c-surface)] text-[var(--c-text-primary)] font-medium',
  ghost: 'border-transparent bg-transparent text-[var(--c-text-secondary)] font-medium hover:bg-[var(--c-nav-hover-bg)]',
  accent: 'border-transparent bg-[var(--c-accent)] text-white font-semibold',
};

const buttonSizeClasses: Record<ButtonSize, string> = {
  sm: 'h-7 gap-1.5 px-2.5 text-xs',
  md: 'h-8 gap-2 px-3 text-[13px]',
  lg: 'h-9 gap-2 px-4 text-[13px]',
};

export function Btn({
  children,
  className = '',
  kind = 'ghost',
  leadingIcon: LeadingIcon,
  size = 'md',
  ...props
}: ButtonHTMLAttributes<HTMLButtonElement> & {
  kind?: ButtonKind;
  leadingIcon?: LucideIcon;
  size?: ButtonSize;
}) {
  return (
    <button
      {...props}
      className={`inline-flex items-center justify-center rounded-md border font-sans transition hover:border-[var(--c-border-strong)] disabled:cursor-not-allowed disabled:opacity-50 ${buttonKindClasses[kind]} ${buttonSizeClasses[size]} ${className}`}
    >
      {LeadingIcon ? <LeadingIcon className={size === 'sm' ? 'h-3.5 w-3.5' : 'h-4 w-4'} /> : null}
      {children}
    </button>
  );
}

const chipToneClasses = {
  muted: 'border-[var(--c-border)] bg-[var(--c-surface)] text-[var(--c-text-secondary)]',
  accent: 'border-[var(--c-accent-border)] bg-[var(--c-accent-faint)] text-[var(--c-accent-text)]',
  error: 'border-[var(--c-red-border)] bg-[var(--c-red-faint)] text-[var(--c-red-text)]',
  success: 'border-[var(--c-green-border)] bg-[var(--c-green-faint)] text-[var(--c-green-text)]',
  amber: 'border-[var(--c-amber-border)] bg-[var(--c-amber-faint)] text-[var(--c-amber-text)]',
} as const;

const textAlignClasses = {
  center: 'text-center',
  left: 'text-left',
  right: 'text-right',
} as const;

export function Chip({
  children,
  className = '',
  closeLabel = 'Remove filter',
  icon: Icon,
  onClose,
  tone = 'muted',
}: {
  children: ReactNode;
  className?: string;
  closeLabel?: string;
  icon?: LucideIcon;
  onClose?: () => void;
  tone?: keyof typeof chipToneClasses;
}) {
  return (
    <span
      className={`inline-flex h-5 items-center gap-1.5 whitespace-nowrap rounded border px-1.5 text-[11px] font-medium ${chipToneClasses[tone]} ${className}`}
    >
      {Icon ? <Icon className="h-3 w-3" /> : null}
      {children}
      {onClose ? (
        <button
          type="button"
          aria-label={closeLabel}
          className="-mr-0.5 inline-flex text-current opacity-70 hover:opacity-100 focus:outline-none focus:ring-2 focus:ring-[var(--c-accent-faint)]"
          onClick={onClose}
        >
          <X className="h-2.5 w-2.5" />
        </button>
      ) : null}
    </span>
  );
}

export function PageHeader({
  actions,
  description,
  eyebrow,
  tabs,
  title,
}: {
  actions?: ReactNode;
  description?: ReactNode;
  eyebrow?: ReactNode;
  tabs?: Array<{ active?: boolean; count?: number; id: string; label: string; onClick?: () => void }>;
  title: ReactNode;
}) {
  return (
    <header className={`border-b border-[var(--c-border)] px-6 pt-5 ${tabs ? 'pb-0' : 'pb-4'}`}>
      <div className="flex items-start justify-between gap-6">
        <div className="min-w-0">
          {eyebrow ? (
            <div className="mb-1.5 text-[11px] font-medium tracking-[0.01em] text-[var(--c-text-muted)]">
              {eyebrow}
            </div>
          ) : null}
          <h1 className="m-0 text-xl font-bold leading-tight tracking-[-0.015em] text-[var(--c-text-primary)]">
            {title}
          </h1>
          {description ? (
            <p className="mt-1.5 max-w-3xl text-[13px] leading-6 text-[var(--c-text-secondary)]">
              {description}
            </p>
          ) : null}
        </div>
        {actions ? <div className="flex shrink-0 items-center gap-1.5">{actions}</div> : null}
      </div>
      {tabs ? (
        <div className="mt-3 flex border-b border-[var(--c-border)]">
          {tabs.map((tab) => (
            <button
              key={tab.id}
              type="button"
              onClick={tab.onClick}
              className={`-mb-px border-b-2 px-3.5 py-2 text-[13px] font-medium ${
                tab.active
                  ? 'border-[var(--c-accent)] text-[var(--c-text-primary)]'
                  : 'border-transparent text-[var(--c-text-secondary)]'
              }`}
            >
              {tab.label}
              {tab.count != null ? (
                <span className="ml-1.5 font-mono text-[11px] tabular-nums text-[var(--c-text-muted)]">
                  {tab.count}
                </span>
              ) : null}
            </button>
          ))}
        </div>
      ) : null}
    </header>
  );
}

export function FilterBar({
  children,
  className = '',
  count = 0,
  onClear,
  right,
}: {
  children: ReactNode;
  className?: string;
  count?: number;
  onClear?: () => void;
  right?: ReactNode;
}) {
  return (
    <div className={`flex flex-wrap items-center gap-2 border-b border-[var(--c-border)] bg-[var(--c-app-bg)] px-6 py-2.5 ${className}`}>
      {children}
      {count > 0 && onClear ? (
        <button
          type="button"
          className="ml-1 text-xs font-medium text-[var(--c-text-muted)] hover:text-[var(--c-text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--c-accent-faint)]"
          onClick={onClear}
        >
          Clear ({count})
        </button>
      ) : null}
      <div className="flex-1" />
      {right}
    </div>
  );
}

export function SearchInput({
  className = '',
  onClear,
  widthClass = 'w-[280px]',
  ...props
}: InputHTMLAttributes<HTMLInputElement> & {
  onClear?: () => void;
  widthClass?: string;
}) {
  return (
    <div className={`flex h-7 items-center gap-1.5 rounded-md border border-[var(--c-border)] bg-[var(--c-app-bg)] px-2.5 ${widthClass} ${className}`}>
      <Search className="h-3.5 w-3.5 text-[var(--c-text-muted)]" />
      <input
        {...props}
        className="app-input min-w-0 flex-1 border-0 bg-transparent p-0 text-[12.5px] text-[var(--c-text-primary)] outline-none placeholder:text-[var(--c-text-muted)] focus:border-0 focus:ring-0"
      />
      {props.value && onClear ? (
        <button
          type="button"
          className="text-[var(--c-text-muted)] hover:text-[var(--c-text-primary)]"
          onClick={onClear}
        >
          <X className="h-3 w-3" />
        </button>
      ) : null}
    </div>
  );
}

export function DataTable({
  children,
  className = '',
}: {
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={`min-h-0 flex-1 overflow-auto bg-[var(--c-app-bg)] ${className}`}>
      <table className="w-full table-fixed border-separate border-spacing-0">{children}</table>
    </div>
  );
}

export function Th({
  align = 'left',
  children,
  className = '',
  onSort,
  sortActive = false,
  sortDir = 'desc',
  sortable = false,
}: {
  align?: 'left' | 'right' | 'center';
  children: ReactNode;
  className?: string;
  onSort?: () => void;
  sortActive?: boolean;
  sortDir?: 'asc' | 'desc';
  sortable?: boolean;
}) {
  const content = sortable ? (
    <button
      type="button"
      onClick={onSort}
      className={`inline-flex items-center gap-1 ${sortActive ? 'text-[var(--c-text-primary)]' : ''}`}
    >
      {children}
      {sortActive ? (
        sortDir === 'asc' ? <ArrowUp className="h-2.5 w-2.5" /> : <ArrowDown className="h-2.5 w-2.5" />
      ) : null}
    </button>
  ) : (
    children
  );

  return (
    <th
      className={`sticky top-0 z-10 h-[30px] whitespace-nowrap border-b border-[var(--c-border)] bg-[var(--c-table-head-bg)] px-3.5 py-1.5 ${textAlignClasses[align]} text-[11px] font-semibold tracking-[0.02em] text-[var(--c-text-muted)] ${className}`}
    >
      {content}
    </th>
  );
}

export function Td({
  align = 'left',
  children,
  className = '',
  dim = false,
  mono = false,
}: {
  align?: 'left' | 'right' | 'center';
  children: ReactNode;
  className?: string;
  dim?: boolean;
  mono?: boolean;
}) {
  return (
    <td
      className={`h-10 max-w-0 overflow-hidden text-ellipsis whitespace-nowrap border-b border-[var(--c-border-subtle)] px-3.5 ${textAlignClasses[align]} align-middle text-[13px] ${
        mono ? 'font-mono tabular-nums' : ''
      } ${dim ? 'text-[var(--c-text-muted)]' : 'text-[var(--c-text-primary)]'} ${className}`}
    >
      {children}
    </td>
  );
}

export function Tr({
  children,
  className = '',
  selected = false,
  ...props
}: HTMLAttributes<HTMLTableRowElement> & {
  selected?: boolean;
}) {
  return (
    <tr
      {...props}
      className={`${selected ? 'bg-[var(--c-row-selected-bg)]' : ''} ${className}`}
    >
      {children}
    </tr>
  );
}

export function FacetGroup({
  children,
  count,
  defaultOpen = true,
  label,
}: {
  children: ReactNode;
  count?: number;
  defaultOpen?: boolean;
  label: ReactNode;
}) {
  return (
    <details className="border-b border-[var(--c-border)]" open={defaultOpen}>
      <summary className="flex h-9 cursor-pointer list-none items-center gap-2 px-3.5 text-xs font-semibold text-[var(--c-text-primary)] [&::-webkit-details-marker]:hidden">
        <ChevronRight className="h-3 w-3 text-[var(--c-text-muted)] details-open:hidden" />
        <ChevronDown className="hidden h-3 w-3 text-[var(--c-text-muted)] details-open:block" />
        <span>{label}</span>
        {count ? <span className="font-mono text-[11px] text-[var(--c-text-muted)]">{count}</span> : null}
      </summary>
      <div className="px-3.5 pb-3">{children}</div>
    </details>
  );
}

export function FacetItem({
  ariaLabel,
  checked,
  count,
  dot,
  label,
  onChange,
}: {
  ariaLabel?: string;
  checked: boolean;
  count?: number;
  dot?: string;
  label: ReactNode;
  onChange: () => void;
}) {
  return (
    <label className="flex cursor-pointer items-center gap-2 py-1 text-[12.5px] text-[var(--c-text-secondary)]">
      <input
        aria-label={ariaLabel}
        checked={checked}
        className="h-[13px] w-[13px] accent-[var(--c-accent)]"
        onChange={onChange}
        type="checkbox"
      />
      {dot ? <span className="h-1.5 w-1.5 rounded-full" style={{ background: dot }} /> : null}
      <span className={`min-w-0 flex-1 truncate ${checked ? 'font-medium text-[var(--c-text-primary)]' : ''}`}>
        {label}
      </span>
      {count != null ? (
        <span className="font-mono text-[11px] tabular-nums text-[var(--c-text-muted)]">{count}</span>
      ) : null}
    </label>
  );
}
