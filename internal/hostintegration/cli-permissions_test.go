package hostintegration

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestClaudeCLIPermissionsPreserveUserRulesAndAreIdempotent(t *testing.T) {
	t.Parallel()
	for _, scope := range []Scope{ScopeProject, ScopeUser} {
		t.Run(string(scope), func(t *testing.T) {
			workspace, root := t.TempDir(), t.TempDir()
			request := Request{Workspace: workspace, Root: root, Scope: scope, Binary: filepath.Join(workspace, "skillhub"), Hosts: []Host{HostClaude}}
			path := filepath.Join(root, adapters[0].permissionsPath(scope))
			writeTestFile(t, path, []byte(`{"model":"keep","permissions":{"allow":["Bash(ls)","Bash(skillhub:*)"],"ask":["Bash(user)"],"deny":["Bash(user-deny)"]}}`), 0o600)
			applyAll(t, request)
			for key, want := range map[string][]string{
				"allow": {"Bash(ls)", "Bash(skillhub:*)"},
				"ask":   {"Bash(user)", "Bash(skillhub * --yes*)", "Bash(skillhub * confirm *)"},
				"deny":  {"Bash(user-deny)", "Bash(skillhub * --approve-content*)"},
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
		})
	}
}
