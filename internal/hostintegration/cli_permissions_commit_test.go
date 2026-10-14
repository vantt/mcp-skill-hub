package hostintegration

import "testing"

// An agent has no terminal, so a change goes through two steps: a preview that
// runs freely, then the same command with --yes, which the person approves. The
// rules must keep that shape for the commands that bring changes into the hub.
func TestClaudeRulesKeepTheTwoStepShapeForCommitsAndUpdates(t *testing.T) {
	t.Parallel()
	ask, allow, deny := ruleValues("ask"), ruleValues("allow"), ruleValues("deny")
	for _, command := range []string{
		"skillhub skill commit x --yes",
		"skillhub skill commit ./my-skill --yes",
		"skillhub skill update x --yes",
		"skillhub skill update x --yes --no-commit",
		"skillhub skill add https://github.com/owner/repo --yes",
	} {
		if !matchesAny(ask, command) {
			t.Errorf("%q matches no ask rule, so an agent could apply it without a prompt", command)
		}
	}
	for _, command := range []string{
		"skillhub skill commit x",
		"skillhub skill commit x --diff",
		"skillhub skill update x",
		"skillhub skill update x --diff",
		"skillhub skill activate x --diff",
	} {
		if matchesAny(ask, command) || !matchesAny(allow, command) {
			t.Errorf("%q is a preview and must run without a prompt", command)
		}
	}
	for _, command := range []string{
		"skillhub skill commit x --approve-content sha256:abc",
		"skillhub skill commit x --yes --approve-content sha256:abc",
		"skillhub skill add https://github.com/owner/repo --approve-content sha256:abc",
		"skillhub skill edit x --approve-content sha256:abc",
	} {
		if !matchesAny(deny, command) {
			t.Errorf("%q matches no deny rule, so an agent could approve third-party content", command)
		}
	}
}
