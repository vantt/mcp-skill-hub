import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
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
});
