export async function copyToClipboard(text: string): Promise<void> {
  if (
    typeof navigator === 'undefined' ||
    navigator.clipboard === undefined ||
    typeof navigator.clipboard.writeText !== 'function'
  ) {
    throw new Error('Clipboard API is unavailable');
  }

  await navigator.clipboard.writeText(text);
}
