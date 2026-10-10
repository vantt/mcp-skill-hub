import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { BrowserRouter } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { HomeScreen } from './HomeScreen';
import { resolveActionCta } from './action-cta';
import { loadGolden } from '../../test/golden';
import type { CurationHome } from '../../api/types';

function renderHomeScreen(data: CurationHome) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
  queryClient.setQueryData(['home'], data);

  return render(
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <HomeScreen />
      </BrowserRouter>
    </QueryClientProvider>,
  );
}

describe('HomeScreen', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders with golden home.json', () => {
    const golden = loadGolden<CurationHome>('home');
    renderHomeScreen(golden);

    expect(screen.getByText('Git has uncommitted canonical changes')).toBeInTheDocument();
    expect(screen.getByText('Workspace')).toBeInTheDocument();
    expect(screen.getByText('Healthy')).toBeInTheDocument();
    expect(screen.getByText('Index current')).toBeInTheDocument();
  });

  it('resolves CTAs for all known kinds plus future_kind', () => {
    const kinds: Array<{ kind: string; id?: string; count?: number; wantType: string; wantText?: string }> = [
      { kind: 'repair_workspace', wantType: 'command', wantText: 'skillhub doctor --fix' },
      { kind: 'recover_workspace', wantType: 'command', wantText: 'skillhub doctor --fix' },
      { kind: 'rebuild_index', wantType: 'command', wantText: 'skillhub rebuild' },
      { kind: 'retry_unavailable_sources', wantType: 'link', wantText: '/sources' },
      { kind: 'check_due_sources', wantType: 'link', wantText: '/sources' },
      { kind: 'distill_changed_sources', wantType: 'link', wantText: '/sources?filter=ready' },
      { kind: 'first_run_commit', wantType: 'command', wantText: 'git commit -m "feat: initial skillhub workspace"' },
      { kind: 'review_git_changes', wantType: 'command', wantText: 'git status' },
      { kind: 'review_upstream_updates', wantType: 'link', wantText: '/skills?upstream=updates' },
      { kind: 'track_upstream_skills', wantType: 'command', wantText: 'skillhub source backfill' },
      { kind: 'link_orphan_sources', wantType: 'link', wantText: '/sources' },
      { kind: 'review_lessons', id: 'sample-skill', wantType: 'link', wantText: '/skills/sample-skill?tab=distill' },
      { kind: 'future_kind', wantType: 'none' },
    ];

    for (const item of kinds) {
      const cta = resolveActionCta({
        kind: item.kind,
        id: item.id,
        count: item.count,
      });
      expect(cta.type).toBe(item.wantType);
      if (cta.type === 'command') {
        expect(cta.command).toContain(item.wantText ?? '');
      } else if (cta.type === 'link') {
        expect(cta.to).toBe(item.wantText);
        if (item.count && item.count > 1) {
          expect(cta.note).toContain('other runs');
        }
      }
    }
  });

  it('renders synthetic body with each action kind', () => {
    const base: CurationHome = {
      schema_version: '1',
      status: 'ok',
      summary: 'Action test',
      workspace: {
        health: 'valid',
        index: 'current',
        git_dirty: false,
        git_configured: true,
        recovery_pending: false,
      },
      actions: [
        {
          kind: 'repair_workspace',
          count: 1,
          priority: 1,
          summary: 'Repair needed',
          command: 'skillhub doctor --fix',
        },
      ],
      categories: [],
    };

    const { unmount } = renderHomeScreen(base);
    expect(screen.getByText('Repair needed')).toBeInTheDocument();
    expect(screen.getByText('skillhub doctor --fix')).toBeInTheDocument();
    unmount();

    const linkBase: CurationHome = {
      ...base,
      actions: [
        {
          kind: 'retry_unavailable_sources',
          priority: 1,
          summary: 'Retry sources needed',
          count: 1,
        },
      ],
    };
    renderHomeScreen(linkBase);
    expect(screen.getByText('Retry sources needed')).toBeInTheDocument();
    expect(screen.getByText('Open sources →')).toBeInTheDocument();
  });

  it('renders Unavailable and no digit for degraded workspace (health: invalid, index: stale)', () => {
    const degraded: CurationHome = {
      schema_version: '1',
      status: 'ok',
      summary: 'Degraded',
      workspace: {
        health: 'invalid',
        index: 'stale',
        git_dirty: false,
        git_configured: true,
        recovery_pending: false,
      },
      actions: [],
      categories: [
        { kind: 'attention_items', count: 5, availability: 'available' },
        { kind: 'changed_sources', count: 3, availability: 'available' },
      ],
    };

    renderHomeScreen(degraded);

    const unavailableLabels = screen.getAllByText('Unavailable');
    expect(unavailableLabels.length).toBeGreaterThan(0);

    // Assert that the category counts (5 and 3) are NOT rendered anywhere
    expect(screen.queryByText('5')).toBeNull();
    expect(screen.queryByText('3')).toBeNull();
  });
});
