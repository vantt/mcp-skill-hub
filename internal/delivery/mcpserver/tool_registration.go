package mcpserver

import (
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (adapter *Server) registerTools(tools *toolRegistry) {
	switch adapter.profile {
	case ProfileRuntime:
		adapter.registerRuntimeTools(tools)
	case ProfileCuration:
		adapter.registerCurationTools(tools)
	default:
		adapter.registerAllTools(tools)
	}
}

func (adapter *Server) registerRuntimeTools(tools *toolRegistry) {
	adapter.registerResolverTools(tools)
	adapter.registerSkillGetTool(tools)
}

func (adapter *Server) registerCurationTools(tools *toolRegistry) {
	adapter.registerSourceTools(tools)
	adapter.registerSourceImportTools(tools)
	adapter.registerSourceWatchTools(tools)
	adapter.registerUpstreamTools(tools)
	adapter.registerInsightTools(tools)
	adapter.registerSkillCurationTools(tools)
	adapter.registerSkillAddTools(tools)
	adapter.registerSkillReviewTools(tools)
	adapter.registerWorkspaceTools(tools)
}

func (adapter *Server) registerAllTools(tools *toolRegistry) {
	adapter.registerRuntimeTools(tools)
	adapter.registerCurationTools(tools)
}

func addTool[In, Out any](tools *toolRegistry, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, toolOutcome[Out]], options ...toolOption) {
	var schemaOptions toolSchemaOptions
	for _, option := range options {
		option(&schemaOptions)
	}
	tool.InputSchema, tool.OutputSchema = deriveToolSchemas[In, Out](tool.Name, schemaOptions)
	tools.add(tool, func(server *mcp.Server) { mcp.AddTool(server, tool, handler) })
}

func annotations(readOnly, destructive, idempotent, openWorld bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{ReadOnlyHint: readOnly, DestructiveHint: &destructive, IdempotentHint: idempotent, OpenWorldHint: &openWorld}
}
