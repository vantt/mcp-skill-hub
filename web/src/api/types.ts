export interface SessionResponse {
  api_version: number;
  skillhub_version: string;
  workspace_id: string;
  workspace_name: string;
}

export interface WorkspaceHealth {
  health: 'valid' | 'invalid' | 'recovery_required';
  index: 'current' | 'stale';
  git_dirty: boolean;
  git_configured: boolean;
  recovery_pending: boolean;
}

export interface HomeAction {
  kind: string;
  count: number;
  priority: number;
  summary: string;
  command?: string;
  id?: string;
  label?: string;
}

export interface HomeCategory {
  kind: string;
  count: number;
  availability: 'available' | 'not_configured' | 'unavailable';
}

export interface CurationHome {
  schema_version: string;
  status: string;
  summary: string;
  workspace: WorkspaceHealth;
  actions: HomeAction[];
  categories: HomeCategory[];
}

export type SkillLifecycleState = 'draft' | 'active' | 'deprecated' | 'archived';

export interface SkillListItem {
  id: string;
  state: string;
  lifecycle_state: SkillLifecycleState;
  active_locally: boolean;
  routing_eligible: boolean;
  collection: string;
  name: string;
}

export interface SkillListResult {
  schema_version: string;
  status: string;
  summary: string;
  skills: SkillListItem[];
}

export interface SkillResource {
  path: string;
  kind: string;
  digest: string;
  size_bytes: number;
}

export interface SkillRouting {
  operations: string[];
  triggers: string[];
  not_for: string[];
  min_scope: string;
}

export interface SkillDetail {
  skill_id: string;
  name: string;
  description: string;
  status: string;
  path: string;
  catalog_snapshot: string;
  content: string;
  content_digest: string;
  state_basis: string;
  lifecycle_state: SkillLifecycleState;
  routing_eligible: boolean;
  diverged: boolean;
  routing: SkillRouting;
  resources: SkillResource[];
}

export interface SkillReviewResult {
  schema_version: string;
  status: string;
  summary: string;
  skill_id: string;
  collection: string;
  name: string;
  description: string;
  lifecycle_state: SkillLifecycleState;
  active_locally: boolean;
  routing_eligible: boolean;
  valid: boolean;
  activation_readiness: {
    ready: boolean;
    untouched_scaffold: boolean;
  };
  resource_status: {
    entrypoint_path: string;
    entrypoint_digest: string;
    resource_count: number;
    total_bytes: number;
    resources: SkillResource[];
  };
  diverged: boolean;
  next_action: string;
}

export interface ConfirmationPins {
  proposal_id: string;
  proposal_digest: string;
  base_version: string;
}

export interface SkillProposal {
  schema_version: string;
  status: string;
  summary: string;
  skill_id?: string;
  from_state?: string;
  to_state?: string;
  paths?: string[];
  impact?: string;
  warning?: string;
  diff?: string;
  stat?: string;
  confirmation: {
    policy_revision?: string;
    action_class?: string;
    application_command?: string;
    confirmation?: {
      required: boolean;
      mode?: string;
      pins: ConfirmationPins;
    };
    required?: boolean;
    pins?: ConfirmationPins;
  };
}

export interface SkillMutationResult {
  schema_version: string;
  status: string;
  summary: string;
  operation_id: string;
  skill_id?: string;
}

export interface SkillAddProposal {
  schema_version: string;
  status: string;
  summary: string;
  candidate_id?: string;
  confirmation: {
    confirmation?: {
      pins: ConfirmationPins;
    };
    pins?: ConfirmationPins;
  };
  diff?: string;
}

export interface SkillAddResult {
  schema_version: string;
  status: string;
  summary: string;
  operation_id: string;
  added_skills?: string[];
}
