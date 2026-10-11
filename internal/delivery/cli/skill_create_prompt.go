package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/term"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
	"github.com/vantt/mcp-skill-hub/internal/shellquote"
	"github.com/vantt/mcp-skill-hub/internal/skill"
)

// skillCreateInput is where the guided questions read answers from. Tests replace it.
var skillCreateInput io.Reader = os.Stdin

// skillCreateInteractive reports whether `skill create` may ask questions: both the
// input and the output must be a terminal, so scripts and agents never block on a prompt.
var skillCreateInteractive = func(stdout io.Writer) bool {
	return term.IsTerminal(int(os.Stdin.Fd())) && stdoutIsTerminal(stdout)
}

const skillCreateMissingFlags = "create requires --id, --collection, --name, and --description"

var (
	errGuidedCancelled = errors.New("skill creation cancelled")
	errGuidedEmpty     = errors.New("an answer is required")
	nonKebabRun        = regexp.MustCompile(`[^a-z0-9]+`)
)

// guidedPrompter asks one question at a time and reads one line per answer.
type guidedPrompter struct {
	reader *bufio.Reader
	out    io.Writer
}

type lineResult struct {
	line string
	err  error
}

// readLine returns the next trimmed line. End of input and a cancelled context
// both mean the person gave up.
func (p *guidedPrompter) readLine(ctx context.Context) (string, error) {
	done := make(chan lineResult, 1)
	go func() {
		line, err := p.reader.ReadString('\n')
		done <- lineResult{line: line, err: err}
	}()
	select {
	case <-ctx.Done():
		return "", errGuidedCancelled
	case res := <-done:
		if res.err != nil && strings.TrimSpace(res.line) == "" {
			return "", errGuidedCancelled
		}
		return strings.TrimSpace(res.line), nil
	}
}

// ask prints the question and returns the answer; an empty answer takes the suggestion
// when there is one and is errGuidedEmpty otherwise.
func (p *guidedPrompter) ask(ctx context.Context, question, suggestion string) (string, error) {
	if suggestion != "" {
		fmt.Fprintf(p.out, "%s [%s]: ", question, suggestion)
	} else {
		fmt.Fprintf(p.out, "%s: ", question)
	}
	answer, err := p.readLine(ctx)
	if err != nil {
		return "", err
	}
	if answer == "" {
		if suggestion == "" {
			return "", errGuidedEmpty
		}
		return suggestion, nil
	}
	return answer, nil
}

// askLines collects one or more lines; the first is required and a blank line ends the list.
func (p *guidedPrompter) askLines(ctx context.Context, question string) ([]string, error) {
	var lines []string
	for {
		prompt := question
		if len(lines) > 0 {
			prompt = "Another one (blank line to finish)"
		}
		answer, err := p.ask(ctx, prompt, "")
		if errors.Is(err, errGuidedEmpty) && len(lines) > 0 {
			return lines, nil
		}
		if err != nil {
			return nil, err
		}
		lines = append(lines, answer)
	}
}

// suggestSkillID turns a display name into a kebab-case id, or "" when nothing usable remains.
func suggestSkillID(name string) string {
	id := strings.Trim(nonKebabRun.ReplaceAllString(strings.ToLower(name), "-"), "-")
	if len(id) > 63 {
		id = strings.Trim(id[:63], "-")
	}
	return id
}

// existingSkillContext returns the ids in use and the collection a new skill defaults to:
// the only existing collection, otherwise core.
func existingSkillContext(ctx context.Context, service app.SkillService, workspace string) (map[string]bool, string, error) {
	list, err := service.ListSkills(ctx, workspace, "")
	if err != nil {
		return nil, "", err
	}
	ids := make(map[string]bool, len(list.Skills))
	collections := map[string]bool{}
	for _, entry := range list.Skills {
		ids[entry.ID] = true
		collections[entry.Collection] = true
	}
	if len(collections) == 1 {
		for name := range collections {
			return ids, name, nil
		}
	}
	return ids, "core", nil
}

// checkGuidedID runs the same validation as the flag form by previewing a create for the id.
// It returns a message when the id can be asked for again, and an error when the preview fails
// for any other reason.
func checkGuidedID(ctx context.Context, service app.SkillService, flags skillFlags, ids map[string]bool, id string) (string, error) {
	if ids[id] {
		return fmt.Sprintf("A skill with the id %q already exists. Choose another id.", id), nil
	}
	_, err := service.PreviewCreate(ctx, flags.workspace, skill.CreateInput{
		ID: id, Collection: flags.collection, Name: "probe", Description: "probe",
	}, false)
	switch {
	case err == nil:
		return "", nil
	case errors.Is(err, skill.ErrAlreadyExists):
		return fmt.Sprintf("A skill with the id %q already exists. Choose another id.", id), nil
	case strings.Contains(err.Error(), "kebab-case"):
		return "Use lowercase letters, digits and hyphens, starting with a letter or digit (for example commit-style).", nil
	default:
		return "", err
	}
}

// runGuidedSkillCreateCommand starts the guided questions when the session is a terminal and
// otherwise reports the missing flags, as a create without them always did.
func runGuidedSkillCreateCommand(ctx context.Context, flags skillFlags, stdout, stderr io.Writer) int {
	if !skillCreateInteractive(stdout) {
		return writeInvalidRequest(stdout, stderr, false, skillCreateMissingFlags, "Review `skillhub skill create` arguments and retry.")
	}
	resolved, err := resolveWorkspace(flags.workspace)
	if err != nil {
		var resErr *WorkspaceResolutionError
		if errors.As(err, &resErr) {
			return writeWorkspaceResolutionError(stdout, stderr, false, resErr)
		}
		return writeInvalidRequest(stdout, stderr, false, err.Error(), "Review `skillhub skill create` arguments and retry.")
	}
	flags.workspace = resolved
	return runGuidedSkillCreate(ctx, app.SkillService{}, flags, skillCreateInput, stdout, stderr)
}

// runGuidedSkillCreate asks for the skill's identity and routing, shows a summary and saves the
// draft. The answers are the confirmation, so nothing is written until the last answer is in.
func runGuidedSkillCreate(ctx context.Context, service app.SkillService, flags skillFlags, in io.Reader, stdout, stderr io.Writer) int {
	prompter := &guidedPrompter{reader: bufio.NewReader(in), out: stdout}
	ids, defaultCollection, err := existingSkillContext(ctx, service, flags.workspace)
	if err != nil {
		return writeSkillErrorFor(flags.id, stdout, stderr, false, err)
	}
	if flags.collection == "" {
		flags.collection = defaultCollection
	}
	if flags.minScope == "" {
		flags.minScope = "single_step"
	}
	if len(flags.operations) == 0 {
		flags.operations = []string{"other"}
	}

	p := termui.New(stdout)
	p.Line("Create a draft skill. Answer each question; press Ctrl-C to stop without writing anything.")
	p.Blank()
	answers, err := askGuidedAnswers(ctx, service, &flags, prompter, ids, stdout)
	if err != nil {
		return guidedAnswerExit(err, stdout, stderr)
	}
	flags.name, flags.description = answers.name, answers.description
	flags.triggers, flags.notFor = answers.triggers, answers.notFor

	p.Blank()
	p.Fields(
		termui.Field{Label: "Skill", Value: fmt.Sprintf("%s (%s)", flags.name, flags.id)},
		termui.Field{Label: "Collection", Value: flags.collection},
		termui.Field{Label: "Description", Value: flags.description},
		termui.Field{Label: "Use when", Value: strings.Join(flags.triggers, "; ")},
		termui.Field{Label: "Not for", Value: strings.Join(flags.notFor, "; ")},
		termui.Field{Label: "Min scope", Value: flags.minScope},
	)
	p.Blank()

	preview, err := service.PreviewCreate(ctx, flags.workspace, skill.CreateInput{
		ID: flags.id, Collection: flags.collection, Name: flags.name, Description: flags.description,
		Routing: skill.RoutingInput{Operations: flags.operations, Triggers: flags.triggers, NotFor: flags.notFor, MinScope: flags.minScope},
	}, false)
	if err != nil {
		return writeSkillErrorFor(flags.id, stdout, stderr, false, err)
	}
	result, err := service.ConfirmSkillMutation(ctx, flags.workspace, preview, preview.Confirmation.Confirmation.Pins)
	if err != nil {
		return writeSkillErrorFor(flags.id, stdout, stderr, false, err)
	}
	p.Line(result.Summary)
	file := filepath.Join(flags.workspace, "skills", flags.collection, flags.id, "SKILL.md")
	p.Fields(termui.Field{Label: "File", Value: file})
	p.Next("replace the starter text in that file with your instructions, then activate the skill:", "skillhub skill activate "+shellquote.Arg(flags.id)+" --yes")
	return 0
}

type guidedAnswers struct {
	name, description string
	triggers, notFor  []string
}

// askGuidedAnswers asks the questions in order. The name comes first so the id can be suggested from it.
func askGuidedAnswers(ctx context.Context, service app.SkillService, flags *skillFlags, prompter *guidedPrompter, ids map[string]bool, stdout io.Writer) (guidedAnswers, error) {
	var answers guidedAnswers
	var err error
	if answers.name, err = prompter.ask(ctx, "Name of the skill (for example: Commit style)", ""); err != nil {
		return answers, err
	}
	if flags.id == "" {
		suggestion := suggestSkillID(answers.name)
		for {
			id, askErr := prompter.ask(ctx, "Skill id (lowercase, hyphens)", suggestion)
			if askErr != nil {
				return answers, askErr
			}
			message, checkErr := checkGuidedID(ctx, service, *flags, ids, id)
			if checkErr != nil {
				return answers, checkErr
			}
			if message == "" {
				flags.id = id
				break
			}
			fmt.Fprintln(stdout, message)
			suggestion = ""
		}
	}
	if answers.description, err = prompter.ask(ctx, "One-sentence description of what the skill does", ""); err != nil {
		return answers, err
	}
	if answers.triggers, err = prompter.askLines(ctx, "When should an agent use it? (a phrase such as \"write a commit message\")"); err != nil {
		return answers, err
	}
	answers.notFor, err = prompter.askLines(ctx, "When should an agent not use it? (a phrase such as \"write release notes\")")
	return answers, err
}

// guidedAnswerExit maps a stopped conversation to an exit code; nothing was written.
func guidedAnswerExit(err error, stdout, stderr io.Writer) int {
	if errors.Is(err, errGuidedCancelled) {
		fmt.Fprintln(stdout)
		p := termui.New(stderr)
		p.Line("Cancelled. Nothing was written.")
		return 130
	}
	if errors.Is(err, errGuidedEmpty) {
		p := termui.New(stderr)
		p.Error("A required answer was empty.", "Nothing was written.", "Run `skillhub skill create` again and answer every question.")
		return 2
	}
	return writeSkillErrorFor("", stdout, stderr, false, err)
}
