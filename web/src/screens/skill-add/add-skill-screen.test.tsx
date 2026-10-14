import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { SkillAddProposal } from '../../api/types';

const previewSkillAdd = vi.fn();

vi.mock('../../api/queries', async (orig) => {
  const actual = await orig<typeof import('../../api/queries')>();
  return { ...actual, previewSkillAdd: (...args: unknown[]) => previewSkillAdd(...args) };
});

import { AddSkillScreen } from './AddSkillScreen';

function proposal(over: Partial<SkillAddProposal> = {}): SkillAddProposal {
  return {
    schema_version: '1',
    status: 'ok',
    summary: 'Add demo-skill',
    skill_id: 'demo-skill',
    collection: 'default',
    diff: { added: ['skills/default/demo-skill/SKILL.md'], modified: [], deleted: [] },
    confirmation: {
      confirmation: { pins: { proposal_id: 'PROP-1', proposal_digest: 'sha256:d', base_version: 'v1' } },
    },
    ...over,
  };
}

async function previewWith(result: SkillAddProposal) {
  previewSkillAdd.mockResolvedValue(result);
  render(
    <QueryClientProvider client={new QueryClient()}>
      <MemoryRouter>
        <AddSkillScreen />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  fireEvent.change(document.getElementById('gh-url') as HTMLInputElement, {
    target: { value: 'https://github.com/acme/skills/tree/main/demo-skill' },
  });
  fireEvent.click(screen.getByRole('button', { name: 'Discover skills' }));
  await screen.findByText('Files that would be written');
}

describe('AddSkillScreen review step', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('lists the files that were left out, with the reason', async () => {
    await previewWith(proposal({ excluded: [{ path: '.env', reason: 'may hold secrets' }] }));

    const leftOut = screen.getByText('Left out (not copied into the hub)').closest('section') as HTMLElement;
    expect(within(leftOut).getByText('.env')).toBeInTheDocument();
    expect(within(leftOut).getByText(/may hold secrets/)).toBeInTheDocument();
  });

  it('shows no left-out list when nothing was left out', async () => {
    await previewWith(proposal());

    expect(screen.queryByText('Left out (not copied into the hub)')).not.toBeInTheDocument();
  });
});

describe('AddSkillScreen collection', () => {
  it('files imported skills under the default collection unless told otherwise', () => {
    render(
      <QueryClientProvider client={new QueryClient()}>
        <MemoryRouter>
          <AddSkillScreen />
        </MemoryRouter>
      </QueryClientProvider>,
    );
    fireEvent.click(screen.getByText('Advanced'));
    expect((document.getElementById('gh-col') as HTMLInputElement).value).toBe('default');
    expect(screen.getByText(/Pick default if unsure/)).toBeInTheDocument();
    expect(screen.queryByText(/Leave it as core/)).not.toBeInTheDocument();
  });
});
