import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { InsightDetailResult } from '../../api/types';
import { loadGolden } from '../../test/golden';
import { InsightDetailScreen } from './InsightDetailScreen';

describe('InsightDetailScreen', () => {
  let queryClient: QueryClient;
  const goldenInsight = loadGolden<InsightDetailResult>('insight-detail');
  const insId = goldenInsight.insight.id;

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

  function renderWithRoute(id = insId) {
    return render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[`/inbox/${id}`]}>
          <Routes>
            <Route path="/inbox/:id" element={<InsightDetailScreen />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );
  }

  it('renders 404 Not Found state when insight is unknown', async () => {
    vi.spyOn(global, 'fetch').mockResolvedValue(
      new Response(
        JSON.stringify({
          schema_version: '1',
          status: 'error',
          error: { code: 'invalid_request', render: { ERROR: 'insight not found' } },
        }),
        { status: 404, headers: { 'Content-Type': 'application/json' } },
      ),
    );

    renderWithRoute('INS-NONEXISTENT');

    expect(await screen.findByText('Insight not found')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Back to Inbox' })).toHaveAttribute(
      'href',
      '/inbox',
    );
  });

  it('renders detail view with recommendation, rationale, findings, and active Compose patch button', () => {
    queryClient.setQueryData(['insight', insId], goldenInsight);

    renderWithRoute();

    expect(screen.getByText(goldenInsight.insight.recommendation)).toBeInTheDocument();
    expect(screen.getByText(goldenInsight.insight.rationale)).toBeInTheDocument();
    expect(screen.getByText(/Findings · 1/)).toBeInTheDocument();
    expect(screen.getByText(/high/i)).toBeInTheDocument();

    const composeBtn = screen.getByRole('button', { name: 'Compose patch' });
    expect(composeBtn).toBeInTheDocument();
    expect(composeBtn).not.toBeDisabled();
  });

  it('disables Compose patch button with reason when insight is stale', () => {
    const firstFinding = goldenInsight.findings[0]!;
    const staleInsight: InsightDetailResult = {
      ...goldenInsight,
      findings: [
        {
          ...firstFinding,
          status: 'removed', // direct finding removed -> stale
        },
      ],
    };
    queryClient.setQueryData(['insight', insId], staleInsight);

    renderWithRoute();

    const composeBtn = screen.getByRole('button', { name: 'Compose patch' });
    expect(composeBtn).toBeDisabled();
    expect(composeBtn).toHaveAttribute(
      'title',
      'Supporting evidence is stale. Re-run distillation or inspect findings before composing.',
    );
  });

  it('decision modal requires non-empty rationale and calls decide endpoint', async () => {
    queryClient.setQueryData(['insight', insId], goldenInsight);

    let decideCalled = false;
    vi.spyOn(global, 'fetch').mockImplementation(async (input, init) => {
      const url = String(input);
      if (url.includes('/decision') && init?.method === 'POST') {
        decideCalled = true;
        const parsed = JSON.parse(String(init.body));
        expect(parsed.decision).toBe('plan');
        expect(parsed.rationale).toBe('Plan it for next sprint.');
        return new Response(
          JSON.stringify({
            schema_version: '1',
            status: 'ok',
            insight: { ...goldenInsight.insight, status: 'planned' },
            operation_id: 'OP-DECIDE-1',
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        );
      }
      return new Response(JSON.stringify(goldenInsight), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    });

    renderWithRoute();

    const planBtn = screen.getByRole('button', { name: 'Plan' });
    fireEvent.click(planBtn);

    // Modal opens
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    const confirmBtn = screen.getByRole('button', { name: 'Confirm plan' });
    expect(confirmBtn).toBeDisabled();

    // Type rationale
    const textarea = screen.getByPlaceholderText('Explain the reason for this decision…');
    fireEvent.change(textarea, { target: { value: 'Plan it for next sprint.' } });
    expect(confirmBtn).not.toBeDisabled();

    fireEvent.click(confirmBtn);
    await screen.findByText('Status');
    expect(decideCalled).toBe(true);
  });
});
