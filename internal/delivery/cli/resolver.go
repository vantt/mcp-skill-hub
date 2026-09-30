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
	"time"
	"unicode/utf8"

	"github.com/vantt/mcp-skill-hub/internal/app"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

const maxResolveRequestBytes = 64 << 10

func runResolve(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	workspacePath, requestPath, jsonOutput, err := resolveFlags(args)
	if err != nil {
		var resErr *WorkspaceResolutionError
		if errors.As(err, &resErr) {
			return writeWorkspaceResolutionError(stdout, stderr, hasJSONFlag(args), resErr)
		}
		return writeInvalidRequest(stdout, stderr, hasJSONFlag(args), err.Error(), "Run `skillhub resolve --workspace <path> --request <request.json> --json`.")
	}
	request, err := readResolveRequest(requestPath)
	if err != nil {
		return writeInvalidRequest(stdout, stderr, jsonOutput, err.Error(), "Correct the versioned resolve request and retry.")
	}
	service := app.ResolverService{}
	// Telemetry is best-effort and disposable. Failure to open, record, flush, or
	// close it must never alter the resolver response or process exit status.
	if recorder, telemetryErr := (app.TelemetryService{}).Open(workspacePath); telemetryErr == nil {
		service.Telemetry = recorder
		defer closeResolutionTelemetry(recorder)
	}
	response, err := service.Resolve(ctx, workspacePath, request)
	if errors.Is(err, context.Canceled) {
		return writeWorkspaceResult(app.Result{}, err, stdout, stderr, jsonOutput)
	}
	if err != nil {
		if classified := classifyResolveError(err); classified != nil {
			return writeStructuredError(stdout, stderr, jsonOutput, classified)
		}
		return writeInvalidWorkspace(stdout, stderr, jsonOutput, err)
	}
	if jsonOutput {
		if err := writeJSON(stdout, response); err != nil {
			fmt.Fprintf(stderr, "ERROR: Unable to write the resolution response.\nWHY: %v\nFIX: Check the output destination and retry.\n", err)
			return 1
		}
		return 0
	}
	switch response.Status {
	case resolverpkg.StatusResolved:
		fmt.Fprintf(stdout, "Resolved %s (%s confidence). Host approval is required before activation.\n", response.Primary.ID, response.Primary.Confidence)
	case resolverpkg.StatusNeedsContext:
		fmt.Fprintln(stdout, response.Question.Text)
		if len(response.Question.Choices) > 0 {
			fmt.Fprintf(stdout, "Choices: %s\n", strings.Join(response.Question.Choices, " | "))
		}
		field := response.Question.Field
		if field == "" {
			field = "scope"
		}
		sampleChoice := "single_step"
		if len(response.Question.Choices) > 0 {
			sampleChoice = response.Question.Choices[0]
		}
		fmt.Fprintf(stdout, "\nTo answer, set %q in your request JSON, e.g.:\n  {\"task\": {\"description\": \"...\", %q: %q}}\n", field, field, sampleChoice)
		if response.ResolutionID != "" {
			fmt.Fprintf(stdout, "or provide prior in your request:\n  {\"prior\": {\"resolution_id\": %q, \"answers\": {%q: %q}}}\n", response.ResolutionID, field, sampleChoice)
		}
	case resolverpkg.StatusAlreadyCovered:
		fmt.Fprintf(stdout, "The active procedure %s already covers this task.\n", response.CoveredBy)
	case resolverpkg.StatusNoSkill:
		fmt.Fprintf(stdout, "No skill recommended: %s.\n", response.NoSkill.ReasonCode)
	}
	return 0
}

// classifyResolveError maps resolver failures to the same stable codes the MCP
// adapter reports, so both transports agree. It returns nil for failures that
// are genuinely workspace problems.
func classifyResolveError(err error) *app.Error {
	message := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, skill.ErrSnapshotExpired):
		return app.NewSnapshotExpiredError(err.Error())
	case strings.Contains(message, "prior clarification was not issued"):
		return &app.Error{Code: app.ErrorUnknownResolution, Retryable: true, Render: app.ErrorRender{
			Error: "The prior resolution is unavailable.",
			Why:   err.Error(),
			Fix:   "Start a new resolution request without a prior.",
		}}
	case strings.Contains(message, "prior clarification is stale") || strings.Contains(message, "does not match this request"):
		return &app.Error{Code: app.ErrorStaleContext, Retryable: true, Render: app.ErrorRender{
			Error: "The prior resolution no longer matches the current request.",
			Why:   err.Error(),
			Fix:   "Start a new resolution with the current context.",
		}}
	case strings.Contains(message, "catalog is stale") || strings.Contains(message, "catalog is missing") || strings.Contains(message, "catalog is corrupt"):
		return &app.Error{Code: app.ErrorIndexStale, Retryable: true, Render: app.ErrorRender{
			Error: "The derived catalog is unavailable or stale.",
			Why:   err.Error(),
			Fix:   "Run `skillhub rebuild --workspace <path>`, then retry.",
		}}
	}
	return nil
}

func writeStructuredError(stdout, stderr io.Writer, jsonOutput bool, failure *app.Error) int {
	result := app.ErrorResult(failure)
	if jsonOutput {
		if err := writeJSON(stdout, result); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		return 2
	}
	fmt.Fprintf(stderr, "ERROR: %s\nWHY: %s\nFIX: %s\n", failure.Render.Error, failure.Render.Why, failure.Render.Fix)
	return 2
}

func closeResolutionTelemetry(recorder *telemetry.Recorder) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = recorder.Close(ctx)
}

func resolveFlags(args []string) (workspacePath, requestPath string, jsonOutput bool, err error) {
	for index := 0; index < len(args); index++ {
		switch args[index] {
		case "--json":
			jsonOutput = true
		case "--workspace", "--request":
			if index+1 == len(args) || strings.HasPrefix(args[index+1], "-") {
				return "", "", jsonOutput, fmt.Errorf("%s requires a value", args[index])
			}
			if args[index] == "--workspace" {
				workspacePath = args[index+1]
			} else {
				requestPath = args[index+1]
			}
			index++
		default:
			return "", "", jsonOutput, fmt.Errorf("unknown argument %q", args[index])
		}
	}
	if requestPath == "" {
		return "", "", jsonOutput, errors.New("--request is required")
	}
	resolved, resErr := resolveWorkspace(workspacePath)
	if resErr != nil {
		return "", "", jsonOutput, resErr
	}
	return resolved, requestPath, jsonOutput, nil
}

func readResolveRequest(path string) (resolverpkg.Request, error) {
	var request resolverpkg.Request
	file, err := os.Open(path)
	if err != nil {
		return request, fmt.Errorf("read request: %w", err)
	}
	defer file.Close()
	limited := io.LimitReader(file, maxResolveRequestBytes+1)
	contents, err := io.ReadAll(limited)
	if err != nil {
		return request, fmt.Errorf("read request: %w", err)
	}
	if len(contents) > maxResolveRequestBytes {
		return request, fmt.Errorf("request exceeds %d bytes", maxResolveRequestBytes)
	}
	if !utf8.Valid(contents) {
		return request, fmt.Errorf("request must be valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(contents))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, fmt.Errorf("parse request: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return request, fmt.Errorf("request contains trailing data")
	}
	if _, err := resolverpkg.NormalizeRequest(request); err != nil {
		return request, err
	}
	return request, nil
}
