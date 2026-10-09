import { QueryClient, QueryClientProvider, type UseQueryResult } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import * as queries from '../../api/queries';
import type { SkillDistillDocument } from '../../api/types';
import { DistillTab } from './DistillTab';

describe('DistillTab', () => {
  let queryClient: QueryClient;

  beforeEach(() => {
    vi.restoreAllMocks();
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });
  });

  function renderTab(skillId = 'test-skill') {
    return render(
      <QueryClientProvider client={queryClient}>
        <DistillTab skillId={skillId} />
      </QueryClientProvider>,
    );
  }

  it('renders not found message when distill document is absent', () => {
    vi.spyOn(queries, 'useSkillDistill').mockReturnValue({
      data: undefined,
      isLoading: false,
      error: new Error('not found'),
    } as unknown as UseQueryResult<SkillDistillDocument, Error>);
    renderTab();
    expect(screen.getByText('Distillation Knowledge')).toBeInTheDocument();
    expect(
      screen.getByText('No distillation document (.meta/distill.yaml) recorded for this skill yet.'),
    ).toBeInTheDocument();
  });

  it('renders goal, cursors, coverage, and lessons from document', () => {
    const mockDoc: SkillDistillDocument = {
      goal: {
        status: 'confirmed',
        purpose: 'Learn robust retry and recovery patterns',
        in_scope: ['authoring gate', 'audits'],
        out_of_scope: ['general feature development'],
        failures_it_prevents: ['junk tests accumulate'],
      },
      cursors: {
        openclaw: '3f9c2a1b4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a',
      },
      coverage: {
        openclaw: {
          read: ['docs/retry.md'],
        },
      },
      lessons: [
        {
          key: 'retry-jitter',
          layer: 'content',
          what: 'Exponential backoff with full jitter prevents thundering herd',
          notable: 'Prevents server collapse on retry storms',
          where: [
            'openclaw@3f9c2a1b4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a:docs/retry.md#L10-L20',
          ],
          contrast: 'extends',
          score: {
            relevance: 3,
            facts: ['a', 'b'],
            impact: 2,
            evidence: 2,
            effort: 1,
            why: 'Crucial for reliability',
          },
          decision: {
            state: 'planned',
            reason: 'Validated pattern',
            at: '2026-10-09T09:00:00Z',
          },
        },
      ],
    };

    vi.spyOn(queries, 'useSkillDistill').mockReturnValue({
      data: mockDoc,
      isLoading: false,
      error: null,
    } as unknown as UseQueryResult<SkillDistillDocument, Error>);

    renderTab();

    expect(screen.getByText('Distillation Goal')).toBeInTheDocument();
    expect(screen.getByText('Learn robust retry and recovery patterns')).toBeInTheDocument();
    expect(screen.getByText(/Tracked Cursors/)).toBeInTheDocument();
    expect(screen.getAllByText('openclaw').length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText('3f9c2a1b4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a')).toBeInTheDocument();
    expect(screen.getByText(/Coverage Analysis/)).toBeInTheDocument();
    expect(screen.getByText(/Distilled Lessons/)).toBeInTheDocument();
    expect(screen.getByText('retry-jitter')).toBeInTheDocument();
    expect(screen.getAllByText('planned').length).toBeGreaterThanOrEqual(1);
    expect(
      screen.getByText('Exponential backoff with full jitter prevents thundering herd'),
    ).toBeInTheDocument();
  });
});
