import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type {
  InsightApplicationPreview,
  InsightApplicationResult,
  InsightDetailResult,
  SessionResponse,
  SkillDetail,
} from '../../api/types';
import { loadGolden } from '../../test/golden';
import { PatchComposerScreen } from './PatchComposerScreen';

describe('PatchComposerScreen', () => {
  let queryClient: QueryClient;
  const goldenInsight = loadGolden<InsightDetailResult>('insight-detail');
  const goldenSkill = loadGolden<SkillDetail>('skill-detail');
  const goldenSession = loadGolden<SessionResponse>('session');
  const insId = goldenInsight.insight.id;

  beforeEach(() => {
    vi.restoreAllMocks();
    window.localStorage.clear();
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });
    queryClient.setQueryData(['session'], goldenSession);
    queryClient.setQueryData(['insight', insId], goldenInsight);
    queryClient.setQueryData(['skill', goldenInsight.insight.skill_id], goldenSkill);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    window.localStorage.clear();
  });

  function renderWithRoute() {
    return render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[`/inbox/${insId}/apply`]}>
          <Routes>
            <Route path="/inbox/:id/apply" element={<PatchComposerScreen />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );
  }

  it('preview is disabled with reasons when content is unchanged or coverage is incomplete', () => {
    renderWithRoute();

    const previewBtn = screen.getByRole('button', { name: 'Preview apply' });
    expect(previewBtn).toBeDisabled();
    expect(previewBtn).toHaveAttribute(
      'title',
      'Change the skill content before previewing.',
    );

    // Edit content so it differs
    const textarea = screen.getByLabelText('SKILL.md replacement content');
    fireEvent.change(textarea, {
      target: { value: goldenSkill.content + '\n\n## Retry Logic\nRetry safely.\n' },
    });

    // Still disabled because observation is unmapped
    expect(previewBtn).toBeDisabled();
    expect(previewBtn).toHaveAttribute(
      'title',
      'Map all required observations before previewing.',
    );

    // Map the observation
    const conceptInput = screen.getByPlaceholderText('Concept, e.g. retry limits');
    fireEvent.change(conceptInput, { target: { value: 'retry-logic' } });
    const addBtn = screen.getByRole('button', { name: '+ Add concept' });
    fireEvent.click(addBtn);

    // Now preview is enabled!
    expect(previewBtn).not.toBeDisabled();
  });

  it('opens ConflictDrawer when skill digest mismatches baseline on preview', async () => {
    // Mock fetch for refetchSkill returning updated digest
    vi.spyOn(global, 'fetch').mockImplementation(async (input) => {
      const url = String(input);
      if (url.includes(`/api/v1/skills/${goldenInsight.insight.skill_id}`)) {
        return new Response(
          JSON.stringify({
            ...goldenSkill,
            content_digest: 'sha256:concurrently-modified-digest',
            content: goldenSkill.content + '\n# Modified outside\n',
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        );
      }
      if (url.includes('/api/v1/insights')) {
        return new Response(JSON.stringify(goldenInsight), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.includes('/api/v1/session')) {
        return new Response(JSON.stringify(goldenSession), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      return new Response(JSON.stringify({}), { status: 200 });
    });

    renderWithRoute();

    // Edit content and add concept
    const textarea = screen.getByLabelText('SKILL.md replacement content');
    fireEvent.change(textarea, {
      target: { value: goldenSkill.content + '\n\n## Retry Logic\nRetry safely.\n' },
    });
    const conceptInput = screen.getByPlaceholderText('Concept, e.g. retry limits');
    fireEvent.change(conceptInput, { target: { value: 'retry-logic' } });
    const addBtn = screen.getByRole('button', { name: '+ Add concept' });
    fireEvent.click(addBtn);

    const previewBtn = screen.getByRole('button', { name: 'Preview apply' });
    fireEvent.click(previewBtn);

    // ConflictDrawer opens
    expect(await screen.findByText('SKILL.md changed since you opened it')).toBeInTheDocument();
  });

  it('previews proposal and confirms patch application, showing receipt and invalidating queries', async () => {
    let previewCalled = false;
    let confirmCalled = false;

    const mockPreview: InsightApplicationPreview = {
      schema_version: '1',
      status: 'action_required',
      summary: 'Preview generated',
      proposal_id: 'PROP-INSIGHT-1',
      proposal_digest: 'sha256:prop-digest-1',
      base_catalog_version: 'base-version-1',
      insight_id: insId,
      skill_id: goldenInsight.insight.skill_id,
      path_pins: [],
      diff: '+## Retry Logic\n+Retry safely.\n',
      confirmation: {
        confirmation: {
          required: true,
          pins: {
            proposal_id: 'PROP-INSIGHT-1',
            proposal_digest: 'sha256:prop-digest-1',
            base_version: 'base-version-1',
          },
        },
      },
    };

    const mockResult: InsightApplicationResult = {
      schema_version: '1',
      status: 'applied',
      summary: 'Applied successfully',
      operation_id: 'OP-APPLY-123',
    };

    vi.spyOn(global, 'fetch').mockImplementation(async (input) => {
      const url = String(input);
      if (url.includes('/apply/preview')) {
        previewCalled = true;
        return new Response(JSON.stringify(mockPreview), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.includes('/apply/confirm')) {
        confirmCalled = true;
        return new Response(JSON.stringify(mockResult), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.includes(`/api/v1/skills/${goldenInsight.insight.skill_id}`)) {
        return new Response(JSON.stringify(goldenSkill), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.includes('/api/v1/insights')) {
        return new Response(JSON.stringify(goldenInsight), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      if (url.includes('/api/v1/session')) {
        return new Response(JSON.stringify(goldenSession), {
          status: 200,
          headers: { 'Content-Type': 'application/json' },
        });
      }
      return new Response(JSON.stringify({}), { status: 200 });
    });

    renderWithRoute();

    // Edit content and add concept
    const textarea = screen.getByLabelText('SKILL.md replacement content');
    fireEvent.change(textarea, {
      target: { value: goldenSkill.content + '\n\n## Retry Logic\nRetry safely.\n' },
    });
    const conceptInput = screen.getByPlaceholderText('Concept, e.g. retry limits');
    fireEvent.change(conceptInput, { target: { value: 'retry-logic' } });
    const addBtn = screen.getByRole('button', { name: '+ Add concept' });
    fireEvent.click(addBtn);

    const previewBtn = screen.getByRole('button', { name: 'Preview apply' });
    fireEvent.click(previewBtn);

    // ProposalPreview opens
    expect(await screen.findByRole('dialog')).toBeInTheDocument();
    expect(previewCalled).toBe(true);

    const applyConfirmBtn = screen.getByRole('button', { name: 'Apply patch' });
    fireEvent.click(applyConfirmBtn);

    // Receipt card displayed
    expect(await screen.findByText('Insight patch applied successfully!')).toBeInTheDocument();
    expect(confirmCalled).toBe(true);
    expect(screen.getByRole('link', { name: 'View Skill Review →' })).toBeInTheDocument();
  });
});
