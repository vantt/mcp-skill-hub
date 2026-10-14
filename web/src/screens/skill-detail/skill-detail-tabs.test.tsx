import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router';
import { describe, expect, it } from 'vitest';
import type { SessionResponse, SkillDetail, SkillFiles, SkillReviewResult, SkillSourcesResult } from '../../api/types';
import { loadGolden } from '../../test/golden';
import { SkillDetailScreen } from './SkillDetailScreen';

const SKILL_ID = 'review-skill';

const FILES = {
  schema_version: '1',
  status: 'ok',
  summary: 'review-skill: 1 file.',
  items: [],
  suggested_actions: [],
  warnings: [],
  error: null,
  skill_id: SKILL_ID,
  state: 'active',
  base_version: 'sha256:base',
  files: [{ path: 'SKILL.md', size_bytes: 10, digest: 'sha256:a', text: true, editable: true, executable: false }],
} satisfies SkillFiles;

function LocationProbe() {
  const location = useLocation();
  return <output data-testid="location-search">{location.search}</output>;
}

function renderAt(url: string) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  queryClient.setQueryData(['session'], loadGolden<SessionResponse>('session'));
  queryClient.setQueryData(['skill', SKILL_ID], loadGolden<SkillDetail>('skill-detail'));
  queryClient.setQueryData(['skill-review', SKILL_ID], loadGolden<SkillReviewResult>('skill-review'));
  queryClient.setQueryData(['skill-sources', SKILL_ID], loadGolden<SkillSourcesResult>('skill-sources'));
  queryClient.setQueryData(['skill-files', SKILL_ID], FILES);
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[url]}>
        <Routes>
          <Route
            path="/skills/:id"
            element={
              <>
                <SkillDetailScreen />
                <LocationProbe />
              </>
            }
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('SkillDetailScreen tabs', () => {
  it('opens Overview first and by default when the URL has no tab', () => {
    renderAt(`/skills/${SKILL_ID}`);

    const tabs = screen.getAllByRole('tab');
    expect(tabs[0]).toHaveTextContent('Overview');
    expect(tabs[0]).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('tab', { name: 'Review' })).toHaveAttribute('aria-selected', 'false');
    expect(screen.getByText('Review changed code safely.', { selector: 'p' })).toBeInTheDocument();
  });

  it('keeps ?tab=review opening the Review tab', () => {
    renderAt(`/skills/${SKILL_ID}?tab=review`);

    expect(screen.getByRole('tab', { name: 'Review' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('tab', { name: 'Overview' })).toHaveAttribute('aria-selected', 'false');
    expect(screen.getByText('Activation readiness')).toBeInTheDocument();
  });

  it('switches back to Overview when its tab is clicked', () => {
    renderAt(`/skills/${SKILL_ID}?tab=review`);

    fireEvent.click(screen.getByRole('tab', { name: 'Overview' }));

    expect(screen.getByRole('tab', { name: 'Overview' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.queryByText('Activation readiness')).not.toBeInTheDocument();
  });

  it('drops the open file and review view when another tab is chosen', () => {
    renderAt(`/skills/${SKILL_ID}?tab=files&file=references%2Frun.sh&view=review`);

    expect(screen.getByRole('tab', { name: 'Files' })).toHaveAttribute('aria-selected', 'true');

    fireEvent.click(screen.getByRole('tab', { name: 'Review' }));

    const search = new URLSearchParams(screen.getByTestId('location-search').textContent ?? '');
    expect(search.get('tab')).toBe('review');
    expect(search.has('file')).toBe(false);
    expect(search.has('view')).toBe(false);
  });

  it('has no Editor or Resources tab, and sends their old links to Files and Routing', () => {
    renderAt(`/skills/${SKILL_ID}?tab=resources`);
    expect(screen.queryByRole('tab', { name: 'Editor' })).not.toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'Resources' })).not.toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Files' })).toHaveAttribute('aria-selected', 'true');
  });

  it('opens Routing for an old editor link, and Files when the link named the content', () => {
    const first = renderAt(`/skills/${SKILL_ID}?tab=editor`);
    expect(screen.getByRole('tab', { name: 'Routing' })).toHaveAttribute('aria-selected', 'true');
    first.unmount();
    renderAt(`/skills/${SKILL_ID}?tab=editor&field=content`);
    expect(screen.getByRole('tab', { name: 'Files' })).toHaveAttribute('aria-selected', 'true');
  });
});
