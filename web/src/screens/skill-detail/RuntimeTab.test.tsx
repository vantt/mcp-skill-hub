import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { RuntimeTab } from './RuntimeTab';
import { loadGolden } from '../../test/golden';
import type { RuntimeHints, SkillRuntimeStatus } from '../../api/types';

function renderRuntimeTab(statusData?: SkillRuntimeStatus, skillId = 'review-skill', hints?: RuntimeHints) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });

  if (statusData) {
    queryClient.setQueryData(['skill-runtime', skillId], statusData);
  }

  return {
    queryClient,
    ...render(
      <QueryClientProvider client={queryClient}>
        <RuntimeTab skillId={skillId} runtimeHints={hints} />
      </QueryClientProvider>,
    ),
  };
}

describe('RuntimeTab', () => {
  const sentinel = 'SENTINEL-TEST-SECRET';

  it('renders third-party unapproved runtime with hints and env keys', () => {
    const status = loadGolden<SkillRuntimeStatus>('skill-runtime-third-party');
    const hints: RuntimeHints = {
      interpreters: ['python3'],
      dependency_manifests: ['package.json'],
      missing_lockfiles: ['package.json (package-lock.json)'],
      absolute_install_paths: ['~/.claude/skills/target'],
      missing_runtime_block: false,
      install_prose_detected: true,
      install_cues: ['pip install'],
    };

    const { container } = renderRuntimeTab(status, 'vendor-skill', hints);

    expect(screen.getByText('Runtime readiness')).toBeInTheDocument();
    expect(screen.getByText('Needs review')).toBeInTheDocument();
    expect(screen.getByText('VENDOR_TOKEN')).toBeInTheDocument();
    expect(screen.getByText('python3')).toBeInTheDocument();
    expect(screen.getByText('Values are never shown.')).toBeInTheDocument();

    // Authoring warnings
    expect(screen.getByText(/Absolute install paths/)).toBeInTheDocument();
    expect(screen.getByText(/Missing lockfiles/)).toBeInTheDocument();

    // Verify secret sentinel is never rendered
    expect(container.textContent).not.toContain(sentinel);
  });

  it('renders approved skill with doctor evidence and terminal basis', () => {
    const status = loadGolden<SkillRuntimeStatus>('skill-runtime-approved');
    const { container } = renderRuntimeTab(status, 'approved-skill');

    expect(screen.getByText('Runtime readiness')).toBeInTheDocument();
    expect(screen.getByText('Ready')).toBeInTheDocument();
    expect(screen.getByText(/basis: terminal/)).toBeInTheDocument();
    expect(screen.getByText('Supported platforms')).toBeInTheDocument();

    expect(screen.getByText('Re-check from your terminal:')).toBeInTheDocument();
    expect(screen.getByText('skillhub skill doctor approved-skill')).toBeInTheDocument();

    expect(container.textContent).not.toContain(sentinel);
  });

  it('renders empty state when skill declares no runtime requirements', () => {
    const status = loadGolden<SkillRuntimeStatus>('skill-runtime');
    const { container } = renderRuntimeTab(status, 'review-skill');

    expect(screen.getByText('This skill declares no runtime requirements.')).toBeInTheDocument();
    expect(container.textContent).not.toContain(sentinel);
  });
});
