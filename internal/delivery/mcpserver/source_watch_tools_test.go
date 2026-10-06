package mcpserver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
)

func TestSourceWatchPreviewRejectsLocalFolders(t *testing.T) {
	t.Parallel()
	root := newEmptyMCPWorkspace(t)
	session := connectInMemoryServer(t, root)

	localCases := []string{
		"./sources/local",
		"../sources/local",
		filepath.Join(root, "sources"),
		"~/sources/repo",
		"sources-local",
	}

	for _, loc := range localCases {
		t.Run(loc, func(t *testing.T) {
			res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
				Name: "source_watch_preview",
				Arguments: map[string]any{
					"locator": loc,
				},
			})
			if err != nil {
				t.Fatalf("call tool error: %v", err)
			}
			if !res.IsError {
				t.Fatalf("expected source_watch_preview to reject local folder %q", loc)
			}
			var outcome toolOutcome[app.SourceProposal]
			decodeStructuredContent(t, res, &outcome)
			if outcome.Error == nil || (outcome.Error.Code != "local_watch_unsupported" && outcome.Error.Code != "invalid_request") {
				t.Fatalf("expected local_watch_unsupported or invalid_request for %q, got: %#v", loc, outcome.Error)
			}
		})
	}
}

func TestSourceWatchConfirmPinRequirements(t *testing.T) {
	t.Parallel()
	root := newEmptyMCPWorkspace(t)
	session := connectInMemoryServer(t, root)

	cases := []struct {
		name string
		args map[string]any
	}{
		{
			name: "missing all pins",
			args: map[string]any{},
		},
		{
			name: "missing proposal_digest and base_version",
			args: map[string]any{
				"proposal_id": "prp_watch123",
			},
		},
		{
			name: "missing base_version",
			args: map[string]any{
				"proposal_id":     "prp_watch123",
				"proposal_digest": "sha256:0000000000000000000000000000000000000000000000000000000000000000",
			},
		},
		{
			name: "empty strings",
			args: map[string]any{
				"proposal_id":     "   ",
				"proposal_digest": "   ",
				"base_version":    "   ",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
				Name:      "source_watch_confirm",
				Arguments: tc.args,
			})
			if err != nil {
				t.Fatalf("unexpected call error: %v", err)
			}
			if !res.IsError {
				t.Fatal("expected isError=true for missing pins")
			}
			if res.StructuredContent != nil {
				var outcome toolOutcome[app.SourceMutationResult]
				decodeStructuredContent(t, res, &outcome)
				if outcome.Error == nil || outcome.Error.Code != "invalid_request" {
					t.Fatalf("expected invalid_request for missing pins, got %#v", outcome.Error)
				}
			}
		})
	}
}

func TestSourceWatchPreviewRequiresSkillID(t *testing.T) {
	t.Parallel()
	root := newEmptyMCPWorkspace(t)
	session := connectInMemoryServer(t, root)

	res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "source_watch_preview",
		Arguments: map[string]any{
			"locator": "https://github.com/example/skills.git",
		},
	})
	if err != nil {
		t.Fatalf("call tool error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected error for source_watch_preview without skill_id")
	}
	var outcome toolOutcome[app.SourceProposal]
	decodeStructuredContent(t, res, &outcome)
	if outcome.Error == nil || outcome.Error.Code != "invalid_request" {
		t.Fatalf("expected invalid_request for missing skill_id, got %#v", outcome.Error)
	}
}

func TestSourceLinkAndUnwatchWorkflowViaMCP(t *testing.T) {
	t.Parallel()
	root := newEmptyMCPWorkspace(t)
	session := connectInMemoryServer(t, root)

	// Create a skill first
	skRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_create_preview",
		Arguments: map[string]any{
			"skill_id":    "test-skill",
			"name":        "Test Skill",
			"description": "Test skill description",
		},
	})
	if err != nil || skRes.IsError {
		t.Fatalf("create preview failed: %v", err)
	}
	var skPrev toolOutcome[app.SkillProposal]
	decodeStructuredContent(t, skRes, &skPrev)
	skPins := skPrev.Result.Confirmation.Confirmation.Pins

	skConfRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "skill_create_confirm",
		Arguments: map[string]any{
			"proposal_id":     skPins.ProposalID,
			"proposal_digest": skPins.ProposalDigest,
			"base_version":    skPins.BaseVersion,
		},
	})
	if err != nil || skConfRes.IsError {
		t.Fatalf("create confirm failed: %v", err)
	}

	// Seed source records
	seedSourceRecordForMCP(t, root, "src-detach", "https://github.com/example/repo1.git", "main")
	seedSourceRecordForMCP(t, root, "src-unwatch", "https://github.com/example/repo2.git", "main")

	// 1. Link attach preview
	linkAttachRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "source_link_preview",
		Arguments: map[string]any{
			"action":    "attach",
			"skill_id":  "test-skill",
			"source_id": "src-detach",
		},
	})
	if err != nil || linkAttachRes.IsError {
		t.Fatalf("source_link_preview attach failed: %v, %#v", err, linkAttachRes)
	}
	var attachOutcome toolOutcome[app.SourceProposal]
	decodeStructuredContent(t, linkAttachRes, &attachOutcome)
	attachPins := attachOutcome.Result.Confirmation.Confirmation.Pins

	// Confirm via source_watch_confirm
	confAttachRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "source_watch_confirm",
		Arguments: map[string]any{
			"proposal_id":     attachPins.ProposalID,
			"proposal_digest": attachPins.ProposalDigest,
			"base_version":    attachPins.BaseVersion,
		},
	})
	if err != nil || confAttachRes.IsError {
		t.Fatalf("source_watch_confirm for attach failed: %v, %#v", err, confAttachRes)
	}

	// Verify link was created
	linkPath := filepath.Join(root, "sources", "skills", "LINK-test-skill--src-detach.yaml")
	if _, err := os.Stat(linkPath); err != nil {
		t.Fatalf("link file missing after attach: %v", err)
	}

	// 2. Link detach preview
	linkDetachRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "source_link_preview",
		Arguments: map[string]any{
			"action":    "detach",
			"skill_id":  "test-skill",
			"source_id": "src-detach",
		},
	})
	if err != nil || linkDetachRes.IsError {
		t.Fatalf("source_link_preview detach failed: %v, %#v", err, linkDetachRes)
	}
	var detachOutcome toolOutcome[app.SourceProposal]
	decodeStructuredContent(t, linkDetachRes, &detachOutcome)
	detachPins := detachOutcome.Result.Confirmation.Confirmation.Pins

	// Confirm detach via source_watch_confirm
	confDetachRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "source_watch_confirm",
		Arguments: map[string]any{
			"proposal_id":     detachPins.ProposalID,
			"proposal_digest": detachPins.ProposalDigest,
			"base_version":    detachPins.BaseVersion,
		},
	})
	if err != nil || confDetachRes.IsError {
		t.Fatalf("source_watch_confirm for detach failed: %v, %#v", err, confDetachRes)
	}

	// 3. Unwatch preview
	unwatchRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "source_unwatch_preview",
		Arguments: map[string]any{
			"source_id": "src-unwatch",
		},
	})
	if err != nil || unwatchRes.IsError {
		t.Fatalf("source_unwatch_preview failed: %v, %#v", err, unwatchRes)
	}
	var unwatchOutcome toolOutcome[app.SourceProposal]
	decodeStructuredContent(t, unwatchRes, &unwatchOutcome)
	unwatchPins := unwatchOutcome.Result.Confirmation.Confirmation.Pins

	// Confirm unwatch via source_watch_confirm
	confUnwatchRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "source_watch_confirm",
		Arguments: map[string]any{
			"proposal_id":     unwatchPins.ProposalID,
			"proposal_digest": unwatchPins.ProposalDigest,
			"base_version":    unwatchPins.BaseVersion,
		},
	})
	if err != nil || confUnwatchRes.IsError {
		t.Fatalf("source_watch_confirm for unwatch failed: %v, %#v", err, confUnwatchRes)
	}

	// Verify source was deleted from catalog
	if _, err := os.Stat(filepath.Join(root, "sources", "catalog", "src-unwatch.yaml")); !os.IsNotExist(err) {
		t.Fatalf("source record still exists after unwatch confirmation")
	}

	// 4. source_watch_confirm with an onboarding (source_onboard) proposal ID returns invalid_request
	onboardPropID := "PROP-test-onboard"
	setProposalInWorkspace(t, root, onboardPropID, "source_onboard")
	badConfRes, err := session.CallTool(t.Context(), &mcp.CallToolParams{
		Name: "source_watch_confirm",
		Arguments: map[string]any{
			"proposal_id":     onboardPropID,
			"proposal_digest": "sha256:1111111111111111111111111111111111111111111111111111111111111111",
			"base_version":    "sha256:2222222222222222222222222222222222222222222222222222222222222222",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !badConfRes.IsError {
		t.Fatalf("expected source_watch_confirm to reject source_onboard proposal")
	}
}
