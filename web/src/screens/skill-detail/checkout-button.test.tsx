import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { CheckoutButton } from './CheckoutButton';

describe('CheckoutButton', () => {
  it('opens a dialog with the checkout command, copies it, and names the way back', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
    render(<CheckoutButton skillId="pdf-tools" />);

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Checkout to a project' }));

    expect(screen.getByRole('dialog')).toBeInTheDocument();
    expect(screen.getByText('skillhub skill checkout pdf-tools')).toBeInTheDocument();
    expect(screen.getByText(/try the skill with your agent, then/)).toBeInTheDocument();
    expect(screen.getByText('skillhub skill commit pdf-tools')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Copy command' }));
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('skillhub skill checkout pdf-tools'));

    fireEvent.click(screen.getByRole('button', { name: 'Close' }));
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });
});
