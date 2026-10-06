import { describe, expect, it } from 'vitest';
import type { Comparison, InsightDetailResult, Observation } from '../api/types';
import {
  addMapping,
  coverage,
  requiredObservations,
  type ConceptMapping,
  type RequiredObservation,
} from './evidence-set';

describe('evidence-set domain logic', () => {
  const sampleFinding1: Observation = {
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

  const sampleFinding2: Observation = {
    schema_version: 1,
    id: 'obs-2',
    source_id: 'src-1',
    run_id: 'run-1',
    stable_key: 'key-2',
    status: 'active',
    what: 'Finding 2',
    vocabulary: ['voc2'],
    evidence: [],
  };

  const sampleFinding3: Observation = {
    schema_version: 1,
    id: 'obs-3',
    source_id: 'src-1',
    run_id: 'run-1',
    stable_key: 'key-3',
    status: 'active',
    what: 'Finding 3',
    vocabulary: ['voc3'],
    evidence: [],
  };

  const sampleComparison: Comparison = {
    schema_version: 1,
    id: 'cmp-1',
    run_id: 'run-1',
    subject: 'Comparison Subject',
    observation_ids: ['obs-2', 'obs-3'],
    verdict: 'convergent',
    tradeoffs: 'none',
    stale: false,
  };

  it('requiredObservations combines direct and comparison observations deduplicated, counting overlaps once', () => {
    // obs-1 is direct only
    // obs-2 is direct AND in comparison (counted once)
    // obs-3 is comparison only
    const detail: InsightDetailResult = {
      schema_version: '1',
      status: 'ok',
      summary: 'Insight loaded',
      insight: {
        schema_version: 1,
        id: 'ins-1',
        run_id: 'run-1',
        stable_key: 'key-1',
        skill_id: 'skill-1',
        status: 'pending',
        recommendation: 'Rec 1',
        observation_ids: ['obs-1', 'obs-2'],
        comparison_ids: ['cmp-1'],
        category: 'reliability',
        priority: 'high',
        rationale: 'Rat 1',
        evidence_digest: 'digest-1',
      },
      findings: [sampleFinding1, sampleFinding2, sampleFinding3],
      comparisons: [sampleComparison],
    };

    const reqs = requiredObservations(detail);
    expect(reqs).toHaveLength(3);

    const ids = reqs.map((r) => r.id);
    expect(ids).toEqual(['obs-1', 'obs-2', 'obs-3']);

    // obs-1 has direct source only
    const req1 = reqs.find((r) => r.id === 'obs-1')!;
    expect(req1.finding).toEqual(sampleFinding1);
    expect(req1.sources).toEqual([{ kind: 'direct' }]);

    // obs-2 has both direct and comparison sources
    const req2 = reqs.find((r) => r.id === 'obs-2')!;
    expect(req2.finding).toEqual(sampleFinding2);
    expect(req2.sources).toHaveLength(2);
    expect(req2.sources[0]).toEqual({ kind: 'direct' });
    expect(req2.sources[1]).toEqual({
      kind: 'comparison',
      comparisonId: 'cmp-1',
      subject: 'Comparison Subject',
      verdict: 'convergent',
    });

    // obs-3 has comparison source only
    const req3 = reqs.find((r) => r.id === 'obs-3')!;
    expect(req3.finding).toEqual(sampleFinding3);
    expect(req3.sources).toEqual([
      {
        kind: 'comparison',
        comparisonId: 'cmp-1',
        subject: 'Comparison Subject',
        verdict: 'convergent',
      },
    ]);
  });

  it('coverage counts observations with non-empty trimmed concept and reports unmapped', () => {
    const required: RequiredObservation[] = [
      { id: 'obs-1', sources: [{ kind: 'direct' }] },
      { id: 'obs-2', sources: [{ kind: 'direct' }] },
      { id: 'obs-3', sources: [{ kind: 'direct' }] },
    ];

    // Case 1: Partial mapping with an empty concept that does not count
    const mappings: ConceptMapping[] = [
      { observation_id: 'obs-1', concept: 'retry' },
      { observation_id: 'obs-2', concept: '   ' }, // should not count
    ];

    const res1 = coverage(mappings, required);
    expect(res1.mapped).toBe(1);
    expect(res1.required).toBe(3);
    expect(res1.unmapped).toEqual(['obs-2', 'obs-3']);

    // Case 2: Full coverage
    const fullMappings: ConceptMapping[] = [
      { observation_id: 'obs-1', concept: 'retry' },
      { observation_id: 'obs-2', concept: 'backoff' },
      { observation_id: 'obs-3', concept: 'timeout' },
    ];

    const res2 = coverage(fullMappings, required);
    expect(res2.mapped).toBe(3);
    expect(res2.required).toBe(3);
    expect(res2.unmapped).toEqual([]);
  });

  it('addMapping rejects duplicates, ignores empty, and allows multiple concepts for one observation', () => {
    let mappings: ConceptMapping[] = [];

    // Add first mapping
    mappings = addMapping(mappings, { observation_id: 'obs-1', concept: 'retry' });
    expect(mappings).toHaveLength(1);
    expect(mappings[0]).toEqual({ observation_id: 'obs-1', concept: 'retry' });

    // Ignore empty or whitespace concept
    mappings = addMapping(mappings, { observation_id: 'obs-1', concept: '   ' });
    expect(mappings).toHaveLength(1);

    // Reject duplicate (case-insensitive)
    mappings = addMapping(mappings, { observation_id: 'obs-1', concept: 'RETRY' });
    expect(mappings).toHaveLength(1);

    // Allow second concept for the same observation
    mappings = addMapping(mappings, { observation_id: 'obs-1', concept: 'backoff' });
    expect(mappings).toHaveLength(2);
    expect(mappings[1]).toEqual({ observation_id: 'obs-1', concept: 'backoff' });

    // Add concept for a different observation
    mappings = addMapping(mappings, { observation_id: 'obs-2', concept: 'retry' });
    expect(mappings).toHaveLength(3);
  });
});
