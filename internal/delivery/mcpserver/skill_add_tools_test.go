package mcpserver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
)

func TestSkillAddPreviewRejectsRawLocalLocators(t *testing.T) {
	t.Parallel()
	root := newEmptyMCPWorkspace(t)
	session := connectInMemoryServer(t, root)

	// Create a dummy local folder that would exist if accessed
	dummyPath := filepath.Join(root, "local-folder-skill")
	_ = os.MkdirAll(dummyPath, 0o755)
	_ = os.WriteFile(filepath.Join(dummyPath, "SKILL.md"), []byte("---\nname: test\n---\n"), 0o644)

	localLocators := []string{
		"./local-folder-skill",
		"../local-folder-skill",
		dummyPath,
		"~/skills/my-skill",
		"local-folder-skill",
		"file://" + dummyPath,
		"https://gitlab.com/owner/repo",
	}

	for _, loc := range localLocators {
		t.Run(loc, func(t *testing.T) {
			res, err := session.CallTool(t.Context(), &mcp.CallToolParams{
				Name: "skill_add_preview",
				Arguments: map[string]any{
					"locator": loc,
				},
			})
			if err != nil {
				t.Fatalf("call tool error: %v", err)
			}
			if !res.IsError {
				t.Fatalf("expected skill_add_preview to reject raw/local/non-github locator %q", loc)
			}
			var outcome toolOutcome[app.SkillAddProposal]
			decodeStructuredContent(t, res, &outcome)
			if outcome.Error == nil || outcome.Error.Code != "invalid_request" {
				t.Fatalf("expected invalid_request error for %q, got: %#v", loc, outcome.Error)
			}
		})
	}

	// Verify nothing was written to workspace skills
	entries, _ := os.ReadDir(filepath.Join(root, "skills"))
	for _, entry := range entries {
		subEntries, _ := os.ReadDir(filepath.Join(root, "skills", entry.Name()))
		if len(subEntries) > 0 {
			t.Fatalf("expected 0 skills written after rejected previews, found in %s: %d", entry.Name(), len(subEntries))
		}
	}
}

func TestSkillAddConfirmPinRequirements(t *testing.T) {
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
				"proposal_id": "prp_test123",
			},
		},
		{
			name: "missing base_version",
			args: map[string]any{
				"proposal_id":     "prp_test123",
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
				Name:      "skill_add_confirm",
				Arguments: tc.args,
			})
			if err != nil {
				t.Fatalf("unexpected call error: %v", err)
			}
			if !res.IsError {
				t.Fatal("expected isError=true for missing pins")
			}
			if res.StructuredContent != nil {
				var outcome toolOutcome[app.SkillAddResult]
				decodeStructuredContent(t, res, &outcome)
				if outcome.Error == nil || outcome.Error.Code != "invalid_request" {
					t.Fatalf("expected invalid_request for missing pins, got %#v", outcome.Error)
				}
			}
		})
	}
}
