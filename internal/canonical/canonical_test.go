package canonical

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

func TestValidateHappyPathAndEntityFailures(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	issues, err := Validate(root)
	if err != nil || len(issues) != 0 {
		t.Fatalf("empty initialized workspace is invalid: %#v, %v", issues, err)
	}
	for _, relative := range []string{"sources/catalog/source-a.yaml", "sources/intake/duplicate.yaml", "sources/skills/broken.yaml"} {
		contents, err := os.ReadFile(filepath.Join("..", "..", "testdata", "workspaces", "invalid-entities", filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		write(t, root, relative, string(contents))
	}
	issues, err = Validate(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 2 {
		t.Fatalf("want duplicate and broken reference, got %#v", issues)
	}
}

func TestValidateStrictYAMLAndPluralReferences(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	write(t, root, "sources/catalog/source.yaml", "id: SRC-1\n")
	write(t, root, "sources/skills/link.yaml", "id: LINK-1\nsource_ids:\n  - SRC-1\n  - SRC-MISSING\n")
	write(t, root, "sources/intake/malformed.yaml", "id: [unterminated\n")
	write(t, root, "sources/intake/wrong-shape.yaml", "id: CANDIDATE-1\nsource_ids: SRC-1\n")
	issues, err := Validate(root)
	if err != nil {
		t.Fatal(err)
	}
	joined := issueMessages(issues)
	for _, expected := range []string{
		"broken reference source_ids: \"SRC-MISSING\" does not exist",
		"malformed.yaml: invalid YAML:",
		"wrong-shape.yaml: invalid YAML: source_ids must be a sequence of non-empty strings",
	} {
		if !strings.Contains(joined, expected) {
			t.Errorf("validation issues missing %q:\n%s", expected, joined)
		}
	}
}

func TestValidateRejectsCanonicalSourceCredentialsAndOperationalFields(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	write(t, root, "sources/catalog/unsafe.yaml", "schema_version: 1\nid: unsafe\nadapter: living-http\nlocator:\n  url: https://user:password@example.com/doc\nstatus: watching\nlast_checked_at: 2026-09-29T00:00:00Z\n")
	issues, err := Validate(root)
	if err != nil {
		t.Fatal(err)
	}
	joined := issueMessages(issues)
	if !strings.Contains(joined, "last_checked_at") && !strings.Contains(joined, "invalid source locator") {
		t.Fatalf("unsafe source policy was accepted: %s", joined)
	}
}

func TestValidateRejectsOrphanAndIncompleteActiveSkills(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	write(t, root, "skills/software/orphan/SKILL.md", "# Orphan\n")
	write(t, root, "skills/software/incomplete/skill.meta.yaml", "schema_version: 1\nid: incomplete\nname: Incomplete\nstatus: active\ndescription: Missing routing requirements.\n")
	write(t, root, "skills/software/incomplete/SKILL.md", "# Incomplete\n")
	issues, err := Validate(root)
	if err != nil {
		t.Fatal(err)
	}
	joined := issueMessages(issues)
	for _, expected := range []string{"orphan/skill.meta.yaml: skill metadata is missing", "active skill routing metadata is required"} {
		if !strings.Contains(joined, expected) {
			t.Errorf("validation issues missing %q:\n%s", expected, joined)
		}
	}
}

func TestValidateRejectsReservedSystemCuratorIDForEveryStatus(t *testing.T) {
	for _, status := range []string{"draft", "active", "deprecated", "archived"} {
		t.Run(status, func(t *testing.T) {
			metadata := "schema_version: 1\nid: system-curator\nname: Workspace Curator\nstatus: " + status + "\ndescription: Attempts to shadow the bundled skill.\nrouting:\n  triggers: [curate workspace]\n  not_for: [ordinary work]\n  min_scope: multi_step\n"
			_, issues := validateSkillMetadata("skills/core/system-curator/skill.meta.yaml", []byte(metadata))
			joined := issueMessages(issues)
			if !strings.Contains(joined, `skill id "system-curator" is reserved for the bundled system skill; choose a different workspace skill id`) {
				t.Fatalf("reserved ID was accepted for status %q: %s", status, joined)
			}
		})
	}
}

func TestValidateRoutingRelationshipsRequireCanonicalPolicyFields(t *testing.T) {
	base := "schema_version: 1\nid: owner\nname: Owner\nstatus: active\ndescription: Route work.\nrouting:\n  triggers: [route work]\n  not_for: []\n  min_scope: multi_step\nquality:\n  routing_review_rationale: reviewed\n"
	for name, suffix := range map[string]string{
		"support-role":       "  supporting:\n    - skill: helper\n      when: {operation: review}\n      role: arbitrary prose\n      activation: on-demand\n",
		"support-operation":  "  supporting:\n    - skill: helper\n      when: {operation: arbitrary}\n      role: validation\n      activation: on-demand\n",
		"support-activation": "  supporting:\n    - skill: helper\n      when: {operation: review}\n      role: validation\n      activation: always\n",
		"equivalence-policy": "  equivalent_to:\n    - skill: helper\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, issues := validateSkillMetadata("skills/core/owner/skill.meta.yaml", []byte(strings.Replace(base, "quality:", suffix+"quality:", 1)))
			if len(issues) == 0 {
				t.Fatal("invalid relationship metadata accepted")
			}
		})
	}
	valid := "  supporting:\n    - skill: helper\n      when: {operation: review}\n      role: validation\n      activation: on-demand\n  equivalent_to:\n    - skill: helper-two\n      preference: self\n      version_policy: latest-reviewed\n"
	_, issues := validateSkillMetadata("skills/core/owner/skill.meta.yaml", []byte(strings.Replace(base, "quality:", valid+"quality:", 1)))
	if len(issues) != 0 {
		t.Fatalf("canonical relationships rejected: %#v", issues)
	}
}

func TestValidateDetectsUnmergedGitIndex(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	path := "sources/catalog/conflict.yaml"
	write(t, root, path, "id: SRC-CONFLICT\n")
	output, err := exec.Command("git", "-C", root, "hash-object", "-w", path).Output()
	if err != nil {
		t.Fatalf("hash conflict fixture: %v", err)
	}
	blob := strings.TrimSpace(string(output))
	indexInfo := strings.Join([]string{
		"100644 " + blob + " 1\t" + path,
		"100644 " + blob + " 2\t" + path,
		"100644 " + blob + " 3\t" + path,
	}, "\n") + "\n"
	command := exec.Command("git", "-C", root, "update-index", "--index-info")
	command.Stdin = strings.NewReader(indexInfo)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create index conflict: %v: %s", err, output)
	}
	issues, err := Validate(root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(issueMessages(issues), path+": unresolved Git index conflict") {
		t.Fatalf("unmerged index conflict was not reported: %#v", issues)
	}
}

func TestValidateRejectsSymlinkEscape(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.yaml")
	write(t, filepath.Dir(outside), filepath.Base(outside), "id: OUTSIDE\n")
	if err := os.Symlink(outside, filepath.Join(root, "sources", "catalog", "escape.yaml")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	issues, err := Validate(root)
	if err != nil || len(issues) == 0 {
		t.Fatalf("symlink was not reported: %#v, %v", issues, err)
	}
}

func TestScanRejectsObservedConcurrentModification(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	write(t, root, "sources/catalog/source.yaml", "id: SRC-1\n")
	_, err := scan(root, func() {
		write(t, root, "sources/catalog/source.yaml", "id: SRC-1\nname: changed\n")
	})
	if err == nil || !strings.Contains(err.Error(), "changed during scan") {
		t.Fatalf("Scan accepted observed concurrent modification: %v", err)
	}
}

func TestScanIsDeterministicAndUsesSlashPaths(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	write(t, root, "skills/collection/example/SKILL.md", "# Example\n")
	first, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Scan(root)
	if err != nil {
		t.Fatal(err)
	}
	if first.CatalogSnapshot != second.CatalogSnapshot || first.ProjectionInputDigest != second.ProjectionInputDigest {
		t.Fatal("scanner output was not deterministic")
	}
	for _, item := range first.Files {
		if strings.Contains(item.Path, "\\") {
			t.Fatalf("non-normalized path: %s", item.Path)
		}
	}
}

func issueMessages(issues []Issue) string {
	messages := make([]string, 0, len(issues))
	for _, issue := range issues {
		messages = append(messages, issue.Path+": "+issue.Message)
	}
	return strings.Join(messages, "\n")
}

func write(t *testing.T, root, relative, contents string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}
