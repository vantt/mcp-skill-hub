package main

import (
	"os"
	"path/filepath"
	"testing"
)

func makeHub(t *testing.T) string {
	t.Helper()
	hub := t.TempDir()
	if err := os.MkdirAll(filepath.Join(hub, ".skillhub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(hub, ".skillhub", "schema-version"), []byte("4\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return hub
}

func TestGuardRefusesAnOutputInsideAHub(t *testing.T) {
	hub := makeHub(t)
	nested := filepath.Join(hub, "skills", "x", "out.json")
	for _, out := range []string{filepath.Join(hub, "out.json"), nested} {
		if err := checkOutputPath(out, "in.yaml"); err == nil {
			t.Errorf("expected a refusal for %s", out)
		}
	}
}

func TestGuardRefusesTheInputFileAsOutput(t *testing.T) {
	in := filepath.Join(t.TempDir(), "distill.yaml")
	if err := os.WriteFile(in, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := checkOutputPath(in, in); err == nil {
		t.Error("expected a refusal when --out equals --in")
	}
}

func TestGuardRefusesASymlinkThatPointsIntoAHub(t *testing.T) {
	hub := makeHub(t)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(hub, link); err != nil {
		t.Skip("symlinks not available")
	}
	if err := checkOutputPath(filepath.Join(link, "out.json"), "in.yaml"); err == nil {
		t.Error("expected a refusal through a symlink")
	}
}

func TestGuardAllowsAScratchFolder(t *testing.T) {
	if err := checkOutputPath(filepath.Join(t.TempDir(), "out.json"), "in.yaml"); err != nil {
		t.Errorf("unexpected refusal: %v", err)
	}
}
