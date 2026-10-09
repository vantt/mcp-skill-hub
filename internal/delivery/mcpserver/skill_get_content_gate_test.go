package mcpserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	"gopkg.in/yaml.v3"
)

// setSkillMeta rewrites the review-skill manifest with replace and rebuilds
// the catalog.
func setSkillMeta(t *testing.T, root string, mutate func(map[string]any)) string {
	t.Helper()
	metaPath := filepath.Join(root, "skills", "core", "review-skill", ".meta", "skill.yaml")
	if _, err := os.Stat(metaPath); os.IsNotExist(err) {
		metaPath = filepath.Join(root, "skills", "core", "review-skill", "skill.meta.yaml")
	}
	meta, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(meta, &doc); err != nil {
		t.Fatal(err)
	}
	mutate(doc)
	updated, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metaPath, updated, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (app.CatalogService{}).BuildCatalogGeneration(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	return metaPath
}

func TestSkillGetWithholdsUnapprovedThirdPartyContentInEveryLifecycleState(t *testing.T) {
	t.Parallel()
	for _, state := range []string{"draft", "deprecated", "archived"} {
		t.Run(state, func(t *testing.T) {
			t.Parallel()
			root := newMCPWorkspace(t)
			setSkillMeta(t, root, func(doc map[string]any) {
				doc["status"] = state
				doc["sources"] = []any{
					map[string]any{
						"id":         "upstream-skill",
						"roles":      []string{"upstream"},
						"kind":       "github",
						"repository": "https://github.com/example/skills",
					},
				}
				doc["quality"] = map[string]any{
					"reviewed": false,
				}
			})
			session := connectDistributionSession(t, root)
			withheld := callSkillGet(t, session, "review-skill")
			if withheld.Content != "" || withheld.Local == nil || withheld.Local.Status != app.LocalStatusReviewRequired || withheld.Local.ReviewCommand != "skillhub skill review review-skill" || len(withheld.Local.ReasonCodes) == 0 {
				t.Fatalf("%s third-party skill must withhold content: %#v", state, withheld)
			}
			if withheld.LifecycleState != state || withheld.Name == "" || withheld.Description == "" {
				t.Fatalf("identity and lifecycle must stay: %#v", withheld.SkillDetail)
			}
			if withheld.Local.Path != "" || len(withheld.Local.Resources) != 0 || withheld.Local.Env != nil {
				t.Fatalf("withheld skill leaked local paths: %#v", withheld.Local)
			}
		})
	}
}

func TestSkillGetServesDraftContentForApprovedAndLocalSkills(t *testing.T) {
	t.Parallel()
	root := newMCPWorkspace(t)
	setSkillMeta(t, root, func(doc map[string]any) {
		doc["status"] = "draft"
		doc["sources"] = []any{
			map[string]any{
				"id":         "upstream-skill",
				"roles":      []string{"upstream"},
				"kind":       "github",
				"repository": "https://github.com/example/skills",
			},
		}
		doc["quality"] = map[string]any{
			"reviewed": false,
		}
	})
	session := connectDistributionSession(t, root)
	if got := callSkillGet(t, session, "review-skill"); got.Content != "" {
		t.Fatalf("unapproved draft returned content: %#v", got)
	}

	review, err := (app.SkillService{}).ReviewSkill(t.Context(), root, "review-skill")
	if err != nil {
		t.Fatal(err)
	}
	setSkillMeta(t, root, func(doc map[string]any) {
		doc["quality"] = map[string]any{
			"reviewed":                true,
			"content_reviewed_digest": review.ContentTrust.ContentDigest,
		}
	})
	approved := callSkillGet(t, session, "review-skill")
	if !strings.Contains(approved.Content, "# Review") || (approved.Local != nil && approved.Local.Status == app.LocalStatusReviewRequired) {
		t.Fatalf("approved draft must return content: %#v", approved)
	}

	// A local-folder skill is trusted by design, so its draft is readable.
	setSkillMeta(t, root, func(doc map[string]any) {
		delete(doc, "sources")
		delete(doc, "provenance")
		doc["quality"] = map[string]any{
			"reviewed": false,
		}
	})
	if got := callSkillGet(t, session, "review-skill"); !strings.Contains(got.Content, "# Review") {
		t.Fatalf("local draft must return content: %#v", got)
	}
}

func TestMCPResponsesNeverContainStoredEnvValues(t *testing.T) {
	t.Parallel()
	const sentinel = "mcp-sentinel-secret-91be44"
	root := newMCPWorkspace(t)
	if _, err := (app.SkillEnvService{}).Set(t.Context(), root, "review-skill", "REVIEW_TOKEN", sentinel); err != nil {
		t.Fatal(err)
	}
	session := connectDistributionSession(t, root)

	got := callSkillGet(t, session, "review-skill")
	if got.Local == nil || got.Local.Env[skillruntime.EnvConfigDir] != filepath.Join(root, "runtime", "config", "review-skill") {
		t.Fatalf("skill_get must export the config directory: %#v", got.Local)
	}
	listed, err := mcp.CallCustomMethod[*listSkillsParams, *listSkillsResult](t.Context(), session, "skills/list", &listSkillsParams{})
	if err != nil {
		t.Fatal(err)
	}
	entry := findSkillEntryByName(t, listed.Skills, "review-skill")
	responses := []any{got, listed}
	for _, resource := range entry.Resources {
		read, err := session.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: resource.URI})
		if err != nil {
			t.Fatal(err)
		}
		responses = append(responses, read)
	}
	resolved, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "skill_resolve", Arguments: map[string]any{
		"schema_version": "1", "request_id": "REQ-env-sentinel",
		"task": map[string]any{"description": "review changed code", "scope": "multi_step"}, "operation": "review",
	}})
	if err != nil || resolved.IsError {
		t.Fatalf("skill_resolve = %#v, %v", resolved, err)
	}
	responses = append(responses, resolved)
	for _, response := range responses {
		encoded, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), sentinel) {
			t.Fatalf("an MCP response leaks a stored value: %s", encoded)
		}
	}
}
