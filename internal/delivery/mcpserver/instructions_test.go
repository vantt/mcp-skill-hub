package mcpserver

import (
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func profileToolNames(profile Profile) []string {
	collector := newToolCollector()
	(&Server{profile: profile}).registerTools(collector)
	return collector.names()
}

var backtickedToken = regexp.MustCompile("`([a-z_]+)`")

func TestInstructionsNameOnlyRegisteredTools(t *testing.T) {
	allNames := profileToolNames(ProfileAll)
	for _, profile := range []Profile{ProfileRuntime, ProfileCuration, ProfileAll} {
		names := profileToolNames(profile)
		text := serverInstructions(names, instructionOptions{skillsExtension: true, curatorSkill: true})
		for _, name := range allNames {
			named, registered := strings.Contains(text, "`"+name+"`"), slices.Contains(names, name)
			// Curation tools are described as a group, so only a runtime tool
			// must be named exactly when it is registered.
			if named && !registered || slices.Contains(runtimeToolNames, name) && named != registered {
				t.Errorf("profile %s: instructions name %s = %v, registered = %v", profile, name, named, registered)
			}
		}
	}
}

func TestInstructionParagraphsDeclareTheToolsTheyName(t *testing.T) {
	allNames := profileToolNames(ProfileAll)
	for _, paragraph := range runtimeParagraphs {
		for _, match := range backtickedToken.FindAllStringSubmatch(paragraph.text, -1) {
			if slices.Contains(allNames, match[1]) && !slices.Contains(paragraph.requires, match[1]) {
				t.Errorf("paragraph %q names %s without requiring it", paragraph.text, match[1])
			}
		}
	}
}

func TestInstructionsFitClientLimit(t *testing.T) {
	ascii := regexp.MustCompile(`^[\x00-\x7f]*$`)
	cases := map[string]string{
		"runtime with extension": serverInstructions(profileToolNames(ProfileRuntime), instructionOptions{skillsExtension: true, curatorSkill: true}),
		"runtime plain":          serverInstructions(profileToolNames(ProfileRuntime), instructionOptions{}),
		"curation":               serverInstructions(profileToolNames(ProfileCuration), instructionOptions{curatorSkill: true}),
	}
	for name, text := range cases {
		if text == "" || len(text) > 2048 || !ascii.MatchString(text) {
			t.Errorf("%s: %d bytes, ascii %v", name, len(text), ascii.MatchString(text))
		}
	}
}

func TestInitializeCarriesInstructions(t *testing.T) {
	for _, profile := range []Profile{ProfileRuntime, ProfileCuration} {
		_, server, err := New(newMCPWorkspace(t), io.Discard, profile)
		if err != nil {
			t.Fatal(err)
		}
		clientTransport, serverTransport := mcp.NewInMemoryTransports()
		serverSession, err := server.Connect(t.Context(), serverTransport, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = serverSession.Close() })
		session, err := mcp.NewClient(&mcp.Implementation{Name: "instructions", Version: "1"}, nil).Connect(t.Context(), clientTransport, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = session.Close() })
		want := serverInstructions(profileToolNames(profile), instructionOptions{skillsExtension: profile != ProfileCuration, curatorSkill: true})
		got := session.InitializeResult().Instructions
		if got != want || got == "" {
			t.Errorf("profile %s: initialize instructions %q, want %q", profile, got, want)
		}
		if profile == ProfileCuration && strings.Contains(got, "skill_resolve") {
			t.Errorf("curation instructions name skill_resolve: %q", got)
		}
	}
}

func TestProxyInitializeCarriesRuntimeInstructions(t *testing.T) {
	var requests atomic.Int32
	hub := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	t.Cleanup(hub.Close)
	proxied := startTestProxy(t, hub.URL, "token", t.TempDir(), nil)
	got := proxied.session.InitializeResult().Instructions
	if !strings.Contains(got, "skill_resolve") || strings.Contains(got, "skills/get") || strings.Contains(got, "system-curator") {
		t.Errorf("proxy instructions: %q", got)
	}
	if n := requests.Load(); n != 0 {
		t.Errorf("initialize sent %d requests to the hub", n)
	}
}
