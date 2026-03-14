import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { TruncationBanner } from './TruncationBanner';

describe('TruncationBanner', () => {
  it('renders full metadata', () => {
    render(
      <TruncationBanner
        title="Input payload"
        truncated
        originalSizeBytes={524288}
        reason="size_limit"
      />
    );

    expect(screen.getByText('Payload truncated')).toBeInTheDocument();
    expect(
      screen.getByText(/Input payload was truncated before storage\./)
    ).toBeInTheDocument();
    expect(screen.getByText(/Original size: 512.0 KB/)).toBeInTheDocument();
    expect(screen.getByText(/Reason: size limit/)).toBeInTheDocument();
  });

  it('renders partial metadata', () => {
    render(
      <TruncationBanner
        title="Output payload"
        truncated
        originalSizeBytes={1048576}
      />
    );

    expect(screen.getByText(/Original size: 1.0 MB/)).toBeInTheDocument();
    expect(screen.queryByText(/Reason:/)).not.toBeInTheDocument();
  });

  it('renders flag-only metadata', () => {
    render(<TruncationBanner title="Input payload" truncated />);

    expect(screen.getByText(/Input payload was truncated before storage\./)).toBeInTheDocument();
  });

  it('renders nothing when truncation is false or absent', () => {
    const { rerender, container } = render(
      <TruncationBanner
        title="Input payload"
        truncated={false}
        originalSizeBytes={2048}
      />
    );

    expect(container).toBeEmptyDOMElement();

    rerender(<TruncationBanner title="Input payload" />);
    expect(container).toBeEmptyDOMElement();
  });
});
