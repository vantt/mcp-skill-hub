import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes, useLocation } from 'react-router';
import { describe, expect, it } from 'vitest';
import type { SessionResponse, SkillDetail, SkillReviewResult, SkillSourcesResult } from '../../api/types';
import { loadGolden } from '../../test/golden';
import { SkillDetailScreen } from './SkillDetailScreen';

const SKILL_ID = 'review-skill';

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

  it('drops the open resource file and edit mode when another tab is chosen', () => {
    renderAt(`/skills/${SKILL_ID}?tab=resources&file=skills%2Fcore%2Fx%2Frun.sh&edit=1`);

    expect(screen.getByRole('tab', { name: 'Resources' })).toHaveAttribute('aria-selected', 'true');

    fireEvent.click(screen.getByRole('tab', { name: 'Review' }));

    const search = new URLSearchParams(screen.getByTestId('location-search').textContent ?? '');
    expect(search.get('tab')).toBe('review');
    expect(search.has('file')).toBe(false);
    expect(search.has('edit')).toBe(false);
  });
});
