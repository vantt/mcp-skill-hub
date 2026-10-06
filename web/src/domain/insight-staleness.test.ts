import { describe, expect, it } from 'vitest';
import type { Comparison, InsightDetailResult, Observation } from '../api/types';
import { isInsightStale } from './insight-staleness';

describe('isInsightStale', () => {
  const baseFinding: Observation = {
    schema_version: 1,
    id: 'obs-1',
    source_id: 'src-1',
    run_id: 'run-1',
    stable_key: 'key-1',
    status: 'active',
    what: 'Finding 1',
    vocabulary: ['voc1'],
    evidence: [],
  };

  const baseComparison: Comparison = {
    schema_version: 1,
    id: 'cmp-1',
    run_id: 'run-1',
    subject: 'Subject 1',
    observation_ids: ['obs-1'],
    verdict: 'convergent',
    tradeoffs: '',
    stale: false,
  };

  const createDetail = (
    findingOverrides?: Partial<Observation> | null,
    comparisonOverrides?: Partial<Comparison>,
  ): InsightDetailResult => {
    return {
      schema_version: '1',
      status: 'ok',
      summary: 'Detail',
      insight: {
        schema_version: 1,
        id: 'ins-1',
        run_id: 'run-1',
        stable_key: 'key-1',
        skill_id: 'skill-1',
        status: 'pending',
        recommendation: 'Rec',
        observation_ids: ['obs-1'],
        comparison_ids: ['cmp-1'],
        category: 'reliability',
        priority: 'high',
        rationale: 'Rationale',
        evidence_digest: 'digest-1',
      },
      findings:
        findingOverrides === null ? [] : [{ ...baseFinding, ...findingOverrides }],
      comparisons: [{ ...baseComparison, ...comparisonOverrides }],
    };
  };

  it('returns false when all direct findings are active and comparisons are fresh', () => {
    const detail = createDetail();
    expect(isInsightStale(detail)).toBe(false);
  });

  it('returns true when a direct finding is missing', () => {
    const detail = createDetail(null);
    expect(isInsightStale(detail)).toBe(true);
  });

  it('returns true when a direct finding has status removed', () => {
    const detail = createDetail({ status: 'removed' });
    expect(isInsightStale(detail)).toBe(true);
  });

  it('returns true when a direct finding has status superseded', () => {
    const detail = createDetail({ status: 'superseded' });
    expect(isInsightStale(detail)).toBe(true);
  });

  it('returns true when a comparison is stale', () => {
    const detail = createDetail(undefined, { stale: true });
    expect(isInsightStale(detail)).toBe(true);
  });
});
