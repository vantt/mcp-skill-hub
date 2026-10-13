package mcpserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/skillhub-mcp/internal/catalog"
)

func callLoadBenchSkillGet(t *testing.T, session *mcp.ClientSession) (map[string]any, skillGetResult) {
	t.Helper()
	response, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "skill_get", Arguments: map[string]any{"skill_id": loadBenchSkillID}})
	if err != nil || response.IsError {
		t.Fatalf("skill_get = %#v, %v", response, err)
	}
	var outcome toolOutcome[skillGetResult]
	decodeStructuredContent(t, response, &outcome)
	if outcome.Result == nil {
		t.Fatalf("skill_get has no result: %#v", response)
	}
	var raw struct {
		Result map[string]any `json:"result"`
	}
	decodeStructuredContent(t, response, &raw)
	return raw.Result, *outcome.Result
}

// Editing a skill file and rebuilding the catalog makes the next skill_get
// return the new content, its digest, and a new local folder named by it.
func TestSkillGetFollowsAnEditAfterRebuild(t *testing.T) {
	root := newLoadBenchWorkspace(t)
	_, session := connectLoadBenchSession(t, root)

	raw, before := callLoadBenchSkillGet(t, session)
	if before.Local == nil || before.Local.Path == "" || before.ContentDigest == "" {
		t.Fatalf("first skill_get lacks a local path or digest: %#v", before)
	}
	for _, dropped := range []string{"diverged", "changed_resources", "missing_resources", "state_basis", "routing_eligible"} {
		if _, present := raw[dropped]; present {
			t.Errorf("skill_get still carries %q", dropped)
		}
	}

	entrypoint := filepath.Join(root, "skills", "benchmark", loadBenchSkillID, "SKILL.md")
	original, err := os.ReadFile(entrypoint)
	if err != nil {
		t.Fatal(err)
	}
	edited := string(original) + "\nA line added after the first load.\n"
	if err := os.WriteFile(entrypoint, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := catalog.BuildCatalogGeneration(t.Context(), root, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}

	_, after := callLoadBenchSkillGet(t, session)
	if after.Content != edited || !strings.Contains(after.Content, "A line added after the first load.") {
		t.Fatalf("skill_get after the edit returned stale content")
	}
	if after.ContentDigest == before.ContentDigest || after.CatalogSnapshot == before.CatalogSnapshot {
		t.Fatalf("digest or snapshot unchanged after the edit: %q %q", after.ContentDigest, after.CatalogSnapshot)
	}
	if after.Local == nil || after.Local.Path == "" || after.Local.Path == before.Local.Path {
		t.Fatalf("local path after the edit = %v, before %q", after.Local, before.Local.Path)
	}
	if after.Local.ManifestVersion == before.Local.ManifestVersion {
		t.Fatalf("manifest version unchanged after the edit: %q", after.Local.ManifestVersion)
	}
	if !strings.Contains(filepath.Base(after.Local.Path), strings.TrimPrefix(after.Local.ManifestVersion, "sha256:")[:16]) {
		t.Fatalf("local folder %q does not carry the manifest digest %q", after.Local.Path, after.Local.ManifestVersion)
	}
	written, err := os.ReadFile(filepath.Join(after.Local.Path, "SKILL.md"))
	if err != nil || string(written) != edited {
		t.Fatalf("local SKILL.md = %q, %v", written, err)
	}
}
