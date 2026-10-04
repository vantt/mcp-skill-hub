package skillruntime

import (
	"strings"
	"testing"
)

func TestIsDependencyManifest(t *testing.T) {
	for _, name := range []string{
		"package.json", "package-lock.json", "npm-shrinkwrap.json", "pnpm-lock.yaml", "yarn.lock",
		"requirements.txt", "requirements-dev.txt", "pyproject.toml", "uv.lock", "poetry.lock",
		"Pipfile", "Pipfile.lock", "go.mod", "go.sum", "Gemfile", "Gemfile.lock", "Cargo.toml", "Cargo.lock",
		"scripts/package.json", "tools/py/requirements.txt",
	} {
		if !IsDependencyManifest(name) {
			t.Errorf("%q should be a dependency manifest", name)
		}
	}
	for _, name := range []string{"SKILL.md", "scripts/run.py", "requirements.md", "my-package.json", "notes/go.mod.bak", "Makefile"} {
		if IsDependencyManifest(name) {
			t.Errorf("%q should not be a dependency manifest", name)
		}
	}
}

func TestDepsKeyIgnoresContentEditsButTracksDependencies(t *testing.T) {
	base := []ResourceDigest{
		{Path: "SKILL.md", Digest: "sha256:1"},
		{Path: "package-lock.json", Digest: "sha256:lock"},
		{Path: "scripts/requirements.txt", Digest: "sha256:req"},
	}
	spec := Spec{Setup: Setup{Command: "npm ci"}}
	key := DepsKey(base, spec, true)
	if len(key) != 16 || strings.Trim(key, "0123456789abcdef") != "" {
		t.Fatalf("key %q is not 16 lowercase hex characters", key)
	}
	reordered := []ResourceDigest{base[2], base[0], base[1]}
	if DepsKey(reordered, spec, true) != key {
		t.Fatal("key depends on input order")
	}
	edited := []ResourceDigest{{Path: "SKILL.md", Digest: "sha256:edited"}, base[1], base[2], {Path: "references/x.md", Digest: "sha256:x"}, {Path: "skill.meta.yaml", Digest: "sha256:m"}}
	if DepsKey(edited, spec, true) != key {
		t.Fatal("non-dependency edits changed the key")
	}
	lockEdited := []ResourceDigest{base[0], {Path: "package-lock.json", Digest: "sha256:lock2"}, base[2]}
	if DepsKey(lockEdited, spec, true) == key {
		t.Fatal("a lock file edit did not change the key")
	}
	if DepsKey(base, Spec{Setup: Setup{Command: "npm install"}}, true) == key {
		t.Fatal("a runtime change did not change the key")
	}
	if DepsKey(base, spec, false) == key {
		t.Fatal("removing the runtime block did not change the key")
	}
}

func TestStateDirName(t *testing.T) {
	name, err := StateDirName("my-skill", "0123456789abcdef")
	if err != nil || name != "my-skill@0123456789abcdef" {
		t.Fatalf("name = %q, err = %v", name, err)
	}
	for _, args := range [][2]string{{"../x", "0123456789abcdef"}, {"My Skill", "0123456789abcdef"}, {"ok", "short"}, {"ok", "0123456789ABCDEF"}, {"ok", "../../etc/passwd"}} {
		if _, err := StateDirName(args[0], args[1]); err == nil {
			t.Errorf("StateDirName(%q, %q) accepted", args[0], args[1])
		}
	}
}
