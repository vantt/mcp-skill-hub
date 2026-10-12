package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/paging"
	"github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/skill"
)

const (
	SchemaVersion = "1"
)

type noArgs struct{}

type pageInput struct {
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum number of entries to return; defaults to 25 and cannot exceed 100."`
	Cursor string `json:"cursor,omitempty" jsonschema:"Opaque cursor returned by the previous page."`
}

type sourceCheckInput struct {
	SourceIDs []string `json:"source_ids,omitempty"`
	AllDue    bool     `json:"all_due,omitempty"`
}

type confirmationInput struct {
	ProposalID     string `json:"proposal_id"`
	ProposalDigest string `json:"proposal_digest"`
	BaseVersion    string `json:"base_version"`
}

type skillAddPreviewInput struct {
	Locator        string `json:"locator" jsonschema:"Public GitHub repository URL (e.g. https://github.com/owner/repo). Local filesystem paths are rejected."`
	Selection      string `json:"selection,omitempty" jsonschema:"Optional selected skill name or relative path when multiple skills exist in the repository."`
	All            bool   `json:"all,omitempty" jsonschema:"Import all discovered skills when multiple exist in the repository."`
	TargetID       string `json:"target_id,omitempty" jsonschema:"Optional explicit target skill ID (lowercase alphanumeric with dashes)."`
	Collection     string `json:"collection,omitempty" jsonschema:"Target collection name, defaults to 'default'."`
	IdempotencyKey string `json:"idempotency_key,omitempty" jsonschema:"Optional idempotency key."`
	FullDiff       bool   `json:"full_diff,omitempty" jsonschema:"Whether to return full unified diff instead of summary."`
}

type sourceWatchPreviewInput struct {
	Locator           string `json:"locator" jsonschema:"Public GitHub repository URL (e.g. https://github.com/owner/repo). Local folders are rejected."`
	SkillID           string `json:"skill_id,omitempty" jsonschema:"Skill identifier this watched source will attach to as a learning reference."`
	SourceID          string `json:"source_id,omitempty" jsonschema:"Optional explicit source identifier. Derived from repo/path if omitted."`
	Ref               string `json:"ref,omitempty" jsonschema:"Optional branch, tag, or commit ref to monitor. Defaults to default branch."`
	Path              string `json:"path,omitempty" jsonschema:"Optional subdirectory path within the repository to scope watching."`
	Cadence           string `json:"cadence,omitempty" jsonschema:"Monitoring cadence: manual, daily, or weekly. Defaults to daily when monitoring is enabled."`
	MonitoringEnabled *bool  `json:"monitoring_enabled,omitempty" jsonschema:"Whether background monitoring check is enabled."`
	Trust             string `json:"trust,omitempty" jsonschema:"Trust tier: untrusted, reviewed, or trusted. Defaults to untrusted."`
	License           string `json:"license,omitempty" jsonschema:"Optional license override if not automatically detected."`
	IdempotencyKey    string `json:"idempotency_key,omitempty" jsonschema:"Optional caller-provided idempotency key."`
}

type skillReviewInput struct {
	SkillID string `json:"skill_id" jsonschema:"Skill identifier to review."`
}

type sourceImportPreviewInput struct {
	SourceID       string   `json:"source_id" jsonschema:"Source identifier to import skills from."`
	Path           string   `json:"path,omitempty" jsonschema:"Optional subdirectory within the source repository to search for skills."`
	Skills         []string `json:"skills,omitempty" jsonschema:"Optional list of skill names or IDs to import. If omitted, all discovered skills are considered."`
	IdempotencyKey string   `json:"idempotency_key,omitempty" jsonschema:"Optional caller-provided idempotency key."`
}

type skillUpstreamStatusInput struct {
	SkillID string `json:"skill_id,omitempty" jsonschema:"Optional skill identifier to inspect. If omitted, returns upstream status for all tracked skills."`
}

type SkillUpstreamItem struct {
	SkillID           string                  `json:"skill_id"`
	SourceID          string                  `json:"source_id"`
	Repository        string                  `json:"repository"`
	Ref               string                  `json:"ref"`
	Path              string                  `json:"path"`
	BaseCommit        string                  `json:"base_commit"`
	LatestCommit      string                  `json:"latest_commit"`
	LatestCommittedAt string                  `json:"latest_committed_at,omitempty"`
	ChangedFiles      int                     `json:"changed_files"`
	Files             []app.SkillUpstreamFile `json:"files,omitempty"`
	Local             string                  `json:"local"`
	Status            string                  `json:"status"`
	CheckedAt         string                  `json:"checked_at"`
	Error             string                  `json:"error,omitempty"`
	NextAction        string                  `json:"next_action,omitempty"`
	WebUIHint         string                  `json:"webui_hint,omitempty"`
}

type UpstreamListResult struct {
	Skills []SkillUpstreamItem `json:"skills"`
}

type SkillUpstreamResult struct {
	Skill SkillUpstreamItem `json:"skill"`
}

type skillUpstreamStatusResult struct {
	Skill  *SkillUpstreamItem  `json:"skill,omitempty"`
	Skills []SkillUpstreamItem `json:"skills,omitempty"`
}

type skillUpdatePreviewInput struct {
	SkillID               string              `json:"skill_id"`
	Name                  *string             `json:"name,omitempty"`
	Description           *string             `json:"description,omitempty"`
	Content               *string             `json:"content,omitempty"`
	ExpectedContentDigest string              `json:"expected_content_digest,omitempty"`
	Routing               *skill.RoutingInput `json:"routing,omitempty"`
	Rationale             *string             `json:"rationale,omitempty"`
	Runtime               map[string]any      `json:"runtime,omitempty" jsonschema:"replacement runtime block {requires:{bins,env,platforms},setup:{check,command}}; an empty object removes the block; omit to keep it"`
	IdempotencyKey        string              `json:"idempotency_key,omitempty"`
	FullDiff              bool                `json:"full_diff,omitempty"`
}

type skillCreatePreviewInput struct {
	SkillID        string              `json:"skill_id"`
	Collection     string              `json:"collection,omitempty"`
	Name           string              `json:"name"`
	Description    string              `json:"description"`
	Content        *string             `json:"content,omitempty"`
	Routing        *skill.RoutingInput `json:"routing,omitempty"`
	Rationale      *string             `json:"rationale,omitempty"`
	IdempotencyKey string              `json:"idempotency_key,omitempty"`
	FullDiff       bool                `json:"full_diff,omitempty"`
}

type skillTransitionPreviewInput struct {
	SkillID        string `json:"skill_id"`
	Target         string `json:"target"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	FullDiff       bool   `json:"full_diff,omitempty"`
}

type skillListInput struct {
	State  string `json:"state,omitempty" jsonschema:"Optional lifecycle state to filter by (draft, active, deprecated, archived)."`
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum number of skills to return; defaults to 50 and cannot exceed 100."`
	Cursor string `json:"cursor,omitempty" jsonschema:"Opaque cursor returned by the previous page."`
}

type skillGetInput struct {
	SkillID string `json:"skill_id"`
}

// skillGetResult adds the MCP-only local snapshot to the runtime skill core.
// Content shadows the embedded field so it is omitted when the skill awaits
// content review.
type skillGetResult struct {
	app.SkillCore
	Content string          `json:"content,omitempty"`
	Local   *app.LocalSkill `json:"local,omitempty"`
}

type curationSessionRecordInput struct {
	SchemaVersion            string  `json:"schema_version"`
	EventID                  string  `json:"event_id"`
	Status                   string  `json:"status"`
	Basis                    string  `json:"basis"`
	TurnsToNextAction        *int64  `json:"turns_to_next_action,omitempty"`
	UnnecessaryConfirmations *int64  `json:"unnecessary_confirmations,omitempty"`
	PromptsPerBatch          *int64  `json:"prompts_per_batch,omitempty"`
	BatchSize                *int64  `json:"batch_size,omitempty"`
	AutoFinalized            *bool   `json:"auto_finalized,omitempty"`
	RecoveryCompleted        *bool   `json:"recovery_completed,omitempty"`
	RoutineGitNoise          *int64  `json:"routine_git_noise,omitempty"`
	DurationMS               *int64  `json:"duration_ms,omitempty"`
	ErrorCode                *string `json:"error_code,omitempty"`
}

type workspaceDiffInput struct {
	OperationID string `json:"operation_id,omitempty"`
	pageInput
}

type workspaceDiffResult struct {
	Kind      string                            `json:"kind"`
	GitDirty  bool                              `json:"git_dirty,omitempty"`
	Files     *paging.Page[app.DiffFile]        `json:"files,omitempty"`
	Operation *paging.Page[app.OperationChange] `json:"operation_changes,omitempty"`
}

type feedbackInput struct {
	SchemaVersion string  `json:"schema_version"`
	ResolutionID  string  `json:"resolution_id"`
	EventID       string  `json:"event_id"`
	Outcome       string  `json:"outcome"`
	ReasonCode    *string `json:"reason_code,omitempty"`
	SelectedSkill string  `json:"selected_skill,omitempty"`
	Utility       *string `json:"utility,omitempty"`
	Basis         *string `json:"basis,omitempty"`
}

type resolveInput resolver.Request

type toolError struct {
	Code            string `json:"code"`
	Message         string `json:"message"`
	Retryable       bool   `json:"retryable"`
	SuggestedAction string `json:"suggested_action"`
	CorrelationID   string `json:"correlation_id,omitempty"`
}

type toolOutcome[T any] struct {
	SchemaVersion string     `json:"schema_version"`
	Result        *T         `json:"result,omitempty"`
	Error         *toolError `json:"error,omitempty"`
}

type routingEvaluationResult struct {
	Resolution resolver.Response `json:"resolution"`
	Mutation   bool              `json:"mutation"`
}

type resolveResult struct {
	Resolution             resolver.Response `json:"resolution"`
	ActivationContext      *bool             `json:"activation_context_supported,omitempty"`
	Feedback               *bool             `json:"feedback_supported,omitempty"`
	SchemaMajor            string            `json:"schema_major"`
	ResourceVersionPinning bool              `json:"resource_version_pinning"`
}

// SEP-2640 wire DTOs stay local until the official Go SDK ships the extension.
type listSkillsParams struct {
	mcp.ParamsBase
	Cursor string `json:"cursor,omitempty"`
}

type getSkillParams struct {
	mcp.ParamsBase
	URI string `json:"uri"`
}

type skillEntry struct {
	URI         string                    `json:"uri"`
	Frontmatter map[string]any            `json:"frontmatter"`
	Resources   []app.DistributedResource `json:"resources"`
	Local       *app.LocalSkill           `json:"local,omitempty"`
}

type listSkillsResult struct {
	mcp.ResultBase
	ResultType string       `json:"resultType"`
	Skills     []skillEntry `json:"skills"`
	NextCursor string       `json:"nextCursor,omitempty"`
	TTLMS      int64        `json:"ttlMs"`
	CacheScope string       `json:"cacheScope"`
}

type getSkillResult struct {
	mcp.ResultBase
	ResultType string     `json:"resultType"`
	Skill      skillEntry `json:"skill"`
	TTLMS      int64      `json:"ttlMs"`
	CacheScope string     `json:"cacheScope"`
}
