import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../../api/client';
import type { SkillDetail, SkillFileEntry } from '../../api/types';

const previewSkillChanges = vi.fn();
const confirmSkillMutation = vi.fn();

function entry(path: string, text = true): SkillFileEntry {
  return { path, size_bytes: 12, digest: `sha256:${path}`, text, editable: text, executable: false };
}

const FILES = {
  base_version: 'sha256:base',
  files: [entry('SKILL.md'), entry('references/forms.md'), entry('references/tables.md'), entry('assets/logo.png', false)],
};

vi.mock('../../api/queries', () => ({
  useSkillFiles: () => ({ data: FILES, isError: false, refetch: () => Promise.resolve({ data: FILES }) }),
  useSkillFileContent: (_id: string, path: string) => ({
    data: { path, text: path !== 'assets/logo.png', content: `text of ${path}` },
    isPending: false,
  }),
  previewSkillChanges: (...args: unknown[]) => previewSkillChanges(...args),
  confirmSkillMutation: (...args: unknown[]) => confirmSkillMutation(...args),
}));

import { FilesTab } from './FilesTab';

const skill = { skill_id: 'pdf-tools', name: 'PDF tools' } as SkillDetail;

function renderFiles(url = '/skills/pdf-tools?tab=files') {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[url]}>
        <FilesTab skill={skill} workspaceId="ws" />
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

const preview = {
  schema_version: '1',
  status: 'action_required',
  summary: 'ready',
  base_version: 'sha256:base',
  default_message: 'Commit pdf-tools: 2 files',
  routing_lint: [],
  files: [
    { path: 'references/forms.md', change: 'M', added: 1, removed: 1, size: 5, binary: false, diff: '--- a/x\n+++ b/x\n@@ -1 +1 @@\n-old\n+new\n' },
    { path: 'references/ocr.md', change: 'A', added: 1, removed: 0, size: 5, binary: false, diff: '--- a/y\n+++ b/y\n@@ -0,0 +1 @@\n+ocr\n' },
  ],
  confirmation: { confirmation: { pins: { proposal_id: 'P', proposal_digest: 'D', base_version: 'B' } } },
};

describe('FilesTab', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
  });

  it('shows the tree with SKILL.md first and opens a file to read', async () => {
    renderFiles();
    const items = await screen.findAllByRole('treeitem');
    expect(items[0]).toHaveAccessibleName('SKILL.md');
    fireEvent.click(screen.getByRole('treeitem', { name: 'references/forms.md' }));
    expect(await screen.findByText('text of references/forms.md')).toBeInTheDocument();
    expect(screen.queryByText(/not committed/)).not.toBeInTheDocument();
  });

  it('counts pending changes in the bar, and Discard clears them', async () => {
    renderFiles();
    fireEvent.click(await screen.findByRole('treeitem', { name: 'references/forms.md' }));
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    fireEvent.change(screen.getByRole('textbox', { name: 'Edit references/forms.md' }), { target: { value: 'new words' } });
    expect(screen.getByText('1 change not committed')).toBeInTheDocument();
    expect(screen.getByRole('treeitem', { name: 'references/forms.md (modified)' })).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: '+ File' }));
    fireEvent.change(screen.getByLabelText(/Path of the new file/), { target: { value: 'references/ocr.md' } });
    fireEvent.click(screen.getByRole('button', { name: 'Add' }));
    expect(screen.getByText('2 changes not committed')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Discard' }));
    fireEvent.click(screen.getByRole('button', { name: 'Discard changes' }));
    await waitFor(() => expect(screen.queryByText(/not committed/)).not.toBeInTheDocument());
    expect(screen.queryByRole('treeitem', { name: /ocr.md/ })).not.toBeInTheDocument();
  });

  it('renames and deletes a file, and Restore puts a deleted file back', async () => {
    renderFiles();
    fireEvent.click(await screen.findByRole('treeitem', { name: 'references/tables.md' }));
    fireEvent.click(screen.getByRole('button', { name: 'Rename' }));
    fireEvent.change(screen.getByLabelText('New path'), { target: { value: 'references/data-tables.md' } });
    fireEvent.click(screen.getAllByRole('button', { name: 'Rename' }).at(-1)!);
    expect(screen.getByRole('treeitem', { name: 'references/data-tables.md (renamed from references/tables.md)' })).toBeInTheDocument();

    fireEvent.click(screen.getByRole('treeitem', { name: 'references/forms.md' }));
    fireEvent.click(screen.getByRole('button', { name: 'Delete' }));
    expect(screen.getByRole('treeitem', { name: 'references/forms.md (deleted)' })).toBeInTheDocument();
    expect(screen.getByText('2 changes not committed')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Restore' }));
    expect(screen.getByText('1 change not committed')).toBeInTheDocument();
  });

  it('refuses a name that is already used', async () => {
    renderFiles();
    fireEvent.click(await screen.findByRole('button', { name: '+ File' }));
    fireEvent.change(screen.getByLabelText(/Path of the new file/), { target: { value: 'references/forms.md' } });
    fireEvent.click(screen.getByRole('button', { name: 'Add' }));
    expect(screen.getByRole('alert')).toHaveTextContent('references/forms.md is already in this skill.');
  });

  it('keeps pending changes after a reload of the tab', async () => {
    const first = renderFiles();
    fireEvent.click(await screen.findByRole('button', { name: '+ File' }));
    fireEvent.change(screen.getByLabelText(/Path of the new file/), { target: { value: 'notes.md' } });
    fireEvent.click(screen.getByRole('button', { name: 'Add' }));
    await waitFor(() => expect(localStorage.length).toBeGreaterThan(0), { timeout: 2000 });
    first.unmount();
    renderFiles();
    expect(await screen.findByText('1 change not committed')).toBeInTheDocument();
    expect(screen.getByRole('treeitem', { name: 'notes.md (added)' })).toBeInTheDocument();
  });

  it('reviews the changes, commits them with the typed message, and says who gets the commit', async () => {
    previewSkillChanges.mockResolvedValue(preview);
    confirmSkillMutation.mockResolvedValue({
      status: 'applied',
      commit: { status: 'committed', hash: '4be19c0ffee', reach: { this_hub_now: true, push_needed: true, push_command: 'git -C ~/skill-hub push' } },
    });
    renderFiles();
    fireEvent.click(await screen.findByRole('treeitem', { name: 'references/forms.md' }));
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    fireEvent.change(screen.getByRole('textbox', { name: 'Edit references/forms.md' }), { target: { value: 'new' } });
    fireEvent.click(screen.getByRole('button', { name: /Review/ }));

    expect(await screen.findByText('Review changes')).toBeInTheDocument();
    expect(previewSkillChanges).toHaveBeenCalledWith('pdf-tools', {
      base_version: 'sha256:base',
      changes: [{ action: 'modify', path: 'references/forms.md', content: 'new', encoding: 'utf8', expected_digest: 'sha256:references/forms.md' }],
    });
    expect(await screen.findByText('Routing lint: no findings.')).toBeInTheDocument();
    expect(screen.getByLabelText('Commit message')).toHaveValue('Commit pdf-tools: 2 files');

    fireEvent.change(screen.getByLabelText('Commit message'), { target: { value: 'Tidy forms' } });
    fireEvent.click(screen.getByRole('button', { name: 'Commit to the hub' }));

    expect(await screen.findByTestId('commit-result')).toHaveTextContent(
      "Committed 4be19c0. This machine's agents get it now; others after you push (git -C ~/skill-hub push).",
    );
    expect(confirmSkillMutation).toHaveBeenCalledWith('P', { proposalDigest: 'D', baseVersion: 'B' }, 'Tidy forms');
    // The changes are committed, so none are pending any more.
    fireEvent.click(screen.getByRole('button', { name: 'Back to files' }));
    expect(screen.queryByText(/not committed/)).not.toBeInTheDocument();
  });

  async function reviewOneEdit() {
    previewSkillChanges.mockResolvedValue(preview);
    renderFiles();
    fireEvent.click(await screen.findByRole('treeitem', { name: 'references/forms.md' }));
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    fireEvent.change(screen.getByRole('textbox', { name: 'Edit references/forms.md' }), { target: { value: 'new' } });
    fireEvent.click(screen.getByRole('button', { name: /Review/ }));
    fireEvent.click(await screen.findByRole('button', { name: 'Commit to the hub' }));
  }

  it('says why a commit over files with uncommitted edits was refused, keeps the changes, and opens no drawer', async () => {
    confirmSkillMutation.mockRejectedValue(
      new ApiError(409, {
        code: 'edit_conflict',
        render: { ERROR: 'SKILL.md has edits that were not committed.', WHY: 'These files already differ from Git.', FIX: 'Commit them first.' },
      }),
    );
    await reviewOneEdit();
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'SKILL.md has edits that were not committed. These files already differ from Git. Commit them first.',
    );
    expect(screen.queryByText('The hub changed after you started these changes.')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Back to files' }));
    expect(screen.getByText('1 change not committed')).toBeInTheDocument();
  });

  it('opens the conflict drawer when the hub changed between the preview and the commit', async () => {
    confirmSkillMutation.mockRejectedValue(
      new ApiError(409, { code: 'stale_proposal', render: { ERROR: 'The proposal can no longer be confirmed.', WHY: 'x', FIX: 'y' } }),
    );
    await reviewOneEdit();
    expect(await screen.findByText('The hub changed after you started these changes.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Keep my edits on the latest version' })).toBeInTheDocument();
  });
});
