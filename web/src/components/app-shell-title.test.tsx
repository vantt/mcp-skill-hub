import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it, vi } from 'vitest';

const SUB = 'SUB-01M5291K4CWKQH7K96HNCFQB0V';

vi.mock('../api/client', async (orig) => {
  const actual = await orig<typeof import('../api/client')>();
  return {
    ...actual,
    apiFetch: (path: string) => {
      if (path === `/submissions/${SUB}`) return Promise.resolve({ skill_id: 'release-checklist', files: [] });
      if (path === '/review-queue') return Promise.resolve({ items: [] });
      return Promise.resolve({});
    },
  };
});

import { AppShell } from './AppShell';

function renderAt(path: string) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[path]}>
        <AppShell>
          <div />
        </AppShell>
      </MemoryRouter>
    </QueryClientProvider>,
  );
}

describe('AppShell page title', () => {
  it('names the review queue', async () => {
    renderAt('/review');
    await waitFor(() => expect(document.title).toBe('Waiting for review · Skill Hub'));
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('Waiting for review');
  });

  it('names a submitted skill under the review queue, not the home page', async () => {
    renderAt(`/review/submissions/${SUB}`);
    await waitFor(() => expect(document.title).toBe('release-checklist · Skill Hub'));
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('release-checklist');
    const crumb = document.querySelector('.app-crumb a');
    expect(crumb).toHaveTextContent('Waiting for review');
    expect(crumb).toHaveAttribute('href', '/review');
  });
});
