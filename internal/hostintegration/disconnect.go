package hostintegration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
)

func prepareDisconnect(request Request) (preparedInspection, error) {
	workspace, root, scope, binary, selected, err := validateRequest(request)
	if err != nil {
		return preparedInspection{}, err
	}
	request.Workspace, request.Root, request.Scope, request.Binary = workspace, root, scope, binary
	prepared := preparedInspection{workspace: workspace, root: root, scope: scope, binary: binary}
	for _, adapter := range selected {
		if adapter.Host != HostClaude {
			return preparedInspection{}, fmt.Errorf("disconnect currently supports only Claude Code; use --host claude")
		}
		host := preparedHost{adapter: adapter}
		config, skill, instructions := adapter.relativePaths(scope)
		for _, target := range []struct {
			kind     ChangeKind
			relative string
		}{
			{ChangeMCP, config}, {ChangeHostPermissions, adapter.permissionsPath(scope)},
			{ChangeNativeSkill, skill}, {ChangeBootstrap, instructions}, {ChangePermissionReceipt, claudePermissionReceiptPath(scope)},
		} {
			path := filepath.Join(root, filepath.FromSlash(target.relative))
			raw, mode, exists, err := readManagedFile(path, root)
			if err != nil {
				return preparedInspection{}, err
			}
			desired, err := desiredDisconnectFile(target.kind, raw, request)
			if err != nil {
				return preparedInspection{}, err
			}
			file := newPreparedFile(HostClaude, target.kind, path, raw, desired, mode, exists, "")
			if !exists {
				file.state.Current = true
			}
			switch target.kind {
			case ChangeMCP:
				file.preview = mcpChangePreview(HostClaude, raw, desired)
			case ChangeHostPermissions:
				file.preview = claudePermissionsPreview(raw, desired)
			default:
				file.preview = "remove unedited connect-managed content; preserve user content"
			}
			host.files = append(host.files, file)
		}
		prepared.hosts = append(prepared.hosts, host)
	}
	return prepared, nil
}

func desiredDisconnectFile(kind ChangeKind, raw []byte, request Request) ([]byte, error) {
	switch kind {
	case ChangeMCP:
		return removeClaudeRegistration(raw, request.Binary, request.Workspace)
	case ChangeHostPermissions:
		receipt, _, _, err := readManagedFile(filepath.Join(request.Root, filepath.FromSlash(claudePermissionReceiptPath(request.Scope))), request.Root)
		if err != nil {
			return nil, err
		}
		return removeClaudePermissions(raw, receipt)
	case ChangePermissionReceipt:
		if _, err := decodeClaudePermissionReceipt(raw); err != nil {
			return nil, err
		}
		return nil, nil
	case ChangeNativeSkill:
		// Project and user connections rooted at HOME share the native skill.
		otherConfig := adapters[0].UserConfigRelativePath
		if request.Scope == ScopeUser {
			otherConfig = adapters[0].ConfigRelativePath
		}
		other, _, _, err := readManagedFile(filepath.Join(request.Root, filepath.FromSlash(otherConfig)), request.Root)
		if err != nil {
			return nil, err
		}
		if _, registered := RegisteredWorkspace(HostClaude, other); registered {
			return raw, nil
		}
		if nativeCuratorOwned(raw) {
			return nil, nil
		}
		return raw, nil
	case ChangeBootstrap:
		block := bootstrapBlock(detectNewline(raw))
		if index := bytes.Index(raw, block); index >= 0 {
			return splice(raw, index, index+len(block), nil), nil
		}
		return raw, nil
	}
	return nil, fmt.Errorf("unsupported disconnect surface %q", kind)
}

func removeClaudeRegistration(raw []byte, binary, workspace string) ([]byte, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return raw, nil
	}
	var config struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, err
	}
	updated := raw
	for _, name := range []string{"skillhub", "skillhub-curation"} {
		existing, ok := config.MCPServers[name]
		if !ok {
			continue
		}
		args := []string{"mcp", "serve", "--profile", "runtime", "--workspace", workspace}
		if name == "skillhub-curation" {
			args[3] = "curation"
		}
		expected, _ := json.Marshal(claudeMCPServer{Type: "stdio", Command: binary, Args: args})
		var have, want any
		_ = json.Unmarshal(existing, &have)
		_ = json.Unmarshal(expected, &want)
		if !reflect.DeepEqual(have, want) {
			continue
		}
		var err error
		updated, err = removeJSONPath(updated, []string{"mcpServers", name})
		if err != nil {
			return nil, err
		}
	}
	return updated, nil
}

func removalChange(change Change, plan PlanResult) bool {
	return nativeRemoval(change) || (plan.Remove && change.Desired == nil && (change.Kind == ChangeNativeSkill || change.Kind == ChangePermissionReceipt))
}
