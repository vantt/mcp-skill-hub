import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { SessionResponse, SkillDetail, SkillReviewResult, SkillSourcesResult } from '../../api/types';
import { loadGolden } from '../../test/golden';

const previewSkillTransition = vi.fn();
const confirmSkillMutation = vi.fn();

vi.mock('../../api/queries', async (orig) => {
  const actual = await orig<typeof import('../../api/queries')>();
  return {
    ...actual,
    previewSkillTransition: (...args: unknown[]) => previewSkillTransition(...args),
    confirmSkillMutation: (...args: unknown[]) => confirmSkillMutation(...args),
  };
});

import { SkillDetailScreen } from './SkillDetailScreen';

const SKILL_ID = 'review-skill';

const proposal = {
  schema_version: '1',
  status: 'ok',
  summary: 'Deprecate review-skill',
  confirmation: {
    confirmation: {
      required: true,
      pins: { proposal_id: 'PROP-1', proposal_digest: 'sha256:d', base_version: 'v1' },
    },
  },
};

function renderScreen() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  queryClient.setQueryData(['session'], loadGolden<SessionResponse>('session'));
  queryClient.setQueryData(['skill', SKILL_ID], loadGolden<SkillDetail>('skill-detail'));
  queryClient.setQueryData(['skill-review', SKILL_ID], loadGolden<SkillReviewResult>('skill-review'));
  queryClient.setQueryData(['skill-sources', SKILL_ID], loadGolden<SkillSourcesResult>('skill-sources'));
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[`/skills/${SKILL_ID}`]}>
        <Routes>
          <Route path="/skills/:id" element={<SkillDetailScreen />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('SkillDetailScreen lifecycle toast', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    previewSkillTransition.mockResolvedValue(proposal);
    confirmSkillMutation.mockResolvedValue({ operation_id: 'OP-20261014-abc' });
  });

  it('says what the skill is now and shows no operation id', async () => {
    renderScreen();

    fireEvent.click(screen.getByRole('button', { name: 'Deprecate' }));
    fireEvent.click(await screen.findByRole('button', { name: 'Deprecate skill' }));

    expect(await screen.findByText(`${SKILL_ID} is now deprecated.`)).toBeInTheDocument();
    await waitFor(() => expect(confirmSkillMutation).toHaveBeenCalledTimes(1));
    expect(screen.queryByText(/OP-/)).not.toBeInTheDocument();
    expect(screen.queryByText(/Transitioned/)).not.toBeInTheDocument();
  });
});
