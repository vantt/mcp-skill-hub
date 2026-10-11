import type { SkillUpstream } from '../../api/types';

export type Tone = 'success' | 'warning' | 'danger' | 'neutral';

// Maps an upstream status from the API to the badge tone and label the skill detail tabs show.
export function upstreamStatusBadge(status: string): { tone: Tone; label: string } {
  switch (status) {
    case 'up_to_date':
      return { tone: 'success', label: 'Up to date' };
    case 'update_available':
      return { tone: 'warning', label: 'Update available' };
    case 'modified':
      return { tone: 'warning', label: 'Modified locally' };
    case 'unknown':
      return { tone: 'warning', label: 'Not checked' };
    case 'diverged':
      return { tone: 'danger', label: 'Diverged' };
    case 'upstream_removed':
      return { tone: 'danger', label: 'Removed upstream' };
    case 'unavailable':
      return { tone: 'danger', label: 'Unreachable' };
    case 'pinned':
      return { tone: 'neutral', label: 'Pinned' };
    case 'untracked':
      return { tone: 'neutral', label: 'Not tracked' };
    default:
      return { tone: 'neutral', label: status };
  }
}

// An upstream is tracked only when it is linked to a source record; an untracked
// upstream cannot be checked or reviewed until it is backfilled.
export function isUpstreamTracked(upstream: SkillUpstream | null | undefined): boolean {
  return upstream != null && upstream.status !== 'untracked' && upstream.source_id !== '';
}

export function resourceSummaryText(count: number, totalBytes: number): string {
  return `${count} ${count === 1 ? 'file' : 'files'} · ${totalBytes} bytes`;
}
