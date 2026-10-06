import type { InsightDetailResult } from '../api/types';

export function isInsightStale(detail: InsightDetailResult): boolean {
  const findingsMap = new Map<string, string>();
  for (const f of detail.findings || []) {
    findingsMap.set(f.id, f.status);
  }

  // A direct finding is missing or not active
  for (const obsId of detail.insight.observation_ids || []) {
    const status = findingsMap.get(obsId);
    if (!status || status !== 'active') {
      return true;
    }
  }

  // A comparison is stale
  for (const comp of detail.comparisons || []) {
    if (comp.stale) {
      return true;
    }
  }

  return false;
}
