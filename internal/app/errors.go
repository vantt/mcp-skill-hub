package app

// ErrorCode is a stable, machine-readable application error code.
type ErrorCode string

const (
	ErrorInvalidRequest               ErrorCode = "invalid_request"
	ErrorUnsupportedSchema            ErrorCode = "unsupported_schema"
	ErrorStaleContext                 ErrorCode = "stale_context"
	ErrorUnknownResolution            ErrorCode = "unknown_resolution"
	ErrorClarificationBudgetExhausted ErrorCode = "clarification_budget_exhausted"
	ErrorResolutionRetryExhausted     ErrorCode = "resolution_retry_exhausted"
	ErrorIndexStale                   ErrorCode = "index_stale"
	ErrorSnapshotExpired              ErrorCode = "snapshot_expired"
	ErrorResourceDigestMismatch       ErrorCode = "resource_digest_mismatch"
	ErrorPermissionDenied             ErrorCode = "permission_denied"
	ErrorInternal                     ErrorCode = "internal_error"
	ErrorWorkspaceInvalid             ErrorCode = "workspace_invalid"
	ErrorRecoveryRequired             ErrorCode = "recovery_required"
	ErrorRunInterrupted               ErrorCode = "run_interrupted"
	ErrorPartialDistillFailure        ErrorCode = "partial_distill_failure"
	ErrorSourceUnavailable            ErrorCode = "source_unavailable"
	ErrorStaleProposal                ErrorCode = "stale_proposal"
	ErrorResourceReadFailed           ErrorCode = "resource_read_failed"
	ErrorOperationCancelled           ErrorCode = "operation_cancelled"
	ErrorAmbiguousLocator             ErrorCode = "ambiguous_locator"
	ErrorAmbiguousRef                 ErrorCode = "ambiguous_ref"
	ErrorSkillSelectionRequired       ErrorCode = "skill_selection_required"
	ErrorSkillConflict                ErrorCode = "skill_conflict"
	ErrorSourceConflict               ErrorCode = "source_conflict"
	ErrorResourceLimitsExceeded       ErrorCode = "resource_limits_exceeded"
	ErrorSourceChanged                ErrorCode = "source_changed"
	ErrorEditConflict                 ErrorCode = "edit_conflict"
	ErrorValidationFailed             ErrorCode = "validation_failed"
	ErrorLocalWatchUnsupported        ErrorCode = "local_watch_unsupported"
	ErrorResourceContentUnavailable   ErrorCode = "resource_content_unavailable"
)

// Error is the structured, user-safe error member of a Result.
type Error struct {
	Code      ErrorCode   `json:"code"`
	Retryable bool        `json:"retryable,omitempty"`
	Render    ErrorRender `json:"render"`
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Render.Why != "" {
		return e.Render.Error + ": " + e.Render.Why
	}
	return e.Render.Error
}

// ErrorRender is the required human-readable rendering of an Error.
type ErrorRender struct {
	Error string `json:"ERROR"`
	Why   string `json:"WHY"`
	Fix   string `json:"FIX"`
}

// NewInvalidRequestError returns a safe error for invalid command-line input.
func NewInvalidRequestError(reason, fix string) *Error {
	return &Error{
		Code: ErrorInvalidRequest,
		Render: ErrorRender{
			Error: "The request cannot be accepted.",
			Why:   reason,
			Fix:   fix,
		},
	}
}

// NewRecoveryRequiredError keeps interrupted canonical writes actionable.
func NewRecoveryRequiredError(reason string) *Error {
	return &Error{Code: ErrorRecoveryRequired, Render: ErrorRender{
		Error: "Workspace recovery is required.",
		Why:   reason,
		Fix:   "Review `skillhub doctor --workspace <path>`, then run `skillhub doctor --fix --workspace <path> --yes` to roll forward safely.",
	}}
}

// NewSnapshotExpiredError instructs callers to resolve/read against the active generation.
func NewSnapshotExpiredError(reason string) *Error {
	return &Error{Code: ErrorSnapshotExpired, Retryable: true, Render: ErrorRender{
		Error: "The pinned catalog snapshot is no longer available.",
		Why:   reason,
		Fix:   "Resolve or read the skill again from the active catalog snapshot.",
	}}
}

// NewResourceDigestMismatchError rejects bytes that differ from the published manifest.
func NewResourceDigestMismatchError(reason string) *Error {
	return &Error{Code: ErrorResourceDigestMismatch, Retryable: true, Render: ErrorRender{
		Error: "The resource no longer matches its manifest digest.",
		Why:   reason,
		Fix:   "Rebuild the catalog if the edit is valid, then fetch a new manifest.",
	}}
}

// NewOperationCancelledError reports cancellation before a safe publication boundary.
func NewOperationCancelledError() *Error {
	return &Error{Code: ErrorOperationCancelled, Retryable: true, Render: ErrorRender{
		Error: "The operation was cancelled.",
		Why:   "Cancellation was received before publication completed.",
		Fix:   "Retry the operation when ready; no new catalog generation was published.",
	}}
}

// ErrorResult returns a correctly shaped error result.
func ErrorResult(err *Error) Result {
	result := NewResult(StatusError, err.Render.Error)
	result.Error = err
	return result
}

// NewAmbiguousLocatorError reports multiple matches for a locator.
func NewAmbiguousLocatorError(reason, fix string) *Error {
	return &Error{
		Code: ErrorAmbiguousLocator,
		Render: ErrorRender{
			Error: "The locator is ambiguous.",
			Why:   reason,
			Fix:   fix,
		},
	}
}

// NewAmbiguousRefError reports multiple matches for a ref or revision.
func NewAmbiguousRefError(reason, fix string) *Error {
	return &Error{
		Code: ErrorAmbiguousRef,
		Render: ErrorRender{
			Error: "The reference is ambiguous.",
			Why:   reason,
			Fix:   fix,
		},
	}
}

// NewSkillSelectionRequiredError reports that explicit skill selection is needed.
func NewSkillSelectionRequiredError(reason, fix string) *Error {
	return &Error{
		Code: ErrorSkillSelectionRequired,
		Render: ErrorRender{
			Error: "Skill selection is required.",
			Why:   reason,
			Fix:   fix,
		},
	}
}

// NewSkillConflictError reports an existing skill collision.
func NewSkillConflictError(reason, fix string) *Error {
	return &Error{
		Code: ErrorSkillConflict,
		Render: ErrorRender{
			Error: "Skill conflict detected.",
			Why:   reason,
			Fix:   fix,
		},
	}
}

// NewSourceConflictError reports an existing source collision.
func NewSourceConflictError(reason, fix string) *Error {
	return &Error{
		Code: ErrorSourceConflict,
		Render: ErrorRender{
			Error: "Source conflict detected.",
			Why:   reason,
			Fix:   fix,
		},
	}
}

// NewResourceLimitsExceededError reports file size or count limits exceeded.
func NewResourceLimitsExceededError(reason, fix string) *Error {
	return &Error{
		Code: ErrorResourceLimitsExceeded,
		Render: ErrorRender{
			Error: "Resource limits exceeded.",
			Why:   reason,
			Fix:   fix,
		},
	}
}

// NewSourceChangedError reports that upstream source changed.
func NewSourceChangedError(reason, fix string) *Error {
	return &Error{
		Code: ErrorSourceChanged,
		Render: ErrorRender{
			Error: "The source has changed.",
			Why:   reason,
			Fix:   fix,
		},
	}
}

// NewEditConflictError reports concurrent edit conflicts.
func NewEditConflictError(reason, fix string) *Error {
	return &Error{
		Code: ErrorEditConflict,
		Render: ErrorRender{
			Error: "Edit conflict detected.",
			Why:   reason,
			Fix:   fix,
		},
	}
}

// NewStaleProposalError reports that a proposal is stale.
func NewStaleProposalError(reason, fix string) *Error {
	return &Error{
		Code: ErrorStaleProposal,
		Render: ErrorRender{
			Error: "The proposal is stale.",
			Why:   reason,
			Fix:   fix,
		},
	}
}

// NewValidationFailedError reports validation failure.
func NewValidationFailedError(reason, fix string) *Error {
	return &Error{
		Code: ErrorValidationFailed,
		Render: ErrorRender{
			Error: "Validation failed.",
			Why:   reason,
			Fix:   fix,
		},
	}
}

// NewLocalWatchUnsupportedError reports that local watch is unsupported.
func NewLocalWatchUnsupportedError(reason, fix string) *Error {
	return &Error{
		Code: ErrorLocalWatchUnsupported,
		Render: ErrorRender{
			Error: "Local watch is unsupported.",
			Why:   reason,
			Fix:   fix,
		},
	}
}

// NewResourceContentUnavailableError reports that resource content is unavailable.
func NewResourceContentUnavailableError(reason, fix string) *Error {
	return &Error{
		Code: ErrorResourceContentUnavailable,
		Render: ErrorRender{
			Error: "Resource content is unavailable.",
			Why:   reason,
			Fix:   fix,
		},
	}
}
