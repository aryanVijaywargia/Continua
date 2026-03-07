interface JsonViewerProps {
  data: unknown;
  className?: string;
}

/**
 * Shared JSON viewer for structured payloads.
 */
export function JsonViewer({ data, className = '' }: JsonViewerProps) {
  return (
    <pre
      className={`overflow-x-auto rounded border bg-gray-50 p-3 text-xs font-mono leading-5 text-gray-800 ${className}`.trim()}
    >
      {JSON.stringify(data ?? null, null, 2)}
    </pre>
  );
}
