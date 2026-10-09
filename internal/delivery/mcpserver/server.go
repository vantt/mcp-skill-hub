// Package mcpserver exposes Skill Hub application services over MCP stdio.
package mcpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/delivery/paging"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	"github.com/vantt/mcp-skill-hub/internal/systemskills"
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
	snapshots    app.SnapshotService
	telemetry    app.TelemetrySink
	tracker      *activationTracker
	logger       *slog.Logger
}

var activeDiagnostics atomic.Pointer[slog.Logger]
var correlationSequence atomic.Uint64

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
	adapter := &Server{
		workspace: root,
		snapshots: app.NewSnapshotService(),
		tracker:   newActivationTracker(nil, root),
		logger:    logger,
	}
	capabilities := &mcp.ServerCapabilities{
		Resources: &mcp.ResourceCapabilities{},
		Tools:     &mcp.ToolCapabilities{},
	}
	capabilities.AddExtension("io.modelcontextprotocol/skills", map[string]any{"directoryRead": false})
	server := mcp.NewServer(&mcp.Implementation{Name: "skillhub", Version: "1"}, &mcp.ServerOptions{
		Capabilities: capabilities,
		PageSize:     paging.MaximumLimit,
		Logger:       logger,
		SetCacheable: func(_ context.Context, _ mcp.Request, cache *mcp.Cacheable) {
			cache.TTLMs = cacheTTLMS
			cache.CacheScope = "private"
		},
	})
	server.AddReceivingMiddleware(adapter.telemetryMiddleware)
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
	if gcErr := adapter.snapshots.CollectGarbage(ctx, adapter.workspace, app.SnapshotGCMinAge); gcErr != nil {
		adapter.logger.Warn("local skill snapshot garbage collection failed", "error", gcErr)
	}
	if recorder, telemetryErr := (app.TelemetryService{}).Open(adapter.workspace); telemetryErr != nil {
		adapter.logger.Warn("MCP telemetry recorder could not be opened")
	} else {
		adapter.telemetry = recorder
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

func (adapter *Server) clientForReq(req mcp.Request) telemetry.Client {
	client := telemetry.Client{Name: "other"}
	if req != nil {
		if s, ok := req.GetSession().(*mcp.ServerSession); ok && s != nil {
			if params := s.InitializeParams(); params != nil && params.ClientInfo != nil {
				client = NormalizeClient(params.ClientInfo.Name, params.ClientInfo.Version)
			}
		}
	}
	return client
}

func (adapter *Server) telemetryMiddleware(next mcp.MethodHandler) mcp.MethodHandler {
	return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
		if strings.HasPrefix(method, "skills/") && method != "skills/list" && method != "skills/get" {
			app.RecordServerMetric(ctx, adapter.telemetry, adapter.workspace, app.ServerMetric{Name: "unsupported_method_calls", Value: 1, Client: adapter.clientForReq(req)})
		}
		result, err := next(ctx, method, req)
		if err == nil && method == "tools/list" {
			if encoded, encErr := json.Marshal(result); encErr == nil {
				app.RecordServerMetric(ctx, adapter.telemetry, adapter.workspace, app.ServerMetric{Name: "tools_list_bytes", Value: float64(len(encoded)), Client: adapter.clientForReq(req)})
			}
		}
		return result, err
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

func (adapter *Server) listSkills(ctx context.Context, session *mcp.ServerSession, params *listSkillsParams) (*listSkillsResult, error) {
	entries, snapshot, skipped, err := adapter.distribution.ListSkillsReport(ctx, adapter.workspace)
	if err != nil {
		return nil, adapter.distributionRPCError(ctx, session, err)
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
	owner := paging.Owner(filter, struct {
		Snapshot string                 `json:"snapshot"`
		Entries  []app.DistributedSkill `json:"entries"`
	}{Snapshot: snapshot, Entries: entries})
	lastKey, err := paging.DecodeCursor(params.Cursor, owner, filter)
	if err != nil {
		return nil, invalidParams("snapshot_expired", "The skill listing cursor is invalid or expired.")
	}
	paged, err := paging.Make(entries, paging.DefaultLimit, lastKey, owner, filter, func(entry app.DistributedSkill) string { return entry.SkillID })
	if err != nil {
		return nil, invalidParams("snapshot_expired", "The skill listing cursor is invalid or expired.")
	}
	result := &listSkillsResult{ResultType: "complete", Skills: []skillEntry{}, TTLMS: cacheTTLMS, CacheScope: "private", NextCursor: paged.NextCursor}
	for _, entry := range paged.Items {
		result.Skills = append(result.Skills, toSkillEntry(entry))
	}
	return result, nil
}

func (adapter *Server) getSkill(ctx context.Context, session *mcp.ServerSession, params *getSkillParams) (*getSkillResult, error) {
	if params == nil || params.URI == "" || len(params.URI) > 4096 {
		return nil, invalidParams("snapshot_expired", "A bounded skill SKILL.md URI is required.")
	}
	entry, err := adapter.distribution.GetSkill(ctx, adapter.workspace, params.URI)
	if err != nil {
		return nil, adapter.distributionRPCError(ctx, session, err)
	}
	result := toSkillEntry(entry)
	result.Local = adapter.localSkill(ctx, entry.SkillID, "active")
	blocked := result.Local != nil && result.Local.Status == app.LocalStatusReviewRequired
	var reasons []string
	if blocked && result.Local != nil {
		reasons = result.Local.ReasonCodes
	}
	adapter.recordLoad(ctx, session, app.SkillLoad{
		SkillID:      entry.SkillID,
		ResourceKind: "entrypoint",
		Surface:      "skills_get",
		Blocked:      blocked,
		ReasonCodes:  reasons,
	})
	return &getSkillResult{ResultType: "complete", Skill: result, TTLMS: cacheTTLMS, CacheScope: "private"}, nil
}

// localSkill exports or reuses the local snapshot for an activated skill. The
// bundled curator is instruction-only and gets none; non-active skills report
// unavailable; an export failure degrades to unavailable instead of failing
// the request.
func (adapter *Server) localSkill(ctx context.Context, skillID, lifecycleState string) *app.LocalSkill {
	if skillID == systemskills.CuratorSkillID {
		return nil
	}
	if lifecycleState != "active" {
		local := app.LocalSkill{Status: app.LocalStatusUnavailable, ReasonCodes: []string{app.LocalReasonNotServable}}
		return &local
	}
	local, err := adapter.snapshots.Ensure(ctx, adapter.workspace, skillID)
	if err != nil {
		adapter.logger.Warn("local skill snapshot could not be exported", "skill_id", skillID, "error", err)
		local = app.LocalSkill{Status: app.LocalStatusUnavailable, ReasonCodes: []string{app.LocalReasonExportFailed}}
	}
	return &local
}

func (adapter *Server) readResource(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	if request == nil || request.Params == nil || len(request.Params.URI) > 4096 {
		return nil, invalidParams("snapshot_expired", "A bounded skill resource URI is required.")
	}
	content, err := adapter.distribution.ReadResource(ctx, adapter.workspace, request.Params.URI)
	if err != nil {
		return nil, adapter.distributionRPCError(ctx, request.Session, err)
	}
	meta, refused, reasons := adapter.localResourceMeta(ctx, content.SkillID, content.Path)
	kind := mapResourceKind(content.Path)
	if refused {
		var session *mcp.ServerSession
		if request != nil {
			session = request.Session
		}
		adapter.recordLoad(ctx, session, app.SkillLoad{
			SkillID:      content.SkillID,
			ResourceKind: kind,
			Surface:      "resources_read",
			Blocked:      true,
			ReasonCodes:  reasons,
		})
		return nil, invalidParams("content_review_required", fmt.Sprintf("This skill's content has not been approved, so none of it is readable; do not use the skill and ask the user to run `skillhub skill review %s`.", content.SkillID))
	}
	var session *mcp.ServerSession
	if request != nil {
		session = request.Session
	}
	adapter.recordLoad(ctx, session, app.SkillLoad{
		SkillID:      content.SkillID,
		ResourceKind: kind,
		Surface:      "resources_read",
	})

	var unlisted bool
	entries, _, lookupErr := adapter.distribution.LookupSkills(ctx, adapter.workspace, []string{content.SkillID})
	if lookupErr == nil {
		if entry, ok := entries[content.SkillID]; ok {
			unlisted = true
			for _, r := range entry.Resources {
				if r.URI == request.Params.URI {
					unlisted = false
					break
				}
			}
		}
	}
	if unlisted {
		app.RecordServerMetric(ctx, adapter.telemetry, adapter.workspace, app.ServerMetric{Name: "unlisted_resource_reads", Value: 1, SkillID: content.SkillID, Client: adapter.callerContext(request.Session).Client})
	}

	resource := &mcp.ResourceContents{URI: content.URI, MIMEType: content.MIMEType}
	if meta != nil {
		resource.Meta = meta
	}
	if content.IsText() {
		resource.Text = string(content.Bytes)
	} else {
		resource.Blob = content.Bytes
	}
	return &mcp.ReadResourceResult{Cacheable: mcp.Cacheable{TTLMs: cacheTTLMS, CacheScope: "private"}, Contents: []*mcp.ResourceContents{resource}}, nil
}

func mapResourceKind(path string) string {
	if path == "SKILL.md" || filepath.Base(path) == "SKILL.md" {
		return "entrypoint"
	}
	parts := strings.Split(path, "/")
	for _, kind := range []string{"references", "scripts", "assets"} {
		for _, part := range parts {
			if part == kind {
				return strings.TrimSuffix(kind, "s")
			}
		}
	}
	return "resource"
}

func (adapter *Server) recordLoad(ctx context.Context, session *mcp.ServerSession, load app.SkillLoad) {
	if load.SkillID == "" || load.SkillID == systemskills.CuratorSkillID {
		return
	}
	load.Attribution = "unsolicited"
	if adapter.tracker != nil {
		load.ResolutionID, load.Attribution, load.CatalogSnapshot, load.PolicyRevision, load.Client = adapter.tracker.attributeDetails(session, load.SkillID)
		load.SessionIDHash = adapter.tracker.sessionHash(session)
		if !load.Blocked && load.ResourceKind == "entrypoint" {
			load.FirstActivation = adapter.tracker.markActivation(session, load.ResolutionID, load.SkillID)
		}
		if load.FirstActivation && (load.Attribution == "override" || load.Attribution == "after_no_skill") {
			if res, ok := adapter.tracker.resolutionData(session, load.ResolutionID); ok {
				_ = app.RecordCase(ctx, adapter.telemetry, telemetry.CaseRecord{
					ResolutionID:    res.ResolutionID,
					OccurredAt:      time.Now().UTC(),
					Kind:            load.Attribution,
					Client:          res.Client,
					CatalogSnapshot: res.CatalogSnapshot,
					PriorVerified:   res.PriorVerified,
					Task:            map[string]any{"description": res.TaskDescription},
					Operation:       res.Operation,
					Request:         res.Request,
					Resolver: map[string]any{
						"status":         res.Status,
						"topk_skill_ids": res.TopKSkillIDs,
						"topk_matched":   res.TopKMatched,
						"topk_channels":  res.TopKChannels,
					},
					Chosen: load.SkillID,
				})
			}
		}
	}
	app.RecordSkillLoad(ctx, adapter.telemetry, adapter.workspace, load)
}

func (adapter *Server) callerContext(session *mcp.ServerSession) app.CallerContext {
	sessionHash := ""
	client := telemetry.Client{Name: "other"}
	if session != nil {
		if params := session.InitializeParams(); params != nil && params.ClientInfo != nil {
			client = NormalizeClient(params.ClientInfo.Name, params.ClientInfo.Version)
		}
	}
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

// localResourceMeta maps a verified resource URI to its file in the local
// snapshot. refused is true when the skill awaits content review, in which
// case no resource, SKILL.md included, may be served.
func (adapter *Server) localResourceMeta(ctx context.Context, skillID, relative string) (meta mcp.Meta, refused bool, reasons []string) {
	if skillID == "" || skillID == systemskills.CuratorSkillID {
		return nil, false, nil
	}
	local, err := adapter.snapshots.Ensure(ctx, adapter.workspace, skillID)
	if err != nil {
		adapter.logger.Warn("local skill snapshot could not be exported", "skill_id", skillID, "error", err)
		return nil, false, nil
	}
	if local.Status == app.LocalStatusReviewRequired {
		return nil, true, local.ReasonCodes
	}
	for _, resource := range local.Resources {
		if resource.Path == relative {
			return mcp.Meta{"io.skillhub/local_path": resource.LocalPath}, false, nil
		}
	}
	return nil, false, nil
}

func toSkillEntry(entry app.DistributedSkill) skillEntry {
	return skillEntry{URI: entry.URI, Frontmatter: entry.Frontmatter, Resources: entry.Resources}
}

func (adapter *Server) distributionRPCError(ctx context.Context, session *mcp.ServerSession, err error) error {
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
		app.RecordServerMetric(ctx, adapter.telemetry, adapter.workspace, app.ServerMetric{Name: "snapshot_expired_requests", Value: 1, Client: adapter.callerContext(session).Client})
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
	adapter.registerUpstreamTools(server)
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
			"reason_code": {"user_rejected", "scope_mismatch", "capability_unavailable", "constraint_conflict", "workflow_completed", "workflow_failed", "abandoned", "host_report", "setup_failed"},
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
		schema.MaxItems = intPointer(paging.MaximumLimit)
	}
	for name, property := range schema.Properties {
		switch name {
		case "limit":
			minimum, maximum := float64(1), float64(paging.MaximumLimit)
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

func internalToolError(message string) toolError {
	correlation := correlationID()
	if logger := activeDiagnostics.Load(); logger != nil {
		logger.Error("MCP tool internal failure", "correlation_id", correlation)
	}
	return toolError{Code: "internal_error", Message: message, Retryable: true, SuggestedAction: "Retry once; if the failure persists, use the correlation ID with stderr diagnostics.", CorrelationID: correlation}
}

func correlationID() string {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fmt.Sprintf("fallback-%016x", correlationSequence.Add(1))
	}
	return hex.EncodeToString(value[:])
}

func applicationError(value any) *app.Error {
	_ = errorEnvelope{}
	return app.ErrorOf(value, nil)
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
