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

  it('names a companion file instead of SKILL.md and downloads the draft under its name', () => {
    const downloads: Array<{ name: string; type: string }> = [];
    let lastBlob: Blob | null = null;
    const originalCreate = URL.createObjectURL;
    const originalRevoke = URL.revokeObjectURL;
    URL.createObjectURL = (obj: Blob | MediaSource) => {
      lastBlob = obj as Blob;
      return 'blob:draft';
    };
    URL.revokeObjectURL = () => {};
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
      downloads.push({ name: this.download, type: lastBlob?.type ?? '' });
    });

    try {
      render(<ConflictDrawer {...defaultProps} fileLabel="skills/core/foo/scripts/run.sh" />);

      expect(
        screen.getByText('skills/core/foo/scripts/run.sh changed since you opened it'),
      ).toBeInTheDocument();
      expect(screen.getByText('Compare the full skills/core/foo/scripts/run.sh')).toBeInTheDocument();
      expect(screen.queryByText(/SKILL\.md/)).not.toBeInTheDocument();

      fireEvent.click(screen.getByRole('button', { name: 'Download draft' }));
      expect(downloads).toEqual([{ name: 'draft-run.sh', type: 'text/plain;charset=utf-8' }]);
    } finally {
      URL.createObjectURL = originalCreate;
      URL.revokeObjectURL = originalRevoke;
    }
  });

  it('closes on Escape', () => {
    const onClose = vi.fn();
    render(<ConflictDrawer {...defaultProps} onClose={onClose} />);

    fireEvent.keyDown(document, { key: 'Escape' });
    expect(onClose).toHaveBeenCalled();
  });
});
