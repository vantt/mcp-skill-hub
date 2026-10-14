import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { TreeRow } from '../../state/changeset';
import { FileTree } from './FileTree';

const rows: TreeRow[] = [
  { path: 'scripts/extract.sh', status: '' },
  { path: 'SKILL.md', status: '' },
  { path: 'references/tables.md', status: '' },
  { path: 'references/forms.md', status: 'M' },
  { path: 'references/ocr.md', status: 'A' },
  { path: 'notes.md', status: 'D' },
  { path: 'references/guide.md', status: 'R', from: 'references/old.md' },
];

describe('FileTree', () => {
  it('lists SKILL.md first, then folders with their files nested, then the other top files', () => {
    render(<FileTree rows={rows} selected="SKILL.md" onSelect={() => {}} />);
    const items = screen.getAllByRole('treeitem').map((item) => item.getAttribute('aria-label'));
    expect(items).toEqual([
      'SKILL.md',
      'references/forms.md (modified)',
      'references/guide.md (renamed from references/old.md)',
      'references/ocr.md (added)',
      'references/tables.md',
      'scripts/extract.sh',
      'notes.md (deleted)',
    ]);
    expect(screen.getByText('references')).toBeInTheDocument();
    expect(screen.getByText('scripts')).toBeInTheDocument();
  });

  it('marks the open file and reports a click', () => {
    const onSelect = vi.fn();
    render(<FileTree rows={rows} selected="references/tables.md" onSelect={onSelect} />);
    expect(screen.getByRole('treeitem', { name: 'references/tables.md' })).toHaveAttribute('aria-selected', 'true');
    fireEvent.click(screen.getByRole('treeitem', { name: 'scripts/extract.sh' }));
    expect(onSelect).toHaveBeenCalledWith('scripts/extract.sh');
  });
});
