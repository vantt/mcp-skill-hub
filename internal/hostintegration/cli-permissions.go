package hostintegration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

const claudePermissionReceiptPath = ".claude/skillhub-permissions.json"

var claudeCLIRules = []struct {
	key    string
	values []string
}{
	{"allow", []string{"Bash(skillhub:*)"}},
	{"ask", []string{"Bash(skillhub * --yes*)", "Bash(skillhub * confirm *)"}},
	{"deny", []string{"Bash(skillhub * --approve-content*)"}},
}

type claudePermissionReceipt struct {
	Version int                 `json:"version"`
	Entries map[string][]string `json:"entries"`
}

func decodeClaudePermissionReceipt(raw []byte) (claudePermissionReceipt, error) {
	receipt := claudePermissionReceipt{Version: 1, Entries: map[string][]string{}}
	if len(bytes.TrimSpace(raw)) == 0 {
		return receipt, nil
	}
	if err := json.Unmarshal(raw, &receipt); err != nil {
		return receipt, fmt.Errorf("invalid permission ownership receipt: %w", err)
	}
	if receipt.Version != 1 || receipt.Entries == nil {
		return receipt, fmt.Errorf("unsupported permission ownership receipt")
	}
	for key := range receipt.Entries {
		if key != "additionalDirectories" && key != "allow" && key != "ask" && key != "deny" {
			return receipt, fmt.Errorf("unknown permission ownership key %q", key)
		}
	}
	return receipt, nil
}

func desiredClaudePermissionReceipt(raw, settings, desiredSettings []byte) ([]byte, error) {
	receipt, err := decodeClaudePermissionReceipt(raw)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"additionalDirectories", "allow", "ask", "deny"} {
		existing, err := jsonStringArrayAt(settings, []string{"permissions", key})
		if err != nil {
			return nil, err
		}
		wanted, err := jsonStringArrayAt(desiredSettings, []string{"permissions", key})
		if err != nil {
			return nil, err
		}
		for _, value := range wanted {
			if !slices.Contains(existing, value) && !slices.Contains(receipt.Entries[key], value) {
				receipt.Entries[key] = append(receipt.Entries[key], value)
			}
		}
	}
	encoded, err := json.MarshalIndent(receipt, "", "  ")
	return append(encoded, '\n'), err
}

func prepareClaudePermissionReceipt(adapter Adapter, scope Scope, root, workspace string) (preparedFile, error) {
	path := filepath.Join(root, filepath.FromSlash(claudePermissionReceiptPath))
	raw, mode, exists, err := readManagedFile(path, root)
	if err != nil {
		return preparedFile{}, err
	}
	settings, _, _, err := readManagedFile(filepath.Join(root, filepath.FromSlash(adapter.permissionsPath(scope))), root)
	if err != nil {
		return preparedFile{}, err
	}
	permissions, err := preparePermissions(adapter, scope, filepath.Join(root, filepath.FromSlash(adapter.permissionsPath(scope))), root, workspace)
	if err != nil {
		return preparedFile{}, err
	}
	if permissions.state.Conflict != "" {
		return preparedFile{}, fmt.Errorf("%s", permissions.state.Conflict)
	}
	desired, err := desiredClaudePermissionReceipt(raw, settings, permissions.desired)
	if err != nil {
		return preparedFile{}, err
	}
	file := newPreparedFile(HostClaude, ChangePermissionReceipt, path, raw, desired, mode, exists, "")
	file.mode = 0o600
	file.preview = "record only permission rules and runtime directories added by connect for safe removal"
	return file, nil
}

func claudePermissionsPreview(raw, desired []byte) string {
	var out strings.Builder
	out.WriteString("managed permissions:")
	for _, key := range []string{"additionalDirectories", "allow", "ask", "deny"} {
		before, _ := jsonStringArrayAt(raw, []string{"permissions", key})
		after, _ := jsonStringArrayAt(desired, []string{"permissions", key})
		for _, value := range after {
			if !slices.Contains(before, value) {
				fmt.Fprintf(&out, "\npermissions.%s: add %s", key, value)
			}
		}
		for _, value := range before {
			if !slices.Contains(after, value) {
				fmt.Fprintf(&out, "\npermissions.%s: remove %s", key, value)
			}
		}
	}
	return boundPreview(out.String())
}

func removeClaudePermissions(raw, receiptRaw []byte) ([]byte, error) {
	receipt, err := decodeClaudePermissionReceipt(receiptRaw)
	if err != nil {
		return nil, err
	}
	updated := raw
	for _, key := range []string{"additionalDirectories", "allow", "ask", "deny"} {
		existing, err := jsonStringArrayAt(updated, []string{"permissions", key})
		if err != nil {
			return nil, err
		}
		kept := make([]string, 0, len(existing))
		for _, value := range existing {
			if !slices.Contains(receipt.Entries[key], value) {
				kept = append(kept, value)
			}
		}
		if len(kept) == len(existing) {
			continue
		}
		encoded, _ := json.Marshal(kept)
		updated, err = upsertJSONPath(updated, []string{"permissions", key}, encoded)
		if err != nil {
			return nil, err
		}
	}
	return updated, nil
}
