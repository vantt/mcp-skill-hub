package mcpserver

import (
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
