package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

// ClassifyError classifies an arbitrary error into a structured *Error.
// It is the single source of truth for error classification across delivery adapters.
func ClassifyError(err error) *Error {
	if err == nil {
		return nil
	}
	var appErr *Error
	if errors.As(err, &appErr) && appErr != nil {
		return appErr
	}
	var missingActivationErr *MissingActivationRequirementsError
	if errors.As(err, &missingActivationErr) {
		return &Error{
			Code:      ErrorInvalidRequest,
			Retryable: false,
			Render: ErrorRender{
				Error: missingActivationErr.Error(),
				Fix:   fmt.Sprintf("Run skill_update_preview to configure the missing fields (%s) before activating.", strings.Join(missingActivationErr.Missing, ", ")),
			},
		}
	}
	if strings.Contains(err.Error(), "does not exist; use skill_create_preview") {
		return &Error{
			Code:      ErrorInvalidRequest,
			Retryable: false,
			Render: ErrorRender{
				Error: err.Error(),
				Fix:   "Use skill_create_preview to create a new draft skill.",
			},
		}
	}
	var conflictErr *skill.EditConflictError
	if errors.As(err, &conflictErr) || errors.Is(err, skill.ErrEditConflict) {
		msg := "A concurrent edit conflict occurred on the skill content."
		if conflictErr != nil {
			msg = conflictErr.Error()
		}
		return &Error{
			Code:      ErrorEditConflict,
			Retryable: false,
			Render: ErrorRender{
				Error: msg,
				Fix:   "Read the latest skill content and digest via skill_get, then retry skill_update_preview with the new expected_content_digest.",
			},
		}
	}
	if errors.Is(err, context.Canceled) {
		return &Error{
			Code:      ErrorOperationCancelled,
			Retryable: true,
			Render: ErrorRender{
				Error: "The operation was cancelled before completion.",
				Fix:   "Retry when ready.",
			},
		}
	}
	if errors.Is(err, telemetry.ErrFeedbackResolutionNotFound) {
		return &Error{
			Code:      ErrorUnknownResolution,
			Retryable: true,
			Render: ErrorRender{
				Error: "The prior resolution is unavailable.",
				Fix:   "Start a new resolution request.",
			},
		}
	}
	if errors.Is(err, telemetry.ErrFeedbackConflict) {
		return &Error{
			Code:      ErrorInvalidRequest,
			Retryable: false,
			Render: ErrorRender{
				Error: "The event ID conflicts with existing feedback.",
				Fix:   "Retry with the original feedback or use a new event_id.",
			},
		}
	}
	if errors.Is(err, telemetry.ErrCurationSessionConflict) {
		return &Error{
			Code:      ErrorInvalidRequest,
			Retryable: false,
			Render: ErrorRender{
				Error: "The event ID conflicts with existing curation measurements.",
				Fix:   "Retry with the original measurements or use a new event_id.",
			},
		}
	}
	if errors.Is(err, skill.ErrSnapshotExpired) {
		return &Error{
			Code:      ErrorSnapshotExpired,
			Retryable: true,
			Render: ErrorRender{
				Error: "The pinned snapshot is no longer available.",
				Fix:   "Resolve or list again before retrying.",
			},
		}
	}
	if errors.Is(err, skill.ErrResourceDigestMismatch) {
		return &Error{
			Code:      ErrorResourceDigestMismatch,
			Retryable: false,
			Render: ErrorRender{
				Error: "Resource integrity verification failed; no bytes were used.",
				Fix:   "Validate and rebuild the catalog before retrying.",
			},
		}
	}
	if errors.Is(err, skill.ErrNotFound) || errors.Is(err, skill.ErrAlreadyExists) || errors.Is(err, skill.ErrInvalidTransition) || errors.Is(err, mutation.ErrIdempotencyConflict) {
		return &Error{
			Code:      ErrorInvalidRequest,
			Retryable: false,
			Render: ErrorRender{
				Error: "The request conflicts with the current object state.",
				Fix:   "Refresh the object state, correct the request, and retry.",
			},
		}
	}
	if errors.Is(err, mutation.ErrConflict) {
		return &Error{
			Code:      ErrorStaleContext,
			Retryable: true,
			Render: ErrorRender{
				Error: "The request was based on stale canonical state.",
				Fix:   "Refresh the object and regenerate the operation before retrying.",
			},
		}
	}
	if errors.Is(err, mutation.ErrRecoveryRequired) {
		return &Error{
			Code:      ErrorRecoveryRequired,
			Retryable: true,
			Render: ErrorRender{
				Error: "Workspace recovery is required before another mutation.",
				Fix:   "Run workspace validation and recovery before retrying.",
			},
		}
	}
	if errors.Is(err, mutation.ErrWorkspaceBusy) {
		return &Error{
			Code:      ErrorInternal,
			Retryable: true,
			Render: ErrorRender{
				Error: "The workspace lock could not be acquired.",
				Fix:   "Retry once; if the failure persists, use the correlation ID with stderr diagnostics.",
			},
		}
	}
	if errors.Is(err, os.ErrPermission) {
		return &Error{
			Code:      ErrorPermissionDenied,
			Retryable: false,
			Render: ErrorRender{
				Error: "The operation is not permitted.",
				Fix:   "Correct workspace permissions or host policy before retrying.",
			},
		}
	}
	if errors.Is(err, sourcepkg.ErrInvalidLocator) || errors.Is(err, sourcepkg.ErrUnsafeAddress) || errors.Is(err, sourcepkg.ErrLimitExceeded) {
		return &Error{
			Code:      ErrorInvalidRequest,
			Retryable: false,
			Render: ErrorRender{
				Error: "The request violates a source locator or resource limit.",
				Fix:   "Correct the bounded source request and retry.",
			},
		}
	}
	if errors.Is(err, sourcepkg.ErrRevisionMismatch) || errors.Is(err, sourcepkg.ErrHistoryUnavailable) {
		return &Error{
			Code:      ErrorSourceUnavailable,
			Retryable: true,
			Render: ErrorRender{
				Error: "The requested source revision is unavailable.",
				Fix:   "Refresh the source revision and retry.",
			},
		}
	}
	if errors.Is(err, catalog.ErrCatalogUnavailable) {
		return &Error{
			Code:      ErrorIndexStale,
			Retryable: true,
			Render: ErrorRender{
				Error: "The derived catalog is unavailable or stale.",
				Fix:   "Run workspace_rebuild, then retry.",
			},
		}
	}

	// Substring rules: these exist only for errors that are not yet typed; new code must return *Error or a sentinel.
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "snapshot_expired") || strings.Contains(message, "cursor is invalid or expired") {
		return &Error{
			Code:      ErrorSnapshotExpired,
			Retryable: true,
			Render: ErrorRender{
				Error: "The pinned snapshot or cursor is no longer available.",
				Fix:   "Restart the list or resolution and retry with its new pins.",
			},
		}
	}
	if strings.Contains(message, "schema_version") || strings.Contains(message, "unsupported schema") {
		return &Error{
			Code:      ErrorUnsupportedSchema,
			Retryable: false,
			Render: ErrorRender{
				Error: "The requested schema major is not supported.",
				Fix:   "Use schema_version 1.",
			},
		}
	}
	if strings.Contains(message, "prior clarification was not issued") {
		return &Error{
			Code:      ErrorUnknownResolution,
			Retryable: true,
			Render: ErrorRender{
				Error: "The prior resolution is unavailable.",
				Fix:   "Start a new resolution request.",
			},
		}
	}
	if strings.Contains(message, "prior clarification is stale") || strings.Contains(message, "does not match this request") {
		return &Error{
			Code:      ErrorStaleContext,
			Retryable: true,
			Render: ErrorRender{
				Error: "The prior resolution no longer matches the current request.",
				Fix:   "Start a new resolution with the current context.",
			},
		}
	}
	if strings.Contains(message, "catalog is stale") || strings.Contains(message, "catalog is missing") || strings.Contains(message, "catalog is corrupt") {
		return &Error{
			Code:      ErrorIndexStale,
			Retryable: true,
			Render: ErrorRender{
				Error: "The derived catalog is unavailable or stale.",
				Fix:   "Run workspace_rebuild, then retry.",
			},
		}
	}
	if knownRequestMessage(message) {
		return &Error{
			Code:      ErrorInvalidRequest,
			Retryable: false,
			Render: ErrorRender{
				Error: "The request conflicts with validation rules or the current object state.",
				Fix:   "Inspect the tool schema and current object state, correct the request, and retry.",
			},
		}
	}

	return &Error{
		Code:      ErrorInternal,
		Retryable: true,
		Render: ErrorRender{
			Error: "The operation failed internally.",
			Fix:   "Retry once; if the failure persists, use the correlation ID with stderr diagnostics.",
		},
	}
}

func knownRequestMessage(message string) bool {
	for _, marker := range []string{
		" is required", " are required", " must ", " cannot ", " invalid", "not found", "does not exist",
		"already exists", "already used", "unsupported", "conflict", "stale proposal", "exceeds ",
		"not one of", "no changes", "unsafe source", "awaiting-decision", "confirmation pins",
	} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

// ErrorOf returns an application Error from either an error or a Result containing an Error.
func ErrorOf(value any, err error) *Error {
	if err != nil {
		return ClassifyError(err)
	}
	if appErrHolder, ok := value.(interface{ ApplicationError() *Error }); ok {
		return appErrHolder.ApplicationError()
	}
	return nil
}
