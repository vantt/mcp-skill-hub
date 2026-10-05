import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { ContentTrustCard } from './ContentTrustCard';
import { loadGolden } from '../../test/golden';
import type { ContentTrust, SkillReviewResult } from '../../api/types';

describe('ContentTrustCard', () => {
  it('renders review required state without approve buttons from golden fixture', () => {
    const review = loadGolden<SkillReviewResult>('skill-review-third-party');
    expect(review.content_trust).toBeDefined();
    const trust = review.content_trust!;

    const { container } = render(<ContentTrustCard trust={trust} />);

    expect(screen.getByText('Review required')).toBeInTheDocument();
    expect(screen.getByText(trust.content_digest)).toBeInTheDocument();
    if (trust.approve_command) {
      expect(screen.getByText(trust.approve_command)).toBeInTheDocument();
    }
    expect(
      screen.getByText(
        'Approval is CLI-only. Review the content, then run this command in your terminal. The WebUI and agents cannot approve content.'
      )
    ).toBeInTheDocument();
    expect(
      screen.getByText('Agents get no content and no files from this skill until it is approved.')
    ).toBeInTheDocument();

    // Verify security requirement: no approve button and no form element
    const approveButtons = screen.queryAllByRole('button', { name: /approve/i });
    expect(approveButtons).toHaveLength(0);
    expect(container.querySelector('form')).toBeNull();
  });

  it('renders changed since approval with modified paths and diff command', () => {
    const trust: ContentTrust = {
      third_party: true,
      approved: false,
      content_digest: 'sha256:1122334455667788',
      approve_command: 'skillhub skill edit vendor-skill --approve-content sha256:1122334455667788',
      changes_since_approval: {
        found: true,
        scripts_changed: true,
        runtime_changed: true,
        dependencies_changed: false,
        modified: ['scripts/run.py'],
        diff_command: 'git diff sha256:old..HEAD -- skills/core/vendor-skill',
        history_truncated: false,
      },
    };

    render(<ContentTrustCard trust={trust} />);

    expect(screen.getByText('Changed since approval')).toBeInTheDocument();
    expect(screen.getByText('scripts changed')).toBeInTheDocument();
    expect(screen.getByText('runtime changed')).toBeInTheDocument();
    expect(screen.queryByText('dependencies changed')).toBeNull();
    expect(screen.getByText('Modified: scripts/run.py')).toBeInTheDocument();
    expect(
      screen.getByText('git diff sha256:old..HEAD -- skills/core/vendor-skill')
    ).toBeInTheDocument();
  });

  it('renders history truncation note when history_truncated is true', () => {
    const trust: ContentTrust = {
      third_party: true,
      approved: false,
      content_digest: 'sha256:1122334455667788',
      changes_since_approval: {
        found: true,
        scripts_changed: false,
        runtime_changed: false,
        dependencies_changed: false,
        modified: ['file.txt'],
        history_truncated: true,
      },
    };

    render(<ContentTrustCard trust={trust} />);

    expect(
      screen.getByText('History was truncated; older commits are not shown.')
    ).toBeInTheDocument();
  });

  it('renders approved state for approved third-party skill', () => {
    const trust: ContentTrust = {
      third_party: true,
      approved: true,
      content_digest: 'sha256:approveddigest12345678',
    };

    render(<ContentTrustCard trust={trust} />);

    expect(screen.getByText('Approved')).toBeInTheDocument();
    expect(screen.getByText("Agents receive this skill's content.")).toBeInTheDocument();
  });
});
