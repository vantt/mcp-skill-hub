package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/app"
	"github.com/vantt/skillhub-mcp/internal/delivery/cli/termui"
	"github.com/vantt/skillhub-mcp/internal/shellquote"
	"github.com/vantt/skillhub-mcp/internal/skill"
	"gopkg.in/yaml.v3"
)

type skillFlags struct {
	workspace, id, collection, name, description, contentFile, minScope, rationale string
	fromSuggestion                                                                 string
	proposalID, proposalDigest, baseVersion, idempotencyKey, state                 string
	locator, ref, subPath, approveContent, runtimeFile, target, writeConflicts     string
	operations, triggers, notFor, examples, counterExamples, skills, positionals   []string
	accepts, manuals                                                               []string
	jsonOutput, yes, diff, editor, verbose, all, check, noCheck, exitCode          bool
	noCommit, editRouting, discard                                                 bool
	// remote is the hub on another machine, set for the commands that work
	// from a project connected to one: commit and checkout.
	remote *app.RemoteHub
	// rerun is the command the person typed, for the hint lines of a preview.
	rerun string
}

// skillSubcommands lists the skill subcommands in the order `skillhub help
// skill` shows them; every "which subcommand" hint is built from it.
var skillSubcommands = []string{"add", "create", "checkout", "commit", "show", "review", "doctor", "env", "edit", "outdated", "upstream", "update", "activate", "deprecate", "archive", "confirm", "list"}

var skillSubcommandFix = "Run `skillhub skill " + strings.Join(skillSubcommands, "|") + "`."

// hubChangingSkillSubcommands lists the skill subcommands that write to the hub.
var hubChangingSkillSubcommands = map[string]bool{
	"add": true, "commit": true, "edit": true, "update": true, "activate": true, "deprecate": true, "archive": true, "confirm": true,
}

func runSkill(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), "skill requires a subcommand", skillSubcommandFix)
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
	if subcommand == "create" {
		return runSkillCreate(ctx, flags, stdout, stderr)
	}
	if subcommand == "doctor" {
		return runSkillDoctor(ctx, flags, stdout, stderr)
	}
	if subcommand == "env" {
		return runSkillEnv(ctx, flags, stdout, stderr)
	}
	service := app.SkillService{}
	if subcommand == "add" {
		return runSkillAdd(ctx, service, flags, stdout, stderr)
	}
	if subcommand == "checkout" {
		if flags.remote != nil {
			return runRemoteCheckout(ctx, flags, stdout, stderr)
		}
		return runSkillCheckout(ctx, flags, stdout, stderr)
	}
	if subcommand == "commit" {
		if flags.remote != nil {
			return runRemoteCommit(ctx, flags, stdout, stderr)
		}
		return runSkillCommit(ctx, flags, stdout, stderr)
	}
	if subcommand == "outdated" {
		return runSkillOutdated(ctx, flags, stdout, stderr)
	}
	if subcommand == "upstream" {
		return runSkillUpstream(ctx, flags, stdout, stderr)
	}
	if subcommand == "update" {
		return runSkillUpdate(ctx, flags, stdout, stderr)
	}
	if subcommand == "review" {
		if _, isSubmission := app.SubmissionIDOf(flags.id); isSubmission {
			return runSubmissionReview(ctx, flags, stdout, stderr)
		}
		result, err := service.ReviewSkill(ctx, flags.workspace, flags.id)
		if err != nil {
			return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
		}
		code := writeSkillReview(stdout, stderr, flags.jsonOutput, flags.verbose, result)
		if code == 0 && !flags.jsonOutput {
			writeWaitingSubmissions(stdout, flags.workspace, flags.id)
		}
		return code
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
				p := termui.New(stderr)
				p.Line(err.Error())
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
			p := termui.New(stdout)
			var fields []termui.Field
			fields = append(fields,
				termui.Field{Label: "Skill", Value: fmt.Sprintf("%s (%s)", result.Manifest.Name, flags.id)},
				termui.Field{Label: "State", Value: result.Manifest.Status},
			)
			if filePath != "" {
				fields = append(fields, termui.Field{Label: "File", Value: filePath})
			}
			fields = append(fields,
				termui.Field{Label: "Triggers", Value: triggers},
				termui.Field{Label: "Not for", Value: notFor},
				termui.Field{Label: "Min scope", Value: minScope},
			)
			if flags.verbose {
				fields = append(fields, termui.Field{Label: "Catalog snapshot", Value: result.Manifest.CatalogSnapshot})
			}
			p.Fields(fields...)
			p.Blank()
			p.Raw(result.Content)
			if !strings.HasSuffix(result.Content, "\n") {
				p.Raw("\n")
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
		res, err := service.DispatchConfirmProposal(commitContext(ctx, flags.noCommit, stdout), flags.workspace, flags.proposalID, pins)
		if err != nil {
			return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
		}
		_ = cleanupProposalRecovery(flags.workspace, flags.proposalID)
		switch r := res.(type) {
		case app.SkillMutationResult:
			return writeSkillMutation(stdout, stderr, flags.jsonOutput, flags.verbose, r, nextStepForSkill(ctx, service, r, flags.workspace))
		case app.SkillAddResult:
			return writeResult(stdout, stderr, flags.jsonOutput, r, func(p *termui.Printer) {
				writeSkillAddResult(p, flags.verbose, r)
			})
		case app.UpstreamUpdateResult:
			return writeResult(stdout, stderr, flags.jsonOutput, r, func(p *termui.Printer) {
				p.Line(r.Summary)
				if r.TrustImpact.ReviewRequiredAfterApply {
					p.Next("review the updated skill:", "skillhub skill review "+shellquote.Arg(r.SkillID))
				}
			})
		default:
			if flags.jsonOutput {
				if err := writeJSON(stdout, res); err != nil {
					p := termui.New(stderr)
					p.Line(err.Error())
					return 1
				}
				return 0
			}
			p := termui.New(stdout)
			p.Line(fmt.Sprintf("Proposal %s confirmed.", flags.proposalID))
			return 0
		}
	}

	var preview app.SkillProposal
	var suggestion *app.RoutingSuggestion
	var recoveryID string
	switch subcommand {
	case "create":
		content, err := readContentFile(flags.contentFile)
		if err != nil {
			return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
		}
		preview, err = service.PreviewCreate(ctx, flags.workspace, skill.CreateInput{
			ID: flags.id, IdempotencyKey: flags.idempotencyKey, Collection: flags.collection, Name: flags.name, Description: flags.description, Content: content,
			Routing:   skill.RoutingInput{Operations: flags.operations, Triggers: flags.triggers, NotFor: flags.notFor, MinScope: flags.minScope, Examples: flags.examples, CounterExamples: flags.counterExamples},
			Rationale: flags.rationale,
		}, flags.diff)
		if err != nil {
			return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
		}
	case "edit":
		if flags.fromSuggestion != "" {
			var code int
			if preview, suggestion, code = previewSuggestionEdit(ctx, service, flags, stdout, stderr); code != 0 {
				return code
			}
			break
		}
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
		if flags.approveContent != "" {
			update.ContentReviewedDigest = &flags.approveContent
		}
		if flags.runtimeFile != "" {
			block, readErr := readRuntimeFile(flags.runtimeFile)
			if readErr != nil {
				return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, readErr)
			}
			update.Runtime = block
		}
		if flags.hasRoutingFlags() {
			current, err := service.ReadSkillRouting(ctx, flags.workspace, flags.id)
			if err != nil {
				return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
			}
			update.Routing = mergeRouting(current, flags)
		}
		fullDiff := flags.diff || flags.editor
		preview, err = service.PreviewSkillUpdate(ctx, flags.workspace, flags.id, update, fullDiff)
		if err != nil {
			if recoveryID != "" && !flags.jsonOutput {
				p := termui.New(stderr)
				p.Line(fmt.Sprintf("Edited content saved to %s", recPath))
			}
			return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
		}
		if preview.Error != nil {
			if recoveryID != "" && !flags.jsonOutput {
				p := termui.New(stderr)
				p.Line(fmt.Sprintf("Edited content saved to %s", recPath))
			}
			if flags.jsonOutput {
				_ = writeJSON(stdout, preview)
				return 2
			}
			p := termui.New(stderr)
			p.Error(preview.Error.Render.Error, preview.Error.Render.Why, preview.Error.Render.Fix)
			return 2
		}
		if recoveryID != "" {
			_, _ = service.SaveEditorRecovery(ctx, flags.workspace, preview.Confirmation.Confirmation.Pins.ProposalID, content)
		}
	case "activate", "deprecate", "archive":
		preview, err = service.PreviewTransitionWithKey(ctx, flags.workspace, flags.id, subcommandTarget(subcommand), flags.diff, flags.idempotencyKey)
	default:
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, fmt.Sprintf("unsupported skill subcommand %q", subcommand), skillSubcommandFix)
	}
	if err != nil {
		return writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
	}
	if !flags.yes {
		apply, code := confirmInPlace(stdout, flags.jsonOutput, preview.Error == nil, func(interactive bool) int {
			if flags.editor && !flags.jsonOutput {
				writeEditorSkillPreview(stdout, preview, flags.rerun, interactive)
				return 0
			}
			if suggestion != nil {
				return writeSuggestionPreview(stdout, stderr, flags, preview, *suggestion, interactive)
			}
			return writeSkillPreview(stdout, stderr, flags.jsonOutput, preview, previewHints{rerun: flags.rerun, diffShown: flags.diff, interactive: interactive})
		})
		if !apply {
			return code
		}
	}
	if !flags.jsonOutput {
		p := termui.New(stdout)
		if preview.RoutingImpact != nil {
			for _, warning := range preview.RoutingImpact.Warnings {
				p.Warning(warning)
			}
		}
		for _, warning := range preview.Warnings {
			p.Warning(warning.Summary)
		}
	}
	result, err := service.ConfirmSkillMutation(commitContext(ctx, flags.noCommit, stdout), flags.workspace, preview, preview.Confirmation.Confirmation.Pins)
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
	return writeSkillMutation(stdout, stderr, flags.jsonOutput, flags.verbose, result, nextStepForSkill(ctx, service, result, flags.workspace))
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
		case "--workspace", "--id", "--collection", "--name", "--description", "--content-file", "--min-scope", "--rationale", "--operation", "--trigger", "--not-for", "--example", "--counter-example", "--approve-content", "--runtime-file", "--from-suggestion", "--proposal", "--proposal-digest", "--base-version", "--idempotency-key", "--state", "--ref", "--path", "--skill", "--target", "--write-conflicts", "--accept", "--manual":
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
			case "--from-suggestion":
				flags.fromSuggestion = item
			case "--rationale":
				flags.rationale = item
			case "--operation":
				flags.operations = append(flags.operations, item)
			case "--trigger":
				flags.triggers = append(flags.triggers, item)
			case "--not-for":
				flags.notFor = append(flags.notFor, item)
			case "--example":
				flags.examples = append(flags.examples, item)
			case "--counter-example":
				flags.counterExamples = append(flags.counterExamples, item)
			case "--approve-content":
				flags.approveContent = item
			case "--runtime-file":
				flags.runtimeFile = item
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
			case "--target":
				flags.target = item
			case "--write-conflicts":
				flags.writeConflicts = item
			case "--accept":
				flags.accepts = append(flags.accepts, item)
			case "--manual":
				flags.manuals = append(flags.manuals, item)
			}
		case "--all":
			flags.all = true
		case "--json":
			flags.jsonOutput = true
		case "--yes":
			flags.yes = true
		case "--diff":
			flags.diff = true
		case "--no-commit":
			flags.noCommit = true
		case "--edit-routing":
			flags.editRouting = true
		case "--discard":
			flags.discard = true
		case "--editor":
			flags.editor = true
		case "--verbose":
			flags.verbose = true
		case "--check":
			flags.check = true
		case "--no-check":
			flags.noCheck = true
		case "--exit-code":
			flags.exitCode = true
		default:
			if strings.HasPrefix(value, "-") {
				return flags, fmt.Errorf("unknown argument %q", value)
			}
			positionals = append(positionals, value)
		}
	}
	flags.positionals = positionals
	flags.rerun = rerunCommand("skillhub skill", append([]string{subcommand}, args...))
	if flags.noCommit && !hubChangingSkillSubcommands[subcommand] {
		return flags, fmt.Errorf("--no-commit is available only for commands that change the hub")
	}
	if flags.discard && subcommand != "review" {
		return flags, errors.New("--discard is available only for skill review of a submission")
	}
	if flags.state != "" && subcommand != "list" {
		return flags, errors.New("--state is available only for skill list")
	}
	if flags.runtimeFile != "" && subcommand != "edit" && subcommand != "commit" {
		return flags, errors.New("--runtime-file is available only for skill edit and skill commit")
	}
	if flags.fromSuggestion != "" && subcommand != "edit" {
		return flags, errors.New("--from-suggestion is available only for skill edit")
	}
	switch subcommand {
	case "outdated":
		if len(positionals) != 0 {
			return flags, errors.New("outdated accepts no positional arguments")
		}
	case "upstream":
		if len(positionals) == 1 {
			if flags.id != "" && flags.id != positionals[0] {
				return flags, fmt.Errorf("positional skill ID %q conflicts with --id %q", positionals[0], flags.id)
			}
			flags.id = positionals[0]
		} else if len(positionals) == 0 && flags.id != "" {
			// flags.id provided via --id
		} else {
			return flags, errors.New("upstream requires exactly one skill ID")
		}
	case "update":
		if len(positionals) == 1 {
			if flags.id != "" && flags.id != positionals[0] {
				return flags, fmt.Errorf("positional skill ID %q conflicts with --id %q", positionals[0], flags.id)
			}
			flags.id = positionals[0]
		} else if len(positionals) == 0 && flags.id != "" {
			// flags.id provided via --id
		} else {
			return flags, errors.New("update requires a skill ID (e.g. `skillhub skill update <id>`). To update the skillhub binary itself, run `skillhub update`")
		}
		for _, acc := range flags.accepts {
			parts := strings.SplitN(acc, "=", 2)
			if len(parts) != 2 || (parts[1] != "upstream" && parts[1] != "local" && parts[1] != "merged") {
				val := acc
				if len(parts) == 2 {
					val = parts[1]
				}
				return flags, fmt.Errorf("invalid --accept value %q: allowed values are upstream, local, or merged", val)
			}
		}
		for _, man := range flags.manuals {
			parts := strings.SplitN(man, "=", 2)
			if len(parts) != 2 {
				return flags, fmt.Errorf("invalid --manual flag %q: must be <path>=<file>", man)
			}
			filePath := strings.TrimSpace(parts[1])
			if _, statErr := os.Stat(filePath); statErr != nil {
				return flags, fmt.Errorf("manual file %q does not exist", filePath)
			}
		}
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
		if _, isSubmission := app.SubmissionIDOf(flags.id); isSubmission {
			// A submission is applied only by a person at a terminal, so there
			// is no --yes. --discard drops it.
			if flags.yes || flags.idempotencyKey != "" || flags.editor || flags.contentFile != "" || flags.proposalID != "" || flags.approveContent != "" || flags.collection != "" || flags.runtimeFile != "" {
				return flags, errors.New("a submission is approved by a person at a terminal; review accepts only --diff, --discard, --accept, --manual, --write-conflicts, --edit-routing and the routing flags for it")
			}
			if err := validateResolutionFlags(&flags); err != nil {
				return flags, err
			}
			if flags.discard && (flags.diff || flags.editRouting || len(flags.accepts)+len(flags.manuals) > 0 || flags.writeConflicts != "") {
				return flags, errors.New("--discard cannot be combined with other review flags")
			}
		} else if flags.yes || flags.diff || flags.idempotencyKey != "" || flags.editor || flags.contentFile != "" || flags.proposalID != "" || flags.discard || flags.editRouting || len(flags.accepts)+len(flags.manuals) > 0 || flags.writeConflicts != "" {
			return flags, errors.New("review does not accept mutation flags")
		}
	case "doctor":
		if len(positionals) == 1 {
			if flags.id != "" && flags.id != positionals[0] {
				return flags, fmt.Errorf("positional skill ID %q conflicts with --id %q", positionals[0], flags.id)
			}
			flags.id = positionals[0]
		} else if len(positionals) != 0 || flags.id == "" {
			return flags, errors.New("doctor requires exactly one skill ID")
		}
		if flags.collection != "" || flags.name != "" || flags.description != "" || flags.contentFile != "" || flags.minScope != "" || flags.rationale != "" || flags.hasRoutingFlags() || flags.approveContent != "" || flags.editor || flags.yes || flags.diff || flags.idempotencyKey != "" || flags.proposalID != "" || flags.proposalDigest != "" || flags.baseVersion != "" || flags.all || len(flags.skills) != 0 || flags.ref != "" || flags.subPath != "" {
			return flags, errors.New("doctor accepts only its skill ID, --workspace, and --json")
		}
	case "env":
		if err := parseSkillEnvArgs(&flags, positionals); err != nil {
			return flags, err
		}
	case "list":
		if len(positionals) != 0 {
			return flags, errors.New("list accepts no positional arguments")
		}
		if flags.id != "" || flags.collection != "" || flags.name != "" || flags.description != "" || flags.contentFile != "" || flags.minScope != "" || flags.rationale != "" || flags.hasRoutingFlags() || flags.approveContent != "" || flags.editor || flags.yes || flags.diff || flags.idempotencyKey != "" || flags.proposalID != "" || flags.proposalDigest != "" || flags.baseVersion != "" {
			return flags, errors.New("list accepts only --state, --workspace, and --json")
		}
	case "create":
		if err := parseSkillCreateFlags(&flags, positionals); err != nil {
			return flags, err
		}
	case "checkout":
		if err := parseSkillCheckoutFlags(&flags, positionals); err != nil {
			return flags, err
		}
	case "commit":
		if err := parseSkillCommitFlags(&flags, positionals); err != nil {
			return flags, err
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
		if flags.fromSuggestion != "" {
			if flags.editor || flags.contentFile != "" || flags.name != "" || flags.description != "" || flags.rationale != "" || flags.hasRoutingFlags() || flags.approveContent != "" || flags.runtimeFile != "" {
				return flags, errors.New("--from-suggestion adds the suggested text as it is, so it cannot be combined with --example, --counter-example, --not-for, --trigger, --operation, --min-scope, --description, --name, --rationale, --approve-content, --content-file, --runtime-file or --editor")
			}
		} else if !flags.editor && flags.contentFile == "" && flags.name == "" && flags.description == "" && flags.rationale == "" && !flags.hasRoutingFlags() && flags.approveContent == "" && flags.runtimeFile == "" {
			return flags, errors.New("edit requires changed fields, --content-file, --runtime-file, --editor, or --from-suggestion")
		}
		if flags.approveContent != "" && !contentDigestPattern.MatchString(flags.approveContent) {
			return flags, errors.New("--approve-content requires a digest of the form sha256:<64 lowercase hex characters>")
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
		if flags.collection != "" || flags.name != "" || flags.description != "" || flags.contentFile != "" || flags.minScope != "" || flags.rationale != "" || flags.hasRoutingFlags() || flags.approveContent != "" || flags.editor || flags.proposalID != "" || flags.proposalDigest != "" || flags.baseVersion != "" {
			return flags, fmt.Errorf("%s accepts only its skill ID and output, workspace, diff, or confirmation flags", subcommand)
		}
		if subcommand == "show" && (flags.yes || flags.diff || flags.idempotencyKey != "") {
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
		if flags.id != "" || flags.collection != "" || flags.name != "" || flags.description != "" || flags.contentFile != "" || flags.minScope != "" || flags.rationale != "" || flags.hasRoutingFlags() || flags.approveContent != "" || flags.editor || flags.diff || flags.idempotencyKey != "" {
			return flags, errors.New("confirm accepts only workspace, proposal pins, --yes, and output flags")
		}
	default:
		return flags, fmt.Errorf("unsupported skill subcommand %q", subcommand)
	}
	if subcommand == "create" {
		// A create writes in the project only, so it needs no hub.
		return flags, nil
	}
	resolved, resErr := resolveWorkspace(flags.workspace)
	if resErr != nil {
		// A project connected to a hub on another machine can still check a
		// skill out and send one back for review there.
		if subcommand == "commit" || subcommand == "checkout" {
			if hub, remote := remoteHubFor(flags.workspace); remote {
				flags.remote = &hub
				return flags, nil
			}
		}
		return flags, resErr
	}
	flags.workspace = resolved
	return flags, nil
}

// readRuntimeFile reads a YAML runtime block. An empty mapping ({}) is returned
// as an empty non-nil map, which removes the block on edit.
func readRuntimeFile(path string) (map[string]any, error) {
	contents, err := readContentFile(path)
	if err != nil {
		return nil, err
	}
	var block map[string]any
	if err := yaml.Unmarshal(contents, &block); err != nil {
		return nil, fmt.Errorf("runtime file is not valid YAML: %w", err)
	}
	if block == nil {
		return nil, errors.New("runtime file must contain a YAML mapping; use {} to remove the runtime block")
	}
	return block, nil
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

// contentDigestPattern matches the content digest a human approves with
// --approve-content; it mirrors quality.content_reviewed_digest validation.
var contentDigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// hasRoutingFlags reports whether any routing field flag was supplied.
func (flags skillFlags) hasRoutingFlags() bool {
	return len(flags.operations)+len(flags.triggers)+len(flags.notFor)+len(flags.examples)+len(flags.counterExamples) > 0 || flags.minScope != ""
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
	if len(flags.examples) > 0 {
		current.Examples = flags.examples
	}
	if len(flags.counterExamples) > 0 {
		current.CounterExamples = flags.counterExamples
	}
	return &current
}

func writeSkillPreview(stdout, stderr io.Writer, jsonOutput bool, preview app.SkillProposal, hints previewHints) int {
	return writeResult(stdout, stderr, jsonOutput, preview, func(p *termui.Printer) {
		p.Line(preview.Summary)
		p.Bullets(fmt.Sprintf("%d added, %d modified, %d deleted file(s).", len(preview.Diff.Added), len(preview.Diff.Modified), len(preview.Diff.Deleted)))
		pins := preview.Confirmation.Confirmation.Pins
		if !hints.interactive {
			p.Fields(
				termui.Field{Label: "Proposal", Value: pins.ProposalID},
				termui.Field{Label: "Digest", Value: pins.ProposalDigest},
				termui.Field{Label: "Base version", Value: pins.BaseVersion},
			)
		}
		if preview.RoutingImpact != nil {
			p.Fields(termui.Field{Label: "Routing impact", Value: preview.RoutingImpact.Summary})
			for _, warning := range preview.RoutingImpact.Warnings {
				p.Warning(warning)
			}
		}
		for _, warning := range preview.Warnings {
			p.Warning(warning.Summary)
		}
		if preview.FullDiff != "" {
			p.Raw(preview.FullDiff)
			if !strings.HasSuffix(preview.FullDiff, "\n") {
				p.Raw("\n")
			}
		}
		previewTail(p, hints.rerun, hints.diffShown, hints.interactive)
	})
}

// skillConfirmCommand renders the confirm command with every pin the
// application layer requires, so a printed next step can be run as shown.
func skillConfirmCommand(pins app.ConfirmationPins) string {
	return fmt.Sprintf("skillhub skill confirm --proposal %s --proposal-digest %s --base-version %s", shellquote.Arg(pins.ProposalID), shellquote.Arg(pins.ProposalDigest), shellquote.Arg(pins.BaseVersion))
}

func writeSkillMutation(stdout, stderr io.Writer, jsonOutput, verbose bool, result app.SkillMutationResult, next string) int {
	return writeResult(stdout, stderr, jsonOutput, result, func(p *termui.Printer) {
		p.Line(result.Summary)
		writeCommitLines(p, result.Commit)
		if verbose {
			p.Fields(
				termui.Field{Label: "Operation", Value: result.OperationID},
				termui.Field{Label: "Catalog snapshot", Value: result.CatalogSnapshot},
				termui.Field{Label: "Generation", Value: result.Generation},
				termui.Field{Label: "Git dirty", Value: fmt.Sprintf("%t", result.GitDirty)},
			)
		}
		if next != "" {
			p.Next(next, "")
		}
	})
}

// nextStepForSkill names what to do after a skill change. A draft gets the
// step the review computes for its current state; every other state ends with
// the commit command `skillhub status` prints for the same hub, unless the
// change was committed already or the commit line named the manual command.
func nextStepForSkill(ctx context.Context, service app.SkillService, result app.SkillMutationResult, workspacePath string) string {
	root := workspacePath
	if resolved, err := skill.ResolveWorkspace(workspacePath); err == nil {
		root = resolved
	}
	commit := "`" + app.GitCommitCommand(root, result.ChangedPaths) + "`"
	summary := result.Summary
	switch {
	case strings.Contains(summary, "Draft skill") && strings.Contains(summary, "saved"):
		if review, err := service.ReviewSkill(ctx, workspacePath, result.SkillID); err == nil {
			return review.NextAction
		}
		return fmt.Sprintf("preview activation with `skillhub skill activate %s`.", result.SkillID)
	case strings.Contains(summary, "active") || strings.Contains(summary, "activated"):
		if result.Commit != nil {
			return fmt.Sprintf("ask your agent to use it, or inspect with `skillhub skill show %s`.", result.SkillID)
		}
		return fmt.Sprintf("ask your agent to use it, or inspect with `skillhub skill show %s`. Commit with %s.", result.SkillID, commit)
	case result.Commit != nil:
		return ""
	default:
		return fmt.Sprintf("commit with %s.", commit)
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
	if refusal := app.UncommittedEditsError(err); refusal != nil {
		writeStructuredSkillError(stdout, stderr, jsonOutput, refusal)
		return 2
	}
	if structured := (*app.Error)(nil); errors.As(err, &structured) && structured.Render.Error != "" {
		// The service already said what happened, why and what to run.
		writeStructuredSkillError(stdout, stderr, jsonOutput, structured)
		return 2
	}
	why := err.Error()
	var missingErr *app.MissingActivationRequirementsError
	if errors.As(err, &missingErr) {
		var editFlags []string
		needsContent := false
		for _, m := range missingErr.Missing {
			switch {
			case strings.Contains(m, "trigger"):
				editFlags = append(editFlags, `--trigger "<when to use>"`)
			case strings.Contains(m, "not_for") || strings.Contains(m, "rationale"):
				editFlags = append(editFlags, `--not-for "<when not to use>"`)
			case strings.Contains(m, "min_scope"):
				editFlags = append(editFlags, `--min-scope single_step`)
			case strings.Contains(m, "content"):
				needsContent = true
			}
		}
		targetID := missingErr.SkillID
		if targetID == "" {
			targetID = id
		}
		why = fmt.Sprintf("skill %s requires: %s", targetID, strings.Join(missingErr.Missing, ", "))
		var fixes []string
		if len(editFlags) > 0 {
			fixes = append(fixes, fmt.Sprintf("`skillhub skill edit %s %s`", targetID, strings.Join(editFlags, " ")))
		}
		if needsContent {
			fixes = append(fixes, fmt.Sprintf("`skillhub skill edit %s --editor` (or `--content-file <file>`) to replace the untouched scaffold instructions", targetID))
		}
		if len(fixes) == 0 {
			fixes = append(fixes, fmt.Sprintf("`skillhub skill review %s`", targetID))
		}
		fix := fmt.Sprintf("Preview the fix with %s, review and confirm it, then preview `skillhub skill activate %s`.", strings.Join(fixes, " and "), targetID)
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
		return "Preview `skillhub skill edit " + id + " --not-for \"<when not to use>\"`, then review and confirm before retrying. If nothing applies, preview `skillhub skill edit " + id + " --rationale \"<reason>\"`."
	case strings.Contains(message, "routing.min_scope"):
		return "Preview `skillhub skill edit " + id + " --min-scope single_step`, then review and confirm before retrying."
	case strings.Contains(message, "at least one routing trigger"):
		return "Preview `skillhub skill edit " + id + " --trigger \"<when to use>\"`, then review and confirm before retrying."
	}
	return "Correct the skill fields or workspace state and retry."
}

func writeStructuredSkillError(stdout, stderr io.Writer, jsonOutput bool, structured *app.Error) {
	result := app.ErrorResult(structured)
	if jsonOutput {
		if err := writeJSON(stdout, result); err != nil {
			p := termui.New(stderr)
			p.Line(err.Error())
		}
		return
	}
	p := termui.New(stderr)
	p.Error(structured.Render.Error, structured.Render.Why, structured.Render.Fix)
}

func runSkillList(ctx context.Context, service app.SkillService, flags skillFlags, stdout, stderr io.Writer) int {
	result, err := service.ListSkills(ctx, flags.workspace, flags.state)
	if err != nil {
		return writeSkillError(stdout, stderr, flags.jsonOutput, err)
	}
	return writeResult(stdout, stderr, flags.jsonOutput, result, func(p *termui.Printer) {
		if len(result.Skills) == 0 {
			p.Line("No skills found.")
			return
		}
		headers := []string{"ID", "STATE", "COLLECTION", "NAME"}
		rows := make([][]string, 0, len(result.Skills))
		for _, entry := range result.Skills {
			rows = append(rows, []string{entry.ID, entry.State, entry.Collection, entry.Name})
		}
		p.Table(headers, rows)
	})
}

// previewSuggestionEdit stores the edit preview of a suggestion. It returns a
// non-zero exit code when the command is over.
func previewSuggestionEdit(ctx context.Context, service app.SkillService, flags skillFlags, stdout, stderr io.Writer) (app.SkillProposal, *app.RoutingSuggestion, int) {
	proposal, err := service.PreviewRoutingSuggestion(ctx, flags.workspace, flags.fromSuggestion)
	if err != nil {
		return app.SkillProposal{}, nil, writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, err)
	}
	if proposal.Suggestion.SkillID != flags.id {
		mismatch := app.NewInvalidRequestError(
			fmt.Sprintf("Suggestion %s is for skill %s, not %s.", flags.fromSuggestion, proposal.Suggestion.SkillID, flags.id),
			"Run `skillhub skill edit "+shellquote.Arg(proposal.Suggestion.SkillID)+" --from-suggestion "+shellquote.Arg(flags.fromSuggestion)+"`.")
		return app.SkillProposal{}, nil, writeSkillErrorFor(flags.id, stdout, stderr, flags.jsonOutput, mismatch)
	}
	if proposal.Error != nil {
		if flags.jsonOutput {
			_ = writeJSON(stdout, proposal)
			return app.SkillProposal{}, nil, 2
		}
		p := termui.New(stderr)
		p.Error(proposal.Error.Render.Error, proposal.Error.Render.Why, proposal.Error.Render.Fix)
		return app.SkillProposal{}, nil, 2
	}
	return proposal.SkillProposal, &proposal.Suggestion, 0
}

// writeSuggestionPreview prints a suggestion's reason and evidence before the
// ordinary edit preview. The preview always holds the full diff, so it is shown
// only when --diff asked for it. Phrasings are untrusted: the printer escapes
// them, and the only command printed holds IDs.
func writeSuggestionPreview(stdout, stderr io.Writer, flags skillFlags, preview app.SkillProposal, suggestion app.RoutingSuggestion, interactive bool) int {
	if flags.jsonOutput {
		return writeResult(stdout, stderr, true, app.RoutingSuggestionProposal{SkillProposal: preview, Suggestion: suggestion}, nil)
	}
	p := termui.New(stdout)
	p.Fields(
		termui.Field{Label: "Suggestion", Value: fmt.Sprintf("%s  %s  %s", suggestion.ID, suggestion.Field, strconv.Quote(suggestion.Text))},
		termui.Field{Label: "Why", Value: suggestion.Evidence.Reason},
		termui.Field{Label: "Evidence", Value: suggestionEvidenceLine(suggestion.Evidence)},
	)
	shown := preview
	if !flags.diff {
		shown.FullDiff = ""
	}
	code := writeSkillPreview(stdout, stderr, false, shown, previewHints{rerun: flags.rerun, diffShown: flags.diff, interactive: interactive})
	if code == 0 && !interactive {
		termui.New(stdout).Raw("Or apply this proposal later: " + skillConfirmCommand(preview.Confirmation.Confirmation.Pins) + "\n")
	}
	return code
}
