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
	jsonOutput, monitoring                                                                                             bool
}

func runSource(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), "source requires a subcommand", "Run `skillhub source capture|list|show|triage|confirm`.")
	}
	sub := args[0]
	flags, positionals, err := parseSourceFlags(args[1:])
	if err != nil {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Review source command arguments and retry.")
	}
	service := app.SourceService{}
	switch sub {
	case "capture":
		if len(positionals) != 1 || flags.reason == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "capture requires one locator and --reason", "Run `skillhub source capture <locator> --reason <text> --workspace <path>`.")
		}
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
				result.Candidates = []sourcepkg.Candidate{candidate}
				result.Sources = nil
				return writeSourceList(stdout, stderr, flags.jsonOutput, result, nil)
			}
		}
		for _, record := range result.Sources {
			if record.ID == positionals[0] {
				result.Candidates = nil
				result.Sources = []sourcepkg.Record{record}
				return writeSourceList(stdout, stderr, flags.jsonOutput, result, nil)
			}
		}
		return writeSourceError(stdout, stderr, flags.jsonOutput, errors.New("source record not found"))
	case "triage":
		if len(positionals) != 1 || flags.decision == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "triage requires one candidate ID and --decision", "Use accept, defer, or reject.")
		}
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
		preview, err := service.LoadSourceProposal(ctx, flags.workspace, flags.proposalID)
		if err != nil {
			return writeSourceError(stdout, stderr, flags.jsonOutput, err)
		}
		result, err := service.ConfirmSourceProposal(ctx, flags.workspace, preview, app.ConfirmationPins{ProposalID: flags.proposalID, ProposalDigest: flags.proposalDigest, BaseVersion: flags.baseVersion})
		if err != nil {
			return writeSourceError(stdout, stderr, flags.jsonOutput, err)
		}
		return writeSourceMutation(stdout, stderr, flags.jsonOutput, result)
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
		case "--workspace", "--reason", "--status", "--decision", "--source-id", "--adapter", "--ref", "--path", "--license", "--trust", "--cadence", "--skill-id", "--proposal", "--proposal-digest", "--base-version", "--idempotency-key":
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
			case "--source-id":
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
	if flags.workspace == "" {
		return flags, nil, errors.New("--workspace is required for non-interactive use")
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
	if path == "" {
		return writeInvalidRequest(stdout, stderr, jsonOutput, "--workspace is required for non-interactive use", "Run `skillhub check --workspace <path> [--all-due|--all|source-id...]`.")
	}
	result, err := (app.SourceService{}).CheckSources(ctx, path, ids, allDue)
	return writeSourceValue(stdout, stderr, jsonOutput, result, err)
}

func writeSourceValue(stdout, stderr io.Writer, jsonOutput bool, value any, err error) int {
	if err != nil {
		return writeSourceError(stdout, stderr, jsonOutput, err)
	}
	if jsonOutput {
		if err := writeJSON(stdout, value); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	switch result := value.(type) {
	case app.SourceCandidateResult:
		fmt.Fprintf(stdout, "%s\nCandidate: %s\n", result.Summary, result.Candidate.ID)
	case app.SourceCheckResult:
		fmt.Fprintln(stdout, result.Summary)
		for _, item := range result.Results {
			fmt.Fprintf(stdout, "- %s: %s\n", item.SourceID, item.Status)
		}
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
	pins := result.Confirmation.Confirmation.Pins
	fmt.Fprintf(stdout, "%s\nDetected: %s; adapter %s; revision %s; license %s; trust %s; cadence %s.\nProposal: %s\nDigest: %s\nBase catalog: %s\nNo canonical files changed. Confirm these exact pins with `skillhub source confirm`.\n", result.Summary, result.Source.Identity.Name, result.Source.Adapter, result.Source.CurrentRevision.Value, firstText(result.Source.License, "unknown"), result.Source.Trust.Source, result.Source.Monitoring.Cadence, pins.ProposalID, pins.ProposalDigest, pins.BaseVersion)
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
	return writeInvalidRequest(stdout, stderr, jsonOutput, err.Error(), "Correct the source locator, policy, or workspace state and retry.")
}
func firstText(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
