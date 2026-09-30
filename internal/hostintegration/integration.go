package hostintegration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/vantt/mcp-skill-hub/internal/systemskills"
)

const missingDigest = "missing"

type claudeMCPServer struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type geminiMCPServer struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	CWD     string   `json:"cwd"`
	Trust   bool     `json:"trust"`
}

var adapters = []Adapter{
	{Host: HostClaude, Level: LevelNativeSkillBestEffort, ConfigRelativePath: ".mcp.json", SkillRelativePath: ".claude/skills/system-curator/SKILL.md", InstructionFileName: "CLAUDE.md", UserConfigRelativePath: ".claude.json", UserSkillRelativePath: ".claude/skills/system-curator/SKILL.md", UserInstructionRelativePath: ".claude/CLAUDE.md", NativeSkill: true},
	{Host: HostCodex, Level: LevelNativeSkillBestEffort, ConfigRelativePath: ".codex/config.toml", SkillRelativePath: ".agents/skills/system-curator/SKILL.md", InstructionFileName: "AGENTS.md", UserConfigRelativePath: ".codex/config.toml", UserSkillRelativePath: ".agents/skills/system-curator/SKILL.md", UserInstructionRelativePath: ".codex/AGENTS.md", NativeSkill: true},
	{Host: HostGemini, Level: LevelNativeSkillBestEffort, ConfigRelativePath: ".gemini/settings.json", SkillRelativePath: ".gemini/skills/system-curator/SKILL.md", InstructionFileName: "GEMINI.md", UserConfigRelativePath: ".gemini/settings.json", UserSkillRelativePath: ".gemini/skills/system-curator/SKILL.md", UserInstructionRelativePath: ".gemini/GEMINI.md", NativeSkill: true},
}

// SupportedAdapters returns a defensive copy in deterministic order.
func SupportedAdapters() []Adapter { return append([]Adapter(nil), adapters...) }

type preparedFile struct {
	state   FileState
	desired []byte
	mode    os.FileMode
	preview string
}

type preparedHost struct {
	adapter Adapter
	files   []preparedFile
}

type preparedInspection struct {
	workspace string
	root      string
	scope     Scope
	binary    string
	hosts     []preparedHost
}

func inspect(ctx context.Context, request Request) (Inspection, error) {
	prepared, err := prepare(ctx, request)
	if err != nil {
		return Inspection{}, err
	}
	result := Inspection{Workspace: prepared.workspace, Root: prepared.root, Binary: prepared.binary}
	for _, host := range prepared.hosts {
		report := HostInspection{Host: host.adapter.Host, Level: host.adapter.Level}
		for _, file := range host.files {
			report.Files = append(report.Files, file.state)
		}
		result.Hosts = append(result.Hosts, report)
	}
	return result, nil
}

func buildPlan(ctx context.Context, request Request) (PlanResult, error) {
	prepared, err := prepare(ctx, request)
	if err != nil {
		return PlanResult{}, err
	}
	plan := PlanResult{Workspace: prepared.workspace, Root: prepared.root, Scope: prepared.scope, Binary: prepared.binary}
	for _, kind := range []ChangeKind{ChangeMCP, ChangeNativeSkill, ChangeBootstrap} {
		for _, host := range prepared.hosts {
			for _, file := range host.files {
				if file.state.Conflict != "" {
					return PlanResult{}, &ConflictError{Path: file.state.Path, Reason: file.state.Conflict}
				}
				if file.state.Kind != kind || file.state.Current {
					continue
				}
				plan.Changes = append(plan.Changes, Change{
					Host: host.adapter.Host, Kind: kind, Path: file.state.Path,
					PreimageDigest: file.state.PreimageDigest,
					Desired:        append([]byte(nil), file.desired...), Mode: uint32(file.mode.Perm()),
					Preview: file.preview,
				})
			}
		}
	}
	return plan, nil
}

func prepare(ctx context.Context, request Request) (preparedInspection, error) {
	if err := ctx.Err(); err != nil {
		return preparedInspection{}, err
	}
	workspace, root, scope, binary, selected, err := validateRequest(request)
	if err != nil {
		return preparedInspection{}, err
	}
	prepared := preparedInspection{workspace: workspace, root: root, scope: scope, binary: binary}
	bundle := systemskills.CuratorBundle()
	for _, adapter := range selected {
		if err := ctx.Err(); err != nil {
			return preparedInspection{}, err
		}
		host := preparedHost{adapter: adapter}
		configRel, skillRel, instructionRel := adapter.relativePaths(scope)
		configPath := filepath.Join(root, filepath.FromSlash(configRel))
		config, err := prepareConfig(adapter.Host, configPath, root, workspace, binary)
		if err != nil {
			return preparedInspection{}, err
		}
		host.files = append(host.files, config)
		if adapter.NativeSkill {
			skillPath := filepath.Join(root, filepath.FromSlash(skillRel))
			skill, err := prepareExactFile(ChangeNativeSkill, skillPath, root, []byte(bundle.Instructions))
			if err != nil {
				return preparedInspection{}, err
			}
			host.files = append(host.files, skill)
		}
		instructionPath := filepath.Join(root, filepath.FromSlash(instructionRel))
		globalHasBootstrap := false
		if scope == ScopeProject {
			if home, homeErr := os.UserHomeDir(); homeErr == nil {
				userInstructionPath := filepath.Join(home, filepath.FromSlash(adapter.UserInstructionRelativePath))
				if userContent, err := os.ReadFile(userInstructionPath); err == nil {
					if bytes.Contains(userContent, []byte(bootstrapStart)) {
						globalHasBootstrap = true
					}
				}
			}
		}
		instructions, err := prepareInstructions(instructionPath, root, globalHasBootstrap)
		if err != nil {
			return preparedInspection{}, err
		}
		host.files = append(host.files, instructions)
		prepared.hosts = append(prepared.hosts, host)
	}
	return prepared, nil
}

func validateRequest(request Request) (workspace, root string, scope Scope, binary string, selected []Adapter, err error) {
	if !filepath.IsAbs(request.Workspace) {
		return "", "", "", "", nil, fmt.Errorf("workspace path must be absolute")
	}
	if !filepath.IsAbs(request.Binary) {
		return "", "", "", "", nil, fmt.Errorf("binary path must be absolute")
	}
	workspace = filepath.Clean(request.Workspace)
	binary = filepath.Clean(request.Binary)
	root = workspace
	if request.Root != "" {
		if !filepath.IsAbs(request.Root) {
			return "", "", "", "", nil, fmt.Errorf("root path must be absolute")
		}
		root = filepath.Clean(request.Root)
	}
	scope = request.Scope
	switch scope {
	case "":
		scope = ScopeProject
	case ScopeProject, ScopeUser:
	default:
		return "", "", "", "", nil, fmt.Errorf("unsupported scope %q", scope)
	}
	for _, target := range []struct{ label, path string }{{"workspace", workspace}, {"root", root}} {
		info, statErr := os.Stat(target.path)
		if statErr != nil {
			return "", "", "", "", nil, fmt.Errorf("inspect %s: %w", target.label, statErr)
		}
		if !info.IsDir() {
			return "", "", "", "", nil, fmt.Errorf("%s is not a directory", target.label)
		}
	}
	// Only the write root is symlink-checked: the workspace is referenced by
	// path inside registrations and never written by host integration.
	if err := rejectSymlinkComponents(root, root); err != nil {
		return "", "", "", "", nil, err
	}

	requested := request.Hosts
	if len(requested) == 0 {
		requested = []Host{HostClaude, HostCodex, HostGemini}
	}
	seen := make(map[Host]bool, len(requested))
	selected = make([]Adapter, 0, len(requested))
	for _, host := range requested {
		if seen[host] {
			continue
		}
		seen[host] = true
		index := slices.IndexFunc(adapters, func(adapter Adapter) bool { return adapter.Host == host })
		if index < 0 {
			return "", "", "", "", nil, fmt.Errorf("unsupported host %q", host)
		}
		selected = append(selected, adapters[index])
	}
	return workspace, root, scope, binary, selected, nil
}

func prepareConfig(host Host, path, root, workspace, binary string) (preparedFile, error) {
	raw, mode, exists, err := readManagedFile(path, root)
	if err != nil {
		return preparedFile{}, err
	}
	var desired []byte
	switch host {
	case HostClaude:
		desired, err = desiredClaudeConfig(raw, binary, workspace)
	case HostGemini:
		desired, err = desiredGeminiConfig(raw, binary, workspace)
	case HostCodex:
		desired, err = upsertCodexTOML(raw, binary, workspace)
	default:
		err = fmt.Errorf("unsupported host %q", host)
	}
	if err != nil {
		return newPreparedFile(host, ChangeMCP, path, raw, raw, mode, exists, err.Error()), nil
	}
	return newPreparedFile(host, ChangeMCP, path, raw, desired, mode, exists, ""), nil
}

func desiredClaudeConfig(raw []byte, binary, workspace string) ([]byte, error) {
	encoded, err := json.Marshal(claudeMCPServer{Type: "stdio", Command: binary, Args: []string{"mcp", "serve", "--workspace", workspace}})
	if err != nil {
		return nil, fmt.Errorf("encode Claude MCP registration: %w", err)
	}
	return upsertJSONPath(raw, []string{"mcpServers", "skillhub"}, encoded)
}

func desiredGeminiConfig(raw []byte, binary, workspace string) ([]byte, error) {
	encoded, err := json.Marshal(geminiMCPServer{Command: binary, Args: []string{"mcp", "serve", "--workspace", workspace}, CWD: workspace, Trust: false})
	if err != nil {
		return nil, fmt.Errorf("encode Gemini MCP registration: %w", err)
	}
	return upsertJSONPath(raw, []string{"mcpServers", "skillhub"}, encoded)
}

func prepareExactFile(kind ChangeKind, path, root string, desired []byte) (preparedFile, error) {
	raw, mode, exists, err := readManagedFile(path, root)
	if err != nil {
		return preparedFile{}, err
	}
	return newPreparedFile("", kind, path, raw, desired, mode, exists, ""), nil
}

func prepareInstructions(path, root string, globalHasBootstrap bool) (preparedFile, error) {
	raw, mode, exists, err := readManagedFile(path, root)
	if err != nil {
		return preparedFile{}, err
	}
	if globalHasBootstrap && !bytes.Contains(raw, []byte(bootstrapStart)) {
		return newPreparedFile("", ChangeBootstrap, path, raw, raw, mode, exists, ""), nil
	}
	desired, conflict := updateBootstrap(raw)
	return newPreparedFile("", ChangeBootstrap, path, raw, desired, mode, exists, conflict), nil
}

func newPreparedFile(host Host, kind ChangeKind, path string, raw, desired []byte, mode os.FileMode, exists bool, conflict string) preparedFile {
	if !exists {
		mode = 0o644
	}
	return preparedFile{
		state:   FileState{Kind: kind, Path: path, Exists: exists, Current: conflict == "" && bytes.Equal(raw, desired), PreimageDigest: digest(raw, exists), Conflict: conflict},
		desired: append([]byte(nil), desired...), mode: mode,
		preview: changePreview(host, kind, raw, desired, exists),
	}
}

func digest(content []byte, exists bool) string {
	if !exists {
		return missingDigest
	}
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}
