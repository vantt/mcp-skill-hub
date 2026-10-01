package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
)

type sourceFlags struct {
	workspace, locator, reason, status, decision, sourceID, adapter, ref, sourcePath, license, trust, cadence, skillID string
	proposalID, proposalDigest, baseVersion, idempotencyKey                                                            string
	jsonOutput, monitoring, yes                                                                                        bool
	skills                                                                                                             []string
}

func runSource(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), "source requires a subcommand", "Run `skillhub source watch|check|capture|list|show|triage|confirm|import`.")
	}
	sub := args[0]
	if sub == "check" {
		return runCheck(ctx, args[1:], stdout, stderr)
	}
	switch sub {
	case "watch", "capture", "list", "show", "triage", "confirm", "import":
	default:
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), fmt.Sprintf("unsupported source subcommand %q", sub), "Run `skillhub source watch|check|capture|list|show|triage|confirm|import`.")
	}
	flags, positionals, err := parseSourceFlags(args[1:])
	if err != nil {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Review source command arguments and retry.")
	}
	switch sub {
	case "watch":
		if len(flags.skills) > 0 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, `unknown argument "--skill"`, "watch does not take --skill.")
		}
		if len(positionals) != 1 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "watch requires one locator", "Run `skillhub source watch <locator> [--id <id>] [--ref <ref>] [--path <path>] [--cadence daily|weekly|manual] [--yes]`.")
		}
	case "capture":
		if flags.yes {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, `unknown argument "--yes"`, "capture applies immediately without --yes.")
		}
		if len(flags.skills) > 0 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, `unknown argument "--skill"`, "capture does not take --skill.")
		}
		if len(positionals) != 1 || flags.reason == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "capture requires one locator and --reason", "Run `skillhub source capture <locator> --reason <text> --workspace <path>`.")
		}
	case "list":
		if flags.yes {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, `unknown argument "--yes"`, "list does not take --yes.")
		}
		if len(flags.skills) > 0 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, `unknown argument "--skill"`, "list does not take --skill.")
		}
		if len(positionals) != 0 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "list accepts no positional arguments", "Remove extra arguments.")
		}
	case "show":
		if flags.yes {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, `unknown argument "--yes"`, "show does not take --yes.")
		}
		if len(flags.skills) > 0 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, `unknown argument "--skill"`, "show does not take --skill.")
		}
		if len(positionals) != 1 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "show requires one candidate or source ID", "Provide exactly one ID.")
		}
	case "triage":
		if flags.yes {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, `unknown argument "--yes"`, "triage does not take --yes.")
		}
		if len(flags.skills) > 0 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, `unknown argument "--skill"`, "triage does not take --skill.")
		}
		if len(positionals) != 1 || flags.decision == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "triage requires one candidate ID and --decision", "Use accept, defer, or reject.")
		}
	case "confirm":
		if flags.yes {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, `unknown argument "--yes"`, "confirm does not take --yes.")
		}
		if len(flags.skills) > 0 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, `unknown argument "--skill"`, "confirm does not take --skill.")
		}
		if len(positionals) != 0 || flags.proposalID == "" || flags.proposalDigest == "" || flags.baseVersion == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "confirm requires --proposal, --proposal-digest, and --base-version", "Pass the exact pins printed by triage.")
		}
	case "import":
		if len(positionals) != 1 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "import requires one source ID", "Run `skillhub source import <source-id> [--path <subdir>] [--skill <name>]... [--yes]`.")
		}
	}
	resolved, resErr := resolveWorkspace(flags.workspace)
	if resErr != nil {
		return writeWorkspaceResolutionError(stdout, stderr, flags.jsonOutput, resErr)
	}
	flags.workspace = resolved
	service := app.SourceService{}
	switch sub {
	case "watch":
		locator := ""
		if len(positionals) > 0 {
			locator = positionals[0]
		}
		return runSourceWatch(ctx, service, flags, locator, stdout, stderr)
	case "capture":
		if len(positionals) != 1 || flags.reason == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "capture requires one locator and --reason", "Run `skillhub source capture <locator> --reason <text> --workspace <path>`.")
		}
		finishTelemetry := startCommandTelemetry(flags.workspace, func(sink app.TelemetrySink) { service.Telemetry = sink })
		defer finishTelemetry()
		result, err := service.CaptureSourceCandidate(ctx, flags.workspace, app.SourceCandidateInput{Locator: positionals[0], Reason: flags.reason, IdempotencyKey: flags.idempotencyKey})
		return writeSourceValue(stdout, stderr, flags.jsonOutput, result, err)
	case "list":
		if len(positionals) != 0 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "list accepts no positional arguments", "Remove extra arguments.")
		}
		result, err := service.ListSources(ctx, flags.workspace, flags.status)
		return writeSourceList(stdout, stderr, flags.jsonOutput, result, err)
	case "show":
		if len(positionals) != 1 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "show requires one candidate or source ID", "Provide exactly one ID.")
		}
		result, err := service.ListSources(ctx, flags.workspace, "")
		if err != nil {
			return writeSourceError(stdout, stderr, flags.jsonOutput, err)
		}
		for _, candidate := range result.Candidates {
			if candidate.ID == positionals[0] {
				return writeSingleCandidate(stdout, stderr, flags.jsonOutput, candidate)
			}
		}
		for _, record := range result.Sources {
			if record.ID == positionals[0] {
				return writeSingleSource(stdout, stderr, flags.jsonOutput, record)
			}
		}
		return writeSourceError(stdout, stderr, flags.jsonOutput, errors.New("source record not found"))
	case "triage":
		if len(positionals) != 1 || flags.decision == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "triage requires one candidate ID and --decision", "Use accept, defer, or reject.")
		}
		finishTelemetry := startCommandTelemetry(flags.workspace, func(sink app.TelemetrySink) { service.Telemetry = sink })
		defer finishTelemetry()
		proposal, result, err := service.TriageSourceCandidate(ctx, flags.workspace, app.SourceTriageInput{CandidateID: positionals[0], Decision: flags.decision, DecisionReason: flags.reason, SourceID: flags.sourceID, Adapter: flags.adapter, Ref: flags.ref, SourcePath: flags.sourcePath, License: flags.license, Trust: flags.trust, Cadence: flags.cadence, SkillID: flags.skillID, MonitoringEnabled: flags.monitoring, IdempotencyKey: flags.idempotencyKey})
		if err != nil {
			return writeSourceError(stdout, stderr, flags.jsonOutput, err)
		}
		if flags.decision == "accept" {
			return writeSourceProposal(stdout, stderr, flags.jsonOutput, proposal)
		}
		return writeSourceMutation(stdout, stderr, flags.jsonOutput, result)
	case "confirm":
		if len(positionals) != 0 || flags.proposalID == "" || flags.proposalDigest == "" || flags.baseVersion == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "confirm requires --proposal, --proposal-digest, and --base-version", "Pass the exact pins printed by triage.")
		}
		finishTelemetry := startCommandTelemetry(flags.workspace, func(sink app.TelemetrySink) { service.Telemetry = sink })
		defer finishTelemetry()
		preview, err := service.LoadSourceProposal(ctx, flags.workspace, flags.proposalID)
		if err != nil {
			return writeSourceError(stdout, stderr, flags.jsonOutput, err)
		}
		result, err := service.ConfirmSourceProposal(ctx, flags.workspace, preview, app.ConfirmationPins{ProposalID: flags.proposalID, ProposalDigest: flags.proposalDigest, BaseVersion: flags.baseVersion})
		if err != nil {
			return writeSourceError(stdout, stderr, flags.jsonOutput, err)
		}
		return writeSourceMutation(stdout, stderr, flags.jsonOutput, result)
	case "import":
		sourceID := positionals[0]
		importService := app.SourceImportService{}
		if flags.proposalID != "" {
			if flags.proposalDigest == "" || flags.baseVersion == "" {
				return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "confirming import requires --proposal, --proposal-digest, and --base-version", "Pass all three pins or use --yes.")
			}
			preview, err := importService.LoadSourceImportProposal(ctx, flags.workspace, flags.proposalID)
			if err != nil {
				return writeSourceError(stdout, stderr, flags.jsonOutput, err)
			}
			result, err := importService.ConfirmSourceImport(ctx, flags.workspace, preview, app.ConfirmationPins{
				ProposalID:     flags.proposalID,
				ProposalDigest: flags.proposalDigest,
				BaseVersion:    flags.baseVersion,
			})
			return writeSourceImportResult(stdout, stderr, flags.jsonOutput, result, err)
		}
		preview, err := importService.PreviewSourceImport(ctx, flags.workspace, app.SourceImportPreviewInput{
			SourceID:       sourceID,
			Path:           flags.sourcePath,
			Skills:         flags.skills,
			IdempotencyKey: flags.idempotencyKey,
		})
		if err != nil {
			return writeSourceError(stdout, stderr, flags.jsonOutput, err)
		}
		if flags.yes {
			if len(preview.Importable) == 0 {
				if flags.jsonOutput {
					return writeSourceJSON(stdout, stderr, preview)
				}
				fmt.Fprintln(stdout, preview.Summary)
				for _, sk := range preview.Skipped {
					fmt.Fprintf(stdout, "- skipped %s: %s\n", sk.TargetID, sk.SkipReason)
				}
				return 0
			}
			result, err := importService.ConfirmSourceImport(ctx, flags.workspace, preview, preview.Confirmation.Confirmation.Pins)
			return writeSourceImportResult(stdout, stderr, flags.jsonOutput, result, err)
		}
		return writeSourceImportProposal(stdout, stderr, flags.jsonOutput, preview, sourceID)
	default:
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, fmt.Sprintf("unsupported source subcommand %q", sub), "Run `skillhub source capture|list|show|triage|confirm`.")
	}
}

func parseSourceFlags(args []string) (sourceFlags, []string, error) {
	flags := sourceFlags{monitoring: true}
	positionals := []string{}
	for i := 0; i < len(args); i++ {
		value := args[i]
		next := func() (string, error) {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
				return "", fmt.Errorf("%s requires a value", value)
			}
			i++
			return args[i], nil
		}
		switch value {
		case "--workspace", "--reason", "--status", "--decision", "--source-id", "--id", "--adapter", "--ref", "--path", "--license", "--trust", "--cadence", "--skill-id", "--proposal", "--proposal-digest", "--base-version", "--idempotency-key":
			item, err := next()
			if err != nil {
				return flags, nil, err
			}
			switch value {
			case "--workspace":
				flags.workspace = item
			case "--reason":
				flags.reason = item
			case "--status":
				flags.status = item
			case "--decision":
				flags.decision = item
			case "--source-id", "--id":
				flags.sourceID = item
			case "--adapter":
				flags.adapter = item
			case "--ref":
				flags.ref = item
			case "--path":
				flags.sourcePath = item
			case "--license":
				flags.license = item
			case "--trust":
				flags.trust = item
			case "--cadence":
				flags.cadence = item
			case "--skill-id":
				flags.skillID = item
			case "--proposal":
				flags.proposalID = item
			case "--proposal-digest":
				flags.proposalDigest = item
			case "--base-version":
				flags.baseVersion = item
			case "--idempotency-key":
				flags.idempotencyKey = item
			}
		case "--skill":
			item, err := next()
			if err != nil {
				return flags, nil, err
			}
			flags.skills = append(flags.skills, item)
		case "--yes":
			flags.yes = true
		case "--json":
			flags.jsonOutput = true
		case "--no-monitor":
			flags.monitoring = false
		default:
			if strings.HasPrefix(value, "-") {
				return flags, nil, fmt.Errorf("unknown argument %q", value)
			}
			positionals = append(positionals, value)
		}
	}
	return flags, positionals, nil
}

func runCheck(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	path := ""
	jsonOutput := false
	allDue := false
	ids := []string{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--workspace":
			if i+1 >= len(args) {
				return writeInvalidRequest(stdout, stderr, jsonOutput, "--workspace requires a path", "Provide a workspace path.")
			}
			i++
			path = args[i]
		case "--json":
			jsonOutput = true
		case "--all-due":
			allDue = true
		case "--all":
		default:
			if strings.HasPrefix(args[i], "-") {
				return writeInvalidRequest(stdout, stderr, jsonOutput, "unknown check argument "+args[i], "Review check arguments.")
			}
			ids = append(ids, args[i])
		}
	}
	resolved, resErr := resolveWorkspace(path)
	if resErr != nil {
		return writeWorkspaceResolutionError(stdout, stderr, jsonOutput, resErr)
	}
	path = resolved
	service := app.SourceService{}
	finishTelemetry := startCommandTelemetry(path, func(sink app.TelemetrySink) { service.Telemetry = sink })
	defer finishTelemetry()
	result, err := service.CheckSources(ctx, path, ids, allDue)
	return writeSourceValue(stdout, stderr, jsonOutput, result, err)
}

func writeSourceValue(stdout, stderr io.Writer, jsonOutput bool, value any, err error) int {
	if err != nil {
		return writeSourceError(stdout, stderr, jsonOutput, err)
	}
	switch result := value.(type) {
	case app.SourceCandidateResult:
		if jsonOutput {
			if err := writeJSON(stdout, value); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			return 0
		}
		fmt.Fprintf(stdout, "%s\nCandidate: %s\n", result.Summary, result.Candidate.ID)
		return 0
	case app.SourceCheckResult:
		if result.Error != nil {
			if jsonOutput {
				if err := writeJSON(stdout, result); err != nil {
					fmt.Fprintln(stderr, err)
					return 1
				}
				return 2
			}
			fmt.Fprintf(stderr, "ERROR: %s\nWHY: %s\nFIX: %s\n", result.Error.Render.Error, result.Error.Render.Why, result.Error.Render.Fix)
			return 2
		}
		if jsonOutput {
			if err := writeJSON(stdout, value); err != nil {
				fmt.Fprintln(stderr, err)
				return 1
			}
			if result.Status == app.StatusError {
				return 2
			}
			return 0
		}
		fmt.Fprintln(stdout, result.Summary)
		for _, item := range result.Results {
			fmt.Fprintf(stdout, "- %s: %s\n", item.SourceID, item.Status)
		}
		for _, warning := range result.Warnings {
			fmt.Fprintf(stdout, "WARNING: %s\n", warning.Summary)
		}
		if result.Status == app.StatusError {
			return 2
		}
		return 0
	}
	if jsonOutput {
		if err := writeJSON(stdout, value); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	return 0
}
func writeSourceList(stdout, stderr io.Writer, jsonOutput bool, result app.SourceListResult, err error) int {
	if err != nil {
		return writeSourceError(stdout, stderr, jsonOutput, err)
	}
	if jsonOutput {
		if err := writeJSON(stdout, result); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	fmt.Fprintln(stdout, result.Summary)
	for _, item := range result.Candidates {
		fmt.Fprintf(stdout, "- %s [%s] %s — %s\n", item.ID, item.Status, item.Locator, item.Reason)
	}
	for _, item := range result.Sources {
		fmt.Fprintf(stdout, "- %s [%s] %s (%s)\n", item.ID, item.Status, item.Identity.Name, item.Adapter)
	}
	return 0
}
func writeSourceProposal(stdout, stderr io.Writer, jsonOutput bool, result app.SourceProposal) int {
	if jsonOutput {
		return writeSourceJSON(stdout, stderr, result)
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(stdout, "WARNING: %s\n", warning.Summary)
	}
	pins := result.Confirmation.Confirmation.Pins
	fmt.Fprintf(stdout, "%s\nDetected: %s; adapter %s; revision %s; license %s; trust %s; cadence %s.\nProposal: %s\nDigest: %s\nBase version: %s\nNo files changed. Confirm with:\n  skillhub source confirm --proposal %s --proposal-digest %s --base-version %s\n", result.Summary, result.Source.Identity.Name, result.Source.Adapter, result.Source.CurrentRevision.Value, firstText(result.Source.License, "unknown"), result.Source.Trust.Source, result.Source.Monitoring.Cadence, pins.ProposalID, pins.ProposalDigest, pins.BaseVersion, pins.ProposalID, pins.ProposalDigest, pins.BaseVersion)
	return 0
}

func writeSingleCandidate(stdout, stderr io.Writer, jsonOutput bool, candidate sourcepkg.Candidate) int {
	if jsonOutput {
		return writeSourceJSON(stdout, stderr, candidate)
	}
	fmt.Fprintf(stdout, "- %s [%s] %s — %s\n", candidate.ID, candidate.Status, candidate.Locator, candidate.Reason)
	return 0
}

func writeSingleSource(stdout, stderr io.Writer, jsonOutput bool, record sourcepkg.Record) int {
	if jsonOutput {
		return writeSourceJSON(stdout, stderr, record)
	}
	fmt.Fprintf(stdout, "- %s [%s] %s (%s)\n", record.ID, record.Status, record.Identity.Name, record.Adapter)
	return 0
}

func writeSourceImportProposal(stdout, stderr io.Writer, jsonOutput bool, proposal app.SourceImportProposal, sourceID string) int {
	if jsonOutput {
		return writeSourceJSON(stdout, stderr, proposal)
	}
	pins := proposal.Confirmation.Confirmation.Pins
	fmt.Fprintf(stdout, "%s\n", proposal.Summary)
	for _, item := range proposal.Importable {
		fmt.Fprintf(stdout, "- %s -> %s [draft] (importable)\n", item.Name, item.TargetID)
	}
	for _, item := range proposal.Skipped {
		fmt.Fprintf(stdout, "- %s -> %s [skipped: %s]\n", item.Name, item.TargetID, item.SkipReason)
	}
	for _, warn := range proposal.Warnings {
		fmt.Fprintf(stdout, "WARNING: %s\n", warn.Summary)
	}
	fmt.Fprintf(stdout, "\nProposal: %s\nDigest: %s\nBase version: %s\nConfirm with:\n  skillhub source import %s --proposal %s --proposal-digest %s --base-version %s\nor re-run with --yes to import directly.\n", pins.ProposalID, pins.ProposalDigest, pins.BaseVersion, sourceID, pins.ProposalID, pins.ProposalDigest, pins.BaseVersion)
	return 0
}

func writeSourceImportResult(stdout, stderr io.Writer, jsonOutput bool, result app.SourceImportResult, err error) int {
	if err != nil {
		return writeSourceError(stdout, stderr, jsonOutput, err)
	}
	if jsonOutput {
		return writeSourceJSON(stdout, stderr, result)
	}
	if result.Status == app.StatusError {
		fmt.Fprintln(stderr, result.Summary)
		return 2
	}
	fmt.Fprintln(stdout, result.Summary)
	if len(result.SkippedIDs) > 0 {
		fmt.Fprintf(stdout, "Skipped %d existing skill(s): %s\n", len(result.SkippedIDs), strings.Join(result.SkippedIDs, ", "))
	}
	return 0
}

func writeSourceMutation(stdout, stderr io.Writer, jsonOutput bool, result app.SourceMutationResult) int {
	if jsonOutput {
		return writeSourceJSON(stdout, stderr, result)
	}
	if result.Status == app.StatusError {
		fmt.Fprintln(stderr, result.Summary)
		return 2
	}
	if result.SourceID != "" {
		fmt.Fprintf(stdout, "Watching %s. First analysis is ready: ask your agent 'distill new sources' or run `skillhub distill prepare %s`.\nWatching does not auto-import skills; accepted insights can create draft skills.\n", result.SourceID, result.SourceID)
		return 0
	}
	fmt.Fprintf(stdout, "%s\nOperation: %s\nCatalog snapshot: %s\nGit dirty: %t\n", result.Summary, result.OperationID, result.CatalogSnapshot, result.GitDirty)
	return 0
}

func writeSourceJSON(stdout, stderr io.Writer, value any) int {
	if err := writeJSON(stdout, value); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

func writeSourceError(stdout, stderr io.Writer, jsonOutput bool, err error) int {
	var limitErr *sourcepkg.LimitExceededError
	if errors.As(err, &limitErr) {
		why := fmt.Sprintf("Source resource limit exceeded: %s (%d > %d).", limitErr.Limit, limitErr.Actual, limitErr.Max)
		fix := "Scope the source using `skillhub source triage <candidate-id> --decision accept --path <subdir>`."
		return writeInvalidRequest(stdout, stderr, jsonOutput, why, fix)
	}
	why := err.Error()
	if strings.Contains(why, "statat") && strings.Contains(why, "no such file or directory") {
		why = "The requested file or record was not found."
	}
	return writeInvalidRequest(stdout, stderr, jsonOutput, why, "Correct the source locator, policy, or workspace state and retry.")
}
func firstText(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
