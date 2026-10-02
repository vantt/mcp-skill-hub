package mcpserver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
)

func TestSourceImportMCPPreviewAndConfirm(t *testing.T) {
	t.Parallel()
	root := newEmptyMCPWorkspace(t)

	// Create an upstream filesystem source directory with a skill
	fixture := filepath.Join(root, "sources", "upstream")
	skillDir := filepath.Join(fixture, "mcp-agent-skill")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	skillMD := "---\nname: mcp-agent-skill\ndescription: An MCP agent skill\n---\n# MCP Agent Skill\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o600); err != nil {
		t.Fatal(err)
	}

	session := connectInMemoryServer(t, root)

	// 1. Add candidate
	intakeRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "source_intake_add",
		Arguments: map[string]any{
			"locator":         "sources/upstream",
			"reason":          "test import",
			"idempotency_key": "import-test-key",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var intakeOutcome toolOutcome[app.SourceCandidateResult]
	decodeStructuredContent(t, intakeRes, &intakeOutcome)
	candidateID := intakeOutcome.Result.Candidate.ID
	// 2. Triage candidate
	triageRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "source_triage",
		Arguments: map[string]any{
			"candidate_id": candidateID,
			"decision":     "accept",
			"source_id":    "mcp-source",
			"adapter":      "filesystem",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var triageOutcome toolOutcome[sourceTriageResult]
	decodeStructuredContent(t, triageRes, &triageOutcome)
	triagePins := triageOutcome.Result.Preview.Confirmation.Confirmation.Pins
	// 3. Confirm triage
	confirmTriageRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "source_triage",
		Arguments: map[string]any{
			"confirmation": map[string]any{
				"proposal_id":     triagePins.ProposalID,
				"proposal_digest": triagePins.ProposalDigest,
				"base_version":    triagePins.BaseVersion,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if confirmTriageRes.IsError {
		t.Fatal("expected triage confirm to succeed")
	}

	// 4. Preview import via MCP
	importPrevRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "source_import_preview",
		Arguments: map[string]any{
			"source_id": "mcp-source",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if importPrevRes.IsError {
		t.Fatal("expected source_import_preview to succeed")
	}
	var importProposal toolOutcome[app.SourceImportProposal]
	decodeStructuredContent(t, importPrevRes, &importProposal)

	if len(importProposal.Result.Discovered) != 1 {
		t.Fatalf("discovered = %d, want 1", len(importProposal.Result.Discovered))
	}
	if len(importProposal.Result.Importable) != 1 {
		t.Fatalf("importable = %d, want 1", len(importProposal.Result.Importable))
	}
	if importProposal.Result.Importable[0].TargetID != "mcp-agent-skill" {
		t.Fatalf("target_id = %s, want mcp-agent-skill", importProposal.Result.Importable[0].TargetID)
	}
	importPins := importProposal.Result.Confirmation.Confirmation.Pins
	if importPins.ProposalID == "" {
		t.Fatal("missing import proposal ID")
	}

	// 5. Confirm import via MCP
	importConfRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "source_import_confirm",
		Arguments: map[string]any{
			"proposal_id":     importPins.ProposalID,
			"proposal_digest": importPins.ProposalDigest,
			"base_version":    importPins.BaseVersion,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if importConfRes.IsError {
		t.Fatal("expected source_import_confirm to succeed")
	}
	var importResult toolOutcome[app.SourceImportResult]
	decodeStructuredContent(t, importConfRes, &importResult)

	if importResult.Result.ImportedCount != 1 {
		t.Fatalf("imported_count = %d, want 1", importResult.Result.ImportedCount)
	}
	if len(importResult.Result.ImportedIDs) != 1 || importResult.Result.ImportedIDs[0] != "mcp-agent-skill" {
		t.Fatalf("imported_ids = %v", importResult.Result.ImportedIDs)
	}

	// 6. Verify skill exists in draft state via skill_get
	getRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_get",
		Arguments: map[string]any{
			"skill_id": "mcp-agent-skill",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if getRes.IsError {
		t.Fatal("expected skill_get to succeed")
	}
	var getResult toolOutcome[skillGetResult]
	decodeStructuredContent(t, getRes, &getResult)
	if getResult.Result.Status != "draft" {
		t.Fatalf("status = %q, want draft", getResult.Result.Status)
	}

	// 7. Preview again via MCP: must show as skipped conflict (idempotent)
	rePrevRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "source_import_preview",
		Arguments: map[string]any{
			"source_id": "mcp-source",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var reProposal toolOutcome[app.SourceImportProposal]
	decodeStructuredContent(t, rePrevRes, &reProposal)
	if len(reProposal.Result.Importable) != 0 {
		t.Fatalf("re-preview importable = %d, want 0", len(reProposal.Result.Importable))
	}
	if len(reProposal.Result.Skipped) != 1 || !reProposal.Result.Skipped[0].Conflict {
		t.Fatalf("re-preview skipped = %#v", reProposal.Result.Skipped)
	}
}
