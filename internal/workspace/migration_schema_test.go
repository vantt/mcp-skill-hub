package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyNeverSilentlyMigratesExistingCanonicalVersion(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Apply(root); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(root, ".skillhub", "schema-version")
	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if _, err := Apply(root); err == nil {
		t.Fatal("Apply silently migrated a legacy workspace")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("Apply recreated canonical marker: %v", err)
	}
}

func TestInspectMarksCanonicalVersionIncompatibleAndNotMechanicallyFixable(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Apply(root); err != nil {
		t.Fatal(err)
	}
	for name, marker := range map[string][]byte{"legacy_missing": nil, "legacy_v1": []byte("1\n"), "legacy_v2": []byte("2\n"), "malformed": []byte("future\n"), "future": []byte("4\n")} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(root, ".skillhub", "schema-version")
			if marker == nil {
				if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(path, marker, 0o644); err != nil {
				t.Fatal(err)
			}
			plan, err := Inspect(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(plan.Findings) != 1 || plan.Findings[0].ID != "canonical_schema_incompatible" || plan.Findings[0].Fixable {
				t.Fatalf("findings = %#v", plan.Findings)
			}
		})
	}
}

func TestInspectTreatsMissingMarkerWithCanonicalContentAsIncompatibleWithoutControlDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if _, err := Apply(root); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(root, ".skillhub")); err != nil {
		t.Fatal(err)
	}
	plan, err := Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Findings) != 1 || plan.Findings[0].ID != "schema_version_missing" || !plan.Findings[0].Fixable {
		t.Fatalf("empty workspace findings = %#v", plan.Findings)
	}
	if err := os.WriteFile(filepath.Join(root, "sources", "catalog", "SRC-CLONE.yaml"), []byte("id: SRC-CLONE\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan, err = Inspect(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Findings) != 1 || plan.Findings[0].ID != "canonical_schema_incompatible" || plan.Findings[0].Fixable {
		t.Fatalf("workspace with content findings = %#v", plan.Findings)
	}
}

func TestReadCanonicalFileRejectsSymlinkAndOversizedInput(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "ok"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadCanonicalFile(root, "ok"); err != nil || string(got) != "1\n" {
		t.Fatalf("regular file = %q, %v", got, err)
	}
	if err := os.Symlink(filepath.Join(root, "ok"), filepath.Join(root, "link")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := ReadCanonicalFile(root, "link"); err == nil {
		t.Fatal("symlink was read")
	}
	truncateTestFile(t, filepath.Join(root, "big"), MaxCanonicalFileBytesV1+1)
	if _, err := ReadCanonicalFile(root, "big"); err == nil {
		t.Fatal("oversized file was read")
	}
}
