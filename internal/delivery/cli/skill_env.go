package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
	"golang.org/x/term"
)

// skillEnvInput is where `skill env set` reads a value from when it is not a
// terminal; tests replace it.
var skillEnvInput io.Reader = os.Stdin

// skillEnvMaxInput bounds the value read from a pipe.
const skillEnvMaxInput = 64 << 10

// parseSkillEnvArgs validates `skill env <set|unset|list> <id> [KEY]`. The
// value is never an argument: it would end up in shell history and process
// listings.
func parseSkillEnvArgs(flags *skillFlags, positionals []string) error {
	if flags.id != "" || flags.collection != "" || flags.name != "" || flags.description != "" || flags.contentFile != "" || flags.hasRoutingFlags() || flags.approveContent != "" || flags.editor || flags.yes || flags.fullDiff || flags.idempotencyKey != "" || flags.proposalID != "" || flags.all {
		return errors.New("env accepts only its action, skill ID, variable name, --workspace, and --json")
	}
	if len(positionals) == 0 {
		return errors.New("env requires an action: set, unset, or list")
	}
	want := 3
	switch positionals[0] {
	case "set", "unset":
	case "list":
		want = 2
	default:
		return fmt.Errorf("unsupported env action %q; use set, unset, or list", positionals[0])
	}
	if len(positionals) != want {
		if want == 3 {
			return fmt.Errorf("env %s requires a skill ID and a variable name; the value is read from the terminal or stdin, never from arguments", positionals[0])
		}
		return errors.New("env list requires exactly one skill ID")
	}
	flags.id = positionals[1]
	return nil
}

// runSkillEnv manages a skill's stored secret environment. Output carries key
// names only.
func runSkillEnv(ctx context.Context, flags skillFlags, stdout, stderr io.Writer) int {
	service := app.SkillEnvService{}
	action := flags.positionals[0]
	var (
		result app.SkillEnvResult
		err    error
	)
	switch action {
	case "set":
		value, readErr := readSkillEnvValue(flags.positionals[2], stderr)
		if readErr != nil {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, readErr.Error(), "Pipe the value on stdin or run the command in a terminal.")
		}
		result, err = service.Set(ctx, flags.workspace, flags.id, flags.positionals[2], value)
	case "unset":
		result, err = service.Unset(ctx, flags.workspace, flags.id, flags.positionals[2])
	default:
		result, err = service.List(ctx, flags.workspace, flags.id)
	}
	if err != nil {
		var structured *app.Error
		if errors.As(err, &structured) {
			writeStructuredSkillError(stdout, stderr, flags.jsonOutput, structured)
			return 2
		}
		return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
	}
	if flags.jsonOutput {
		if err := writeJSON(stdout, result); err != nil {
			termui.New(stderr).Line(err.Error())
			return 1
		}
		return 0
	}
	p := termui.New(stdout)
	if action == "list" {
		p.Heading("Stored variables: " + result.SkillID)
		if len(result.Keys) == 0 {
			p.Line("(none)")
		}
		for _, key := range result.Keys {
			p.Line(key)
		}
		p.Line("Values are never shown.")
	} else {
		p.Line(result.Summary)
	}
	if p.Err() != nil {
		return 1
	}
	return 0
}

// readSkillEnvValue reads one secret. From a terminal it prompts on stderr
// without echoing; otherwise it reads stdin and drops one trailing newline.
func readSkillEnvValue(key string, stderr io.Writer) (string, error) {
	if file, ok := skillEnvInput.(*os.File); ok && term.IsTerminal(int(file.Fd())) {
		fmt.Fprintf(stderr, "Value for %s (input is hidden): ", key)
		raw, err := term.ReadPassword(int(file.Fd()))
		fmt.Fprintln(stderr)
		if err != nil {
			return "", errors.New("could not read the value from the terminal")
		}
		return string(raw), nil
	}
	raw, err := io.ReadAll(io.LimitReader(skillEnvInput, skillEnvMaxInput+1))
	if err != nil {
		return "", errors.New("could not read the value from stdin")
	}
	if len(raw) > skillEnvMaxInput {
		return "", errors.New("the value is too large")
	}
	value := strings.TrimSuffix(string(raw), "\n")
	return strings.TrimSuffix(value, "\r"), nil
}
