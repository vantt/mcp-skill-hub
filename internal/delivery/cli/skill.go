package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/skill"
)

type skillFlags struct {
	workspace, id, collection, name, description, contentFile, minScope, rationale string
	proposalID, proposalDigest, baseVersion, idempotencyKey, state                 string
	locator, ref, subPath                                                          string
	operations, triggers, notFor, skills, positionals                              []string
	jsonOutput, yes, fullDiff, editor, verbose, all                                bool
}

func runSkill(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), "skill requires a subcommand", "Run `skillhub skill list|show|create|edit|review|add|confirm|activate|deprecate|archive`.")
	}
	subcommand := args[0]
	flags, err := parseSkillFlags(subcommand, args[1:])
	if err != nil {
		var resErr *WorkspaceResolutionError
		if errors.As(err, &resErr) {
			return writeWorkspaceResolutionError(stdout, stderr, hasJSONFlag(args), resErr)
		}
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Review `skillhub skill "+subcommand+"` arguments and retry.")
	}
	service := app.SkillService{}
	if subcommand == "add" {
		return runSkillAdd(ctx, service, flags, stdout, stderr)
	}
	if subcommand == "review" {
		result, err := service.ReviewSkill(ctx, flags.workspace, flags.id)
		if err != nil {
			return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
		}
		return writeSkillReview(stdout, stderr, flags.jsonOutput, flags.verbose, result)
	}
	if subcommand == "list" {
		return runSkillList(ctx, service, flags, stdout, stderr)
	}
	if subcommand == "show" {
		result, err := service.ReadSkill(ctx, flags.workspace, flags.id)
		if err != nil {
			return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
		}
		if flags.jsonOutput {
			if err := writeJSON(stdout, result); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
		} else {
			routing, _ := service.ReadSkillRouting(ctx, flags.workspace, flags.id)
			filePath := ""
			for _, res := range result.Manifest.Resources {
				if strings.HasSuffix(res.Path, "/SKILL.md") {
					filePath = res.Path
					break
				}
			}
			triggers := strings.Join(routing.Triggers, ", ")
			if triggers == "" {
				triggers = "(none)"
			}
			notFor := strings.Join(routing.NotFor, ", ")
			if notFor == "" {
				notFor = "(none)"
			}
			minScope := routing.MinScope
			if minScope == "" {
				minScope = "(none)"
			}
			fmt.Fprintf(stdout, "Skill: %s (%s)\n", result.Manifest.Name, flags.id)
			fmt.Fprintf(stdout, "State: %s\n", result.Manifest.Status)
			if filePath != "" {
				fmt.Fprintf(stdout, "File: %s\n", filePath)
			}
			fmt.Fprintf(stdout, "Triggers: %s\n", triggers)
			fmt.Fprintf(stdout, "Not for: %s\n", notFor)
			fmt.Fprintf(stdout, "Min scope: %s\n", minScope)
			if flags.verbose {
				fmt.Fprintf(stdout, "Catalog snapshot: %s\n", result.Manifest.CatalogSnapshot)
			}
			fmt.Fprintln(stdout)
			fmt.Fprint(stdout, result.Content)
			if !strings.HasSuffix(result.Content, "\n") {
				fmt.Fprintln(stdout)
			}
		}
		return 0
	}
	if subcommand == "confirm" {
		var pins *app.ConfirmationPins
		if flags.proposalDigest != "" && flags.baseVersion != "" {
			pins = &app.ConfirmationPins{
				ProposalID:     flags.proposalID,
				ProposalDigest: flags.proposalDigest,
				BaseVersion:    flags.baseVersion,
			}
		}
		res, err := service.DispatchConfirmProposal(ctx, flags.workspace, flags.proposalID, pins)
		if err != nil {
			return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
		}
		_ = cleanupProposalRecovery(flags.workspace, flags.proposalID)
		switch r := res.(type) {
		case app.SkillMutationResult:
			return writeSkillMutation(stdout, stderr, flags.jsonOutput, flags.verbose, r, flags.workspace)
		case app.SkillAddResult:
			return writeSkillAddResult(stdout, stderr, flags.jsonOutput, flags.verbose, r, flags.workspace)
		default:
			if flags.jsonOutput {
				if err := writeJSON(stdout, res); err != nil {
					fmt.Fprintln(stderr, err)
					return 1
				}
				return 0
			}
			fmt.Fprintf(stdout, "Proposal %s confirmed.\n", flags.proposalID)
			return 0
		}
	}

	var preview app.SkillProposal
	var recoveryID string
	switch subcommand {
	case "create":
		content, err := readContentFile(flags.contentFile)
		if err != nil {
			return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
		}
		preview, err = service.PreviewCreate(ctx, flags.workspace, skill.CreateInput{
			ID: flags.id, IdempotencyKey: flags.idempotencyKey, Collection: flags.collection, Name: flags.name, Description: flags.description, Content: content,
			Routing:   skill.RoutingInput{Operations: flags.operations, Triggers: flags.triggers, NotFor: flags.notFor, MinScope: flags.minScope},
			Rationale: flags.rationale,
		}, flags.fullDiff)
		if err != nil {
			return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
		}
	case "edit":
		var content []byte
		var setContent bool
		var expectedDigest string
		var recPath string

		if flags.editor {
			session, sessErr := openEditorSession(ctx, service, flags.workspace, flags.id)
			if sessErr != nil {
				return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, sessErr)
			}
			content = session.updated
			setContent = true
			expectedDigest = session.expectedDigest
			recoveryID = session.recoveryID
			recPath = session.recoveryPath
		} else if flags.contentFile != "" {
			var readErr error
			content, readErr = readContentFile(flags.contentFile)
			if readErr != nil {
				return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, readErr)
			}
			setContent = true
		}

		update := skill.UpdateInput{
			IdempotencyKey:        flags.idempotencyKey,
			Content:               content,
			SetContent:            setContent,
			ExpectedContentDigest: expectedDigest,
		}
		if flags.name != "" {
			update.Name = &flags.name
		}
		if flags.description != "" {
			update.Description = &flags.description
		}
		if flags.rationale != "" {
			update.Rationale = &flags.rationale
		}
		if len(flags.operations)+len(flags.triggers)+len(flags.notFor) > 0 || flags.minScope != "" {
			current, err := service.ReadSkillRouting(ctx, flags.workspace, flags.id)
			if err != nil {
				return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
			}
			update.Routing = mergeRouting(current, flags)
		}
		fullDiff := flags.fullDiff || flags.editor
		preview, err = service.PreviewSkillUpdate(ctx, flags.workspace, flags.id, update, fullDiff)
		if err != nil {
			if recoveryID != "" && !flags.jsonOutput {
				fmt.Fprintf(stderr, "Edited content saved to %s\n", recPath)
			}
			return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
		}
		if preview.Error != nil {
			if recoveryID != "" && !flags.jsonOutput {
				fmt.Fprintf(stderr, "Edited content saved to %s\n", recPath)
			}
			if flags.jsonOutput {
				_ = writeJSON(stdout, preview)
				return 2
			}
			fmt.Fprintf(stderr, "ERROR: %s\nWHY: %s\nFIX: %s\n",
				preview.Error.Render.Error, preview.Error.Render.Why, preview.Error.Render.Fix)
			return 2
		}
		if recoveryID != "" {
			_, _ = service.SaveEditorRecovery(ctx, flags.workspace, preview.Confirmation.Confirmation.Pins.ProposalID, content)
		}
	case "activate", "deprecate", "archive":
		preview, err = service.PreviewTransitionWithKey(ctx, flags.workspace, flags.id, subcommandTarget(subcommand), flags.fullDiff, flags.idempotencyKey)
	default:
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, fmt.Sprintf("unsupported skill subcommand %q", subcommand), "Run `skillhub skill list|show|create|edit|review|add|confirm|activate|deprecate|archive`.")
	}
	if err != nil {
		return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
	}
	if !flags.yes {
		if flags.editor && !flags.jsonOutput {
			writeEditorSkillPreview(stdout, preview)
			return 0
		}
		return writeSkillPreview(stdout, stderr, flags.jsonOutput, preview)
	}
	if !flags.jsonOutput && preview.RoutingImpact != nil {
		for _, warning := range preview.RoutingImpact.Warnings {
			fmt.Fprintf(stdout, "WARNING: %s\n", warning)
		}
	}
	result, err := service.ConfirmSkillMutation(ctx, flags.workspace, preview, preview.Confirmation.Confirmation.Pins)
	if err != nil {
		return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
	}
	if recoveryID != "" {
		_ = service.DeleteEditorRecovery(ctx, flags.workspace, recoveryID)
		_ = cleanupProposalRecovery(flags.workspace, preview.Confirmation.Confirmation.Pins.ProposalID)
	}
	if flags.jsonOutput {
		result.Proposal = &preview
	}
	return writeSkillMutation(stdout, stderr, flags.jsonOutput, flags.verbose, result, flags.workspace)
}

func subcommandTarget(subcommand string) string {
	return map[string]string{"activate": "active", "deprecate": "deprecated", "archive": "archived"}[subcommand]
}

func parseSkillFlags(subcommand string, args []string) (skillFlags, error) {
	flags := skillFlags{}
	positionals := []string{}
	for index := 0; index < len(args); index++ {
		value := args[index]
		next := func() (string, error) {
			if index+1 >= len(args) || strings.HasPrefix(args[index+1], "--") {
				return "", fmt.Errorf("%s requires a value", value)
			}
			index++
			return args[index], nil
		}
		switch value {
		case "--workspace", "--id", "--collection", "--name", "--description", "--content-file", "--min-scope", "--rationale", "--operation", "--trigger", "--not-for", "--proposal", "--proposal-digest", "--base-version", "--idempotency-key", "--state", "--ref", "--path", "--skill":
			item, err := next()
			if err != nil {
				return flags, err
			}
			switch value {
			case "--workspace":
				flags.workspace = item
			case "--id":
				flags.id = item
			case "--collection":
				flags.collection = item
			case "--name":
				flags.name = item
			case "--description":
				flags.description = item
			case "--content-file":
				flags.contentFile = item
			case "--min-scope":
				flags.minScope = item
			case "--rationale":
				flags.rationale = item
			case "--operation":
				flags.operations = append(flags.operations, item)
			case "--trigger":
				flags.triggers = append(flags.triggers, item)
			case "--not-for":
				flags.notFor = append(flags.notFor, item)
			case "--proposal":
				flags.proposalID = item
			case "--proposal-digest":
				flags.proposalDigest = item
			case "--base-version":
				flags.baseVersion = item
			case "--idempotency-key":
				flags.idempotencyKey = item
			case "--state":
				flags.state = item
			case "--ref":
				flags.ref = item
			case "--path":
				flags.subPath = item
			case "--skill":
				flags.skills = append(flags.skills, item)
			}
		case "--all":
			flags.all = true
		case "--json":
			flags.jsonOutput = true
		case "--yes":
			flags.yes = true
		case "--full-diff":
			flags.fullDiff = true
		case "--editor":
			flags.editor = true
		case "--verbose":
			flags.verbose = true
		default:
			if strings.HasPrefix(value, "-") {
				return flags, fmt.Errorf("unknown argument %q", value)
			}
			positionals = append(positionals, value)
		}
	}
	flags.positionals = positionals
	if flags.state != "" && subcommand != "list" {
		return flags, errors.New("--state is available only for skill list")
	}
	switch subcommand {
	case "add":
		if len(positionals) != 1 {
			return flags, errors.New("add requires exactly one locator")
		}
		flags.locator = positionals[0]
		if flags.all && len(flags.skills) > 0 {
			return flags, errors.New("--all and --skill cannot be combined")
		}
		if flags.all && flags.id != "" {
			return flags, errors.New("cannot specify --id when adding multiple skills")
		}
	case "review":
		if len(positionals) == 1 {
			if flags.id != "" && flags.id != positionals[0] {
				return flags, fmt.Errorf("positional skill ID %q conflicts with --id %q", positionals[0], flags.id)
			}
			flags.id = positionals[0]
		} else if len(positionals) == 0 && flags.id != "" {
			// flags.id provided via --id
		} else {
			return flags, errors.New("review requires exactly one skill ID")
		}
		if flags.yes || flags.fullDiff || flags.idempotencyKey != "" || flags.editor || flags.contentFile != "" || flags.proposalID != "" {
			return flags, errors.New("review does not accept mutation flags")
		}
	case "list":
		if len(positionals) != 0 {
			return flags, errors.New("list accepts no positional arguments")
		}
		if flags.id != "" || flags.collection != "" || flags.name != "" || flags.description != "" || flags.contentFile != "" || flags.minScope != "" || flags.rationale != "" || len(flags.operations)+len(flags.triggers)+len(flags.notFor) != 0 || flags.editor || flags.yes || flags.fullDiff || flags.idempotencyKey != "" || flags.proposalID != "" || flags.proposalDigest != "" || flags.baseVersion != "" {
			return flags, errors.New("list accepts only --state, --workspace, and --json")
		}
	case "create":
		if len(positionals) == 1 {
			if flags.id != "" && flags.id != positionals[0] {
				return flags, fmt.Errorf("positional skill ID %q conflicts with --id %q", positionals[0], flags.id)
			}
			flags.id = positionals[0]
		} else if len(positionals) > 1 {
			return flags, errors.New("create accepts at most one positional skill ID")
		}
		if flags.id == "" || flags.collection == "" || flags.name == "" || flags.description == "" {
			return flags, errors.New("create requires --id, --collection, --name, and --description")
		}
		if flags.proposalID != "" || flags.proposalDigest != "" || flags.baseVersion != "" {
			return flags, errors.New("create does not accept confirmation pins")
		}
		if flags.editor {
			return flags, errors.New("--editor is available only for skill edit")
		}
	case "edit":
		if len(positionals) == 1 {
			if flags.id != "" && flags.id != positionals[0] {
				return flags, fmt.Errorf("positional skill ID %q conflicts with --id %q", positionals[0], flags.id)
			}
			flags.id = positionals[0]
		} else if len(positionals) == 0 && flags.id != "" {
			// flags.id provided via --id
		} else {
			return flags, errors.New("edit requires exactly one skill ID")
		}
		if flags.proposalID != "" || flags.proposalDigest != "" || flags.baseVersion != "" {
			return flags, errors.New("edit does not accept confirmation pins")
		}
		if flags.editor && flags.contentFile != "" {
			return flags, errors.New("--editor and --content-file cannot be combined")
		}
		if !flags.editor && flags.contentFile == "" && flags.name == "" && flags.description == "" && flags.rationale == "" && len(flags.operations)+len(flags.triggers)+len(flags.notFor) == 0 && flags.minScope == "" {
			return flags, errors.New("edit requires changed fields, --content-file, or --editor")
		}
	case "activate", "deprecate", "archive", "show":
		if len(positionals) == 1 {
			if flags.id != "" && flags.id != positionals[0] {
				return flags, fmt.Errorf("positional skill ID %q conflicts with --id %q", positionals[0], flags.id)
			}
			flags.id = positionals[0]
		} else if len(positionals) == 0 && flags.id != "" {
			// flags.id provided via --id
		} else {
			return flags, fmt.Errorf("%s requires exactly one skill ID", subcommand)
		}
		if flags.collection != "" || flags.name != "" || flags.description != "" || flags.contentFile != "" || flags.minScope != "" || flags.rationale != "" || len(flags.operations)+len(flags.triggers)+len(flags.notFor) != 0 || flags.editor || flags.proposalID != "" || flags.proposalDigest != "" || flags.baseVersion != "" {
			return flags, fmt.Errorf("%s accepts only its skill ID and output, workspace, diff, or confirmation flags", subcommand)
		}
		if subcommand == "show" && (flags.yes || flags.fullDiff || flags.idempotencyKey != "") {
			return flags, errors.New("show does not accept mutation flags")
		}
	case "confirm":
		if len(positionals) == 1 {
			if flags.proposalID != "" && flags.proposalID != positionals[0] {
				return flags, fmt.Errorf("positional proposal ID %q conflicts with --proposal %q", positionals[0], flags.proposalID)
			}
			flags.proposalID = positionals[0]
		} else if len(positionals) > 1 {
			return flags, errors.New("confirm accepts at most one positional proposal ID")
		}
		if flags.proposalID == "" {
			return flags, errors.New("confirm requires a proposal ID")
		}
		if flags.id != "" || flags.collection != "" || flags.name != "" || flags.description != "" || flags.contentFile != "" || flags.minScope != "" || flags.rationale != "" || len(flags.operations)+len(flags.triggers)+len(flags.notFor) != 0 || flags.editor || flags.yes || flags.fullDiff || flags.idempotencyKey != "" {
			return flags, errors.New("confirm accepts only workspace, proposal pins, and output flags")
		}
	default:
		return flags, fmt.Errorf("unsupported skill subcommand %q", subcommand)
	}
	resolved, resErr := resolveWorkspace(flags.workspace)
	if resErr != nil {
		return flags, resErr
	}
	flags.workspace = resolved
	return flags, nil
}

// maxContentFileBytes bounds an input content file; it matches the largest
// resource the canonical validator accepts.
const maxContentFileBytes = 16 << 20

// readContentFile reads the user's own input file from any path (relative to the
// current directory or absolute). The skill's canonical file is still written
// only through the mutation service. A symbolic link or non-regular file is
// refused, and the read is size-bounded.
func readContentFile(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("read content file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("content file must be a regular file, not a symbolic link or directory")
	}
	handle, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read content file: %w", err)
	}
	defer handle.Close()
	opened, err := handle.Stat()
	if err != nil {
		return nil, fmt.Errorf("read content file: %w", err)
	}
	if !os.SameFile(info, opened) {
		return nil, errors.New("content file changed while it was being opened")
	}
	contents, err := io.ReadAll(io.LimitReader(handle, maxContentFileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read content file: %w", err)
	}
	if len(contents) > maxContentFileBytes {
		return nil, fmt.Errorf("content file is larger than %d MiB", maxContentFileBytes>>20)
	}
	return contents, nil
}

// mergeRouting overlays the routing flags the user passed onto the stored routing.
func mergeRouting(current skill.RoutingInput, flags skillFlags) *skill.RoutingInput {
	if len(flags.operations) > 0 {
		current.Operations = flags.operations
	}
	if len(flags.triggers) > 0 {
		current.Triggers = flags.triggers
	}
	if len(flags.notFor) > 0 {
		current.NotFor = flags.notFor
	}
	if flags.minScope != "" {
		current.MinScope = flags.minScope
	}
	return &current
}

func writeSkillPreview(stdout, stderr io.Writer, jsonOutput bool, preview app.SkillProposal) int {
	if jsonOutput {
		if err := writeJSON(stdout, preview); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	fmt.Fprintln(stdout, preview.Summary)
	fmt.Fprintf(stdout, "- %d added, %d modified, %d deleted file(s).\n", len(preview.Diff.Added), len(preview.Diff.Modified), len(preview.Diff.Deleted))
	pins := preview.Confirmation.Confirmation.Pins
	fmt.Fprintf(stdout, "- Proposal: %s\n- Digest: %s\n- Base version: %s\n", pins.ProposalID, pins.ProposalDigest, pins.BaseVersion)
	if preview.RoutingImpact != nil {
		fmt.Fprintf(stdout, "- Routing impact: %s\n", preview.RoutingImpact.Summary)
		for _, warning := range preview.RoutingImpact.Warnings {
			fmt.Fprintf(stdout, "  WARNING: %s\n", warning)
		}
	}
	if preview.FullDiff != "" {
		fmt.Fprintln(stdout, preview.FullDiff)
	}
	fmt.Fprintf(stdout, "No files changed. Confirm with:\n  skillhub skill confirm --proposal %s --proposal-digest %s --base-version %s\n  (or: skillhub skill confirm %s)\nor re-run with --yes to apply directly.\n", pins.ProposalID, pins.ProposalDigest, pins.BaseVersion, pins.ProposalID)
	return 0
}

func writeSkillMutation(stdout, stderr io.Writer, jsonOutput, verbose bool, result app.SkillMutationResult, workspacePath string) int {
	if jsonOutput {
		if err := writeJSON(stdout, result); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	} else if result.Error != nil {
		fmt.Fprintf(stderr, "ERROR: %s\nWHY: %s\nFIX: %s\n", result.Error.Render.Error, result.Error.Render.Why, result.Error.Render.Fix)
	} else {
		fmt.Fprintln(stdout, result.Summary)
		if verbose {
			fmt.Fprintf(stdout, "Operation: %s\nCatalog snapshot: %s\nGeneration: %s\nGit dirty: %t\n", result.OperationID, result.CatalogSnapshot, result.Generation, result.GitDirty)
		}
		if next := nextStepForSkill(result, workspacePath); next != "" {
			fmt.Fprintf(stdout, "Next: %s\n", next)
		}
	}
	if result.Status == app.StatusError {
		return 2
	}
	return 0
}

func nextStepForSkill(result app.SkillMutationResult, workspacePath string) string {
	summary := result.Summary
	switch {
	case strings.Contains(summary, "Draft skill") && strings.Contains(summary, "saved"):
		return fmt.Sprintf("edit the instructions with `skillhub skill edit %s --editor`, then activate with `skillhub skill activate %s --yes`.", result.SkillID, result.SkillID)
	case strings.Contains(summary, "active") || strings.Contains(summary, "activated"):
		return fmt.Sprintf("ask your agent to use it, or inspect with `skillhub skill show %s`. Commit with `git -C %s commit`.", result.SkillID, workspacePath)
	case strings.Contains(summary, "deprecated") || strings.Contains(summary, "archived"):
		return fmt.Sprintf("commit with `git -C %s commit`.", workspacePath)
	default:
		return fmt.Sprintf("commit with `git -C %s commit`.", workspacePath)
	}
}

func writeSkillError(stdout, stderr io.Writer, jsonOutput bool, err error) int {
	return writeSkillErrorFor("", stdout, stderr, jsonOutput, err)
}

// writeSkillErrorFor renders err with fix hints that name the skill being changed.
func writeSkillErrorFor(id string, stdout, stderr io.Writer, jsonOutput bool, err error) int {
	if errors.Is(err, context.Canceled) {
		writeStructuredSkillError(stdout, stderr, jsonOutput, app.NewOperationCancelledError())
		return 130
	}
	if errors.Is(err, skill.ErrSnapshotExpired) {
		writeStructuredSkillError(stdout, stderr, jsonOutput, app.NewSnapshotExpiredError(err.Error()))
		return 2
	}
	if errors.Is(err, skill.ErrResourceDigestMismatch) {
		writeStructuredSkillError(stdout, stderr, jsonOutput, app.NewResourceDigestMismatchError(err.Error()))
		return 2
	}
	why := err.Error()
	var missingErr *app.MissingActivationRequirementsError
	if errors.As(err, &missingErr) {
		var editFlags []string
		for _, m := range missingErr.Missing {
			switch {
			case strings.Contains(m, "trigger"):
				editFlags = append(editFlags, `--trigger "<when to use>"`)
			case strings.Contains(m, "not_for") || strings.Contains(m, "rationale"):
				editFlags = append(editFlags, `--not-for "<when not to use>"`)
			case strings.Contains(m, "min_scope"):
				editFlags = append(editFlags, `--min-scope single_step`)
			}
		}
		targetID := missingErr.SkillID
		if targetID == "" {
			targetID = id
		}
		fixCmd := fmt.Sprintf("skillhub skill edit %s %s --yes", targetID, strings.Join(editFlags, " "))
		why = fmt.Sprintf("skill %s requires: %s", targetID, strings.Join(missingErr.Missing, ", "))
		fix := fmt.Sprintf("Run `%s`, then retry `skillhub skill activate %s --yes`.", fixCmd, targetID)
		return writeInvalidRequest(stdout, stderr, jsonOutput, why, fix)
	}
	if errors.Is(err, skill.ErrNotFound) || why == "skill not found" {
		if id != "" {
			why = fmt.Sprintf("skill not found: %s", id)
		} else {
			why = "skill not found"
		}
	}
	return writeInvalidRequest(stdout, stderr, jsonOutput, why, skillErrorFix(err, id))
}

// skillErrorFix turns known validation failures into the exact command that
// resolves them; other errors keep the generic hint.
func skillErrorFix(err error, id string) string {
	if id == "" {
		id = "<id>"
	}
	message := err.Error()
	switch {
	case errors.Is(err, skill.ErrNotFound) || message == "skill not found":
		return "Run `skillhub skill list` to inspect available skills."
	case strings.Contains(message, "routing.not_for") || strings.Contains(message, "routing_review_rationale"):
		return "Add `--not-for \"<when not to use>\"`: run `skillhub skill edit " + id + " --not-for \"<when not to use>\" --yes`, then retry. If nothing applies, record why with `skillhub skill edit " + id + " --rationale \"<reason>\" --yes`."
	case strings.Contains(message, "routing.min_scope"):
		return "Add `--min-scope <single_step|multi_step|project>`: run `skillhub skill edit " + id + " --min-scope single_step --yes`, then retry."
	case strings.Contains(message, "at least one routing trigger"):
		return "Run `skillhub skill edit " + id + " --trigger \"<when to use>\" --yes`, then retry."
	}
	return "Correct the skill fields or workspace state and retry."
}

func writeStructuredSkillError(stdout, stderr io.Writer, jsonOutput bool, structured *app.Error) {
	result := app.ErrorResult(structured)
	if jsonOutput {
		if err := writeJSON(stdout, result); err != nil {
			fmt.Fprintln(stderr, err)
		}
		return
	}
	fmt.Fprintf(stderr, "ERROR: %s\nWHY: %s\nFIX: %s\n", structured.Render.Error, structured.Render.Why, structured.Render.Fix)
}

func runSkillList(ctx context.Context, service app.SkillService, flags skillFlags, stdout, stderr io.Writer) int {
	result, err := service.ListSkills(ctx, flags.workspace, flags.state)
	if err != nil {
		return writeSkillError(stdout, stderr, flags.jsonOutput, err)
	}
	if flags.jsonOutput {
		if err := writeJSON(stdout, result); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	if len(result.Skills) == 0 {
		fmt.Fprintln(stdout, "No skills found.")
		return 0
	}
	idWidth, stateWidth, collectionWidth := len("ID"), len("STATE"), len("COLLECTION")
	for _, entry := range result.Skills {
		idWidth = max(idWidth, len(entry.ID))
		stateWidth = max(stateWidth, len(entry.State))
		collectionWidth = max(collectionWidth, len(entry.Collection))
	}
	fmt.Fprintf(stdout, "%-*s  %-*s  %-*s  %s\n", idWidth, "ID", stateWidth, "STATE", collectionWidth, "COLLECTION", "NAME")
	for _, entry := range result.Skills {
		fmt.Fprintf(stdout, "%-*s  %-*s  %-*s  %s\n", idWidth, entry.ID, stateWidth, entry.State, collectionWidth, entry.Collection, entry.Name)
	}
	return 0
}
