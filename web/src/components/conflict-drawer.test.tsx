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

  it('says what each side changed and has exactly the four actions', () => {
    render(
      <ConflictDrawer
        {...defaultProps}
        mine={['Instructions (SKILL.md): 2 lines added, 0 lines removed.']}
        theirs={['Description: "Old" becomes "New".']}
        bothFields={['Description']}
      />,
    );

    expect(screen.getByText('SKILL.md changed since you opened it')).toBeInTheDocument();
    expect(screen.getByText('What you changed')).toBeInTheDocument();
    expect(screen.getByText('Instructions (SKILL.md): 2 lines added, 0 lines removed.')).toBeInTheDocument();
    expect(screen.getByText('What changed meanwhile')).toBeInTheDocument();
    expect(screen.getByText('Description: "Old" becomes "New".')).toBeInTheDocument();
    expect(screen.getByRole('note')).toHaveTextContent('Both changes touch: Description');
    // The saved version is the one that was fetched, not the one the editor opened with.
    expect(screen.getByText(/# Latest/)).toBeInTheDocument();
    expect(screen.getByText(/# My Draft/)).toBeInTheDocument();

    // Exactly 4 actions in drawer footer:
    expect(screen.getByRole('button', { name: 'Download draft (.md)' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Copy draft' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Discard my edits' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Keep my edits on the latest version' })).toBeInTheDocument();
  });

  it('discard action asks for confirmation before executing', () => {
    const onDiscard = vi.fn();
    render(<ConflictDrawer {...defaultProps} onDiscard={onDiscard} />);

    const discardBtn = screen.getByRole('button', { name: 'Discard my edits' });
    fireEvent.click(discardBtn);

    // Confirmation dialog appears
    expect(screen.getByText('Discard your edits?')).toBeInTheDocument();
    expect(screen.getByText(/Your edits in this browser will be thrown away/)).toBeInTheDocument();
    expect(onDiscard).not.toHaveBeenCalled();

    // Confirm inside dialog
    const confirmBtn = screen.getAllByRole('button', { name: 'Discard my edits' }).at(-1)!;
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
