package mcpserver

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/jsonrpc"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"gopkg.in/yaml.v3"
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

func TestResourceReadRefusesUnapprovedThirdPartyContent(t *testing.T) {
	t.Parallel()
	root := newMCPWorkspace(t)
	skillDir := filepath.Join(root, "skills", "core", "review-skill")
	if err := os.MkdirAll(filepath.Join(skillDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "scripts", "run.sh"), []byte("#!/bin/sh\necho run\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "notes.md"), []byte("# Notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(skillDir, ".meta", "skill.yaml")
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		metaPath = filepath.Join(skillDir, "skill.meta.yaml")
	}
	meta, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(meta, &doc); err != nil {
		t.Fatal(err)
	}
	doc["sources"] = []any{
		map[string]any{
			"id":         "upstream-src",
			"roles":      []string{"upstream"},
			"kind":       "github",
			"repository": "https://github.com/example/skills",
		},
	}
	doc["quality"] = map[string]any{
		"reviewed": false,
	}
	thirdParty, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metaPath, thirdParty, 0o644); err != nil {
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
	var scriptURI, notesURI, entrypointURI string
	for _, resource := range entry.Resources {
		switch {
		case strings.HasSuffix(resource.URI, "/scripts/run.sh"):
			scriptURI = resource.URI
		case strings.HasSuffix(resource.URI, "/notes.md"):
			notesURI = resource.URI
		case strings.HasSuffix(resource.URI, "/SKILL.md"):
			entrypointURI = resource.URI
		}
	}
	if scriptURI == "" || notesURI == "" || entrypointURI == "" {
		t.Fatalf("resources = %#v", entry.Resources)
	}
	for _, uri := range []string{scriptURI, notesURI, entrypointURI} {
		_, err = session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: uri})
		var rpcErr *jsonrpc.Error
		if !errors.As(err, &rpcErr) || !strings.Contains(string(rpcErr.Data), "content_review_required") || !strings.Contains(rpcErr.Message, "skillhub skill review review-skill") {
			t.Fatalf("unapproved resource %s read error = %v", uri, err)
		}
	}
	withheld := callSkillGet(t, session, "review-skill")
	if withheld.Content != "" || withheld.Local == nil || withheld.Local.Status != app.LocalStatusReviewRequired || withheld.Local.ReviewCommand != "skillhub skill review review-skill" {
		t.Fatalf("skill_get must withhold content of an unapproved skill: %#v", withheld)
	}
	if withheld.SkillID != "review-skill" || withheld.Name == "" || withheld.Description == "" || withheld.LifecycleState != "active" {
		t.Fatalf("skill_get must keep identity and lifecycle metadata: %#v", withheld.SkillDetail)
	}
	if _, err := os.Lstat(filepath.Join(root, "runtime", "cache", "skills")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a snapshot directory was created for an unapproved skill: %v", err)
	}

	// Approving the exact content digest releases every resource.
	review, err := (app.SkillService{}).ReviewSkill(t.Context(), root, "review-skill")
	if err != nil {
		t.Fatal(err)
	}
	approved, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	approvedMeta := strings.Replace(string(approved), "quality:\n", "quality:\n    content_reviewed_digest: "+review.ContentTrust.ContentDigest+"\n", 1)
	if approvedMeta == string(approved) {
		t.Fatalf("fixture metadata has no quality block:\n%s", approved)
	}
	if err := os.WriteFile(metaPath, []byte(approvedMeta), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (app.CatalogService{}).BuildCatalogGeneration(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	if released := callSkillGet(t, session, "review-skill"); !strings.Contains(released.Content, "#") || released.Local == nil || released.Local.Status != app.LocalStatusReady {
		t.Fatalf("approved skill_get = %#v", released)
	}
	if entrypoint, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: entrypointURI}); err != nil || len(entrypoint.Contents) != 1 {
		t.Fatalf("approved SKILL.md read = %#v, %v", entrypoint, err)
	}
	releasedNotes, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: notesURI})
	if err != nil || len(releasedNotes.Contents) != 1 || !strings.Contains(releasedNotes.Contents[0].Text, "# Notes") {
		t.Fatalf("approved notes read = %#v, %v", releasedNotes, err)
	}
	released, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: scriptURI})
	if err != nil || len(released.Contents) != 1 || !strings.Contains(released.Contents[0].Text, "echo run") {
		t.Fatalf("approved script read = %#v, %v", released, err)
	}
	if _, ok := released.Contents[0].Meta["io.skillhub/local_path"]; !ok {
		t.Fatalf("approved script _meta = %#v", released.Contents[0].Meta)
	}
}

func callSkillGet(t *testing.T, session *mcp.ClientSession, id string) skillGetResult {
	t.Helper()
	response, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "skill_get", Arguments: map[string]any{"skill_id": id}})
	if err != nil || response.IsError {
		t.Fatalf("skill_get(%s) = %#v, %v", id, response, err)
	}
	var outcome toolOutcome[skillGetResult]
	decodeStructuredContent(t, response, &outcome)
	if outcome.Result == nil {
		t.Fatalf("skill_get(%s) has no result", id)
	}
	return *outcome.Result
}
