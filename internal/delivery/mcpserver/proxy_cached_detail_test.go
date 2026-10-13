package mcpserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/skillruntime"
)

// A cache entry written by a hub that still sent the curation fields of the
// skill detail loads, and the answer drops those fields.
func TestProxyServesCachedDetailFromAnOlderHub(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte("---\nname: cached-skill\ndescription: Cached.\n---\n\n# Cached\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	detail := `{"skill_id":"cached-skill","name":"cached-skill","description":"Cached.","status":"active","path":"skills/c/cached-skill/SKILL.md",` +
		`"catalog_snapshot":"sha256:aa","content_digest":"sha256:bb","state_basis":"canonical","lifecycle_state":"active","routing_eligible":true,` +
		`"diverged":true,"changed_resources":["skills/c/cached-skill/SKILL.md"],"missing_resources":["skills/c/cached-skill/gone.md"],` +
		`"routing":{"operations":["review"],"triggers":["review cached"],"not_for":[],"min_scope":"multi_step"},"resources":[]}`
	cached := skillruntime.CachedSkill{
		Directory: directory,
		Metadata: skillruntime.BundleMetadata{
			Schema: skillruntime.BundleSchema, SkillID: "cached-skill", DepsKey: "0123456789abcdef", ManifestVersion: "sha256:cc",
			Files:  []skillruntime.BundleFile{{Path: "SKILL.md", Digest: "sha256:dd", Size: 10, Kind: "entrypoint"}},
			Detail: json.RawMessage(detail),
		},
	}
	proxy := &Proxy{hubURL: "https://hub.example", stateRoot: t.TempDir()}
	result, err := proxy.localResult(cached, "hit", "")
	if err != nil || result.IsError {
		t.Fatalf("localResult = %#v, %v", result, err)
	}
	var outcome struct {
		Result map[string]json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(result.StructuredContent.(json.RawMessage), &outcome); err != nil {
		t.Fatal(err)
	}
	for _, kept := range []string{"skill_id", "content", "content_digest", "lifecycle_state", "routing", "local", "remote"} {
		if _, present := outcome.Result[kept]; !present {
			t.Errorf("answer lacks %q: %v", kept, outcome.Result)
		}
	}
	for _, dropped := range []string{"diverged", "changed_resources", "missing_resources", "state_basis", "routing_eligible"} {
		if _, present := outcome.Result[dropped]; present {
			t.Errorf("answer still carries %q", dropped)
		}
	}
}
