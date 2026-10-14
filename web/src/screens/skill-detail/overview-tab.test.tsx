import { fireEvent, render, screen, within } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';
import type { SkillDetail, SkillReviewResult, SkillSourcesResult, SkillUpstream } from '../../api/types';
import { loadGolden } from '../../test/golden';
import { OverviewTab, type OverviewTabProps } from './OverviewTab';

function upstream(overrides: Partial<SkillUpstream> = {}): SkillUpstream {
  return {
    skill_id: 'review-skill',
    source_id: 'vendor-upstream',
    repository: 'https://github.com/vendor/skills',
    ref: 'main',
    path: 'skills/review-skill',
    base_commit: 'a'.repeat(40),
    latest_commit: 'a'.repeat(40),
    changed_files: 0,
    local: 'clean',
    status: 'up_to_date',
    checked_at: '2026-10-01T00:00:00Z',
    ...overrides,
  };
}

function sourcesResult(overrides: Partial<SkillSourcesResult> = {}): SkillSourcesResult {
  return {
    schema_version: '1',
    status: 'ok',
    summary: '',
    skill_id: 'review-skill',
    upstream: null,
    learning: [],
    ...overrides,
  };
}

function renderOverview(props: Partial<OverviewTabProps> = {}) {
  const onOpenTab = vi.fn();
  const view = render(
    <MemoryRouter>
      <OverviewTab
        skill={loadGolden<SkillDetail>('skill-detail')}
        review={loadGolden<SkillReviewResult>('skill-review-third-party')}
        sources={sourcesResult()}
        sourcesLoading={false}
        sourcesError={null}
        onOpenTab={onOpenTab}
        {...props}
      />
    </MemoryRouter>,
  );
  return { onOpenTab, ...view };
}

function card(name: string) {
  return screen.getByRole('region', { name });
}

describe('OverviewTab', () => {
  it('shows the about facts of the skill', () => {
    renderOverview();
    const about = within(card('About'));
    expect(about.getByText('Review Skill')).toBeInTheDocument();
    expect(about.getByText('Review changed code safely.')).toBeInTheDocument();
    expect(about.getByText('core')).toBeInTheDocument();
    expect(about.getByText('Active')).toBeInTheDocument();
    expect(about.getByText(/^3 files · 207 bytes$/)).toBeInTheDocument();
  });

  it('counts the files the author put in the skill, not the hub metadata', () => {
    const review = loadGolden<SkillReviewResult>('skill-review-third-party');
    renderOverview({
      review: {
        ...review,
        resource_status: { ...review.resource_status, resource_count: 2, total_bytes: 400, file_count: 1, file_bytes: 120 },
      },
    });
    expect(within(card('About')).getByText('1 file · 120 bytes')).toBeInTheDocument();
  });

  it('falls back to the older counts when the review has no file counts', () => {
    const review = loadGolden<SkillReviewResult>('skill-review-third-party');
    const status = { ...review.resource_status, resource_count: 2, total_bytes: 400 };
    delete status.file_count;
    delete status.file_bytes;
    renderOverview({ review: { ...review, resource_status: status } });
    expect(within(card('About')).getByText('2 files · 400 bytes')).toBeInTheDocument();
  });

  it('shows the routing scope and lists', () => {
    renderOverview();
    const routing = within(card('Routing'));
    expect(routing.getByText('Several steps')).toBeInTheDocument();
    expect(routing.getByText('review')).toBeInTheDocument();
    expect(routing.getByText('review changed code')).toBeInTheDocument();
    expect(routing.getByText('write prose')).toBeInTheDocument();
  });

  it('puts triggers first as one paragraph and the description after the facts', () => {
    renderOverview();
    const routing = card('Routing');
    const triggers = within(routing).getByText('review changed code').closest('p');
    expect(triggers?.tagName).toBe('P');
    const scope = within(routing).getByText('Several steps');
    expect(triggers!.compareDocumentPosition(scope) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

    const about = card('About');
    const files = within(about).getByText(/^3 files/);
    const description = within(about).getByText('Review changed code safely.');
    expect(files.compareDocumentPosition(description) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it('reads None for empty routing lists', () => {
    const skill = loadGolden<SkillDetail>('skill-detail');
    skill.routing = { operations: [], triggers: [], not_for: [], min_scope: '' };
    renderOverview({ skill });
    expect(within(card('Routing')).getAllByText('None')).toHaveLength(4);
  });

  it('shows the provenance locator from a third-party review', () => {
    renderOverview();
    expect(screen.getByText('Provenance')).toBeInTheDocument();
    expect(screen.getAllByText('https://github.com/vendor/skills').length).toBeGreaterThan(0);
  });

  it('shows a tracked upstream with its status badge', () => {
    renderOverview({ sources: sourcesResult({ upstream: upstream() }) });
    const sources = within(card('Sources'));
    expect(sources.getByText('https://github.com/vendor/skills@main')).toBeInTheDocument();
    expect(sources.getByText(/skills\/review-skill/)).toBeInTheDocument();
    expect(sources.getByText('Up to date')).toBeInTheDocument();
  });

  it('shows Not tracked for an untracked upstream', () => {
    renderOverview({
      sources: sourcesResult({ upstream: upstream({ source_id: '', ref: '', status: 'untracked' }) }),
    });
    const sources = within(card('Sources'));
    expect(sources.getByText('Not tracked')).toBeInTheDocument();
    expect(sources.getByText('https://github.com/vendor/skills')).toBeInTheDocument();
  });

  it('lists every learning reference', () => {
    renderOverview({
      sources: sourcesResult({
        learning: [
          {
            source_id: 'ideas-repo',
            locator: 'https://github.com/acme/ideas',
            role: 'learning',
            monitoring: { enabled: true, cadence: 'weekly' },
            last_checked_at: '2026-10-05T08:00:00Z',
          },
          {
            source_id: 'notes-repo',
            locator: 'https://github.com/acme/notes',
            role: 'learning',
            monitoring: { enabled: false, cadence: '' },
          },
        ],
      }),
    });
    const sources = within(card('Sources'));
    expect(sources.getByText('ideas-repo')).toBeInTheDocument();
    expect(sources.getByText('https://github.com/acme/ideas')).toBeInTheDocument();
    expect(sources.getByText('Last checked: 2026-10-05')).toBeInTheDocument();
    expect(sources.getByText('notes-repo')).toBeInTheDocument();
    expect(sources.getByText('Last checked: never')).toBeInTheDocument();
  });

  it('shows an empty state when the skill has no source', () => {
    renderOverview();
    expect(within(card('Sources')).getByText('No sources yet.')).toBeInTheDocument();
  });

  it('shows a skeleton while sources load', () => {
    renderOverview({ sources: undefined, sourcesLoading: true });
    const sources = within(card('Sources'));
    expect(sources.getByRole('status', { name: 'Loading sources' })).toBeInTheDocument();
    expect(sources.queryByText('No sources yet.')).not.toBeInTheDocument();
  });

  it('shows a short message when sources fail to load', () => {
    renderOverview({ sources: undefined, sourcesError: new Error('boom') });
    expect(within(card('Sources')).getByText(/Could not load sources/)).toBeInTheDocument();
  });

  it('opens the Sources tab from Manage sources and the Routing tab from Edit', () => {
    const { onOpenTab } = renderOverview();
    fireEvent.click(screen.getByRole('button', { name: 'Manage sources' }));
    expect(onOpenTab).toHaveBeenLastCalledWith('sources');
    fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    expect(onOpenTab).toHaveBeenLastCalledWith('routing');
  });
});
