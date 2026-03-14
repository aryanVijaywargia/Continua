import { PayloadInspector } from './PayloadInspector';

interface JsonViewerProps {
  data: unknown;
  className?: string;
}

/**
 * Shared JSON viewer for structured payloads.
 */
export function JsonViewer({ data, className = '' }: JsonViewerProps) {
  return <PayloadInspector data={data} className={className} />;
}
