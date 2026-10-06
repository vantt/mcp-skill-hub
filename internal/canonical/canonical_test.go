package canonical

import (
	"fmt"
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
func TestValidateEnforcesSkillFrontmatterNameMatchesSkillID(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	write(t, root, "skills/software/rr/skill.meta.yaml", "schema_version: 1\nid: rr\nname: RR\nstatus: draft\ndescription: Reliability reviewer.\nrouting: {}\n")
	write(t, root, "skills/software/rr/SKILL.md", "---\nname: WRONG_NAME\n---\n# RR Skill\n")

	issues, err := Validate(root)
	if err != nil {
		t.Fatal(err)
	}
	joined := issueMessages(issues)
	if !strings.Contains(joined, `SKILL.md frontmatter name "WRONG_NAME" does not match skill ID "rr"`) {
		t.Fatalf("expected frontmatter mismatch issue, got:\n%s", joined)
	}

	// Fix frontmatter name
	write(t, root, "skills/software/rr/SKILL.md", "---\nname: rr\n---\n# RR Skill\n")
	issues, err = Validate(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("expected 0 issues after fixing frontmatter name, got:\n%s", issueMessages(issues))
	}
}

func TestValidateAcceptsCloneLikeWorkspaceWithAbsentEmptyDirectories(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	// Delete empty required directories to simulate fresh clone
	for _, dir := range workspace.RequiredDirectories() {
		_ = os.Remove(filepath.Join(root, filepath.FromSlash(dir)))
	}
	issues, err := Validate(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("expected 0 issues on clone-like workspace, got:\n%s", issueMessages(issues))
	}
}

func TestValidateCompanionResourcesOpaqueAndBounded(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	write(t, root, "skills/software/rr/skill.meta.yaml", "schema_version: 1\nid: rr\nname: RR\nstatus: draft\ndescription: Reliability reviewer.\nrouting: {}\n")
	write(t, root, "skills/software/rr/SKILL.md", "---\nname: rr\n---\n# RR Skill\n")

	// 1. Top-level LICENSE.txt
	write(t, root, "skills/software/rr/LICENSE.txt", "MIT License\n")
	// 2. Top-level forms.yaml (not a canonical entity, has arbitrary yaml)
	write(t, root, "skills/software/rr/forms.yaml", "form_title: Feedback Form\nfields:\n  - name: rating\n    type: number\n")
	// 3. Empty companion file
	write(t, root, "skills/software/rr/empty.txt", "")
	// 4. Binary asset
	binaryData := "\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15c4"
	write(t, root, "skills/software/rr/assets/logo.png", binaryData)
	// 5. Nested template
	write(t, root, "skills/software/rr/templates/nested/template.j2", "{% for item in items %}{{ item }}{% endfor %}")

	issues, err := Validate(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("expected all companion resources to pass validation, got:\n%s", issueMessages(issues))
	}
}

func TestValidateDetachedModeWithoutGit(t *testing.T) {
	root := filepath.Join(t.TempDir(), ".skillhub-validate-staged-test")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	// Remove .git
	_ = os.RemoveAll(filepath.Join(root, ".git"))

	unmerged := []string{"skills/software/conflict/SKILL.md"}
	issues, err := ValidateDetached(root, unmerged)
	if err != nil {
		t.Fatalf("ValidateDetached failed: %v", err)
	}
	joined := issueMessages(issues)
	if !strings.Contains(joined, "skills/software/conflict/SKILL.md: unresolved Git index conflict") {
		t.Fatalf("expected unmerged conflict issue in detached mode, got:\n%s", joined)
	}
	if strings.Contains(joined, "Workspace is not a Git repository") {
		t.Fatalf("detached mode must not report missing Git repository, got:\n%s", joined)
	}
}

func TestValidateStructuredOriginValidation(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := workspace.Apply(root); err != nil {
		t.Fatal(err)
	}
	write(t, root, "skills/software/rr/SKILL.md", "---\nname: rr\n---\n# RR Skill\n")

	// Valid local origin
	validLocalMeta := `schema_version: 1
id: rr
name: RR
status: draft
description: Reliability reviewer.
routing: {}
provenance:
  origin:
    kind: local
    name: rr
    path: skills/software/rr
    folder_digest: sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
    added_at: '2026-10-01T12:00:00Z'
`
	write(t, root, "skills/software/rr/skill.meta.yaml", validLocalMeta)
	issues, err := Validate(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("expected valid local origin to pass, got:\n%s", issueMessages(issues))
	}

	// Invalid: absolute path in local origin
	invalidAbsPathMeta := strings.Replace(validLocalMeta, "path: skills/software/rr", "path: /abs/path/rr", 1)
	write(t, root, "skills/software/rr/skill.meta.yaml", invalidAbsPathMeta)
	issues, err = Validate(root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(issueMessages(issues), "provenance.origin.path must be a relative, safe skill path") {
		t.Fatalf("expected rejection of absolute path in origin, got:\n%s", issueMessages(issues))
	}

	// Invalid: repository in local origin
	invalidRepoMeta := strings.Replace(validLocalMeta, "kind: local", "kind: local\n    repository: https://github.com/foo/bar", 1)
	write(t, root, "skills/software/rr/skill.meta.yaml", invalidRepoMeta)
	issues, err = Validate(root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(issueMessages(issues), "provenance.origin.repository is not allowed for local origin") {
		t.Fatalf("expected rejection of repository in local origin, got:\n%s", issueMessages(issues))
	}

	// Valid: with files_digest
	validFilesDigestMeta := strings.Replace(validLocalMeta, "folder_digest:", "files_digest: sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef\n    folder_digest:", 1)
	write(t, root, "skills/software/rr/skill.meta.yaml", validFilesDigestMeta)
	issues, err = Validate(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(issues) != 0 {
		t.Fatalf("expected valid files_digest to pass, got:\n%s", issueMessages(issues))
	}

	// Invalid: files_digest format
	invalidFilesDigestMeta := strings.Replace(validLocalMeta, "folder_digest:", "files_digest: abc\n    folder_digest:", 1)
	write(t, root, "skills/software/rr/skill.meta.yaml", invalidFilesDigestMeta)
	issues, err = Validate(root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(issueMessages(issues), "provenance.origin.files_digest must be a lowercase SHA-256 digest") {
		t.Fatalf("expected rejection of invalid files_digest, got:\n%s", issueMessages(issues))
	}
}

const runtimeAndExamplesMetadata = `schema_version: 1
id: owner
name: Owner
status: active
description: Route work.
runtime:
  requires:
    bins:
      - python3
      - {name: node, version: ">=18"}
    env: [OPENAI_API_KEY]
    platforms: [linux, darwin]
  setup:
    command: "pip install -r requirements.txt"
    check: "python3 scripts/check_env.py"
routing:
  triggers: [route work]
  not_for: [write prose]
  min_scope: multi_step
  examples: [route this request to the owner skill, pick an owner for the task]
  counter_examples: [write a poem]
quality:
  content_reviewed_digest: sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
`

func TestValidateAcceptsRuntimeExamplesAndContentReviewedDigest(t *testing.T) {
	if _, issues := validateSkillMetadata("skills/core/owner/skill.meta.yaml", []byte(runtimeAndExamplesMetadata)); len(issues) != 0 {
		t.Fatalf("valid runtime and example metadata rejected: %s", issueMessages(issues))
	}
}

func TestValidateRejectsInvalidRuntimeExamplesAndContentReviewedDigest(t *testing.T) {
	tooMany := make([]string, 11)
	for index := range tooMany {
		tooMany[index] = fmt.Sprintf("example request %d", index)
	}
	cases := []struct {
		name, old, replacement, want string
	}{
		{"bad bin name", "      - python3\n", "      - python 3\n", "runtime requires bins entries must match"},
		{"bad bin name in mapping", "{name: node,", "{name: \"no/de\",", "bins entry name must match"},
		{"bad version constraint", `version: ">=18"`, `version: "~18"`, "bins entry version"},
		{"unquoted version", `version: ">=18"`, `version: 18`, "bins entry version"},
		{"unknown bin field", `version: ">=18"}`, `version: ">=18", path: /usr/bin/node}`, `unknown field "path"`},
		{"duplicate bin", "      - python3\n", "      - node\n", "duplicate names"},
		{"env with value", "env: [OPENAI_API_KEY]", "env: [OPENAI_API_KEY=secret]", "env entries must be variable names"},
		{"env with leading digit", "env: [OPENAI_API_KEY]", "env: [1KEY]", "env entries must be variable names"},
		{"unknown platform", "platforms: [linux, darwin]", "platforms: [linux, plan9]", "platforms entries must be"},
		{"multi-line command", `command: "pip install -r requirements.txt"`, `command: "pip install\nrm -rf /"`, "setup.command must be a non-empty single-line string"},
		{"empty check", `check: "python3 scripts/check_env.py"`, `check: "  "`, "setup.check must be a non-empty single-line string"},
		{"oversized command", `command: "pip install -r requirements.txt"`, `command: "` + strings.Repeat("a", 1025) + `"`, "at most 1024 bytes"},
		{"unknown runtime key", "  setup:\n", "  network: true\n  setup:\n", `unknown field "network"`},
		{"unknown requires key", "    platforms: [linux, darwin]\n", "    platforms: [linux, darwin]\n    memory: 4G\n", `unknown field "memory"`},
		{"unknown setup key", `    check: "python3 scripts/check_env.py"`, "    check: \"python3 scripts/check_env.py\"\n    cleanup: \"rm -rf .venv\"", `unknown field "cleanup"`},
		{"too many examples", "examples: [route this request to the owner skill, pick an owner for the task]", "examples: [" + strings.Join(tooMany, ", ") + "]", "routing.examples must contain at most 10 entries"},
		{"duplicate examples", "examples: [route this request to the owner skill, pick an owner for the task]", "examples: [same request, same request]", "routing.examples must not contain duplicate values"},
		{"empty counter example", "counter_examples: [write a poem]", `counter_examples: [""]`, "routing.counter_examples must be a sequence of non-empty strings"},
		{"long counter example", "counter_examples: [write a poem]", "counter_examples: [" + strings.Repeat("x", 301) + "]", "routing.counter_examples entries must be at most 300 characters"},
		{"malformed reviewed digest", "content_reviewed_digest: sha256:0123", "content_reviewed_digest: sha256:XYZ", "content_reviewed_digest must be a lowercase SHA-256 digest"},
		{"reviewed digest without prefix", "content_reviewed_digest: sha256:", "content_reviewed_digest: ", "content_reviewed_digest must be a lowercase SHA-256 digest"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(runtimeAndExamplesMetadata, tc.old) {
				t.Fatalf("fixture does not contain %q", tc.old)
			}
			metadata := strings.Replace(runtimeAndExamplesMetadata, tc.old, tc.replacement, 1)
			_, issues := validateSkillMetadata("skills/core/owner/skill.meta.yaml", []byte(metadata))
			if joined := issueMessages(issues); !strings.Contains(joined, tc.want) {
				t.Fatalf("want issue containing %q, got: %s", tc.want, joined)
			}
		})
	}
}

func TestValidateAcceptsExactlyTenRoutingExamplesOfMaximumLength(t *testing.T) {
	examples := make([]string, 10)
	for index := range examples {
		examples[index] = fmt.Sprintf("%03d", index) + strings.Repeat("é", 297)
	}
	metadata := strings.Replace(runtimeAndExamplesMetadata, "examples: [route this request to the owner skill, pick an owner for the task]", "examples: ["+strings.Join(examples, ", ")+"]", 1)
	if _, issues := validateSkillMetadata("skills/core/owner/skill.meta.yaml", []byte(metadata)); len(issues) != 0 {
		t.Fatalf("boundary examples rejected: %s", issueMessages(issues))
	}
}
