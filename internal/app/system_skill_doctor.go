package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/systemskills"
)

// systemSkillProvider adds notices about the system skills installed in the
// hub folder to doctor. Its findings are notices with no fix, so they never
// change the doctor status or stop doctor --fix. Whether the skill files
// themselves are current, missing or changed is reported by the hub's own
// agent connection check, which repairs them through the host plan.
type systemSkillProvider struct {
	// lookPath and nodeVersion are replaced in tests.
	lookPath    func(name string) (string, error)
	nodeVersion func(ctx context.Context, path string) (string, error)
}

func init() { RegisterProvider(newSystemSkillProvider()) }

func newSystemSkillProvider() systemSkillProvider {
	return systemSkillProvider{lookPath: exec.LookPath, nodeVersion: runNodeVersion}
}

const (
	distillLabNodeMajor   = 18
	nodeVersionTimeout    = 5 * time.Second
	distillLabNodeMissing = "distill_lab_node_missing"
)

func (systemSkillProvider) Name() string { return "system-skills" }

func (systemSkillProvider) Applies(report RoleReport) bool { return report.HealthHub() != "" }

func (provider systemSkillProvider) Check(ctx context.Context, report RoleReport, depth Depth) []Finding {
	if depth != DepthFull {
		return nil
	}
	root := report.HealthHub()
	var findings []Finding
	if finding, ok := provider.nodeFinding(ctx, root); ok {
		findings = append(findings, finding)
	}
	return findings
}

// nodeFinding reports a hub whose distill-lab cannot run its script: Node is
// missing, does not answer, or is older than the version the script needs.
func (provider systemSkillProvider) nodeFinding(ctx context.Context, root string) (Finding, bool) {
	entry := filepath.Join(root, ".agents", "skills", systemskills.DistillSkillID, "SKILL.md")
	if _, err := os.Lstat(entry); err != nil {
		return Finding{}, false
	}
	found := "not found"
	if path, err := provider.lookPath("node"); err == nil {
		versionCtx, cancel := context.WithTimeout(ctx, nodeVersionTimeout)
		output, runErr := provider.nodeVersion(versionCtx, path)
		cancel()
		version := strings.TrimSpace(output)
		major, parsed := nodeMajor(version)
		switch {
		case runErr != nil:
			found = "found " + path + ", but `node --version` failed"
		case !parsed:
			found = "found " + path + ", but its version could not be read"
		case major >= distillLabNodeMajor:
			return Finding{}, false
		default:
			found = "found " + version
		}
	}
	return Finding{Code: distillLabNodeMissing, Label: "distill-lab", Severity: SeverityNotice,
		Message: "distill-lab runs scripts/distill.mjs with Node " + strconv.Itoa(distillLabNodeMajor) + " or newer; " + found + ".",
		Manual:  "Install Node from https://nodejs.org"}, true
}

var nodeVersionPattern = regexp.MustCompile(`^v(\d+)\.\d+\.\d+`)

func nodeMajor(version string) (int, bool) {
	match := nodeVersionPattern.FindStringSubmatch(version)
	if match == nil {
		return 0, false
	}
	major, err := strconv.Atoi(match[1])
	return major, err == nil
}

func runNodeVersion(ctx context.Context, path string) (string, error) {
	output, err := exec.CommandContext(ctx, path, "--version").Output()
	return string(output), err
}
