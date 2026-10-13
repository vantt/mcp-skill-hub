package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/catalog"
	"github.com/vantt/skillhub-mcp/internal/skill"
)

func (adapter *Server) distributionRPCError(ctx context.Context, session *mcp.ServerSession, err error) error {
	switch {
	case errors.Is(err, app.ErrResourceContentUnavailable), errors.Is(err, skill.ErrResourceContentUnavailable):
		return adapter.correlatedInternalError("skill resource content unavailable", "resource_content_unavailable", "The requested skill resource has changed or is missing on disk.")
	case errors.Is(err, skill.ErrResourceDigestMismatch):
		return adapter.correlatedInternalError("skill resource integrity verification failed", "resource_digest_mismatch", "Skill resource integrity verification failed.")
	case errors.Is(err, catalog.ErrCatalogUnavailable):
		return invalidParams("index_stale", "The derived catalog is stale or unavailable; run workspace_rebuild and retry.")
	case errors.Is(err, skill.ErrNotFound):
		return invalidParams("skill_not_found", "The requested skill was not found.")
	case errors.Is(err, skill.ErrSnapshotExpired):
		app.RecordServerMetric(ctx, adapter.telemetry, adapter.workspace, app.ServerMetric{Name: "snapshot_expired_requests", Value: 1, Client: adapter.callerContext(session).Client})
		var expiredErr *app.SnapshotExpiredError
		if errors.As(err, &expiredErr) && expiredErr.CurrentURI != "" {
			data, _ := json.Marshal(map[string]string{
				"code":        "snapshot_expired",
				"current_uri": expiredErr.CurrentURI,
			})
			return &jsonrpc.Error{
				Code:    jsonrpc.CodeInvalidParams,
				Message: fmt.Sprintf("the skill was updated; call skills/get %s", expiredErr.CurrentURI),
				Data:    data,
			}
		}
		return invalidParams("snapshot_expired", "The requested skill snapshot is unavailable; refresh the skill entry.")
	default:
		correlation := correlationID()
		adapter.logger.Error("MCP distribution internal failure", "correlation_id", correlation)
		data, _ := json.Marshal(map[string]any{"code": "internal_error", "retryable": true, "correlation_id": correlation})
		return &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: "The skill resource could not be served.", Data: data}
	}
}

// correlatedInternalError logs a failure without its details and returns an
// internal error whose data carries the code and the correlation ID.
func (adapter *Server) correlatedInternalError(logMessage, code, message string) error {
	correlation := correlationID()
	adapter.logger.Error(logMessage, "correlation_id", correlation)
	data, _ := json.Marshal(map[string]string{"code": code, "correlation_id": correlation})
	return &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: message, Data: data}
}

func invalidParams(code, message string) error {
	data, _ := json.Marshal(map[string]string{"code": code})
	return &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: message, Data: data}
}
