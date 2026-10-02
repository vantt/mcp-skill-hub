package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

// errorCharacterizationCase pins the public error code and retryable flag the
// MCP adapter reports for one failure shape. The table was captured from the
// adapter before error classification moved into the application layer and
// must keep passing unchanged afterwards.
type errorCharacterizationCase struct {
	name      string
	err       error
	code      string
	retryable bool
}

func errorCharacterizationCases() []errorCharacterizationCase {
	wrap := func(err error) error { return fmt.Errorf("context: %w", err) }
	return []errorCharacterizationCase{
		{"app error keeps its code", &app.Error{Code: app.ErrorStaleProposal, Render: app.ErrorRender{Error: "stale", Fix: "regenerate"}}, "stale_proposal", false},
		{"app source_unavailable is retryable", &app.Error{Code: app.ErrorSourceUnavailable, Render: app.ErrorRender{Error: "down"}}, "source_unavailable", true},
		{"app index_stale is retryable", &app.Error{Code: app.ErrorIndexStale, Render: app.ErrorRender{Error: "stale index"}}, "index_stale", true},
		{"app validation_failed passes through", &app.Error{Code: app.ErrorValidationFailed, Render: app.ErrorRender{Error: "invalid"}}, "validation_failed", false},
		{"missing activation requirements", &app.MissingActivationRequirementsError{SkillID: "demo", Missing: []string{"trigger"}}, "invalid_request", false},
		{"update of a missing skill", errors.New("skill demo does not exist; use skill_create_preview"), "invalid_request", false},
		{"typed edit conflict", &skill.EditConflictError{Path: "skills/core/demo/SKILL.md", ExpectedDigest: "sha256:a", ActualDigest: "sha256:b"}, "edit_conflict", false},
		{"edit conflict sentinel", wrap(skill.ErrEditConflict), "edit_conflict", false},
		{"context cancelled", wrap(context.Canceled), "operation_cancelled", true},
		{"feedback resolution not found", wrap(telemetry.ErrFeedbackResolutionNotFound), "unknown_resolution", true},
		{"feedback conflict", wrap(telemetry.ErrFeedbackConflict), "invalid_request", false},
		{"curation session conflict", wrap(telemetry.ErrCurationSessionConflict), "invalid_request", false},
		{"skill snapshot expired", wrap(skill.ErrSnapshotExpired), "snapshot_expired", true},
		{"resource digest mismatch", wrap(skill.ErrResourceDigestMismatch), "resource_digest_mismatch", false},
		{"skill not found", wrap(skill.ErrNotFound), "invalid_request", false},
		{"skill already exists", wrap(skill.ErrAlreadyExists), "invalid_request", false},
		{"invalid lifecycle transition", wrap(skill.ErrInvalidTransition), "invalid_request", false},
		{"idempotency conflict", wrap(mutation.ErrIdempotencyConflict), "invalid_request", false},
		{"mutation precondition conflict", wrap(mutation.ErrConflict), "stale_context", true},
		{"mutation recovery required", wrap(mutation.ErrRecoveryRequired), "recovery_required", true},
		{"workspace busy", wrap(mutation.ErrWorkspaceBusy), "internal_error", true},
		{"permission denied", wrap(os.ErrPermission), "permission_denied", false},
		{"invalid source locator", wrap(sourcepkg.ErrInvalidLocator), "invalid_request", false},
		{"unsafe source address", wrap(sourcepkg.ErrUnsafeAddress), "invalid_request", false},
		{"source limit exceeded", wrap(sourcepkg.ErrLimitExceeded), "invalid_request", false},
		{"source revision mismatch", wrap(sourcepkg.ErrRevisionMismatch), "source_unavailable", true},
		{"source history unavailable", wrap(sourcepkg.ErrHistoryUnavailable), "source_unavailable", true},
		{"catalog unavailable", wrap(catalog.ErrCatalogUnavailable), "index_stale", true},
		{"snapshot expired text", errors.New("snapshot_expired: inbox cursor is invalid or expired"), "snapshot_expired", true},
		{"unsupported schema text", errors.New("unsupported schema_version 2"), "unsupported_schema", false},
		{"clarification not issued text", errors.New("prior clarification was not issued"), "unknown_resolution", true},
		{"clarification stale text", errors.New("prior clarification is stale"), "stale_context", true},
		{"catalog stale text", errors.New("catalog is stale"), "index_stale", true},
		{"required field text", errors.New("skill_id is required"), "invalid_request", false},
		{"must text", errors.New("limit must be between 1 and 100"), "invalid_request", false},
		{"not found text", errors.New("comparison not found"), "invalid_request", false},
		{"are required text", errors.New("source_ids and decision are required"), "invalid_request", false},
		{"cannot text", errors.New("run in state finalized cannot transition to in_progress"), "invalid_request", false},
		{"invalid text", errors.New("proposal digest is invalid"), "invalid_request", false},
		{"does not exist text", errors.New("insight comparison CMP-1 does not exist"), "invalid_request", false},
		{"already exists text", errors.New("source demo already exists"), "invalid_request", false},
		{"already used text", errors.New("event_id was already used"), "invalid_request", false},
		{"unsupported text", errors.New("unsupported adapter kind"), "invalid_request", false},
		{"conflict text", errors.New("path conflict detected"), "invalid_request", false},
		{"stale proposal text", errors.New("stale proposal for demo"), "invalid_request", false},
		{"exceeds text", errors.New("payload exceeds the limit"), "invalid_request", false},
		{"not one of text", errors.New("state is not one of draft, active"), "invalid_request", false},
		{"no changes text", errors.New("no changes to apply"), "invalid_request", false},
		{"unsafe source text", errors.New("unsafe source path"), "invalid_request", false},
		{"awaiting-decision text", errors.New("awaiting-decision retry requires an explicit decision or correction"), "invalid_request", false},
		{"confirmation pins text", errors.New("confirmation pins do not match"), "invalid_request", false},
		{"request mismatch text", errors.New("clarification token does not match this request"), "stale_context", true},
		{"catalog missing text", errors.New("catalog is missing"), "index_stale", true},
		{"catalog corrupt text", errors.New("catalog is corrupt"), "index_stale", true},
		{"unclassified error", errors.New("boom"), "internal_error", true},
	}
}

func TestSafeToolErrorCharacterization(t *testing.T) {
	for _, tc := range errorCharacterizationCases() {
		t.Run(tc.name, func(t *testing.T) {
			got := safeToolError(tc.err)
			if got.Code != tc.code || got.Retryable != tc.retryable {
				t.Fatalf("safeToolError(%v) = code %q retryable %v; want code %q retryable %v", tc.err, got.Code, got.Retryable, tc.code, tc.retryable)
			}
			if got.Message == "" {
				t.Fatalf("safeToolError(%v) returned an empty message", tc.err)
			}
			if got.Code == "internal_error" && got.CorrelationID == "" {
				t.Fatalf("internal_error for %v has no correlation ID", tc.err)
			}
		})
	}
}
