import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { UsagePanel } from './UsagePanel';
import { loadGolden } from '../../test/golden';
import type { FunnelReport } from '../../api/types';

function renderUsagePanel(reportData?: FunnelReport, skillId = 'review-skill') {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        retry: false,
      },
    },
  });

  if (reportData) {
    queryClient.setQueryData(['skill-usage', skillId, '30d'], reportData);
    queryClient.setQueryData(['skill-usage', skillId, '7d'], {
      ...reportData,
      window: { since: '2026-09-27', until: '2026-10-04', days: 8 },
    });
  }

  return {
    queryClient,
    ...render(
      <QueryClientProvider client={queryClient}>
        <UsagePanel skillId={skillId} />
      </QueryClientProvider>,
    ),
  };
}

describe('UsagePanel', () => {
  it('renders populated usage metrics from golden skill-usage', () => {
    const golden = loadGolden<FunnelReport>('skill-usage');
    renderUsagePanel(golden);

    // Group titles
    expect(screen.getByText('Recommendations & Activations')).toBeInTheDocument();
    expect(screen.getByText('Content loads')).toBeInTheDocument();
    expect(screen.getByText('Health & Feedback')).toBeInTheDocument();
    expect(screen.getByText('Doctor checks')).toBeInTheDocument();

    // Basis captions
    expect(screen.getAllByText('Basis: server-observed').length).toBe(2);
    expect(screen.getByText('Basis: host-reported')).toBeInTheDocument();
    expect(screen.getByText('Basis: terminal')).toBeInTheDocument();

    // Metric values
    expect(screen.getByText('10')).toBeInTheDocument(); // recommended_primary
    expect(screen.getByText('8')).toBeInTheDocument(); // activations.recommended
    expect(screen.getByText('80.0%')).toBeInTheDocument(); // acceptance_rate
    expect(screen.getByText('25.0%')).toBeInTheDocument(); // doctor_failure_rate
  });

  it('renders empty state when every count is zero', () => {
    const emptyReport: FunnelReport = {
      window: { since: '2026-09-04', until: '2026-10-04', days: 31 },
      raw_retention_days: 14,
      rollup_retention_days: 180,
      metric_basis: {},
      skill: {
        skill_id: 'review-skill',
        recommended_primary: 0,
        recommended_supporting: 0,
        activations: { recommended: 0, override: 0, unsolicited: 0 },
        total_activations: 0,
        acceptance_rate: null,
        overrides: 0,
        misses: 0,
        unsolicited: 0,
        blocked_by_review: 0,
        resolutions_review_required: 0,
        resolutions_setup_required: 0,
        loads: { entrypoint: 0, reference: 0, script: 0, asset: 0, resource: 0 },
        total_loads: 0,
        doctor: { ready: 0, setup_required: 0, unsupported_platform: 0, failed: 0 },
        total_doctor: 0,
        doctor_failure_rate: null,
        setup_failed: 0,
        setup_failed_rate: null,
        negative_feedback: 0,
        negative_after_load: 0,
        transcripts: {},
      },
    };

    renderUsagePanel(emptyReport);

    expect(screen.getByText('No usage recorded')).toBeInTheDocument();
    expect(
      screen.getByText(
        'No recommendations, activations, loads, or doctor checks observed for this skill in the selected time window.',
      ),
    ).toBeInTheDocument();
  });

  it('changes query key when selecting different time windows', () => {
    const golden = loadGolden<FunnelReport>('skill-usage');
    const { queryClient } = renderUsagePanel(golden);

    // Default window is 30 days
    const btn7d = screen.getByRole('button', { name: '7 days' });
    const btn30d = screen.getByRole('button', { name: '30 days' });

    expect(btn30d).toHaveClass('fg-btn--primary');
    expect(btn7d).toHaveClass('fg-btn--ghost');

    // Click 7 days
    fireEvent.click(btn7d);

    expect(btn7d).toHaveClass('fg-btn--primary');
    expect(btn30d).toHaveClass('fg-btn--ghost');

    // Verify query state exists for 7d
    const state7d = queryClient.getQueryState(['skill-usage', 'review-skill', '7d']);
    expect(state7d).toBeDefined();
  });
});
