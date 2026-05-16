import { useEffect, useRef, useState, type ButtonHTMLAttributes, type ReactNode } from 'react';
import { copyToClipboard } from '../utils/clipboard';

const FEEDBACK_DURATION_MS = 2000;

type CopyButtonProps =
  | ({
      getValue: () => string;
      value?: never;
    } & CopyButtonBaseProps)
  | ({
      getValue?: never;
      value: string;
    } & CopyButtonBaseProps);

interface CopyButtonBaseProps
  extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'children'> {
  errorLabel?: ReactNode;
  idleLabel?: ReactNode;
  successLabel?: ReactNode;
}

type CopyStatus = 'idle' | 'success' | 'error';

export function CopyButton({
  className = '',
  errorLabel = 'Failed',
  getValue,
  idleLabel = 'Copy',
  successLabel = 'Copied',
  type = 'button',
  value,
  ...buttonProps
}: CopyButtonProps) {
  const timeoutRef = useRef<number | null>(null);
  const [status, setStatus] = useState<CopyStatus>('idle');

  useEffect(() => {
    return () => {
      if (timeoutRef.current !== null) {
        window.clearTimeout(timeoutRef.current);
      }
    };
  }, []);

  const handleClick = async () => {
    if (timeoutRef.current !== null) {
      window.clearTimeout(timeoutRef.current);
    }

    try {
      await copyToClipboard(getValue ? getValue() : value);
      setStatus('success');
    } catch {
      setStatus('error');
    }

    timeoutRef.current = window.setTimeout(() => {
      setStatus('idle');
      timeoutRef.current = null;
    }, FEEDBACK_DURATION_MS);
  };

  const label =
    status === 'success'
      ? successLabel
      : status === 'error'
        ? errorLabel
        : idleLabel;

  return (
    <button
      {...buttonProps}
      type={type}
      className={`inline-flex items-center justify-center rounded-md border border-[var(--c-border)] bg-[var(--c-surface)] px-2.5 py-1 text-xs font-medium text-[var(--c-text-secondary)] transition hover:border-[var(--c-border-strong)] hover:text-[var(--c-text-primary)] focus:outline-none focus:ring-2 focus:ring-[var(--c-accent-faint)] disabled:cursor-not-allowed disabled:opacity-60 ${className}`.trim()}
      data-copy-status={status}
      onClick={handleClick}
    >
      {label}
    </button>
  );
}
