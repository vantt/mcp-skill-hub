import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { loadGolden } from '../../test/golden';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { SourceListResult } from '../../api/types';
import { SourcesScreen } from './SourcesScreen';

describe('SourcesScreen', () => {
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

  it('renders empty state text when there are no sources', () => {
    const empty: SourceListResult = {
      schema_version: '1',
      status: 'ok',
      summary: '0 source(s).',
      candidates: [],
      sources: [],
      groups: [],
    };
    queryClient.setQueryData(['sources'], empty);

    renderWithProviders(<SourcesScreen />);

    expect(
      screen.getByText('No sources yet. Link a repository whose ideas should improve your skills.'),
    ).toBeInTheDocument();
  });

  it('two groups render repository headings and orphan chip', () => {
    const data: SourceListResult = {
      schema_version: '1',
      status: 'ok',
      summary: '2 source(s) from 2 repositories.',
      candidates: [],
      sources: [
        {
          record: {
            id: 'src-1',
            adapter: 'git',
            locator: { repository: 'https://github.com/example/repo-one', ref: 'main' },
            status: 'watching',
            identity: { name: 'src-1' },
            monitoring: { enabled: true, cadence: 'weekly' },
          },
          skills: ['pdf'],
          role: 'upstream',
          importable_count: 0,
        },
        {
          record: {
            id: 'src-2',
            adapter: 'git',
            locator: { repository: 'https://github.com/example/repo-two', ref: 'main' },
            status: 'watching',
            identity: { name: 'src-2' },
            monitoring: { enabled: true, cadence: 'weekly' },
          },
          skills: [],
          role: 'unattached',
          importable_count: 0,
        },
      ],
      groups: [
        {
          repository: 'https://github.com/example/repo-one',
          sources: [
            {
              id: 'src-1',
              status: 'watching',
              role: 'upstream',
              referencing_skills: ['pdf'],
              skills_vendored_count: 1,
              importable_count: 0,
            },
          ],
        },
        {
          repository: 'https://github.com/example/repo-two',
          sources: [
            {
              id: 'src-2',
              status: 'watching',
              role: 'unattached',
              referencing_skills: [],
              skills_vendored_count: 0,
              importable_count: 0,
            },
          ],
        },
      ],
    };
    queryClient.setQueryData(['sources'], data);

    renderWithProviders(<SourcesScreen />);

    // Assert both repository headings appear
    expect(screen.getByText('https://github.com/example/repo-one')).toBeInTheDocument();
    expect(screen.getByText('https://github.com/example/repo-two')).toBeInTheDocument();

    // Assert orphan warning chip appears for src-2
    expect(screen.getByText('No linked skills')).toBeInTheDocument();
    expect(screen.getByText('no skills')).toBeInTheDocument();
  });

  it('renders error banner with Retry button', async () => {
    vi.spyOn(global, 'fetch').mockRejectedValue(new Error('Failed to load sources from server'));
    const errorClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });

    render(
      <QueryClientProvider client={errorClient}>
        <MemoryRouter>
          <SourcesScreen />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(await screen.findByText('Failed to load sources from server')).toBeInTheDocument();
    expect(screen.getByText('Retry')).toBeInTheDocument();
  });

  it('selectable rows have checkbox, non-selectable rows do not, and button enables upon selection', () => {
    const goldenSources = loadGolden<SourceListResult>('sources');
    const data: SourceListResult = {
      ...goldenSources,
      groups: [
        {
          repository: 'https://github.com/example/repo-one',
          sources: [
            {
              id: 'src-1',
              status: 'changed',
              role: 'learning-source',
              referencing_skills: ['skill-1'],
              skills_vendored_count: 0,
              importable_count: 0,
              ready_to_distill: true,
            },
          ],
        },
        {
          repository: 'https://github.com/example/repo-two',
          sources: [
            {
              id: 'src-2',
              status: 'watching',
              role: 'upstream',
              referencing_skills: ['skill-2'],
              skills_vendored_count: 1,
              importable_count: 0,
              ready_to_distill: false,
              upstream_only: true,
            },
          ],
        },
      ],
    };
    queryClient.setQueryData(['sources'], data);

    renderWithProviders(<SourcesScreen />);

    // src-1 is selectable (has checkbox)
    const checkboxSrc1 = screen.getByLabelText('Select src-1 for distillation');
    expect(checkboxSrc1).toBeInTheDocument();
    expect(checkboxSrc1).not.toBeChecked();

    // src-2 is non-selectable (no checkbox)
    expect(screen.queryByLabelText('Select src-2 for distillation')).not.toBeInTheDocument();

    // Distill button is initially disabled with reason title
    const distillBtn = screen.getByRole('button', { name: /Distill with Curator Agent/i });
    expect(distillBtn).toBeDisabled();
    expect(distillBtn).toHaveAttribute('title', 'Select at least one learning source');

    // Check src-1
    fireEvent.click(checkboxSrc1);
    expect(checkboxSrc1).toBeChecked();

    // Button becomes active link with (1)
    const distillLink = screen.getByRole('link', { name: /Distill with Curator Agent \(1\)/i });
    expect(distillLink).toBeInTheDocument();
    expect(distillLink).toHaveAttribute('href', '/sources/distill?source=src-1');
  });

  it('filters to only selectable sources when ?filter=ready', () => {
    const goldenSources = loadGolden<SourceListResult>('sources');
    const data: SourceListResult = {
      ...goldenSources,
      groups: [
        {
          repository: 'https://github.com/example/repo-one',
          sources: [
            {
              id: 'src-1',
              status: 'changed',
              role: 'learning-source',
              referencing_skills: ['skill-1'],
              skills_vendored_count: 0,
              importable_count: 0,
              ready_to_distill: true,
            },
          ],
        },
        {
          repository: 'https://github.com/example/repo-two',
          sources: [
            {
              id: 'src-2',
              status: 'watching',
              role: 'upstream',
              referencing_skills: ['skill-2'],
              skills_vendored_count: 1,
              importable_count: 0,
              ready_to_distill: false,
              upstream_only: true,
            },
          ],
        },
      ],
    };
    queryClient.setQueryData(['sources'], data);

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={['/sources?filter=ready']}>
          <SourcesScreen />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    // src-1 appears
    expect(screen.getByText('src-1')).toBeInTheDocument();
    expect(screen.getByText('https://github.com/example/repo-one')).toBeInTheDocument();

    // src-2 does NOT appear
    expect(screen.queryByText('src-2')).not.toBeInTheDocument();
    expect(screen.queryByText('https://github.com/example/repo-two')).not.toBeInTheDocument();
  });
});
