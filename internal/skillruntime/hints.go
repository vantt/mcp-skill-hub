package skillruntime

import (
	"bytes"
	"path"
	"regexp"
	"slices"
	"strings"
)

// HintFile is one file of a skill folder: its path relative to the folder and
// its leading bytes. Only the leading bytes of large files are needed.
type HintFile struct {
	Path    string
	Content []byte
}

// Hints are static, content-free observations about what a skill may need from
// the host. They are computed without executing anything and carry only
// interpreter names, file paths, and cue names, never text from the skill.
type Hints struct {
	Interpreters         []string `json:"interpreters"`
	DependencyManifests  []string `json:"dependency_manifests"`
	MissingLockfiles     []string `json:"missing_lockfiles"`
	AbsoluteInstallPaths []string `json:"absolute_install_paths"`
	MissingRuntimeBlock  bool     `json:"missing_runtime_block"`
	InstallProseDetected bool     `json:"install_prose_detected"`
	InstallCues          []string `json:"install_cues"`
}

var (
	extensionInterpreters = map[string]string{
		".py": "python3", ".js": "node", ".mjs": "node", ".cjs": "node", ".ts": "node",
		".sh": "sh", ".bash": "bash", ".rb": "ruby", ".ps1": "pwsh",
	}
	shebangInterpreters = map[string]string{
		"python": "python3", "python3": "python3", "node": "node", "nodejs": "node",
		"sh": "sh", "bash": "bash", "ruby": "ruby", "pwsh": "pwsh", "powershell": "pwsh",
	}
	pythonVersionedName = regexp.MustCompile(`^python3\.\d+$`)

	skillInstallPathMarkers = []string{"~/.claude/skills/", "$HOME/.claude/skills", ".agents/skills/", ".codex/skills/"}

	installCommandCues = []struct {
		name    string
		pattern *regexp.Regexp
	}{
		{"pip install", regexp.MustCompile(`(?i)\bpip3?\s+install\b`)},
		{"npm install", regexp.MustCompile(`(?i)\bnpm\s+install\b`)},
		{"npm i", regexp.MustCompile(`(?i)\bnpm\s+i\s`)},
		{"brew install", regexp.MustCompile(`(?i)\bbrew\s+install\b`)},
		{"apt install", regexp.MustCompile(`(?i)\bapt\s+install\b`)},
		{"apt-get install", regexp.MustCompile(`(?i)\bapt-get\s+install\b`)},
		{"uv sync", regexp.MustCompile(`(?i)\buv\s+sync\b`)},
		{"poetry install", regexp.MustCompile(`(?i)\bpoetry\s+install\b`)},
		{"go install", regexp.MustCompile(`(?i)\bgo\s+install\b`)},
		{"cargo install", regexp.MustCompile(`(?i)\bcargo\s+install\b`)},
	}
	installHeadingCue = regexp.MustCompile(`(?im)^[ \t]{0,3}#{1,6}[ \t]+(prerequisites|installation|setup|requirements)\b`)
)

// AnalyzeHints inspects a skill folder's files. hasSpec says whether the skill
// already declares a runtime block.
func AnalyzeHints(files []HintFile, hasSpec bool) Hints {
	hints := Hints{Interpreters: []string{}, DependencyManifests: []string{}, MissingLockfiles: []string{}, AbsoluteInstallPaths: []string{}, InstallCues: []string{}}
	interpreters := map[string]bool{}
	cues := map[string]bool{}
	for _, file := range files {
		if file.Path == "skill.meta.yaml" {
			continue
		}
		if name := interpreterOf(file); name != "" {
			interpreters[name] = true
		}
		if IsDependencyManifest(file.Path) {
			hints.DependencyManifests = append(hints.DependencyManifests, file.Path)
		}
		if isText(file.Content) && mentionsSkillInstallPath(file.Content) {
			hints.AbsoluteInstallPaths = append(hints.AbsoluteInstallPaths, file.Path)
		}
		if isProseEntrypoint(file.Path) && isText(file.Content) {
			for _, cue := range installCues(file.Content) {
				cues[cue] = true
			}
		}
	}
	for name := range interpreters {
		hints.Interpreters = append(hints.Interpreters, name)
	}
	for cue := range cues {
		hints.InstallCues = append(hints.InstallCues, cue)
	}
	slices.Sort(hints.Interpreters)
	slices.Sort(hints.DependencyManifests)
	slices.Sort(hints.AbsoluteInstallPaths)
	slices.Sort(hints.InstallCues)
	hints.MissingLockfiles = missingLockfiles(files)
	hints.InstallProseDetected = len(hints.InstallCues) > 0
	hints.MissingRuntimeBlock = !hasSpec && (len(hints.Interpreters) > 0 || len(hints.DependencyManifests) > 0)
	return hints
}

// InterpreterOf names the interpreter a file runs under, from its extension or
// shebang line, or returns "" when it is not a script.
func InterpreterOf(relPath string, content []byte) string {
	return interpreterOf(HintFile{Path: relPath, Content: content})
}

func interpreterOf(file HintFile) string {
	if name, ok := extensionInterpreters[strings.ToLower(path.Ext(file.Path))]; ok {
		return name
	}
	if !bytes.HasPrefix(file.Content, []byte("#!")) {
		return ""
	}
	line, _, _ := bytes.Cut(file.Content[2:], []byte("\n"))
	fields := strings.Fields(string(line))
	if len(fields) == 0 {
		return ""
	}
	name := path.Base(fields[0])
	if name == "env" {
		name = ""
		for _, field := range fields[1:] {
			if !strings.HasPrefix(field, "-") && !strings.Contains(field, "=") {
				name = path.Base(field)
				break
			}
		}
	}
	if pythonVersionedName.MatchString(name) {
		name = "python3"
	}
	return shebangInterpreters[name]
}

func isText(content []byte) bool {
	return !bytes.ContainsRune(content, 0)
}

func mentionsSkillInstallPath(content []byte) bool {
	for _, marker := range skillInstallPathMarkers {
		if bytes.Contains(content, []byte(marker)) {
			return true
		}
	}
	return false
}

// isProseEntrypoint reports whether file is the skill's SKILL.md or a README at
// the skill root, the places install instructions are written.
func isProseEntrypoint(relPath string) bool {
	if strings.Contains(relPath, "/") {
		return false
	}
	lower := strings.ToLower(relPath)
	return lower == "skill.md" || lower == "readme" || strings.HasPrefix(lower, "readme.")
}

func installCues(content []byte) []string {
	var found []string
	for _, cue := range installCommandCues {
		if cue.pattern.Match(content) {
			found = append(found, cue.name)
		}
	}
	for _, match := range installHeadingCue.FindAllSubmatch(content, -1) {
		found = append(found, "heading: "+strings.ToLower(string(match[1])))
	}
	return found
}
