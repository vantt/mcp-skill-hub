package mcpserver

import (
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/systemskills"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func TestCuratorSourcePerSessionClient(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	if _, err := (app.CatalogService{}).EnsureCatalog(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	adapter, server, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	entries, _, err := adapter.distribution.LookupSkills(t.Context(), root, []string{systemskills.CuratorSkillID})
	if err != nil {
		t.Fatal(err)
	}
	uri := entries[systemskills.CuratorSkillID].URI
	for _, tc := range []struct {
		name   string
		native bool
	}{
		{"Claude Code", true}, {" claude-code ", true}, {"Codex CLI", true}, {"codex", true}, {"Gemini CLI", true}, {"gemini", true},
		{"Cursor", false}, {"Claude Desktop", false}, {"inspector", false}, {"", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ct, st := mcp.NewInMemoryTransports()
			ss, err := server.Connect(t.Context(), st, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = ss.Close() })
			client := mcp.NewClient(&mcp.Implementation{Name: tc.name, Version: "1"}, nil)
			if err := mcp.AddSendingCustomMethod[*listSkillsParams, *listSkillsResult](client, "skills/list"); err != nil {
				t.Fatal(err)
			}
			if err := mcp.AddSendingCustomMethod[*getSkillParams, *getSkillResult](client, "skills/get"); err != nil {
				t.Fatal(err)
			}
			cs, err := client.Connect(t.Context(), ct, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cs.Close() })
			listed, err := mcp.CallCustomMethod[*listSkillsParams, *listSkillsResult](t.Context(), cs, "skills/list", &listSkillsParams{})
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, entry := range listed.Skills {
				if entry.URI == uri {
					found = true
				}
			}
			if found == tc.native {
				t.Fatalf("curator listed=%v native=%v", found, tc.native)
			}
			got, err := mcp.CallCustomMethod[*getSkillParams, *getSkillResult](t.Context(), cs, "skills/get", &getSkillParams{URI: uri})
			if tc.native {
				var rpcErr *jsonrpc.Error
				if !errors.As(err, &rpcErr) || !strings.Contains(string(rpcErr.Data), "skill_not_found") {
					t.Fatalf("native client skills/get: %v", err)
				}
			} else if err != nil || got.Skill.URI != uri {
				t.Fatalf("MCP client skills/get: %#v, %v", got, err)
			}
		})
	}
	listed, err := adapter.listSkills(t.Context(), nil, &listSkillsParams{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range listed.Skills {
		if entry.URI == uri {
			found = true
		}
	}
	if !found {
		t.Fatal("unidentified session lost curator")
	}
}
