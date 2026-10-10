import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { SkillsScreen } from './SkillsScreen';
import { loadGolden } from '../../test/golden';
import type { SkillListResult } from '../../api/types';

function renderSkillsScreen(data: SkillListResult, initialEntries = ['/skills']) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });
  queryClient.setQueryData(['skills'], data);
  queryClient.setQueryData(['skills', ''], data);

  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={initialEntries}>
        <Routes>
          <Route path="/skills" element={<SkillsScreen />} />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('SkillsScreen', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('renders with golden skills.json', () => {
    const golden = loadGolden<SkillListResult>('skills');
    renderSkillsScreen(golden);

    expect(screen.getByText('Review Skill')).toBeInTheDocument();
    expect(screen.getByText('review-skill')).toBeInTheDocument();
    expect(screen.getAllByText('core').length).toBeGreaterThan(0);
  });

  it('renders update_available chip and filters by ?upstream=updates', () => {
    const data: SkillListResult = {
      schema_version: '1',
      status: 'ok',
      summary: '2 skills',
      skills: [
        {
          id: 'pdf',
          name: 'PDF Tool',
          collection: 'core',
          lifecycle_state: 'active',
          state: 'active',
          active_locally: true,
          routing_eligible: true,
          upstream_status: 'update_available',
        },
        {
          id: 'docx',
          name: 'Docx Tool',
          collection: 'core',
          lifecycle_state: 'active',
          state: 'active',
          active_locally: true,
          routing_eligible: true,
          upstream_status: 'up_to_date',
        },
      ],
    };

    // Render with no filter
    const { unmount } = renderSkillsScreen(data);
    expect(screen.getByText('Update available')).toBeInTheDocument();
    expect(screen.getByText('Updates (1)')).toBeInTheDocument();
    expect(screen.getByText('PDF Tool')).toBeInTheDocument();
    expect(screen.getByText('Docx Tool')).toBeInTheDocument();
    unmount();

    // Render with ?upstream=updates
    renderSkillsScreen(data, ['/skills?upstream=updates']);
    expect(screen.getByText('PDF Tool')).toBeInTheDocument();
    expect(screen.queryByText('Docx Tool')).not.toBeInTheDocument();
  });

  it('explains an empty upstream-updates list and offers the way to check', () => {
    const data: SkillListResult = {
      schema_version: '1',
      status: 'ok',
      summary: '1 skill',
      skills: [
        {
          id: 'docx',
          name: 'Docx Tool',
          collection: 'core',
          lifecycle_state: 'active',
          state: 'active',
          active_locally: true,
          routing_eligible: true,
          upstream_status: 'up_to_date',
        },
      ],
    };
    renderSkillsScreen(data, ['/skills?upstream=updates']);
    expect(screen.getByText(/No skill has an upstream update right now/)).toBeInTheDocument();
    expect(screen.getByText('skillhub skill outdated --check')).toBeInTheDocument();
    // The filter is a visible control, so no URL has to be typed.
    expect(screen.getByLabelText('Upstream')).toHaveValue('updates');
    expect(screen.getByRole('option', { name: 'Has an update' })).toBeInTheDocument();
  });

  it('search narrows rows', () => {
    const data: SkillListResult = {
      schema_version: '1',
      status: 'ok',
      summary: '2 skills',
      skills: [
        {
          id: 'code-review',
          name: 'Code Review',
          collection: 'engineering',
          lifecycle_state: 'active',
          state: 'active',
          active_locally: true,
          routing_eligible: true,
        },
        {
          id: 'release-notes',
          name: 'Release Notes',
          collection: 'docs',
          lifecycle_state: 'deprecated',
          state: 'deprecated',
          active_locally: false,
          routing_eligible: false,
        },
      ],
    };

    renderSkillsScreen(data);

    expect(screen.getByText('Code Review')).toBeInTheDocument();
    expect(screen.getByText('Release Notes')).toBeInTheDocument();

    const searchInput = screen.getByPlaceholderText('Search id or name…');
    fireEvent.change(searchInput, { target: { value: 'code' } });

    expect(screen.getByText('Code Review')).toBeInTheDocument();
    expect(screen.queryByText('Release Notes')).toBeNull();
  });

  it('empty list shows "No skills yet."', () => {
    const empty: SkillListResult = {
      schema_version: '1',
      status: 'ok',
      summary: '0 skills',
      skills: [],
    };

    renderSkillsScreen(empty);
    expect(screen.getByText('No skills yet.')).toBeInTheDocument();
  });

  it('filtered-empty shows "No skills match these filters." with Clear filters', () => {
    const data: SkillListResult = {
      schema_version: '1',
      status: 'ok',
      summary: '1 skill',
      skills: [
        {
          id: 'test-skill',
          name: 'Test Skill',
          collection: 'core',
          lifecycle_state: 'active',
          state: 'active',
          active_locally: true,
          routing_eligible: true,
        },
      ],
    };

    renderSkillsScreen(data, ['/skills?q=nomatch']);

    expect(screen.getByText('No skills match these filters.')).toBeInTheDocument();
    const clearButton = screen.getByText('Clear filters');
    expect(clearButton).toBeInTheDocument();
    fireEvent.click(clearButton);
  });

  it('renders row menu items per lifecycle state', () => {
    const multiStateData: SkillListResult = {
      schema_version: '1',
      status: 'ok',
      summary: '3 skills',
      skills: [
        {
          id: 'active-skill',
          name: 'Active Skill',
          collection: 'core',
          lifecycle_state: 'active',
          state: 'active',
          active_locally: true,
          routing_eligible: true,
        },
        {
          id: 'deprecated-skill',
          name: 'Deprecated Skill',
          collection: 'core',
          lifecycle_state: 'deprecated',
          state: 'deprecated',
          active_locally: false,
          routing_eligible: false,
        },
        {
          id: 'draft-skill',
          name: 'Draft Skill',
          collection: 'core',
          lifecycle_state: 'draft',
          state: 'draft',
          active_locally: false,
          routing_eligible: false,
        },
      ],
    };

    renderSkillsScreen(multiStateData);

    // 1. Active skill menu has Review and Deprecate
    const activeMenuBtn = screen.getByLabelText('Actions for Active Skill');
    fireEvent.click(activeMenuBtn);
    expect(screen.getByText('Review')).toBeInTheDocument();
    expect(screen.getByText('Deprecate')).toBeInTheDocument();
    expect(screen.queryByText('Archive')).toBeNull();

    // 2. Deprecated skill menu has Review and Archive
    const deprecatedMenuBtn = screen.getByLabelText('Actions for Deprecated Skill');
    fireEvent.click(deprecatedMenuBtn);
    expect(screen.getByText('Archive')).toBeInTheDocument();
    expect(screen.queryByText('Deprecate')).toBeNull();

    // 3. Draft skill menu has Review only
    const draftMenuBtn = screen.getByLabelText('Actions for Draft Skill');
    fireEvent.click(draftMenuBtn);
    expect(screen.getByText('Review')).toBeInTheDocument();
    expect(screen.queryByText('Deprecate')).toBeNull();
    expect(screen.queryByText('Archive')).toBeNull();
  });

  it('labels every filter and finds a skill by id', () => {
    renderSkillsScreen({
      schema_version: '1',
      status: 'ok',
      summary: '1 skill',
      skills: [
        { id: 'code-review', name: 'Quality Gate', collection: 'engineering', lifecycle_state: 'active', state: 'active', active_locally: true, routing_eligible: true },
      ],
    } as SkillListResult);

    expect(screen.getByLabelText('Search')).toBeInTheDocument();
    expect(screen.getByLabelText('Lifecycle')).toBeInTheDocument();
    expect(screen.getByLabelText('Collection')).toBeInTheDocument();
    expect(screen.getByLabelText('Upstream')).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText('Search'), { target: { value: 'code-rev' } });
    expect(screen.getByText('Quality Gate')).toBeInTheDocument();
  });
});
