package skillruntime

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestMachineID(t *testing.T) {
	id := MachineID()
	if !regexp.MustCompile(`^[0-9a-f]{12}$`).MatchString(id) || id != MachineID() {
		t.Fatalf("machine id %q is not stable 12-hex", id)
	}
}

func TestRuntimeFingerprint(t *testing.T) {
	spec := Spec{Requires: Requires{Bins: []Bin{{Name: "git"}}}}
	fp := RuntimeFingerprint("1", spec)
	if len(fp) != 64 || fp == RuntimeFingerprint("2", spec) || fp == RuntimeFingerprint("1", Spec{}) {
		t.Fatalf("fingerprint %q not sensitive to version and spec", fp)
	}
}

func TestCachePath(t *testing.T) {
	fp := strings.Repeat("ab", 32)
	got, err := CachePath("/ws", "demo-skill", fp)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/ws", "runtime", "cache", "doctor", MachineID(), "demo-skill@"+fp[:16]+".json")
	if got != want {
		t.Fatalf("path = %s, want %s", got, want)
	}
	for _, bad := range [][2]string{{"../x", fp}, {"Demo", fp}, {"demo", "short"}, {"demo", strings.Repeat("G", 64)}} {
		if _, err := CachePath("/ws", bad[0], bad[1]); err == nil {
			t.Errorf("CachePath(%q, %q) accepted", bad[0], bad[1])
		}
	}
}

func TestCacheRoundTrip(t *testing.T) {
	root := t.TempDir()
	fp := RuntimeFingerprint("1", Spec{})
	if _, ok, err := ReadCache(root, "demo", fp); ok || err != nil {
		t.Fatalf("empty cache read = %v, %v", ok, err)
	}
	want := Result{
		State:       StateReady,
		Checks:      []Check{{Kind: KindEnv, Name: "API_TOKEN", Status: StatusPass}},
		CheckedAt:   time.Date(2026, 10, 4, 2, 0, 0, 0, time.UTC),
		CheckOutput: "token=" + secretSentinel,
	}
	if err := WriteCache(root, "demo", fp, want); err != nil {
		t.Fatal(err)
	}
	got, ok, err := ReadCache(root, "demo", fp)
	if err != nil || !ok {
		t.Fatalf("read = %v, %v", ok, err)
	}
	if got.State != want.State || !got.CheckedAt.Equal(want.CheckedAt) || len(got.Checks) != 1 || got.Checks[0] != want.Checks[0] {
		t.Fatalf("round trip = %+v", got)
	}
	if got.CheckOutput != "" {
		t.Fatal("check output was persisted")
	}
	path, _ := CachePath(root, "demo", fp)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), secretSentinel) {
		t.Fatal("cache file contains the secret sentinel")
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(path)
		dirInfo, _ := os.Stat(filepath.Dir(path))
		if info.Mode().Perm() != 0o600 || dirInfo.Mode().Perm() != 0o700 {
			t.Fatalf("modes file %v dir %v", info.Mode().Perm(), dirInfo.Mode().Perm())
		}
	}
	want.State = StateSetupRequired
	if err := WriteCache(root, "demo", fp, want); err != nil {
		t.Fatal(err)
	}
	if got, _, _ := ReadCache(root, "demo", fp); got.State != StateSetupRequired {
		t.Fatal("overwrite did not replace the cached result")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("leftover temp files: %v", entries)
	}
}

func TestCacheRejectsCorruptState(t *testing.T) {
	root := t.TempDir()
	fp := RuntimeFingerprint("1", Spec{})
	if err := WriteCache(root, "demo", fp, Result{State: StateReady}); err != nil {
		t.Fatal(err)
	}
	path, _ := CachePath(root, "demo", fp)
	if err := os.WriteFile(path, []byte(`{"state":"bogus"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := ReadCache(root, "demo", fp); ok || err == nil {
		t.Fatal("unknown state accepted")
	}
}

func TestCacheRefusesSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	fp := RuntimeFingerprint("1", Spec{})

	root := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "runtime"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "runtime", "cache")); err != nil {
		t.Fatal(err)
	}
	if err := WriteCache(root, "demo", fp, Result{State: StateReady}); err == nil {
		t.Fatal("write through symlinked directory accepted")
	}
	if _, _, err := ReadCache(root, "demo", fp); err == nil {
		t.Fatal("read through symlinked directory accepted")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatal("write escaped through the symlink")
	}

	root = t.TempDir()
	if err := WriteCache(root, "demo", fp, Result{State: StateReady}); err != nil {
		t.Fatal(err)
	}
	path, _ := CachePath(root, "demo", fp)
	target := filepath.Join(outside, "target.json")
	if err := os.WriteFile(target, []byte(`{"state":"ready"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ReadCache(root, "demo", fp); err == nil {
		t.Fatal("symlinked cache file accepted on read")
	}
	if err := WriteCache(root, "demo", fp, Result{State: StateReady}); err == nil {
		t.Fatal("symlinked cache file accepted on write")
	}
}

func TestSetupState(t *testing.T) {
	ready := &Result{State: StateReady}
	needsSetup := &Result{State: StateSetupRequired}
	pass := []Check{{Kind: KindPlatform, Name: "linux", Status: StatusPass}}
	platformFail := []Check{{Kind: KindPlatform, Name: "windows", Status: StatusFail}}
	tests := []struct {
		name      string
		untrusted bool
		hasSpec   bool
		platform  []Check
		cached    *Result
		want      string
	}{
		{name: "untrusted wins over everything", untrusted: true, hasSpec: true, platform: platformFail, cached: ready, want: StateReviewRequired},
		{name: "untrusted without runtime block", untrusted: true, want: StateReviewRequired},
		{name: "no spec", platform: platformFail, cached: ready, want: ""},
		{name: "platform fails", hasSpec: true, platform: platformFail, cached: ready, want: StateUnsupportedPlatform},
		{name: "cached setup required", hasSpec: true, platform: pass, cached: needsSetup, want: StateSetupRequired},
		{name: "cached ready", hasSpec: true, platform: pass, cached: ready, want: StateReady},
		{name: "no cache", hasSpec: true, platform: pass, want: StateUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SetupState(tt.untrusted, tt.hasSpec, tt.platform, tt.cached); got != tt.want {
				t.Fatalf("SetupState = %q, want %q", got, tt.want)
			}
		})
	}
}
