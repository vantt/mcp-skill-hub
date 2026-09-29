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

type distillFlags struct {
	workspace, submission, sourceID, skillID, idempotencyKey string
	jsonOutput, allChanged                                   bool
}

func runDistill(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), "distill requires a subcommand", "Run `skillhub distill prepare|start|submit|get|retry|cancel|findings|comparisons|insights`.")
	}
	sub := args[0]
	flags, positionals, err := parseDistillFlags(args[1:])
	if err != nil {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Review distill command arguments and retry.")
	}
	service := app.DistillService{}
	switch sub {
	case "prepare":
		if len(positionals) == 0 && !flags.allChanged {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "prepare requires source IDs or --all-changed", "Select at least one source.")
		}
		result, callErr := service.PrepareDistillRuns(ctx, flags.workspace, app.DistillPrepareInput{SourceIDs: positionals, AllChanged: flags.allChanged, IdempotencyKey: flags.idempotencyKey})
		return writeDistill(stdout, stderr, flags.jsonOutput, result, callErr)
	case "start", "retry", "cancel", "get":
		if len(positionals) != 1 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, sub+" requires exactly one run ID", "Provide one run ID.")
		}
		var result app.DistillRunResult
		var callErr error
		switch sub {
		case "start":
			result, callErr = service.StartDistillRun(ctx, flags.workspace, positionals[0])
		case "retry":
			result, callErr = service.RetryDistillRun(ctx, flags.workspace, positionals[0])
		case "cancel":
			result, callErr = service.CancelDistillRun(ctx, flags.workspace, positionals[0])
		case "get":
			result, callErr = service.GetDistillRun(ctx, flags.workspace, positionals[0])
		}
		return writeDistill(stdout, stderr, flags.jsonOutput, result, callErr)
	case "submit":
		if len(positionals) != 1 || flags.submission == "" {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, "submit requires one run ID and --submission <json-file>", "Pass a structured submission containing coverage, findings, comparisons, and insights.")
		}
		input, decodeErr := readDistillSubmission(flags.submission)
		if decodeErr != nil {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, decodeErr.Error(), "Correct the structured submission and retry the run.")
		}
		input.IdempotencyKey = firstText(flags.idempotencyKey, input.IdempotencyKey)
		result, callErr := service.SubmitDistillRun(ctx, flags.workspace, positionals[0], input)
		return writeDistill(stdout, stderr, flags.jsonOutput, result, callErr)
	case "findings", "comparisons", "insights":
		if len(positionals) != 0 {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, sub+" accepts no positional arguments", "Use --source-id or --skill-id to filter.")
		}
		result, callErr := service.QueryDistill(ctx, flags.workspace, sub, flags.sourceID, flags.skillID)
		return writeDistill(stdout, stderr, flags.jsonOutput, result, callErr)
	default:
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, fmt.Sprintf("unsupported distill subcommand %q", sub), "Use prepare, start, submit, get, retry, cancel, findings, comparisons, or insights.")
	}
}

func parseDistillFlags(args []string) (distillFlags, []string, error) {
	var flags distillFlags
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
		case "--workspace", "--submission", "--source-id", "--skill-id", "--idempotency-key":
			item, err := next()
			if err != nil {
				return flags, nil, err
			}
			switch value {
			case "--workspace":
				flags.workspace = item
			case "--submission":
				flags.submission = item
			case "--source-id":
				flags.sourceID = item
			case "--skill-id":
				flags.skillID = item
			case "--idempotency-key":
				flags.idempotencyKey = item
			}
		case "--json":
			flags.jsonOutput = true
		case "--all-changed":
			flags.allChanged = true
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

func readDistillSubmission(path string) (app.DistillSubmission, error) {
	info, err := os.Stat(path)
	if err != nil {
		return app.DistillSubmission{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > 8<<20 {
		return app.DistillSubmission{}, errors.New("submission must be a regular JSON file no larger than 8 MiB")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return app.DistillSubmission{}, err
	}
	var value app.DistillSubmission
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, fmt.Errorf("decode submission: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return value, errors.New("submission must contain exactly one JSON object")
	}
	return value, nil
}

func writeDistill(stdout, stderr io.Writer, jsonOutput bool, value any, err error) int {
	if err != nil {
		return writeInvalidRequest(stdout, stderr, jsonOutput, err.Error(), "Inspect the pinned run/package or correct the structured submission, then retry.")
	}
	if jsonOutput {
		if err := writeJSON(stdout, value); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	switch result := value.(type) {
	case app.DistillBatchResult:
		fmt.Fprintln(stdout, result.Summary)
		for _, item := range result.Results {
			state := "prepared"
			if item.Error != "" {
				state = "failed: " + item.Error
			}
			fmt.Fprintf(stdout, "- %s: %s\n", item.SourceID, state)
		}
	case app.DistillRunResult:
		fmt.Fprintf(stdout, "%s\nRun: %s [%s]\n", result.Summary, result.Run.ID, result.Run.State)
	case app.DistillQueryResult:
		fmt.Fprintln(stdout, result.Summary)
		for _, item := range result.Findings {
			fmt.Fprintf(stdout, "- %s [%s] %s\n", item.ID, item.Status, item.What)
		}
		for _, item := range result.Comparisons {
			fmt.Fprintf(stdout, "- %s [%s] %s\n", item.ID, item.Verdict, item.Subject)
		}
		for _, item := range result.Insights {
			fmt.Fprintf(stdout, "- %s [%s] %s\n", item.ID, item.Status, item.Recommendation)
		}
	}
	return 0
}
