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
      className={`inline-flex items-center justify-center rounded-full border border-slate-200 bg-white px-2.5 py-1 text-xs font-medium text-slate-600 transition hover:bg-slate-100 focus:outline-none focus:ring-2 focus:ring-blue-200 disabled:cursor-not-allowed disabled:opacity-60 dark:border-slate-700 dark:bg-slate-900 dark:text-slate-200 dark:hover:bg-slate-800 ${className}`.trim()}
      data-copy-status={status}
      onClick={handleClick}
    >
      {label}
    </button>
  );
}
