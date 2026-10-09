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
  upstream_status?: string;
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
    missing_fields?: string[];
    warnings?: string[];
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
  provenance?: SkillProvenance;
  content_trust?: ContentTrust;
  runtime_hints?: RuntimeHints;
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

export type FunnelSince = '7d' | '30d' | '90d' | '180d';

export interface RateMetric {
  rate: number | null;
  numerator: number;
  denominator: number;
  status?: string;
}

export interface ChainMetrics {
  total_chains: number;
  chains_resolved: number;
  chains_no_skill: number;
  chains_needs_context: number;
  chains_already_covered: number;
  chains_failed: number;
  acceptance_rate: RateMetric;
  override_rate: RateMetric;
  false_no_skill_rate: RateMetric;
  true_no_skill: RateMetric;
  reformulation_rate: RateMetric;
  ignore_rate: RateMetric;
  needs_context_answer_rate: RateMetric;
  bypass_rate: RateMetric;
  negative_after_load: RateMetric;
  first_valid_day?: Record<string, string>;
}

export interface FunnelCut {
  key: string;
  metrics: ChainMetrics;
}

export interface DisagreementChain {
  chain_id: string;
  session_hash: string;
  resolution_id: string;
  event_id: string;
  occurred_at: string;
  kind: string;
  client: string;
  operation?: string;
  recommended_skill?: string;
  loaded_skill?: string;
  reason_codes?: string[];
}

export interface FunnelWindow {
  since: string;
  until: string;
  days: number;
}

export interface SkillFunnel {
  skill_id: string;
  name?: string;
  recommended_primary: number;
  recommended_supporting: number;
  activations: Record<string, number>;
  total_activations: number;
  acceptance_rate: number | null;
  overrides: number;
  misses: number;
  unsolicited: number;
  blocked_by_review: number;
  resolutions_review_required: number;
  resolutions_setup_required: number;
  loads: Record<string, number>;
  total_loads: number;
  doctor: Record<string, number>;
  total_doctor: number;
  doctor_failure_rate: number | null;
  setup_failed: number;
  setup_failed_rate: number | null;
  negative_feedback: number;
  negative_after_load: number;
  transcripts: Record<string, number>;
  chain_metrics?: ChainMetrics;
}

export interface FunnelReport {
  window: FunnelWindow;
  raw_retention_days: number;
  rollup_retention_days: number;
  metric_basis: Record<string, string>;
  skills?: SkillFunnel[];
  skill?: SkillFunnel;
  cuts?: FunnelCut[];
  dead_skills?: string[];
  recommended_never_activated?: string[];
  blocked_by_review?: string[];
  negative_after_load?: string[];
  setup_failures?: string[];
}

export interface ChangesSinceApproval {
  found: boolean;
  commit?: string;
  diff_command?: string;
  added?: string[];
  removed?: string[];
  modified?: string[];
  runtime_changed: boolean;
  scripts_changed: boolean;
  dependencies_changed: boolean;
  history_truncated: boolean;
}

export interface ContentTrust {
  third_party: boolean;
  approved: boolean;
  content_digest: string;
  reason_codes?: string[];
  approve_command?: string;
  changes_since_approval?: ChangesSinceApproval;
}

export interface RuntimeHints {
  interpreters: string[];
  dependency_manifests: string[];
  missing_lockfiles: string[];
  absolute_install_paths: string[];
  missing_runtime_block: boolean;
  install_prose_detected: boolean;
  install_cues: string[];
}

export interface SkillProvenance {
  created_by?: string;
  created_at?: string;
  source_id?: string;
  source_locator?: string;
  source_revision?: string;
  upstream_path?: string;
}

export interface RuntimeBin {
  name: string;
  version?: string;
}

export interface RuntimeSpec {
  requires?: {
    bins?: RuntimeBin[];
    env?: string[];
    platforms?: string[];
  };
  setup?: {
    command?: string;
    check?: string;
  };
}

export interface SetupStatus {
  state: string;
  reason_codes?: string[];
  basis?: string;
  checked_at?: string;
}

export interface DoctorCheck {
  kind: string;
  name: string;
  status: string;
  detail?: string;
}

export interface SkillRuntimeDoctor {
  state: string;
  basis: string;
  checked_at: string;
  checks: DoctorCheck[];
}

export interface SkillRuntimeStatus {
  schema_version: string;
  status: string;
  summary: string;
  skill_id: string;
  runtime: RuntimeSpec | null;
  served: boolean;
  setup?: SetupStatus;
  doctor?: SkillRuntimeDoctor;
  doctor_command: string;
  env_keys: string[];
  env_set_command: string;
  warnings?: Array<{ code: string; summary: string }>;
}

export interface SkillUpstreamFile {
  path: string;
  status: string;
}

export interface UpstreamResolution {
  path: string;
  action: string;
  content?: string;
}

export interface SkillUpstream {
  skill_id: string;
  source_id: string;
  repository: string;
  ref: string;
  path: string;
  base_commit: string;
  latest_commit: string;
  latest_committed_at?: string;
  changed_files: number;
  files?: SkillUpstreamFile[];
  local: string;
  status: string;
  checked_at: string;
  error?: string;
  next_action?: string;
}

export interface UpstreamListResult {
  schema_version: string;
  status: string;
  summary: string;
  skills: SkillUpstream[];
}

export interface UpstreamFile {
  path: string;
  status: string;
  default_action: string;
  action: string;
  conflicts: number;
  mergeable: boolean;
  upstream_diff?: string;
  local_diff?: string;
  result_diff?: string;
  merged_with_markers?: string;
}

export interface UpstreamTrustImpact {
  third_party: boolean;
  currently_approved: boolean;
  review_required_after_apply: boolean;
  review_command: string;
}

export interface UpstreamUpdatePreview {
  schema_version: string;
  status: string;
  summary: string;
  skill_id: string;
  source_id: string;
  base_commit: string;
  target_commit: string;
  target_committed_at?: string;
  base_available: boolean;
  files: UpstreamFile[];
  unchanged_count: number;
  unresolved: string[];
  diff: {
    added: string[];
    modified: string[];
    deleted: string[];
  };
  trust_impact: UpstreamTrustImpact;
  confirmation: {
    confirmation: {
      required: boolean;
      mode?: string;
      pins: ConfirmationPins;
    };
  };
}

export interface UpstreamUpdateResult {
  schema_version: string;
  status: string;
  summary: string;
  skill_id: string;
  operation_id: string;
  changed_paths: string[];
  trust_impact: UpstreamTrustImpact;
}

export interface LearningReference {
  source_id: string;
  locator: string;
  ref?: string;
  path?: string;
  role: string;
  monitoring: {
    enabled: boolean;
    cadence: string;
  };
  last_checked_at?: string;
  availability?: string;
  pending_insights: number;
}

export interface SkillSourcesResult {
  schema_version: string;
  status: string;
  summary: string;
  skill_id: string;
  upstream: SkillUpstream | null;
  learning: LearningReference[];
  pending_insights: number;
}

export interface SourceSummary {
  id: string;
  status: string;
  role: string;
  referencing_skills: string[];
  skills_vendored_count: number;
  importable_count: number;
  last_checked_at?: string;
  current_revision?: {
    kind: string;
    value: string;
    observed_at?: string;
  };
  distilled_revision?: {
    kind: string;
    value: string;
    observed_at?: string;
  };
  ready_to_distill?: boolean;
  upstream_only?: boolean;
}

export interface SourceRepositoryGroup {
  repository: string;
  sources: SourceSummary[];
}

export interface SourceCandidate {
  id: string;
  locator: string;
  status: string;
  reason: string;
  created_at: string;
}

export interface SourceRecord {
  id: string;
  adapter: string;
  locator: {
    repository?: string;
    ref?: string;
    path?: string;
    url?: string;
  };
  status: string;
  identity: {
    name: string;
    canonical?: string;
  };
  monitoring: {
    enabled: boolean;
    cadence: string;
  };
  current_revision?: {
    kind: string;
    value: string;
    observed_at?: string;
  };
  distilled_revision?: {
    kind: string;
    value: string;
    observed_at?: string;
  };
  purpose?: string;
}

export interface SourceListItem {
  record: SourceRecord;
  skills: string[];
  role: string;
  importable_count: number;
}

export interface SourceListResult {
  schema_version: string;
  status: string;
  summary: string;
  candidates: SourceCandidate[];
  sources: SourceListItem[];
  groups?: SourceRepositoryGroup[];
}

export interface SourceProposal {
  schema_version: string;
  status: string;
  summary: string;
  candidate_id?: string;
  source?: SourceRecord;
  diff?: {
    added: string[];
    modified: string[];
    deleted: string[];
  };
  confirmation: {
    confirmation: {
      required: boolean;
      mode?: string;
      pins: ConfirmationPins;
    };
  };
  warnings?: Array<{ code: string; summary: string }>;
}

export interface SourceMutationResult {
  schema_version: string;
  status: string;
  summary: string;
  operation_id?: string;
  source_id?: string;
}

export interface DiscoveredImportSkill {
  name: string;
  target_id: string;
  collection: string;
  description: string;
  path: string;
  conflict: boolean;
  imported?: boolean;
  skip_reason?: string;
}

export interface SourceImportProposal {
  schema_version: string;
  status: string;
  summary: string;
  source_id: string;
  revision: string;
  discovered: DiscoveredImportSkill[];
  importable: DiscoveredImportSkill[];
  skipped: DiscoveredImportSkill[];
  confirmation: {
    confirmation: {
      required: boolean;
      mode?: string;
      pins: ConfirmationPins;
    };
  };
}

export interface SourceCheckItem {
  source_id: string;
  status: string;
  error?: string;
  skills?: SkillUpstream[];
}

export interface SourceCheckResult {
  schema_version: string;
  status: string;
  summary: string;
  checked: number;
  changed: number;
  unavailable: number;
  results: SourceCheckItem[];
  warnings?: Array<{ code: string; summary: string }>;
}
export interface DistillGoal {
  status: 'draft' | 'confirmed' | string;
  purpose: string;
  in_scope: string[];
  out_of_scope: string[];
  failures_it_prevents: string[];
}

export interface DistillCoverageNotReadItem {
  path: string;
  reason: string;
}

export interface DistillCoverageSource {
  read?: string[];
  not_read?: DistillCoverageNotReadItem[];
}

export interface DistillScore {
  relevance: number;
  facts: string[];
  impact: number;
  evidence: number;
  effort: number;
  why: string;
}

export interface DistillDecision {
  state: 'candidate' | 'planned' | 'ported' | 'rejected' | string;
  reason?: string;
  at?: string;
}

export interface DistillLesson {
  key: string;
  layer: string;
  what: string;
  notable: string;
  where: string[];
  contrast: string;
  score: DistillScore;
  final_score?: number;
  also_fits?: string[];
  status?: string;
  found_by?: string;
  decision: DistillDecision;
}

export interface SkillDistillDocument {
  goal: DistillGoal;
  cursors: Record<string, string>;
  coverage?: Record<string, DistillCoverageSource>;
  lessons: DistillLesson[];
}


export interface DistillRun {
  id: string;
  source_id: string;
  state: 'prepared' | 'in_progress' | 'awaiting_decision' | 'failed' | 'finalized' | 'cancelled' | string;
  attempt: number;
  from_revision?: {
    kind: string;
    value: string;
    observed_at?: string;
  };
  to_revision: {
    kind: string;
    value: string;
    observed_at?: string;
  };
  changed_resources: Array<{
    path: string;
    status: string;
    before_digest?: string;
    after_digest?: string;
  }>;
  package_digest: string;
  prepared_at: string;
  started_at?: string;
  finalized_at?: string;
  cancelled_at?: string;
  failure?: string;
  finding_ids?: string[];
  comparison_ids?: string[];
  insight_ids?: string[];
  outstanding_decisions?: Array<{
    category: string;
    detail: string;
  }>;
}

export interface DistillRunResult {
  schema_version: string;
  status: string;
  summary: string;
  run: DistillRun;
  operation_id?: string;
}

export interface InsightRank {
  score: number;
  evidence_sources: number;
  evidence_findings: number;
  impact: string;
  stale: boolean;
}

export interface InsightInboxItem {
  insight: Insight;
  rank: InsightRank;
}

export interface InsightInboxGroup {
  skill_id: string;
  category: string;
  items: InsightInboxItem[];
}

export interface InboxPageResult {
  schema_version: string;
  status: string;
  summary: string;
  groups: InsightInboxGroup[];
  has_more: boolean;
  next_cursor?: string;
  total: number;
}

export interface Observation {
  schema_version: number;
  id: string;
  source_id: string;
  run_id: string;
  stable_key: string;
  status: 'active' | 'removed' | 'superseded' | string;
  what: string;
  vocabulary: string[];
  evidence: Array<{
    revision: {
      kind: string;
      value: string;
      content_digest: string;
    };
    run_id: string;
    package_digest: string;
    path: string;
    locator: string;
    digest: string;
  }>;
}

export interface Comparison {
  schema_version: number;
  id: string;
  run_id: string;
  subject: string;
  observation_ids: string[];
  verdict: string;
  tradeoffs: string;
  stale: boolean;
}

export interface Insight {
  schema_version: number;
  id: string;
  run_id: string;
  stable_key: string;
  skill_id: string;
  status: 'pending' | 'planned' | 'rejected' | 'obsolete' | 'withdrawn' | string;
  recommendation: string;
  observation_ids: string[];
  comparison_ids?: string[];
  category: string;
  priority: string;
  rationale: string;
  evidence_digest: string;
  decision_history?: Array<{
    decision: string;
    rationale: string;
    decided_at: string;
  }>;
}

export interface InsightDetailResult {
  schema_version: string;
  status: string;
  summary: string;
  insight: Insight;
  findings: Observation[];
  comparisons: Comparison[];
}

export interface InsightApplicationPreview {
  schema_version: string;
  status: string;
  summary: string;
  proposal_id: string;
  proposal_digest: string;
  base_catalog_version: string;
  insight_id: string;
  skill_id: string;
  path_pins: Array<{
    path: string;
    digest: string;
  }>;
  diff: string;
  confirmation: {
    confirmation: {
      required: boolean;
      mode?: string;
      pins: ConfirmationPins;
    };
  };
}

export interface InsightApplicationResult {
  schema_version: string;
  status: string;
  summary: string;
  operation_id?: string;
}

export interface InsightDecisionResult {
  schema_version: string;
  status: string;
  summary: string;
  insight: Insight;
  operation_id?: string;
}
