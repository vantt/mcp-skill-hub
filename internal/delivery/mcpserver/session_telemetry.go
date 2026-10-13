package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/telemetry"
)

// toolSession returns the session of a tool call; a nil request has none.
func toolSession(req *mcp.CallToolRequest) *mcp.ServerSession {
	if req == nil {
		return nil
	}
	return req.Session
}

func (adapter *Server) clientForReq(req mcp.Request) telemetry.Client {
	var session *mcp.ServerSession
	if req != nil {
		session, _ = req.GetSession().(*mcp.ServerSession)
	}
	return adapter.sessionClient(session)
}

// sessionClient names the host of a session for telemetry: the normalized
// clientInfo from the initialize handshake ("other" when absent), labelled
// remote when the session came through the network listener.
func (adapter *Server) sessionClient(session *mcp.ServerSession) telemetry.Client {
	client := telemetry.Client{Name: "other"}
	if session != nil {
		if params := session.InitializeParams(); params != nil && params.ClientInfo != nil {
			client = NormalizeClient(params.ClientInfo.Name, params.ClientInfo.Version)
		}
	}
	if adapter.remote {
		client = telemetry.RemoteClient(client)
	}
	return client
}

func (adapter *Server) telemetryMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		result, err := next(ctx, method, req)
		if err == nil && method == "tools/list" {
			if size, ok := adapter.toolsListBytes(result); ok {
				app.RecordServerMetric(ctx, adapter.telemetry, adapter.workspace, app.ServerMetric{Name: "tools_list_bytes", Value: float64(size), Client: adapter.clientForReq(req)})
			}
		}
		return result, err
	}
}

// toolsListBytes reports the encoded size of a tools/list result. The server
// returns the same registered tool objects until the tool set changes, so the
// result is encoded once per tool set and the size is reused for later calls.
func (adapter *Server) toolsListBytes(result mcp.Result) (int, bool) {
	var key strings.Builder
	if listed, ok := result.(*mcp.ListToolsResult); ok {
		for _, tool := range listed.Tools {
			fmt.Fprintf(&key, "%p;", tool)
		}
		key.WriteString(listed.NextCursor)
	}
	adapter.toolsListMu.Lock()
	defer adapter.toolsListMu.Unlock()
	if key.Len() > 0 && adapter.toolsListKey == key.String() {
		return adapter.toolsListSize, true
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return 0, false
	}
	adapter.toolsListKey, adapter.toolsListSize = key.String(), len(encoded)
	return len(encoded), true
}

func (adapter *Server) callerContext(session *mcp.ServerSession) app.CallerContext {
	sessionHash := ""
	client := adapter.sessionClient(session)
	var verifier func(string) bool
	if adapter.tracker != nil && session != nil {
		sessionHash = adapter.tracker.sessionHash(session)
		verifier = func(resID string) bool {
			return adapter.tracker.hasResolution(session, resID)
		}
	}
	return app.CallerContext{
		SessionHash:   sessionHash,
		Client:        client,
		PriorVerifier: verifier,
	}
}
