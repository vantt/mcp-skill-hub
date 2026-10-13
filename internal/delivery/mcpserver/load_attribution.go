package mcpserver

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/systemskills"
	"github.com/vantt/skillhub-mcp/internal/telemetry"
)

// reviewBlock reports whether a skill's local state refuses its content until
// it is reviewed, with the reason codes to record.
func reviewBlock(local *app.LocalSkill) (blocked bool, reasons []string) {
	if local == nil || local.Status != app.LocalStatusReviewRequired {
		return false, nil
	}
	return true, local.ReasonCodes
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
	load.EventID = telemetry.NewEventID()
	var resolution app.LoadResolution
	var resolutionKnown bool
	if adapter.tracker != nil {
		load.ResolutionID, load.Attribution, load.CatalogSnapshot, load.PolicyRevision, load.Client = adapter.tracker.attributeDetails(session, load.SkillID)
		if adapter.remote {
			// A local load without a prior resolution is labelled "skillhub";
			// a remote one keeps its remote host label so it is not counted as
			// a local load.
			load.Client = adapter.sessionClient(session)
		}
		load.SessionIDHash = adapter.tracker.sessionHash(session)
		if !load.Blocked && load.ResourceKind == "entrypoint" {
			load.FirstActivation = adapter.tracker.markActivation(session, load.ResolutionID, load.SkillID)
		}
		if load.Attribution == "override" || load.Attribution == "after_no_skill" {
			if noted, ok := adapter.tracker.resolutionData(session, load.ResolutionID); ok {
				resolution, resolutionKnown = noted.loadResolution(), true
				load.TopKSkillIDs = noted.TopKSkillIDs
				load.TopKMatched = noted.TopKMatched
				load.TopKChannels = noted.TopKChannels
			}
		}
	}
	if resolutionKnown && load.FirstActivation {
		_ = app.RecordLoadCase(ctx, adapter.telemetry, load, resolution)
	}
	app.RecordSkillLoad(ctx, adapter.telemetry, adapter.workspace, load)
}
