package mcpserver

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/delivery/paging"
	"github.com/vantt/mcp-skill-hub/internal/hostintegration"
	"github.com/vantt/mcp-skill-hub/internal/systemskills"
)

func (adapter *Server) registerSkills(server *mcp.Server) error {
	if err := mcp.AddReceivingCustomMethod(server, "skills/list", adapter.listSkills); err != nil {
		return err
	}
	if err := mcp.AddReceivingCustomMethod(server, "skills/get", adapter.getSkill); err != nil {
		return err
	}
	if err := mcp.AddReceivingCustomMethod(server, "skills/bundle", adapter.bundleMethod); err != nil {
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
	entries = adapter.omitAwaitingReview(ctx, entries)
	if nativeCuratorForSession(session) {
		for index, entry := range entries {
			if entry.SkillID == systemskills.CuratorSkillID {
				entries = append(entries[:index], entries[index+1:]...)
				break
			}
		}
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

// omitAwaitingReview drops skills whose content awaits review. A listing may
// be partial, and a host that follows the listing would otherwise try to load
// a skill whose every read is refused. skills/get and resources/read keep
// their refusal for a URI the host already holds.
func (adapter *Server) omitAwaitingReview(ctx context.Context, entries []app.DistributedSkill) []app.DistributedSkill {
	kept := make([]app.DistributedSkill, 0, len(entries))
	for _, entry := range entries {
		if entry.SkillID != systemskills.CuratorSkillID {
			if adapter.remoteLocalSkill(ctx, entry.SkillID).Status == app.LocalStatusReviewRequired {
				continue
			}
		}
		kept = append(kept, entry)
	}
	return kept
}

func (adapter *Server) getSkill(ctx context.Context, session *mcp.ServerSession, params *getSkillParams) (*getSkillResult, error) {
	if params == nil || params.URI == "" || len(params.URI) > 4096 {
		return nil, invalidParams("invalid_uri", "A bounded skill SKILL.md URI is required.")
	}
	entry, err := adapter.distribution.GetSkill(ctx, adapter.workspace, params.URI)
	if err != nil {
		return nil, adapter.distributionRPCError(ctx, session, err)
	}
	if entry.SkillID == systemskills.CuratorSkillID && nativeCuratorForSession(session) {
		return nil, invalidParams("skill_not_found", "This client uses its native system-curator skill.")
	}
	result := toSkillEntry(entry)
	result.Local = adapter.localSkill(ctx, entry.SkillID, "active")
	blocked, reasons := reviewBlock(result.Local)
	adapter.recordLoad(ctx, session, app.SkillLoad{
		SkillID:      entry.SkillID,
		ResourceKind: "entrypoint",
		Surface:      "skills_get",
		Blocked:      blocked,
		ReasonCodes:  reasons,
	})
	return &getSkillResult{ResultType: "complete", Skill: result, TTLMS: cacheTTLMS, CacheScope: "private"}, nil
}

func nativeCuratorForSession(session *mcp.ServerSession) bool {
	if session == nil {
		return false
	}
	params := session.InitializeParams()
	if params == nil || params.ClientInfo == nil {
		return false
	}
	// Desktop has no native integration adapter, despite telemetry grouping
	// all Claude names under claude-code.
	if strings.Contains(strings.ToLower(params.ClientInfo.Name), "desktop") {
		return false
	}
	var host hostintegration.Host
	switch NormalizeClient(params.ClientInfo.Name, params.ClientInfo.Version).Name {
	case "claude-code":
		host = hostintegration.HostClaude
	case "codex":
		host = hostintegration.HostCodex
	case "gemini":
		host = hostintegration.HostGemini
	default:
		// Without an identifiable adapter, native installation cannot be
		// assumed. Preserve MCP delivery rather than remove the only source.
		return false
	}
	return !hostintegration.HostSupportsSkillsExtension(host)
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
	if adapter.remote {
		return adapter.remoteLocalSkill(ctx, skillID)
	}
	local, err := adapter.snapshots.Ensure(ctx, adapter.workspace, skillID)
	if err != nil {
		adapter.logger.Warn("local skill snapshot could not be exported", "skill_id", skillID, "error", err)
		local = app.LocalSkill{Status: app.LocalStatusUnavailable, ReasonCodes: []string{app.LocalReasonExportFailed}}
	}
	return &local
}

func (adapter *Server) readResource(ctx context.Context, request *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
	if request == nil || request.Params == nil || request.Params.URI == "" || len(request.Params.URI) > 4096 {
		return nil, invalidParams("invalid_uri", "A bounded skill resource URI is required.")
	}
	content, err := adapter.distribution.ReadResource(ctx, adapter.workspace, request.Params.URI)
	if err != nil {
		return nil, adapter.distributionRPCError(ctx, request.Session, err)
	}
	meta, refused, reasons := adapter.localResourceMeta(ctx, content.SkillID, content.Path)
	kind := mapResourceKind(content.Path)
	if refused {
		adapter.recordLoad(ctx, request.Session, app.SkillLoad{
			SkillID:      content.SkillID,
			ResourceKind: kind,
			Surface:      "resources_read",
			Blocked:      true,
			ReasonCodes:  reasons,
		})
		return nil, invalidParams("content_review_required", contentReviewRequiredMessage(content.SkillID))
	}
	adapter.recordLoad(ctx, request.Session, app.SkillLoad{
		SkillID:      content.SkillID,
		ResourceKind: kind,
		Surface:      "resources_read",
	})

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

// localResourceMeta maps a verified resource URI to its file in the local
// snapshot. refused is true when the skill awaits content review, in which
// case no resource, SKILL.md included, may be served.
func (adapter *Server) localResourceMeta(ctx context.Context, skillID, relative string) (meta mcp.Meta, refused bool, reasons []string) {
	if skillID == "" || skillID == systemskills.CuratorSkillID {
		return nil, false, nil
	}
	if adapter.remote {
		local := adapter.remoteLocalSkill(ctx, skillID)
		return nil, local.Status == app.LocalStatusReviewRequired, local.ReasonCodes
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
