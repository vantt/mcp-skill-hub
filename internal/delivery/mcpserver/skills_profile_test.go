package mcpserver

import (
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/version"
)

const skillsExtensionKey = "io.modelcontextprotocol/skills"

func connectSkillsClient(t *testing.T, root string, profile Profile) *mcp.ClientSession {
	t.Helper()
	_, server, err := New(root, nil, profile)
	if err != nil {
		t.Fatal(err)
	}
	capabilities := &mcp.ClientCapabilities{}
	capabilities.AddExtension(skillsExtensionKey, map[string]any{})
	client := mcp.NewClient(&mcp.Implementation{Name: "skills-profile-test", Version: "1"}, &mcp.ClientOptions{Capabilities: capabilities})
	if err := mcp.AddSendingCustomMethod[*listSkillsParams, *listSkillsResult](client, "skills/list"); err != nil {
		t.Fatal(err)
	}
	if err := mcp.AddSendingCustomMethod[*getSkillParams, *getSkillResult](client, "skills/get"); err != nil {
		t.Fatal(err)
	}
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(t.Context(), serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	session, err := client.Connect(t.Context(), clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestSkillsExtensionIsServedOnlyByRuntimeAndAllProfiles(t *testing.T) {
	t.Parallel()
	root := newBundleHubFixture(t)
	for _, test := range []struct {
		profile Profile
		serves  bool
	}{{ProfileRuntime, true}, {ProfileAll, true}, {ProfileCuration, false}} {
		t.Run(string(test.profile), func(t *testing.T) {
			t.Parallel()
			session := connectSkillsClient(t, root, test.profile)
			_, declared := session.InitializeResult().Capabilities.Extensions[skillsExtensionKey]
			if declared != test.serves {
				t.Fatalf("extension declared = %v, want %v", declared, test.serves)
			}
			result, err := mcp.CallCustomMethod[*listSkillsParams, *listSkillsResult](t.Context(), session, "skills/list", &listSkillsParams{})
			if !test.serves {
				var rpcErr *jsonrpc.Error
				if !errors.As(err, &rpcErr) || rpcErr.Code != jsonrpc.CodeMethodNotFound {
					t.Fatalf("curation skills/list = %v, want method not found", err)
				}
				return
			}
			if err != nil || len(result.Skills) == 0 {
				t.Fatalf("skills/list = %#v, %v", result, err)
			}
		})
	}
}

func TestServerReportsTheBuildVersion(t *testing.T) {
	t.Parallel()
	session := connectSkillsClient(t, newBundleHubFixture(t), ProfileRuntime)
	got := session.InitializeResult().ServerInfo
	if got == nil || got.Version != version.Current().Version {
		t.Fatalf("serverInfo = %#v, want version %q", got, version.Current().Version)
	}
}

func TestListingOmitsSkillsAwaitingReviewButReadsStayRefused(t *testing.T) {
	root := newBundleHubFixture(t)
	editFixtureMeta(t, root, "test-audit", markFixtureThirdParty)
	session := connectSkillsClient(t, root, ProfileRuntime)

	listed, err := mcp.CallCustomMethod[*listSkillsParams, *listSkillsResult](t.Context(), session, "skills/list", &listSkillsParams{})
	if err != nil {
		t.Fatal(err)
	}
	var uris []string
	for _, entry := range listed.Skills {
		uris = append(uris, entry.URI)
		if strings.Contains(entry.URI, "/test-audit/") {
			t.Fatalf("skill awaiting review is listed: %s", entry.URI)
		}
	}
	if !strings.Contains(strings.Join(uris, " "), "/herdr-cook-plan/") {
		t.Fatalf("a reviewed skill is missing from the listing: %v", uris)
	}

	uri := heldSkillEntry(t, root, "test-audit").URI
	read, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: uri})
	if read != nil || err == nil || !strings.Contains(err.Error(), "has not been approved") {
		t.Fatalf("resources/read of a skill awaiting review = %#v, %v", read, err)
	}
	got, err := mcp.CallCustomMethod[*getSkillParams, *getSkillResult](t.Context(), session, "skills/get", &getSkillParams{URI: uri})
	if err != nil || got.Skill.Local == nil || got.Skill.Local.Status != app.LocalStatusReviewRequired {
		t.Fatalf("skills/get of a skill awaiting review = %#v, %v", got, err)
	}
}

// heldSkillEntry builds the entry of a skill from the hub directly, for tests
// about skills that skills/list no longer shows.
func heldSkillEntry(t *testing.T, root, id string) skillEntry {
	t.Helper()
	held, _, err := (app.DistributionService{}).LookupSkills(t.Context(), root, []string{id})
	if err != nil {
		t.Fatal(err)
	}
	entry, ok := held[id]
	if !ok {
		t.Fatalf("skill %s is not distributed", id)
	}
	return toSkillEntry(entry)
}
