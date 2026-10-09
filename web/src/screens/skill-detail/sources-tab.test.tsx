import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import * as queries from '../../api/queries';
import type { SkillReviewResult, SkillSourcesResult, UpstreamUpdatePreview } from '../../api/types';
import { loadGolden } from '../../test/golden';
import { LearningSection } from './LearningSection';
import { ProvenanceCard } from './ProvenanceCard';
import { SourcesTab } from './SourcesTab';
import { UpstreamReview } from './UpstreamReview';

describe('ProvenanceCard', () => {
  it('renders flat provenance fields from golden skill-review-third-party.json', () => {
    const golden = loadGolden<SkillReviewResult>('skill-review-third-party');
    const onViewSources = vi.fn();
    render(<ProvenanceCard provenance={golden.provenance} onViewSources={onViewSources} />);

    expect(screen.getByText('Provenance')).toBeInTheDocument();
    expect(screen.getByText('https://github.com/vendor/skills')).toBeInTheDocument();
    expect(screen.getByText('aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa')).toBeInTheDocument();
    expect(screen.getByText('vendor-upstream')).toBeInTheDocument();

    const btn = screen.getByText('View sources →');
    fireEvent.click(btn);
    expect(onViewSources).toHaveBeenCalledTimes(1);
  });

  it('renders Created in this workspace when provenance is absent', () => {
    render(<ProvenanceCard provenance={undefined} onViewSources={vi.fn()} />);
    expect(screen.getByText('Created in this workspace')).toBeInTheDocument();
  });
});

describe('SourcesTab', () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    vi.restoreAllMocks();
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function renderWithProviders(ui: React.ReactElement) {
    return render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>{ui}</MemoryRouter>
      </QueryClientProvider>,
    );
  }

  it('local skill hides Upstream section and shows learning empty state', () => {
    const data: SkillSourcesResult = {
      schema_version: '1',
      status: 'ok',
      summary: 'Sources for local-skill.',
      skill_id: 'local-skill',
      upstream: null,
      learning: [],
      pending_insights: 0,
    };
    queryClient.setQueryData(['skill-sources', 'local-skill'], data);

    renderWithProviders(<SourcesTab skillId="local-skill" />);

    expect(screen.queryByText('Upstream repository')).not.toBeInTheDocument();
    expect(
      screen.getByText('No learning references yet. Link a repository or document whose ideas should improve this skill.'),
    ).toBeInTheDocument();
  });

  it('update_available renders Upstream section and enabled Review update button', () => {
    const data: SkillSourcesResult = {
      schema_version: '1',
      status: 'ok',
      summary: 'Sources for my-skill.',
      skill_id: 'my-skill',
      upstream: {
        skill_id: 'my-skill',
        source_id: 'my-source',
        repository: 'https://github.com/example/skills',
        ref: 'main',
        path: 'skills/my-skill',
        base_commit: '1111222233334444555566667777888899990000',
        latest_commit: '4444555566667777888899990000111122223333',
        latest_committed_at: '2026-10-03T12:00:00Z',
        changed_files: 2,
        files: [
          { path: 'SKILL.md', status: 'modified' },
          { path: 'scripts/run.sh', status: 'added' },
        ],
        local: 'clean',
        status: 'update_available',
        checked_at: '2026-10-04T12:00:00Z',
        next_action: 'skillhub skill update my-skill',
      },
      learning: [],
      pending_insights: 0,
    };
    queryClient.setQueryData(['skill-sources', 'my-skill'], data);

    renderWithProviders(<SourcesTab skillId="my-skill" />);

    expect(screen.getByText('Upstream repository')).toBeInTheDocument();
    expect(screen.getByText('Update available')).toBeInTheDocument();
    const reviewBtn = screen.getByText('Review update');
    expect(reviewBtn).toBeInTheDocument();
    expect(reviewBtn).not.toBeDisabled();
  });
});

describe('UpstreamReview', () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    vi.restoreAllMocks();
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });
  });

  function renderWithProviders(ui: React.ReactElement) {
    return render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>{ui}</MemoryRouter>
      </QueryClientProvider>,
    );
  }

  it('unresolved files disable confirm and show decision banner', async () => {
    const preview: UpstreamUpdatePreview = {
      schema_version: '1',
      status: 'action_required',
      summary: '1 file(s) need a decision.',
      skill_id: 'docx',
      source_id: 'anthropics-skills',
      base_commit: '3f9c2a1',
      target_commit: '81d04be',
      base_available: true,
      unchanged_count: 4,
      unresolved: ['SKILL.md'],
      files: [
        {
          path: 'SKILL.md',
          status: 'both_changed',
          default_action: 'merged',
          action: 'merged',
          conflicts: 1,
          mergeable: false,
          merged_with_markers: 'conflict markers',
        },
      ],
      diff: { added: [], modified: [], deleted: [] },
      trust_impact: {
        third_party: true,
        currently_approved: true,
        review_required_after_apply: true,
        review_command: 'skillhub skill review docx',
      },
      confirmation: {
        confirmation: {
          required: true,
          pins: {
            proposal_id: 'PROP-1',
            proposal_digest: 'sha256:1111111111111111111111111111111111111111111111111111111111111111',
            base_version: 'sha256:2222222222222222222222222222222222222222222222222222222222222222',
          },
        },
      },
    };

    vi.spyOn(queries, 'reviewSkillUpdate').mockResolvedValue(preview);

    renderWithProviders(<UpstreamReview skillId="docx" onClose={vi.fn()} />);

    await waitFor(() => {
      expect(
        screen.getByText('1 file(s) need a decision before this update can be applied.'),
      ).toBeInTheDocument();
    });

    expect(screen.queryByText('Confirm update')).not.toBeInTheDocument();
  });

  it('resolved preview shows trust banner, confirm button, and invalidates queries on confirm', async () => {
    const preview: UpstreamUpdatePreview = {
      schema_version: '1',
      status: 'action_required',
      summary: 'Update is ready to apply.',
      skill_id: 'pdf',
      source_id: 'anthropics-skills',
      base_commit: '3f9c2a1',
      target_commit: '81d04be',
      base_available: true,
      unchanged_count: 2,
      unresolved: [],
      files: [
        {
          path: 'SKILL.md',
          status: 'upstream_only',
          default_action: 'upstream',
          action: 'upstream',
          conflicts: 0,
          mergeable: true,
          result_diff: '--- a/SKILL.md\n+++ b/SKILL.md\n',
        },
      ],
      diff: { added: [], modified: ['SKILL.md'], deleted: [] },
      trust_impact: {
        third_party: true,
        currently_approved: true,
        review_required_after_apply: true,
        review_command: 'skillhub skill review pdf',
      },
      confirmation: {
        confirmation: {
          required: true,
          pins: {
            proposal_id: 'PROP-CLEAN',
            proposal_digest: 'sha256:3333333333333333333333333333333333333333333333333333333333333333',
            base_version: 'sha256:4444444444444444444444444444444444444444444444444444444444444444',
          },
        },
      },
    };

    vi.spyOn(queries, 'reviewSkillUpdate').mockResolvedValue(preview);
    const confirmSpy = vi.spyOn(queries, 'confirmUpstreamUpdate').mockResolvedValue({
      schema_version: '1',
      status: 'ok',
      summary: 'Updated pdf.',
      skill_id: 'pdf',
      operation_id: 'OP-123',
      changed_paths: ['SKILL.md'],
      trust_impact: preview.trust_impact,
    });

    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');
    const onClose = vi.fn();

    renderWithProviders(<UpstreamReview skillId="pdf" onClose={onClose} />);

    await waitFor(() => {
      expect(
        screen.getByText(
          'After applying, agents cannot use pdf until you approve the new content. Run skillhub skill review pdf after applying.',
        ),
      ).toBeInTheDocument();
    });

    const confirmBtn = screen.getByText('Confirm update');
    expect(confirmBtn).not.toBeDisabled();
    fireEvent.click(confirmBtn);

    await waitFor(() => {
      expect(confirmSpy).toHaveBeenCalledWith('PROP-CLEAN', {
        proposal_digest: 'sha256:3333333333333333333333333333333333333333333333333333333333333333',
        base_version: 'sha256:4444444444444444444444444444444444444444444444444444444444444444',
      });
      expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['skill-runtime', 'pdf'] });
      expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['skill-sources', 'pdf'] });
      expect(onClose).toHaveBeenCalled();
    });
  });
});

describe('LearningSection', () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    vi.restoreAllMocks();
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });
  });

  it('allows adding and unlinking learning references', async () => {
    const attachSpy = vi.spyOn(queries, 'previewAttachSource').mockResolvedValue({
      schema_version: '1',
      status: 'ok',
      summary: 'Attached source.',
      confirmation: {
        confirmation: {
          required: true,
          pins: {
            proposal_id: 'PROP-ATT',
            proposal_digest: 'sha256:1111111111111111111111111111111111111111111111111111111111111111',
            base_version: 'sha256:2222222222222222222222222222222222222222222222222222222222222222',
          },
        },
      },
    });
    const detachSpy = vi.spyOn(queries, 'previewDetachSource').mockResolvedValue({
      schema_version: '1',
      status: 'ok',
      summary: 'Detached source.',
      confirmation: {
        confirmation: {
          required: true,
          pins: {
            proposal_id: 'PROP-DET',
            proposal_digest: 'sha256:3333333333333333333333333333333333333333333333333333333333333333',
            base_version: 'sha256:4444444444444444444444444444444444444444444444444444444444444444',
          },
        },
      },
    });
    const confirmSpy = vi.spyOn(queries, 'confirmSourceProposal').mockResolvedValue({
      schema_version: '1',
      status: 'ok',
      summary: 'Confirmed.',
    });

    const initialLearning = [
      {
        source_id: 'src-docs',
        locator: 'https://github.com/example/docs',
        role: 'learning-source',
        monitoring: { enabled: true, cadence: 'weekly' },
        pending_insights: 3,
      },
    ];

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <LearningSection skillId="test-skill" learning={initialLearning} />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    // Assert existing reference and pending insights link
    expect(screen.getByText('src-docs')).toBeInTheDocument();
    expect(screen.getByText('3 pending insight(s)')).toBeInTheDocument();

    // Attach new reference
    const input = screen.getByPlaceholderText('https://github.com/owner/repo');
    const addBtn = screen.getByText('Add learning reference');
    fireEvent.change(input, { target: { value: 'https://github.com/example/new' } });
    fireEvent.click(addBtn);

    await waitFor(() => {
      expect(attachSpy).toHaveBeenCalledWith('test-skill', {
        locator: 'https://github.com/example/new',
      });
      expect(confirmSpy).toHaveBeenCalledWith('PROP-ATT', {
        proposal_digest: 'sha256:1111111111111111111111111111111111111111111111111111111111111111',
        base_version: 'sha256:2222222222222222222222222222222222222222222222222222222222222222',
      });
    });

    // Unlink existing reference
    const unlinkBtn = screen.getByText('Unlink');
    fireEvent.click(unlinkBtn);

    expect(screen.getByText('Unlink src-docs')).toBeInTheDocument();
    const unlinkButtons = screen.getAllByRole('button', { name: 'Unlink' });
    const confirmBtn = unlinkButtons[unlinkButtons.length - 1];
    if (!confirmBtn) throw new Error('confirm button not found');
    fireEvent.click(confirmBtn);

    await waitFor(() => {
      expect(detachSpy).toHaveBeenCalledWith('test-skill', 'src-docs');
    });
  });
});
