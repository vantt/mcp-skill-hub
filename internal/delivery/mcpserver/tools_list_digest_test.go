package mcpserver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// toolsListDigests pins the exact serialized tools/list result of each profile:
// names, descriptions, annotations, and the derived input and output schemas.
// A change to schema derivation or shaping that alters what a client sees must
// update these values on purpose.
var toolsListDigests = map[Profile]struct {
	tools  int
	bytes  int
	sha256 string
}{
	ProfileAll:      {tools: 26, bytes: 116692, sha256: "0be24ec641b74c0fae141463657ee96d3ddb990d8cde85c825531aec42de3e7f"},
	ProfileRuntime:  {tools: 3, bytes: 15305, sha256: "defbfe91b21d769f0c73979dfc23b949ae16d2b7a51c33d008bc87c1718739f1"},
	ProfileCuration: {tools: 23, bytes: 101424, sha256: "99613679d56a430f50fc3cfc8a5918c257c15d40e3501a4e11805842c7ea3343"},
}

func serializeToolsList(t testing.TB, tools []*mcp.Tool) (int, string) {
	t.Helper()
	encoded, err := json.Marshal(&mcp.ListToolsResult{Tools: tools})
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(encoded)
	return len(encoded), hex.EncodeToString(sum[:])
}

func TestToolsListSerializationIsStablePerProfile(t *testing.T) {
	for profile, want := range toolsListDigests {
		t.Run(string(profile), func(t *testing.T) {
			tools := listProfileTools(t, profile).Tools
			size, digest := serializeToolsList(t, tools)
			if len(tools) != want.tools || size != want.bytes || digest != want.sha256 {
				t.Fatalf("tools/list for %s changed: tools=%d bytes=%d sha256=%s; pinned tools=%d bytes=%d sha256=%s", profile, len(tools), size, digest, want.tools, want.bytes, want.sha256)
			}
		})
	}
}

func TestProxyRuntimeToolsMatchHubRuntimeProfile(t *testing.T) {
	proxyTools, err := runtimeToolDefinitions()
	if err != nil {
		t.Fatal(err)
	}
	proxySize, proxyDigest := serializeToolsList(t, proxyTools)
	hubSize, hubDigest := serializeToolsList(t, listProfileTools(t, ProfileRuntime).Tools)
	if proxySize != hubSize || proxyDigest != hubDigest {
		t.Fatalf("proxy runtime tools (%d bytes, %s) differ from the runtime profile (%d bytes, %s)", proxySize, proxyDigest, hubSize, hubDigest)
	}
}

func TestToolsListUnchangedOnceEveryToolIsRegisteredInFull(t *testing.T) {
	for profile, want := range toolsListDigests {
		t.Run(string(profile), func(t *testing.T) {
			adapter, server, err := New(newMCPWorkspace(t), nil, profile)
			if err != nil {
				t.Fatal(err)
			}
			if err := adapter.tools.materializeAll(); err != nil {
				t.Fatalf("a tool of the %s profile cannot be registered in full: %v", profile, err)
			}
			client := mcp.NewClient(&mcp.Implementation{Name: "digest", Version: "1"}, nil)
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
			listed, err := session.ListTools(t.Context(), nil)
			if err != nil {
				t.Fatal(err)
			}
			size, digest := serializeToolsList(t, listed.Tools)
			if len(listed.Tools) != want.tools || size != want.bytes || digest != want.sha256 {
				t.Fatalf("tools/list after full registration: tools=%d bytes=%d sha256=%s; pinned tools=%d bytes=%d sha256=%s", len(listed.Tools), size, digest, want.tools, want.bytes, want.sha256)
			}
		})
	}
}
