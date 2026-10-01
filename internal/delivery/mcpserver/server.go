// Package mcpserver exposes Skill Hub application services over MCP stdio.
package mcpserver

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/mutation"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
	contractschemas "github.com/vantt/mcp-skill-hub/schemas"
)

const (
	MaxFrameBytes = 4 << 20
	cacheTTLMS    = 300_000
)

type Server struct {
	workspace    string
	distribution app.DistributionService
	resolver     app.ResolverService
	feedback     app.FeedbackService
	source       app.SourceService
	distill      app.DistillService
	curationUX   app.CurationTelemetryService
	logger       *slog.Logger
}

var activeDiagnostics atomic.Pointer[slog.Logger]
var correlationSequence atomic.Uint64

var cursorMACKey = func() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic("initialize cursor integrity key: " + err.Error())
	}
	return key
}()

// New constructs one stateless server surface for a configured workspace.
func New(workspacePath string, diagnostics io.Writer) (*Server, *mcp.Server, error) {
	root, err := workspace.Discover(workspacePath)
	if err != nil {
		return nil, nil, err
	}
	if diagnostics == nil {
		diagnostics = io.Discard
	}
	logger := slog.New(slog.NewTextHandler(diagnostics, &slog.HandlerOptions{Level: slog.LevelWarn}))
	activeDiagnostics.Store(logger)
	adapter := &Server{workspace: root, logger: logger}
	capabilities := &mcp.ServerCapabilities{
		Resources: &mcp.ResourceCapabilities{},
		Tools:     &mcp.ToolCapabilities{},
	}
	capabilities.AddExtension("io.modelcontextprotocol/skills", map[string]any{"directoryRead": false})
	server := mcp.NewServer(&mcp.Implementation{Name: "skillhub", Version: "1"}, &mcp.ServerOptions{
		Capabilities: capabilities,
		PageSize:     maximumLimit,
		Logger:       logger,
		SetCacheable: func(_ context.Context, _ mcp.Request, cache *mcp.Cacheable) {
			cache.TTLMs = cacheTTLMS
			cache.CacheScope = "private"
		},
	})
	if err := adapter.registerSkills(server); err != nil {
		return nil, nil, err
	}
	adapter.registerTools(server)
	return adapter, server, nil
}

// Serve performs startup validation/rebuild before accepting stdio frames.
func Serve(ctx context.Context, workspacePath string, diagnostics io.Writer) error {
	if _, err := (app.CatalogService{}).EnsureCatalog(ctx, workspacePath); err != nil {
		if diagnostics == nil {
			diagnostics = os.Stderr
		}
		fmt.Fprintf(diagnostics, "warning: catalog ensure/rebuild failed on startup: %v\n", err)
	}
	adapter, server, err := New(workspacePath, diagnostics)
	if err != nil {
		return err
	}
	if recorder, telemetryErr := (app.TelemetryService{}).Open(adapter.workspace); telemetryErr != nil {
		adapter.logger.Warn("MCP telemetry recorder could not be opened")
	} else {
		adapter.resolver.Telemetry = recorder
		adapter.feedback.Recorder = recorder
		adapter.source.Telemetry = recorder
		adapter.distill.Telemetry = recorder
		adapter.curationUX.Recorder = recorder
		defer closeServeTelemetry(recorder, adapter.logger)
	}
	err = server.Run(ctx, &mcp.StdioTransport{MaxLineLength: MaxFrameBytes})
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func closeServeTelemetry(recorder *telemetry.Recorder, logger *slog.Logger) {
	flushContext, cancelFlush := context.WithTimeout(context.Background(), 10*time.Second)
	if err := recorder.Flush(flushContext); err != nil {
		logger.Warn("MCP telemetry recorder could not be flushed")
	}
	cancelFlush()

	closeContext, cancelClose := context.WithTimeout(context.Background(), 10*time.Second)
	if err := recorder.Close(closeContext); err != nil {
		logger.Warn("MCP telemetry recorder could not be closed")
	}
	cancelClose()

	if health := recorder.Health(); health.Errors > 0 {
		logger.Warn("MCP telemetry recorder reported storage failures", "error_count", health.Errors)
	}
}

func (adapter *Server) registerSkills(server *mcp.Server) error {
	if err := mcp.AddReceivingCustomMethod(server, "skills/list", adapter.listSkills); err != nil {
		return err
	}
	if err := mcp.AddReceivingCustomMethod(server, "skills/get", adapter.getSkill); err != nil {
		return err
	}
	server.AddResourceTemplate(&mcp.ResourceTemplate{
		URITemplate: "skill://skillhub/{manifest}/{name}/{+path}",
		Name:        "Skill Hub skill resource",
		Description: "A version-pinned file from a static Skill Hub skill manifest.",
	}, adapter.readResource)
	return nil
}

func (adapter *Server) listSkills(ctx context.Context, _ *mcp.ServerSession, params *listSkillsParams) (*listSkillsResult, error) {
	entries, snapshot, skipped, err := adapter.distribution.ListSkillsReport(ctx, adapter.workspace)
	if err != nil {
		return nil, adapter.distributionRPCError(err)
	}
	if status, inspectErr := (app.CatalogService{}).InspectCatalog(ctx, adapter.workspace); inspectErr == nil {
		if status.ServingMode == catalog.ServingFallback || status.Warning != "" {
			adapter.logger.Warn("serving fallback catalog generation", "warning", status.Warning)
		}
	}
	for _, item := range skipped {
		adapter.logger.Warn("skill omitted from listing because it cannot be served", "skill_id", item.SkillID, "reason", item.Reason)
	}
	filter := "skills"
	owner := pageOwner(filter, struct {
		Snapshot string                 `json:"snapshot"`
		Entries  []app.DistributedSkill `json:"entries"`
	}{Snapshot: snapshot, Entries: entries})
	lastKey, err := decodeCursor(params.Cursor, owner, filter)
	if err != nil {
		return nil, invalidParams("snapshot_expired", "The skill listing cursor is invalid or expired.")
	}
	paged, err := makePage(entries, defaultLimit, lastKey, owner, filter, func(entry app.DistributedSkill) string { return entry.SkillID })
	if err != nil {
		return nil, invalidParams("snapshot_expired", "The skill listing cursor is invalid or expired.")
	}
	result := &listSkillsResult{ResultType: "complete", Skills: []skillEntry{}, TTLMS: cacheTTLMS, CacheScope: "private", NextCursor: paged.NextCursor}
	for _, entry := range paged.Items {
		result.Skills = append(result.Skills, toSkillEntry(entry))
	}
	return result, nil
}

func (adapter *Server) getSkill(ctx context.Context, _ *mcp.ServerSession, params *getSkillParams) (*getSkillResult, error) {
	if params == nil || params.URI == "" || len(params.URI) > 4096 {
		return nil, invalidParams("snapshot_expired", "A bounded skill SKILL.md URI is required.")
	}
	entry, err := adapter.distribution.GetSkill(ctx, adapter.workspace, params.URI)
	if err != nil {
		return nil, adapter.distributionRPCError(err)
	}
	return &getSkillResult{ResultType: "complete", Skill: toSkillEntry(entry), TTLMS: cacheTTLMS, CacheScope: "private"}, nil
}

func (adapter *Server) readResource(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	if request == nil || request.Params == nil || len(request.Params.URI) > 4096 {
		return nil, invalidParams("snapshot_expired", "A bounded skill resource URI is required.")
	}
	content, err := adapter.distribution.ReadResource(ctx, adapter.workspace, request.Params.URI)
	if err != nil {
		return nil, adapter.distributionRPCError(err)
	}
	resource := &mcp.ResourceContents{URI: content.URI, MIMEType: content.MIMEType}
	if content.IsText() {
		resource.Text = string(content.Bytes)
	} else {
		resource.Blob = content.Bytes
	}
	return &mcp.ReadResourceResult{Cacheable: mcp.Cacheable{TTLMs: cacheTTLMS, CacheScope: "private"}, Contents: []*mcp.ResourceContents{resource}}, nil
}

func toSkillEntry(entry app.DistributedSkill) skillEntry {
	return skillEntry{URI: entry.URI, Frontmatter: entry.Frontmatter, Resources: entry.Resources}
}

func (adapter *Server) distributionRPCError(err error) error {
	switch {
	case errors.Is(err, app.ErrResourceContentUnavailable), errors.Is(err, skill.ErrResourceContentUnavailable):
		correlation := correlationID()
		adapter.logger.Error("skill resource content unavailable", "correlation_id", correlation)
		data, _ := json.Marshal(map[string]string{"code": "resource_content_unavailable", "correlation_id": correlation})
		return &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: "The requested skill resource has changed or is missing on disk.", Data: data}
	case errors.Is(err, skill.ErrResourceDigestMismatch):
		correlation := correlationID()
		adapter.logger.Error("skill resource integrity verification failed", "correlation_id", correlation)
		data, _ := json.Marshal(map[string]string{"code": "resource_digest_mismatch", "correlation_id": correlation})
		return &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: "Skill resource integrity verification failed.", Data: data}
	case errors.Is(err, catalog.ErrCatalogUnavailable):
		return invalidParams("index_stale", "The derived catalog is stale or unavailable; run workspace_rebuild and retry.")
	case errors.Is(err, skill.ErrSnapshotExpired), errors.Is(err, skill.ErrNotFound):
		return invalidParams("snapshot_expired", "The requested skill snapshot is unavailable; refresh the skill entry.")
	default:
		correlation := correlationID()
		adapter.logger.Error("MCP distribution internal failure", "correlation_id", correlation)
		data, _ := json.Marshal(map[string]any{"code": "internal_error", "retryable": true, "correlation_id": correlation})
		return &jsonrpc.Error{Code: jsonrpc.CodeInternalError, Message: "The skill resource could not be served.", Data: data}
	}
}

func invalidParams(code, message string) error {
	data, _ := json.Marshal(map[string]string{"code": code})
	return &jsonrpc.Error{Code: jsonrpc.CodeInvalidParams, Message: message, Data: data}
}

func (adapter *Server) registerTools(server *mcp.Server) {
	adapter.registerResolverTools(server)
	adapter.registerSourceTools(server)
	adapter.registerSourceImportTools(server)
	adapter.registerSourceWatchTools(server)
	adapter.registerCurationRunTools(server)
	adapter.registerInsightTools(server)
	adapter.registerSkillTools(server)
	adapter.registerSkillAddTools(server)
	adapter.registerSkillReviewTools(server)
	adapter.registerWorkspaceTools(server)
}

func addTool[In, Out any](server *mcp.Server, tool *mcp.Tool, handler mcp.ToolHandlerFor[In, toolOutcome[Out]]) {
	schema, err := jsonschema.For[In](&jsonschema.ForOptions{})
	if err != nil {
		panic(fmt.Sprintf("derive schema for %s: %v", tool.Name, err))
	}
	closeObjectSchemas(schema)
	applyCommonConstraints(schema)
	if tool.Name == "curation_session_record" {
		applyCurationSessionConstraints(schema)
	}
	normalizeMultiTypeSchemas(schema)
	outputSchema, err := jsonschema.For[toolOutcome[Out]](&jsonschema.ForOptions{})
	if err != nil {
		panic(fmt.Sprintf("derive output schema for %s: %v", tool.Name, err))
	}
	closeObjectSchemas(outputSchema)
	normalizeMultiTypeSchemas(outputSchema)
	if tool.Name == "skill_resolve" || tool.Name == "routing_evaluate" {
		committedRequest, contractErr := contractschemas.ResolverRequest()
		if contractErr != nil {
			panic(fmt.Sprintf("load committed resolver request schema: %v", contractErr))
		}
		committedResponse, contractErr := contractschemas.ResolverResponse()
		if contractErr != nil {
			panic(fmt.Sprintf("load committed resolver response schema: %v", contractErr))
		}
		if !replacePropertySchema(outputSchema, "resolution", committedResponse) {
			panic(fmt.Sprintf("resolver output schema for %s has no resolution property", tool.Name))
		}
		schema = committedRequest
	}
	applyToolInputEnums(tool.Name, schema)
	tool.InputSchema = schema
	tool.OutputSchema = outputSchema
	mcp.AddTool(server, tool, handler)
}

func applyToolInputEnums(toolName string, schema *jsonschema.Schema) {
	if schema == nil {
		return
	}
	switch toolName {
	case "skill_feedback":
		enums := map[string][]any{
			"outcome":     {"activated", "loaded", "used", "abandoned", "rejected", "completed", "failed"},
			"reason_code": {"user_rejected", "scope_mismatch", "capability_unavailable", "constraint_conflict", "workflow_completed", "workflow_failed", "abandoned", "host_report"},
			"utility":     {"helpful", "harmful", "neutral"},
			"basis":       {"user", "evaluator", "controlled-benchmark"},
		}
		for name, values := range enums {
			if property := schema.Properties[name]; property != nil {
				property.Enum = values
			}
		}
	case "source_watch_preview":
		if prop := schema.Properties["cadence"]; prop != nil {
			prop.Enum = []any{"manual", "daily", "weekly"}
		}
		if prop := schema.Properties["trust"]; prop != nil {
			prop.Enum = []any{"untrusted", "reviewed", "trusted"}
		}
	}
}

func replacePropertySchema(schema *jsonschema.Schema, name string, replacement *jsonschema.Schema) bool {
	if schema == nil {
		return false
	}
	replaced := false
	if _, exists := schema.Properties[name]; exists {
		schema.Properties[name] = replacement.CloneSchemas()
		replaced = true
	}
	for _, child := range schema.Properties {
		replaced = replacePropertySchema(child, name, replacement) || replaced
	}
	for _, child := range schema.Defs {
		replaced = replacePropertySchema(child, name, replacement) || replaced
	}
	for _, child := range schema.AnyOf {
		replaced = replacePropertySchema(child, name, replacement) || replaced
	}
	return replacePropertySchema(schema.Items, name, replacement) || replaced
}

func closeObjectSchemas(schema *jsonschema.Schema) {
	if schema == nil {
		return
	}
	if schema.Type == "object" && schema.AdditionalProperties == nil {
		schema.AdditionalProperties = &jsonschema.Schema{Not: &jsonschema.Schema{}}
	}
	for _, child := range schema.Properties {
		closeObjectSchemas(child)
	}
	for _, child := range schema.Defs {
		closeObjectSchemas(child)
	}
	for _, child := range schema.AnyOf {
		closeObjectSchemas(child)
	}
	closeObjectSchemas(schema.Items)
}

// normalizeMultiTypeSchemas preserves nullable semantics while avoiding the
// multi-value `type` form rejected by several function-calling clients.
func normalizeMultiTypeSchemas(schema *jsonschema.Schema) {
	if schema == nil {
		return
	}
	if len(schema.Types) > 0 {
		for _, schemaType := range schema.Types {
			schema.AnyOf = append(schema.AnyOf, &jsonschema.Schema{Type: schemaType})
		}
		schema.Types = nil
	}
	for _, child := range schema.Properties {
		normalizeMultiTypeSchemas(child)
	}
	for _, child := range schema.Defs {
		normalizeMultiTypeSchemas(child)
	}
	for _, child := range schema.AnyOf {
		normalizeMultiTypeSchemas(child)
	}
	normalizeMultiTypeSchemas(schema.Items)
}

func applyCommonConstraints(schema *jsonschema.Schema) {
	if schema == nil {
		return
	}
	if schemaHasType(schema, "array") && schema.MaxItems == nil {
		schema.MaxItems = intPointer(maximumLimit)
	}
	for name, property := range schema.Properties {
		switch name {
		case "limit":
			minimum, maximum := float64(1), float64(maximumLimit)
			property.Minimum, property.Maximum = &minimum, &maximum
		case "schema_version":
			property.Enum = []any{SchemaVersion}
		case "decision":
			property.MaxLength = intPointer(64)
		case "content":
			property.MaxLength = intPointer(1 << 20)
		case "cursor", "uri", "locator", "reason", "rationale", "description", "note":
			property.MaxLength = intPointer(4096)
		default:
			if schemaHasType(property, "string") {
				property.MaxLength = intPointer(2048)
			}
		}
		applyCommonConstraints(property)
	}
	for _, child := range schema.Defs {
		applyCommonConstraints(child)
	}
	applyCommonConstraints(schema.Items)
}

func applyCurationSessionConstraints(schema *jsonschema.Schema) {
	if schema == nil {
		return
	}
	for name, property := range schema.Properties {
		switch name {
		case "status":
			property.Enum = []any{"completed", "partial", "failed", "abandoned"}
		case "basis":
			property.Enum = []any{telemetry.CurationBasisHostReported, telemetry.CurationBasisControlledBenchmark}
		case "error_code":
			property.Enum = []any{"capability_unavailable", "internal_error", "operation_cancelled", "partial_failure", "recovery_required", "source_unavailable", "validation_failed"}
		case "turns_to_next_action", "unnecessary_confirmations", "prompts_per_batch", "batch_size", "routine_git_noise":
			minimum, maximum := float64(0), float64(10_000)
			property.Minimum, property.Maximum = &minimum, &maximum
		case "duration_ms":
			minimum, maximum := float64(0), float64(604_800_000)
			property.Minimum, property.Maximum = &minimum, &maximum
		}
	}
}

func schemaHasType(schema *jsonschema.Schema, target string) bool {
	if schema.Type == target {
		return true
	}
	for _, schemaType := range schema.Types {
		if schemaType == target {
			return true
		}
	}
	return false
}

func intPointer(value int) *int { return &value }

func annotations(readOnly, destructive, idempotent, openWorld bool) *mcp.ToolAnnotations {
	return &mcp.ToolAnnotations{ReadOnlyHint: readOnly, DestructiveHint: &destructive, IdempotentHint: idempotent, OpenWorldHint: &openWorld}
}

func success[T any](value T) (*mcp.CallToolResult, toolOutcome[T], error) {
	return &mcp.CallToolResult{}, toolOutcome[T]{SchemaVersion: SchemaVersion, Result: &value}, nil
}

func failure[T any](err error) (*mcp.CallToolResult, toolOutcome[T], error) {
	item := safeToolError(err)
	return &mcp.CallToolResult{IsError: true}, toolOutcome[T]{SchemaVersion: SchemaVersion, Error: &item}, nil
}

func unavailable[T any](message string) (*mcp.CallToolResult, toolOutcome[T], error) {
	item := toolError{Code: "capability_unavailable", Message: message, Retryable: false, SuggestedAction: "Use an existing CLI/application contract or wait until this semantic is implemented."}
	return &mcp.CallToolResult{IsError: true}, toolOutcome[T]{SchemaVersion: SchemaVersion, Error: &item}, nil
}

func safeToolError(err error) toolError {
	var appErr *app.Error
	if errors.As(err, &appErr) && appErr != nil {
		action := appErr.Render.Fix
		if action == "" {
			action = "Check request parameters and retry."
		}
		msg := appErr.Render.Error
		if msg == "" {
			msg = appErr.Error()
		}
		return toolError{
			Code:            string(appErr.Code),
			Message:         msg,
			Retryable:       appErr.Code == app.ErrorStaleContext || appErr.Code == app.ErrorSnapshotExpired || appErr.Code == app.ErrorSourceUnavailable || appErr.Code == app.ErrorIndexStale,
			SuggestedAction: action,
		}
	}
	var missingActivationErr *app.MissingActivationRequirementsError
	if errors.As(err, &missingActivationErr) {
		return toolError{
			Code:            "invalid_request",
			Message:         missingActivationErr.Error(),
			Retryable:       false,
			SuggestedAction: fmt.Sprintf("Run skill_update_preview to configure the missing fields (%s) before activating.", strings.Join(missingActivationErr.Missing, ", ")),
		}
	}
	if strings.Contains(err.Error(), "does not exist; use skill_create_preview") {
		return toolError{
			Code:            "invalid_request",
			Message:         err.Error(),
			Retryable:       false,
			SuggestedAction: "Use skill_create_preview to create a new draft skill.",
		}
	}
	var conflictErr *skill.EditConflictError
	if errors.As(err, &conflictErr) || errors.Is(err, skill.ErrEditConflict) {
		msg := "A concurrent edit conflict occurred on the skill content."
		if conflictErr != nil {
			msg = conflictErr.Error()
		}
		return toolError{
			Code:            "edit_conflict",
			Message:         msg,
			Retryable:       false,
			SuggestedAction: "Read the latest skill content and digest via skill_get, then retry skill_update_preview with the new expected_content_digest.",
		}
	}
	if errors.Is(err, context.Canceled) {
		return toolError{Code: "operation_cancelled", Message: "The operation was cancelled before completion.", Retryable: true, SuggestedAction: "Retry when ready."}
	}
	if errors.Is(err, telemetry.ErrFeedbackResolutionNotFound) {
		return toolError{Code: "unknown_resolution", Message: "The prior resolution is unavailable.", Retryable: true, SuggestedAction: "Start a new resolution request."}
	}
	if errors.Is(err, telemetry.ErrFeedbackConflict) {
		return toolError{Code: "invalid_request", Message: "The event ID conflicts with existing feedback.", Retryable: false, SuggestedAction: "Retry with the original feedback or use a new event_id."}
	}
	if errors.Is(err, telemetry.ErrCurationSessionConflict) {
		return toolError{Code: "invalid_request", Message: "The event ID conflicts with existing curation measurements.", Retryable: false, SuggestedAction: "Retry with the original measurements or use a new event_id."}
	}
	if errors.Is(err, skill.ErrSnapshotExpired) {
		return toolError{Code: "snapshot_expired", Message: "The pinned snapshot is no longer available.", Retryable: true, SuggestedAction: "Resolve or list again before retrying."}
	}
	if errors.Is(err, skill.ErrResourceDigestMismatch) {
		return toolError{Code: "resource_digest_mismatch", Message: "Resource integrity verification failed; no bytes were used.", Retryable: false, SuggestedAction: "Validate and rebuild the catalog before retrying."}
	}
	if errors.Is(err, skill.ErrNotFound) || errors.Is(err, skill.ErrAlreadyExists) || errors.Is(err, skill.ErrInvalidTransition) || errors.Is(err, mutation.ErrIdempotencyConflict) {
		return toolError{Code: "invalid_request", Message: "The request conflicts with the current object state.", Retryable: false, SuggestedAction: "Refresh the object state, correct the request, and retry."}
	}
	if errors.Is(err, mutation.ErrConflict) {
		return toolError{Code: "stale_context", Message: "The request was based on stale canonical state.", Retryable: true, SuggestedAction: "Refresh the object and regenerate the operation before retrying."}
	}
	if errors.Is(err, mutation.ErrRecoveryRequired) {
		return toolError{Code: "recovery_required", Message: "Workspace recovery is required before another mutation.", Retryable: true, SuggestedAction: "Run workspace validation and recovery before retrying."}
	}
	if errors.Is(err, mutation.ErrWorkspaceBusy) {
		return internalToolError("The workspace lock could not be acquired.")
	}
	if errors.Is(err, os.ErrPermission) {
		return toolError{Code: "permission_denied", Message: "The operation is not permitted.", Retryable: false, SuggestedAction: "Correct workspace permissions or host policy before retrying."}
	}
	if errors.Is(err, sourcepkg.ErrInvalidLocator) || errors.Is(err, sourcepkg.ErrUnsafeAddress) || errors.Is(err, sourcepkg.ErrLimitExceeded) {
		return toolError{Code: "invalid_request", Message: "The request violates a source locator or resource limit.", Retryable: false, SuggestedAction: "Correct the bounded source request and retry."}
	}
	if errors.Is(err, sourcepkg.ErrRevisionMismatch) || errors.Is(err, sourcepkg.ErrHistoryUnavailable) {
		return toolError{Code: "source_unavailable", Message: "The requested source revision is unavailable.", Retryable: true, SuggestedAction: "Refresh the source revision and retry."}
	}
	if errors.Is(err, catalog.ErrCatalogUnavailable) {
		return toolError{Code: "index_stale", Message: "The derived catalog is unavailable or stale.", Retryable: true, SuggestedAction: "Run workspace_rebuild, then retry."}
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "snapshot_expired") || strings.Contains(message, "cursor is invalid or expired") {
		return toolError{Code: "snapshot_expired", Message: "The pinned snapshot or cursor is no longer available.", Retryable: true, SuggestedAction: "Restart the list or resolution and retry with its new pins."}
	}
	if strings.Contains(message, "schema_version") || strings.Contains(message, "unsupported schema") {
		return toolError{Code: "unsupported_schema", Message: "The requested schema major is not supported.", SuggestedAction: "Use schema_version 1."}
	}
	if strings.Contains(message, "prior clarification was not issued") {
		return toolError{Code: "unknown_resolution", Message: "The prior resolution is unavailable.", Retryable: true, SuggestedAction: "Start a new resolution request."}
	}
	if strings.Contains(message, "prior clarification is stale") || strings.Contains(message, "does not match this request") {
		return toolError{Code: "stale_context", Message: "The prior resolution no longer matches the current request.", Retryable: true, SuggestedAction: "Start a new resolution with the current context."}
	}
	if strings.Contains(message, "catalog is stale") || strings.Contains(message, "catalog is missing") || strings.Contains(message, "catalog is corrupt") {
		return toolError{Code: "index_stale", Message: "The derived catalog is unavailable or stale.", Retryable: true, SuggestedAction: "Run workspace_rebuild, then retry."}
	}
	if knownRequestError(message) {
		return toolError{Code: "invalid_request", Message: "The request conflicts with validation rules or the current object state.", Retryable: false, SuggestedAction: "Inspect the tool schema and current object state, correct the request, and retry."}
	}
	return internalToolError("The operation failed internally.")
}

func internalToolError(message string) toolError {
	correlation := correlationID()
	if logger := activeDiagnostics.Load(); logger != nil {
		logger.Error("MCP tool internal failure", "correlation_id", correlation)
	}
	return toolError{Code: "internal_error", Message: message, Retryable: true, SuggestedAction: "Retry once; if the failure persists, use the correlation ID with stderr diagnostics.", CorrelationID: correlation}
}

func knownRequestError(message string) bool {
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

func correlationID() string {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("fallback-%016x", correlationSequence.Add(1))
	}
	return hex.EncodeToString(value[:])
}

type cursorValue struct {
	Version    int    `json:"v"`
	Owner      string `json:"o"`
	FilterHash string `json:"f"`
	LastKey    string `json:"k"`
	Checksum   string `json:"c"`
}

func pageDigest(value any) string {
	encoded, _ := json.Marshal(value)
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func pageOwner(filter string, sortedResult any) string {
	return pageDigest(struct {
		Filter string `json:"filter"`
		Result any    `json:"result"`
	}{Filter: filter, Result: sortedResult})
}

func encodeCursor(owner, filter, lastKey string) string {
	value := cursorValue{Version: 2, Owner: owner, FilterHash: pageDigest(filter), LastKey: lastKey}
	value.Checksum = cursorChecksum(value)
	data, _ := json.Marshal(value)
	return base64.RawURLEncoding.EncodeToString(data)
}

func cursorChecksum(value cursorValue) string {
	value.Checksum = ""
	encoded, _ := json.Marshal(value)
	mac := hmac.New(sha256.New, cursorMACKey)
	_, _ = mac.Write([]byte("skillhub-page-cursor-v2\x00"))
	_, _ = mac.Write(encoded)
	return hex.EncodeToString(mac.Sum(nil))
}

func decodeCursor(cursor, owner, filter string) (string, error) {
	if cursor == "" {
		return "", nil
	}
	if len(cursor) > 4096 {
		return "", errors.New("cursor too large")
	}
	data, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", err
	}
	var value cursorValue
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil || value.Version != 2 || value.Owner != owner || value.FilterHash != pageDigest(filter) || value.LastKey == "" || !hmac.Equal([]byte(value.Checksum), []byte(cursorChecksum(value))) {
		return "", errors.New("cursor mismatch")
	}
	return value.LastKey, nil
}

func normalizeLimit(limit int) (int, error) {
	if limit == 0 {
		return defaultLimit, nil
	}
	if limit < 1 || limit > maximumLimit {
		return 0, fmt.Errorf("limit must be between 1 and %d", maximumLimit)
	}
	return limit, nil
}

func makePage[T any](items []T, limit int, lastKey, owner, filter string, key func(T) string) (page[T], error) {
	start := 0
	if lastKey != "" {
		found := false
		for index, item := range items {
			if key(item) == lastKey {
				start, found = index+1, true
				break
			}
		}
		if !found {
			return page[T]{}, errors.New("cursor last sort key is absent")
		}
	}
	end := min(start+limit, len(items))
	result := page[T]{Items: append([]T(nil), items[start:end]...), HasMore: end < len(items), Total: len(items)}
	if result.HasMore && len(result.Items) > 0 {
		result.NextCursor = encodeCursor(owner, filter, key(result.Items[len(result.Items)-1]))
	}
	return result, nil
}

func applicationError(value any) *app.Error {
	data, err := json.Marshal(value)
	if err != nil {
		return &app.Error{Code: app.ErrorInternal, Retryable: true, Render: app.ErrorRender{Error: "The result could not be encoded.", Fix: "Retry or inspect diagnostics."}}
	}
	var envelope struct {
		Error *app.Error `json:"error"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return &app.Error{Code: app.ErrorInternal, Retryable: true, Render: app.ErrorRender{Error: "The result could not be encoded.", Fix: "Retry or inspect diagnostics."}}
	}
	return envelope.Error
}

func appResult[T any](value T, err error) (*mcp.CallToolResult, toolOutcome[T], error) {
	if err != nil {
		return failure[T](err)
	}
	if appErr := applicationError(value); appErr != nil {
		item := toolError{Code: string(appErr.Code), Message: appErr.Render.Error, Retryable: appErr.Retryable, SuggestedAction: appErr.Render.Fix}
		if appErr.Code == app.ErrorInternal {
			item.CorrelationID = correlationID()
			item.Message = "The operation failed internally."
			item.Retryable = true
			if logger := activeDiagnostics.Load(); logger != nil {
				logger.Error("MCP application internal failure", "correlation_id", item.CorrelationID)
			}
		}
		return &mcp.CallToolResult{IsError: true}, toolOutcome[T]{SchemaVersion: SchemaVersion, Error: &item}, nil
	}
	return success(value)
}
