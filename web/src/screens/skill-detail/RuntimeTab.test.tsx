import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { RuntimeTab } from './RuntimeTab';
import { loadGolden } from '../../test/golden';
import type {
  RuntimeHints,
  SkillReviewResult,
  SkillRuntimeStatus,
} from '../../api/types';

function renderRuntimeTab(
  statusData: SkillRuntimeStatus,
  runtimeHints?: RuntimeHints,
) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });

  queryClient.setQueryData(['skill-runtime', statusData.skill_id], statusData);

  return render(
    <QueryClientProvider client={queryClient}>
      <RuntimeTab skillId={statusData.skill_id} runtimeHints={runtimeHints} />
    </QueryClientProvider>,
  );
}

describe('RuntimeTab', () => {
  it('renders third-party runtime block, hints, and env keys from golden', () => {
    const status = loadGolden<SkillRuntimeStatus>('skill-runtime-third-party');
    const review = loadGolden<SkillReviewResult>('skill-review-third-party');
    const hints = review.runtime_hints;

    const { container } = renderRuntimeTab(status, hints);

    // Header verdict
    expect(screen.getByText('Needs review')).toBeInTheDocument();
    expect(screen.getByText('content_review_required')).toBeInTheDocument();

    // Runtime block
    expect(screen.getAllByText('python3').length).toBeGreaterThan(0);
    expect(screen.getByText('VENDOR_TOKEN')).toBeInTheDocument();
    expect(screen.getByText('stored in skill env')).toBeInTheDocument();
    expect(screen.getByText('Values are never shown.')).toBeInTheDocument();

    // Hints
    expect(screen.getByText('Absolute install paths detected')).toBeInTheDocument();
    expect(screen.getByText(/SKILL\.md/)).toBeInTheDocument();
    expect(screen.getByText('Missing lockfiles')).toBeInTheDocument();
    expect(screen.getByText(/package\.json/)).toBeInTheDocument();

    // Sentinel check: never contains any secret sentinel
    expect(container.textContent).not.toContain('SENTINEL');
  });

  it('renders approved third-party skill with doctor evidence and basis terminal', () => {
    const status = loadGolden<SkillRuntimeStatus>('skill-runtime-approved');
    const { container } = renderRuntimeTab(status);

    // Header verdict
    expect(screen.getByText('Ready')).toBeInTheDocument();
    expect(container.textContent).toContain('basis: terminal');

    // Platform checklist row
    expect(screen.getByText('Platform support')).toBeInTheDocument();
    expect(screen.getByText('pass')).toBeInTheDocument();

    // Sentinel check
    expect(container.textContent).not.toContain('SENTINEL');
  });

  it('renders empty state when skill declares no runtime requirements', () => {
    const status = loadGolden<SkillRuntimeStatus>('skill-runtime');
    const { container } = renderRuntimeTab(status);

    expect(
      screen.getByText('This skill declares no runtime requirements.'),
    ).toBeInTheDocument();
    expect(container.textContent).not.toContain('SENTINEL');
  });
});
