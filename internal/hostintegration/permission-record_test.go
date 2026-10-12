package hostintegration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

func connectClaude(t *testing.T, workspace, root string, remove bool) {
	t.Helper()
	if remove {
		workspace = ""
	}
	planAndApply(t, Request{Workspace: workspace, Root: root, Scope: ScopeProject, Binary: absoluteBinary(t), Hosts: []Host{HostClaude}, Remove: remove})
}

func TestRemoveKeepsAManagedRuleTheUserHadBeforeConnecting(t *testing.T) {
	t.Setenv("SKILLHUB_USER_CONFIG_DIR", t.TempDir())
	workspace, root := t.TempDir(), t.TempDir()
	settings := filepath.Join(root, ".claude", "settings.local.json")
	writeTestFile(t, settings, []byte(`{"permissions":{"allow":["Bash(skillhub:*)","Bash(ls)"]}}`), 0o600)
	connectClaude(t, workspace, root, false)
	if got := jsonStrings(t, settings, "permissions", "ask"); !slices.Equal(got, ruleValues("ask")) {
		t.Fatalf("ask after connect = %v", got)
	}
	connectClaude(t, workspace, root, true)
	if got := jsonStrings(t, settings, "permissions", "allow"); !slices.Equal(got, []string{"Bash(skillhub:*)", "Bash(ls)"}) {
		t.Fatalf("allow after removal = %v", got)
	}
	for _, key := range []string{"ask", "deny"} {
		if got := jsonStrings(t, settings, "permissions", key); len(got) != 0 {
			t.Fatalf("%s rules connect added stayed: %v", key, got)
		}
	}
}

func TestRemoveTakesOutEveryRuleConnectAddedAndForgetsTheRecord(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SKILLHUB_USER_CONFIG_DIR", dir)
	workspace, root := t.TempDir(), t.TempDir()
	connectClaude(t, workspace, root, false)
	connectClaude(t, workspace, root, false)
	connectClaude(t, workspace, root, true)
	if got := fileTree(t, root); len(got) != 0 {
		t.Fatalf("files left after removal: %v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, permissionRecordFile)); !os.IsNotExist(err) {
		t.Fatalf("record stayed after removal: %v", err)
	}
}

func TestRemoveWithoutARecordTakesOutEveryManagedRule(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SKILLHUB_USER_CONFIG_DIR", dir)
	workspace, root := t.TempDir(), t.TempDir()
	connectClaude(t, workspace, root, false)
	if err := os.Remove(filepath.Join(dir, permissionRecordFile)); err != nil {
		t.Fatal(err)
	}
	connectClaude(t, workspace, root, true)
	if got := fileTree(t, root); len(got) != 0 {
		t.Fatalf("managed rules stayed without a record: %v", got)
	}
}

func TestRecordIsPrivateAndKeptOutOfTheSettingsFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SKILLHUB_USER_CONFIG_DIR", dir)
	workspace, root := t.TempDir(), t.TempDir()
	connectClaude(t, workspace, root, false)
	record := filepath.Join(dir, permissionRecordFile)
	if runtime.GOOS != "windows" {
		assertMode(t, record, 0o600)
	}
	settings := filepath.Join(root, ".claude", "settings.local.json")
	if got := string(readTestFile(t, settings)); strings.Contains(got, "permission-records") {
		t.Fatalf("settings file mentions the record: %s", got)
	}
}

func TestReapplyingAfterARuleWentMissingRecordsAndRemovesIt(t *testing.T) {
	t.Setenv("SKILLHUB_USER_CONFIG_DIR", t.TempDir())
	workspace, root := t.TempDir(), t.TempDir()
	settings := filepath.Join(root, ".claude", "settings.local.json")
	connectClaude(t, workspace, root, false)
	asks := ruleValues("ask")
	trimmed, err := json.Marshal(map[string]any{"permissions": map[string]any{
		"allow": ruleValues("allow"), "ask": asks[1:], "deny": ruleValues("deny"),
	}})
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, settings, trimmed, 0o600)
	connectClaude(t, workspace, root, false)
	if got := jsonStrings(t, settings, "permissions", "ask"); !slices.Contains(got, asks[0]) {
		t.Fatalf("ask rule not re-added: %v", got)
	}
	connectClaude(t, workspace, root, true)
	if got := fileTree(t, root); len(got) != 0 {
		t.Fatalf("files left after removal: %v", got)
	}
}

func TestRecordUpdateWaitsForAnotherProcessHoldingTheLockFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SKILLHUB_USER_CONFIG_DIR", dir)
	other := flock.New(filepath.Join(dir, permissionRecordLockFile))
	if locked, err := other.TryLock(); err != nil || !locked {
		t.Fatalf("take the lock file: %v %v", locked, err)
	}
	done := make(chan error, 1)
	go func() {
		done <- setPermissionRecord(ScopeProject, filepath.Join(dir, "settings.json"), map[string][]string{"allow": {"Bash(skillhub:*)"}})
	}()
	select {
	case err := <-done:
		t.Fatalf("update finished while another process held the lock: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	if err := other.Unlock(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("update did not finish after the lock file was released")
	}
	if _, err := os.Stat(filepath.Join(dir, permissionRecordFile)); err != nil {
		t.Fatalf("record was not written: %v", err)
	}
}
