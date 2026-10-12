package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMigrationKeepsTrustVerdictAndUpstreamStatusOfThirdPartySkill(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := (WorkspaceService{}).Init(root, InitOptions{}); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, ".skillhub", "schema-version")
	if err := os.WriteFile(marker, []byte("2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	skillDir := filepath.Join(root, "skills", "default", "imported-upstream")
	if err := os.MkdirAll(filepath.Join(skillDir, ".meta"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# Imported\n\nCopied from a public repository.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := `schema_version: 1
id: imported-upstream
status: active
sources:
  - id: src-upstream
    kind: github
    repo: https://github.com/owner/repo
    ref: 0123456789abcdef0123456789abcdef01234567
    path: skills/imported-upstream
    roles: [upstream]
routing:
  triggers: [imported]
  not_for: [other]
  min_scope: single_step
`
	if err := os.WriteFile(filepath.Join(skillDir, ".meta", "skill.yaml"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}

	skillService := SkillService{}
	trustBefore, err := skillService.ContentTrustFor(t.Context(), root, "imported-upstream")
	if err != nil {
		t.Fatal(err)
	}
	upstreamBefore := upstreamStatusOf(t, root, "imported-upstream")
	if upstreamBefore != "pinned" {
		t.Fatalf("fixture upstream status before migration = %q, want pinned", upstreamBefore)
	}

	if _, err := (MigrationService{}).Migrate(t.Context(), root, 5, true); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	trustAfter, err := skillService.ContentTrustFor(t.Context(), root, "imported-upstream")
	if err != nil {
		t.Fatal(err)
	}
	if !trustBefore.ThirdParty || !reflect.DeepEqual(trustAfter, trustBefore) {
		t.Fatalf("trust verdict changed by migration:\nbefore=%#v\nafter=%#v", trustBefore, trustAfter)
	}
	if upstreamAfter := upstreamStatusOf(t, root, "imported-upstream"); upstreamAfter != upstreamBefore {
		t.Fatalf("upstream status changed by migration: %q to %q", upstreamBefore, upstreamAfter)
	}
}

func upstreamStatusOf(t *testing.T, root, skillID string) string {
	t.Helper()
	upstreams, err := ListSkillUpstream(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	for _, upstream := range upstreams {
		if upstream.SkillID == skillID {
			return upstream.Status
		}
	}
	t.Fatalf("skill %s missing from upstream listing %#v", skillID, upstreams)
	return ""
}
