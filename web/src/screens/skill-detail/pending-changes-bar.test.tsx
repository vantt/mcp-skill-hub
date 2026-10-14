import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { PendingChangesBar } from './PendingChangesBar';

describe('PendingChangesBar', () => {
  it('renders nothing without changes', () => {
    const { container } = render(<PendingChangesBar count={0} onDiscard={() => {}} onReview={() => {}} />);
    expect(container).toBeEmptyDOMElement();
  });

  it('counts the changes, says agents keep the current version, and offers Discard and Review', () => {
    const onDiscard = vi.fn();
    const onReview = vi.fn();
    const { rerender } = render(<PendingChangesBar count={1} onDiscard={onDiscard} onReview={onReview} />);
    expect(screen.getByText('1 change not committed')).toBeInTheDocument();
    expect(screen.getByText(/agents still get the current version/)).toBeInTheDocument();
    rerender(<PendingChangesBar count={2} onDiscard={onDiscard} onReview={onReview} />);
    expect(screen.getByText('2 changes not committed')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Discard' }));
    fireEvent.click(screen.getByRole('button', { name: /Review/ }));
    expect(onDiscard).toHaveBeenCalledOnce();
    expect(onReview).toHaveBeenCalledOnce();
  });
});
