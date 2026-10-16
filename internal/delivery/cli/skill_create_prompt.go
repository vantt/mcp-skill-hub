package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/delivery/cli/termui"
	"github.com/vantt/skillhub-mcp/internal/shellquote"
)

// skillCreateInput is where the create form reads answers from. Tests replace it.
var skillCreateInput io.Reader = os.Stdin

// skillCreateInteractive reports whether `skill create` may ask questions: both the
// input and the output must be a terminal, so scripts and agents never block on a prompt.
var skillCreateInteractive = func(stdout io.Writer) bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && stdoutIsTerminal(stdout)
}

// maxCreateExamples is how many example requests the form collects.
const maxCreateExamples = 2

var errCreateCancelled = errors.New("skill creation cancelled")

// createForm asks one question at a time and reads one line per answer.
type createForm struct {
	reader *bufio.Reader
	out    io.Writer
}

type lineResult struct {
	line string
	err  error
}

// readLine returns the next trimmed line. End of input and a cancelled context
// both mean the person gave up.
func (form *createForm) readLine(ctx context.Context) (string, error) {
	done := make(chan lineResult, 1)
	go func() {
		line, err := form.reader.ReadString('\n')
		done <- lineResult{line: line, err: err}
	}()
	select {
	case <-ctx.Done():
		return "", errCreateCancelled
	case res := <-done:
		if res.err != nil && strings.TrimSpace(res.line) == "" {
			return "", errCreateCancelled
		}
		return strings.TrimSpace(res.line), nil
	}
}

// ask prints a question and returns the answer, which may be empty.
func (form *createForm) ask(ctx context.Context, question string) (string, error) {
	fmt.Fprint(form.out, question+" ")
	return form.readLine(ctx)
}

// askUntilEmpty collects answers until an empty line, at most limit of them
// when limit is positive. An empty first answer gives an empty list.
func (form *createForm) askUntilEmpty(ctx context.Context, question string, limit int) ([]string, error) {
	var answers []string
	for limit <= 0 || len(answers) < limit {
		prompt := question
		if len(answers) > 0 {
			prompt = "Another one?"
		}
		answer, err := form.ask(ctx, prompt)
		if err != nil {
			return nil, err
		}
		if answer == "" {
			break
		}
		answers = append(answers, answer)
	}
	return answers, nil
}

// runSkillCreate scaffolds a new skill in the project. On a terminal it asks
// the questions the flags did not answer; without one the flags must carry the
// id and the description. It writes nothing on the hub.
func runSkillCreate(ctx context.Context, flags skillFlags, stdout, stderr io.Writer) int {
	input := app.SkillScaffoldInput{
		SkillID: flags.id, ProjectDir: currentDirectory(), Name: flags.name, Description: flags.description,
		Triggers: flags.triggers, NotFor: flags.notFor, Examples: flags.examples,
	}
	if flags.id == "" || flags.description == "" {
		if flags.jsonOutput || !skillCreateInteractive(stdout) {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "create requires a skill id and --description",
				"Run `skillhub skill create <id> --description \"<what it does>\" [--trigger <when>] [--not-for <when not>] [--example <request>]`.")
		}
	}
	if skillCreateInteractive(stdout) && !flags.jsonOutput {
		if err := askCreateForm(ctx, &input, flags, stdout); err != nil {
			return createFormExit(err, stdout, stderr)
		}
	}
	result, err := app.ScaffoldProjectSkill(ctx, input)
	if err != nil {
		return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
	}
	return writeResult(stdout, stderr, flags.jsonOutput, result, func(p *termui.Printer) {
		p.Line(result.Summary)
		if result.GitignoreAdded != "" {
			p.Line("Added " + result.GitignoreAdded[1:] + " to .gitignore.")
		}
		p.Line("Write the instructions there and try them: ask your agent to use /" + result.SkillID + ".")
		p.Raw("When ready: skillhub skill commit " + shellquote.Arg(result.SkillID) + "\n")
	})
}

// askCreateForm asks, in order, what the flags left open: the id, what the
// skill does, when to use it, when not to, and a few example requests.
func askCreateForm(ctx context.Context, input *app.SkillScaffoldInput, flags skillFlags, stdout io.Writer) error {
	form := &createForm{reader: bufio.NewReader(skillCreateInput), out: stdout}
	var err error
	if input.SkillID == "" {
		if input.SkillID, err = form.ask(ctx, "Skill id (lowercase letters, digits and hyphens)?"); err != nil {
			return err
		}
	}
	if input.Description == "" {
		if input.Description, err = form.ask(ctx, "What does this skill do?"); err != nil {
			return err
		}
	}
	if len(flags.triggers) == 0 {
		if input.Triggers, err = form.askUntilEmpty(ctx, "When should an agent use it? (empty line to finish)", 0); err != nil {
			return err
		}
	}
	if len(flags.notFor) == 0 {
		if input.NotFor, err = form.askUntilEmpty(ctx, "When should it not? (empty line to finish)", 0); err != nil {
			return err
		}
	}
	if len(flags.examples) == 0 {
		input.Examples, err = form.askUntilEmpty(ctx, "One or two example requests:", maxCreateExamples)
	}
	return err
}

// createFormExit maps a stopped form to an exit code; nothing was written.
func createFormExit(err error, stdout, stderr io.Writer) int {
	if errors.Is(err, errCreateCancelled) {
		fmt.Fprintln(stdout)
		p := termui.New(stderr)
		p.Line("Cancelled. Nothing was written.")
		return 130
	}
	return writeSkillErrorFor("", stdout, stderr, false, err)
}
