package mcpserver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
)

func connectDistributionSession(t *testing.T, root string) *mcp.ClientSession {
	t.Helper()
	_, server, err := New(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	capabilities := &mcp.ClientCapabilities{}
	capabilities.AddExtension("io.modelcontextprotocol/skills", map[string]any{})
	client := mcp.NewClient(&mcp.Implementation{Name: "distribution-test", Version: "1"}, &mcp.ClientOptions{Capabilities: capabilities})
	if err := mcp.AddSendingCustomMethod[*listSkillsParams, *listSkillsResult](client, "skills/list"); err != nil {
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

func TestResourceWithInvalidUTF8IsDeliveredByteForByte(t *testing.T) {
	t.Parallel()
	root := newMCPWorkspace(t)
	invalid := []byte("id,value\n\xff\xfe,1\n")
	assets := filepath.Join(root, "skills", "core", "review-skill", "assets")
	if err := os.MkdirAll(assets, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assets, "data.csv"), invalid, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (app.CatalogService{}).BuildCatalogGeneration(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	session := connectDistributionSession(t, root)
	listed, err := mcp.CallCustomMethod[*listSkillsParams, *listSkillsResult](t.Context(), session, "skills/list", &listSkillsParams{})
	if err != nil {
		t.Fatal(err)
	}
	entry := findSkillEntryByName(t, listed.Skills, "review-skill")
	var advertised app.DistributedResource
	for _, resource := range entry.Resources {
		if filepath.Base(resource.URI) == "data.csv" {
			advertised = resource
		}
	}
	if advertised.URI == "" {
		t.Fatalf("data.csv is not advertised: %#v", entry.Resources)
	}
	result, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: advertised.URI})
	if err != nil || len(result.Contents) != 1 {
		t.Fatalf("resources/read = %#v, %v", result, err)
	}
	contents := result.Contents[0]
	delivered := contents.Blob
	if contents.Text != "" {
		delivered = []byte(contents.Text)
	}
	sum := sha256.Sum256(delivered)
	if got := "sha256:" + hex.EncodeToString(sum[:]); got != advertised.Digest {
		t.Fatalf("delivered digest %s does not match advertised digest %s (text=%q blob=%d bytes)", got, advertised.Digest, contents.Text, len(contents.Blob))
	}
	if string(delivered) != string(invalid) {
		t.Fatalf("delivered bytes = %q, want %q", delivered, invalid)
	}
}

func TestDistributionReportsStaleCatalogAsIndexStale(t *testing.T) {
	t.Parallel()
	root := newMCPWorkspace(t)
	session := connectDistributionSession(t, root)
	// Corrupt the pointer and break canonical files so catalog cannot be served or rebuilt
	currentPath := filepath.Join(root, "runtime", "catalog", "current.json")
	if err := os.WriteFile(currentPath, []byte(`{"generation":"gen_broken","database":"nonexistent.db","catalog_snapshot":"missing"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	badMeta := filepath.Join(root, "skills", "core", "review-skill", "skill.meta.yaml")
	if err := os.WriteFile(badMeta, []byte("invalid: yaml: ["), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := mcp.CallCustomMethod[*listSkillsParams, *listSkillsResult](t.Context(), session, "skills/list", &listSkillsParams{})
	var rpcError *jsonrpc.Error
	if !errors.As(err, &rpcError) {
		t.Fatalf("skills/list error = %v, want a JSON-RPC error", err)
	}
	var data map[string]string
	if json.Unmarshal(rpcError.Data, &data) != nil || data["code"] != "index_stale" {
		t.Fatalf("stale catalog error = code %d data %s, want code index_stale", rpcError.Code, rpcError.Data)
	}
}

func TestSkillsListOmitsUnservableSkillInsteadOfFailing(t *testing.T) {
	t.Parallel()
	root := newMCPWorkspace(t)
	bad := filepath.Join(root, "skills", "core", "renamed-skill")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		t.Fatal(err)
	}
	metadata := "schema_version: 1\nid: renamed-skill\nname: Renamed\nstatus: active\ndescription: Frontmatter name differs from the id.\nrouting:\n  operations: [research]\n  triggers: [renamed research]\n  not_for: [review code]\n  min_scope: multi_step\nquality:\n  reviewed: true\n"
	if err := os.WriteFile(filepath.Join(bad, "skill.meta.yaml"), []byte(metadata), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bad, "SKILL.md"), []byte("---\nname: renamed-skill\ndescription: \"\"\n---\n\n# Renamed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (app.CatalogService{}).BuildCatalogGeneration(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	session := connectDistributionSession(t, root)
	listed, err := mcp.CallCustomMethod[*listSkillsParams, *listSkillsResult](t.Context(), session, "skills/list", &listSkillsParams{})
	if err != nil {
		t.Fatalf("skills/list failed for the whole workspace: %v", err)
	}
	findSkillEntryByName(t, listed.Skills, "review-skill")
	for _, entry := range listed.Skills {
		if name, _ := entry.Frontmatter["name"].(string); name == "renamed-skill" {
			t.Fatalf("unservable skill was listed: %#v", entry)
		}
	}
}
