import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ConflictDrawer } from './ConflictDrawer';

describe('ConflictDrawer', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  const defaultProps = {
    open: true,
    draftContent: '# My Draft\n\nDraft text.',
    latestContent: '# Latest\n\nCanonical text.',
    expectedDigest: 'sha256:11112222',
    latestDigest: 'sha256:33334444',
    onUseLatest: vi.fn(),
    onDiscard: vi.fn(),
    onClose: vi.fn(),
  };

  it('renders both draft and latest content and has exactly the four actions', () => {
    render(<ConflictDrawer {...defaultProps} />);

    expect(screen.getByText('SKILL.md changed since you opened it')).toBeInTheDocument();
    expect(screen.getByText(/# My Draft/)).toBeInTheDocument();
    expect(screen.getByText(/# Latest/)).toBeInTheDocument();

    // Exactly 4 actions in drawer footer:
    expect(screen.getByRole('button', { name: 'Download draft (.md)' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Copy draft' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Discard draft and reload' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Use latest as base' })).toBeInTheDocument();
  });

  it('discard action asks for confirmation before executing', () => {
    const onDiscard = vi.fn();
    render(<ConflictDrawer {...defaultProps} onDiscard={onDiscard} />);

    const discardBtn = screen.getByRole('button', { name: 'Discard draft and reload' });
    fireEvent.click(discardBtn);

    // Confirmation dialog appears
    expect(screen.getByText('Discard draft?')).toBeInTheDocument();
    expect(screen.getByText(/Your uncommitted draft changes will be permanently discarded/)).toBeInTheDocument();
    expect(onDiscard).not.toHaveBeenCalled();

    // Confirm inside dialog
    const confirmBtn = screen.getByRole('button', { name: 'Discard draft' });
    fireEvent.click(confirmBtn);

    expect(onDiscard).toHaveBeenCalledTimes(1);
  });

  it('closes on Escape', () => {
    const onClose = vi.fn();
    render(<ConflictDrawer {...defaultProps} onClose={onClose} />);

    fireEvent.keyDown(document, { key: 'Escape' });
    expect(onClose).toHaveBeenCalled();
  });
});
