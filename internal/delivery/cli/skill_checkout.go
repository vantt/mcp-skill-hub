package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/delivery/cli/termui"
	"github.com/vantt/skillhub-mcp/internal/shellquote"
)

// currentDirectory is the folder a project command works from.
func currentDirectory() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

// runSkillCheckout copies a hub skill into the project, to edit and try there.
func runSkillCheckout(ctx context.Context, flags skillFlags, stdout, stderr io.Writer) int {
	result, err := (app.SkillCheckoutService{}).Checkout(ctx, flags.workspace, app.SkillCheckoutInput{SkillID: flags.id, ProjectDir: currentDirectory()})
	if err != nil {
		return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
	}
	return writeResult(stdout, stderr, flags.jsonOutput, result, func(p *termui.Printer) {
		p.Line(result.Summary)
		if result.GitignoreAdded != "" {
			p.Line("Added " + result.GitignoreAdded[1:] + " to .gitignore.")
		}
		p.Line("The hub keeps serving the current version; agents in other projects are not affected.")
		p.Line(fmt.Sprintf("Try it: ask your agent to use /%s.", result.SkillID))
		p.Raw("When ready: skillhub skill commit " + shellquote.Arg(result.SkillID) + "\n")
	})
}
