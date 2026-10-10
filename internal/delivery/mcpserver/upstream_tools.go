package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
)

const webUIReviewUpdateHint = "Open the skill in the Skill Hub WebUI → Sources tab → Review update."

func (adapter *Server) registerUpstreamTools(server *mcp.Server) {
	addTool(server, &mcp.Tool{
		Name:        "skill_upstream_status",
		Title:       "Get skill upstream status",
		Description: "Report upstream repository status, drift, and changed file counts for tracked skills without network access. Tell the user to run `skillhub skill update <id>` to apply updates.",
		Annotations: annotations(true, false, false, false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, input skillUpstreamStatusInput) (*mcp.CallToolResult, toolOutcome[skillUpstreamStatusResult], error) {
		skillID := strings.TrimSpace(input.SkillID)
		if skillID != "" {
			upstream, err := app.GetSkillUpstream(ctx, adapter.workspace, skillID)
			if err != nil {
				return failure[skillUpstreamStatusResult](err)
			}
			item := toSkillUpstreamItem(upstream)
			return success(skillUpstreamStatusResult{
				Skill:  &item,
				Skills: []SkillUpstreamItem{item},
			})
		}

		allUpstream, err := app.ListSkillUpstream(ctx, adapter.workspace)
		if err != nil {
			return failure[skillUpstreamStatusResult](err)
		}
		items := make([]SkillUpstreamItem, 0, len(allUpstream))
		for _, u := range allUpstream {
			items = append(items, toSkillUpstreamItem(u))
		}
		return success(skillUpstreamStatusResult{
			Skills: items,
		})
	})

}

func toSkillUpstreamItem(sk app.SkillUpstream) SkillUpstreamItem {
	files := make([]app.SkillUpstreamFile, 0, len(sk.Files))
	for _, f := range sk.Files {
		files = append(files, app.SkillUpstreamFile{
			Path:   cleanPrintable(f.Path),
			Status: cleanPrintable(f.Status),
		})
	}
	var webUIHint string
	if sk.Status == "update_available" || sk.Status == "diverged" {
		webUIHint = webUIReviewUpdateHint
	}
	nextAction := sk.NextAction
	if sk.Status == "update_available" || sk.Status == "diverged" {
		nextAction = "skillhub skill update " + sk.SkillID
	}
	return SkillUpstreamItem{
		SkillID:           cleanPrintable(sk.SkillID),
		SourceID:          cleanPrintable(sk.SourceID),
		Repository:        cleanPrintable(sk.Repository),
		Ref:               cleanPrintable(sk.Ref),
		Path:              cleanPrintable(sk.Path),
		BaseCommit:        cleanPrintable(sk.BaseCommit),
		LatestCommit:      cleanPrintable(sk.LatestCommit),
		LatestCommittedAt: sk.LatestCommittedAt,
		ChangedFiles:      sk.ChangedFiles,
		Files:             files,
		Local:             cleanPrintable(sk.Local),
		Status:            cleanPrintable(sk.Status),
		CheckedAt:         sk.CheckedAt,
		Error:             cleanPrintable(sk.Error),
		NextAction:        cleanPrintable(nextAction),
		WebUIHint:         webUIHint,
	}
}

func cleanPrintable(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if unicode.IsPrint(r) && r != '\t' && r != '\n' && r != '\r' {
			sb.WriteRune(r)
		} else {
			fmt.Fprintf(&sb, "\\x%02x", r)
		}
	}
	return sb.String()
}
