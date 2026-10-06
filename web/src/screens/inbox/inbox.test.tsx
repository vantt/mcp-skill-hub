import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { InboxPageResult } from '../../api/types';
import { loadGolden } from '../../test/golden';
import { InboxScreen } from './InboxScreen';

describe('InboxScreen', () => {
  let queryClient: QueryClient;
  const goldenInbox = loadGolden<InboxPageResult>('inbox');

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

  function renderWithProviders(ui: React.ReactElement, initialEntries = ['/inbox']) {
    return render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={initialEntries}>{ui}</MemoryRouter>
      </QueryClientProvider>,
    );
  }

  it('renders empty state when there are no insights', () => {
    const emptyInbox: InboxPageResult = {
      ...goldenInbox,
      groups: [],
      total: 0,
      has_more: false,
    };
    queryClient.setQueryData(['inbox'], emptyInbox);

    renderWithProviders(<InboxScreen />);

    expect(screen.getByText('No pending or planned insights.')).toBeInTheDocument();
  });

  it('renders groups and items from golden inbox response', () => {
    queryClient.setQueryData(['inbox'], goldenInbox);

    renderWithProviders(<InboxScreen />);

    // Group header
    expect(screen.getByText('review-skill')).toBeInTheDocument();
    expect(screen.getByRole('option', { name: 'reliability' })).toBeInTheDocument();

    // Item recommendation and priority
    expect(screen.getByText('Add retry guidance to code review.')).toBeInTheDocument();
    expect(screen.getByText('High')).toBeInTheDocument();
    expect(screen.getByText(/Score 78/)).toBeInTheDocument();
    expect(screen.getByText('Open →')).toBeInTheDocument();
  });

  it('filters by category, status, and priority over loaded pages and shows filtered count', () => {
    const multiInbox: InboxPageResult = {
      ...goldenInbox,
      groups: [
        {
          skill_id: 'review-skill',
          category: 'reliability',
          items: [
            {
              insight: {
                schema_version: 1,
                id: 'ins-1',
                run_id: 'run-1',
                stable_key: 'key-1',
                skill_id: 'review-skill',
                status: 'pending',
                recommendation: 'Reliability rec',
                observation_ids: ['obs-1'],
                category: 'reliability',
                priority: 'high',
                rationale: 'Rat',
                evidence_digest: 'd1',
              },
              rank: { score: 80, evidence_sources: 1, evidence_findings: 1, impact: 'high', stale: false },
            },
          ],
        },
        {
          skill_id: 'review-skill',
          category: 'clarity',
          items: [
            {
              insight: {
                schema_version: 1,
                id: 'ins-2',
                run_id: 'run-1',
                stable_key: 'key-2',
                skill_id: 'review-skill',
                status: 'planned',
                recommendation: 'Clarity rec',
                observation_ids: ['obs-2'],
                category: 'clarity',
                priority: 'medium',
                rationale: 'Rat',
                evidence_digest: 'd2',
              },
              rank: { score: 60, evidence_sources: 1, evidence_findings: 1, impact: 'medium', stale: false },
            },
          ],
        },
      ],
      total: 2,
    };
    queryClient.setQueryData(['inbox'], multiInbox);

    renderWithProviders(<InboxScreen />);

    expect(screen.getByText(/Filtering 2 loaded groups/)).toBeInTheDocument();
    expect(screen.getByText('Reliability rec')).toBeInTheDocument();
    expect(screen.getByText('Clarity rec')).toBeInTheDocument();

    // Filter by Category: reliability
    const categorySelect = screen.getByLabelText('Category');
    fireEvent.change(categorySelect, { target: { value: 'reliability' } });

    expect(screen.getByText(/Filtering 1 loaded groups/)).toBeInTheDocument();
    expect(screen.getByText('Reliability rec')).toBeInTheDocument();
    expect(screen.queryByText('Clarity rec')).not.toBeInTheDocument();
  });

  it('renders filter empty state with Clear filters button when no loaded groups match', () => {
    queryClient.setQueryData(['inbox'], goldenInbox);

    renderWithProviders(<InboxScreen />);

    // Filter by Priority: Low (golden has High)
    const prioritySelect = screen.getByLabelText('Priority');
    fireEvent.change(prioritySelect, { target: { value: 'low' } });

    expect(screen.getByText('No matches in the loaded groups.')).toBeInTheDocument();
    const clearBtn = screen.getByRole('button', { name: 'Clear filters' });
    expect(clearBtn).toBeInTheDocument();

    fireEvent.click(clearBtn);
    expect(screen.getByText('Add retry guidance to code review.')).toBeInTheDocument();
  });
});
