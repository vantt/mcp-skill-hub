import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { ContentTrustCard } from './ContentTrustCard';
import { loadGolden } from '../../test/golden';
import type { ContentTrust, SkillReviewResult } from '../../api/types';

describe('ContentTrustCard', () => {
  it('renders unapproved third-party skill from golden with no approve button', () => {
    const review = loadGolden<SkillReviewResult>('skill-review-third-party');
    expect(review.content_trust).toBeDefined();

    const { container } = render(<ContentTrustCard trust={review.content_trust!} />);

    // Shows Review required badge
    expect(screen.getByText('Review required')).toBeInTheDocument();

    // Shows digest
    expect(screen.getByText(review.content_trust!.content_digest)).toBeInTheDocument();

    // Shows approve command text
    expect(screen.getByText(review.content_trust!.approve_command!)).toBeInTheDocument();
    expect(
      screen.getByText(
        'Approval is CLI-only. Review the content, then run this command in your terminal. The WebUI and agents cannot approve content.',
      ),
    ).toBeInTheDocument();

    // CRITICAL: No approve button and no form element
    const approveButtons = screen.queryAllByRole('button', { name: /approve/i });
    expect(approveButtons).toHaveLength(0);
    expect(container.querySelectorAll('form')).toHaveLength(0);
  });

  it('renders changes since approval when modified', () => {
    const trust: ContentTrust = {
      third_party: true,
      approved: false,
      content_digest: 'sha256:1111222233334444555566667777888899990000aaaabbbbccccddddeeeeffff',
      changes_since_approval: {
        found: true,
        scripts_changed: true,
        runtime_changed: true,
        dependencies_changed: false,
        modified: ['scripts/run.py'],
        diff_command: 'git diff HEAD~1..HEAD',
      },
    };

    render(<ContentTrustCard trust={trust} />);

    expect(screen.getByText('Changed since approval')).toBeInTheDocument();
    expect(screen.getByText('scripts changed')).toBeInTheDocument();
    expect(screen.getByText('runtime changed')).toBeInTheDocument();
    expect(screen.queryByText('dependencies changed')).not.toBeInTheDocument();
    expect(screen.getByText('scripts/run.py')).toBeInTheDocument();
    expect(screen.getByText('git diff HEAD~1..HEAD')).toBeInTheDocument();
  });

  it('shows truncation note when history is truncated', () => {
    const trust: ContentTrust = {
      third_party: true,
      approved: false,
      content_digest: 'sha256:digest-trunc',
      changes_since_approval: {
        found: true,
        history_truncated: true,
      },
    };

    render(<ContentTrustCard trust={trust} />);
    expect(
      screen.getByText('Commit history was truncated during diff walk.'),
    ).toBeInTheDocument();
  });

  it('renders nothing for local skills', () => {
    const review = loadGolden<SkillReviewResult>('skill-review');
    // Local skill has content_trust?.third_party === false (or undefined)
    const trust: ContentTrust = review.content_trust || {
      third_party: false,
      approved: true,
      content_digest: 'sha256:local',
    };

    const { container } = render(<ContentTrustCard trust={trust} />);
    expect(container).toBeEmptyDOMElement();
  });
});
