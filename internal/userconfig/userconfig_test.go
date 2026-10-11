package userconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestMissingFileIsAnEmptyConfig(t *testing.T) {
	t.Parallel()
	config, err := NewStore(filepath.Join(t.TempDir(), "absent")).Load()
	if err != nil {
		t.Fatal(err)
	}
	if config.Hub != "" || len(config.PreviousHubs) != 0 {
		t.Fatalf("config = %+v, want empty", config)
	}
}

func TestCorruptFileNamesTheFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewStore(dir).Load()
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("Load error = %v, want one naming %s", err, path)
	}
	if err := NewStore(dir).SetHub(filepath.Join(dir, "hub")); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("SetHub error = %v, want one naming %s", err, path)
	}
}

func TestOversizedFileIsRefused(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(`{"hub":"`+strings.Repeat("x", maxFileBytes)+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(dir).Load(); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("Load error = %v, want a size refusal", err)
	}
}

func TestSetHubWritesAPrivateFileAndKeepsUnknownKeys(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(t.TempDir(), "nested", "skillhub")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, FileName), []byte(`{"future":{"a":1},"hub":""}`), 0o600); err != nil {
		t.Fatal(err)
	}
	hub := filepath.Join(t.TempDir(), "hub")
	store := NewStore(dir)
	if err := store.SetHub(hub); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, want 0600", info.Mode().Perm())
	}
	raw, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		t.Fatal(err)
	}
	if string(values["future"]) != `{"a":1}` && !strings.Contains(string(values["future"]), `"a": 1`) {
		t.Fatalf("unknown key not kept: %s", raw)
	}
	config, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if config.Hub != hub {
		t.Fatalf("hub = %q, want %q", config.Hub, hub)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("directory holds %d entries after an atomic write, want only %s", len(entries), FileName)
	}
}

func TestSetHubRotatesPreviousHubs(t *testing.T) {
	t.Parallel()
	store := NewStore(t.TempDir())
	base := t.TempDir()
	hub := func(name string) string { return filepath.Join(base, name) }
	for _, name := range []string{"a", "b", "a", "c", "d", "e", "f", "g", "g"} {
		if err := store.SetHub(hub(name)); err != nil {
			t.Fatal(err)
		}
	}
	config, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if config.Hub != hub("g") {
		t.Fatalf("hub = %q", config.Hub)
	}
	want := []string{hub("f"), hub("e"), hub("d"), hub("c"), hub("a")}
	if !reflect.DeepEqual(config.PreviousHubs, want) {
		t.Fatalf("previous hubs = %v, want %v (newest first, deduplicated, at most 5)", config.PreviousHubs, want)
	}
}

func TestSetHubRefusesARelativePath(t *testing.T) {
	t.Parallel()
	if err := NewStore(t.TempDir()).SetHub("relative/hub"); err == nil {
		t.Fatal("SetHub accepted a relative path")
	}
}

func TestDefaultUsesTheOverrideDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(DirEnv, dir)
	hub := filepath.Join(t.TempDir(), "hub")
	if err := Default().SetHub(hub); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); err != nil {
		t.Fatalf("override directory not used: %v", err)
	}
	config, err := Default().Load()
	if err != nil || config.Hub != hub {
		t.Fatalf("Load = %+v, %v", config, err)
	}
}

func TestDefaultFallsBackToTheUserConfigDirectory(t *testing.T) {
	t.Setenv(DirEnv, "")
	base := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", base)
	t.Setenv("HOME", base)
	t.Setenv("APPDATA", base)
	want := filepath.Join(base, "skillhub", FileName)
	if runtime.GOOS == "darwin" {
		want = filepath.Join(base, "Library", "Application Support", "skillhub", FileName)
	}
	if got := Default().(fileStore).Path(); got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}
