package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/delivery/cli/termui"
	"github.com/vantt/mcp-skill-hub/internal/evaluation"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

type evaluationFlags struct {
	workspacePath string
	inputPath     string
	manifestPath  string
	outputPath    string
	partition     evaluation.Partition
	jsonOutput    bool
}

type manifestFlags struct {
	workspacePath string
	suitePath     string
	casePath      string
	experimentID  string
	variant       string
	seed          *int64
	outputPath    string
	jsonOutput    bool
}

type promotionFlags struct {
	workspacePath string
	resolutionID  string
	outputPath    string
	jsonOutput    bool
	yes           bool
}

func runEvaluation(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	const runUsage = "Run `skillhub eval run --workspace <path> --suite <suite> --manifest <manifest>`."
	if err := validateDeliveryArguments(args); err != nil {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), runUsage)
	}
	if len(args) == 0 {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), "eval requires a subcommand", runUsage)
	}
	switch args[0] {
	case "manifest":
		return runEvaluationManifest(ctx, args[1:], stdout, stderr)
	case "run":
		return runEvaluationSuite(ctx, args[1:], stdout, stderr)
	case "promote":
		return runEvaluationPromotion(ctx, args[1:], stdout, stderr)
	case "routing":
		return runEvaluationRouting(ctx, args[1:], stdout, stderr)
	default:
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), fmt.Sprintf("unsupported eval subcommand %q", args[0]), "Run `skillhub eval manifest`, `skillhub eval run`, `skillhub eval promote`, or `skillhub eval routing` with the required flags.")
	}
}

func runEvaluationManifest(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	const usage = "Run `skillhub eval manifest --workspace <path> (--suite <suite> | --case <case>) --experiment-id <id> --variant <variant> [--seed <integer>] [--output <new-file>] [--json]`."
	flags, err := parseManifestFlags(args)
	if err != nil {
		var resErr *WorkspaceResolutionError
		if errors.As(err, &resErr) {
			return writeWorkspaceResolutionError(stdout, stderr, hasJSONFlag(args), resErr)
		}
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), usage)
	}
	if flags.outputPath != "" {
		if err := validateNewOutputPath(flags.outputPath); err != nil {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), "Choose a new regular output path and retry.")
		}
	}
	service := app.EvaluationService{Seed: flags.seed}
	var manifest evaluation.Manifest
	if flags.casePath != "" {
		manifest, err = service.GenerateCaseManifest(ctx, flags.workspacePath, flags.casePath, flags.experimentID, flags.variant)
	} else {
		manifest, err = service.GenerateManifest(ctx, flags.workspacePath, flags.suitePath, flags.experimentID, flags.variant)
	}
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return writeWorkspaceResult(app.Result{}, context.Canceled, stdout, stderr, flags.jsonOutput)
	}
	if err != nil {
		return writeInvalidWorkspace(stdout, stderr, flags.jsonOutput, err)
	}
	contents, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return writeInvalidWorkspace(stdout, stderr, flags.jsonOutput, fmt.Errorf("marshal experiment manifest: %w", err))
	}
	contents = append(contents, '\n')
	if flags.outputPath != "" {
		if err := writeNewFileAtomic(flags.outputPath, contents); err != nil {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), "Choose a new regular output path and retry.")
		}
	}
	if flags.jsonOutput {
		p := termui.New(stdout)
		p.Raw(string(contents))
		if p.Err() != nil {
			pErr := termui.New(stderr)
			pErr.Error("Unable to write the experiment manifest.", p.Err().Error(), "Check the output destination and retry.")
			return 1
		}
		return 0
	}
	p := termui.New(stdout)
	p.Line(fmt.Sprintf("Experiment manifest preview (default seed %d when --seed is omitted):", app.EvaluationDefaultSeed))
	p.Raw(string(contents))
	if p.Err() != nil {
		pErr := termui.New(stderr)
		pErr.Error("Unable to write the experiment manifest preview.", p.Err().Error(), "Check the output destination and retry.")
		return 1
	}
	if flags.outputPath == "" {
		p.Line("No file was written; pass --output <new-file> to save this manifest.")
	} else {
		p.Line(fmt.Sprintf("Manifest written to %s.", flags.outputPath))
	}
	return 0
}

func runEvaluationSuite(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	const usage = "Run `skillhub eval run --workspace <path> --suite <suite> --manifest <manifest>`."
	flags, err := parseEvaluationFlags(args, "--suite", true)
	if err != nil {
		var resErr *WorkspaceResolutionError
		if errors.As(err, &resErr) {
			return writeWorkspaceResolutionError(stdout, stderr, hasJSONFlag(args), resErr)
		}
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), usage)
	}
	manifest, err := evaluation.LoadManifest(flags.manifestPath)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), "Correct the strict experiment manifest and retry.")
	}
	if flags.outputPath != "" {
		if err := validateNewOutputPath(flags.outputPath); err != nil {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), "Choose a new regular output path and retry.")
		}
	}
	options := evaluation.RunOptions{}
	if flags.partition != "" {
		options.Partitions = []evaluation.Partition{flags.partition}
	}
	report, err := (app.EvaluationService{}).RunSuite(ctx, flags.workspacePath, flags.inputPath, manifest, options)
	return finishEvaluation(ctx, report, err, flags, "Evaluation", stdout, stderr)
}

func runEvaluationPromotion(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	const usage = "Run `skillhub eval promote --workspace <path> --resolution-id <id> [--output <new-file>] [--yes] [--json]`."
	flags, err := parsePromotionFlags(args)
	if err != nil {
		var resErr *WorkspaceResolutionError
		if errors.As(err, &resErr) {
			return writeWorkspaceResolutionError(stdout, stderr, hasJSONFlag(args), resErr)
		}
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), usage)
	}
	if flags.yes {
		if err := validateNewOutputPath(flags.outputPath); err != nil {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), "Choose a new regular output path and retry.")
		}
	}
	draft, err := (app.TelemetryService{}).PromotionDraft(ctx, flags.workspacePath, flags.resolutionID)
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return writeWorkspaceResult(app.Result{}, context.Canceled, stdout, stderr, flags.jsonOutput)
	}
	if err != nil {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), "Choose one exact, unambiguous telemetry resolution ID and retry.")
	}
	contents, err := draft.JSON()
	if err != nil {
		return writeInvalidWorkspace(stdout, stderr, flags.jsonOutput, fmt.Errorf("marshal promotion draft: %w", err))
	}
	if flags.yes {
		if err := writeNewFileAtomic(flags.outputPath, contents); err != nil {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), "Choose a new regular output path and retry.")
		}
	}
	if flags.jsonOutput {
		p := termui.New(stdout)
		p.Raw(string(contents))
		if p.Err() != nil {
			pErr := termui.New(stderr)
			pErr.Error("Unable to write the promotion draft.", p.Err().Error(), "Check the output destination and retry.")
			return 1
		}
		return 0
	}
	return writePromotionSummary(draft, contents, flags, stdout, stderr)
}

func runResolution(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if err := validateDeliveryArguments(args); err != nil {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub resolution replay --workspace <path> --case <case> --manifest <manifest>`.")
	}
	if len(args) == 0 || args[0] != "replay" {
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), "resolution supports only the replay subcommand", "Run `skillhub resolution replay --workspace <path> --case <case> --manifest <manifest>`.")
	}
	flags, err := parseEvaluationFlags(args[1:], "--case", false)
	if err != nil {
		var resErr *WorkspaceResolutionError
		if errors.As(err, &resErr) {
			return writeWorkspaceResolutionError(stdout, stderr, hasJSONFlag(args), resErr)
		}
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub resolution replay --workspace <path> --case <case> --manifest <manifest>`.")
	}
	manifest, err := evaluation.LoadManifest(flags.manifestPath)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), "Correct the strict experiment manifest and retry.")
	}
	if flags.outputPath != "" {
		if err := validateNewOutputPath(flags.outputPath); err != nil {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), "Choose a new regular output path and retry.")
		}
	}
	report, err := (app.EvaluationService{}).ReplayCase(ctx, flags.workspacePath, flags.inputPath, manifest, evaluation.RunOptions{})
	return finishEvaluation(ctx, report, err, flags, "Replay", stdout, stderr)
}

func parseManifestFlags(args []string) (manifestFlags, error) {
	var result manifestFlags
	if len(args) > maxDeliveryArgs {
		return result, errors.New("too many arguments")
	}
	seen := map[string]bool{}
	for index := 0; index < len(args); index++ {
		flag := args[index]
		if seen[flag] {
			return result, fmt.Errorf("%s may only be provided once", flag)
		}
		seen[flag] = true
		switch flag {
		case "--json":
			result.jsonOutput = true
		case "--workspace", "--suite", "--case", "--experiment-id", "--variant", "--seed", "--output":
			if index+1 == len(args) || strings.HasPrefix(args[index+1], "-") {
				return result, fmt.Errorf("%s requires a value", flag)
			}
			value := args[index+1]
			if len(value) == 0 || len(value) > maxDeliveryValueSize {
				return result, fmt.Errorf("%s value must be between 1 and %d bytes", flag, maxDeliveryValueSize)
			}
			switch flag {
			case "--workspace":
				result.workspacePath = value
			case "--suite":
				result.suitePath = value
			case "--case":
				result.casePath = value
			case "--experiment-id":
				result.experimentID = value
			case "--variant":
				result.variant = value
			case "--seed":
				seed, err := strconv.ParseInt(value, 10, 64)
				if err != nil || seed < 0 {
					return result, errors.New("--seed must be a non-negative base-10 integer")
				}
				result.seed = &seed
			case "--output":
				result.outputPath = value
			}
			index++
		default:
			return result, fmt.Errorf("unknown argument %q", flag)
		}
	}
	if result.experimentID == "" || result.variant == "" || (result.suitePath == "") == (result.casePath == "") {
		return result, errors.New("exactly one of --suite or --case, --experiment-id, and --variant are required")
	}
	resolved, resErr := resolveWorkspace(result.workspacePath)
	if resErr != nil {
		return result, resErr
	}
	result.workspacePath = resolved
	return result, nil
}

func parsePromotionFlags(args []string) (promotionFlags, error) {
	var result promotionFlags
	if len(args) > maxDeliveryArgs {
		return result, errors.New("too many arguments")
	}
	seen := map[string]bool{}
	for index := 0; index < len(args); index++ {
		flag := args[index]
		if seen[flag] {
			return result, fmt.Errorf("%s may only be provided once", flag)
		}
		seen[flag] = true
		switch flag {
		case "--json":
			result.jsonOutput = true
		case "--yes":
			result.yes = true
		case "--workspace", "--resolution-id", "--output":
			if index+1 == len(args) || strings.HasPrefix(args[index+1], "-") {
				return result, fmt.Errorf("%s requires a value", flag)
			}
			value := args[index+1]
			if len(value) == 0 || len(value) > maxDeliveryValueSize {
				return result, fmt.Errorf("%s value must be between 1 and %d bytes", flag, maxDeliveryValueSize)
			}
			switch flag {
			case "--workspace":
				result.workspacePath = value
			case "--resolution-id":
				result.resolutionID = value
			case "--output":
				result.outputPath = value
			}
			index++
		default:
			return result, fmt.Errorf("unknown argument %q", flag)
		}
	}
	if result.resolutionID == "" {
		return result, errors.New("--resolution-id is required")
	}
	if result.yes && result.outputPath == "" {
		return result, errors.New("--output is required when --yes is provided")
	}
	resolved, resErr := resolveWorkspace(result.workspacePath)
	if resErr != nil {
		return result, resErr
	}
	result.workspacePath = resolved
	return result, nil
}

func parseEvaluationFlags(args []string, inputFlag string, allowPartition bool) (evaluationFlags, error) {
	var result evaluationFlags
	if len(args) > maxDeliveryArgs {
		return result, errors.New("too many arguments")
	}
	seen := map[string]bool{}
	for index := 0; index < len(args); index++ {
		flag := args[index]
		if seen[flag] {
			return result, fmt.Errorf("%s may only be provided once", flag)
		}
		seen[flag] = true
		switch flag {
		case "--json":
			result.jsonOutput = true
		case "--workspace", inputFlag, "--manifest", "--output", "--partition":
			if flag == "--partition" && !allowPartition {
				return result, errors.New("--partition is not valid for resolution replay")
			}
			if index+1 == len(args) || strings.HasPrefix(args[index+1], "-") {
				return result, fmt.Errorf("%s requires a value", flag)
			}
			value := args[index+1]
			if len(value) == 0 || len(value) > maxDeliveryValueSize {
				return result, fmt.Errorf("%s value must be between 1 and %d bytes", flag, maxDeliveryValueSize)
			}
			switch flag {
			case "--workspace":
				result.workspacePath = value
			case inputFlag:
				result.inputPath = value
			case "--manifest":
				result.manifestPath = value
			case "--output":
				result.outputPath = value
			case "--partition":
				result.partition = evaluation.Partition(value)
			}
			index++
		default:
			return result, fmt.Errorf("unknown argument %q", flag)
		}
	}
	if result.inputPath == "" || result.manifestPath == "" {
		return result, fmt.Errorf("%s and --manifest are required", inputFlag)
	}
	resolved, resErr := resolveWorkspace(result.workspacePath)
	if resErr != nil {
		return result, resErr
	}
	result.workspacePath = resolved
	if result.partition != "" && result.partition != evaluation.PartitionDevelopment && result.partition != evaluation.PartitionCalibration && result.partition != evaluation.PartitionHeldOut {
		return result, fmt.Errorf("--partition must be development, calibration, or held_out")
	}
	return result, nil
}

func finishEvaluation(ctx context.Context, report evaluation.Report, operationErr error, flags evaluationFlags, label string, stdout, stderr io.Writer) int {
	if errors.Is(operationErr, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return writeWorkspaceResult(app.Result{}, context.Canceled, stdout, stderr, flags.jsonOutput)
	}
	if operationErr != nil {
		return writeInvalidWorkspace(stdout, stderr, flags.jsonOutput, operationErr)
	}
	contents, err := evaluation.MarshalReport(report)
	if err != nil {
		return writeInvalidWorkspace(stdout, stderr, flags.jsonOutput, fmt.Errorf("marshal evaluation report: %w", err))
	}
	if flags.outputPath != "" {
		if err := writeNewFileAtomic(flags.outputPath, contents); err != nil {
			return writeInvalidRequest(stdout, stderr, flags.jsonOutput, err.Error(), "Choose a new regular output path and retry.")
		}
	}
	if flags.jsonOutput {
		p := termui.New(stdout)
		p.Raw(string(contents))
		if p.Err() != nil {
			pErr := termui.New(stderr)
			pErr.Error("Unable to write the evaluation report.", p.Err().Error(), "Check the output destination and retry.")
			return 1
		}
		return 0
	}
	correct := 0
	for _, outcome := range report.Outcomes {
		if outcome.Correct {
			correct++
		}
	}
	p := termui.New(stdout)
	p.Line(fmt.Sprintf("%s complete: %d samples, %d correct.", label, report.Samples, correct))
	top1 := report.Metrics.AcceptableTop1
	p.Line(fmt.Sprintf("Acceptable top-1: %d/%d.", top1.Numerator, top1.Denominator))
	if flags.outputPath != "" {
		p.Line(fmt.Sprintf("Report written to %s.", flags.outputPath))
	}
	return 0
}

func writePromotionSummary(draft telemetry.PromotionDraft, contents []byte, flags promotionFlags, stdout, stderr io.Writer) int {
	p := termui.New(stdout)
	p.Line(fmt.Sprintf("Promotion draft for resolution %s (review required; incomplete).", draft.ResolutionID))
	p.Line("Sanitized draft:")
	p.Raw(string(contents))
	if !strings.HasSuffix(string(contents), "\n") {
		p.Raw("\n")
	}
	p.Line(fmt.Sprintf("Removed fields: %s.", promotionFieldList(draft.Sanitization.RemovedFields)))
	p.Line(fmt.Sprintf("Unavailable fields: %s.", promotionFieldList(draft.Sanitization.UnavailableFields)))
	if flags.yes {
		p.Line(fmt.Sprintf("Draft written to %s.", flags.outputPath))
		p.Line("Next step: review and complete every required human field, then add the sanitized case through the normal reviewed canonical workflow.")
	} else {
		p.Line("No file was written.")
		if flags.outputPath == "" {
			p.Line("Next step: choose a non-canonical draft path, review this preview, then rerun with --output <new-file> --yes.")
		} else {
			p.Line(fmt.Sprintf("Next step: review this preview, then rerun with --output %s --yes.", flags.outputPath))
		}
	}
	if p.Err() != nil {
		pErr := termui.New(stderr)
		pErr.Error("Unable to write the promotion preview.", p.Err().Error(), "Check the output destination and retry.")
		return 1
	}
	return 0
}

func promotionFieldList(fields []string) string {
	if len(fields) == 0 {
		return "none"
	}
	return strings.Join(fields, ", ")
}

func validateNewOutputPath(path string) error {
	if path == "" || len(path) > maxDeliveryValueSize {
		return fmt.Errorf("output path must be between 1 and %d bytes", maxDeliveryValueSize)
	}
	info, err := os.Lstat(filepath.Clean(path))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect output path: %w", err)
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return errors.New("output path must not be a symlink")
	}
	if !info.Mode().IsRegular() {
		return errors.New("output path must be a regular file")
	}
	return errors.New("output path already exists; refusing to overwrite")
}

func writeNewFileAtomic(path string, contents []byte) error {
	path = filepath.Clean(path)
	if err := validateNewOutputPath(path); err != nil {
		return err
	}
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	temporary, err := os.CreateTemp(parent, ".skillhub-report-*.json")
	if err != nil {
		return fmt.Errorf("create temporary report: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err == nil {
		_, err = temporary.Write(contents)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("write temporary report: %w", err)
	}
	return publishFileWithoutOverwrite(temporaryPath, path)
}

func publishFileWithoutOverwrite(source, target string) error {
	if err := validateNewOutputPath(target); err != nil {
		return err
	}
	if err := os.Link(source, target); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return errors.New("output path already exists; refusing to overwrite")
		}
		return fmt.Errorf("publish output atomically: %w", err)
	}
	if err := os.Remove(source); err != nil {
		return fmt.Errorf("remove output staging file: %w", err)
	}
	if runtime.GOOS == "windows" {
		return nil
	}
	directory, err := os.Open(filepath.Dir(target))
	if err != nil {
		return fmt.Errorf("open output directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil && !errors.Is(err, fs.ErrInvalid) {
		return fmt.Errorf("sync output directory: %w", err)
	}
	return nil
}
