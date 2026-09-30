package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/evaluation"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
	"github.com/vantt/mcp-skill-hub/internal/version"
)

func TestEvaluationCLICommittedArtifactsRunAndRegenerateDeterministically(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate evaluation test source")
	}
	repositoryRoot := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", "..", ".."))
	fixtureRoot := filepath.Join(repositoryRoot, "testdata", "evaluation")
	workspace := filepath.Join(t.TempDir(), "workspace")
	suitePath := filepath.Join(fixtureRoot, "cli-suite-v1.json")
	manifestPath := filepath.Join(fixtureRoot, "cli-manifest-v1.json")

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", workspace, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	copyEvaluationFixtureTree(t, filepath.Join(fixtureRoot, "workspace-overlay"), workspace)
	if err := os.RemoveAll(filepath.Join(workspace, "runtime", "catalog")); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"rebuild", "--workspace", workspace, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("rebuild=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	reportPath := filepath.Join(t.TempDir(), "cli-evaluation-report.json")
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"eval", "run", "--workspace", workspace, "--suite", suitePath, "--manifest", manifestPath, "--output", reportPath, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("eval=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var report evaluation.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	committedManifest, err := evaluation.LoadManifest(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if report.Samples != 4 || len(report.Outcomes) != 4 || len(report.Exclusions) != 0 {
		t.Fatalf("unexpected committed evaluation population: samples=%d outcomes=%d exclusions=%v", report.Samples, len(report.Outcomes), report.Exclusions)
	}
	if !reflect.DeepEqual(report.Manifest, committedManifest) {
		t.Fatalf("report pins differ from committed manifest\nreport=%#v\ncommitted=%#v", report.Manifest, committedManifest)
	}
	for _, outcome := range report.Outcomes {
		if !outcome.Correct {
			t.Errorf("committed case %q was not correct: %#v", outcome.CaseID, outcome)
		}
	}
	writtenReport, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(writtenReport, stdout.Bytes()) {
		t.Fatal("evaluation output differs from the archived report")
	}

	regeneratedPath := filepath.Join(t.TempDir(), "cli-manifest-v1.json")
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"eval", "manifest", "--workspace", workspace, "--suite", suitePath, "--experiment-id", committedManifest.ExperimentID, "--variant", committedManifest.Variant, "--seed", "260929", "--output", regeneratedPath, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("manifest=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	committedBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	regeneratedBytes, err := os.ReadFile(regeneratedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(regeneratedBytes, committedBytes) {
		t.Fatalf("regenerated manifest differs from committed artifact\nregenerated=%s\ncommitted=%s", regeneratedBytes, committedBytes)
	}
}

func copyEvaluationFixtureTree(t *testing.T, source, destination string) {
	t.Helper()
	if err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !entry.Type().IsRegular() {
			return &os.PathError{Op: "copy fixture", Path: path, Err: os.ErrInvalid}
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(target, contents, info.Mode().Perm())
	}); err != nil {
		t.Fatal(err)
	}
}

func TestEvaluationCLIManifestGeneratesRunnableArtifactWithoutCanonicalMutation(t *testing.T) {
	root, suitePath, _, _ := evaluationCLIFixtures(t)
	canonicalBefore := snapshotCanonicalEvaluationAndPolicy(t, root)
	base := []string{"eval", "manifest", "--workspace", root, "--suite", suitePath, "--experiment-id", "generated-cli", "--variant", "production-deterministic"}
	var stdout, stderr bytes.Buffer

	if code := Run(append(append([]string(nil), base...), "--json"), &stdout, &stderr); code != 0 {
		t.Fatalf("manifest preview=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var preview evaluation.Manifest
	if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Seed != app.EvaluationDefaultSeed || preview.Model != nil {
		t.Fatalf("preview manifest=%#v", preview)
	}

	output := filepath.Join(t.TempDir(), "artifacts", "manifest.json")
	stdout.Reset()
	stderr.Reset()
	generate := append(append([]string(nil), base...), "--seed", "7", "--output", output, "--json")
	if code := Run(generate, &stdout, &stderr); code != 0 {
		t.Fatalf("manifest write=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	written, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(written, stdout.Bytes()) {
		t.Fatalf("written manifest differs from preview\nwritten=%s\nstdout=%s", written, stdout.String())
	}
	manifest, err := evaluation.LoadManifest(output)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Seed != 7 || manifest.ExperimentID != "generated-cli" || manifest.Variant != "production-deterministic" {
		t.Fatalf("manifest=%#v", manifest)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"eval", "run", "--workspace", root, "--suite", suitePath, "--manifest", output, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("run generated manifest=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if canonicalAfter := snapshotCanonicalEvaluationAndPolicy(t, root); !reflect.DeepEqual(canonicalBefore, canonicalAfter) {
		t.Fatalf("manifest generation or run changed canonical files\nbefore=%v\nafter=%v", canonicalBefore, canonicalAfter)
	}

	if err := os.RemoveAll(filepath.Join(root, "runtime", "catalog", "generations")); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"eval", "run", "--workspace", root, "--suite", suitePath, "--manifest", output, "--json"}, &stdout, &stderr); code == 0 || !strings.Contains(stdout.String(), evaluation.ErrPinnedArtifactUnavailable.Error()) {
		t.Fatalf("unavailable pin code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestEvaluationCLIManifestRejectsInvalidArgumentsAndOverwrite(t *testing.T) {
	root, suitePath, _, _ := evaluationCLIFixtures(t)
	output := filepath.Join(t.TempDir(), "manifest.json")
	if err := os.WriteFile(output, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	base := []string{"eval", "manifest", "--workspace", root, "--suite", suitePath, "--experiment-id", "experiment", "--variant", "candidate"}
	if code := Run(append(append([]string(nil), base...), "--output", output, "--json"), &stdout, &stderr); code != 2 || !strings.Contains(stdout.String(), "refusing to overwrite") {
		t.Fatalf("overwrite code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	contents, err := os.ReadFile(output)
	if err != nil || string(contents) != "keep" {
		t.Fatalf("existing output changed: %q %v", contents, err)
	}

	for _, args := range [][]string{
		append(append([]string(nil), base...), "--seed", "not-a-number", "--json"),
		append(append([]string(nil), base...), "--variant", "duplicate", "--json"),
		{"eval", "manifest", "--workspace", root, "--suite", suitePath, "--experiment-id", "missing-variant", "--json"},
	} {
		stdout.Reset()
		stderr.Reset()
		if code := Run(args, &stdout, &stderr); code != 2 {
			t.Fatalf("invalid args %v code=%d stdout=%s stderr=%s", args, code, stdout.String(), stderr.String())
		}
	}
}

func TestEvaluationCLIProducesDeterministicJSONAndAtomicOutput(t *testing.T) {
	root, suitePath, manifestPath, _ := evaluationCLIFixtures(t)
	output := filepath.Join(t.TempDir(), "reports", "evaluation.json")
	args := []string{"eval", "run", "--workspace", root, "--suite", suitePath, "--manifest", manifestPath, "--partition", "held_out", "--output", output, "--json"}
	var stdout, stderr bytes.Buffer
	if code := Run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("eval=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	written, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(written, stdout.Bytes()) {
		t.Fatalf("stdout and report differ\nstdout=%s\nreport=%s", stdout.String(), written)
	}
	var report evaluation.Report
	if err := json.Unmarshal(written, &report); err != nil {
		t.Fatal(err)
	}
	if report.Samples != 1 || len(report.Outcomes) != 1 || report.Outcomes[0].Partition != evaluation.PartitionHeldOut {
		t.Fatalf("report=%#v", report)
	}

	first := append([]byte(nil), written...)
	stdout.Reset()
	stderr.Reset()
	if code := Run(args, &stdout, &stderr); code != 2 {
		t.Fatalf("overwrite code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	written, err = os.ReadFile(output)
	if err != nil || !bytes.Equal(first, written) {
		t.Fatalf("existing report changed: err=%v", err)
	}

	secondOutput := filepath.Join(t.TempDir(), "evaluation.json")
	stdout.Reset()
	stderr.Reset()
	args[len(args)-2] = secondOutput
	if code := Run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("second eval=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !reflect.DeepEqual(first, stdout.Bytes()) {
		t.Fatalf("report bytes are nondeterministic\nfirst=%s\nsecond=%s", first, stdout.String())
	}
}

func TestEvaluationPromotionCLIPreviewsAndWritesSanitizedReviewDraft(t *testing.T) {
	root, _, _, _ := evaluationCLIFixtures(t)
	seedPromotionTelemetry(t, root, "res-promote", "code-review")
	canonicalBefore := snapshotCanonicalEvaluationAndPolicy(t, root)
	output := filepath.Join(t.TempDir(), "drafts", "promotion.json")
	base := []string{"eval", "promote", "--workspace", root, "--resolution-id", "res-promote", "--output", output}

	var stdout, stderr bytes.Buffer
	if code := Run(base, &stdout, &stderr); code != 0 {
		t.Fatalf("promote preview=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("preview wrote output: %v", err)
	}
	for _, text := range []string{"review required; incomplete", "Removed fields:", "Unavailable fields:", "No file was written.", "Next step:"} {
		if !strings.Contains(stdout.String(), text) {
			t.Fatalf("human preview omits %q: %s", text, stdout.String())
		}
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run(append(append([]string(nil), base...), "--json"), &stdout, &stderr); code != 0 {
		t.Fatalf("JSON preview=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var preview telemetry.PromotionDraft
	if err := json.Unmarshal(stdout.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if !preview.ReviewRequired || !preview.CaseTemplate.Incomplete || preview.Observed.TopSkillID != "code-review" {
		t.Fatalf("preview draft=%+v", preview)
	}
	if strings.Contains(stdout.String(), "private task text") {
		t.Fatalf("preview leaked raw task: %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	confirmed := append(append([]string(nil), base...), "--yes", "--json")
	if code := Run(confirmed, &stdout, &stderr); code != 0 {
		t.Fatalf("confirmed promote=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	written, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(written, stdout.Bytes()) {
		t.Fatalf("written draft differs from JSON result\nwritten=%s\nstdout=%s", written, stdout.String())
	}
	if canonicalAfter := snapshotCanonicalEvaluationAndPolicy(t, root); !reflect.DeepEqual(canonicalBefore, canonicalAfter) {
		t.Fatalf("promotion changed canonical evaluation or policy files\nbefore=%v\nafter=%v", canonicalBefore, canonicalAfter)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run(confirmed, &stdout, &stderr); code != 2 || !strings.Contains(stdout.String(), "refusing to overwrite") {
		t.Fatalf("overwrite code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	unchanged, err := os.ReadFile(output)
	if err != nil || !bytes.Equal(unchanged, written) {
		t.Fatalf("overwrite changed existing draft: %v", err)
	}
}

func TestEvaluationPromotionCLIRejectsUnsafeConfirmationAndResolutionErrors(t *testing.T) {
	root, _, _, _ := evaluationCLIFixtures(t)
	seedPromotionTelemetry(t, root, "res-conflict", "code-review")
	seedPromotionTelemetry(t, root, "res-conflict", "architecture-review")
	var stdout, stderr bytes.Buffer

	if code := Run([]string{"eval", "promote", "--workspace", root, "--resolution-id", "res-missing", "--json"}, &stdout, &stderr); code != 2 || !strings.Contains(stdout.String(), "telemetry resolution not found") {
		t.Fatalf("missing code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"eval", "promote", "--workspace", root, "--resolution-id", "res-conflict", "--json"}, &stdout, &stderr); code != 2 || !strings.Contains(stdout.String(), "telemetry resolution is ambiguous") {
		t.Fatalf("ambiguous code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"eval", "promote", "--workspace", root, "--resolution-id", "res-conflict", "--yes", "--json"}, &stdout, &stderr); code != 2 || !strings.Contains(stdout.String(), "--output is required") {
		t.Fatalf("missing output code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	target := filepath.Join(t.TempDir(), "target.json")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "draft.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"eval", "promote", "--workspace", root, "--resolution-id", "res-conflict", "--output", link, "--yes", "--json"}, &stdout, &stderr); code != 2 || !strings.Contains(stdout.String(), "must not be a symlink") {
		t.Fatalf("symlink code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	contents, err := os.ReadFile(target)
	if err != nil || string(contents) != "keep" {
		t.Fatalf("symlink target changed: %q %v", contents, err)
	}
}

func TestResolutionReplayCLIUsesStrictPinnedCase(t *testing.T) {
	root, _, manifestPath, casePath := evaluationCLIFixtures(t)
	manifestData, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest evaluation.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.SuiteDigest = ""
	caseData, err := os.ReadFile(casePath)
	if err != nil {
		t.Fatal(err)
	}
	var replayCase evaluation.Case
	if err := json.Unmarshal(caseData, &replayCase); err != nil {
		t.Fatal(err)
	}
	manifest.CaseDigest, err = evaluation.DigestCase(replayCase)
	if err != nil {
		t.Fatal(err)
	}
	writeCLIJSON(t, manifestPath, manifest)

	var stdout, stderr bytes.Buffer
	if code := Run([]string{"resolution", "replay", "--workspace", root, "--case", casePath, "--manifest", manifestPath, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("replay=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var report evaluation.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Samples != 1 || report.Outcomes[0].CaseID != replayCase.ID {
		t.Fatalf("report=%#v", report)
	}
}

func TestEvaluationCLIRejectsUnknownManifestFieldsInvalidPartitionsAndUnsafeOutput(t *testing.T) {
	root, suitePath, manifestPath, _ := evaluationCLIFixtures(t)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte(`"experiment_id"`), []byte(`"unknown":true,"experiment_id"`), 1)
	if err := os.WriteFile(manifestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	base := []string{"eval", "run", "--workspace", root, "--suite", suitePath, "--manifest", manifestPath, "--json"}
	var stdout, stderr bytes.Buffer
	if code := Run(base, &stdout, &stderr); code != 2 || !bytes.Contains(stdout.Bytes(), []byte(`"code":"invalid_request"`)) {
		t.Fatalf("strict manifest code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	_, _, validManifest, _ := evaluationCLIFixtures(t)
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"eval", "run", "--workspace", root, "--suite", suitePath, "--manifest", validManifest, "--partition", "latest", "--json"}, &stdout, &stderr); code != 2 {
		t.Fatalf("partition code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"eval", "run", "--workspace", root, "--workspace", root, "--suite", suitePath, "--manifest", validManifest, "--json"}, &stdout, &stderr); code != 2 {
		t.Fatalf("duplicate code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}

	target := filepath.Join(t.TempDir(), "target.json")
	if err := os.WriteFile(target, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "report.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"eval", "run", "--workspace", root, "--suite", suitePath, "--manifest", validManifest, "--output", link, "--json"}, &stdout, &stderr); code != 2 {
		t.Fatalf("symlink code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	contents, err := os.ReadFile(target)
	if err != nil || string(contents) != "keep" {
		t.Fatalf("symlink target changed: %q %v", contents, err)
	}
}

func TestEvaluationCLIPropagatesCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root, suitePath, manifestPath, _ := evaluationCLIFixtures(t)
	var stdout, stderr bytes.Buffer
	if code := RunContext(ctx, []string{"eval", "run", "--workspace", root, "--suite", suitePath, "--manifest", manifestPath, "--json"}, &stdout, &stderr); code != 130 {
		t.Fatalf("cancel code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func evaluationCLIFixtures(t *testing.T) (root, suitePath, manifestPath, casePath string) {
	t.Helper()
	root = filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	handle, err := catalog.OpenCurrent(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	policy, policyErr := resolverpkg.LoadPolicy(context.Background(), handle.DB)
	snapshot := handle.Pointer.CatalogSnapshot
	closeErr := handle.Close()
	if policyErr != nil {
		t.Fatal(policyErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	caseFixture := evaluation.Case{
		SchemaVersion: evaluation.SchemaVersion,
		ID:            "held-out-no-skill",
		Partition:     evaluation.PartitionHeldOut,
		Tags:          []string{"no-skill"},
		Request: resolverpkg.Request{
			SchemaVersion: resolverpkg.SchemaVersion,
			RequestID:     "case-held-out",
			Task:          resolverpkg.Task{Description: "find a nonexistent quantum gardening procedure"},
		},
		Expected: evaluation.Expected{AcceptableStatuses: []resolverpkg.Status{resolverpkg.StatusNoSkill}, NoSkill: true},
	}
	suite := evaluation.Suite{SchemaVersion: evaluation.SchemaVersion, ID: "cli-suite", Sanitization: "reviewed", Cases: []evaluation.Case{caseFixture}}
	suitePath = filepath.Join(t.TempDir(), "suite.json")
	casePath = filepath.Join(t.TempDir(), "case.json")
	writeCLIJSON(t, suitePath, suite)
	writeCLIJSON(t, casePath, caseFixture)
	digest, err := evaluation.DigestSuite(suite)
	if err != nil {
		t.Fatal(err)
	}
	build := version.Current()
	manifest := evaluation.Manifest{
		SchemaVersion:        evaluation.SchemaVersion,
		ExperimentID:         "cli-evaluation",
		SuiteDigest:          digest,
		CatalogSnapshot:      snapshot,
		PolicyRevision:       policy.Revision,
		ProtocolSchema:       resolverpkg.SchemaVersion,
		NormalizationVersion: app.EvaluationNormalizationVersion,
		IndexVersion:         app.EvaluationIndexVersion,
		FactProviderFixture:  app.EvaluationFactProviderFixture,
		FactProviderVersion:  app.EvaluationFactProviderVersion,
		Seed:                 42,
		Binary:               evaluation.BinaryIdentity{Version: build.Version, Commit: build.Commit},
		Variant:              "production-deterministic",
	}
	manifestPath = filepath.Join(t.TempDir(), "manifest.json")
	writeCLIJSON(t, manifestPath, manifest)
	return root, suitePath, manifestPath, casePath
}

func seedPromotionTelemetry(t *testing.T, root, resolutionID, skillID string) {
	t.Helper()
	recorder, err := (app.TelemetryService{}).Open(root)
	if err != nil {
		t.Fatal(err)
	}
	recorder.Record(telemetry.Event{
		Type: telemetry.EventResolutionCompleted, ResolutionID: resolutionID, RequestID: "req-private",
		CatalogSnapshot: "sha256:catalog", PolicyRevision: "sha256:policy", Client: telemetry.Client{Name: "test"},
		Payload: map[string]any{"status": "resolved", "top_skill_id": skillID, "reason_codes": []string{"trigger_match"}, "operation": "review"},
	})
	// Raw task content is intentionally attempted and rejected by the telemetry
	// allowlist; the valid event above remains sufficient for a promotion draft.
	recorder.Record(telemetry.Event{
		Type: telemetry.EventResolutionCompleted, ResolutionID: resolutionID,
		CatalogSnapshot: "sha256:catalog", PolicyRevision: "sha256:policy", Client: telemetry.Client{Name: "test"},
		Payload: map[string]any{"status": "resolved", "task": "private task text"},
	})
	if err := recorder.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func snapshotCanonicalEvaluationAndPolicy(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := make(map[string]string)
	for _, relativeRoot := range []string{"evals", "config"} {
		base := filepath.Join(root, relativeRoot)
		err := filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			contents, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			snapshot[filepath.ToSlash(relative)] = string(contents)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return snapshot
}

func writeCLIJSON(t *testing.T, path string, value any) {
	t.Helper()
	contents, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(contents, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestEvaluationCLIRejectsOversizedFlagValue(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"eval", "run", "--workspace", strings.Repeat("w", maxDeliveryValueSize+1), "--suite", "suite.json", "--manifest", "manifest.json", "--json"}, &stdout, &stderr); code != 2 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"eval", "promote", "--workspace", "workspace", "--resolution-id", strings.Repeat("r", maxDeliveryValueSize+1), "--json"}, &stdout, &stderr); code != 2 {
		t.Fatalf("promote code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestResolutionReplayAcceptsCLIGeneratedCaseManifest(t *testing.T) {
	root, suitePath, _, casePath := evaluationCLIFixtures(t)
	manifestPath := filepath.Join(t.TempDir(), "case-manifest.json")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"eval", "manifest", "--workspace", root, "--case", casePath, "--experiment-id", "case-replay", "--variant", "production-deterministic", "--output", manifestPath, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("case manifest=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var manifest evaluation.Manifest
	if err := json.Unmarshal(stdout.Bytes(), &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.CaseDigest == "" || manifest.SuiteDigest != "" {
		t.Fatalf("manifest digests = case %q suite %q", manifest.CaseDigest, manifest.SuiteDigest)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"resolution", "replay", "--workspace", root, "--case", casePath, "--manifest", manifestPath, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("replay=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var report evaluation.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil || report.Samples != 1 {
		t.Fatalf("report=%#v err=%v", report, err)
	}

	stdout.Reset()
	stderr.Reset()
	both := []string{"eval", "manifest", "--workspace", root, "--case", casePath, "--suite", suitePath, "--experiment-id", "x", "--variant", "y", "--json"}
	if code := Run(both, &stdout, &stderr); code != 2 {
		t.Fatalf("suite and case together code=%d stdout=%s", code, stdout.String())
	}
}

func TestEvaluationHumanOutputCountsCorrectOutcomesLikeJSON(t *testing.T) {
	root, suitePath, manifestPath, _ := evaluationCLIFixtures(t)
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"eval", "run", "--workspace", root, "--suite", suitePath, "--manifest", manifestPath}, &stdout, &stderr); code != 0 {
		t.Fatalf("run=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "1 samples, 1 correct.") || !strings.Contains(stdout.String(), "Acceptable top-1: 0/0.") {
		t.Fatalf("human output = %q", stdout.String())
	}
}
