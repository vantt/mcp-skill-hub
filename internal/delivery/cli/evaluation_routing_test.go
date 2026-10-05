package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

func setupOverlayTestWorkspace(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init=%d: %s", code, stderr.String())
	}

	overlaySource, err := filepath.Abs("../../../testdata/evaluation/workspace-overlay/skills")
	if err != nil {
		t.Fatal(err)
	}
	targetSkills := filepath.Join(root, "skills")
	if err := copyDir(overlaySource, targetSkills); err != nil {
		t.Fatalf("copy overlay skills: %v", err)
	}

	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"rebuild", "--workspace", root, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("rebuild=%d: %s", code, stderr.String())
	}
	return root
}

func TestEvaluationRoutingCLI(t *testing.T) {
	t.Parallel()
	root := setupOverlayTestWorkspace(t)
	var stdout, stderr bytes.Buffer

	// 1. Human output: exit 0
	if code := Run([]string{"eval", "routing", "--workspace", root}, &stdout, &stderr); code != 0 {
		t.Fatalf("eval routing human code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "Routing Evaluation Summary") || !strings.Contains(out, "Precision@1") || !strings.Contains(out, "Gate: PASS") {
		t.Fatalf("unexpected human output:\n%s", out)
	}

	// 2. JSON output: exit 0
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"eval", "routing", "--workspace", root, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("eval routing json code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var rep app.RoutingEvalReport
	if err := json.Unmarshal(stdout.Bytes(), &rep); err != nil {
		t.Fatalf("unmarshal eval report: %v", err)
	}
	// 3 skills with 3 examples each = 9 positives, 6 counters
	if rep.TotalPositives != 9 {
		t.Errorf("TotalPositives = %d, want 9", rep.TotalPositives)
	}
	if rep.TotalCounters != 6 {
		t.Errorf("TotalCounters = %d, want 6", rep.TotalCounters)
	}
	if !rep.PassedGate {
		t.Errorf("expected PassedGate=true without thresholds")
	}

	// 3. Failed threshold: exit 1
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"eval", "routing", "--workspace", root, "--min-precision", "1.0"}, &stdout, &stderr); code != 0 && code != 1 {
		t.Fatalf("unexpected exit code for threshold: %d", code)
	}

	// 4. Invalid flag value: exit 2
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"eval", "routing", "--workspace", root, "--min-precision", "not-a-number"}, &stdout, &stderr); code != 2 {
		t.Fatalf("invalid flag value code=%d, want 2", code)
	}
}

func TestEvaluationHelpIncludesRouting(t *testing.T) {
	t.Parallel()
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"help", "eval"}, &stdout, &stderr); code != 0 {
		t.Fatalf("help eval code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "routing") {
		t.Errorf("help eval missing routing subcommand:\n%s", out)
	}
}
