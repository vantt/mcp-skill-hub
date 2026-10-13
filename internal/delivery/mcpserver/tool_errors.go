package mcpserver

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/skillhub-mcp/internal/app"
)

var (
	activeDiagnostics   atomic.Pointer[slog.Logger]
	correlationSequence atomic.Uint64
)

func success[T any](value T) (*mcp.CallToolResult, toolOutcome[T], error) {
	return &mcp.CallToolResult{}, toolOutcome[T]{SchemaVersion: SchemaVersion, Result: &value}, nil
}

func failure[T any](err error) (*mcp.CallToolResult, toolOutcome[T], error) {
	item := safeToolError(err)
	return &mcp.CallToolResult{IsError: true}, toolOutcome[T]{SchemaVersion: SchemaVersion, Error: &item}, nil
}

// appResult turns an application call into a tool outcome. A returned error is
// classified by safeToolError. A result that embeds an application error goes
// through resultToolError instead, which passes the application's own retry
// flag and wording through unchanged.
func appResult[T any](value T, err error) (*mcp.CallToolResult, toolOutcome[T], error) {
	if err != nil {
		return failure[T](err)
	}
	if appErr := app.ErrorOf(value, nil); appErr != nil {
		item := resultToolError(appErr)
		return &mcp.CallToolResult{IsError: true}, toolOutcome[T]{SchemaVersion: SchemaVersion, Error: &item}, nil
	}
	return success(value)
}

// safeToolError classifies a failure raised while serving a request. When the
// failure already is an application error, only the codes that a caller can
// cure by refreshing or waiting are retryable; any other failure keeps the
// retry flag of its classification.
func safeToolError(err error) toolError {
	var originalAppErr *app.Error
	isAppErr := errors.As(err, &originalAppErr) && originalAppErr != nil

	appErr := app.ClassifyError(err)
	if appErr == nil {
		return toolError{}
	}

	msg := appErr.Render.Error
	if msg == "" {
		msg = appErr.Error()
	}

	if appErr.Code == app.ErrorInternal {
		return internalToolError(msg)
	}

	action := appErr.Render.Fix
	if action == "" {
		action = "Check request parameters and retry."
	}

	retryable := appErr.Retryable
	if isAppErr {
		retryable = appErr.Code == app.ErrorStaleContext || appErr.Code == app.ErrorSnapshotExpired || appErr.Code == app.ErrorSourceUnavailable || appErr.Code == app.ErrorIndexStale
	}

	return toolError{
		Code:            string(appErr.Code),
		Message:         msg,
		Retryable:       retryable,
		SuggestedAction: action,
	}
}

// resultToolError maps an application error carried inside a successful call's
// result. An internal one is replaced by a generic message with a correlation
// ID; every other code keeps the application's wording and retry flag.
func resultToolError(appErr *app.Error) toolError {
	item := toolError{Code: string(appErr.Code), Message: appErr.Render.Error, Retryable: appErr.Retryable, SuggestedAction: appErr.Render.Fix}
	if appErr.Code == app.ErrorInternal {
		item.CorrelationID = logInternalFailure("application")
		item.Message = "The operation failed internally."
		item.Retryable = true
	}
	return item
}

func internalToolError(message string) toolError {
	return toolError{Code: "internal_error", Message: message, Retryable: true, SuggestedAction: "Retry once; if the failure persists, use the correlation ID with stderr diagnostics.", CorrelationID: logInternalFailure("tool")}
}

// logInternalFailure logs an internal failure without its details and returns
// the correlation ID that ties the logged line to the response.
func logInternalFailure(source string) string {
	correlation := correlationID()
	if logger := activeDiagnostics.Load(); logger != nil {
		logger.Error("MCP "+source+" internal failure", "correlation_id", correlation)
	}
	return correlation
}

func correlationID() string {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("fallback-%016x", correlationSequence.Add(1))
	}
	return hex.EncodeToString(value[:])
}
