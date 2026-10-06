import type { SourceSummary } from '../api/types';

export interface DistillLabelResult {
  label: string;
  selectable: boolean;
}

export function distillLabel(
  summary: SourceSummary,
  sessionCheck?: { availability?: string },
): DistillLabelResult {
  if (sessionCheck?.availability === 'unavailable') {
    return { label: 'Unreachable (last check)', selectable: false };
  }

  if (!summary.ready_to_distill) {
    if (summary.upstream_only) {
      return { label: 'Not a learning source', selectable: false };
    }
    return { label: 'Up to date', selectable: false };
  }

  if (summary.status === 'changed') {
    return { label: 'Changed', selectable: true };
  }

  if (summary.status === 'distill_pending') {
    return { label: 'Distill pending', selectable: true };
  }

  if (!summary.distilled_revision || !summary.distilled_revision.value) {
    return { label: 'Never distilled', selectable: true };
  }

  return { label: 'Changed', selectable: true };
}
