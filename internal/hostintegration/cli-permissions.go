package hostintegration

import "slices"

// claudeCLIRules are the Claude Code permission rules connect writes. Every
// skillhub command is allowed, but a command that changes the hub, the agent
// connections, the token or the cache asks first. A command name always comes
// right after "skillhub" (no flag goes before it), so each ask rule names it
// there and a skill id or a path that holds such a name ("connect-helper",
// "~/serve-hub") runs without a prompt; flags after it may come in any order.
// Claude Code matches each part of a compound command on its own, and "*"
// matches any text (code.claude.com/docs/en/permissions).
var claudeCLIRules = []struct {
	key    string
	values []string
}{
	{"allow", []string{"Bash(skillhub:*)"}},
	{"ask", []string{
		"Bash(skillhub * --yes*)", "Bash(skillhub * confirm *)",
		"Bash(skillhub init*)", "Bash(skillhub connect*)", "Bash(skillhub doctor *--fix*)",
		"Bash(skillhub token rotate*)", "Bash(skillhub cache clear*)", "Bash(skillhub serve*)",
	}},
	{"deny", []string{"Bash(skillhub * --approve-content*)"}},
}

// retiredClaudeCLIRules are rules an earlier release wrote and the current set
// replaces: these ask rules matched the command name anywhere in the line, so
// a skill id such as "connect-helper" asked too. connect and doctor --fix take
// out the ones connect added, and connect --remove takes them out like the
// current rules.
var retiredClaudeCLIRules = map[string][]string{
	"ask": {
		"Bash(skillhub *init*)", "Bash(skillhub *connect*)", "Bash(skillhub *--fix*)",
		"Bash(skillhub *token rotate*)", "Bash(skillhub *cache clear*)", "Bash(skillhub *serve*)",
	},
}

// withoutRetiredClaudeCLIRules takes out of a settings file the retired rules
// connect added to it: the ones its record names, or all of them when there
// is no record (a connection made before records, whose managed rules are all
// connect's). A retired rule the person had before connecting stays.
func withoutRetiredClaudeCLIRules(raw []byte, settings settingsFile) ([]byte, error) {
	owned, err := managedRulesToRemove(settings.scope, settings.path)
	if err != nil {
		return nil, err
	}
	updated := raw
	for key, retired := range retiredClaudeCLIRules {
		var installed []string
		for _, value := range retired {
			if slices.Contains(owned[key], value) {
				installed = append(installed, value)
			}
		}
		if updated, err = stripJSONStringArray(updated, []string{"permissions", key}, installed, false); err != nil {
			return nil, err
		}
	}
	return updated, nil
}
