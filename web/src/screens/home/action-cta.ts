export type CtaResult =
  | { type: 'command'; command: string; label: string }
  | { type: 'link'; to: string; label: string; note?: string }
  | { type: 'none' };

export interface ActionInput {
  kind: string;
  id?: string;
  count?: number;
  command?: string;
  label?: string;
  summary?: string;
}

export function resolveActionCta(action?: ActionInput): CtaResult {
  if (!action) {
    return { type: 'none' };
  }

  switch (action.kind) {
    case 'repair_workspace':
    case 'recover_workspace':
      return {
        type: 'command',
        command: action.command || 'skillhub doctor --fix',
        label: 'Copy command',
      };
    case 'resume_run': {
      const runId = action.id ?? '';
      const count = action.count ?? 1;
      return {
        type: 'link',
        to: `/sources/runs/${encodeURIComponent(runId)}`,
        label: 'Open run →',
        note: count > 1 ? `and ${count - 1} other runs` : undefined,
      };
    }
    case 'rebuild_index':
      return {
        type: 'command',
        command: action.command || 'skillhub rebuild',
        label: 'Copy command',
      };
    case 'retry_unavailable_sources':
      return {
        type: 'link',
        to: '/sources',
        label: 'Open sources →',
      };
    case 'check_due_sources':
      return {
        type: 'link',
        to: '/sources',
        label: 'Check due sources →',
      };
    case 'distill_changed_sources':
      return {
        type: 'link',
        to: '/sources?filter=ready',
        label: 'Open sources →',
      };
    case 'review_insights':
      return {
        type: 'link',
        to: '/inbox',
        label: 'Open Inbox →',
      };
    case 'first_run_commit':
      return {
        type: 'command',
        command: action.command || 'git commit -m "feat: initial skillhub workspace"',
        label: 'Copy command',
      };
    case 'review_git_changes':
      return {
        type: 'command',
        command: action.command || 'git status',
        label: 'Copy command',
      };
    case 'review_upstream_updates':
      return {
        type: 'link',
        to: '/skills?upstream=updates',
        label: 'Review updates →',
      };
    case 'track_upstream_skills':
      return {
        type: 'command',
        command: action.command || 'skillhub source backfill',
        label: 'Copy command',
      };
    case 'link_orphan_sources':
      return {
        type: 'link',
        to: '/sources',
        label: 'Open sources →',
      };
    default:
      return { type: 'none' };
  }
}
