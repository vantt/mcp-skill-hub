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
	ProfileAll:      {tools: 26, bytes: 117489, sha256: "a9ddbaa3c235a47bf43d82e55eaa361ec5c338781375a6df428fc5ad85f356dc"},
	ProfileRuntime:  {tools: 3, bytes: 16172, sha256: "b8640d7dbdb13d1701c85a1726f2f42202f79b01631c3034aa3384c99aa89c5e"},
	ProfileCuration: {tools: 23, bytes: 101354, sha256: "48b3e2a6aa49405aea75ef2e51517705b4cdc3644eff8c90c1fc1ec74776dbed"},
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
