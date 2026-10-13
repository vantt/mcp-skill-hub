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

	"github.com/vantt/mcp-skill-hub/internal/hostintegration"
	"github.com/vantt/mcp-skill-hub/internal/shellquote"
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
	findings = append(findings, systemSkillGitFindings(ctx, root)...)
	return findings
}

// systemSkillFolders are the hub folders, relative and with forward slashes,
// that hold the generated system skill files; the hub .gitignore ignores them.
func systemSkillFolders() []string {
	var folders []string
	for _, skill := range systemskills.SystemSkills() {
		folders = append(folders, ".agents/skills/"+skill.ID+"/", ".claude/skills/"+skill.ID+"/")
	}
	return folders
}

// systemSkillGitFindings names the files in the generated system skill
// folders whose Git state needs a person: a generated file that Git still
// tracks (hubs created before the folders were ignored committed them), and
// a file Skill Hub did not write that the ignore entries now hide. The
// commands are printed for a person to run; nothing here changes Git.
func systemSkillGitFindings(ctx context.Context, root string) []Finding {
	var findings []Finding
	folders := systemSkillFolders()
	tracked, err := gitPaths(ctx, root, append([]string{"ls-files", "-z", "--"}, folders...)...)
	if err != nil {
		return nil
	}
	var owned []string
	for _, path := range tracked {
		if raw, ok := readGeneratedFile(root, path); ok && hostintegration.SystemSkillFileOwned(path, raw) {
			owned = append(owned, path)
		}
	}
	if len(owned) > 0 {
		owned = sortedUnique(owned)
		findings = append(findings, Finding{Code: "system_skill_tracked", Label: "Tracked system skills", Severity: SeverityNotice,
			Message: "Git tracks " + strings.Join(owned, ", ") + ", which Skill Hub regenerates from its version; stop tracking them with the command below, then commit.",
			Manual:  untrackCommand(root, owned), untrack: owned})
	}
	ignored, err := gitPaths(ctx, root, append([]string{"ls-files", "-z", "--others", "--ignored", "--exclude-standard", "--"}, folders...)...)
	if err != nil {
		return findings
	}
	for _, path := range ignored {
		if raw, ok := readGeneratedFile(root, path); !ok || !hostintegration.SystemSkillFileOwned(path, raw) {
			findings = append(findings, Finding{Code: "system_skill_unowned_ignored", Label: "Ignored file", Severity: SeverityNotice,
				Message: path + " is ignored by .gitignore and was not written by Skill Hub; move it to another skill folder, or keep it in Git with the command below.",
				Manual:  "git -C " + shellquote.Arg(root) + " add -f -- " + shellquote.Arg(path)})
		}
	}
	return findings
}

// gitPaths runs a NUL-separated git listing in root.
func gitPaths(ctx context.Context, root string, args ...string) ([]string, error) {
	output, err := offlineGitCommand(ctx, root, args...).Output()
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, path := range strings.Split(string(output), "\x00") {
		if path != "" {
			paths = append(paths, path)
		}
	}
	return paths, nil
}

// maxGeneratedFileBytes bounds what the ownership check reads; every bundled
// system skill file is far smaller.
const maxGeneratedFileBytes = 4 << 20

// readGeneratedFile reads a regular file below root by its slash path,
// refusing symbolic links and large files.
func readGeneratedFile(root, slashPath string) ([]byte, bool) {
	path := filepath.Join(root, filepath.FromSlash(slashPath))
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxGeneratedFileBytes {
		return nil, false
	}
	raw, err := os.ReadFile(path)
	return raw, err == nil
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
		// Installing Node is no single command, so the advice is part of
		// the message and Manual stays empty.
		Message: "distill-lab runs scripts/distill.mjs with Node " + strconv.Itoa(distillLabNodeMajor) + " or newer; " + found + ". Install Node from https://nodejs.org.",
		Reason:  "Skill Hub does not install Node; install it from https://nodejs.org."}, true
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
