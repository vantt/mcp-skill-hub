package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

type insightFlags struct {
	workspace, decision, reason, proposalFile, proposalID, proposalDigest, baseVersion string
	state, note, supersedes, idempotencyKey, artifact, finding                         string
	evidence                                                                           []string
	jsonOutput, yes                                                                    bool
}

func runInbox(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags, positionals, err := parseInsightFlags(args)
	if err != nil || len(positionals) != 0 {
		if err == nil {
			err = errors.New("inbox accepts no positional arguments")
		}
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Pass only --workspace and optional --json.")
	}
	resolved, resErr := resolveWorkspace(flags.workspace)
	if resErr != nil {
		return writeWorkspaceResolutionError(stdout, stderr, flags.jsonOutput, resErr)
	}
	result, callErr := (app.InsightService{}).GetInsightInbox(ctx, resolved)
	return writeInsightResult(stdout, stderr, flags.jsonOutput, result, callErr)
}

func runInsight(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), "insight requires a subcommand", "Use decide, apply, confirm, outcome, provenance, impact, or operation-diff.")
	}
	sub := args[0]
	switch sub {
	case "show", "decide", "plan", "reject", "obsolete", "reopen", "apply", "confirm", "outcome", "provenance", "impact", "operation-diff":
	default:
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), fmt.Sprintf("unsupported insight subcommand %q", sub), "Use decide, apply, confirm, outcome, provenance, impact, or operation-diff.")
	}
	flags, positionals, err := parseInsightFlags(args[1:])
	if err != nil {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Review insight command arguments and retry.")
	}
	switch sub {
	case "show":
		if len(positionals) != 1 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "show requires one insight ID", "Provide one insight ID.")
		}
	case "decide", "plan", "reject", "obsolete", "reopen":
		decision := flags.decision
		if sub != "decide" {
			decision = sub
		}
		if len(positionals) != 1 || decision == "" || flags.reason == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, sub+" requires one insight ID and --reason", "Choose plan, reject, obsolete, or reopen with an explicit rationale.")
		}
	case "apply":
		if flags.yes {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "apply is preview-only and does not accept --yes", "Run apply without --yes, then use a separate insight confirm invocation with the exact persisted proposal pins.")
		}
		if len(positionals) != 1 || flags.proposalFile == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "apply requires one insight ID and --proposal-file", "Provide a JSON application proposal with exact changes and mappings.")
		}
	case "confirm":
		if len(positionals) != 0 || flags.proposalID == "" || flags.proposalDigest == "" || flags.baseVersion == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "confirm requires --proposal, --proposal-digest, and --base-version", "Confirm the exact pins returned by apply preview.")
		}
	case "outcome":
		if len(positionals) != 1 || flags.state == "" || flags.note == "" || len(flags.evidence) == 0 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "outcome requires one incorporation ID, --state, --note, and at least one --evidence", "Record confirmed, adjusted, or ineffective only from explicit evidence.")
		}
	case "provenance":
		if len(positionals) != 0 || flags.artifact == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "provenance requires --artifact <workspace-relative-path>", "Provide one local artifact path.")
		}
	case "impact":
		if len(positionals) != 0 || flags.finding == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "impact requires --finding <observation-id>", "Provide one changed finding ID.")
		}
	case "operation-diff":
		if len(positionals) != 1 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "operation-diff requires one operation ID", "Provide the managed operation ID.")
		}
	}
	resolved, resErr := resolveWorkspace(flags.workspace)
	if resErr != nil {
		return writeWorkspaceResolutionError(stdout, stderr, flags.jsonOutput, resErr)
	}
	flags.workspace = resolved
	service := app.InsightService{}
	switch sub {
	case "show":
		if len(positionals) != 1 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "show requires one insight ID", "Provide one insight ID.")
		}
		result, callErr := service.GetInsightDetail(ctx, flags.workspace, positionals[0])
		return writeInsightResult(stdout, stderr, flags.jsonOutput, result, callErr)
	case "decide", "plan", "reject", "obsolete", "reopen":
		decision := flags.decision
		if sub != "decide" {
			decision = sub
		}
		if len(positionals) != 1 || decision == "" || flags.reason == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, sub+" requires one insight ID and --reason", "Choose plan, reject, obsolete, or reopen with an explicit rationale.")
		}
		result, callErr := service.DecideInsight(ctx, flags.workspace, positionals[0], app.InsightDecisionInput{Decision: decision, Rationale: flags.reason, IdempotencyKey: flags.idempotencyKey})
		return writeInsightResult(stdout, stderr, flags.jsonOutput, result, callErr)
	case "apply":
		if flags.yes {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "apply is preview-only and does not accept --yes", "Run apply without --yes, then use a separate insight confirm invocation with the exact persisted proposal pins.")
		}
		if len(positionals) != 1 || flags.proposalFile == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "apply requires one insight ID and --proposal-file", "Provide a JSON application proposal with exact changes and mappings.")
		}
		input, readErr := readInsightApplication(flags.proposalFile)
		if readErr != nil {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, readErr.Error(), "Correct the proposal JSON and retry.")
		}
		input.IdempotencyKey = firstText(flags.idempotencyKey, input.IdempotencyKey)
		preview, callErr := service.PreviewInsightApplication(ctx, flags.workspace, positionals[0], input)
		if callErr != nil {
			return writeInsightResult(stdout, stderr, flags.jsonOutput, preview, callErr)
		}
		// Application creation is always preview-only. Confirmation is deliberately
		// a separate invocation that reloads the persisted immutable proposal.
		return writeInsightResult(stdout, stderr, flags.jsonOutput, preview, nil)
	case "confirm":
		if len(positionals) != 0 || flags.proposalID == "" || flags.proposalDigest == "" || flags.baseVersion == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "confirm requires --proposal, --proposal-digest, and --base-version", "Confirm the exact pins returned by apply preview.")
		}
		result, callErr := service.ConfirmInsightApplication(ctx, flags.workspace, flags.proposalID, flags.proposalDigest, flags.baseVersion)
		return writeInsightResult(stdout, stderr, flags.jsonOutput, result, callErr)
	case "outcome":
		if len(positionals) != 1 || flags.state == "" || flags.note == "" || len(flags.evidence) == 0 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "outcome requires one incorporation ID, --state, --note, and at least one --evidence", "Record confirmed, adjusted, or ineffective only from explicit evidence.")
		}
		result, callErr := service.RecordIncorporationOutcome(ctx, flags.workspace, positionals[0], app.OutcomeInput{State: flags.state, Evidence: flags.evidence, Note: flags.note, Supersedes: flags.supersedes, IdempotencyKey: flags.idempotencyKey})
		return writeInsightResult(stdout, stderr, flags.jsonOutput, result, callErr)
	case "provenance":
		if len(positionals) != 0 || flags.artifact == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "provenance requires --artifact <workspace-relative-path>", "Provide one local artifact path.")
		}
		result, callErr := service.QueryArtifactProvenance(ctx, flags.workspace, flags.artifact)
		return writeInsightResult(stdout, stderr, flags.jsonOutput, result, callErr)
	case "impact":
		if len(positionals) != 0 || flags.finding == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "impact requires --finding <observation-id>", "Provide one changed finding ID.")
		}
		result, callErr := service.QueryFindingImpact(ctx, flags.workspace, flags.finding)
		return writeInsightResult(stdout, stderr, flags.jsonOutput, result, callErr)
	case "operation-diff":
		if len(positionals) != 1 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "operation-diff requires one operation ID", "Provide the managed operation ID.")
		}
		result, callErr := service.GetOperationDiff(ctx, flags.workspace, positionals[0])
		return writeInsightResult(stdout, stderr, flags.jsonOutput, result, callErr)
	default:
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, fmt.Sprintf("unsupported insight subcommand %q", sub), "Use decide, apply, confirm, outcome, provenance, impact, or operation-diff.")
	}
}

func parseInsightFlags(args []string) (insightFlags, []string, error) {
	var flags insightFlags
	var positionals []string
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
		case "--workspace", "--decision", "--reason", "--proposal-file", "--proposal", "--proposal-digest", "--base-version", "--state", "--note", "--supersedes", "--idempotency-key", "--artifact", "--finding", "--evidence":
			item, e := next()
			if e != nil {
				return flags, nil, e
			}
			switch value {
			case "--workspace":
				flags.workspace = item
			case "--decision":
				flags.decision = item
			case "--reason":
				flags.reason = item
			case "--proposal-file":
				flags.proposalFile = item
			case "--proposal":
				flags.proposalID = item
			case "--proposal-digest":
				flags.proposalDigest = item
			case "--base-version":
				flags.baseVersion = item
			case "--state":
				flags.state = item
			case "--note":
				flags.note = item
			case "--supersedes":
				flags.supersedes = item
			case "--idempotency-key":
				flags.idempotencyKey = item
			case "--artifact":
				flags.artifact = item
			case "--finding":
				flags.finding = item
			case "--evidence":
				flags.evidence = append(flags.evidence, item)
			}
		case "--json":
			flags.jsonOutput = true
		case "--yes":
			flags.yes = true
		default:
			if strings.HasPrefix(value, "-") {
				return flags, nil, fmt.Errorf("unknown argument %q", value)
			}
			positionals = append(positionals, value)
		}
	}
	return flags, positionals, nil
}

func readInsightApplication(path string) (app.PreviewInsightInput, error) {
	info, err := os.Stat(path)
	if err != nil {
		return app.PreviewInsightInput{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 16<<20 {
		return app.PreviewInsightInput{}, errors.New("proposal must be a regular JSON file no larger than 16 MiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return app.PreviewInsightInput{}, err
	}
	var value app.PreviewInsightInput
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return value, errors.New("proposal must contain exactly one JSON object")
	}
	return value, nil
}

func writeInsightResult(stdout, stderr io.Writer, jsonOutput bool, value any, err error) int {
	if err != nil {
		return writeInvalidRequest(stdout, stderr, jsonOutput, err.Error(), "Review the insight state, evidence, proposal pins, and workspace, then retry.")
	}
	if jsonOutput {
		if err := writeJSON(stdout, value); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if result, ok := value.(app.InsightApplicationResult); ok && result.Status == app.StatusError {
			return 2
		}
		return 0
	}
	switch result := value.(type) {
	case app.InsightInboxResult:
		fmt.Fprintln(stdout, result.Summary)
		for _, group := range result.Groups {
			fmt.Fprintf(stdout, "%s / %s\n", group.SkillID, group.Category)
			for _, item := range group.Items {
				fmt.Fprintf(stdout, "- %s score=%d evidence_sources=%d impact=%s stale=%t: %s\n", item.Insight.ID, item.Rank.Score, item.Rank.EvidenceSources, item.Rank.Impact, item.Rank.Stale, item.Insight.Recommendation)
			}
		}
	case app.InsightDetailResult:
		fmt.Fprintf(stdout, "%s\n%s [%s]: %s\n", result.Summary, result.Insight.ID, result.Insight.Status, result.Insight.Recommendation)
		for _, finding := range result.Findings {
			fmt.Fprintf(stdout, "- %s @ %s: %s\n", finding.ID, finding.LastSeen.Value, finding.What)
		}
	case app.InsightDecisionResult:
		fmt.Fprintf(stdout, "%s\n%s [%s]\nOperation: %s\n", result.Summary, result.Insight.ID, result.Insight.Status, result.OperationID)
	case app.InsightApplicationPreview:
		fmt.Fprintf(stdout, "%s\nProposal: %s\nDigest: %s\nBase version: %s\n%sConfirm with:\n  skillhub insight confirm --proposal %s --proposal-digest %s --base-version %s\n", result.Summary, result.ProposalID, result.ProposalDigest, result.BaseCatalogVersion, result.Diff, result.ProposalID, result.ProposalDigest, result.BaseCatalogVersion)
	case app.InsightApplicationResult:
		if result.Error != nil {
			fmt.Fprintf(stderr, "ERROR: %s\nWHY: %s\nFIX: %s\n", result.Error.Render.Error, result.Error.Render.Why, result.Error.Render.Fix)
			return 2
		}
		fmt.Fprintf(stdout, "%s\nOperation: %s\nIncorporation: %s\nGit dirty: %t\n", result.Summary, result.OperationID, result.IncorporationID, result.GitDirty)
	case app.OutcomeResult:
		fmt.Fprintf(stdout, "%s\nOutcome: %s [%s]\n", result.Summary, result.Outcome.ID, result.Outcome.State)
	case app.ProvenanceResult:
		fmt.Fprintln(stdout, result.Summary)
		for _, path := range result.AffectedArtifacts {
			fmt.Fprintf(stdout, "- %s\n", path)
		}
	case app.OperationDiffResult:
		fmt.Fprintln(stdout, result.Summary)
		for _, change := range result.Changes {
			if change.DiffAvailable {
				fmt.Fprint(stdout, change.Diff)
			} else {
				fmt.Fprintf(stdout, "%s: digest-only metadata before=%s after=%s\n", change.Path, change.BeforeDigest, change.AfterDigest)
			}
		}
		fmt.Fprintf(stdout, "Review: %s\nWARNING: %s\n", result.ReviewCommand, result.Warning)
		for _, line := range result.RestoreGuidance {
			fmt.Fprintf(stdout, "- %s\n", line)
		}
	}
	return 0
}
