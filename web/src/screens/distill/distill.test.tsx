import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { SourceListResult } from '../../api/types';
import { loadGolden } from '../../test/golden';
import { DistillHandoffScreen } from './DistillHandoffScreen';

describe('DistillHandoffScreen', () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    vi.restoreAllMocks();
    window.localStorage.clear();
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    window.localStorage.clear();
  });

  function sourcesWith(
    sources: Array<{ id: string; ready: boolean; skills: string[] }>,
  ): SourceListResult {
    const golden = loadGolden<SourceListResult>('sources');
    return {
      ...golden,
      sources: [],
      groups: [
        {
          repository: 'https://github.com/example/repo',
          sources: sources.map((s) => ({
            id: s.id,
            status: s.ready ? 'changed' : 'watching',
            role: 'learning-source',
            referencing_skills: s.skills,
            skills_vendored_count: 0,
            importable_count: 0,
            ready_to_distill: s.ready,
            current_revision: { kind: 'git-commit', value: 'c2' },
            distilled_revision: { kind: 'git-commit', value: 'c1' },
          })),
        },
      ],
    };
  }

  function renderAt(url: string) {
    return render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[url]}>
          <DistillHandoffScreen />
        </MemoryRouter>
      </QueryClientProvider>,
    );
  }

  it('offers the ready sources to pick when none is selected', async () => {
    queryClient.setQueryData(
      ['sources'],
      sourcesWith([
        { id: 'source-a', ready: true, skills: ['skill-1'] },
        { id: 'source-b', ready: false, skills: ['skill-2'] },
      ]),
    );
    renderAt('/sources/distill');

    expect(screen.getByText('Sources ready to distill')).toBeInTheDocument();
    expect(screen.getByText('source-a')).toBeInTheDocument();
    expect(screen.queryByText('source-b')).toBeNull();
    expect(screen.getByRole('button', { name: 'Continue with selected sources' })).toBeDisabled();

    await userEvent.click(screen.getByRole('checkbox'));
    expect(screen.getByRole('link', { name: 'Continue with selected sources' })).toHaveAttribute(
      'href',
      '/sources/distill?source=source-a',
    );
  });

  it('says why nothing can be picked and links to Sources', () => {
    queryClient.setQueryData(['sources'], sourcesWith([{ id: 'source-b', ready: false, skills: ['skill-2'] }]));
    renderAt('/sources/distill');
    expect(screen.getByText(/No source is ready to distill/)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Open Sources' })).toHaveAttribute('href', '/sources');
  });

  it('explains what a source is when there are none', () => {
    queryClient.setQueryData(['sources'], sourcesWith([]));
    renderAt('/sources/distill');
    expect(screen.getByText(/A source is a repository your skills learn from/)).toBeInTheDocument();
  });

  it('names each skill with its source in the brief', () => {
    queryClient.setQueryData(
      ['sources'],
      sourcesWith([
        { id: 'source-a', ready: true, skills: ['skill-1'] },
        { id: 'source-b', ready: true, skills: ['skill-2'] },
      ]),
    );
    renderAt('/sources/distill?source=source-a&source=source-b');

    expect(screen.getByText('Selected sources (2)')).toBeInTheDocument();
    const pre = screen.getByText(/Use the distill-lab skill/);
    expect(pre.textContent).toContain('- Distill skill-1 from source source-a');
    expect(pre.textContent).toContain('- Distill skill-2 from source source-b');
    expect(pre.textContent).not.toContain('#token=');
    expect(screen.getByRole('link', { name: 'Change selection' })).toHaveAttribute('href', '/sources/distill');
  });

  it('flags a requested source that is not ready', () => {
    queryClient.setQueryData(['sources'], sourcesWith([{ id: 'source-b', ready: false, skills: ['skill-2'] }]));
    renderAt('/sources/distill?source=source-b');
    expect(screen.getByText(/Not ready to distill/)).toBeInTheDocument();
    expect(screen.queryByText(/Use the distill-lab skill/)).toBeNull();
  });
});
