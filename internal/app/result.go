// Package app defines application-layer contracts shared by delivery adapters.
package app

const ResultSchemaVersion = "1"

// Status is the stable outcome classification carried by every Result.
type Status string

const (
	StatusOK               Status = "ok"
	StatusReady            Status = "ready"
	StatusActionRequired   Status = "action_required"
	StatusApplied          Status = "applied"
	StatusRecoveryRequired Status = "recovery_required"
	StatusPartialFailure   Status = "partial_failure"
	StatusError            Status = "error"
)

// Result is the public response contract returned by all delivery adapters.
type Result struct {
	SchemaVersion    string         `json:"schema_version"`
	Status           Status         `json:"status"`
	Summary          string         `json:"summary"`
	Items            []Item         `json:"items"`
	SuggestedActions []Action       `json:"suggested_actions"`
	Warnings         []Warning      `json:"warnings"`
	Error            *Error         `json:"error"`
	Details          map[string]any `json:"details,omitempty"`
}

// NewResult creates a result with the fields that must always be present.
func NewResult(status Status, summary string) Result {
	return Result{
		SchemaVersion:    ResultSchemaVersion,
		Status:           status,
		Summary:          summary,
		Items:            []Item{},
		SuggestedActions: []Action{},
		Warnings:         []Warning{},
		Error:            nil,
	}
}

// Item is a compact summary of a result item.
type Item struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
	Impact  string `json:"impact"`
}

// Action is a safe next command that may be suggested to a caller.
type Action struct {
	Label                string `json:"label"`
	Command              string `json:"command"`
	RequiresConfirmation bool   `json:"requires_confirmation,omitempty"`
}

// Warning describes a non-fatal, actionable condition.
type Warning struct {
	Code    string `json:"code"`
	Summary string `json:"summary"`
}
