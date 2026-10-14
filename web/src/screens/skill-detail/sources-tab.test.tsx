import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../../api/client';
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

  it('says the skill was made on this machine when provenance is absent', () => {
    render(<ProvenanceCard provenance={undefined} onViewSources={vi.fn()} />);
    expect(
      screen.getByText('Made on this machine (created here or added from a folder); no tracked source.'),
    ).toBeInTheDocument();
    expect(screen.queryByText('Created in this workspace')).not.toBeInTheDocument();
  });
});

describe('SourcesTab', () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    vi.restoreAllMocks();
    // Seeded data stays fresh so no background refetch hits the network mid-test.
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, staleTime: Infinity },
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

  it('local skill shows one empty state with Add source and no upstream or learning card', () => {
    const data: SkillSourcesResult = {
      schema_version: '1',
      status: 'ok',
      summary: 'Sources for local-skill.',
      skill_id: 'local-skill',
      upstream: null,
      learning: [],
    };
    queryClient.setQueryData(['skill-sources', 'local-skill'], data);

    renderWithProviders(<SourcesTab skillId="local-skill" />);

    expect(screen.queryByText('Upstream repository')).not.toBeInTheDocument();
    expect(screen.queryByText('Learning references')).not.toBeInTheDocument();
    expect(screen.getAllByText('No sources yet')).toHaveLength(2);
    expect(screen.getByText(/an upstream source is the repository the skill was copied from/)).toBeInTheDocument();
    expect(screen.getAllByRole('button', { name: 'Add source' })).toHaveLength(2);
  });

  it('Add source opens the add form in place of the empty state', () => {
    queryClient.setQueryData(['skill-sources', 'local-skill'], {
      schema_version: '1',
      status: 'ok',
      summary: 'Sources for local-skill.',
      skill_id: 'local-skill',
      upstream: null,
      learning: [],
    } satisfies SkillSourcesResult);
    queryClient.setQueryData(['sources'], { schema_version: '1', status: 'ok', summary: '', candidates: [], sources: [] });

    renderWithProviders(<SourcesTab skillId="local-skill" />);
    const [barButton] = screen.getAllByRole('button', { name: 'Add source' });
    if (!barButton) throw new Error('Add source button not found');
    fireEvent.click(barButton);

    expect(screen.getByText('Add a learning reference')).toBeInTheDocument();
    expect(screen.queryByText(/an upstream source is the repository/)).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Add source' })).toBeDisabled();
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
    };
    queryClient.setQueryData(['skill-sources', 'my-skill'], data);

    renderWithProviders(<SourcesTab skillId="my-skill" />);

    expect(screen.getByText('Upstream repository')).toBeInTheDocument();
    expect(screen.getByText('Update available')).toBeInTheDocument();
    const reviewBtn = screen.getByText('Review update');
    expect(reviewBtn).toBeInTheDocument();
    expect(reviewBtn).not.toBeDisabled();
  });

  const untrackedData: SkillSourcesResult = {
    schema_version: '1',
    status: 'ok',
    summary: 'Sources for herdr-cook-plan.',
    skill_id: 'herdr-cook-plan',
    upstream: {
      skill_id: 'herdr-cook-plan',
      source_id: '',
      repository: 'https://github.com/thieung/herdr-cook-plan',
      ref: '',
      path: '',
      base_commit: '',
      latest_commit: '',
      changed_files: -1,
      local: '',
      status: 'untracked',
      checked_at: '',
    },
    learning: [],
  };

  const trackPins = {
    proposal_id: 'PROP-TRACK',
    proposal_digest: 'sha256:5555555555555555555555555555555555555555555555555555555555555555',
    base_version: 'sha256:6666666666666666666666666666666666666666666666666666666666666666',
  };

  it('untracked upstream shows Not tracked, disables Check now and Review update, and keeps the fallback command', () => {
    queryClient.setQueryData(['skill-sources', 'herdr-cook-plan'], untrackedData);

    renderWithProviders(<SourcesTab skillId="herdr-cook-plan" />);

    expect(screen.getByText('Upstream repository')).toBeInTheDocument();
    expect(screen.getByText('Not tracked')).toBeInTheDocument();
    expect(screen.getByText('Upstream: thieung/herdr-cook-plan (not tracked)')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Check now' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Review update' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'Track upstream' })).not.toBeDisabled();
    expect(screen.getByText('Or run this in a terminal:')).toBeInTheDocument();
    expect(screen.getByText('skillhub source backfill --skill herdr-cook-plan')).toBeInTheDocument();
    expect(screen.getByText('Learning references')).toBeInTheDocument();
  });

  it('Track upstream previews the full diff and confirms with the pins, then invalidates the skill and source queries', async () => {
    queryClient.setQueryData(['skill-sources', 'herdr-cook-plan'], untrackedData);
    const fullDiff =
      '--- a/skills/default/herdr-cook-plan/.meta/skill.yaml\n+++ b/skills/default/herdr-cook-plan/.meta/skill.yaml\n@@ -1,1 +1,1 @@\n-  - id: ""\n+  - id: thieung-herdr-cook-plan\n';
    const previewSpy = vi.spyOn(queries, 'previewTrackUpstream').mockResolvedValue({
      schema_version: '1',
      status: 'action_required',
      summary: 'Track the upstream of herdr-cook-plan.',
      diff: {
        added: ['sources/catalog/thieung-herdr-cook-plan.yaml'],
        modified: ['skills/default/herdr-cook-plan/.meta/skill.yaml'],
        deleted: [],
      },
      full_diff: fullDiff,
      candidate: {
        skill_id: 'herdr-cook-plan',
        kind: 'origin_without_source',
        source_id: 'thieung-herdr-cook-plan',
        repo_path: '',
        create_source: true,
      },
      confirmation: { confirmation: { required: true, pins: trackPins } },
    });
    const confirmSpy = vi.spyOn(queries, 'confirmSourceProposal').mockResolvedValue({
      schema_version: '1',
      status: 'ok',
      summary: 'Tracked the upstream of herdr-cook-plan.',
    });
    // Record invalidations without refetching, which would need a server.
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries').mockResolvedValue(undefined);

    renderWithProviders(<SourcesTab skillId="herdr-cook-plan" />);
    fireEvent.click(screen.getByRole('button', { name: 'Track upstream' }));

    const dialog = await screen.findByRole('dialog');
    expect(previewSpy).toHaveBeenCalledWith('herdr-cook-plan');
    expect(confirmSpy).not.toHaveBeenCalled();
    expect(within(dialog).getByText('Track upstream for herdr-cook-plan?')).toBeInTheDocument();
    expect(within(dialog).getByText('sources/catalog/thieung-herdr-cook-plan.yaml')).toBeInTheDocument();
    expect(within(dialog).getAllByText('skills/default/herdr-cook-plan/.meta/skill.yaml').length).toBeGreaterThan(0);
    expect(
      within(dialog).getByText("Creates a new source record thieung-herdr-cook-plan and links this skill's upstream to it."),
    ).toBeInTheDocument();
    expect(within(dialog).getAllByText(/id: thieung-herdr-cook-plan/).length).toBeGreaterThan(0);

    fireEvent.click(within(dialog).getByRole('button', { name: 'Track upstream' }));

    await waitFor(() => {
      expect(confirmSpy).toHaveBeenCalledWith('PROP-TRACK', {
        proposal_digest: trackPins.proposal_digest,
        base_version: trackPins.base_version,
      });
    });
    await waitFor(() => {
      for (const queryKey of [
        ['skill-sources', 'herdr-cook-plan'],
        ['skill-review', 'herdr-cook-plan'],
        ['skill', 'herdr-cook-plan'],
        ['skills'],
        ['sources'],
      ]) {
        expect(invalidateSpy).toHaveBeenCalledWith({ queryKey });
      }
    });
    expect(await screen.findByText('Tracked the upstream of herdr-cook-plan.')).toBeInTheDocument();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('Track upstream without pins shows the summary and opens no preview', async () => {
    queryClient.setQueryData(['skill-sources', 'herdr-cook-plan'], untrackedData);
    vi.spyOn(queries, 'previewTrackUpstream').mockResolvedValue({
      schema_version: '1',
      status: 'ok',
      summary: 'Nothing to track: the upstream is already linked.',
      confirmation: { confirmation: { required: false, pins: { proposal_id: '', proposal_digest: '', base_version: '' } } },
    });
    const confirmSpy = vi.spyOn(queries, 'confirmSourceProposal');

    renderWithProviders(<SourcesTab skillId="herdr-cook-plan" />);
    fireEvent.click(screen.getByRole('button', { name: 'Track upstream' }));

    expect(await screen.findByText('Nothing to track: the upstream is already linked.')).toBeInTheDocument();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    expect(confirmSpy).not.toHaveBeenCalled();
  });

  it('Track upstream error shows the reason from the API', async () => {
    queryClient.setQueryData(['skill-sources', 'herdr-cook-plan'], untrackedData);
    vi.spyOn(queries, 'previewTrackUpstream').mockRejectedValue(
      new ApiError(409, {
        code: 'conflict',
        render: { ERROR: 'Cannot track upstream.', WHY: 'The skill has no upstream origin.', FIX: '' },
      }),
    );

    renderWithProviders(<SourcesTab skillId="herdr-cook-plan" />);
    fireEvent.click(screen.getByRole('button', { name: 'Track upstream' }));

    expect(await screen.findByText('The skill has no upstream origin.')).toBeInTheDocument();
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

  it('lists learning references without an inline add form and unlinks through the confirm dialog', async () => {
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
      },
    ];

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter>
          <LearningSection skillId="test-skill" learning={initialLearning} />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(screen.getByText('src-docs')).toBeInTheDocument();
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
    expect(screen.getByText(/Use Add source above to link one/)).toBeInTheDocument();

    fireEvent.click(screen.getByText('Unlink'));

    expect(screen.getByText('Unlink src-docs')).toBeInTheDocument();
    const unlinkButtons = screen.getAllByRole('button', { name: 'Unlink' });
    const confirmBtn = unlinkButtons[unlinkButtons.length - 1];
    if (!confirmBtn) throw new Error('confirm button not found');
    fireEvent.click(confirmBtn);

    await waitFor(() => {
      expect(detachSpy).toHaveBeenCalledWith('test-skill', 'src-docs');
      expect(confirmSpy).toHaveBeenCalledWith('PROP-DET', {
        proposal_digest: 'sha256:3333333333333333333333333333333333333333333333333333333333333333',
        base_version: 'sha256:4444444444444444444444444444444444444444444444444444444444444444',
      });
    });
  });
});
