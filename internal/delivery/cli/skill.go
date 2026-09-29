package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/skill"
)

type skillFlags struct {
	workspace, id, collection, name, description, contentFile, minScope, rationale string
	proposalID, proposalDigest, baseVersion, idempotencyKey                        string
	operations, triggers, notFor                                                   []string
	jsonOutput, yes, fullDiff, editor                                              bool
}

func runSkill(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), "skill requires a subcommand", "Run `skillhub skill create|edit|activate|deprecate|archive|show`.")
	}
	subcommand := args[0]
	flags, err := parseSkillFlags(subcommand, args[1:])
	if err != nil {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Review `skillhub skill "+subcommand+"` arguments and retry.")
	}
	service := app.SkillService{}
	if subcommand == "show" {
		result, err := service.ReadSkill(ctx, flags.workspace, flags.id)
		if err != nil {
			return writeSkillError(stdout, stderr, flags.jsonOutput, err)
		}
		if flags.jsonOutput {
			if err := writeJSON(stdout, result); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
		} else {
			fmt.Fprintf(stdout, "%s\nCatalog snapshot: %s\n\n%s", result.Summary, result.Manifest.CatalogSnapshot, result.Content)
		}
		return 0
	}
	if subcommand == "confirm" {
		preview, err := service.LoadSkillProposal(ctx, flags.workspace, flags.proposalID)
		if err != nil {
			return writeSkillError(stdout, stderr, flags.jsonOutput, err)
		}
		pins := app.ConfirmationPins{ProposalID: flags.proposalID, ProposalDigest: flags.proposalDigest, BaseVersion: flags.baseVersion}
		result, err := service.ConfirmSkillMutation(ctx, flags.workspace, preview, pins)
		if err != nil {
			return writeSkillError(stdout, stderr, flags.jsonOutput, err)
		}
		return writeSkillMutation(stdout, stderr, flags.jsonOutput, result)
	}

	var preview app.SkillProposal
	switch subcommand {
	case "create":
		content, err := readContentFile(flags.workspace, flags.contentFile)
		if err != nil {
			return writeSkillError(stdout, stderr, flags.jsonOutput, err)
		}
		preview, err = service.PreviewCreate(ctx, flags.workspace, skill.CreateInput{
			ID: flags.id, IdempotencyKey: flags.idempotencyKey, Collection: flags.collection, Name: flags.name, Description: flags.description, Content: content,
			Routing:   skill.RoutingInput{Operations: flags.operations, Triggers: flags.triggers, NotFor: flags.notFor, MinScope: flags.minScope},
			Rationale: flags.rationale,
		}, flags.fullDiff)
		if err != nil {
			return writeSkillError(stdout, stderr, flags.jsonOutput, err)
		}
	case "edit":
		content, setContent, err := editContent(ctx, service, flags)
		if err != nil {
			return writeSkillError(stdout, stderr, flags.jsonOutput, err)
		}
		update := skill.UpdateInput{IdempotencyKey: flags.idempotencyKey, Content: content, SetContent: setContent}
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
			update.Routing = &skill.RoutingInput{Operations: flags.operations, Triggers: flags.triggers, NotFor: flags.notFor, MinScope: flags.minScope}
		}
		preview, err = service.PreviewSkillUpdate(ctx, flags.workspace, flags.id, update, flags.fullDiff)
		if err != nil {
			return writeSkillError(stdout, stderr, flags.jsonOutput, err)
		}
	case "activate", "deprecate", "archive":
		preview, err = service.PreviewTransitionWithKey(ctx, flags.workspace, flags.id, subcommandTarget(subcommand), flags.fullDiff, flags.idempotencyKey)
	default:
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, fmt.Sprintf("unsupported skill subcommand %q", subcommand), "Run `skillhub skill create|edit|activate|deprecate|archive|show`.")
	}
	if err != nil {
		return writeSkillError(stdout, stderr, flags.jsonOutput, err)
	}
	if !flags.yes {
		return writeSkillPreview(stdout, stderr, flags.jsonOutput, preview)
	}
	if !flags.jsonOutput {
		if code := writeSkillPreview(stdout, stderr, false, preview); code != 0 {
			return code
		}
	}
	result, err := service.ConfirmSkillMutation(ctx, flags.workspace, preview, preview.Confirmation.Confirmation.Pins)
	if err != nil {
		return writeSkillError(stdout, stderr, flags.jsonOutput, err)
	}
	if flags.jsonOutput {
		result.Proposal = &preview
	}
	return writeSkillMutation(stdout, stderr, flags.jsonOutput, result)
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
		case "--workspace", "--id", "--collection", "--name", "--description", "--content-file", "--min-scope", "--rationale", "--operation", "--trigger", "--not-for", "--proposal", "--proposal-digest", "--base-version", "--idempotency-key":
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
			}
		case "--json":
			flags.jsonOutput = true
		case "--yes":
			flags.yes = true
		case "--full-diff":
			flags.fullDiff = true
		case "--editor":
			flags.editor = true
		default:
			if strings.HasPrefix(value, "-") {
				return flags, fmt.Errorf("unknown argument %q", value)
			}
			positionals = append(positionals, value)
		}
	}
	if flags.workspace == "" {
		return flags, errors.New("--workspace is required for non-interactive use")
	}
	switch subcommand {
	case "create":
		if len(positionals) != 0 {
			return flags, errors.New("create does not accept positional arguments")
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
		if len(positionals) != 1 {
			return flags, errors.New("edit requires exactly one skill ID")
		}
		if flags.id != "" || flags.collection != "" {
			return flags, errors.New("edit accepts the skill ID only as its positional argument")
		}
		if flags.proposalID != "" || flags.proposalDigest != "" || flags.baseVersion != "" {
			return flags, errors.New("edit does not accept confirmation pins")
		}
		flags.id = positionals[0]
		if flags.editor && flags.contentFile != "" {
			return flags, errors.New("--editor and --content-file cannot be combined")
		}
		if !flags.editor && flags.contentFile == "" && flags.name == "" && flags.description == "" && flags.rationale == "" && len(flags.operations)+len(flags.triggers)+len(flags.notFor) == 0 && flags.minScope == "" {
			return flags, errors.New("edit requires changed fields, --content-file, or --editor")
		}
	case "activate", "deprecate", "archive", "show":
		if len(positionals) != 1 {
			return flags, fmt.Errorf("%s requires exactly one skill ID", subcommand)
		}
		if flags.id != "" || flags.collection != "" || flags.name != "" || flags.description != "" || flags.contentFile != "" || flags.minScope != "" || flags.rationale != "" || len(flags.operations)+len(flags.triggers)+len(flags.notFor) != 0 || flags.editor || flags.proposalID != "" || flags.proposalDigest != "" || flags.baseVersion != "" {
			return flags, fmt.Errorf("%s accepts only its skill ID and output, workspace, diff, or confirmation flags", subcommand)
		}
		flags.id = positionals[0]
		if subcommand == "show" && (flags.yes || flags.fullDiff || flags.idempotencyKey != "") {
			return flags, errors.New("show does not accept mutation flags")
		}
	case "confirm":
		if len(positionals) != 0 || flags.proposalID == "" || flags.proposalDigest == "" || flags.baseVersion == "" {
			return flags, errors.New("confirm requires --proposal, --proposal-digest, and --base-version")
		}
		if flags.id != "" || flags.collection != "" || flags.name != "" || flags.description != "" || flags.contentFile != "" || flags.minScope != "" || flags.rationale != "" || len(flags.operations)+len(flags.triggers)+len(flags.notFor) != 0 || flags.editor || flags.yes || flags.fullDiff || flags.idempotencyKey != "" {
			return flags, errors.New("confirm accepts only workspace, proposal pins, and output flags")
		}
	default:
		return flags, fmt.Errorf("unsupported skill subcommand %q", subcommand)
	}
	return flags, nil
}

func readContentFile(workspacePath, path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
	if strings.Contains(path, `\\`) || strings.HasPrefix(path, "/") || path == "." || path == ".." || strings.HasPrefix(path, "../") {
		return nil, errors.New("--content-file must be a workspace-relative path")
	}
	root, err := skill.ResolveWorkspace(workspacePath)
	if err != nil {
		return nil, err
	}
	handle, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer handle.Close()
	info, err := handle.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("read content file: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, errors.New("content file must be a regular workspace file")
	}
	contents, err := handle.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read content file: %w", err)
	}
	return contents, nil
}

func editContent(ctx context.Context, service app.SkillService, flags skillFlags) ([]byte, bool, error) {
	if flags.contentFile != "" {
		contents, err := readContentFile(flags.workspace, flags.contentFile)
		return contents, true, err
	}
	if !flags.editor {
		return nil, false, nil
	}
	contents, err := service.ReadSkillContentForEdit(ctx, flags.workspace, flags.id)
	if err != nil {
		return nil, false, err
	}
	temporary, err := os.CreateTemp("", "skillhub-edit-*.md")
	if err != nil {
		return nil, false, err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0o600); err == nil {
		_, err = temporary.Write(contents)
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, false, err
	}
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	fields := strings.Fields(editor)
	if len(fields) == 0 {
		return nil, false, errors.New("$VISUAL or $EDITOR must be set for --editor")
	}
	command := exec.CommandContext(ctx, fields[0], append(fields[1:], name)...)
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stderr, os.Stderr
	if err := command.Run(); err != nil {
		return nil, false, fmt.Errorf("external editor failed: %w", err)
	}
	temporaryRoot, err := os.OpenRoot(filepath.Dir(name))
	if err != nil {
		return nil, false, err
	}
	defer temporaryRoot.Close()
	base := filepath.Base(name)
	info, err := temporaryRoot.Lstat(base)
	if err != nil {
		return nil, false, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, false, errors.New("external editor replaced the temporary file with an unsafe object")
	}
	updated, err := temporaryRoot.ReadFile(base)
	if err != nil {
		return nil, false, err
	}
	if bytes.Equal(contents, updated) {
		return nil, false, errors.New("external editor made no changes")
	}
	return updated, true, nil
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
	fmt.Fprintf(stdout, "- Proposal: %s\n- Digest: %s\n- Base catalog: %s\n", preview.Confirmation.Confirmation.Pins.ProposalID, preview.Confirmation.Confirmation.Pins.ProposalDigest, preview.Confirmation.Confirmation.Pins.BaseVersion)
	if preview.RoutingImpact != nil {
		fmt.Fprintf(stdout, "- Routing impact: %s\n", preview.RoutingImpact.Summary)
		for _, warning := range preview.RoutingImpact.Warnings {
			fmt.Fprintf(stdout, "  WARNING: %s\n", warning)
		}
	}
	if preview.FullDiff != "" {
		fmt.Fprintln(stdout, preview.FullDiff)
	}
	fmt.Fprintln(stdout, "No files changed. Confirm these exact pins with `skillhub skill confirm`; --yes confirms only the proposal shown in this invocation.")
	return 0
}

func writeSkillMutation(stdout, stderr io.Writer, jsonOutput bool, result app.SkillMutationResult) int {
	if jsonOutput {
		if err := writeJSON(stdout, result); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	} else if result.Error != nil {
		fmt.Fprintf(stderr, "ERROR: %s\nWHY: %s\nFIX: %s\n", result.Error.Render.Error, result.Error.Render.Why, result.Error.Render.Fix)
	} else {
		fmt.Fprintln(stdout, result.Summary)
		fmt.Fprintf(stdout, "Operation: %s\nCatalog snapshot: %s\nGeneration: %s\nGit dirty: %t\n", result.OperationID, result.CatalogSnapshot, result.Generation, result.GitDirty)
	}
	if result.Status == app.StatusError {
		return 2
	}
	return 0
}

func writeSkillError(stdout, stderr io.Writer, jsonOutput bool, err error) int {
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
	return writeInvalidRequest(stdout, stderr, jsonOutput, err.Error(), "Correct the skill fields or workspace state and retry.")
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
