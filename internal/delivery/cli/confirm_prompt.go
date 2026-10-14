package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/vantt/skillhub-mcp/internal/delivery/cli/termui"
	"github.com/vantt/skillhub-mcp/internal/shellquote"
	"github.com/vantt/skillhub-mcp/internal/workspace"
)

// confirmInput is where the "Apply this change?" answer is read from. Tests
// replace it.
var confirmInput io.Reader = os.Stdin

// confirmInteractive reports whether a person is at a terminal: both the input
// and the output must be one, so scripts and agents never block on a prompt.
// Tests replace it.
var confirmInteractive = func(stdout io.Writer) bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && stdoutIsTerminal(stdout)
}

// confirmInPlace prints a preview with print and, when a person is at a
// terminal, asks whether to apply exactly that preview. It reports whether the
// caller should apply, and the exit code to use when it should not. print
// receives whether the prompt follows, so the preview ends with the right
// hint lines, and returns its own exit code.
func confirmInPlace(stdout io.Writer, jsonOutput, canApply bool, print func(interactive bool) int) (apply bool, code int) {
	interactive := canApply && !jsonOutput && confirmInteractive(stdout)
	code = print(interactive)
	if !interactive || code != 0 {
		return false, code
	}
	if askToApply(stdout) {
		return true, 0
	}
	fmt.Fprintln(stdout, "Nothing written.")
	return false, 0
}

// askToApply asks for one answer. Only "y" or "yes" applies the change.
func askToApply(stdout io.Writer) bool {
	fmt.Fprint(stdout, "Apply this change? [y/N] ")
	line, _ := bufio.NewReader(confirmInput).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	}
	return false
}

// previewHints says how a preview ends: the command that applies it, whether
// the diff is already on screen, and whether a prompt follows.
type previewHints struct {
	rerun                  string
	diffShown, interactive bool
}

// previewTail ends a preview. Without a terminal it says nothing was written
// and names the two follow-up commands; at a terminal the prompt follows.
func previewTail(p *termui.Printer, rerun string, diffShown, interactive bool) {
	if !interactive {
		p.Line("Nothing written yet.")
	}
	if !diffShown {
		p.Raw("Show the diff: " + rerun + " --diff\n")
	}
	if !interactive {
		p.Raw("Apply: " + rerun + " --yes\n")
	}
}

// rerunCommand renders the command the person typed, so a printed hint can be
// run as shown.
func rerunCommand(prefix string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, prefix)
	for _, arg := range args {
		parts = append(parts, shellquote.Arg(arg))
	}
	return strings.Join(parts, " ")
}

// commitContext carries the commit policy of this command to the mutation
// layer: commit unless --no-commit, and let signing prompt when a person is at
// a terminal.
func commitContext(ctx context.Context, noCommit bool, stdout io.Writer) context.Context {
	return workspace.WithCommitPolicy(ctx, workspace.CommitPolicy{Disabled: noCommit, Interactive: confirmInteractive(stdout)})
}

// writeCommitLines prints what happened to the Git commit of an applied change.
func writeCommitLines(p *termui.Printer, outcome *workspace.CommitOutcome) {
	if outcome == nil {
		return
	}
	switch outcome.Status {
	case workspace.CommitCommitted:
		p.Line(fmt.Sprintf("Committed %s %q.", workspace.ShortHash(outcome.Hash), outcome.Message))
		text := "Agents using this hub get it now."
		if outcome.Reach.PushNeeded {
			text = "Agents using this hub get it now; other clones after `" + outcome.Reach.PushCommand + "`."
		}
		p.Line(text)
	case workspace.CommitFailed:
		p.Line("Not committed: " + outcome.Reason)
		p.Raw("Finish it with: " + outcome.ManualCommand + "\n")
	case workspace.CommitSkipped:
		p.Line("Not committed: " + outcome.Reason)
	}
}
