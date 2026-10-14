import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { SkillReviewContent } from '../../api/types';

const previewSkillApproval = vi.fn();
const confirmSkillMutation = vi.fn();
let review: SkillReviewContent;

vi.mock('../../api/queries', () => ({
  useSkillReviewContent: () => ({ data: review, isLoading: false, error: null }),
  previewSkillApproval: (...args: unknown[]) => previewSkillApproval(...args),
  confirmSkillMutation: (...args: unknown[]) => confirmSkillMutation(...args),
}));

import { FullReviewScreen } from './FullReviewScreen';

function makeReview(over: Partial<SkillReviewContent> = {}): SkillReviewContent {
  return {
    schema_version: '1',
    status: 'ok',
    summary: '',
    items: [],
    suggested_actions: [],
    warnings: [],
    error: null,
    skill_id: 'pdf',
    state: 'active',
    title: 'PDF',
    license: 'Proprietary',
    origin: 'github.com/anthropics/skills@main',
    third_party: true,
    approved: false,
    content_digest: 'sha256:abc',
    baseline: 'approved',
    baseline_commit: '1234567890abcdef',
    files: [
      { path: 'SKILL.md', size_bytes: 10, binary: false, changed: true, change: 'M', added: 12, removed: 3, diff: '--- a/SKILL.md\n+++ b/SKILL.md\n@@ -1 +1 @@\n-a\n+b\n', truncated: false, content: 'b' },
      { path: 'scripts/fill.py', size_bytes: 10, binary: false, changed: true, change: 'M', added: 2, removed: 2, diff: '--- a/f\n+++ b/f\n@@ -1 +1 @@\n-x\n+y\n', truncated: false, content: 'y' },
      { path: 'references/a.md', size_bytes: 3, binary: false, changed: false, added: 0, removed: 0, truncated: false, content: 'a' },
    ],
    ...over,
  };
}

function renderScreen() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={['/skills/pdf/review-content']}>
        <Routes>
          <Route path="/skills/:id/review-content" element={<FullReviewScreen />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

function open(path: string) {
  const summary = screen.getByText(path).closest('summary')!;
  fireEvent.click(summary);
  // jsdom does not fire the toggle event itself.
  const details = summary.parentElement as HTMLDetailsElement;
  details.open = true;
  fireEvent(details, new Event('toggle'));
}

describe('FullReviewScreen', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    review = makeReview();
  });

  it('shows the license, the origin, what the files are compared with, and the files in order', () => {
    renderScreen();
    expect(screen.getByText('License: Proprietary · Origin: github.com/anthropics/skills@main')).toBeInTheDocument();
    expect(screen.getByText(/Compared with the content you last approved \(commit 1234567\)/)).toBeInTheDocument();
    const changed = screen.getByRole('list', { name: 'Changed files' });
    expect(changed.textContent).toMatch(/SKILL\.md.*scripts\/fill\.py/s);
    expect(screen.getByText(/1 unchanged file/)).toBeInTheDocument();
  });

  it('keeps Approve disabled until every changed file was opened', async () => {
    previewSkillApproval.mockResolvedValue({ confirmation: { confirmation: { pins: { proposal_id: 'P', proposal_digest: 'D', base_version: 'B' } } } });
    confirmSkillMutation.mockResolvedValue({
      commit: { status: 'committed', hash: 'abcdef012345', reach: { this_hub_now: true, push_needed: false } },
    });
    renderScreen();
    const approve = screen.getByRole('button', { name: 'Approve' });
    expect(approve).toBeDisabled();
    expect(screen.getByTestId('approve-hint')).toHaveTextContent('open every changed file first: 2 left');

    open('SKILL.md');
    expect(approve).toBeDisabled();
    expect(screen.getByTestId('approve-hint')).toHaveTextContent('1 left');
    expect(screen.getByText('(opened)')).toBeInTheDocument();

    open('scripts/fill.py');
    expect(approve).toBeEnabled();
    expect(screen.queryByTestId('approve-hint')).not.toBeInTheDocument();

    fireEvent.click(approve);
    await waitFor(() => expect(confirmSkillMutation).toHaveBeenCalled());
    expect(previewSkillApproval).toHaveBeenCalledWith('pdf', 'sha256:abc');
    expect(confirmSkillMutation).toHaveBeenCalledWith('P', { proposalDigest: 'D', baseVersion: 'B' });
    expect(await screen.findByTestId('approve-result')).toHaveTextContent("Approved. Committed abcdef0. This machine's agents get it now.");
  });

  it('has nothing to approve for a skill that is not from another repository', () => {
    review = makeReview({ third_party: false, state: 'draft', baseline: 'none' });
    renderScreen();
    expect(screen.queryByRole('button', { name: 'Approve' })).not.toBeInTheDocument();
    expect(screen.getByText(/nothing to approve/)).toBeInTheDocument();
  });
});
