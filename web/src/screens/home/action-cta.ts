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
    case 'host_integration_missing':
      return {
        type: 'command',
        command: 'skillhub doctor --fix',
        label: 'Copy command',
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
    case 'review_lessons': {
      const skillId = action.id || '';
      return {
        type: 'link',
        to: skillId ? `/skills/${encodeURIComponent(skillId)}?tab=distill` : '/skills',
        label: 'Review lessons →',
      };
    }
    default:
      return { type: 'none' };
  }
}

// Plain-language help shown under the next action: what it means and why it matters.
// A title replaces the raw summary when the summary is an internal term.
export interface ActionHelp {
  title?: string;
  why: string;
}

const ACTION_HELP: Record<string, ActionHelp> = {
  host_integration_missing: {
    title: 'Your coding agents are not connected to this hub',
    why: 'The files that tell your coding agent (Claude Code, Codex and others) to ask Skill Hub for skills are missing, for example after a git pull. Until they are back, agents cannot use your skills. Run the command in a terminal; it restores them.',
  },
  repair_workspace: {
    why: 'The workspace files are not valid, so other work is paused. The command repairs what it safely can and lists the rest.',
  },
  recover_workspace: {
    why: 'A change stopped half way. The command finishes or rolls it back so nothing is left in between.',
  },
  rebuild_index: {
    why: 'The search index is out of date, so skill lists and counts may be wrong. The command rebuilds it from the skill files.',
  },
  review_git_changes: {
    why: 'Skill files changed and are not committed to Git yet. The web UI never commits; check the changes in a terminal and commit when they look right.',
  },
  first_run_commit: {
    why: 'This workspace is new. Commit it once so later changes can be reviewed and undone.',
  },
};

export function actionHelp(kind?: string): ActionHelp | undefined {
  return kind ? ACTION_HELP[kind] : undefined;
}
