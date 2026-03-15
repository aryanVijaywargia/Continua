import { useState } from 'react';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it, vi } from 'vitest';
import { PaginationControls } from './PaginationControls';

describe('PaginationControls', () => {
  it('renders advanced pagination controls and disabled states', () => {
    render(
      <PaginationControls
        offset={0}
        pageSize={20}
        total={150}
        onOffsetChange={vi.fn()}
        onPageSizeChange={vi.fn()}
      />
    );

    expect(screen.getByText('Showing 1-20 of 150')).toBeInTheDocument();
    expect(screen.getByText('Page 1 of 8')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'First page' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Previous page' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Next page' })).toBeEnabled();
    expect(screen.getByRole('button', { name: 'Last page' })).toBeEnabled();

    const rowsPerPage = screen.getByRole('combobox', { name: 'Rows per page' });
    expect(rowsPerPage).toHaveDisplayValue('20');
    expect(screen.getByRole('option', { name: '20' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: '50' })).toBeInTheDocument();
    expect(screen.getByRole('option', { name: '100' })).toBeInTheDocument();
  });

  it('navigates to the last page', async () => {
    const user = userEvent.setup();

    function Wrapper() {
      const [offset, setOffset] = useState(0);

      return (
        <PaginationControls
          offset={offset}
          pageSize={20}
          total={150}
          onOffsetChange={setOffset}
          onPageSizeChange={vi.fn()}
        />
      );
    }

    render(<Wrapper />);
    await user.click(screen.getByRole('button', { name: 'Last page' }));

    expect(screen.getByText('Showing 141-150 of 150')).toBeInTheDocument();
    expect(screen.getByText('Page 8 of 8')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Next page' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Last page' })).toBeDisabled();
  });

  it('resets offset when the page size changes', async () => {
    const user = userEvent.setup();

    function Wrapper() {
      const [offset, setOffset] = useState(40);
      const [pageSize, setPageSize] = useState(20);

      return (
        <PaginationControls
          offset={offset}
          pageSize={pageSize}
          total={150}
          onOffsetChange={setOffset}
          onPageSizeChange={(nextPageSize) => {
            setPageSize(nextPageSize);
            setOffset(0);
          }}
        />
      );
    }

    render(<Wrapper />);
    await user.selectOptions(screen.getByRole('combobox', { name: 'Rows per page' }), '50');

    expect(screen.getByText('Showing 1-50 of 150')).toBeInTheDocument();
    expect(screen.getByText('Page 1 of 3')).toBeInTheDocument();
  });

  it('repairs stale offsets automatically', async () => {
    function Wrapper() {
      const [offset, setOffset] = useState(80);

      return (
        <PaginationControls
          offset={offset}
          pageSize={20}
          total={60}
          currentItemCount={0}
          onOffsetChange={setOffset}
          onPageSizeChange={vi.fn()}
          onRepairOffset={setOffset}
        />
      );
    }

    render(<Wrapper />);

    await waitFor(() => {
      expect(screen.getByText('Showing 41-60 of 60')).toBeInTheDocument();
    });
  });
});
