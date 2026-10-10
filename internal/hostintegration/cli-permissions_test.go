package hostintegration

import (
	"context"
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
