package mcpserver

import (
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/systemskills"
)

func connectNamedClient(t *testing.T, server *mcp.Server, name string) *mcp.ClientSession {
	t.Helper()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: name, Version: "1"}, nil)
	if err := mcp.AddSendingCustomMethod[*listSkillsParams, *listSkillsResult](client, "skills/list"); err != nil {
		t.Fatal(err)
	}
	if err := mcp.AddSendingCustomMethod[*getSkillParams, *getSkillResult](client, "skills/get"); err != nil {
		t.Fatal(err)
	}
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestSkillsGetServesTheCuratorToNativeSessionsThatDoNotListIt(t *testing.T) {
	_, server, err := New(newMCPWorkspace(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	other := connectNamedClient(t, server, "skillhub-test")
	otherListing, err := mcp.CallCustomMethod[*listSkillsParams, *listSkillsResult](t.Context(), other, "skills/list", &listSkillsParams{})
	if err != nil {
		t.Fatal(err)
	}
	curatorURI := findSkillEntryByName(t, otherListing.Skills, systemskills.CuratorSkillID).URI

	native := connectNamedClient(t, server, "claude-code")
	nativeListing, err := mcp.CallCustomMethod[*listSkillsParams, *listSkillsResult](t.Context(), native, "skills/list", &listSkillsParams{})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range nativeListing.Skills {
		if entry.Frontmatter["name"] == systemskills.CuratorSkillID {
			t.Fatal("a native session must not see the curator in skills/list")
		}
	}

	got, err := mcp.CallCustomMethod[*getSkillParams, *getSkillResult](t.Context(), native, "skills/get", &getSkillParams{URI: curatorURI})
	if err != nil {
		t.Fatalf("skills/get for the curator on a native session: %v", err)
	}
	if got.Skill.URI != curatorURI || got.Skill.Frontmatter["name"] != systemskills.CuratorSkillID || got.Skill.Local != nil {
		t.Fatalf("skills/get curator entry = %#v", got.Skill)
	}
}
