package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
)

func listRuntimeDirTree(t *testing.T, root string) []string {
	t.Helper()
	var entries []string
	runtimeDir := filepath.Join(root, "runtime")
	if _, err := os.Stat(runtimeDir); err != nil {
		return entries
	}
	_ = filepath.WalkDir(runtimeDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(runtimeDir, p)
		entries = append(entries, filepath.ToSlash(rel))
		return nil
	})
	slices.Sort(entries)
	return entries
}

func TestRuntimeStatus(t *testing.T) {
	root := newSkillWorkspace(t)
	service := SkillService{}
	ctx := context.Background()

	// 1. Local skill without runtime
	createActiveDistributionSkill(t, root, "local-skill", "Local Skill")
	if _, err := (CatalogService{}).BuildCatalogGeneration(ctx, root); err != nil {
		t.Fatal(err)
	}

	st1, err := service.RuntimeStatus(ctx, root, "local-skill")
	if err != nil {
		t.Fatalf("local-skill status: %v", err)
	}
	if st1.Runtime != nil {
		t.Errorf("expected nil Runtime for local skill, got: %#v", st1.Runtime)
	}
	if !st1.Served {
		t.Errorf("expected served=true for active local skill")
	}
	if st1.Setup != nil {
		t.Errorf("expected nil Setup for local skill without runtime, got: %#v", st1.Setup)
	}
	if st1.Doctor != nil {
		t.Errorf("expected nil Doctor for local skill, got: %#v", st1.Doctor)
	}
	if len(st1.EnvKeys) != 0 {
		t.Errorf("expected empty EnvKeys, got: %v", st1.EnvKeys)
	}
	if st1.DoctorCommand != "skillhub skill doctor local-skill" {
		t.Errorf("unexpected doctor command: %q", st1.DoctorCommand)
	}
	if st1.EnvSetCommand != "skillhub skill env set local-skill <KEY>" {
		t.Errorf("unexpected env set command: %q", st1.EnvSetCommand)
	}

	// 2. Unapproved third-party with runtime
	createActiveDistributionSkill(t, root, "vendor-skill", "Vendor Skill")
	updateSkillMeta(t, root, "vendor-skill", func(doc map[string]any) {
		markThirdParty(doc)
		withRuntime(doc)
	})
	if _, err := (CatalogService{}).BuildCatalogGeneration(ctx, root); err != nil {
		t.Fatal(err)
	}

	st2, err := service.RuntimeStatus(ctx, root, "vendor-skill")
	if err != nil {
		t.Fatalf("vendor-skill status: %v", err)
	}
	if st2.Runtime == nil {
		t.Fatal("expected Runtime spec for vendor-skill")
	}
	if !st2.Served {
		t.Errorf("expected served=true for vendor-skill")
	}
	if st2.Setup == nil || st2.Setup.State != "review_required" {
		t.Fatalf("expected setup state review_required, got: %#v", st2.Setup)
	}
	if st2.Doctor != nil {
		t.Errorf("expected nil Doctor for unapproved third-party skill, got: %#v", st2.Doctor)
	}

	// 3. Approved third-party with runtime after real SkillDoctorService{}.Run
	createActiveDistributionSkill(t, root, "approved-skill", "Approved Skill")
	updateSkillMeta(t, root, "approved-skill", func(doc map[string]any) {
		markThirdParty(doc)
		doc["runtime"] = map[string]any{
			"requires": map[string]any{
				"platforms": []any{"linux", "darwin", "windows", "freebsd"},
			},
		}
	})
	// Calculate trust digest and approve
	trust, err := service.ContentTrustFor(ctx, root, "approved-skill")
	if err != nil {
		t.Fatal(err)
	}
	updateSkillMeta(t, root, "approved-skill", setReviewedDigest(trust.ContentDigest))
	if _, err := (CatalogService{}).BuildCatalogGeneration(ctx, root); err != nil {
		t.Fatal(err)
	}
	// Run doctor to cache result
	docRes, err := (SkillDoctorService{}).Run(ctx, root, "approved-skill")
	if err != nil {
		t.Fatalf("doctor run: %v", err)
	}
	if docRes.State != "ready" {
		t.Fatalf("expected doctor state ready, got: %s", docRes.State)
	}

	st3, err := service.RuntimeStatus(ctx, root, "approved-skill")
	if err != nil {
		t.Fatalf("approved-skill status: %v", err)
	}
	if st3.Doctor == nil || st3.Doctor.State != "ready" || st3.Doctor.Basis != "terminal" {
		t.Fatalf("expected Doctor state=ready basis=terminal, got: %#v", st3.Doctor)
	}
	if st3.Setup == nil || st3.Setup.Basis != "terminal" || st3.Setup.State != "ready" {
		t.Fatalf("expected Setup state=ready basis=terminal, got: %#v", st3.Setup)
	}

	// 4. Draft skill
	createTestSkill(t, root, "draft-skill", "Draft Skill", false)
	st4, err := service.RuntimeStatus(ctx, root, "draft-skill")
	if err != nil {
		t.Fatalf("draft-skill status: %v", err)
	}
	if st4.Served {
		t.Errorf("expected served=false for draft skill")
	}
	if st4.Setup != nil {
		t.Errorf("expected nil Setup for unserved skill, got: %#v", st4.Setup)
	}

	// 5. Stored key via SkillEnvService{}.Set -> env_keys == ["RUNTIME_TEST_TOKEN"] and never leaks value
	const secretVal = "SENTINEL-SECRET-ENV-VALUE-999"
	createActiveDistributionSkill(t, root, "env-skill", "Env Skill")
	if _, err := (SkillEnvService{}).Set(ctx, root, "env-skill", "RUNTIME_TEST_TOKEN", secretVal); err != nil {
		t.Fatalf("env set: %v", err)
	}

	st5, err := service.RuntimeStatus(ctx, root, "env-skill")
	if err != nil {
		t.Fatalf("env-skill status: %v", err)
	}
	if !slices.Equal(st5.EnvKeys, []string{"RUNTIME_TEST_TOKEN"}) {
		t.Fatalf("expected EnvKeys=[RUNTIME_TEST_TOKEN], got: %v", st5.EnvKeys)
	}
	encoded, err := json.Marshal(st5)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(secretVal)) {
		t.Fatalf("RuntimeStatus JSON leaked secret env value: %s", string(encoded))
	}

	// 6. Read-only: recursive listing of runtime/ is identical before and after
	createActiveDistributionSkill(t, root, "fresh-skill", "Fresh Skill")
	if _, err := (CatalogService{}).BuildCatalogGeneration(ctx, root); err != nil {
		t.Fatal(err)
	}
	if h, err := catalog.OpenCurrent(ctx, root); err == nil {
		_ = h.Close()
	}
	beforeTree := listRuntimeDirTree(t, root)
	_, err = service.RuntimeStatus(ctx, root, "fresh-skill")
	if err != nil {
		t.Fatalf("fresh-skill status: %v", err)
	}
	afterTree := listRuntimeDirTree(t, root)
	if !slices.Equal(beforeTree, afterTree) {
		t.Fatalf("RuntimeStatus modified runtime/ directory:\nbefore=%v\nafter=%v", beforeTree, afterTree)
	}
	for _, path := range afterTree {
		if strings.Contains(path, "fresh-skill") {
			t.Fatalf("fresh-skill runtime artifact unexpectedly created: %s", path)
		}
	}
	// 7. Malformed env file -> env_file_unusable warning, no error
	createActiveDistributionSkill(t, root, "bad-env-skill", "Bad Env Skill")
	badEnvDir := filepath.Join(root, "runtime", "config", "bad-env-skill")
	_ = os.MkdirAll(badEnvDir, 0o700)
	_ = os.WriteFile(filepath.Join(badEnvDir, "env"), []byte("BROKEN_SYNTAX_WITHOUT_EQUALS\n"), 0o600)

	st7, err := service.RuntimeStatus(ctx, root, "bad-env-skill")
	if err != nil {
		t.Fatalf("expected no error for malformed env file, got: %v", err)
	}
	if len(st7.EnvKeys) != 0 {
		t.Errorf("expected empty EnvKeys for malformed file, got: %v", st7.EnvKeys)
	}
	w := findWarningByCode(st7.Warnings, "env_file_unusable")
	if w == nil {
		t.Fatalf("expected env_file_unusable warning, got: %+v", st7.Warnings)
	}
}
