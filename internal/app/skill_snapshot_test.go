package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/skill"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	"gopkg.in/yaml.v3"
)

// updateSkillMeta rewrites skill.meta.yaml of a core-collection skill.
func updateSkillMeta(t *testing.T, root, id string, mutate func(map[string]any)) {
	t.Helper()
	metaPath := filepath.Join(root, "skills", "core", id, "skill.meta.yaml")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	mutate(document)
	encoded, err := yaml.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metaPath, encoded, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeSkillFile(t *testing.T, root, id, relative, contents string) {
	t.Helper()
	target := filepath.Join(root, "skills", "core", id, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func markThirdParty(document map[string]any) {
	document["provenance"] = map[string]any{
		"created_by": "skillhub",
		"origin":     map[string]any{"kind": "github", "repository": "https://github.com/example/skills", "commit": strings.Repeat("a", 40)},
	}
}

func withRuntime(document map[string]any) {
	document["runtime"] = map[string]any{
		"requires": map[string]any{"bins": []any{"python3"}, "env": []any{"SNAPSHOT_TEST_TOKEN"}},
		"setup":    map[string]any{"command": "pip install -r requirements.txt", "check": "python3 scripts/check.py"},
	}
}

func setReviewedDigest(digest string) func(map[string]any) {
	return func(document map[string]any) {
		quality, _ := document["quality"].(map[string]any)
		if quality == nil {
			quality = map[string]any{}
		}
		quality["content_reviewed_digest"] = digest
		document["quality"] = quality
	}
}

func fakeSnapshotService() SnapshotService {
	return SnapshotService{
		goos: "linux",
		memo: &snapshotMemo{entries: map[string]snapshotMemoEntry{}},
	}
}

func fileDigest(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func ensureSnapshot(t *testing.T, service SnapshotService, root, id string) LocalSkill {
	t.Helper()
	local, err := service.Ensure(t.Context(), root, id)
	if err != nil {
		t.Fatalf("Ensure(%s) error = %v", id, err)
	}
	return local
}

func assertSnapshotMatchesManifest(t *testing.T, root string, local LocalSkill) {
	t.Helper()
	cacheRoot := filepath.Join(root, "runtime", "cache", "skills") + string(filepath.Separator)
	if !filepath.IsAbs(local.Path) || !strings.HasPrefix(local.Path, cacheRoot) {
		t.Fatalf("snapshot path %q is not inside %q", local.Path, cacheRoot)
	}
	if strings.HasPrefix(local.Path, filepath.Join(root, "skills")) {
		t.Fatalf("snapshot path %q points at the live workspace", local.Path)
	}
	entries, _, err := (DistributionService{}).LookupSkills(t.Context(), root, []string{filepath.Base(strings.SplitN(filepath.Base(local.Path), "@", 2)[0])})
	if err != nil {
		t.Fatal(err)
	}
	var entry DistributedSkill
	for _, value := range entries {
		entry = value
	}
	if entry.Version != local.ManifestVersion {
		t.Fatalf("manifest version = %s, want %s", local.ManifestVersion, entry.Version)
	}
	digests := map[string]string{}
	for _, resource := range entry.Resources {
		parts := strings.SplitN(strings.TrimPrefix(resource.URI, "skill://skillhub/"), "/", 3)
		digests[parts[2]] = resource.Digest
	}
	for _, resource := range local.Resources {
		if got := fileDigest(t, resource.LocalPath); got != digests[resource.Path] {
			t.Fatalf("snapshot file %s digest = %s, want %s", resource.Path, got, digests[resource.Path])
		}
		if runtime.GOOS != "windows" {
			info, err := os.Stat(resource.LocalPath)
			if err != nil {
				t.Fatal(err)
			}
			// The catalog records no file modes, so every file is plain read-only.
			want := os.FileMode(0o444)
			if info.Mode().Perm() != want {
				t.Fatalf("snapshot file %s mode = %v, want %v", resource.Path, info.Mode().Perm(), want)
			}
		}
	}
	if _, err := os.Lstat(filepath.Join(local.Path, snapshotMarkerName)); err != nil {
		t.Fatalf("snapshot marker missing: %v", err)
	}
}

func TestSnapshotExportsVerifiedCopyAndReusesIt(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "snap-basic", "Snap Basic")
	writeSkillFile(t, root, "snap-basic", "references/guide.md", "# Guide\n")
	service := fakeSnapshotService()

	local := ensureSnapshot(t, service, root, "snap-basic")
	if local.Status != LocalStatusReady || len(local.ReasonCodes) != 0 || local.Preflight != nil {
		t.Fatalf("local = %#v", local)
	}
	digest := strings.TrimPrefix(local.ManifestVersion, "sha256:")
	if filepath.Base(local.Path) != "snap-basic@"+digest[:16] {
		t.Fatalf("snapshot dir = %s", filepath.Base(local.Path))
	}
	if len(local.Resources) != 2 {
		t.Fatalf("resources = %#v", local.Resources)
	}
	for _, resource := range local.Resources {
		if resource.Path == "references/guide.md" && resource.Kind != "reference" {
			t.Fatalf("guide kind = %q", resource.Kind)
		}
		if resource.Path == "SKILL.md" && resource.Kind != "instructions" {
			t.Fatalf("SKILL.md kind = %q", resource.Kind)
		}
	}
	assertSnapshotMatchesManifest(t, root, local)

	entrypoint := filepath.Join(local.Path, "SKILL.md")
	before, err := os.Stat(entrypoint)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(local.Path, old, old); err != nil {
		t.Fatal(err)
	}
	again := ensureSnapshot(t, service, root, "snap-basic")
	if again.Path != local.Path {
		t.Fatalf("second path = %s, want %s", again.Path, local.Path)
	}
	after, err := os.Stat(entrypoint)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(before.ModTime()) || !os.SameFile(before, after) {
		t.Fatal("reuse rewrote the snapshot file")
	}
	dirInfo, err := os.Stat(local.Path)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(dirInfo.ModTime()) > time.Hour {
		t.Fatalf("reuse did not touch the snapshot mtime: %v", dirInfo.ModTime())
	}
}

func TestSnapshotRebuildsTamperedFile(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "snap-tamper", "Snap Tamper")
	service := fakeSnapshotService()
	local := ensureSnapshot(t, service, root, "snap-tamper")
	entrypoint := filepath.Join(local.Path, "SKILL.md")
	want := fileDigest(t, entrypoint)
	if err := os.Chmod(entrypoint, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entrypoint, []byte("# injected\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	extra := filepath.Join(local.Path, "node_modules")
	if err := os.Mkdir(extra, 0o755); err != nil {
		t.Fatal(err)
	}

	rebuilt := ensureSnapshot(t, service, root, "snap-tamper")
	if rebuilt.Path != local.Path || fileDigest(t, entrypoint) != want {
		t.Fatalf("tampered snapshot was not rebuilt: %#v", rebuilt)
	}
	assertSnapshotMatchesManifest(t, root, rebuilt)

	// Setup artifacts beside the skill files do not invalidate a snapshot.
	if err := os.Mkdir(filepath.Join(rebuilt.Path, ".venv"), 0o755); err != nil {
		t.Fatal(err)
	}
	before, _ := os.Stat(entrypoint)
	ensureSnapshot(t, service, root, "snap-tamper")
	after, _ := os.Stat(entrypoint)
	if !os.SameFile(before, after) {
		t.Fatal("extra setup artifacts triggered a rebuild")
	}
}

func TestSnapshotRefusesSymlinkedFileInsideSnapshot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "snap-link", "Snap Link")
	service := fakeSnapshotService()
	local := ensureSnapshot(t, service, root, "snap-link")
	entrypoint := filepath.Join(local.Path, "SKILL.md")
	want := fileDigest(t, entrypoint)
	outside := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(outside, []byte("# outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(entrypoint); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, entrypoint); err != nil {
		t.Fatal(err)
	}
	ensureSnapshot(t, service, root, "snap-link")
	info, err := os.Lstat(entrypoint)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 || fileDigest(t, entrypoint) != want {
		t.Fatal("symlinked snapshot file was reused")
	}
}

func TestSnapshotWorkingTreeDriftIsUnavailable(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "snap-drift", "Snap Drift")
	writeSkillFile(t, root, "snap-drift", "SKILL.md", "---\nname: snap-drift\ndescription: Mutated.\n---\n\n# Mutated\n")
	// Invalid metadata keeps the published generation in place instead of rebuilding it.
	writeSkillFile(t, root, "snap-drift", "skill.meta.yaml", "schema_version: 1\nid: snap-drift\nstatus: shiny\n")

	local := ensureSnapshot(t, fakeSnapshotService(), root, "snap-drift")
	if local.Status != LocalStatusUnavailable || !slices.Equal(local.ReasonCodes, []string{LocalReasonResourceContentUnavailable}) || local.Path != "" {
		t.Fatalf("local = %#v", local)
	}
}

func TestSnapshotDraftSkillIsUnavailable(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	service := SkillService{}
	preview, err := service.PreviewCreate(t.Context(), root, skill.CreateInput{
		ID: "snap-draft", Collection: "core", Name: "Snap Draft", Description: "Draft fixture.",
		Content: []byte("---\nname: snap-draft\ndescription: Draft fixture.\n---\n\n# Draft\n"),
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := service.ConfirmSkillMutation(t.Context(), root, preview, preview.Confirmation.Confirmation.Pins); err != nil || result.Error != nil {
		t.Fatalf("create = %#v, %v", result, err)
	}
	for _, id := range []string{"snap-draft", "missing-skill"} {
		local := ensureSnapshot(t, fakeSnapshotService(), root, id)
		if local.Status != LocalStatusUnavailable || !slices.Equal(local.ReasonCodes, []string{LocalReasonNotServable}) {
			t.Fatalf("%s local = %#v", id, local)
		}
	}
}

func TestSnapshotRequiresContentReviewForThirdPartySkill(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "snap-third", "Snap Third")
	writeSkillFile(t, root, "snap-third", "scripts/x.sh", "#!/bin/sh\necho hi\n")
	writeSkillFile(t, root, "snap-third", "references/guide.md", "# Guide\n")
	writeSkillFile(t, root, "snap-third", "requirements.txt", "requests==2.0\n")
	updateSkillMeta(t, root, "snap-third", func(document map[string]any) {
		markThirdParty(document)
		withRuntime(document)
	})
	service := fakeSnapshotService()

	local := ensureSnapshot(t, service, root, "snap-third")
	if local.Status != LocalStatusReviewRequired || !slices.Equal(local.ReasonCodes, []string{skillruntime.ReasonContentReviewRequired}) {
		t.Fatalf("local = %#v", local)
	}
	if local.Path != "" || len(local.Resources) != 0 || local.Preflight != nil || local.ReviewCommand != "skillhub skill review snap-third" {
		t.Fatalf("an unapproved skill must expose no path, resources, or preflight: %#v", local)
	}
	for _, directory := range []string{filepath.Join("runtime", "cache", "skills"), filepath.Join("runtime", "envs")} {
		if _, err := os.Lstat(filepath.Join(root, directory)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s was created for an unapproved skill: %v", directory, err)
		}
	}
	// The entrypoint stays readable through the distribution service.
	entries, _, err := (DistributionService{}).LookupSkills(t.Context(), root, []string{"snap-third"})
	if err != nil || entries["snap-third"].URI == "" {
		t.Fatalf("entrypoint must stay distributable: %v", err)
	}

	review, err := (SkillService{}).ReviewSkill(t.Context(), root, "snap-third")
	if err != nil {
		t.Fatal(err)
	}
	trust := review.ContentTrust
	if !trust.ThirdParty || trust.Approved || !trust.RequiresReview() || !strings.HasPrefix(trust.ContentDigest, "sha256:") {
		t.Fatalf("review content trust = %#v", trust)
	}
	if want := "skillhub skill edit snap-third --approve-content " + trust.ContentDigest; trust.ApproveCommand != want {
		t.Fatalf("approve command = %q, want %q", trust.ApproveCommand, want)
	}

	// Approving the digest the review printed serves the full snapshot.
	updateSkillMeta(t, root, "snap-third", setReviewedDigest(trust.ContentDigest))
	approved := ensureSnapshot(t, service, root, "snap-third")
	if approved.Status != LocalStatusReady || len(approved.ReasonCodes) != 0 || approved.Path == "" || approved.ReviewCommand != "" {
		t.Fatalf("approved = %#v", approved)
	}
	if approved.ManifestVersion != local.ManifestVersion {
		t.Fatalf("approval changed the manifest version: %s != %s", approved.ManifestVersion, local.ManifestVersion)
	}
	for _, relative := range []string{"scripts/x.sh", "references/guide.md", "requirements.txt"} {
		if _, err := os.Lstat(filepath.Join(approved.Path, filepath.FromSlash(relative))); err != nil {
			t.Fatalf("approved file %s missing: %v", relative, err)
		}
	}
	if approved.Preflight == nil || approved.Preflight.Check != "python3 scripts/check.py" || approved.Preflight.Setup == "" {
		t.Fatalf("approved preflight = %#v", approved.Preflight)
	}
	assertSnapshotMatchesManifest(t, root, approved)
	review, err = (SkillService{}).ReviewSkill(t.Context(), root, "snap-third")
	if err != nil || !review.ContentTrust.Approved || review.ContentTrust.RequiresReview() || review.ContentTrust.ApproveCommand != "" {
		t.Fatalf("approved review trust = %#v, %v", review.ContentTrust, err)
	}

	// Any content change, here to a reference file, makes the approval stale.
	writeSkillFile(t, root, "snap-third", "references/guide.md", "# Guide, edited\n")
	stale := ensureSnapshot(t, service, root, "snap-third")
	if stale.Status != LocalStatusReviewRequired || !slices.Equal(stale.ReasonCodes, []string{skillruntime.ReasonContentReviewRequired, skillruntime.ReasonContentReviewStale}) || stale.Path != "" {
		t.Fatalf("stale = %#v", stale)
	}
	// So does a change to the runtime block.
	review, err = (SkillService{}).ReviewSkill(t.Context(), root, "snap-third")
	if err != nil {
		t.Fatal(err)
	}
	updateSkillMeta(t, root, "snap-third", setReviewedDigest(review.ContentTrust.ContentDigest))
	if again := ensureSnapshot(t, service, root, "snap-third"); again.Status != LocalStatusReady {
		t.Fatalf("re-approval did not restore the skill: %#v", again)
	}
	updateSkillMeta(t, root, "snap-third", func(document map[string]any) {
		document["runtime"].(map[string]any)["setup"] = map[string]any{"command": "pip install evil", "check": "true"}
	})
	if changed := ensureSnapshot(t, service, root, "snap-third"); changed.Status != LocalStatusReviewRequired {
		t.Fatalf("runtime change kept the approval: %#v", changed)
	}
}

func TestSnapshotServesLocalSkillScripts(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "snap-local", "Snap Local")
	writeSkillFile(t, root, "snap-local", "scripts/run.py", "print('run')\n")
	local := ensureSnapshot(t, fakeSnapshotService(), root, "snap-local")
	if local.Status != LocalStatusReady {
		t.Fatalf("local = %#v", local)
	}
	found := false
	for _, resource := range local.Resources {
		if resource.Path == "scripts/run.py" {
			found = resource.Kind == "script"
		}
	}
	if !found {
		t.Fatalf("script not exported: %#v", local.Resources)
	}
	assertSnapshotMatchesManifest(t, root, local)
}

func TestSnapshotTrustedSkillWithoutRuntimeBlockGetsStateDirectoryAndEnv(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "snap-prose", "Snap Prose")
	local := ensureSnapshot(t, fakeSnapshotService(), root, "snap-prose")
	if local.Status != LocalStatusReady || local.Preflight != nil {
		t.Fatalf("local = %#v", local)
	}
	info, err := os.Stat(local.StateDirectory)
	if err != nil || !info.IsDir() || !strings.HasPrefix(local.StateDirectory, filepath.Join(root, "runtime", "envs")+string(filepath.Separator)) {
		t.Fatalf("state directory %q: %v", local.StateDirectory, err)
	}
	if local.Env[skillruntime.EnvSkillDir] != local.Path || local.Env[skillruntime.EnvStateDir] != local.StateDirectory || local.Env[skillruntime.EnvConfigDir] != filepath.Join(root, "runtime", "config", "snap-prose") || len(local.Env) != 3 {
		t.Fatalf("env = %#v", local.Env)
	}
	writeSkillFile(t, root, "snap-prose", "requirements.txt", "requests\n")
	withDeps := ensureSnapshot(t, fakeSnapshotService(), root, "snap-prose")
	if withDeps.StateDirectory == local.StateDirectory {
		t.Fatalf("dependency manifest must select a new state directory: %q", withDeps.StateDirectory)
	}
	if _, err := os.Stat(filepath.Join(withDeps.StateDirectory, "requirements.txt")); err != nil {
		t.Fatalf("dependency copy missing: %v", err)
	}
}

func TestSnapshotPreflightChecksOnlyPlatformAndPointsAtStateDirectory(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "snap-runtime", "Snap Runtime")
	writeSkillFile(t, root, "snap-runtime", "scripts/check.py", "print('ok')\n")
	updateSkillMeta(t, root, "snap-runtime", func(document map[string]any) {
		withRuntime(document)
		document["runtime"].(map[string]any)["requires"].(map[string]any)["platforms"] = []any{"linux"}
	})
	service := fakeSnapshotService()

	local := ensureSnapshot(t, service, root, "snap-runtime")
	plan := local.Preflight
	if plan == nil || plan.State != skillruntime.StateUnknown || plan.Doctor != nil {
		t.Fatalf("preflight = %#v", plan)
	}
	if !strings.HasPrefix(plan.StateDirectory, filepath.Join(root, "runtime", "envs")+string(filepath.Separator)) || plan.WorkingDirectory != plan.StateDirectory {
		t.Fatalf("state directory = %q, working directory = %q", plan.StateDirectory, plan.WorkingDirectory)
	}
	if plan.Env[skillruntime.EnvSkillDir] != local.Path || plan.Env[skillruntime.EnvStateDir] != plan.StateDirectory || plan.Env[skillruntime.EnvConfigDir] != filepath.Join(root, "runtime", "config", "snap-runtime") || len(plan.Env) != 3 {
		t.Fatalf("env = %#v", plan.Env)
	}
	if len(plan.Requires.Bins) != 1 || plan.Requires.Bins[0].Name != "python3" || !slices.Equal(plan.Requires.Env, []string{"SNAPSHOT_TEST_TOKEN"}) {
		t.Fatalf("requires = %#v", plan.Requires)
	}
	if len(plan.LiveChecks) != 1 || plan.LiveChecks[0].Kind != skillruntime.KindPlatform || plan.LiveChecks[0].Status != skillruntime.StatusPass {
		t.Fatalf("live checks must hold only the platform check: %#v", plan.LiveChecks)
	}
	for _, name := range []string{skillruntime.EnvSkillDir, skillruntime.EnvStateDir} {
		if !strings.Contains(plan.Instruction, name) {
			t.Fatalf("instruction does not mention %s: %q", name, plan.Instruction)
		}
	}
	if plan.Check == "" || !strings.Contains(plan.Instruction, "working_directory") {
		t.Fatalf("preflight commands = %#v", plan)
	}

	spec, _, err := skillruntime.ParseSpec(readCatalogContentJSON(t, root, "snap-runtime"))
	if err != nil {
		t.Fatal(err)
	}
	checkedAt := time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC)
	if err := skillruntime.WriteCache(root, "snap-runtime", skillruntime.RuntimeFingerprint(local.ManifestVersion, spec), skillruntime.Result{State: skillruntime.StateReady, Basis: skillruntime.BasisTerminal, CheckedAt: checkedAt}); err != nil {
		t.Fatal(err)
	}
	cached := ensureSnapshot(t, service, root, "snap-runtime")
	if cached.Preflight.State != skillruntime.StateReady || cached.Preflight.Doctor == nil || cached.Preflight.Doctor.CheckedAt != "2026-10-04T08:00:00Z" || cached.Preflight.Doctor.Basis != skillruntime.BasisTerminal {
		t.Fatalf("cached preflight = %#v", cached.Preflight)
	}

	other := service
	other.goos = "windows"
	failing := ensureSnapshot(t, other, root, "snap-runtime")
	if failing.Preflight.State != skillruntime.StateUnsupportedPlatform {
		t.Fatalf("wrong platform preflight = %#v", failing.Preflight)
	}
}

func TestStateDirectoryIsStableAcrossContentEditsAndChangesWithDependencies(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "snap-state", "Snap State")
	writeSkillFile(t, root, "snap-state", "package.json", `{"name":"x"}`+"\n")
	writeSkillFile(t, root, "snap-state", "package-lock.json", `{"lockfileVersion":3}`+"\n")
	writeSkillFile(t, root, "snap-state", "scripts/requirements.txt", "requests==2.0\n")
	updateSkillMeta(t, root, "snap-state", withRuntime)
	service := fakeSnapshotService()

	first := ensureSnapshot(t, service, root, "snap-state")
	state := first.Preflight.StateDirectory
	info, err := os.Lstat(state)
	if err != nil || !info.IsDir() {
		t.Fatalf("state directory missing: %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatalf("state directory mode = %v", info.Mode().Perm())
	}
	for _, relative := range []string{"package.json", "package-lock.json", "scripts/requirements.txt"} {
		copied := filepath.Join(state, filepath.FromSlash(relative))
		stat, err := os.Stat(copied)
		if err != nil {
			t.Fatalf("dependency %s was not copied: %v", relative, err)
		}
		if runtime.GOOS != "windows" && stat.Mode().Perm()&0o200 == 0 {
			t.Fatalf("dependency copy %s is not writable: %v", relative, stat.Mode().Perm())
		}
	}
	if _, err := os.Lstat(filepath.Join(state, "SKILL.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("only dependency files are copied: %v", err)
	}

	// An installer rewrites the lock file in the state directory; reuse keeps it.
	lockCopy := filepath.Join(state, "package-lock.json")
	if err := os.WriteFile(lockCopy, []byte("rewritten by installer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	installed := filepath.Join(state, "node_modules")
	if err := os.Mkdir(installed, 0o755); err != nil {
		t.Fatal(err)
	}

	// A SKILL.md edit changes the snapshot but keeps the state directory and its contents.
	writeSkillFile(t, root, "snap-state", "SKILL.md", "---\nname: snap-state\ndescription: Edited.\n---\n\n# Edited\n")
	edited := ensureSnapshot(t, service, root, "snap-state")
	if edited.ManifestVersion == first.ManifestVersion || edited.Path == first.Path {
		t.Fatalf("SKILL.md edit did not produce a new snapshot: %#v", edited)
	}
	if edited.Preflight.StateDirectory != state {
		t.Fatalf("SKILL.md edit moved the state directory: %s -> %s", state, edited.Preflight.StateDirectory)
	}
	if data, err := os.ReadFile(lockCopy); err != nil || string(data) != "rewritten by installer\n" {
		t.Fatalf("reuse overwrote the dependency copy: %q, %v", data, err)
	}
	if _, err := os.Lstat(installed); err != nil {
		t.Fatalf("installed dependencies were lost: %v", err)
	}

	// A dependency file edit selects a different state directory.
	writeSkillFile(t, root, "snap-state", "package-lock.json", `{"lockfileVersion":3,"changed":true}`+"\n")
	relocked := ensureSnapshot(t, service, root, "snap-state")
	if relocked.Preflight.StateDirectory == state {
		t.Fatal("a lock file change kept the old state directory")
	}
	if data, err := os.ReadFile(filepath.Join(relocked.Preflight.StateDirectory, "package-lock.json")); err != nil || !strings.Contains(string(data), "changed") {
		t.Fatalf("new state directory lacks the new lock file: %q, %v", data, err)
	}
	// The old directory is untouched until garbage collection.
	if _, err := os.Lstat(installed); err != nil {
		t.Fatalf("old state directory was disturbed: %v", err)
	}
}

func TestStateDirectoryGarbageCollection(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "snap-envgc", "Snap Env GC")
	writeSkillFile(t, root, "snap-envgc", "package-lock.json", "{}\n")
	updateSkillMeta(t, root, "snap-envgc", withRuntime)
	createActiveDistributionSkill(t, root, "snap-envgc-third", "Snap Env GC Third")
	updateSkillMeta(t, root, "snap-envgc-third", func(document map[string]any) {
		markThirdParty(document)
		withRuntime(document)
	})
	service := fakeSnapshotService()
	current := ensureSnapshot(t, service, root, "snap-envgc").Preflight.StateDirectory

	envs := filepath.Join(root, "runtime", "envs")
	staleOld := filepath.Join(envs, "snap-envgc@0000000000000000")
	staleRecent := filepath.Join(envs, "snap-envgc@1111111111111111")
	unapproved := filepath.Join(envs, "snap-envgc-third@2222222222222222")
	oldStaging := filepath.Join(envs, stateStagingPrefix+"abandoned")
	for _, directory := range []string{staleOld, staleRecent, unapproved, oldStaging} {
		if err := os.MkdirAll(filepath.Join(directory, "node_modules", "pkg"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "node_modules", "pkg", "index.js"), []byte("x"), 0o444); err != nil {
			t.Fatal(err)
		}
	}
	// Read-only directories (as left by some package managers) must not block removal.
	if err := os.Chmod(filepath.Join(staleOld, "node_modules", "pkg"), 0o555); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-8 * 24 * time.Hour)
	for _, directory := range []string{current, staleOld, unapproved} {
		if err := os.Chtimes(directory, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(oldStaging, time.Now().Add(-2*time.Hour), time.Now().Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := service.CollectGarbage(t.Context(), root, SnapshotGCMinAge); err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{staleOld, unapproved, oldStaging} {
		if _, err := os.Lstat(removed); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s was not collected: %v", removed, err)
		}
	}
	for _, kept := range []string{current, staleRecent} {
		if _, err := os.Lstat(kept); err != nil {
			t.Fatalf("%s was collected: %v", kept, err)
		}
	}
}

func TestSnapshotConcurrentEnsureSettlesOnOneValidDirectory(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "snap-race", "Snap Race")
	writeSkillFile(t, root, "snap-race", "references/a.md", "# A\n")
	if _, err := (CatalogService{}).BuildCatalogGeneration(t.Context(), root); err != nil {
		t.Fatal(err)
	}
	service := fakeSnapshotService()
	const workers = 4
	results := make([]LocalSkill, workers)
	errs := make([]error, workers)
	var group sync.WaitGroup
	for index := range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			results[index], errs[index] = service.Ensure(t.Context(), root, "snap-race")
		}()
	}
	group.Wait()
	for index := range workers {
		if errs[index] != nil || results[index].Status != LocalStatusReady || results[index].Path != results[0].Path {
			t.Fatalf("worker %d = %#v, %v", index, results[index], errs[index])
		}
	}
	assertSnapshotMatchesManifest(t, root, results[0])
	entries, err := os.ReadDir(filepath.Join(root, "runtime", "cache", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		if entry.Name() != snapshotStagingName {
			names = append(names, entry.Name())
		}
	}
	if len(names) != 1 {
		t.Fatalf("snapshot entries = %v", names)
	}
	staging, err := os.ReadDir(filepath.Join(root, "runtime", "cache", "skills", snapshotStagingName))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if len(staging) != 0 {
		t.Fatalf("staging leftovers = %v", staging)
	}
}

func TestSnapshotGarbageCollectionKeepsCurrentAndRemovesOutdated(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "snap-gc", "Snap GC")
	service := fakeSnapshotService()
	current := ensureSnapshot(t, service, root, "snap-gc")
	cacheRoot := filepath.Join(root, "runtime", "cache", "skills")
	outdated := filepath.Join(cacheRoot, "snap-gc@0000000000000000")
	recent := filepath.Join(cacheRoot, "snap-gc@1111111111111111")
	oldStaging := filepath.Join(cacheRoot, snapshotStagingName, "abandoned")
	for _, directory := range []string{outdated, recent, oldStaging} {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte("# old\n"), 0o444); err != nil {
			t.Fatal(err)
		}
	}
	old := time.Now().Add(-48 * time.Hour)
	for _, directory := range []string{outdated, current.Path, oldStaging} {
		if err := os.Chtimes(directory, old, old); err != nil {
			t.Fatal(err)
		}
	}
	if err := service.CollectGarbage(t.Context(), root, SnapshotGCMinAge); err != nil {
		t.Fatal(err)
	}
	for _, removed := range []string{outdated, oldStaging} {
		if _, err := os.Lstat(removed); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s was not collected: %v", removed, err)
		}
	}
	for _, kept := range []string{current.Path, recent} {
		if _, err := os.Lstat(kept); err != nil {
			t.Fatalf("%s was collected: %v", kept, err)
		}
	}
}

func TestSnapshotRefusesSymlinkedCacheDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on Windows")
	}
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "snap-escape", "Snap Escape")
	cacheDir := filepath.Join(root, "runtime", "cache")
	if err := os.RemoveAll(cacheDir); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, cacheDir); err != nil {
		t.Fatal(err)
	}
	if _, err := (SnapshotService{}).Ensure(t.Context(), root, "snap-escape"); err == nil || !strings.Contains(err.Error(), "unsafe snapshot directory") {
		t.Fatalf("Ensure through a symlinked cache dir error = %v", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("snapshot wrote outside the workspace: %v", entries)
	}
	if err := (SnapshotService{}).CollectGarbage(t.Context(), root, 0); err == nil {
		t.Fatal("garbage collection followed a symlinked cache directory")
	}
}

func TestValidateSnapshotRelativePath(t *testing.T) {
	t.Parallel()
	for _, valid := range []string{"SKILL.md", "scripts/run.py", "a/b/c.txt"} {
		if err := validateSnapshotRelativePath(valid); err != nil {
			t.Fatalf("%q rejected: %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "/etc/passwd", "../x", "a/../../x", "a//b", "./a", `a\b`, "C:/x", "a/"} {
		if err := validateSnapshotRelativePath(invalid); err == nil {
			t.Fatalf("%q accepted", invalid)
		}
	}
}

func TestSnapshotMemoSkipsRehashUntilStatChanges(t *testing.T) {
	t.Parallel()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "snap-memo", "Snap Memo")
	service := fakeSnapshotService()
	local := ensureSnapshot(t, service, root, "snap-memo")
	entrypoint := filepath.Join(local.Path, "SKILL.md")
	original, err := os.ReadFile(entrypoint)
	if err != nil {
		t.Fatal(err)
	}
	// A second call records nothing new but must hit the memo.
	ensureSnapshot(t, service, root, "snap-memo")
	info, err := os.Stat(entrypoint)
	if err != nil {
		t.Fatal(err)
	}

	// Same-size content swap with the original mtime restored is invisible to
	// the stat check, which proves a memo hit does not re-hash file contents.
	tampered := []byte(strings.Repeat("x", len(original)))
	if err := os.Chmod(entrypoint, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entrypoint, tampered, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(entrypoint, info.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(entrypoint, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	ensureSnapshot(t, service, root, "snap-memo")
	if got, _ := os.ReadFile(entrypoint); string(got) != string(tampered) {
		t.Fatal("memo hit unexpectedly re-hashed and rebuilt the snapshot")
	}

	// A process without a memo verifies in full and repairs the snapshot.
	fresh := fakeSnapshotService()
	fresh.memo = &snapshotMemo{entries: map[string]snapshotMemoEntry{}}
	ensureSnapshot(t, fresh, root, "snap-memo")
	if got, _ := os.ReadFile(entrypoint); string(got) != string(original) {
		t.Fatal("first use in a process did not detect the tampered file")
	}

	// A stat change on a memoized file forces full verification again.
	ensureSnapshot(t, service, root, "snap-memo")
	if err := os.Chmod(entrypoint, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(entrypoint, tampered, 0o644); err != nil {
		t.Fatal(err)
	}
	ensureSnapshot(t, service, root, "snap-memo")
	if got, _ := os.ReadFile(entrypoint); string(got) != string(original) {
		t.Fatal("changed file stat did not trigger re-verification")
	}

	// A removed snapshot directory is re-exported despite the memo.
	removeSnapshotTree(local.Path)
	again := ensureSnapshot(t, service, root, "snap-memo")
	assertSnapshotMatchesManifest(t, root, again)
}
