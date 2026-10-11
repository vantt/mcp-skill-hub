package hostintegration

import (
	"fmt"
	"slices"
	"strings"
)

// claudeCLIRules are the Claude Code permission rules connect writes. Every
// skillhub command is allowed, but a command that changes the hub, the agent
// connections, the token or the cache asks first. The ask rules for those
// commands match the name anywhere after "skillhub ", so the order of flags
// does not matter; one that matches too much (a skill id containing
// "connect") only adds a prompt.
var claudeCLIRules = []struct {
	key    string
	values []string
}{
	{"allow", []string{"Bash(skillhub:*)"}},
	{"ask", []string{
		"Bash(skillhub * --yes*)", "Bash(skillhub * confirm *)",
		"Bash(skillhub *init*)", "Bash(skillhub *connect*)", "Bash(skillhub *--fix*)",
		"Bash(skillhub *token rotate*)", "Bash(skillhub *cache clear*)", "Bash(skillhub *serve*)",
	}},
	{"deny", []string{"Bash(skillhub * --approve-content*)"}},
}

func claudePermissionsPreview(raw, desired []byte) string {
	var out strings.Builder
	out.WriteString("managed permissions:")
	for _, key := range []string{"additionalDirectories", "allow", "ask", "deny"} {
		before, _ := jsonStringArrayAt(raw, []string{"permissions", key})
		after, _ := jsonStringArrayAt(desired, []string{"permissions", key})
		for _, value := range after {
			if !slices.Contains(before, value) {
				fmt.Fprintf(&out, "\npermissions.%s: add %s", key, value)
			}
		}
	}
	return boundPreview(out.String())
}
