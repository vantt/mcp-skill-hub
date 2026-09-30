package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
)

const (
	SchemaVersion = "1"
	defaultLimit  = 25
	maximumLimit  = 100
)

type noArgs struct{}

type pageInput struct {
	Limit  int    `json:"limit,omitempty" jsonschema:"Maximum number of entries to return; defaults to 25 and cannot exceed 100."`
	Cursor string `json:"cursor,omitempty" jsonschema:"Opaque cursor returned by the previous page."`
}

type sourceIntakeAddInput struct {
	Locator        string `json:"locator"`
	Reason         string `json:"reason"`
	IdempotencyKey string `json:"idempotency_key"`
}

type sourceIntakeListInput struct {
	Status string `json:"status,omitempty"`
	pageInput
}

type sourceTriageInput struct {
	CandidateID       string                `json:"candidate_id,omitempty"`
	Decision          string                `json:"decision,omitempty"`
	DecisionReason    string                `json:"decision_reason,omitempty"`
	SourceID          string                `json:"source_id,omitempty"`
	Adapter           string                `json:"adapter,omitempty"`
	Ref               string                `json:"ref,omitempty"`
	SourcePath        string                `json:"source_path,omitempty"`
	License           string                `json:"license,omitempty"`
	Trust             string                `json:"trust,omitempty"`
	Cadence           string                `json:"cadence,omitempty"`
	SkillID           string                `json:"skill_id,omitempty"`
	MonitoringEnabled *bool                 `json:"monitoring_enabled,omitempty"`
	IdempotencyKey    string                `json:"idempotency_key,omitempty"`
	Confirmation      *app.ConfirmationPins `json:"confirmation,omitempty"`
}

type sourceCheckInput struct {
	SourceIDs []string `json:"source_ids,omitempty"`
	AllDue    bool     `json:"all_due,omitempty"`
}

type curationRunStartInput struct {
	SourceIDs      []string `json:"source_ids,omitempty"`
	AllChanged     bool     `json:"all_changed,omitempty"`
	IdempotencyKey string   `json:"idempotency_key"`
}

type runIDInput struct {
	RunID string `json:"run_id"`
}

type curationRunRetryInput struct {
	RunID    string `json:"run_id"`
	Decision string `json:"decision,omitempty"`
}

type curationRunSubmitInput struct {
	RunID      string                `json:"run_id"`
	Submission app.DistillSubmission `json:"submission"`
}

type observationListInput struct {
	SourceID string `json:"source_id,omitempty"`
	pageInput
}

type comparisonGetInput struct {
	ComparisonID string `json:"comparison_id"`
}

type insightIDInput struct {
	InsightID string `json:"insight_id"`
}

type insightDecideInput struct {
	InsightID      string `json:"insight_id"`
	Decision       string `json:"decision"`
	Rationale      string `json:"rationale"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

type insightApplyPreviewInput struct {
	InsightID string                  `json:"insight_id"`
	Input     app.PreviewInsightInput `json:"input"`
}

type confirmationInput struct {
	ProposalID     string `json:"proposal_id"`
	ProposalDigest string `json:"proposal_digest"`
	BaseVersion    string `json:"base_version"`
}

type sourceImportPreviewInput struct {
	SourceID       string   `json:"source_id" jsonschema:"Source identifier to import skills from."`
	Path           string   `json:"path,omitempty" jsonschema:"Optional subdirectory within the source repository to search for skills."`
	Skills         []string `json:"skills,omitempty" jsonschema:"Optional list of skill names or IDs to import. If omitted, all discovered skills are considered."`
	IdempotencyKey string   `json:"idempotency_key,omitempty" jsonschema:"Optional caller-provided idempotency key."`
}

type skillUpdatePreviewInput struct {
	SkillID        string              `json:"skill_id"`
	Name           *string             `json:"name,omitempty"`
	Description    *string             `json:"description,omitempty"`
	Content        *string             `json:"content,omitempty"`
	Routing        *skill.RoutingInput `json:"routing,omitempty"`
	Rationale      *string             `json:"rationale,omitempty"`
	IdempotencyKey string              `json:"idempotency_key,omitempty"`
	FullDiff       bool                `json:"full_diff,omitempty"`
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
	State string `json:"state,omitempty"`
}

type skillGetInput struct {
	SkillID string `json:"skill_id"`
}

type skillGetResult struct {
	SkillID         string             `json:"skill_id"`
	Name            string             `json:"name"`
	Description     string             `json:"description"`
	Status          string             `json:"status"`
	Path            string             `json:"path"`
	CatalogSnapshot string             `json:"catalog_snapshot"`
	Content         string             `json:"content"`
	Routing         skill.RoutingInput `json:"routing"`
	Rationale       string             `json:"rationale,omitempty"`
	Resources       []skill.Resource   `json:"resources"`
}

type outcomeRecordInput struct {
	IncorporationID string   `json:"incorporation_id"`
	EventID         string   `json:"event_id"`
	State           string   `json:"state"`
	Evidence        []string `json:"evidence"`
	Note            string   `json:"note"`
	Supersedes      string   `json:"supersedes,omitempty"`
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
	Kind      string                     `json:"kind"`
	GitDirty  bool                       `json:"git_dirty,omitempty"`
	Files     *page[app.DiffFile]        `json:"files,omitempty"`
	Operation *page[app.OperationChange] `json:"operation_changes,omitempty"`
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

type page[T any] struct {
	Items      []T    `json:"items"`
	HasMore    bool   `json:"has_more"`
	NextCursor string `json:"next_cursor,omitempty"`
	Total      int    `json:"total,omitempty"`
}

type runStartItem struct {
	SourceID string                  `json:"source_id"`
	Prepared *app.DistillPrepareItem `json:"prepared,omitempty"`
	Started  *app.DistillRunResult   `json:"started,omitempty"`
	Error    *toolError              `json:"error,omitempty"`
}

type runStartResult struct {
	Prepared int            `json:"prepared"`
	Started  int            `json:"started"`
	Failed   int            `json:"failed"`
	Items    []runStartItem `json:"items"`
}

type routingEvaluationResult struct {
	Resolution resolver.Response `json:"resolution"`
	Mutation   bool              `json:"mutation"`
}

type sourceTriageResult struct {
	Preview  *app.SourceProposal       `json:"preview,omitempty"`
	Mutation *app.SourceMutationResult `json:"mutation,omitempty"`
}

type sourceListItem struct {
	Kind      string               `json:"kind"`
	Candidate *sourcepkg.Candidate `json:"candidate,omitempty"`
	Source    *sourcepkg.Record    `json:"source,omitempty"`
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
