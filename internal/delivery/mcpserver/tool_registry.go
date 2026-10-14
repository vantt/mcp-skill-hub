package mcpserver

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolRegistry collects tool definitions, then either lists them as they are
// or installs them on one server.
//
// The SDK resolves both schemas of a typed tool when the tool is added, which
// costs most of the server's startup and allocates megabytes for tools a
// session may never call. Installing therefore lists every tool at once from
// its derived schema, with a placeholder handler, and adds the typed tool, which
// resolves its schemas, only when the first call for that tool arrives. What a
// client sees in tools/list, and how a call is validated and answered, does not
// change.
type toolRegistry struct {
	server    *mcp.Server // nil until installed
	pending   map[string]*pendingTool
	collected []collectedTool
}

// collectedTool is a tool whose schemas are derived and whose typed
// registration has not run.
type collectedTool struct {
	tool     *mcp.Tool
	register func(*mcp.Server)
}

// pendingTool is a listed tool whose typed registration has not run yet.
type pendingTool struct {
	once     sync.Once
	register func(*mcp.Server)
}

// newToolCollector returns a registry that derives tool definitions and
// registers nothing until it is installed.
func newToolCollector() *toolRegistry {
	return &toolRegistry{}
}

// names returns the collected tool names in the order a server lists them.
func (registry *toolRegistry) names() []string {
	definitions := registry.definitions()
	names := make([]string, len(definitions))
	for i, tool := range definitions {
		names[i] = tool.Name
	}
	return names
}

// definitions returns the collected tools in the order a server lists them.
func (registry *toolRegistry) definitions() []*mcp.Tool {
	tools := make([]*mcp.Tool, len(registry.collected))
	for i, collected := range registry.collected {
		tools[i] = collected.tool
	}
	slices.SortFunc(tools, func(a, b *mcp.Tool) int { return strings.Compare(a.Name, b.Name) })
	return tools
}

// add records a tool whose schemas are already derived.
func (registry *toolRegistry) add(tool *mcp.Tool, register func(*mcp.Server)) {
	registry.collected = append(registry.collected, collectedTool{tool: tool, register: register})
}

// install lists every collected tool on server with a placeholder handler and
// registers each typed tool when its first call arrives.
func (registry *toolRegistry) install(server *mcp.Server) {
	registry.server = server
	registry.pending = map[string]*pendingTool{}
	server.AddReceivingMiddleware(registry.registerOnFirstCall)
	for _, collected := range registry.collected {
		registry.pending[collected.tool.Name] = &pendingTool{register: collected.register}
		server.AddTool(collected.tool, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return nil, &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: "The tool is not ready."}
		})
	}
}

// materialize adds the typed tool for name if it is still only listed.
func (registry *toolRegistry) materialize(name string) {
	if tool := registry.pending[name]; tool != nil {
		tool.once.Do(func() { tool.register(registry.server) })
	}
}

// materializeAll adds every listed tool as its typed form. It fails with the
// SDK's own error when a tool's schemas cannot be resolved, which would
// otherwise surface at the first call.
func (registry *toolRegistry) materializeAll() (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.New("tool registration failed: " + describePanic(recovered))
		}
	}()
	for name := range registry.pending {
		registry.materialize(name)
	}
	return nil
}

func describePanic(recovered any) string {
	if err, ok := recovered.(error); ok {
		return err.Error()
	}
	if text, ok := recovered.(string); ok {
		return text
	}
	return "unknown panic"
}

// registerOnFirstCall adds the typed tool before the server looks the tool up.
func (registry *toolRegistry) registerOnFirstCall(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if method == "tools/call" {
			if call, ok := req.(*mcp.CallToolRequest); ok && call.Params != nil {
				registry.materialize(call.Params.Name)
			}
		}
		return next(ctx, method, req)
	}
}
