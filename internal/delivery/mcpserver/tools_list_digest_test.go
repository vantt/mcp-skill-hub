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
	ProfileAll:      {tools: 26, bytes: 115891, sha256: "17effd9c492d6346d4f281497eb448cf240b635e81e1a75d80fd4abedefc59f4"},
	ProfileRuntime:  {tools: 3, bytes: 15373, sha256: "294392dd6818604ee678e0a732471ffc4eca75b6dd5b2b0ca37790e2c926a0e2"},
	ProfileCuration: {tools: 23, bytes: 100555, sha256: "ba0386bd4108783119303c095a3c59a7508560effb88312209d56ca9c296f0df"},
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
