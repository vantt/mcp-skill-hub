import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen } from '@testing-library/react';
import { BrowserRouter } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { HomeScreen } from './HomeScreen';
import { actionHelp, actionTitle, resolveActionCta } from './action-cta';
import { translate } from '../../i18n';
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
    expect(screen.getByText('1 source is temporarily unavailable')).toBeInTheDocument();
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
        { kind: 'sources_ready_to_distill', count: 3, availability: 'available' },
      ],
    };

    renderHomeScreen(degraded);

    const unavailableLabels = screen.getAllByText('Unavailable');
    expect(unavailableLabels.length).toBeGreaterThan(0);

    // Assert that the category counts (5 and 3) are NOT rendered anywhere
    expect(screen.queryByText('5')).toBeNull();
    expect(screen.queryByText('3')).toBeNull();
  });

  it('explains a missing host integration and offers the fix command', () => {
    expect(resolveActionCta({ kind: 'host_integration_missing' })).toEqual({
      type: 'command',
      command: 'skillhub doctor --fix',
      label: 'Copy command',
    });
    expect(actionHelp('host_integration_missing')?.why).toContain('cannot use your skills');

    renderHomeScreen({
      schema_version: '1',
      status: 'action_required',
      summary: 'host integration missing',
      workspace: { health: 'valid', index: 'current', git_dirty: false, git_configured: true, recovery_pending: false },
      actions: [{ kind: 'host_integration_missing', priority: 85, count: 1, summary: 'host integration missing', command: 'doctor' }],
      categories: [],
    });
    expect(screen.getByText('Your coding agents are not connected to this hub')).toBeInTheDocument();
    expect(screen.getByText(/The files that tell your coding agent/)).toBeInTheDocument();
    expect(screen.getByText('skillhub doctor --fix')).toBeInTheDocument();
    expect(screen.queryByText('host integration missing')).toBeNull();
  });

  it('shows each count once and marks unconfigured categories', () => {
    renderHomeScreen({
      schema_version: '1',
      status: 'ok',
      summary: 'ok',
      workspace: { health: 'valid', index: 'current', git_dirty: false, git_configured: true, recovery_pending: false },
      actions: [],
      categories: [
        { kind: 'sources_ready_to_distill', count: 4, availability: 'available' },
        { kind: 'routing_evaluations', count: 0, availability: 'not_configured' },
      ],
    });
    expect(screen.getAllByText('Sources ready to distill')).toHaveLength(1);
    expect(screen.getByText('Routing evaluations')).toBeInTheDocument();
    expect(screen.queryByText('Action categories')).toBeNull();
    expect(screen.getByText('Not set up')).toBeInTheDocument();
  });

  it('words count-based next actions in the singular and plural', () => {
    const cases: Array<{ kind: string; summary: string; one: string; three: string }> = [
      {
        kind: 'distill_changed_sources',
        summary: '%d source(s) are ready to distill',
        one: '1 source is ready to distill',
        three: '3 sources are ready to distill',
      },
      {
        kind: 'check_due_sources',
        summary: '%d source(s) are due for an update check',
        one: '1 source is due for an update check',
        three: '3 sources are due for an update check',
      },
      {
        kind: 'retry_unavailable_sources',
        summary: '%d source(s) are temporarily unavailable',
        one: '1 source is temporarily unavailable',
        three: '3 sources are temporarily unavailable',
      },
      {
        kind: 'review_upstream_updates',
        summary: '%d skill(s) have upstream changes to review',
        one: '1 skill has upstream changes to review',
        three: '3 skills have upstream changes to review',
      },
      {
        kind: 'review_lessons',
        summary: '%d candidate lesson(s) ready for review',
        one: '1 candidate lesson is ready for review',
        three: '3 candidate lessons are ready for review',
      },
    ];

    for (const item of cases) {
      for (const [count, want] of [
        [1, item.one],
        [3, item.three],
      ] as const) {
        const { unmount, container } = renderHomeScreen({
          schema_version: '1',
          status: 'action_required',
          summary: 'counts',
          workspace: { health: 'valid', index: 'current', git_dirty: false, git_configured: true, recovery_pending: false },
          actions: [{ kind: item.kind, id: 'sample-skill', priority: 70, count, summary: item.summary.replace('%d', String(count)) }],
          categories: [],
        });
        expect(screen.getByText(want)).toBeInTheDocument();
        expect(container.textContent).not.toContain('(s)');
        unmount();
      }
    }
  });

  it('falls back to the summary when a count kind has no usable count', () => {
    expect(actionTitle({ kind: 'distill_changed_sources', summary: '2 source(s) are ready to distill' }, translate)).toBe(
      '2 source(s) are ready to distill',
    );
    expect(
      actionTitle({ kind: 'check_due_sources', count: 0, summary: 'due summary' }, translate),
    ).toBe('due summary');
    expect(actionTitle({ kind: 'future_kind', count: 2, summary: 'future summary' }, translate)).toBe('future summary');
  });

  it('keeps the help title ahead of any count sentence', () => {
    expect(actionTitle({ kind: 'host_integration_missing', count: 3, summary: 'raw' }, translate)).toBe(
      'Your coding agents are not connected to this hub',
    );
  });
});
