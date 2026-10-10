package hostintegration

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestClaudeCLIPermissionsPreserveUserRulesAndTrackOwnedAdditions(t *testing.T) {
	t.Parallel()
	for _, scope := range []Scope{ScopeProject, ScopeUser} {
		t.Run(string(scope), func(t *testing.T) {
			workspace, root := t.TempDir(), t.TempDir()
			request := Request{Workspace: workspace, Root: root, Scope: scope, Binary: filepath.Join(workspace, "skillhub"), Hosts: []Host{HostClaude}}
			path := filepath.Join(root, adapters[0].permissionsPath(scope))
			writeTestFile(t, path, []byte(`{"model":"keep","permissions":{"allow":["Bash(ls)","Bash(skillhub:*)"],"ask":["Bash(user)"]}}`), 0o600)
			applyAll(t, request)
			for key, want := range map[string][]string{
				"allow": {"Bash(ls)", "Bash(skillhub:*)"},
				"ask":   {"Bash(user)", "Bash(skillhub * --yes*)", "Bash(skillhub * confirm *)"},
				"deny":  {"Bash(skillhub * --approve-content*)"},
			} {
				if got := jsonStrings(t, path, "permissions", key); !slices.Equal(got, want) {
					t.Fatalf("%s = %v, want %v", key, got, want)
				}
			}
			before := string(readTestFile(t, path))
			applyAll(t, request)
			if got := string(readTestFile(t, path)); got != before {
				t.Fatal("rerun changed permissions")
			}
			// A user appends another rule after connect; removal must retain it too.
			raw := readTestFile(t, path)
			updated, err := ensureJSONStringArray(raw, []string{"permissions", "deny"}, []string{"Bash(user-deny)"})
			if err != nil {
				t.Fatal(err)
			}
			writeTestFile(t, path, updated, 0o600)
			request.Remove = true
			plan, err := Plan(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if string(readTestFile(t, path)) != string(updated) {
				t.Fatal("removal preview wrote settings")
			}
			if _, err := Apply(context.Background(), plan, ApplyOptions{Confirmed: true}); err != nil {
				t.Fatal(err)
			}
			for key, want := range map[string][]string{"allow": {"Bash(ls)", "Bash(skillhub:*)"}, "ask": {"Bash(user)"}, "deny": {"Bash(user-deny)"}, "additionalDirectories": {}} {
				if got := jsonStrings(t, path, "permissions", key); !slices.Equal(got, want) {
					t.Fatalf("after removal %s = %v, want %v", key, got, want)
				}
			}
			assertContains(t, path, `"model":"keep"`)
			again, err := Plan(context.Background(), request)
			if err != nil || len(again.Changes) != 0 {
				t.Fatalf("second removal = %+v, %v", again, err)
			}
		})
	}
}

func TestClaudePermissionOwnershipScopesDoNotCollide(t *testing.T) {
	t.Parallel()
	workspace, root := t.TempDir(), t.TempDir()
	request := Request{Workspace: workspace, Root: root, Scope: ScopeUser, Binary: filepath.Join(workspace, "skillhub"), Hosts: []Host{HostClaude}}
	applyAll(t, request)
	userReceipt := filepath.Join(root, ".claude", "skillhub-permissions.json")
	before := readTestFile(t, userReceipt)
	nativePath := filepath.Join(root, ".claude", "skills", "system-curator", "SKILL.md")
	nativeBefore := readTestFile(t, nativePath)
	request.Scope = ScopeProject
	applyAll(t, request)
	request.Remove = true
	applyAll(t, request)
	if got := readTestFile(t, userReceipt); !slices.Equal(got, before) {
		t.Fatal("project disconnect changed user ownership")
	}
	userSettings := filepath.Join(root, ".claude", "settings.json")
	if got := jsonStrings(t, userSettings, "permissions", "allow"); !slices.Equal(got, []string{"Bash(skillhub:*)"}) {
		t.Fatalf("project removal changed user rules: %v", got)
	}
	if got := readTestFile(t, nativePath); !slices.Equal(got, nativeBefore) {
		t.Fatal("project disconnect changed shared user curator")
	}
	request.Scope = ScopeUser
	applyAll(t, request)
	if got := jsonStrings(t, userSettings, "permissions", "allow"); len(got) != 0 {
		t.Fatalf("user disconnect lost ownership: %v", got)
	}
}

func TestFailedConnectDoesNotClaimFutureUserPermission(t *testing.T) {
	workspace, root := t.TempDir(), t.TempDir()
	request := Request{Workspace: workspace, Root: root, Binary: filepath.Join(workspace, "skillhub"), Hosts: []Host{HostClaude}}
	plan, err := Plan(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	original := atomicWriteBeforeRename
	t.Cleanup(func() { atomicWriteBeforeRename = original })
	atomicWriteBeforeRename = func(relative string) error {
		if relative == filepath.Join(".claude", "settings.local.json") {
			return errors.New("permission write failed")
		}
		return nil
	}
	if _, err := Apply(context.Background(), plan, ApplyOptions{Confirmed: true}); err == nil {
		t.Fatal("expected injected write failure")
	}
	atomicWriteBeforeRename = original
	receiptPath := filepath.Join(root, ".claude", "skillhub-permissions.local.json")
	if _, err := os.Stat(receiptPath); !os.IsNotExist(err) {
		t.Fatalf("failed connect recorded uncommitted ownership: %v", err)
	}
	settings := claudeLocal(root)
	writeTestFile(t, settings, []byte(`{"permissions":{"ask":["Bash(skillhub * --yes*)"]}}`), 0o600)
	applyAll(t, request)
	request.Remove = true
	applyAll(t, request)
	if got := jsonStrings(t, settings, "permissions", "ask"); !slices.Equal(got, []string{"Bash(skillhub * --yes*)"}) {
		t.Fatalf("disconnect removed user permission after failed connect: %v", got)
	}
}
