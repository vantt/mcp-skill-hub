package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/vantt/skillhub-mcp/internal/mutation"
	"github.com/vantt/skillhub-mcp/internal/workspace"
)

// commitCtx asks for a Git commit the way the command line does.
func commitCtx() context.Context {
	return workspace.WithCommitPolicy(context.Background(), workspace.CommitPolicy{})
}

func writeProjectFile(t *testing.T, folder, rel, body string, mode os.FileMode) {
	t.Helper()
	full := filepath.Join(folder, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(full, mode); err != nil {
		t.Fatal(err)
	}
}

// checkedOut seeds a hub skill, checks it out into a fresh project and returns
// the hub, the project and the project folder.
func checkedOut(t *testing.T, skill fixtureSkill) (hub, project, folder string) {
	t.Helper()
	hub = newGitHub(t)
	seedHubSkill(t, hub, skill)
	commitHubAll(t, hub, "add "+skill.id)
	project = newProjectDir(t)
	result, err := (SkillCheckoutService{}).Checkout(context.Background(), hub, SkillCheckoutInput{SkillID: skill.id, ProjectDir: project})
	if err != nil || result.Error != nil {
		t.Fatalf("checkout: %v, %#v", err, result.Error)
	}
	return hub, project, filepath.Join(project, ".claude", "skills", skill.id)
}

func previewCommit(t *testing.T, hub, project, target string, input SkillCommitInput) SkillCommitPreview {
	t.Helper()
	input.Target, input.ProjectDir = target, project
	preview, err := (SkillCommitService{}).Preview(context.Background(), hub, input)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Error != nil {
		t.Fatalf("preview refused: %+v", preview.Error.Render)
	}
	return preview
}

func TestSkillCommitUpdateWhenTheHubDidNotChange(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	t.Parallel()
	hub, project, folder := checkedOut(t, fixtureSkill{id: "review-checklist", files: map[string]string{"references/old.md": "old\n", "notes.md": "keep\n"}})
	writeProjectFile(t, folder, "SKILL.md", "---\nname: review-checklist\ndescription: Does review-checklist things. Use when you need review-checklist.\n---\n\n# review-checklist\n\nline one\nline TWO\nline three\nline four\n", 0o644)
	writeProjectFile(t, folder, "scripts/new.sh", "#!/bin/sh\necho hi\n", 0o755)
	if err := os.Remove(filepath.Join(folder, "references", "old.md")); err != nil {
		t.Fatal(err)
	}
	writeProjectFile(t, folder, ".env", "TOKEN=1\n", 0o644)

	preview := previewCommit(t, hub, project, "review-checklist", SkillCommitInput{})
	if preview.Case != commitCaseUpdate || preview.NothingToCommit {
		t.Fatalf("case = %s nothing=%v", preview.Case, preview.NothingToCommit)
	}
	got := map[string]SkillCommitFile{}
	for _, file := range preview.Files {
		got[file.Path] = file
	}
	if got["SKILL.md"].Change != "M" || got["SKILL.md"].Added != 2 || got["SKILL.md"].Removed != 1 ||
		got["scripts/new.sh"].Change != "A" || got["references/old.md"].Change != "D" || len(got) != 3 {
		t.Fatalf("files = %+v", preview.Files)
	}
	if len(preview.Excluded) != 1 || preview.Excluded[0].Path != ".env" {
		t.Fatalf("excluded = %+v", preview.Excluded)
	}
	headBefore := hubGit(t, hub, "rev-parse", "HEAD")
	result, err := (SkillCommitService{}).Apply(commitCtx(), hub, preview, SkillCommitApplyOptions{})
	if err != nil || result.Error != nil {
		t.Fatalf("apply: %v, %#v", err, result.Error)
	}
	if result.Commit == nil || result.Commit.Status != workspace.CommitCommitted {
		t.Fatalf("commit = %+v", result.Commit)
	}
	if hubGit(t, hub, "rev-list", "--count", headBefore+"..HEAD") != "1" {
		t.Fatalf("expected exactly one new hub commit")
	}
	changed := strings.Fields(hubGit(t, hub, "show", "--name-only", "--format=", "HEAD"))
	for _, path := range changed {
		if !strings.HasPrefix(path, "skills/default/review-checklist/") && !strings.HasPrefix(path, "history/operations/") {
			t.Errorf("commit touches %s outside the skill and its receipt", path)
		}
	}
	info, err := os.Stat(filepath.Join(hub, "skills", "default", "review-checklist", "scripts", "new.sh"))
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("new.sh: %v mode=%v", err, info)
	}
	if _, err := os.Stat(filepath.Join(hub, "skills", "default", "review-checklist", ".env")); !os.IsNotExist(err) {
		t.Fatalf(".env reached the hub")
	}
	marker, found, _ := ReadCheckoutMarker(folder)
	if !found || marker.BaseCommit != result.Commit.Hash {
		t.Fatalf("marker after commit = %+v", marker)
	}
	again := previewCommit(t, hub, project, "review-checklist", SkillCommitInput{})
	if !again.NothingToCommit || again.Case != commitCaseUpdate {
		t.Fatalf("re-commit = %+v", again)
	}
}

func replaceInHub(t *testing.T, hub, id, from, to string) {
	t.Helper()
	path := filepath.Join(hub, "skills", "default", id, "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), from) {
		t.Fatalf("hub SKILL.md has no %q", from)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(data), from, to, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	commitHubAll(t, hub, "edit "+id)
}

func replaceInProject(t *testing.T, folder, from, to string) {
	t.Helper()
	path := filepath.Join(folder, "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), from) {
		t.Fatalf("project SKILL.md has no %q", from)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(data), from, to, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hubSkillMD(t *testing.T, hub, id string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(hub, "skills", "default", id, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSkillCommitMergesWhatTheHubChangedMeanwhile(t *testing.T) {
	t.Parallel()
	hub, project, folder := checkedOut(t, fixtureSkill{id: "pdf", files: map[string]string{"scripts/fill.py": "print('v1')\n"}})
	replaceInHub(t, hub, "pdf", "line three", "line 3 from the hub")
	if err := os.WriteFile(filepath.Join(hub, "skills", "default", "pdf", "scripts", "fill.py"), []byte("print('v2')\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitHubAll(t, hub, "hub edits fill.py")
	replaceInProject(t, folder, "line one", "line 1 from the project")

	preview := previewCommit(t, hub, project, "pdf", SkillCommitInput{})
	if preview.Case != commitCaseMerge || len(preview.Conflicts) != 0 || preview.ProjectChanged != 1 || preview.HubChanged != 2 {
		t.Fatalf("case=%s conflicts=%v project=%d hub=%d", preview.Case, preview.Conflicts, preview.ProjectChanged, preview.HubChanged)
	}
	words := map[string]string{}
	for _, entry := range preview.Merge {
		words[entry.Path] = entry.Status
	}
	if words["SKILL.md"] != "changed in both, merged" || words["scripts/fill.py"] != "changed on the hub, kept" {
		t.Fatalf("merge words = %+v", words)
	}
	result, err := (SkillCommitService{}).Apply(commitCtx(), hub, preview, SkillCommitApplyOptions{})
	if err != nil || result.Error != nil {
		t.Fatalf("apply: %v, %#v", err, result.Error)
	}
	merged := hubSkillMD(t, hub, "pdf")
	if !strings.Contains(merged, "line 1 from the project") || !strings.Contains(merged, "line 3 from the hub") {
		t.Fatalf("merged hub SKILL.md lost an edit:\n%s", merged)
	}
	if got, _ := os.ReadFile(filepath.Join(folder, "SKILL.md")); string(got) != merged {
		t.Fatalf("project folder was not brought to the merged version:\n%s", got)
	}
	if got, _ := os.ReadFile(filepath.Join(folder, "scripts", "fill.py")); string(got) != "print('v2')\n" {
		t.Fatalf("hub change to fill.py did not reach the project: %q", got)
	}
	if again := previewCommit(t, hub, project, "pdf", SkillCommitInput{}); !again.NothingToCommit {
		t.Fatalf("after a merge the folder must match the hub, got %+v", again.Files)
	}
}

func TestSkillCommitSameLineNeedsADecision(t *testing.T) {
	t.Parallel()
	hub, project, folder := checkedOut(t, fixtureSkill{id: "pdf"})
	replaceInHub(t, hub, "pdf", "line two", "hub two")
	replaceInProject(t, folder, "line two", "project two")

	preview := previewCommit(t, hub, project, "pdf", SkillCommitInput{})
	if len(preview.Conflicts) != 1 || preview.Conflicts[0] != "SKILL.md" || preview.Merge[0].Status != "needs a decision" {
		t.Fatalf("conflicts = %v merge = %+v", preview.Conflicts, preview.Merge)
	}
	headBefore := hubGit(t, hub, "rev-parse", "HEAD")
	refused, err := (SkillCommitService{}).Apply(commitCtx(), hub, preview, SkillCommitApplyOptions{})
	if err != nil || refused.Error == nil || !strings.Contains(refused.Error.Render.Fix, "--accept") {
		t.Fatalf("apply with a conflict = %+v, %v", refused.Error, err)
	}
	if hubGit(t, hub, "rev-parse", "HEAD") != headBefore || !strings.Contains(hubSkillMD(t, hub, "pdf"), "hub two") {
		t.Fatalf("a refused commit changed the hub")
	}
	resolved := previewCommit(t, hub, project, "pdf", SkillCommitInput{Resolutions: []SkillCommitResolution{{Path: "SKILL.md", Choice: "project"}}})
	if len(resolved.Conflicts) != 0 || resolved.Merge[0].Action != "project" {
		t.Fatalf("resolved = %+v", resolved.Merge)
	}
	if result, err := (SkillCommitService{}).Apply(commitCtx(), hub, resolved, SkillCommitApplyOptions{}); err != nil || result.Error != nil {
		t.Fatalf("apply: %v, %#v", err, result.Error)
	}
	if got := hubSkillMD(t, hub, "pdf"); !strings.Contains(got, "project two") || strings.Contains(got, "hub two") {
		t.Fatalf("project choice not applied:\n%s", got)
	}
	stray := previewCommit(t, hub, project, "pdf", SkillCommitInput{})
	if !stray.NothingToCommit {
		t.Fatalf("expected nothing to commit, got %+v", stray.Files)
	}
	bad, err := (SkillCommitService{}).Preview(context.Background(), hub, SkillCommitInput{Target: "pdf", ProjectDir: project, Resolutions: []SkillCommitResolution{{Path: "nope.md", Choice: "hub"}}})
	if err != nil || bad.Error == nil {
		t.Fatalf("a choice for a file with no difference must be refused, got %+v %v", bad.Error, err)
	}
}

func TestSkillCommitWithoutABaseReportsEveryDifferingFileAndWritesNothing(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	dir := seedHubSkill(t, hub, fixtureSkill{id: "pdf"})
	commitHubAll(t, hub, "add pdf")
	// An uncommitted edit at checkout time leaves no commit to rebuild the base from.
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(strings.Replace(hubSkillMD(t, hub, "pdf"), "line one", "dirty one", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	project := newProjectDir(t)
	if result, err := (SkillCheckoutService{}).Checkout(context.Background(), hub, SkillCheckoutInput{SkillID: "pdf", ProjectDir: project}); err != nil || result.Error != nil || result.BaseCommit != "" {
		t.Fatalf("checkout = %+v %v", result, err)
	}
	folder := filepath.Join(project, ".claude", "skills", "pdf")
	replaceInHub(t, hub, "pdf", "line three", "hub three")
	replaceInProject(t, folder, "line two", "project two")

	preview := previewCommit(t, hub, project, "pdf", SkillCommitInput{})
	if len(preview.Conflicts) != 1 || preview.Conflicts[0] != "SKILL.md" {
		t.Fatalf("conflicts = %v, want every differing file", preview.Conflicts)
	}
	headBefore := hubGit(t, hub, "rev-parse", "HEAD")
	if result, err := (SkillCommitService{}).Apply(commitCtx(), hub, preview, SkillCommitApplyOptions{}); err != nil || result.Error == nil {
		t.Fatalf("apply = %+v %v", result.Error, err)
	}
	if hubGit(t, hub, "rev-parse", "HEAD") != headBefore || strings.Contains(hubSkillMD(t, hub, "pdf"), "project two") {
		t.Fatalf("nothing may be written without a base")
	}
}

const teamReleaseSkill = "---\nname: team-release\ndescription: Cut a release for the team. Use when you cut a release or bump the version. Do not use for publishing a package to npm.\n---\n\n# Team release\n\n## When to use\n\n- cut a release\n\n## Examples\n\n- cut release 1.4 for the api\n- bump the version and tag it\n- prepare the release notes\n\n## Steps\n\n1. Bump the version.\n2. Tag it.\n"

// vendoredDir is a folder the import filter always leaves out.
const vendoredDir = "node_" + "modules"

func newProjectSkill(t *testing.T, project string) string {
	t.Helper()
	folder := filepath.Join(project, ".claude", "skills", "team-release")
	writeProjectFile(t, folder, "SKILL.md", teamReleaseSkill, 0o644)
	writeProjectFile(t, folder, "scripts/bump.sh", "#!/bin/sh\necho bump\n", 0o755)
	writeProjectFile(t, folder, ".env", "TOKEN=abc\n", 0o644)
	writeProjectFile(t, folder, vendoredDir+"/x/index.js", "x\n", 0o644)
	writeProjectFile(t, folder, ".meta/skill.yaml", "schema_version: 1\nid: team-release\nstatus: active\nquality:\n  reviewed: true\n  content_reviewed_digest: sha256:"+strings.Repeat("a", 64)+"\n", 0o644)
	return folder
}

func TestSkillCommitBareIDReadsTheProjectSkillEvenWithAFolderOfThatNameHere(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	project := newProjectDir(t)
	folder := newProjectSkill(t, project)
	decoy := filepath.Join(project, "team-release")
	writeProjectFile(t, decoy, "SKILL.md", "---\nname: team-release\ndescription: An older copy holding only its SKILL.md.\n---\n# Old\n", 0o644)

	if preview := previewCommit(t, hub, project, "team-release", SkillCommitInput{}); preview.Folder != folder {
		t.Fatalf("a bare id read %s, want %s", preview.Folder, folder)
	}
	if preview := previewCommit(t, hub, project, "./team-release", SkillCommitInput{}); preview.Folder != decoy {
		t.Fatalf("a path read %s, want %s", preview.Folder, decoy)
	}
}

func TestSkillCommitNewSkillIsAUnapprovedDraftAndLeavesOutManagedFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes are not meaningful on Windows")
	}
	t.Parallel()
	hub := newGitHub(t)
	project := newProjectDir(t)
	folder := newProjectSkill(t, project)

	preview := previewCommit(t, hub, project, ".claude/skills/team-release", SkillCommitInput{})
	if preview.Case != commitCaseNew || preview.Collection != "default" {
		t.Fatalf("case=%s collection=%s", preview.Case, preview.Collection)
	}
	left := map[string]string{}
	for _, file := range preview.Excluded {
		left[file.Path] = file.Reason
	}
	if left[".env"] == "" || left[vendoredDir] == "" || left[".meta"] == "" || len(left) != 3 {
		t.Fatalf("excluded = %+v", preview.Excluded)
	}
	if preview.RoutingSuggestions == nil || len(preview.RoutingSuggestions.Triggers) == 0 || len(preview.RoutingSuggestions.NotFor) == 0 || preview.RoutingSuggestions.MinScope != "single_step" {
		t.Fatalf("routing suggestions = %+v", preview.RoutingSuggestions)
	}
	if !preview.ActivationReady {
		t.Fatalf("activation should be ready with the suggested routing: %v", preview.ActivationMissing)
	}
	if result, err := (SkillCommitService{}).Apply(commitCtx(), hub, preview, SkillCommitApplyOptions{}); err != nil || result.Error != nil || result.Activated {
		t.Fatalf("apply = %+v %v", result.Error, err)
	}
	dir := filepath.Join(hub, "skills", "default", "team-release")
	meta, err := os.ReadFile(filepath.Join(dir, ".meta", "skill.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"status: active", "content_reviewed_digest", "reviewed: true"} {
		if strings.Contains(string(meta), forbidden) {
			t.Fatalf("hub meta took %q from the folder:\n%s", forbidden, meta)
		}
	}
	if !strings.Contains(string(meta), "status: draft") || !strings.Contains(string(meta), "cut a release") {
		t.Fatalf("hub meta = %s", meta)
	}
	for _, rel := range []string{".env", vendoredDir, "scripts/bump.sh"} {
		_, statErr := os.Stat(filepath.Join(dir, filepath.FromSlash(rel)))
		if (rel == "scripts/bump.sh") != (statErr == nil) {
			t.Errorf("%s present=%v", rel, statErr == nil)
		}
	}
	if info, _ := os.Stat(filepath.Join(dir, "scripts", "bump.sh")); info == nil || info.Mode().Perm() != 0o755 {
		t.Errorf("bump.sh lost its executable bit")
	}
	if _, found, _ := ReadCheckoutMarker(folder); !found {
		t.Fatalf("a committed folder must carry a marker")
	}
	if data, _ := os.ReadFile(filepath.Join(project, ".gitignore")); !strings.Contains(string(data), ".skillhub-checkout.json") {
		t.Fatalf(".gitignore = %q", data)
	}
	if again := previewCommit(t, hub, project, "team-release", SkillCommitInput{}); !again.NothingToCommit || again.Case != commitCaseUpdate {
		t.Fatalf("re-commit must be an update with nothing to commit, got %+v", again)
	}
	writeProjectFile(t, folder, "references/more.md", "more\n", 0o644)
	if second := previewCommit(t, hub, project, ".claude/skills/team-release", SkillCommitInput{}); second.Case != commitCaseUpdate || len(second.Files) != 1 || second.Files[0].Change != "A" {
		t.Fatalf("committing the same folder again is an update with a diff, got %+v", second)
	}
}

func TestSkillCommitNewSkillChainsAddAndActivateIntoOneCommit(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	project := newProjectDir(t)
	newProjectSkill(t, project)
	preview := previewCommit(t, hub, project, ".claude/skills/team-release", SkillCommitInput{})
	headBefore := hubGit(t, hub, "rev-parse", "HEAD")
	result, err := (SkillCommitService{}).Apply(commitCtx(), hub, preview, SkillCommitApplyOptions{Activate: true})
	if err != nil || result.Error != nil || !result.Activated {
		t.Fatalf("apply = %+v %v activated=%v", result.Error, err, result.Activated)
	}
	if hubGit(t, hub, "rev-list", "--count", headBefore+"..HEAD") != "1" {
		t.Fatalf("add and activate must be one commit")
	}
	if subject := hubGit(t, hub, "log", "-1", "--format=%s"); subject != "Add team-release from a project, activate" {
		t.Fatalf("subject = %q", subject)
	}
	meta, _ := os.ReadFile(filepath.Join(hub, "skills", "default", "team-release", ".meta", "skill.yaml"))
	if !strings.Contains(string(meta), "status: active") {
		t.Fatalf("not active:\n%s", meta)
	}
	if status := hubGit(t, hub, "status", "--porcelain", "--", "skills"); status != "" {
		t.Fatalf("skills left uncommitted:\n%s", status)
	}
}

func TestSkillCommitReplacesTheHubCopyOfAFolderThatWasNeverCheckedOut(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	seedHubSkill(t, hub, fixtureSkill{id: "alpha", files: map[string]string{"references/old.md": "old\n"}})
	commitHubAll(t, hub, "add alpha")
	project := newProjectDir(t)
	folder := filepath.Join(project, "alpha")
	writeProjectFile(t, folder, "SKILL.md", "---\nname: alpha\ndescription: Does alpha things. Use when you need alpha.\n---\n\n# alpha\n\nbrand new body\n", 0o644)

	preview := previewCommit(t, hub, project, "./alpha", SkillCommitInput{})
	if preview.Case != commitCaseReplace || len(preview.Warnings) == 0 {
		t.Fatalf("case=%s warnings=%v", preview.Case, preview.Warnings)
	}
	changes := map[string]string{}
	for _, file := range preview.Files {
		changes[file.Path] = file.Change
	}
	if changes["SKILL.md"] != "M" || changes["references/old.md"] != "D" {
		t.Fatalf("files = %+v", preview.Files)
	}
	if result, err := (SkillCommitService{}).Apply(commitCtx(), hub, preview, SkillCommitApplyOptions{}); err != nil || result.Error != nil {
		t.Fatalf("apply = %+v %v", result.Error, err)
	}
	if !strings.Contains(hubSkillMD(t, hub, "alpha"), "brand new body") {
		t.Fatalf("hub copy was not replaced")
	}
	if _, err := os.Stat(filepath.Join(hub, "skills", "default", "alpha", "references", "old.md")); !os.IsNotExist(err) {
		t.Fatalf("the file the folder lacks must be removed from the hub")
	}
}

func TestSkillCommitThirdPartyApprovesTheNewContentInTheSameCommit(t *testing.T) {
	t.Parallel()
	hub, project, folder := checkedOut(t, fixtureSkill{id: "pdf", origin: "github"})
	replaceInProject(t, folder, "line one", "line one, edited")
	preview := previewCommit(t, hub, project, "pdf", SkillCommitInput{})
	if preview.Trust == nil || !preview.Trust.ThirdParty || preview.Trust.Origin != "github.com/example/pdf" {
		t.Fatalf("trust = %+v", preview.Trust)
	}
	headBefore := hubGit(t, hub, "rev-parse", "HEAD")
	refused, err := (SkillCommitService{}).Apply(commitCtx(), hub, preview, SkillCommitApplyOptions{})
	if err != nil || refused.Error == nil || refused.Error.Code != ErrorContentReviewRequired {
		t.Fatalf("unapproved third-party apply = %+v %v", refused.Error, err)
	}
	if hubGit(t, hub, "rev-parse", "HEAD") != headBefore || strings.Contains(hubSkillMD(t, hub, "pdf"), "edited") {
		t.Fatalf("a refused commit changed the hub")
	}
	result, err := (SkillCommitService{}).Apply(commitCtx(), hub, preview, SkillCommitApplyOptions{Approve: true})
	if err != nil || result.Error != nil || !result.Approved {
		t.Fatalf("apply = %+v %v", result.Error, err)
	}
	if hubGit(t, hub, "rev-list", "--count", headBefore+"..HEAD") != "1" {
		t.Fatalf("content and approval must be one commit")
	}
	if subject := hubGit(t, hub, "log", "-1", "--format=%s"); subject != "Commit pdf from a project (1 file), approve content" {
		t.Fatalf("subject = %q", subject)
	}
	review, err := (SkillService{}).ReviewSkill(context.Background(), hub, "pdf")
	if err != nil {
		t.Fatal(err)
	}
	if !review.ContentTrust.Approved || review.ContentTrust.ContentDigest != preview.Trust.ContentDigest {
		t.Fatalf("content trust = %+v, want approved at %s", review.ContentTrust, preview.Trust.ContentDigest)
	}
}

func TestSkillCommitRefusesAHubSkillWithUncommittedEditsBeforeWritingAnything(t *testing.T) {
	t.Parallel()
	hub, project, folder := checkedOut(t, fixtureSkill{id: "pdf"})
	hubEntry := filepath.Join(hub, "skills", "default", "pdf", "SKILL.md")
	if err := os.WriteFile(hubEntry, []byte(strings.Replace(hubSkillMD(t, hub, "pdf"), "line three", "hand-edited three", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	replaceInProject(t, folder, "line one", "line 1 from the project")
	preview := previewCommit(t, hub, project, "pdf", SkillCommitInput{})
	head := hubGit(t, hub, "rev-parse", "HEAD")
	_, err := (SkillCommitService{}).Apply(commitCtx(), hub, preview, SkillCommitApplyOptions{})
	var uncommitted *mutation.UncommittedEditsError
	if !errors.As(err, &uncommitted) {
		t.Fatalf("err = %v, want the uncommitted edits refusal", err)
	}
	if hubGit(t, hub, "rev-parse", "HEAD") != head || strings.Contains(hubSkillMD(t, hub, "pdf"), "from the project") {
		t.Fatalf("a refused commit changed the hub")
	}
}

func TestSkillCommitKeepsTheDraftWhenActivationCannotHappen(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	project := newProjectDir(t)
	if _, err := ScaffoldProjectSkill(context.Background(), SkillScaffoldInput{SkillID: "deploy-notes", ProjectDir: project, Description: "Summarize a deploy", Triggers: []string{"after a deploy"}, NotFor: []string{"staging"}}); err != nil {
		t.Fatal(err)
	}
	preview := previewCommit(t, hub, project, "deploy-notes", SkillCommitInput{})
	if preview.ActivationReady {
		t.Fatalf("an untouched scaffold must not be activation ready")
	}
	head := hubGit(t, hub, "rev-parse", "HEAD")
	result, err := (SkillCommitService{}).Apply(commitCtx(), hub, preview, SkillCommitApplyOptions{Activate: true})
	if err != nil || result.Error != nil {
		t.Fatalf("apply = %+v %v", result.Error, err)
	}
	if result.Activated || len(result.Warnings) == 0 || result.Warnings[0].Code != "activation_failed" {
		t.Fatalf("result = %+v", result)
	}
	if hubGit(t, hub, "rev-list", "--count", head+"..HEAD") != "1" || hubGit(t, hub, "log", "-1", "--format=%s") != "Add deploy-notes from a project" {
		t.Fatalf("the draft must be committed alone, with a subject that does not claim activation")
	}
	if status := hubGit(t, hub, "status", "--porcelain", "--", "skills"); status != "" {
		t.Fatalf("skills left uncommitted:\n%s", status)
	}
}

func TestSkillCommitFindsTheBaseInTheHubHistoryWhenTheMarkerKeptNoCommit(t *testing.T) {
	t.Parallel()
	hub, project, folder := checkedOut(t, fixtureSkill{id: "pdf"})
	marker, _, err := ReadCheckoutMarker(folder)
	if err != nil {
		t.Fatal(err)
	}
	marker.BaseCommit = ""
	if err := WriteCheckoutMarker(folder, marker); err != nil {
		t.Fatal(err)
	}
	replaceInHub(t, hub, "pdf", "line three", "line 3 from the hub")
	replaceInProject(t, folder, "line one", "line 1 from the project")
	preview := previewCommit(t, hub, project, "pdf", SkillCommitInput{})
	if preview.Case != commitCaseMerge || len(preview.Conflicts) != 0 || preview.Merge[0].Status != "changed in both, merged" {
		t.Fatalf("merge = %+v conflicts = %v", preview.Merge, preview.Conflicts)
	}
}

func TestSkillCommitFindsTheBaseInAServedSnapshotWhenHistoryHasNone(t *testing.T) {
	t.Parallel()
	hub := newGitHub(t)
	dir := seedHubSkill(t, hub, fixtureSkill{id: "pdf"})
	commitHubAll(t, hub, "add pdf")
	// An uncommitted edit at checkout leaves no commit holding the base.
	dirty := strings.Replace(hubSkillMD(t, hub, "pdf"), "line one", "dirty one", 1)
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(dirty), 0o644); err != nil {
		t.Fatal(err)
	}
	project := newProjectDir(t)
	result, err := (SkillCheckoutService{}).Checkout(context.Background(), hub, SkillCheckoutInput{SkillID: "pdf", ProjectDir: project})
	if err != nil || result.Error != nil || result.BaseCommit != "" {
		t.Fatalf("checkout = %+v %v", result, err)
	}
	folder := filepath.Join(project, ".claude", "skills", "pdf")
	// The snapshot the runtime served at that moment is still in the cache.
	snapshot := filepath.Join(hub, "runtime", "cache", "skills", "pdf@0123456789abcdef")
	writeProjectFile(t, snapshot, "SKILL.md", dirty, 0o644)
	marker := `{"schema":1,"skill_id":"pdf","manifest_version":"v","files":[{"path":"SKILL.md","digest":"` + digestOf([]byte(dirty)) + `","size":` + strconv.Itoa(len(dirty)) + `}]}`
	writeProjectFile(t, snapshot, ".skillhub-snapshot.json", marker, 0o644)

	replaceInHub(t, hub, "pdf", "line three", "line 3 from the hub")
	replaceInProject(t, folder, "# pdf", "# pdf from the project")
	preview := previewCommit(t, hub, project, "pdf", SkillCommitInput{})
	if preview.Case != commitCaseMerge || len(preview.Conflicts) != 0 || preview.Merge[0].Status != "changed in both, merged" {
		t.Fatalf("merge = %+v conflicts = %v", preview.Merge, preview.Conflicts)
	}
}
