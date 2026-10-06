import type { InsightDetailResult, Observation } from '../api/types';

export interface RequiredObservationSource {
  kind: 'direct' | 'comparison';
  comparisonId?: string;
  subject?: string;
  verdict?: string;
}

export interface RequiredObservation {
  id: string;
  finding?: Observation;
  sources: RequiredObservationSource[];
}

export interface ConceptMapping {
  observation_id: string;
  concept: string;
}

export interface CoverageResult {
  mapped: number;
  required: number;
  unmapped: string[];
}

export function requiredObservations(detail: InsightDetailResult): RequiredObservation[] {
  const findingsMap = new Map<string, Observation>();
  for (const f of detail.findings || []) {
    findingsMap.set(f.id, f);
  }

  const map = new Map<string, RequiredObservation>();

  for (const obsId of detail.insight.observation_ids || []) {
    let entry = map.get(obsId);
    if (!entry) {
      entry = {
        id: obsId,
        finding: findingsMap.get(obsId),
        sources: [],
      };
      map.set(obsId, entry);
    }
    entry.sources.push({ kind: 'direct' });
  }

  for (const comp of detail.comparisons || []) {
    for (const obsId of comp.observation_ids || []) {
      let entry = map.get(obsId);
      if (!entry) {
        entry = {
          id: obsId,
          finding: findingsMap.get(obsId),
          sources: [],
        };
        map.set(obsId, entry);
      }
      entry.sources.push({
        kind: 'comparison',
        comparisonId: comp.id,
        subject: comp.subject,
        verdict: comp.verdict,
      });
    }
  }

  return Array.from(map.values());
}

export function coverage(
  mappings: ConceptMapping[],
  required: RequiredObservation[],
): CoverageResult {
  const mappedObsIds = new Set<string>();
  for (const m of mappings) {
    if (m.concept && m.concept.trim().length > 0) {
      mappedObsIds.add(m.observation_id);
    }
  }

  const unmapped: string[] = [];
  for (const r of required) {
    if (!mappedObsIds.has(r.id)) {
      unmapped.push(r.id);
    }
  }

  const mappedCount = required.length - unmapped.length;
  return {
    mapped: mappedCount,
    required: required.length,
    unmapped,
  };
}

export function addMapping(
  mappings: ConceptMapping[],
  newMapping: ConceptMapping,
): ConceptMapping[] {
  const obsId = newMapping.observation_id.trim();
  const concept = newMapping.concept.trim();
  if (!concept) {
    return mappings;
  }
  const exists = mappings.some(
    (m) =>
      m.observation_id.trim() === obsId &&
      m.concept.trim().toLowerCase() === concept.toLowerCase(),
  );
  if (exists) {
    return mappings;
  }
  return [...mappings, { observation_id: obsId, concept }];
}
