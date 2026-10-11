import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../../api/client';
import type { SkillDetail, SkillResourceContent } from '../../api/types';
import { loadDraft } from '../../state/drafts';

const useSkillResourceContent = vi.fn();
const previewSkillResourceUpdate = vi.fn();
const confirmSkillMutation = vi.fn();
const apiFetch = vi.fn();

vi.mock('../../api/queries', () => ({
  useSkillResourceContent: (...args: unknown[]) => useSkillResourceContent(...args),
  previewSkillResourceUpdate: (...args: unknown[]) => previewSkillResourceUpdate(...args),
  confirmSkillMutation: (...args: unknown[]) => confirmSkillMutation(...args),
}));
vi.mock('../../api/client', async (orig) => {
  const actual = await orig<typeof import('../../api/client')>();
  return { ...actual, apiFetch: (...args: unknown[]) => apiFetch(...args) };
});

import { ResourceFileView } from './ResourceFileView';

const SCRIPT = 'skills/core/demo/scripts/run.sh';
const DOC = 'skills/core/demo/references/guide.md';
const IMAGE = 'skills/core/demo/assets/logo.png';

const skill = {
  skill_id: 'demo',
  name: 'Demo',
  lifecycle_state: 'active',
  path: 'skills/core/demo/SKILL.md',
  resources: [
    { path: SCRIPT, kind: 'script', digest: 'sha256:s1', size_bytes: 18 },
    { path: DOC, kind: 'reference', digest: 'sha256:d1', size_bytes: 40 },
    { path: IMAGE, kind: 'asset', digest: 'sha256:img', size_bytes: 2048 },
  ],
} as unknown as SkillDetail;

function file(over: Partial<SkillResourceContent> = {}): SkillResourceContent {
  return {
    skill_id: 'demo',
    path: SCRIPT,
    size_bytes: 18,
    digest: 'sha256:s1',
    editable: true,
    reason: '',
    line_ending: 'lf',
    truncated: false,
    content: '#!/bin/sh\necho hi\n',
    ...over,
  };
}

const proposal = {
  schema_version: '1',
  status: 'action_required',
  summary: 'Skill edit is ready for review.',
  diff: { added: null, modified: [SCRIPT], deleted: null },
  full_diff: `--- a/${SCRIPT}\n+++ b/${SCRIPT}\n@@ -1,2 +1,2 @@\n #!/bin/sh\n-echo hi\n+echo bye\n`,
  confirmation: { confirmation: { pins: { proposal_id: 'P', proposal_digest: 'D', base_version: 'B' } } },
};

function renderView(
  opts: { path?: string; startInEdit?: boolean; thirdParty?: boolean; data?: SkillResourceContent } = {},
) {
  useSkillResourceContent.mockReturnValue({ data: opts.data ?? file(), isError: false, error: null });
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const invalidate = vi.spyOn(queryClient, 'invalidateQueries');
  const onClose = vi.fn();
  const view = render(
    <QueryClientProvider client={queryClient}>
      <ResourceFileView
        skill={skill}
        workspaceId="ws"
        path={opts.path ?? SCRIPT}
        startInEdit={opts.startInEdit ?? false}
        thirdParty={opts.thirdParty ?? false}
        onClose={onClose}
      />
    </QueryClientProvider>,
  );
  return { ...view, invalidate, onClose };
}

function editor(name = 'run.sh') {
  return screen.getByRole('textbox', { name: `${name} content` }) as HTMLTextAreaElement;
}

describe('ResourceFileView', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
  });

  it('shows a text file read-only with an Edit button and a way back', () => {
    const { onClose } = renderView();
    expect(screen.getByText(SCRIPT)).toBeInTheDocument();
    expect(screen.getByTestId('resource-source')).toHaveTextContent('echo hi');
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Edit' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Back to resources' }));
    expect(onClose).toHaveBeenCalled();
  });

  it('renders a Markdown file with its frontmatter as a YAML block and offers the source', () => {
    renderView({
      path: DOC,
      data: file({ path: DOC, digest: 'sha256:d1', content: '---\ntitle: Guide\n---\n# Heading\n\nBody.\n' }),
    });
    expect(screen.getByLabelText('Frontmatter')).toHaveTextContent('title: Guide');
    expect(screen.getByRole('heading', { name: 'Heading' })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Source' }));
    expect(screen.getByTestId('resource-source')).toHaveTextContent('# Heading');
  });

  it('explains a binary file without showing its content or an Edit button', () => {
    renderView({
      path: IMAGE,
      data: file({ path: IMAGE, size_bytes: 2048, digest: 'sha256:img', editable: false, reason: 'binary', content: undefined }),
    });
    expect(screen.getByRole('note')).toHaveTextContent(
      'Binary file: its content is not shown. Size 2.0 KiB, digest sha256:img.',
    );
    expect(screen.queryByTestId('resource-source')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Edit' })).not.toBeInTheDocument();
  });

  it('shows the truncated notice for a large text file and offers no Edit button', () => {
    renderView({ data: file({ editable: false, reason: 'too_large', truncated: true, size_bytes: 2_000_000 }) });
    expect(screen.getByRole('note')).toHaveTextContent('Showing the first 1 MiB; this file is too large to edit here.');
    expect(screen.getByTestId('resource-source')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Edit' })).not.toBeInTheDocument();
  });

  it('falls back to read-only with the reason when opened with Edit on a file that cannot be edited', () => {
    renderView({ startInEdit: true, data: file({ editable: false, reason: 'missing', content: undefined, digest: '' }) });
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
    expect(screen.getByRole('note')).toHaveTextContent('This file cannot be edited here.');
    expect(screen.getByRole('note')).toHaveTextContent('skillhub maintain rebuild');
  });

  it('previews the edit, confirms with the pins, clears the draft and refreshes every view of the file', async () => {
    previewSkillResourceUpdate.mockResolvedValue(proposal);
    confirmSkillMutation.mockResolvedValue({ schema_version: '1', status: 'ok', summary: 'Applied.', operation_id: 'op' });
    apiFetch.mockResolvedValue(file({ digest: 'sha256:s2', content: '#!/bin/sh\necho bye\n' }));

    const { invalidate } = renderView({ startInEdit: true });
    const preview = screen.getByRole('button', { name: 'Preview changes' });
    expect(preview).toBeDisabled();

    fireEvent.change(editor(), { target: { value: '#!/bin/sh\necho bye\n' } });
    fireEvent.click(preview);

    expect(await screen.findByText('Save changes to run.sh?')).toBeInTheDocument();
    expect(previewSkillResourceUpdate).toHaveBeenCalledWith('demo', {
      path: SCRIPT,
      content: '#!/bin/sh\necho bye\n',
      expected_digest: 'sha256:s1',
    });
    expect(screen.getByText(`Edited ${SCRIPT}`)).toBeInTheDocument();
    expect(screen.getByText('echo bye')).toBeInTheDocument();
    expect(screen.queryByText(/came from another repository/)).not.toBeInTheDocument();
    expect(confirmSkillMutation).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole('button', { name: 'Save changes' }));
    await waitFor(() =>
      expect(confirmSkillMutation).toHaveBeenCalledWith('P', { proposalId: 'P', proposalDigest: 'D', baseVersion: 'B' }),
    );
    expect(await screen.findByText('Saved run.sh.')).toBeInTheDocument();
    await waitFor(() => expect(screen.getByTestId('resource-source')).toHaveTextContent('echo bye'));
    expect(loadDraft('ws', 'resource', `demo:${SCRIPT}`, 'sha256:s2')).toBeNull();
    const keys = invalidate.mock.calls.map(([arg]) => JSON.stringify((arg as { queryKey: unknown }).queryKey));
    for (const key of [
      ['skill', 'demo'],
      ['skill-review', 'demo'],
      ['skill-resources', 'demo'],
      ['skill-resource', 'demo', SCRIPT],
      ['skills'],
      ['home'],
    ]) {
      expect(keys).toContain(JSON.stringify(key));
    }
  });

  it('warns that approval is needed again only for a skill from another repository', async () => {
    previewSkillResourceUpdate.mockResolvedValue(proposal);
    renderView({ startInEdit: true, thirdParty: true });
    fireEvent.change(editor(), { target: { value: 'changed\n' } });
    fireEvent.click(screen.getByRole('button', { name: 'Preview changes' }));
    expect(await screen.findByText(/came from another repository/)).toBeInTheDocument();
  });

  it('sends nothing when the preview is cancelled', async () => {
    previewSkillResourceUpdate.mockResolvedValue(proposal);
    renderView({ startInEdit: true });
    fireEvent.change(editor(), { target: { value: 'changed\n' } });
    fireEvent.click(screen.getByRole('button', { name: 'Preview changes' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Cancel' }));
    expect(screen.queryByText('Save changes to run.sh?')).not.toBeInTheDocument();
    expect(confirmSkillMutation).not.toHaveBeenCalled();
  });

  it('keeps the draft on Cancel edit and restores it after a remount', async () => {
    vi.useFakeTimers();
    try {
      const first = renderView({ startInEdit: true });
      fireEvent.change(editor(), { target: { value: 'draft text\n' } });
      act(() => {
        vi.advanceTimersByTime(600);
      });
      expect(screen.getByText(/Draft saved in this browser/)).toBeInTheDocument();

      fireEvent.click(screen.getByRole('button', { name: 'Cancel edit' }));
      expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
      fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
      expect(editor().value).toBe('draft text\n');
      first.unmount();

      renderView({ startInEdit: true });
      expect(editor().value).toBe('draft text\n');
    } finally {
      vi.useRealTimers();
    }
  });

  it('opens the conflict drawer when the file changed on disk; keep-mine previews against the new digest', async () => {
    previewSkillResourceUpdate
      .mockRejectedValueOnce(
        new ApiError(409, { code: 'edit_conflict', render: { ERROR: 'Edit conflict detected.', WHY: 'x', FIX: 'y' } }),
      )
      .mockResolvedValueOnce(proposal);
    apiFetch.mockResolvedValue(file({ digest: 'sha256:disk', content: '#!/bin/sh\necho disk\n' }));

    renderView({ startInEdit: true });
    fireEvent.change(editor(), { target: { value: 'mine\n' } });
    fireEvent.click(screen.getByRole('button', { name: 'Preview changes' }));

    expect(await screen.findByText('run.sh changed since you opened it')).toBeInTheDocument();
    expect(screen.getByText('Edited run.sh here')).toBeInTheDocument();
    expect(screen.getByText('run.sh changed on disk')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Keep my edits on the latest version' }));
    await waitFor(() => expect(previewSkillResourceUpdate).toHaveBeenCalledTimes(2));
    expect(previewSkillResourceUpdate.mock.calls[1]![1]).toEqual({
      path: SCRIPT,
      content: 'mine\n',
      expected_digest: 'sha256:disk',
    });
    expect(await screen.findByText('Save changes to run.sh?')).toBeInTheDocument();
  });

  it('discarding on a conflict shows the latest text and drops the draft', async () => {
    previewSkillResourceUpdate.mockRejectedValueOnce(
      new ApiError(409, { code: 'edit_conflict', render: { ERROR: 'Edit conflict detected.', WHY: 'x', FIX: 'y' } }),
    );
    apiFetch.mockResolvedValue(file({ digest: 'sha256:disk', content: '#!/bin/sh\necho disk\n' }));

    renderView({ startInEdit: true });
    fireEvent.change(editor(), { target: { value: 'mine\n' } });
    fireEvent.click(screen.getByRole('button', { name: 'Preview changes' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Discard my edits' }));
    // The drawer asks once more before dropping the edits.
    const discardButtons = await screen.findAllByRole('button', { name: 'Discard my edits' });
    fireEvent.click(discardButtons[discardButtons.length - 1]!);

    await waitFor(() => expect(editor().value).toBe('#!/bin/sh\necho disk\n'));
    expect(screen.queryByText('run.sh changed since you opened it')).not.toBeInTheDocument();
    expect(loadDraft('ws', 'resource', `demo:${SCRIPT}`, 'sha256:disk')).toBeNull();
  });

  it('shows why a stale proposal was refused and keeps the draft', async () => {
    previewSkillResourceUpdate.mockResolvedValue(proposal);
    confirmSkillMutation.mockRejectedValue(
      new ApiError(409, {
        code: 'stale_proposal',
        render: {
          ERROR: 'The proposal can no longer be confirmed.',
          WHY: 'The target or confirmation pins changed after the preview was created.',
          FIX: 'Regenerate the proposal and review the updated diff.',
        },
      }),
    );

    renderView({ startInEdit: true });
    fireEvent.change(editor(), { target: { value: 'mine\n' } });
    fireEvent.click(screen.getByRole('button', { name: 'Preview changes' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Save changes' }));

    expect(await screen.findByRole('alert')).toHaveTextContent(
      'The target or confirmation pins changed after the preview was created.',
    );
    expect(editor().value).toBe('mine\n');
    await waitFor(() =>
      expect(loadDraft<{ content: string }>('ws', 'resource', `demo:${SCRIPT}`, 'sha256:s1')?.value.content).toBe('mine\n'),
    );
  });

  it('compares CRLF files as the text area reports them, so an untouched file has no changes', () => {
    renderView({ startInEdit: true, data: file({ line_ending: 'crlf', content: 'a\r\nb\r\n' }) });
    expect(editor().value).toBe('a\nb\n');
    expect(screen.getByRole('button', { name: 'Preview changes' })).toBeDisabled();
  });
});
