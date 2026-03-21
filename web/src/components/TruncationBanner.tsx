import { formatBytes } from '../utils/format';

interface TruncationBannerProps {
  originalSizeBytes?: number;
  reason?: string;
  title: string;
  truncated?: boolean;
}

export function TruncationBanner({
  originalSizeBytes,
  reason,
  title,
  truncated,
}: TruncationBannerProps) {
  if (!truncated) {
    return null;
  }

  const details = [
    originalSizeBytes !== undefined
      ? `Original size: ${formatBytes(originalSizeBytes)}`
      : null,
    reason ? `Reason: ${formatReason(reason)}` : null,
  ].filter((detail): detail is string => detail !== null);

  return (
    <div className="mb-3 rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-900 dark:border-amber-500/40 dark:bg-amber-500/10 dark:text-amber-100">
      <div className="text-[11px] font-semibold uppercase tracking-[0.16em] text-amber-800 dark:text-amber-200">
        Payload truncated
      </div>
      <p className="mt-1">{title} was truncated before storage.</p>
      {details.length > 0 && (
        <p className="mt-1 text-xs text-amber-800 dark:text-amber-200">{details.join(' | ')}</p>
      )}
    </div>
  );
}

function formatReason(reason: string): string {
  return reason.replaceAll('_', ' ');
}
