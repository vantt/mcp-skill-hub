import { describe, expect, it } from 'vitest';
import type { SourceSummary } from '../api/types';
import { distillLabel } from './source-distill';

describe('distillLabel', () => {
  const baseSummary: SourceSummary = {
    id: 'src-1',
    status: 'watching',
    role: 'learning-source',
    referencing_skills: ['skill-1'],
    skills_vendored_count: 0,
    importable_count: 0,
    ready_to_distill: true,
  };

  const cases: Array<{
    name: string;
    summary: Partial<SourceSummary>;
    sessionCheck?: { availability?: string };
    wantLabel: string;
    wantSelectable: boolean;
  }> = [
    {
      name: 'priority: session check unavailable on changed source returns Unreachable',
      summary: { ...baseSummary, status: 'changed', ready_to_distill: true },
      sessionCheck: { availability: 'unavailable' },
      wantLabel: 'Unreachable (last check)',
      wantSelectable: false,
    },
    {
      name: 'not ready to distill and upstream_only returns Not a learning source',
      summary: { ...baseSummary, ready_to_distill: false, upstream_only: true },
      wantLabel: 'Not a learning source',
      wantSelectable: false,
    },
    {
      name: 'not ready to distill and not upstream_only returns Up to date',
      summary: { ...baseSummary, ready_to_distill: false, upstream_only: false },
      wantLabel: 'Up to date',
      wantSelectable: false,
    },
    {
      name: 'status changed returns Changed and is selectable',
      summary: { ...baseSummary, status: 'changed', ready_to_distill: true },
      wantLabel: 'Changed',
      wantSelectable: true,
    },
    {
      name: 'status distill_pending returns Distill pending and is selectable',
      summary: { ...baseSummary, status: 'distill_pending', ready_to_distill: true },
      wantLabel: 'Distill pending',
      wantSelectable: true,
    },
    {
      name: 'no distilled_revision returns Never distilled and is selectable',
      summary: { ...baseSummary, status: 'watching', ready_to_distill: true, distilled_revision: undefined },
      wantLabel: 'Never distilled',
      wantSelectable: true,
    },
  ];

  for (const tc of cases) {
    it(tc.name, () => {
      const res = distillLabel(tc.summary as SourceSummary, tc.sessionCheck);
      expect(res.label).toBe(tc.wantLabel);
      expect(res.selectable).toBe(tc.wantSelectable);
    });
  }
});
