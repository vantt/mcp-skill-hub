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
	"reflect"
	"slices"
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
	{Host: HostClaude, Level: LevelNativeSkillBestEffort, ConfigRelativePath: ".mcp.json", InstructionFileName: "CLAUDE.md", UserConfigRelativePath: ".claude.json", UserInstructionRelativePath: ".claude/CLAUDE.md", NativeSkill: true, SkillsDirRelativePath: ".claude/skills", Wrapper: true, PermissionsRelativePath: ".claude/settings.local.json", SharedPermissionsRelativePath: ".claude/settings.json", UserPermissionsRelativePath: ".claude/settings.json"},
	{Host: HostCodex, Level: LevelNativeSkillBestEffort, ConfigRelativePath: ".codex/config.toml", InstructionFileName: "AGENTS.md", UserConfigRelativePath: ".codex/config.toml", UserInstructionRelativePath: ".codex/AGENTS.md", NativeSkill: true, SkillsDirRelativePath: SharedSkillsDirRelativePath},
	{Host: HostGemini, Level: LevelNativeSkillBestEffort, ConfigRelativePath: ".gemini/settings.json", InstructionFileName: "GEMINI.md", UserConfigRelativePath: ".gemini/settings.json", UserInstructionRelativePath: ".gemini/GEMINI.md", NativeSkill: true, SkillsDirRelativePath: SharedSkillsDirRelativePath, LegacySkillsDirRelativePath: ".gemini/skills"},
}

// SupportedAdapters returns a defensive copy in deterministic order.
func SupportedAdapters() []Adapter { return append([]Adapter(nil), adapters...) }

type preparedFile struct {
	state   FileState
	desired []byte
	mode    os.FileMode
	preview string
	remove  bool
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
	hub       *RemoteHub
	remove    bool
	hosts     []preparedHost
	// shared holds the files of the shared Agent Skills folder.
	shared  []preparedFile
	notices []Notice
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
	for _, file := range prepared.shared {
		result.Shared = append(result.Shared, file.state)
	}
	return result, nil
}

func buildPlan(ctx context.Context, request Request) (PlanResult, error) {
	prepared, err := prepare(ctx, request)
	if err != nil {
		return PlanResult{}, err
	}
	plan := PlanResult{Workspace: prepared.workspace, Root: prepared.root, Scope: prepared.scope, Binary: prepared.binary, Hub: prepared.hub, Remove: prepared.remove, Notices: prepared.notices}
	type hostFile struct {
		host Host
		file preparedFile
	}
	var files []hostFile
	for _, file := range prepared.shared {
		files = append(files, hostFile{file: file})
	}
	for _, host := range prepared.hosts {
		for _, file := range host.files {
			files = append(files, hostFile{host: host.adapter.Host, file: file})
		}
	}
	for _, entry := range files {
		if entry.file.state.Conflict != "" {
			return PlanResult{}, &ConflictError{Path: entry.file.state.Path, Reason: entry.file.state.Conflict}
		}
	}
	for _, entry := range files {
		file := entry.file
		if file.state.Blocked != "" {
			state := file.state
			state.Host = entry.host
			plan.Blocked = append(plan.Blocked, state)
			continue
		}
		if file.state.Current {
			continue
		}
		plan.Changes = append(plan.Changes, Change{
			Host: entry.host, Kind: file.state.Kind, Path: file.state.Path,
			PreimageDigest: file.state.PreimageDigest,
			Desired:        append([]byte(nil), file.desired...), Mode: uint32(file.mode.Perm()),
			Preview: file.preview, Remove: file.remove,
		})
	}
	// A stable sort keeps the host order inside each rank.
	slices.SortStableFunc(plan.Changes, func(a, b Change) int { return changeRank(a) - changeRank(b) })
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
	prepared := preparedInspection{workspace: workspace, root: root, scope: scope, binary: binary, hub: request.Hub, remove: request.Remove}
	mode := planMode{workspace: workspace, binary: binary, hub: request.Hub, remove: request.Remove}
	skills, err := prepareSystemSkills(selected, scope, root, workspace, mode)
	if err != nil {
		return preparedInspection{}, err
	}
	prepared.shared, prepared.notices = skills.shared, skills.notices
	for _, adapter := range selected {
		if err := ctx.Err(); err != nil {
			return preparedInspection{}, err
		}
		host := preparedHost{adapter: adapter}
		configRel, instructionRel := adapter.relativePaths(scope)
		configPath := filepath.Join(root, filepath.FromSlash(configRel))
		previous, err := scopeRegistration(adapter, scope, root)
		if err != nil {
			return preparedInspection{}, err
		}
		config, err := prepareConfigFor(adapter.Host, configPath, root, mode)
		if err != nil {
			return preparedInspection{}, err
		}
		host.files = append(host.files, config)
		if permissionsRel := adapter.permissionsPath(scope); permissionsRel != "" {
			permissions, err := preparePermissions(adapter, scope, filepath.Join(root, filepath.FromSlash(permissionsRel)), root, mode, previous)
			if err != nil {
				return preparedInspection{}, err
			}
			host.files = append(host.files, permissions)
		}
		host.files = append(host.files, skills.hosts[adapter.Host]...)
		instructionPath := filepath.Join(root, filepath.FromSlash(instructionRel))
		if mode.remove {
			instructions, err := prepareInstructionsRemoval(instructionPath, root)
			if err != nil {
				return preparedInspection{}, err
			}
			host.files = append(host.files, instructions)
			prepared.hosts = append(prepared.hosts, host)
			continue
		}
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
	local := request.Hub == nil && !request.Remove
	if request.Hub != nil && request.Remove {
		return "", "", "", "", nil, fmt.Errorf("a plan either connects a remote hub or removes the connection")
	}
	if request.Hub != nil {
		if err := validateRemoteHub(*request.Hub); err != nil {
			return "", "", "", "", nil, err
		}
	}
	if local && !filepath.IsAbs(request.Workspace) {
		return "", "", "", "", nil, fmt.Errorf("workspace path must be absolute")
	}
	if !local && request.Workspace != "" {
		return "", "", "", "", nil, fmt.Errorf("a remote or removal plan serves no workspace")
	}
	if !filepath.IsAbs(request.Binary) {
		return "", "", "", "", nil, fmt.Errorf("binary path must be absolute")
	}
	if local {
		workspace = filepath.Clean(request.Workspace)
	}
	binary = filepath.Clean(request.Binary)
	root = workspace
	if request.Root != "" {
		if !filepath.IsAbs(request.Root) {
			return "", "", "", "", nil, fmt.Errorf("root path must be absolute")
		}
		root = filepath.Clean(request.Root)
	}
	if root == "" {
		return "", "", "", "", nil, fmt.Errorf("root path must be set for a remote or removal plan")
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
		if target.path == "" {
			continue
		}
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

func prepareConfig(host Host, path, root string, plan planMode) (preparedFile, error) {
	workspace := plan.workspace
	raw, mode, exists, err := readManagedFile(path, root)
	if err != nil {
		return preparedFile{}, err
	}
	desired, _, err := desiredConfig(host, raw, plan)
	if err != nil {
		return newPreparedFile(host, ChangeMCP, path, raw, raw, mode, exists, err.Error()), nil
	}
	file := newPreparedFile(host, ChangeMCP, path, raw, desired, mode, exists, "")
	if note := allowedDirsPreview(host, raw, workspace); note != "" {
		file.preview = boundPreview(file.preview + "\n" + note)
	}
	return file, nil
}

func preparePermissions(adapter Adapter, scope Scope, path, root string, plan planMode, previous Registration) (preparedFile, error) {
	if !plan.local() {
		return prepareNonLocalPermissions(adapter, scope, path, root, plan, previous)
	}
	workspace := plan.workspace
	host := adapter.Host
	raw, mode, exists, err := readManagedFile(path, root)
	if err != nil {
		return preparedFile{}, err
	}
	// A local hub replacing a remote one drops the remote cache directory;
	// over a local entry nothing is dropped.
	base, err := stripJSONStringArray(raw, claudeAllowedDirsPath, staleAccessDirs(previous, plan), true)
	if err != nil {
		return newPreparedFile(host, ChangeHostPermissions, path, raw, raw, mode, exists, err.Error()), nil
	}
	desired, err := desiredClaudePermissions(base, workspace)
	if err != nil {
		return newPreparedFile(host, ChangeHostPermissions, path, raw, raw, mode, exists, err.Error()), nil
	}
	if scope == ScopeProject && adapter.SharedPermissionsRelativePath != "" {
		sharedPath := filepath.Join(root, filepath.FromSlash(adapter.SharedPermissionsRelativePath))
		shared, _, sharedExists, readErr := readManagedFile(sharedPath, root)
		if readErr != nil {
			return preparedFile{}, readErr
		}
		if sharedExists {
			withDirs, dirErr := ensureJSONStringArray(shared, claudeAllowedDirsPath, RuntimeAccessDirs(workspace))
			if dirErr == nil && bytes.Equal(withDirs, shared) {
				desired, err = desiredClaudeCLIRules(base)
				if err != nil {
					return preparedFile{}, err
				}
			}
		}
	}
	file := newPreparedFile(host, ChangeHostPermissions, path, raw, desired, mode, exists, "")
	file.preview = claudePermissionsPreview(raw, desired)
	return file, nil
}

// Claude Code curates through the CLI, independently of its verified MCP toggle.
// Keep only the runtime server; remove the wave-4 curation entry only if its
// complete contents still match the registration connect wrote.
func desiredClaudeCLICurationConfig(raw []byte, binary, workspace string) ([]byte, error) {
	encoded, err := json.Marshal(claudeMCPServer{Type: "stdio", Command: binary, Args: []string{"mcp", "serve", "--profile", "runtime", "--workspace", workspace}})
	if err != nil {
		return nil, fmt.Errorf("encode Claude runtime MCP registration: %w", err)
	}
	updated, err := upsertJSONPath(raw, []string{"mcpServers", "skillhub"}, encoded)
	if err != nil {
		return nil, err
	}
	var config struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(updated, &config); err != nil {
		return nil, err
	}
	curation, ok := config.MCPServers["skillhub-curation"]
	if !ok {
		return updated, nil
	}
	expected, _ := json.Marshal(claudeMCPServer{Type: "stdio", Command: binary, Args: []string{"mcp", "serve", "--profile", "curation", "--workspace", workspace}})
	var have, want any
	if json.Unmarshal(curation, &have) != nil || json.Unmarshal(expected, &want) != nil || !reflect.DeepEqual(have, want) {
		return updated, nil
	}
	return removeJSONPath(updated, []string{"mcpServers", "skillhub-curation"})
}

func desiredGeminiConfig(raw []byte, binary, workspace string, supportsToggle bool) ([]byte, error) {
	var registered []byte
	if supportsToggle {
		runtimeServer, err := json.Marshal(geminiMCPServer{Command: binary, Args: []string{"mcp", "serve", "--profile", "runtime", "--workspace", workspace}, CWD: workspace, Trust: false})
		if err != nil {
			return nil, fmt.Errorf("encode Gemini runtime MCP registration: %w", err)
		}
		curationServer, err := json.Marshal(geminiMCPServer{Command: binary, Args: []string{"mcp", "serve", "--profile", "curation", "--workspace", workspace}, CWD: workspace, Trust: false})
		if err != nil {
			return nil, fmt.Errorf("encode Gemini curation MCP registration: %w", err)
		}
		updated, err := upsertJSONPath(raw, []string{"mcpServers", "skillhub"}, runtimeServer)
		if err != nil {
			return nil, err
		}
		registered, err = upsertJSONPath(updated, []string{"mcpServers", "skillhub-curation"}, curationServer)
		if err != nil {
			return nil, err
		}
	} else {
		encoded, err := json.Marshal(geminiMCPServer{Command: binary, Args: []string{"mcp", "serve", "--workspace", workspace}, CWD: workspace, Trust: false})
		if err != nil {
			return nil, fmt.Errorf("encode Gemini MCP registration: %w", err)
		}
		updated, err := upsertJSONPath(raw, []string{"mcpServers", "skillhub"}, encoded)
		if err != nil {
			return nil, err
		}
		registered, err = removeJSONPath(updated, []string{"mcpServers", "skillhub-curation"})
		if err != nil {
			return nil, err
		}
	}
	return desiredGeminiAllowedDirs(registered, workspace)
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
