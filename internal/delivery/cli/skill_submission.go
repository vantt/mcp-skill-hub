package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/delivery/cli/termui"
	"github.com/vantt/skillhub-mcp/internal/shellquote"
	"github.com/vantt/skillhub-mcp/internal/workspace"
)

// remoteHubFor finds the hub on another machine this directory is connected
// to, when no hub was chosen explicitly and none is on this machine.
func remoteHubFor(workspaceFlag string) (app.RemoteHub, bool) {
	if strings.TrimSpace(workspaceFlag) != "" || strings.TrimSpace(os.Getenv(workspaceEnvVar)) != "" {
		return app.RemoteHub{}, false
	}
	return app.DetectRemoteHub(app.DetectRoleHere())
}

// remoteCommitFlagProblem names a flag a remote commit cannot take: what the
// hub decides is decided by the person who reviews the submission there.
func remoteCommitFlagProblem(flags skillFlags) string {
	switch {
	case flags.collection != "":
		return "--collection"
	case flags.runtimeFile != "":
		return "--runtime-file"
	case flags.hasRoutingFlags():
		return "the routing flags"
	case flags.editRouting:
		return "--edit-routing"
	case len(flags.accepts)+len(flags.manuals) > 0 || flags.writeConflicts != "":
		return "--accept, --manual and --write-conflicts"
	case flags.approveContent != "":
		return "--approve-content"
	case flags.diff:
		return "--diff"
	}
	return ""
}

// runRemoteCommit sends a project folder to the hub on another machine, where
// a person reviews it. Nothing on the hub changes until they approve.
func runRemoteCommit(ctx context.Context, flags skillFlags, stdout, stderr io.Writer) int {
	hub := *flags.remote
	if problem := remoteCommitFlagProblem(flags); problem != "" {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput,
			problem+" is decided by the person who reviews the submission on the hub machine",
			"Send the folder as it is; the reviewer sets collection and routing there.")
	}
	preview, err := remoteCommitService(flags).Preview(ctx, hub, flags.positionals[0], currentDirectory())
	if err != nil {
		return writeSkillErrorFor(flags.positionals[0], stdout, stderr, flags.jsonOutput, err)
	}
	if app.ErrorOf(preview, nil) != nil || preview.NothingToCommit {
		return writeResult(stdout, stderr, flags.jsonOutput, preview, func(p *termui.Printer) { p.Line(preview.Summary) })
	}
	question := fmt.Sprintf("Send %s to the hub at %s for review?", preview.SkillID, hub.URL)
	if flags.yes {
		if !hub.MaySendToken {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput,
				fmt.Sprintf("only this project's files name the hub at %s, so --yes does not send your hub token and files there", hub.URL),
				"Run the command in a terminal and answer the question, or trust that hub with `skillhub connect --hub "+hub.URL+" --user`.")
		}
		return sendSubmission(ctx, flags, preview, stdout, stderr)
	}
	interactive := !flags.jsonOutput && confirmInteractive(stdout)
	code := writeResult(stdout, stderr, flags.jsonOutput, preview, func(p *termui.Printer) {
		writeRemoteCommitPreview(p, preview)
		if !interactive {
			p.Line("Nothing sent yet.")
			p.Raw("Send: " + flags.rerun + " --yes\n")
		}
	})
	if !interactive || code != 0 {
		return code
	}
	if !askYesNo(stdout, question) {
		fmt.Fprintln(stdout, "Nothing sent.")
		return 0
	}
	return sendSubmission(ctx, flags, preview, stdout, stderr)
}

// remoteCommitService names this machine with --label, or with its host name.
func remoteCommitService(flags skillFlags) app.RemoteCommitService {
	service := app.RemoteCommitService{}
	if flags.label != nil {
		service.Label = *flags.label
	}
	return service
}

func sendSubmission(ctx context.Context, flags skillFlags, preview app.RemoteCommitPreview, stdout, stderr io.Writer) int {
	result, err := remoteCommitService(flags).Send(ctx, *flags.remote, preview)
	if err != nil {
		return writeSkillErrorFor(preview.SkillID, stdout, stderr, flags.jsonOutput, err)
	}
	return writeResult(stdout, stderr, flags.jsonOutput, result, func(p *termui.Printer) {
		p.Line(fmt.Sprintf("Sent as %s.  A person on the hub machine reviews it; agents get it only after approval.", result.SubmissionID))
	})
}

func writeRemoteCommitPreview(p *termui.Printer, preview app.RemoteCommitPreview) {
	p.Line(preview.Summary)
	if preview.CheckedOut {
		rows := make([]string, 0, len(preview.Files))
		for _, file := range preview.Files {
			rows = append(rows, file.Change+" "+file.Path+"\t"+termui.Bytes(file.Size))
		}
		if len(rows) > 0 {
			p.Raw(alignedRows(rows, "  ", false))
		}
	} else {
		paths := make([]string, 0, len(preview.Files))
		for _, file := range preview.Files {
			paths = append(paths, file.Path)
		}
		p.Line(fmt.Sprintf("Files (%d, %s): %s", len(preview.Files), termui.Bytes(preview.TotalBytes), strings.Join(paths, ", ")))
	}
	writeLeftOutLine(p, preview.Excluded)
	for _, warning := range preview.Warnings {
		p.Line(warning.Summary)
	}
}

// runRemoteCheckout copies an active skill of the hub on another machine into
// the project, through the bundle the agents' proxy fetches.
func runRemoteCheckout(ctx context.Context, flags skillFlags, stdout, stderr io.Writer) int {
	// Fetching sends the hub token. A hub only this project's files name could
	// be anyone's, so a person must say yes first.
	if !flags.remote.MaySendToken {
		if flags.jsonOutput || !confirmInteractive(stdout) {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput,
				fmt.Sprintf("only this project's files name the hub at %s, so checkout does not send your hub token there", flags.remote.URL),
				"Run the command in a terminal and answer the question, or trust that hub with `skillhub connect --hub "+flags.remote.URL+" --user`.")
		}
		if !askYesNo(stdout, fmt.Sprintf("Fetch %s from the hub at %s? Your hub token is sent there.", flags.id, flags.remote.URL)) {
			fmt.Fprintln(stdout, "Nothing fetched.")
			return 0
		}
	}
	result, err := (app.RemoteCheckoutService{}).Checkout(ctx, *flags.remote, app.SkillCheckoutInput{SkillID: flags.id, ProjectDir: currentDirectory()})
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

// runSubmissionReview previews a waiting submission like a folder commit and,
// at a terminal, asks the questions that apply it. An agent never approves:
// without a terminal it only shows the preview.
func runSubmissionReview(ctx context.Context, flags skillFlags, stdout, stderr io.Writer) int {
	id := flags.id
	root := flags.workspace
	submission, err := readQueuedSubmission(root, id)
	if err != nil {
		return writeSkillErrorFor(id, stdout, stderr, flags.jsonOutput, err)
	}
	if flags.discard {
		if err := app.DiscardSubmission(root, id); err != nil {
			return writeSkillErrorFor(id, stdout, stderr, flags.jsonOutput, err)
		}
		result := struct {
			app.Result
			SubmissionID string `json:"submission_id"`
			SkillID      string `json:"skill_id"`
		}{app.NewResult(app.StatusApplied, fmt.Sprintf("Discarded %s (%s from %s).", id, submission.SkillID, submission.ClientLabel)), id, submission.SkillID}
		return writeResult(stdout, stderr, flags.jsonOutput, result, func(p *termui.Printer) { p.Line(result.Summary) })
	}
	input := resolutionInput(flags)
	input.FullDiff = flags.diff
	service := app.SkillCommitService{}
	preview, err := service.PreviewSubmission(ctx, root, id, input)
	if err != nil {
		return writeSkillErrorFor(id, stdout, stderr, flags.jsonOutput, err)
	}
	if app.ErrorOf(preview, nil) != nil {
		return writeResult(stdout, stderr, flags.jsonOutput, preview, nil)
	}
	if flags.editRouting {
		edited, code, ok := editSuggestedRouting(ctx, flags, preview, stdout, stderr)
		if !ok {
			return code
		}
		input.Routing = edited
		if preview, err = service.PreviewSubmission(ctx, root, id, input); err != nil {
			return writeSkillErrorFor(id, stdout, stderr, flags.jsonOutput, err)
		}
		if app.ErrorOf(preview, nil) != nil {
			return writeResult(stdout, stderr, flags.jsonOutput, preview, nil)
		}
	}
	if flags.writeConflicts != "" {
		if writeErr := writeConflictsFiles(flags.writeConflicts, preview.MergeFiles); writeErr != nil {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, writeErr.Error(), "Choose an empty or different directory for --write-conflicts.")
		}
	}
	preview.Summary = submissionSummary(preview, submission)
	canApply := !preview.NothingToCommit && len(preview.Conflicts) == 0
	interactive := canApply && !flags.jsonOutput && confirmInteractive(stdout)
	code := writeResult(stdout, stderr, flags.jsonOutput, preview, func(p *termui.Printer) {
		writeSkillCommitPreview(p, preview, previewHints{rerun: flags.rerun, diffShown: flags.diff, interactive: interactive, submission: true})
		if preview.NothingToCommit {
			p.Raw("Drop it with: skillhub skill review " + id + " --discard\n")
		}
	})
	if !interactive || code != 0 {
		return code
	}
	options, proceed := askSubmissionQuestions(stdout, preview)
	if !proceed {
		return 0
	}
	result, err := service.ApplySubmission(commitContext(ctx, false, stdout), root, preview, options)
	if err != nil {
		return writeSkillErrorFor(id, stdout, stderr, flags.jsonOutput, err)
	}
	return writeResult(stdout, stderr, flags.jsonOutput, result, func(p *termui.Printer) { writeSubmissionResult(p, result) })
}

func readQueuedSubmission(root, id string) (app.Submission, error) {
	return app.ReadSubmission(root, id)
}

// submissionSummary is the first line of a submission's preview.
func submissionSummary(preview app.SkillCommitPreview, submission app.Submission) string {
	who := fmt.Sprintf("%s (submitted from %s)", submission.SkillID, submission.ClientLabel)
	switch preview.Case {
	case "new":
		return who + ": a new skill."
	case "replace":
		return who + ": a rewrite of the hub's copy."
	case "merge":
		return fmt.Sprintf("%s: %s in the submission; the hub changed %s since it was checked out.", who, termui.Plural(preview.ProjectChanged, "file changed", "files changed"), termui.Plural(preview.HubChanged, "file", "files"))
	}
	if preview.NothingToCommit {
		return who + ": it matches the hub, so there is nothing to apply."
	}
	return fmt.Sprintf("%s: %s changed since checkout.", who, termui.Plural(len(preview.Files), "file", "files"))
}

// askSubmissionQuestions asks what a submission needs, in order: apply, then
// approve (always, since the content comes from another machine), then, for a
// new skill that can be, activate. A "no" at any step writes nothing.
func askSubmissionQuestions(stdout io.Writer, preview app.SkillCommitPreview) (app.SkillCommitApplyOptions, bool) {
	options := app.SkillCommitApplyOptions{}
	declined := func(reason string) (app.SkillCommitApplyOptions, bool) {
		termui.New(stdout).Line("Nothing written." + reason)
		return options, false
	}
	first := "Apply this change?"
	if preview.Case == "new" {
		first = fmt.Sprintf("Add %s as a draft?", preview.SkillID)
	}
	if !askYesNo(stdout, first) {
		return declined("")
	}
	if !askYesNo(stdout, "Approve this content for agents?") {
		return declined(" Content from another machine is applied only together with its approval.")
	}
	options.Approve = true
	if preview.Case == "new" && preview.ActivationReady {
		options.Activate = askYesNo(stdout, fmt.Sprintf("Activate %s now?", preview.SkillID))
	}
	return options, true
}

func writeSubmissionResult(p *termui.Printer, result app.SkillCommitResult) {
	outcome := result.Commit
	if outcome == nil || outcome.Status != workspace.CommitCommitted {
		p.Line(result.Summary)
		writeCommitLines(p, outcome)
		return
	}
	p.Line(fmt.Sprintf("Committed %s %q.", workspace.ShortHash(outcome.Hash), outcome.Message))
	for _, warning := range result.Warnings {
		p.Warning(warning.Summary)
	}
	if outcome.Reach.Agents != "" {
		p.Line(outcome.Reach.Agents)
	}
	if outcome.Reach.PushNeeded {
		p.Line("Other clones get it after `" + outcome.Reach.PushCommand + "`.")
	}
	if result.Case == "new" && !result.Activated {
		p.Next("it is approved but still a draft:", "skillhub skill activate "+shellquote.Arg(result.SkillID))
	}
}

// writeWaitingSubmissions names the submissions that wait for a skill, below
// the skill's own review.
func writeWaitingSubmissions(stdout io.Writer, root, skillID string) {
	submissions, err := app.ListSubmissions(root)
	if err != nil {
		return
	}
	var lines []string
	for _, submission := range submissions {
		if submission.SkillID == skillID {
			lines = append(lines, fmt.Sprintf("%s from %s (%s): skillhub skill review %s", submission.ID, submission.ClientLabel, submission.SubmittedAt[:min(10, len(submission.SubmittedAt))], submission.ID))
		}
	}
	if len(lines) == 0 {
		return
	}
	p := termui.New(stdout)
	p.Heading(termui.Plural(len(lines), "submission waits", "submissions wait") + " for this skill")
	p.Bullets(lines...)
}
