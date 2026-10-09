package app

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	"gopkg.in/yaml.v3"
)

func TestSkillRuntimeStatusApp(t *testing.T) {
	t.Parallel()

	// Helper to create an active skill in a new workspace
	updateAnySkillMeta := func(t *testing.T, root, id string, mutate func(map[string]any)) {
		t.Helper()
		_, skillRelDir, skillMetaBytes, err := locateSkillDir(root, id)
		if err != nil {
			t.Fatal(err)
		}
		var document map[string]any
		if err := yaml.Unmarshal(skillMetaBytes, &document); err != nil {
			t.Fatal(err)
		}
		mutate(document)
		encoded, err := yaml.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		metaPath := filepath.Join(root, filepath.FromSlash(skillRelDir), ".meta", "skill.yaml")
		if _, err := os.Stat(metaPath); os.IsNotExist(err) {
			metaPath = filepath.Join(root, filepath.FromSlash(skillRelDir), "skill.meta.yaml")
		}
		if err := os.WriteFile(metaPath, encoded, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Helper to create an active skill in a new workspace
	setupTest := func(t *testing.T) (string, string) {
		t.Helper()
		root := newSkillWorkspace(t)
		service := SkillService{}
		createAndActivateSkill(t, service, root)
		return root, "consumer-review"
	}

	t.Run("local_skill_without_runtime", func(t *testing.T) {
		root, id := setupTest(t)
		service := SkillService{}
		status, err := service.RuntimeStatus(t.Context(), root, id)
		if err != nil {
			t.Fatalf("RuntimeStatus failed: %v", err)
		}
		if status.Runtime != nil {
			t.Fatalf("expected nil runtime, got: %+v", status.Runtime)
		}
		if !status.Served {
			t.Fatalf("expected served=true, got false")
		}
		if status.Setup != nil {
			t.Fatalf("expected nil setup for local skill without runtime, got: %+v", status.Setup)
		}
		if status.Doctor != nil {
			t.Fatalf("expected nil doctor, got: %+v", status.Doctor)
		}
		if len(status.EnvKeys) != 0 {
			t.Fatalf("expected empty env keys, got: %v", status.EnvKeys)
		}
	})

	t.Run("unapproved_third_party_with_runtime", func(t *testing.T) {
		root, id := setupTest(t)
		updateAnySkillMeta(t, root, id, func(doc map[string]any) {
			markThirdParty(doc)
			withRuntime(doc)
		})
		if _, err := catalog.BuildCatalogGeneration(t.Context(), root, catalog.BuildOptions{}); err != nil {
			t.Fatal(err)
		}

		service := SkillService{}
		status, err := service.RuntimeStatus(t.Context(), root, id)
		if err != nil {
			t.Fatalf("RuntimeStatus failed: %v", err)
		}
		if status.Runtime == nil {
			t.Fatal("expected runtime spec, got nil")
		}
		if !status.Served {
			t.Fatal("expected served=true")
		}
		if status.Setup == nil || status.Setup.State != skillruntime.StateReviewRequired {
			t.Fatalf("expected setup.state == review_required, got: %+v", status.Setup)
		}
		if status.Doctor != nil {
			t.Fatalf("expected nil doctor for unapproved third-party, got: %+v", status.Doctor)
		}
	})

	t.Run("approved_third_party_with_runtime_after_doctor", func(t *testing.T) {
		root, id := setupTest(t)
		updateAnySkillMeta(t, root, id, func(doc map[string]any) {
			markThirdParty(doc)
			doc["runtime"] = map[string]any{
				"requires": map[string]any{
					"platforms": []any{"linux", "darwin", "windows", "freebsd"},
				},
			}
		})
		if _, err := catalog.BuildCatalogGeneration(t.Context(), root, catalog.BuildOptions{}); err != nil {
			t.Fatal(err)
		}

		trust, err := (SkillService{}).ContentTrustFor(t.Context(), root, id)
		if err != nil || trust.ContentDigest == "" {
			t.Fatalf("expected content digest, got err=%v", err)
		}
		updateAnySkillMeta(t, root, id, setReviewedDigest(trust.ContentDigest))
		if _, err := catalog.BuildCatalogGeneration(t.Context(), root, catalog.BuildOptions{}); err != nil {
			t.Fatal(err)
		}

		// Run doctor once to populate cache
		docResult, docErr := (SkillDoctorService{}).Run(t.Context(), root, id)
		if docErr != nil {
			t.Fatalf("doctor run failed: %v", docErr)
		}
		if docResult.State != "ready" {
			t.Fatalf("expected doctor state ready, got: %s", docResult.State)
		}

		service := SkillService{}
		status, err := service.RuntimeStatus(t.Context(), root, id)
		if err != nil {
			t.Fatalf("RuntimeStatus failed: %v", err)
		}
		if status.Doctor == nil {
			t.Fatal("expected doctor result, got nil")
		}
		if status.Doctor.State != "ready" {
			t.Fatalf("expected doctor.state ready, got: %s", status.Doctor.State)
		}
		if status.Doctor.Basis != "terminal" {
			t.Fatalf("expected doctor.basis terminal, got: %s", status.Doctor.Basis)
		}
		if status.Setup == nil || status.Setup.Basis != skillruntime.BasisTerminal {
			t.Fatalf("expected setup.basis terminal, got: %+v", status.Setup)
		}
	})

	t.Run("draft_skill", func(t *testing.T) {
		root := newSkillWorkspace(t)
		skillDir := filepath.Join(root, "skills", "core", "draft-skill")
		if err := os.MkdirAll(skillDir, 0o755); err != nil {
			t.Fatal(err)
		}
		meta := `schema_version: 1
id: draft-skill
name: Draft Skill
status: draft
description: A draft skill description.
routing:
  triggers: [draft]
`
		if err := os.WriteFile(filepath.Join(skillDir, "skill.meta.yaml"), []byte(meta), 0o644); err != nil {
			t.Fatal(err)
		}
		skillMD := "---\nname: draft-skill\ndescription: A draft skill description.\n---\n# Draft Skill\n"
		if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
			t.Fatal(err)
		}
		service := SkillService{}

		status, err := service.RuntimeStatus(t.Context(), root, "draft-skill")
		if err != nil {
			t.Fatalf("RuntimeStatus on draft skill failed: %v", err)
		}
		if status.Served {
			t.Fatal("expected served=false for draft skill")
		}
		if status.Setup != nil {
			t.Fatalf("expected nil setup for unserved draft skill, got: %+v", status.Setup)
		}
	})

	t.Run("stored_key_and_sentinel_leak_prevention", func(t *testing.T) {
		root, id := setupTest(t)
		const sentinel = "SENTINEL-SECRET-ENV-VALUE-998877"
		envService := SkillEnvService{}
		if _, err := envService.Set(t.Context(), root, id, "RUNTIME_TEST_TOKEN", sentinel); err != nil {
			t.Fatal(err)
		}

		service := SkillService{}
		status, err := service.RuntimeStatus(t.Context(), root, id)
		if err != nil {
			t.Fatalf("RuntimeStatus failed: %v", err)
		}
		if !reflect.DeepEqual(status.EnvKeys, []string{"RUNTIME_TEST_TOKEN"}) {
			t.Fatalf("expected [RUNTIME_TEST_TOKEN], got: %v", status.EnvKeys)
		}

		encoded, err := json.Marshal(status)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), sentinel) {
			t.Fatalf("JSON response leaked stored env value: %s", string(encoded))
		}
	})

	t.Run("read_only_integrity", func(t *testing.T) {
		root, id := setupTest(t)

		// Open and close catalog handle once to warm catalog/pins
		if h, err := catalog.OpenCurrentLocked(t.Context(), root); err == nil {
			_ = h.Close()
		}
		listRuntimePaths := func() map[string]bool {
			paths := map[string]bool{}
			runtimeDir := filepath.Join(root, "runtime")
			_ = filepath.WalkDir(runtimeDir, func(path string, d fs.DirEntry, err error) error {
				if err == nil {
					rel, _ := filepath.Rel(runtimeDir, path)
					paths[rel] = true
				}
				return nil
			})
			return paths
		}

		before := listRuntimePaths()

		service := SkillService{}
		status, err := service.RuntimeStatus(t.Context(), root, id)
		if err != nil {
			t.Fatalf("RuntimeStatus failed: %v", err)
		}
		_ = status

		after := listRuntimePaths()

		for k := range after {
			if !before[k] {
				t.Logf("NEW PATH IN AFTER: %s", k)
			}
		}
		if !reflect.DeepEqual(before, after) {
			t.Fatalf("RuntimeStatus created new runtime paths:\nbefore=%v\nafter=%v", before, after)
		}

		// Verify no snapshot, state dir, or config dir created
		for _, check := range []string{
			filepath.Join(root, "runtime", "cache", "skills", id),
			filepath.Join(root, "runtime", "envs", id),
			filepath.Join(root, "runtime", "config", id),
		} {
			if _, err := os.Stat(check); !os.IsNotExist(err) {
				t.Fatalf("RuntimeStatus created prohibited runtime path: %s", check)
			}
		}
	})

	t.Run("malformed_env_file", func(t *testing.T) {
		root, id := setupTest(t)
		configDir := filepath.Join(root, "runtime", "config", id)
		if err := os.MkdirAll(configDir, 0o700); err != nil {
			t.Fatal(err)
		}
		// Write malformed env file
		if err := os.WriteFile(filepath.Join(configDir, "env"), []byte("INVALID LINE WITHOUT EQUALS\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		service := SkillService{}
		status, err := service.RuntimeStatus(t.Context(), root, id)
		if err != nil {
			t.Fatalf("RuntimeStatus should not fail on malformed env file: %v", err)
		}
		if len(status.EnvKeys) != 0 {
			t.Fatalf("expected empty env keys, got: %v", status.EnvKeys)
		}
		foundWarning := false
		for _, w := range status.Warnings {
			if w.Code == "env_file_unusable" {
				foundWarning = true
			}
		}
		if !foundWarning {
			t.Fatalf("expected env_file_unusable warning, got: %v", status.Warnings)
		}
	})
}
