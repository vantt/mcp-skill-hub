package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	"github.com/vantt/mcp-skill-hub/internal/systemskills"
)

// newBundleWorkspace has one active local skill with companion files, hub
// metadata under .meta/, and a runtime block.
func newBundleWorkspace(t *testing.T) string {
	t.Helper()
	root := newSkillWorkspace(t)
	createActiveDistributionSkill(t, root, "bundle-skill", "Bundle Skill")
	writeSkillFile(t, root, "bundle-skill", "references/guide.md", "# Guide\n\nRead me.\n")
	writeSkillFile(t, root, "bundle-skill", "scripts/check.sh", "#!/bin/sh\necho ready\n")
	writeSkillFile(t, root, "bundle-skill", "requirements.txt", "requests==2.0\n")
	writeSkillFile(t, root, "bundle-skill", ".meta/notes.yaml", "curation: only on the hub\n")
	updateSkillMeta(t, root, "bundle-skill", withRuntime)
	rebuildBundleCatalog(t, root)
	return root
}

func rebuildBundleCatalog(t *testing.T, root string) {
	t.Helper()
	if _, err := (CatalogService{}).BuildCatalogGeneration(t.Context(), root); err != nil {
		t.Fatal(err)
	}
}

func TestBundleLimitsMatchTheCatalog(t *testing.T) {
	if skillruntime.BundleMaxFiles != catalog.MaxDistributedSkillResources || skillruntime.BundleMaxBytes != catalog.MaxDistributedSkillBytes {
		t.Fatalf("bundle limits %d files / %d bytes differ from the catalog's %d / %d", skillruntime.BundleMaxFiles, skillruntime.BundleMaxBytes, catalog.MaxDistributedSkillResources, catalog.MaxDistributedSkillBytes)
	}
}

func TestSkillBundleHoldsExactlyTheDistributedFiles(t *testing.T) {
	t.Parallel()
	root := newBundleWorkspace(t)
	service := DistributionService{}
	entries, _, err := service.LookupSkills(t.Context(), root, []string{"bundle-skill"})
	if err != nil {
		t.Fatal(err)
	}
	entry := entries["bundle-skill"]

	bundle, err := service.SkillBundle(t.Context(), root, "bundle-skill", nil)
	if err != nil {
		t.Fatal(err)
	}
	if "sha256:"+bundle.Digest != entry.Version || !strings.Contains(entry.URI, bundle.Digest) {
		t.Fatalf("bundle digest %s does not match the skill URI %s", bundle.Digest, entry.URI)
	}
	again, err := service.SkillBundle(t.Context(), root, "bundle-skill", nil)
	if err != nil || !bytes.Equal(again.Archive, bundle.Archive) {
		t.Fatalf("a second bundle of the same version differs (%v)", err)
	}

	cache := skillruntime.RemoteCache{Root: t.TempDir()}
	cached, err := cache.Install("hub-test", "bundle-skill", bundle.Digest, bytes.NewReader(bundle.Archive))
	if err != nil {
		t.Fatal(err)
	}
	if len(cached.Metadata.Files) != len(entry.Resources) {
		t.Fatalf("bundle has %d files, the skill URI pins %d", len(cached.Metadata.Files), len(entry.Resources))
	}
	for _, file := range cached.Metadata.Files {
		if strings.HasPrefix(file.Path, ".meta/") {
			t.Fatalf("hub metadata %s was bundled", file.Path)
		}
		want, err := os.ReadFile(filepath.Join(root, "skills", "core", "bundle-skill", filepath.FromSlash(file.Path)))
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(cached.Directory, filepath.FromSlash(file.Path)))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("%s differs from the hub's file (%v)", file.Path, err)
		}
	}
	if cached.Metadata.Runtime == nil || cached.Metadata.Runtime.Setup.Check != "python3 scripts/check.py" {
		t.Fatalf("runtime block = %+v", cached.Metadata.Runtime)
	}
	if !strings.Contains(string(cached.Metadata.Detail), `"skill_id":"bundle-skill"`) || strings.Contains(string(cached.Metadata.Detail), "# Bundle Skill") {
		t.Fatalf("detail should name the skill and leave SKILL.md to the files: %s", cached.Metadata.Detail)
	}

	notModified, err := service.SkillBundle(t.Context(), root, "bundle-skill", []string{strings.Repeat("0", 64), bundle.Digest})
	if err != nil || !notModified.NotModified || len(notModified.Archive) != 0 || notModified.Digest != bundle.Digest {
		t.Fatalf("bundle for a held version = %+v, %v", notModified, err)
	}
}

func TestSkillBundleRefusesWhatLocalDeliveryRefuses(t *testing.T) {
	t.Parallel()
	root := newBundleWorkspace(t)
	service := DistributionService{}

	for _, id := range []string{"missing-skill", systemskills.CuratorSkillID, "../escape"} {
		if _, err := service.SkillBundle(t.Context(), root, id, nil); !errors.Is(err, skill.ErrNotFound) {
			t.Fatalf("bundle %q error = %v, want not found", id, err)
		}
	}

	updateSkillMeta(t, root, "bundle-skill", markThirdParty)
	rebuildBundleCatalog(t, root)
	_, err := service.SkillBundle(t.Context(), root, "bundle-skill", nil)
	var review *ContentReviewRequiredError
	if !errors.As(err, &review) || !errors.Is(err, ErrContentReviewRequired) || len(review.ReasonCodes) == 0 {
		t.Fatalf("bundle of an unapproved skill error = %v", err)
	}
	local, err := fakeSnapshotService().Ensure(t.Context(), root, "bundle-skill")
	if err != nil || local.Status != LocalStatusReviewRequired {
		t.Fatalf("local snapshot = %+v, %v", local, err)
	}
	if strings.Join(local.ReasonCodes, ",") != strings.Join(review.ReasonCodes, ",") {
		t.Fatalf("reasons differ: local %v, bundle %v", local.ReasonCodes, review.ReasonCodes)
	}
	// A held digest never bypasses the trust check.
	entries, _, _ := service.LookupSkills(t.Context(), root, []string{"bundle-skill"})
	held := strings.TrimPrefix(entries["bundle-skill"].Version, "sha256:")
	if _, err := service.SkillBundle(t.Context(), root, "bundle-skill", []string{held}); !errors.Is(err, ErrContentReviewRequired) {
		t.Fatalf("held digest of an unapproved skill error = %v", err)
	}
	status, err := (SnapshotService{}).RemoteStatus(t.Context(), root, "bundle-skill")
	if err != nil || status.Status != LocalStatusReviewRequired || status.Path != "" || status.ReviewCommand != "skillhub skill review bundle-skill" {
		t.Fatalf("remote status = %+v, %v", status, err)
	}

	updateSkillMeta(t, root, "bundle-skill", func(document map[string]any) { document["status"] = "draft" })
	rebuildBundleCatalog(t, root)
	if _, err := service.SkillBundle(t.Context(), root, "bundle-skill", nil); !errors.Is(err, skill.ErrNotFound) {
		t.Fatalf("bundle of a draft error = %v", err)
	}
}

func TestRemoteStatusCarriesNoPath(t *testing.T) {
	t.Parallel()
	root := newBundleWorkspace(t)
	status, err := (SnapshotService{}).RemoteStatus(t.Context(), root, "bundle-skill")
	if err != nil || status.Status != LocalStatusReady || !strings.HasPrefix(status.ManifestVersion, "sha256:") {
		t.Fatalf("remote status = %+v, %v", status, err)
	}
	if status.Path != "" || status.StateDirectory != "" || status.Env != nil || status.Resources != nil || status.Preflight != nil {
		t.Fatalf("remote status exposes hub paths: %+v", status)
	}
	if _, err := os.Stat(filepath.Join(root, "runtime", "cache", "skills")); err == nil {
		entries, _ := os.ReadDir(filepath.Join(root, "runtime", "cache", "skills"))
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "bundle-skill@") {
				t.Fatal("remote status exported a snapshot on the hub")
			}
		}
	}
}

func TestClientLocalSkillPointsAtTheClientCopy(t *testing.T) {
	t.Parallel()
	root := newBundleWorkspace(t)
	bundle, err := (DistributionService{}).SkillBundle(t.Context(), root, "bundle-skill", nil)
	if err != nil {
		t.Fatal(err)
	}
	client := t.TempDir()
	cache := skillruntime.RemoteCache{Root: filepath.Join(client, "remote")}
	cached, err := cache.Install("hub-test", "bundle-skill", bundle.Digest, bytes.NewReader(bundle.Archive))
	if err != nil {
		t.Fatal(err)
	}
	stateRoot := cache.StateRoot("hub-test")
	local, err := ClientLocalSkill(stateRoot, cached.Directory, cached.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	if local.Status != LocalStatusReady || local.Path != cached.Directory || local.ManifestVersion != "sha256:"+bundle.Digest {
		t.Fatalf("local = %+v", local)
	}
	if local.Env[skillruntime.EnvSkillDir] != cached.Directory || !strings.HasPrefix(local.Env[skillruntime.EnvStateDir], stateRoot) || !strings.HasPrefix(local.Env[skillruntime.EnvConfigDir], stateRoot) {
		t.Fatalf("env = %v", local.Env)
	}
	for _, value := range append([]string{local.Path, local.StateDirectory}, local.Env[skillruntime.EnvConfigDir]) {
		if strings.HasPrefix(value, root) {
			t.Fatalf("%s points into the hub workspace", value)
		}
	}
	if local.Preflight == nil || local.Preflight.Check != "python3 scripts/check.py" || local.Preflight.WorkingDirectory != local.StateDirectory {
		t.Fatalf("preflight = %+v", local.Preflight)
	}
	if data, err := os.ReadFile(filepath.Join(local.StateDirectory, "requirements.txt")); err != nil || string(data) != "requests==2.0\n" {
		t.Fatalf("dependency manifest in the state directory = %q, %v", data, err)
	}
	for _, resource := range local.Resources {
		if _, err := os.Stat(resource.LocalPath); err != nil || !strings.HasPrefix(resource.LocalPath, cached.Directory) {
			t.Fatalf("resource %+v: %v", resource, err)
		}
	}
	// A second call reuses the same state directory.
	again, err := ClientLocalSkill(stateRoot, cached.Directory, cached.Metadata)
	if err != nil || again.StateDirectory != local.StateDirectory {
		t.Fatalf("second call = %+v, %v", again, err)
	}
}
