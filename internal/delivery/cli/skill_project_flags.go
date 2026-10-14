package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// singleSkillID takes the one skill id of a command from its positional
// argument or --id.
func singleSkillID(flags *skillFlags, positionals []string, command string) error {
	switch {
	case len(positionals) == 1:
		if flags.id != "" && flags.id != positionals[0] {
			return fmt.Errorf("positional skill ID %q conflicts with --id %q", positionals[0], flags.id)
		}
		flags.id = positionals[0]
	case len(positionals) == 0 && flags.id != "":
	default:
		return fmt.Errorf("%s requires exactly one skill ID", command)
	}
	return nil
}

// hasEditFlags reports flags that only make sense for a command that edits
// a skill in the hub, which create, checkout and commit do not.
func (flags skillFlags) hasEditFlags() bool {
	return flags.name != "" || flags.description != "" || flags.contentFile != "" || flags.rationale != "" ||
		flags.runtimeFile != "" || flags.editor || flags.proposalID != "" || flags.proposalDigest != "" ||
		flags.baseVersion != "" || flags.idempotencyKey != ""
}

func parseSkillCreateFlags(flags *skillFlags, positionals []string) error {
	if len(positionals) > 1 {
		return errors.New("create accepts at most one positional skill ID")
	}
	if len(positionals) == 1 {
		if flags.id != "" && flags.id != positionals[0] {
			return fmt.Errorf("positional skill ID %q conflicts with --id %q", positionals[0], flags.id)
		}
		flags.id = positionals[0]
	}
	switch {
	case flags.collection != "":
		return errors.New("--collection belongs to `skillhub skill commit`: create writes only in your project")
	case flags.contentFile != "" || flags.rationale != "" || flags.runtimeFile != "" || flags.editor:
		return errors.New("create takes --name, --description, --trigger, --not-for and --example; write the rest in the SKILL.md it creates")
	case flags.approveContent != "":
		return errors.New("--approve-content is available only for skill edit and skill commit")
	case flags.proposalID != "" || flags.proposalDigest != "" || flags.baseVersion != "":
		return errors.New("create does not accept confirmation pins")
	case flags.diff || flags.yes || flags.idempotencyKey != "" || flags.minScope != "" || len(flags.operations) > 0 || len(flags.counterExamples) > 0:
		return errors.New("create takes --name, --description, --trigger, --not-for and --example; routing details are set when you commit")
	}
	return nil
}

func parseSkillCheckoutFlags(flags *skillFlags, positionals []string) error {
	if err := singleSkillID(flags, positionals, "checkout"); err != nil {
		return err
	}
	if flags.collection != "" || flags.hasEditFlags() || flags.hasRoutingFlags() || flags.approveContent != "" || flags.yes || flags.diff || flags.editRouting || len(flags.accepts)+len(flags.manuals) > 0 || flags.writeConflicts != "" {
		return errors.New("checkout accepts only its skill ID, --workspace, and --json")
	}
	return nil
}

func parseSkillCommitFlags(flags *skillFlags, positionals []string) error {
	if len(positionals) != 1 || flags.id != "" {
		return errors.New("commit requires exactly one skill id or folder")
	}
	if flags.hasEditFlags() {
		return errors.New("commit takes the skill from its folder; edit the folder, then commit it")
	}
	for _, item := range flags.accepts {
		path, choice, found := strings.Cut(item, "=")
		if !found || path == "" || (choice != "hub" && choice != "project") {
			return fmt.Errorf("invalid --accept value %q: use <path>=hub or <path>=project", item)
		}
	}
	for _, item := range flags.manuals {
		path, file, found := strings.Cut(item, "=")
		if !found || path == "" {
			return fmt.Errorf("invalid --manual flag %q: must be <path>=<file>", item)
		}
		if _, err := os.Stat(strings.TrimSpace(file)); err != nil {
			return fmt.Errorf("manual file %q does not exist", strings.TrimSpace(file))
		}
	}
	if flags.approveContent != "" && !contentDigestPattern.MatchString(flags.approveContent) {
		return errors.New("--approve-content requires a digest of the form sha256:<64 lowercase hex characters>")
	}
	if flags.editRouting && (flags.yes || flags.jsonOutput) {
		return errors.New("--edit-routing needs a terminal and cannot be combined with --yes or --json")
	}
	return nil
}
