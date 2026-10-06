import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { DistillRunResult, SessionResponse, SourceListResult } from '../../api/types';
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

  it('renders empty selection state when no sources are provided', () => {
    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={['/sources/distill']}>
          <DistillHandoffScreen />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    expect(
      screen.getByText('Select learning sources on the Sources screen first.'),
    ).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Back to Sources' })).toHaveAttribute(
      'href',
      '/sources',
    );
  });

  it('brief contains both source IDs and the key, and is stable across re-renders', () => {
    const goldenSession = loadGolden<SessionResponse>('session');
    queryClient.setQueryData(['session'], goldenSession);

    const goldenSources = loadGolden<SourceListResult>('sources');
    const sourcesData: SourceListResult = {
      ...goldenSources,
      groups: [
        {
          repository: 'https://github.com/example/repo',
          sources: [
            {
              id: 'source-a',
              status: 'changed',
              role: 'learning-source',
              referencing_skills: ['skill-1'],
              skills_vendored_count: 0,
              importable_count: 0,
              ready_to_distill: true,
              current_revision: { kind: 'git-commit', value: 'c2' },
              distilled_revision: { kind: 'git-commit', value: 'c1' },
            },
            {
              id: 'source-b',
              status: 'changed',
              role: 'learning-source',
              referencing_skills: ['skill-2'],
              skills_vendored_count: 0,
              importable_count: 0,
              ready_to_distill: true,
              current_revision: { kind: 'git-commit', value: 'b2' },
              distilled_revision: { kind: 'git-commit', value: 'b1' },
            },
          ],
        },
      ],
    };
    queryClient.setQueryData(['sources'], sourcesData);

    const { unmount } = render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={['/sources/distill?source=source-a&source=source-b']}>
          <DistillHandoffScreen />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    // Selected sources are listed
    expect(screen.getByText('Selected sources (2)')).toBeInTheDocument();
    expect(screen.getByText('source-a')).toBeInTheDocument();
    expect(screen.getByText('source-b')).toBeInTheDocument();

    // Brief contains both source IDs and curation_run_start
    const pre = screen.getByText(/curation_run_start/i);
    expect(pre).toBeInTheDocument();
    expect(pre.textContent).toContain('["source-a","source-b"]');
    expect(pre.textContent).not.toContain('#token=');

    // Extract the idempotency_key
    const match = pre.textContent?.match(/idempotency_key:\s*"([^"]+)"/);
    expect(match).not.toBeNull();
    const key = match![1];

    unmount();

    // Reopen with the same selection -> same key
    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={['/sources/distill?source=source-a&source=source-b']}>
          <DistillHandoffScreen />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    const preAgain = screen.getByText(/curation_run_start/i);
    expect(preAgain.textContent).toContain(`idempotency_key: "${key}"`);
  });

  it('pasted run IDs show per-ID results', async () => {
    const goldenSession = loadGolden<SessionResponse>('session');
    queryClient.setQueryData(['session'], goldenSession);

    const goldenSources = loadGolden<SourceListResult>('sources');
    const sourcesData: SourceListResult = {
      ...goldenSources,
      groups: [
        {
          repository: 'https://github.com/example/repo',
          sources: [
            {
              id: 'source-a',
              status: 'changed',
              role: 'learning-source',
              referencing_skills: ['skill-1'],
              skills_vendored_count: 0,
              importable_count: 0,
              ready_to_distill: true,
            },
          ],
        },
      ],
    };
    queryClient.setQueryData(['sources'], sourcesData);
    const goldenRun = loadGolden<DistillRunResult>('run');

    vi.spyOn(global, 'fetch').mockImplementation(async (input) => {
      const url = String(input);
      if (url.includes('/api/v1/sources')) {
        return new Response(JSON.stringify(sourcesData), {
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
      if (url.includes('/api/v1/runs/RUN-OK1')) {
        return new Response(
          JSON.stringify({
            ...goldenRun,
            run: {
              ...goldenRun.run,
              id: 'RUN-OK1',
              source_id: 'source-a',
              state: 'in_progress',
            },
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        );
      }
      return new Response(
        JSON.stringify({
          schema_version: '1',
          status: 'error',
          error: {
            code: 'invalid_request',
            render: { ERROR: 'Run not found' },
          },
        }),
        { status: 404, headers: { 'Content-Type': 'application/json' } },
      );
    });

    render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={['/sources/distill?source=source-a']}>
          <DistillHandoffScreen />
        </MemoryRouter>
      </QueryClientProvider>,
    );

    const textarea = screen.getByLabelText('Paste run IDs returned by agent');
    fireEvent.change(textarea, { target: { value: 'RUN-OK1\nRUN-FAIL2' } });

    const openRunsBtn = screen.getByRole('button', { name: 'Open runs' });
    fireEvent.click(openRunsBtn);

    // Assert RUN-OK1 displays success with link
    const runLink = await screen.findByRole('link', { name: 'RUN-OK1' });
    expect(runLink).toBeInTheDocument();
    expect(runLink).toHaveAttribute('href', '/sources/runs/RUN-OK1');

    // Assert RUN-FAIL2 displays error
    expect(await screen.findByText('Run not found')).toBeInTheDocument();
    expect(screen.getByText('RUN-FAIL2')).toBeInTheDocument();
  });
});
