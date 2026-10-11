package hostintegration

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
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
				"ask":   append([]string{"Bash(user)"}, ruleValues("ask")...),
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

func ruleValues(key string) []string {
	for _, rule := range claudeCLIRules {
		if rule.key == key {
			return rule.values
		}
	}
	return nil
}

// claudeRuleMatches applies a Bash(...) rule to a command line the way Claude
// Code matches it: "name:*" is a prefix rule, and otherwise "*" matches any
// run of characters, including none, and the rule must match the whole line.
func claudeRuleMatches(rule, command string) bool {
	pattern := strings.TrimSuffix(strings.TrimPrefix(rule, "Bash("), ")")
	if prefix, ok := strings.CutSuffix(pattern, ":*"); ok {
		return command == prefix || strings.HasPrefix(command, prefix+" ")
	}
	parts := strings.Split(pattern, "*")
	for index, part := range parts {
		parts[index] = regexp.QuoteMeta(part)
	}
	return regexp.MustCompile("^" + strings.Join(parts, ".*") + "$").MatchString(command)
}

func matchesAny(rules []string, command string) bool {
	return slices.ContainsFunc(rules, func(rule string) bool { return claudeRuleMatches(rule, command) })
}

// TestClaudeAskRulesMatchCommandsInAnyFlagOrder keeps a prompt in front of every
// command that changes the hub, a connection, the token or the cache, whatever
// order its flags come in, while read-only commands run without one.
func TestClaudeAskRulesMatchCommandsInAnyFlagOrder(t *testing.T) {
	t.Parallel()
	ask, allow := ruleValues("ask"), ruleValues("allow")
	for _, command := range []string{
		"skillhub doctor --fix",
		"skillhub doctor --json --fix",
		"skillhub doctor --fix --json",
		"skillhub connect --dry-run",
		"skillhub connect -g",
		"skillhub connect --hub http://h:7420 --token-file f",
		"skillhub init",
		"skillhub init https://x/y.git",
		"skillhub token rotate --token-file f",
		"skillhub cache clear --hub http://h:7420",
		"skillhub serve",
		"skillhub serve web --addr 127.0.0.1:7421",
		"skillhub skill activate my-skill --yes",
		"skillhub skill confirm PROP-1",
	} {
		if !matchesAny(ask, command) {
			t.Errorf("%q matches no ask rule", command)
		}
	}
	for _, command := range []string{"skillhub status", "skillhub skill list", "skillhub doctor", "skillhub token show"} {
		if matchesAny(ask, command) {
			t.Errorf("%q matches an ask rule", command)
		}
		if !matchesAny(allow, command) {
			t.Errorf("%q matches no allow rule", command)
		}
	}
}
