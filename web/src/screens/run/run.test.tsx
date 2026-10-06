import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, within } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { DistillRunResult, SessionResponse } from '../../api/types';
import { addRecentRun } from '../../state/recent-runs';
import { loadGolden } from '../../test/golden';
import { RunScreen } from './RunScreen';

describe('RunScreen', () => {
  let queryClient: QueryClient;
  const goldenRun = loadGolden<DistillRunResult>('run');
  const goldenSession = loadGolden<SessionResponse>('session');

  beforeEach(() => {
    vi.restoreAllMocks();
    window.localStorage.clear();
    queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false },
      },
    });
    queryClient.setQueryData(['session'], goldenSession);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    window.localStorage.clear();
  });

  function renderWithRoute(runId: string) {
    return render(
      <QueryClientProvider client={queryClient}>
        <MemoryRouter initialEntries={[`/sources/runs/${runId}`]}>
          <Routes>
            <Route path="/sources/runs/:id" element={<RunScreen />} />
          </Routes>
        </MemoryRouter>
      </QueryClientProvider>,
    );
  }

  it('renders prepared state panel', () => {
    const data: DistillRunResult = {
      ...goldenRun,
      run: {
        ...goldenRun.run,
        id: 'RUN-PREP',
        state: 'prepared',
      },
    };
    queryClient.setQueryData(['distill-run', 'RUN-PREP'], data);

    renderWithRoute('RUN-PREP');

    expect(screen.getByText('Prepared')).toBeInTheDocument();
    expect(screen.getByText('Waiting for Curator Agent to start the run.')).toBeInTheDocument();
  });

  it('renders in_progress state panel', () => {
    const data: DistillRunResult = {
      ...goldenRun,
      run: {
        ...goldenRun.run,
        id: 'RUN-INPROG',
        state: 'in_progress',
      },
    };
    queryClient.setQueryData(['distill-run', 'RUN-INPROG'], data);

    renderWithRoute('RUN-INPROG');

    expect(screen.getByText('In progress')).toBeInTheDocument();
    expect(
      screen.getByText('The Curator Agent is analyzing changes and producing findings.'),
    ).toBeInTheDocument();
  });

  it('renders awaiting_decision panel with required decision before copying resume handoff', () => {
    const wsId = goldenSession.workspace_id;
    addRecentRun(wsId, {
      runId: 'RUN-DECIDE',
      sourceId: 'source-a',
      state: 'awaiting_decision',
      idempotencyKey: 'key-test-123',
      openedAt: Date.now(),
    });

    const data: DistillRunResult = {
      ...goldenRun,
      run: {
        ...goldenRun.run,
        id: 'RUN-DECIDE',
        state: 'awaiting_decision',
        outstanding_decisions: [
          {
            category: 'ambiguity',
            detail: 'Nitpicks section removed upstream. Keep or drop?',
          },
        ],
      },
    };
    queryClient.setQueryData(['distill-run', 'RUN-DECIDE'], data);

    renderWithRoute('RUN-DECIDE');

    expect(screen.getByText('Outstanding decision')).toBeInTheDocument();
    expect(screen.getByText('ambiguity')).toBeInTheDocument();
    expect(screen.getByText('Nitpicks section removed upstream. Keep or drop?')).toBeInTheDocument();

    const resumeBtn = screen.getByRole('button', { name: 'Copy resume handoff' });
    expect(resumeBtn).toBeDisabled();

    // Type a decision
    const textarea = screen.getByLabelText(/Decision/);
    fireEvent.change(textarea, { target: { value: 'Drop nitpicks guidance.' } });

    expect(resumeBtn).not.toBeDisabled();
  });

  it('renders failed state panel', () => {
    const wsId = goldenSession.workspace_id;
    addRecentRun(wsId, {
      runId: 'RUN-FAIL',
      sourceId: 'source-a',
      state: 'failed',
      idempotencyKey: 'key-test-456',
      openedAt: Date.now(),
    });

    const data: DistillRunResult = {
      ...goldenRun,
      run: {
        ...goldenRun.run,
        id: 'RUN-FAIL',
        state: 'failed',
        failure: 'Coverage missing for checklist.md',
      },
    };
    queryClient.setQueryData(['distill-run', 'RUN-FAIL'], data);

    renderWithRoute('RUN-FAIL');

    expect(screen.getByText('Run failed')).toBeInTheDocument();
    expect(screen.getByText('Coverage missing for checklist.md')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Copy resume handoff' })).not.toBeDisabled();
  });

  it('renders finalized state panel with Open Inbox link and hides cancel button', () => {
    const data: DistillRunResult = {
      ...goldenRun,
      run: {
        ...goldenRun.run,
        id: 'RUN-FINAL',
        state: 'finalized',
        finding_ids: ['f1', 'f2'],
        comparison_ids: ['c1'],
        insight_ids: ['ins1', 'ins2', 'ins3'],
      },
    };
    queryClient.setQueryData(['distill-run', 'RUN-FINAL'], data);

    renderWithRoute('RUN-FINAL');

    expect(screen.getByText('Finalized')).toBeInTheDocument();
    const inboxLink = screen.getByRole('link', { name: 'Open Inbox →' });
    expect(inboxLink).toBeInTheDocument();
    expect(inboxLink).toHaveAttribute('href', '/inbox');

    // Cancel button must be hidden
    expect(screen.queryByRole('button', { name: 'Cancel run' })).not.toBeInTheDocument();
  });

  it('renders cancelled state panel and hides cancel button', () => {
    const data: DistillRunResult = {
      ...goldenRun,
      run: {
        ...goldenRun.run,
        id: 'RUN-CANCELLED',
        state: 'cancelled',
        cancelled_at: '2026-10-05T12:00:00Z',
      },
    };
    queryClient.setQueryData(['distill-run', 'RUN-CANCELLED'], data);

    renderWithRoute('RUN-CANCELLED');

    expect(screen.getByText('Cancelled')).toBeInTheDocument();
    expect(
      screen.getByText(/This run was cancelled. The source cursor was not advanced./),
    ).toBeInTheDocument();

    // Cancel button must be hidden
    expect(screen.queryByRole('button', { name: 'Cancel run' })).not.toBeInTheDocument();
  });

  it('opens cancel dialog with consequence text and submits cancellation', async () => {
    const data: DistillRunResult = {
      ...goldenRun,
      run: {
        ...goldenRun.run,
        id: 'RUN-TOCANCEL',
        state: 'in_progress',
      },
    };
    queryClient.setQueryData(['distill-run', 'RUN-TOCANCEL'], data);

    let cancelCalled = false;
    vi.spyOn(global, 'fetch').mockImplementation(async (input, init) => {
      const url = String(input);
      if (url.includes('/api/v1/runs/RUN-TOCANCEL/cancel') && init?.method === 'POST') {
        cancelCalled = true;
        return new Response(
          JSON.stringify({
            ...goldenRun,
            run: {
              ...goldenRun.run,
              id: 'RUN-TOCANCEL',
              state: 'cancelled',
            },
          }),
          { status: 200, headers: { 'Content-Type': 'application/json' } },
        );
      }
      return new Response(JSON.stringify(data), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      });
    });

    renderWithRoute('RUN-TOCANCEL');

    const cancelBtn = screen.getByRole('button', { name: 'Cancel run' });
    fireEvent.click(cancelBtn);

    // ConfirmDialog opens with spec consequence text
    const dialog = screen.getByRole('alertdialog');
    expect(dialog).toBeInTheDocument();
    expect(within(dialog).getByRole('button', { name: 'Cancel run' })).toBeInTheDocument();
    expect(
      within(dialog).getByText(
        'Cancelling this run will stop processing. The source cursor does not advance, and an agent working on this run will fail to submit.',
      ),
    ).toBeInTheDocument();

    // Click confirm in the dialog
    const confirmBtn = within(dialog).getByRole('button', { name: 'Cancel run' });
    fireEvent.click(confirmBtn);

    await screen.findByRole('button', { name: 'Refresh status' });
    expect(cancelCalled).toBe(true);
  });

  it('hides resume action and displays advisory when run has no stored key', () => {
    // No recent-runs entry added
    const data: DistillRunResult = {
      ...goldenRun,
      run: {
        ...goldenRun.run,
        id: 'RUN-NOKEY',
        state: 'awaiting_decision',
      },
    };
    queryClient.setQueryData(['distill-run', 'RUN-NOKEY'], data);

    renderWithRoute('RUN-NOKEY');

    expect(screen.queryByRole('button', { name: 'Copy resume handoff' })).not.toBeInTheDocument();
    expect(
      screen.getByText(
        "This browser does not have the run's handoff key, so resume is unavailable. Cancel the run, then create a new handoff.",
      ),
    ).toBeInTheDocument();
  });

  it('renders 404 Not Found state with link back to /sources', async () => {
    vi.spyOn(global, 'fetch').mockResolvedValue(
      new Response(
        JSON.stringify({
          schema_version: '1',
          status: 'error',
          error: {
            code: 'invalid_request',
            render: { ERROR: 'distill run not found' },
          },
        }),
        { status: 404, headers: { 'Content-Type': 'application/json' } },
      ),
    );

    renderWithRoute('RUN-NONEXISTENT');

    expect(await screen.findByText('Run not found')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Back to Sources' })).toHaveAttribute('href', '/sources');
  });
});
