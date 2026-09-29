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
)

// Error is the structured, user-safe error member of a Result.
type Error struct {
	Code      ErrorCode   `json:"code"`
	Retryable bool        `json:"retryable,omitempty"`
	Render    ErrorRender `json:"render"`
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
