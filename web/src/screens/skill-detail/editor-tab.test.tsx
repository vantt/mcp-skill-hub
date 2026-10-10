import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../../api/client';
import type { SkillDetail } from '../../api/types';

const previewSkillUpdate = vi.fn();
const apiFetch = vi.fn();

vi.mock('../../api/queries', () => ({
  previewSkillUpdate: (...args: unknown[]) => previewSkillUpdate(...args),
  confirmSkillMutation: vi.fn(),
}));
vi.mock('../../api/client', async (orig) => {
  const actual = await orig<typeof import('../../api/client')>();
  return { ...actual, apiFetch: (...args: unknown[]) => apiFetch(...args) };
});

import { EditorTab } from './EditorTab';

function makeSkill(over: Partial<SkillDetail> = {}): SkillDetail {
  return {
    skill_id: 'demo',
    name: 'Demo',
    description: 'Old words',
    content: '# Demo\n\nOne.',
    content_digest: 'sha256:old',
    lifecycle_state: 'draft',
    path: 'skills/core/demo/SKILL.md',
    routing: { operations: [], triggers: [], not_for: [], min_scope: '' },
    ...over,
  } as SkillDetail;
}

function renderEditor(skill: SkillDetail, focusField?: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <EditorTab skill={skill} workspaceId="ws" focusField={focusField} />
    </QueryClientProvider>,
  );
}

const proposal = {
  schema_version: '1',
  status: 'action_required',
  summary: 'Skill edit is ready for review.',
  diff: { added: null, modified: ['skills/core/demo/SKILL.md'], deleted: null },
  full_diff: '--- a/x\n+++ b/x\n-a\n+b\n',
  confirmation: { confirmation: { pins: { proposal_id: 'P', proposal_digest: 'D', base_version: 'B' } } },
};

describe('EditorTab', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
  });

  it('puts the cursor in the field the Review tab pointed at', () => {
    renderEditor(makeSkill(), 'triggers');
    expect(document.activeElement).toBe(document.getElementById('edit-trigs'));
  });

  it('explains each routing field in one line', () => {
    renderEditor(makeSkill());
    expect(screen.getByText(/Phrases a user might say/)).toBeInTheDocument();
    expect(screen.getByText(/What this skill should not be used for/)).toBeInTheDocument();
    expect(screen.getByText(/How big a job has to be/)).toBeInTheDocument();
  });

  it('keeps Preview changes off until something changed, then says what will change in words', async () => {
    previewSkillUpdate.mockResolvedValue(proposal);
    renderEditor(makeSkill());
    const button = screen.getByRole('button', { name: 'Preview changes' });
    expect(button).toBeDisabled();

    fireEvent.change(document.getElementById('edit-desc')!, { target: { value: 'New words' } });
    expect(button).toBeEnabled();
    fireEvent.click(button);

    expect(await screen.findByText('Save changes to demo?')).toBeInTheDocument();
    expect(screen.getByText('Description: "Old words" becomes "New words".')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Save changes' })).toBeInTheDocument();
  });

  it('shows the latest saved version on a conflict and does not undo the edit made elsewhere', async () => {
    const latest = makeSkill({ description: 'Changed elsewhere', content_digest: 'sha256:new' });
    previewSkillUpdate
      .mockRejectedValueOnce(
        new ApiError(409, { code: 'edit_conflict', render: { ERROR: 'Edit conflict detected.', WHY: 'x', FIX: 'y' } }),
      )
      .mockResolvedValueOnce(proposal);
    apiFetch.mockResolvedValue(latest);

    renderEditor(makeSkill());
    fireEvent.change(document.getElementById('edit-trigs')!, { target: { value: 'ship it' } });
    fireEvent.click(screen.getByRole('button', { name: 'Preview changes' }));

    expect(await screen.findByText('What changed meanwhile')).toBeInTheDocument();
    expect(screen.getByText('Description: "Old words" becomes "Changed elsewhere".')).toBeInTheDocument();
    expect(screen.getByText('Triggers: added "ship it".')).toBeInTheDocument();

    fireEvent.click(screen.getByRole('button', { name: 'Keep my edits on the latest version' }));
    await waitFor(() => expect(previewSkillUpdate).toHaveBeenCalledTimes(2));

    const [, body] = previewSkillUpdate.mock.calls[1]!;
    expect(body.expected_content_digest).toBe('sha256:new');
    // The description was never touched here, so it must not be sent back as the old text.
    expect(body.description).toBeUndefined();
    expect(body.routing.triggers).toEqual(['ship it']);
    expect((document.getElementById('edit-desc') as HTMLTextAreaElement).value).toBe('Changed elsewhere');
  });
});
