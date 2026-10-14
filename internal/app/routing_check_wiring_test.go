package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/catalog"
	"github.com/vantt/skillhub-mcp/internal/skill"
)

const (
	stolenPhrase = "tidy up the changelog entries for the version"
	ownExample   = "write the release notes for the new version"
)

// newCommittedLintHub builds a hub with two active skills, a release-notes
// skill X and a changelog skill Y, publishes the catalog and commits the tree.
func newCommittedLintHub(t *testing.T) string {
	t.Helper()
	root := newSkillWorkspace(t)
	service := SkillService{}
	createExampleSkill(t, service, root, "release-notes", "draft release notes", ownExample, true)
	createExampleSkill(t, service, root, "changelog-curation", "curate the changelog", stolenPhrase, true)
	if _, err := os.Stat(filepath.Join(root, ".git")); err != nil {
		runGit(t, root, "init", "-q")
	}
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test User")
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-q", "-m", "hub")
	return root
}

func createExampleSkill(t *testing.T, service SkillService, root, id, trigger, example string, activate bool) {
	t.Helper()
	ctx := context.Background()
	preview, err := service.PreviewCreate(ctx, root, skill.CreateInput{
		ID: id, Collection: "software", Name: id, Description: "Handle " + trigger + ".",
		Content: []byte("# " + id + "\n\nReal instructions.\n"),
		Routing: skill.RoutingInput{Triggers: []string{trigger}, NotFor: []string{"design brokers"}, MinScope: "multi_step", Examples: []string{example}},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmSkillMutation(ctx, root, preview, preview.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}
	if !activate {
		return
	}
	preview, err = service.PreviewActivate(ctx, root, id, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.ConfirmSkillMutation(ctx, root, preview, preview.Confirmation.Confirmation.Pins); err != nil {
		t.Fatal(err)
	}
}

// addTrigger appends a trigger to a skill's canonical meta without rebuilding.
func addTrigger(t *testing.T, root, id, trigger string) {
	t.Helper()
	path := filepath.Join(root, "skills", "software", id, "skill.meta.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		path = filepath.Join(root, "skills", "software", id, ".meta", "skill.yaml")
		if data, err = os.ReadFile(path); err != nil {
			t.Fatal(err)
		}
	}
	text := string(data)
	if !strings.Contains(text, "  triggers:\n") {
		t.Fatalf("meta has no triggers block:\n%s", text)
	}
	text = strings.Replace(text, "  triggers:\n", "  triggers:\n    - "+trigger+"\n", 1)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

// rebuildHub publishes the working tree, as the hub does after every change.
func rebuildHub(t *testing.T, root string) {
	t.Helper()
	if _, err := catalog.BuildCatalogGeneration(context.Background(), root, catalog.BuildOptions{}); err != nil {
		t.Fatal(err)
	}
}

func warningsWithCode(warnings []Warning, code string) []Warning {
	var found []Warning
	for _, w := range warnings {
		if w.Code == code {
			found = append(found, w)
		}
	}
	return found
}

func TestReviewReportsExampleTakenByNewTrigger(t *testing.T) {
	t.Parallel()
	root := newCommittedLintHub(t)
	service := SkillService{}
	before := runGit(t, root, "status", "--porcelain")
	pointer, err := os.ReadFile(filepath.Join(root, "runtime", "catalog", "CURRENT"))
	if err != nil {
		pointer = nil
	}

	addTrigger(t, root, "release-notes", stolenPhrase)
	review, err := service.ReviewSkill(context.Background(), root, "release-notes")
	if err != nil {
		t.Fatal(err)
	}
	var taken string
	for _, w := range review.ActivationReadiness.Warnings {
		if strings.HasPrefix(w, "example_taken_by:") {
			taken = w
		}
	}
	if taken == "" {
		t.Fatalf("review of release-notes did not report the stolen example: %v", review.ActivationReadiness.Warnings)
	}
	for _, want := range []string{`"changelog-curation"`, `now resolves to "release-notes"`, stolenPhrase} {
		if !strings.Contains(taken, want) {
			t.Fatalf("warning %q lacks %q", taken, want)
		}
	}

	after := runGit(t, root, "status", "--porcelain")
	if !strings.Contains(after, "release-notes/") || strings.Count(after, "\n") != strings.Count(before, "\n")+1 {
		t.Fatalf("review must change nothing but the edit itself:\nbefore %q\nafter %q", before, after)
	}
	if pointer != nil {
		if now, _ := os.ReadFile(filepath.Join(root, "runtime", "catalog", "CURRENT")); string(now) != string(pointer) {
			t.Fatal("review moved the catalog pointer")
		}
	}
}

func TestReviewDraftReportsItsOwnMissAndWhatItTakes(t *testing.T) {
	t.Parallel()
	root := newCommittedLintHub(t)
	service := SkillService{}

	// A draft whose example is another skill's request misses and names the winner.
	createExampleSkill(t, service, root, "notes-draft", "collect notes", stolenPhrase, false)
	review, err := service.ReviewSkill(context.Background(), root, "notes-draft")
	if err != nil {
		t.Fatal(err)
	}
	miss := warningsFromReview(review, "example_resolves_elsewhere:")
	if len(miss) != 1 || !strings.Contains(miss[0], "changelog-curation") {
		t.Fatalf("draft miss = %v (all: %v)", miss, review.ActivationReadiness.Warnings)
	}

	// A draft that takes another skill's example reports what it would take.
	createExampleSkill(t, service, root, "changelog-draft", stolenPhrase, "list the entries of the changelog", false)
	review, err = service.ReviewSkill(context.Background(), root, "changelog-draft")
	if err != nil {
		t.Fatal(err)
	}
	if taken := warningsFromReview(review, "example_taken_by:"); len(taken) == 0 || !strings.Contains(taken[0], "changelog-curation") {
		t.Fatalf("draft takes nothing: %v", review.ActivationReadiness.Warnings)
	}
}

func warningsFromReview(review SkillReviewResult, prefix string) []string {
	var found []string
	for _, w := range review.ActivationReadiness.Warnings {
		if strings.HasPrefix(w, prefix) {
			found = append(found, w)
		}
	}
	return found
}

func TestValidateChangedModeChecksOnlyChangedSkills(t *testing.T) {
	t.Parallel()
	root := newCommittedLintHub(t)
	service := WorkspaceService{}
	validate := func() Result {
		result, err := service.ValidateWorkspace(context.Background(), root)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	clean := validate()
	for _, code := range []string{"example_resolves_elsewhere", "example_taken_by", "self_resolution_skipped"} {
		if found := warningsWithCode(clean.Warnings, code); len(found) != 0 {
			t.Fatalf("a clean tree reported %s: %v", code, found)
		}
	}

	addTrigger(t, root, "release-notes", stolenPhrase)
	rebuildHub(t, root)
	first := validate()
	taken := warningsWithCode(first.Warnings, "example_taken_by")
	if len(taken) != 1 || !strings.Contains(taken[0].Summary, "release-notes") || !strings.Contains(taken[0].Summary, "changelog-curation") {
		t.Fatalf("edited skill must report the stolen example: %v", first.Warnings)
	}
	if second := validate(); !slices.Equal(first.Warnings, second.Warnings) {
		t.Fatalf("validate output changed between runs:\n%v\n%v", first.Warnings, second.Warnings)
	}
}

func TestValidateChangedModeReportsPolicyChangeOnce(t *testing.T) {
	t.Parallel()
	root := newCommittedLintHub(t)
	addTrigger(t, root, "release-notes", stolenPhrase)
	policy := filepath.Join(root, "config", "recommendation.yaml")
	if err := os.MkdirAll(filepath.Dir(policy), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policy, []byte("# edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rebuildHub(t, root)
	result, err := (WorkspaceService{}).ValidateWorkspace(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	skipped := warningsWithCode(result.Warnings, "self_resolution_skipped")
	if len(skipped) != 1 || !strings.Contains(skipped[0].Summary, "skillhub maintain validate --full") {
		t.Fatalf("policy change must give one skip: %v", result.Warnings)
	}
	if found := warningsWithCode(result.Warnings, "example_taken_by"); len(found) != 0 {
		t.Fatalf("a stale policy must not produce findings: %v", found)
	}
}

func TestValidateFullModeChecksEveryActiveSkill(t *testing.T) {
	t.Parallel()
	root := newCommittedLintHub(t)
	// A skill whose own example another skill wins, committed with the tree.
	service := SkillService{}
	createExampleSkill(t, service, root, "release-draft", "assemble the notes", stolenPhrase, true)
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-q", "-m", "more")

	changed, err := (WorkspaceService{}).ValidateWorkspace(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if found := warningsWithCode(changed.Warnings, "example_resolves_elsewhere"); len(found) != 0 {
		t.Fatalf("a clean tree reports nothing in changed mode: %v", found)
	}
	full, err := (WorkspaceService{}).ValidateWorkspaceWith(context.Background(), root, ValidateOptions{Full: true})
	if err != nil {
		t.Fatal(err)
	}
	found := warningsWithCode(full.Warnings, "example_resolves_elsewhere")
	if len(found) == 0 {
		t.Fatalf("full mode must report the own miss: %v", full.Warnings)
	}
	again, _ := (WorkspaceService{}).ValidateWorkspaceWith(context.Background(), root, ValidateOptions{Full: true})
	if !slices.Equal(full.Warnings, again.Warnings) {
		t.Fatal("full mode output changed between runs")
	}
}

func TestValidateWithoutHeadChecksEverySkill(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	service := SkillService{}
	createExampleSkill(t, service, root, "changelog-curation", "curate the changelog", stolenPhrase, true)
	createExampleSkill(t, service, root, "release-draft", "assemble the notes", stolenPhrase, true)
	result, err := (WorkspaceService{}).ValidateWorkspace(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(warningsWithCode(result.Warnings, "example_resolves_elsewhere")) == 0 {
		t.Fatalf("a hub without commits checks every active skill: %v", result.Warnings)
	}
}

func TestValidateStagedChecksStagedSkillsAndStaysWarningsOnly(t *testing.T) {
	t.Parallel()
	root := newCommittedLintHub(t)
	addTrigger(t, root, "release-notes", stolenPhrase)
	rebuildHub(t, root)
	svc := WorkspaceService{}

	unstaged, err := svc.ValidateStaged(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if found := warningsWithCode(unstaged.Warnings, "example_taken_by"); len(found) != 0 {
		t.Fatalf("an unstaged edit is not checked by the staged mode: %v", found)
	}

	runGit(t, root, "add", "skills/software/release-notes")
	staged, err := svc.ValidateStaged(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if staged.Status != StatusOK || len(staged.Items) != 0 {
		t.Fatalf("lint findings must stay warnings: %#v", staged)
	}
	if taken := warningsWithCode(staged.Warnings, "example_taken_by"); len(taken) != 1 {
		t.Fatalf("staged edit must report the stolen example: %v", staged.Warnings)
	}
	again, _ := svc.ValidateStaged(context.Background(), root)
	if !slices.Equal(staged.Warnings, again.Warnings) {
		t.Fatal("staged output changed between runs")
	}
}

func TestValidateStagedSkipsQuietlyWithoutACatalog(t *testing.T) {
	t.Parallel()
	root := setupTestGitRepo(t)
	writeSkill(t, root, "fresh", "fresh", "Body")
	runGit(t, root, "add", "-A")
	result, err := (WorkspaceService{}).ValidateStaged(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("a hub without a catalog must not warn in the hook: %v", result.Warnings)
	}
}

func TestChangedSkillsParsePaths(t *testing.T) {
	t.Parallel()
	set := parseChangedPaths([]string{
		"skills/software/a/SKILL.md", "skills/software/a/skill.meta.yaml", "skills/core/b/.meta/skill.yaml",
		"skills/software/c", "docs/readme.md", "config/recommendation.yaml",
	})
	if !set.policyChanged || len(set.collections) != 2 || len(set.collections["a"]) != 1 || set.collections["b"][0] != "core" {
		t.Fatalf("parsed set = %+v", set)
	}
}

func TestPreviewLintReportsAnExampleTheProposedSkillTakes(t *testing.T) {
	t.Parallel()
	root := newCommittedLintHub(t)
	addTrigger(t, root, "release-notes", stolenPhrase)
	meta, err := os.ReadFile(filepath.Join(root, "skills", "software", "release-notes", ".meta", "skill.yaml"))
	if err != nil {
		meta, err = os.ReadFile(filepath.Join(root, "skills", "software", "release-notes", "skill.meta.yaml"))
		if err != nil {
			t.Fatal(err)
		}
	}
	final := map[string]treeFile{"SKILL.md": {Data: []byte("# release-notes\n\nReal instructions.\n")}}
	findings := changesLint(context.Background(), root, "release-notes", meta, final)
	if !slices.ContainsFunc(findings, func(f SkillCommitLint) bool {
		return f.Code == "example_taken_by" && strings.Contains(f.Message, "changelog-curation")
	}) {
		t.Fatalf("changes preview lint = %+v", findings)
	}
	plan := &commitPlan{root: root, id: "release-notes", metaAfter: meta, final: final}
	if findings := commitLint(context.Background(), plan); !slices.ContainsFunc(findings, func(f SkillCommitLint) bool { return f.Code == "example_taken_by" }) {
		t.Fatalf("commit preview lint = %+v", findings)
	}
}
