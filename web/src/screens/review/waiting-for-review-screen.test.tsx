import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import type { ReviewQueueItem } from '../../api/types';

let items: ReviewQueueItem[] = [];

vi.mock('../../api/queries', () => ({
  useReviewQueue: () => ({ data: { items }, isLoading: false, error: null }),
}));

import { WaitingForReviewScreen } from './WaitingForReviewScreen';

function renderScreen() {
  return render(
    <MemoryRouter>
      <WaitingForReviewScreen />
    </MemoryRouter>,
  );
}

describe('WaitingForReviewScreen', () => {
  it('lists drafts and changed third-party skills with a link to each full review', () => {
    items = [
      { kind: 'content_changed', skill_id: 'pdf', title: 'PDF', reason: 'content changed since it was approved', since: '2026-10-14', origin: 'github.com/anthropics/skills' },
      { kind: 'draft', skill_id: 'team-release', title: 'Team release', reason: 'new draft', since: '2026-10-14' },
    ];
    renderScreen();
    expect(screen.getByRole('heading', { name: 'Waiting for review (2)' })).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'PDF' })).toHaveAttribute('href', '/skills/pdf/review-content');
    expect(screen.getByText('Content changed · needs approval')).toBeInTheDocument();
    expect(screen.getByText('New draft')).toBeInTheDocument();
    expect(screen.getByText('github.com/anthropics/skills')).toBeInTheDocument();
  });

  it('says so when nothing waits', () => {
    items = [];
    renderScreen();
    expect(screen.getByText('Nothing is waiting for review')).toBeInTheDocument();
  });
});
