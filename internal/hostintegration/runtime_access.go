package hostintegration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// RuntimeAccessDirs returns the workspace directories an agent host must be
// allowed to touch for materialized skills: the immutable snapshots, the
// per-skill writable state, and the per-skill config (secret env files). They
// live outside the project, so hosts sandbox or prompt for them by default.
// The relative paths mirror the snapshot, state, and config roots under the
// workspace runtime directory.
func RuntimeAccessDirs(workspace string) []string {
	return []string{
		filepath.Join(workspace, "runtime", "cache", "skills"),
		filepath.Join(workspace, "runtime", "envs"),
		filepath.Join(workspace, "runtime", "config"),
	}
}

// Host settings keys, checked against the vendors' current documentation:
//   - Claude Code: permissions.additionalDirectories in settings.json
//     (code.claude.com/docs/en/permissions, code.claude.com/docs/en/settings).
//   - Codex: sandbox_workspace_write.writable_roots in config.toml
//     (learn.chatgpt.com/docs/config-file/config-reference).
//   - Gemini CLI: context.includeDirectories in settings.json
//     (geminicli.com/docs/reference/configuration).
var (
	claudeAllowedDirsPath = []string{"permissions", "additionalDirectories"}
	geminiAllowedDirsPath = []string{"context", "includeDirectories"}
)

const codexWritableRootsTable = "sandbox_workspace_write"

func desiredClaudePermissions(raw []byte, workspace string) ([]byte, error) {
	updated, err := ensureJSONStringArray(raw, claudeAllowedDirsPath, RuntimeAccessDirs(workspace))
	if err != nil {
		return nil, err
	}
	return desiredClaudeCLIRules(updated)
}

func desiredClaudeCLIRules(raw []byte) ([]byte, error) {
	updated := raw
	for _, rule := range claudeCLIRules {
		existing, err := jsonStringArrayAt(updated, []string{"permissions", rule.key})
		if err != nil {
			return nil, err
		}
		values := append([]string(nil), existing...)
		for _, value := range rule.values {
			if !slices.Contains(values, value) {
				values = append(values, value)
			}
		}
		if slices.Equal(existing, values) {
			continue
		}
		encoded, _ := json.Marshal(values)
		updated, err = upsertJSONPath(updated, []string{"permissions", rule.key}, encoded)
		if err != nil {
			return nil, err
		}
	}
	return updated, nil
}

// ensureJSONStringArray makes the array of strings at path contain every value,
// keeping existing entries and their order and appending only the missing ones.
// The file is returned byte-identical when nothing is missing.
func ensureJSONStringArray(raw []byte, path []string, values []string) ([]byte, error) {
	existing, err := jsonStringArrayAt(raw, path)
	if err != nil {
		return nil, err
	}
	missing := missingEntries(existing, values)
	if len(missing) == 0 {
		return raw, nil
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(append(existing, missing...)); err != nil {
		return nil, fmt.Errorf("encode %s: %w", strings.Join(path, "."), err)
	}
	return upsertJSONPath(raw, path, bytes.TrimSpace(encoded.Bytes()))
}

func jsonStringArrayAt(raw []byte, path []string) ([]string, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, nil
	}
	var node map[string]json.RawMessage
	if err := json.Unmarshal(raw, &node); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	for index, key := range path {
		value, ok := node[key]
		if !ok {
			return nil, nil
		}
		if index == len(path)-1 {
			var list []string
			if err := json.Unmarshal(value, &list); err != nil {
				return nil, fmt.Errorf("JSON path %q is not an array of strings", strings.Join(path, "."))
			}
			return list, nil
		}
		node = nil
		if err := json.Unmarshal(value, &node); err != nil || node == nil {
			return nil, fmt.Errorf("JSON path %q is not an object", strings.Join(path[:index+1], "."))
		}
	}
	return nil, nil
}

// missingEntries returns the wanted directories absent from existing, comparing
// cleaned paths so a trailing slash does not cause a duplicate.
func missingEntries(existing, wanted []string) []string {
	have := make(map[string]struct{}, len(existing))
	for _, entry := range existing {
		have[filepath.Clean(entry)] = struct{}{}
	}
	var missing []string
	for _, entry := range wanted {
		if _, ok := have[filepath.Clean(entry)]; !ok {
			missing = append(missing, entry)
		}
	}
	return missing
}

func desiredGeminiAllowedDirs(raw []byte, workspace string) ([]byte, error) {
	return ensureJSONStringArray(raw, geminiAllowedDirsPath, RuntimeAccessDirs(workspace))
}

// allowedDirsPreview describes the directory entries a plan will add, so the
// preview of a config file shows more than the MCP registration fields.
func allowedDirsPreview(host Host, raw []byte, workspace string) string {
	var existing []string
	switch host {
	case HostGemini:
		existing, _ = jsonStringArrayAt(raw, geminiAllowedDirsPath)
	case HostClaude:
		existing, _ = jsonStringArrayAt(raw, claudeAllowedDirsPath)
	case HostCodex:
		existing, _ = codexWritableRoots(raw)
	}
	missing := missingEntries(existing, RuntimeAccessDirs(workspace))
	if len(missing) == 0 {
		return ""
	}
	return "allowed directories to add: " + strings.Join(missing, ", ")
}

func codexWritableRoots(raw []byte) ([]string, error) {
	statements, err := scanTOMLStatements(raw)
	if err != nil {
		return nil, err
	}
	currentTable := ""
	for _, statement := range statements {
		if statement.kind == 'h' {
			currentTable = normalizeTOMLTable(statement.name)
			continue
		}
		if currentTable == codexWritableRootsTable && normalizeTOMLTable(statement.name) == "writable_roots" {
			return parseTOMLStringArray(raw, statement)
		}
	}
	return nil, nil
}

// desiredCodexConfig applies the skillhub MCP registration and the runtime
// directory allowance to config.toml.
func desiredCodexConfig(raw []byte, binary, workspace string, supportsToggle bool) ([]byte, error) {
	var registered []byte
	var err error
	if supportsToggle {
		registered, err = upsertCodexTOMLTable(raw, "mcp_servers.skillhub", binary, []string{"mcp", "serve", "--profile", "runtime", "--workspace", workspace}, workspace)
		if err != nil {
			return nil, err
		}
		registered, err = upsertCodexTOMLTable(registered, "mcp_servers.skillhub-curation", binary, []string{"mcp", "serve", "--profile", "curation", "--workspace", workspace}, workspace)
		if err != nil {
			return nil, err
		}
	} else {
		registered, err = upsertCodexTOMLTable(raw, "mcp_servers.skillhub", binary, []string{"mcp", "serve", "--workspace", workspace}, workspace)
		if err != nil {
			return nil, err
		}
		registered, err = removeCodexTOMLTable(registered, "mcp_servers.skillhub-curation")
		if err != nil {
			return nil, err
		}
	}
	return ensureCodexWritableRoots(registered, RuntimeAccessDirs(workspace))
}

// ensureCodexWritableRoots adds the directories to
// [sandbox_workspace_write].writable_roots, keeping existing entries.
func ensureCodexWritableRoots(raw []byte, dirs []string) ([]byte, error) {
	statements, err := scanTOMLStatements(raw)
	if err != nil {
		return nil, fmt.Errorf("cannot safely update Codex TOML: %w", err)
	}
	newline := detectNewline(raw)

	sectionStart, sectionEnd := -1, len(raw)
	for index, statement := range statements {
		if statement.kind != 'h' || normalizeTOMLTable(statement.name) != codexWritableRootsTable {
			continue
		}
		if sectionStart >= 0 {
			return nil, fmt.Errorf("duplicate TOML table %s", codexWritableRootsTable)
		}
		sectionStart = statement.start
		for _, later := range statements[index+1:] {
			if later.kind == 'h' {
				sectionEnd = later.start
				break
			}
		}
	}
	currentTable := ""
	for _, statement := range statements {
		if statement.kind == 'h' {
			currentTable = normalizeTOMLTable(statement.name)
			continue
		}
		full := normalizeTOMLTable(statement.name)
		if currentTable != "" {
			full = currentTable + "." + full
		}
		inTable := currentTable == codexWritableRootsTable || strings.HasPrefix(currentTable, codexWritableRootsTable+".")
		if full == codexWritableRootsTable || (!inTable && strings.HasPrefix(full, codexWritableRootsTable+".")) {
			return nil, fmt.Errorf("ambiguous TOML definition of %s outside a [%s] table; edit it manually", codexWritableRootsTable, codexWritableRootsTable)
		}
	}

	if sectionStart < 0 {
		line := "writable_roots = " + tomlStringArray(dirs)
		return appendTOMLBlock(raw, newline, "["+codexWritableRootsTable+"]"+newline+line), nil
	}

	section := append([]byte(nil), raw[sectionStart:sectionEnd]...)
	sectionStatements, err := scanTOMLStatements(section)
	if err != nil {
		return nil, fmt.Errorf("cannot safely update Codex TOML: %w", err)
	}
	var matches []tomlStatement
	for _, statement := range sectionStatements {
		if statement.kind == 'a' && normalizeTOMLTable(statement.name) == "writable_roots" {
			matches = append(matches, statement)
		}
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("duplicate TOML key writable_roots in %s", codexWritableRootsTable)
	}
	if len(matches) == 0 {
		line := "writable_roots = " + tomlStringArray(dirs)
		return splice(raw, sectionStart, sectionEnd, insertTOMLLine(section, newline, line)), nil
	}
	existing, err := parseTOMLStringArray(section, matches[0])
	if err != nil {
		return nil, fmt.Errorf("cannot safely update writable_roots: %w", err)
	}
	missing := missingEntries(existing, dirs)
	if len(missing) == 0 {
		return raw, nil
	}
	line := "writable_roots = " + tomlStringArray(append(existing, missing...))
	updated, _, err := replaceTOMLKey(section, "writable_roots", line)
	if err != nil {
		return nil, fmt.Errorf("cannot safely update writable_roots: %w", err)
	}
	return splice(raw, sectionStart, sectionEnd, updated), nil
}

func tomlStringArray(values []string) string {
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = tomlString(value)
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

// parseTOMLStringArray reads the array value of an assignment statement. Only
// single-line-style string elements (basic or literal) are understood; anything
// else is reported so the file is never rewritten from a partial reading.
func parseTOMLStringArray(raw []byte, statement tomlStatement) ([]string, error) {
	offset := skipTOMLHorizontal(raw, statement.start)
	equals, err := findTOMLEquals(raw, offset)
	if err != nil || equals < 0 {
		return nil, fmt.Errorf("cannot locate the value of %s", strings.TrimSpace(statement.name))
	}
	index := skipTOMLHorizontal(raw, equals+1)
	if index >= statement.end || raw[index] != '[' {
		return nil, fmt.Errorf("%s is not an array", strings.TrimSpace(statement.name))
	}
	index++
	var values []string
	for index < statement.end {
		switch raw[index] {
		case ' ', '\t', '\r', '\n', ',':
			index++
		case '#':
			index = tomlNextLine(raw, index)
		case ']':
			return values, nil
		case '"', '\'':
			end, err := scanTOMLString(raw, index)
			if err != nil {
				return nil, err
			}
			token := raw[index:end]
			if bytes.HasPrefix(token, []byte(`"""`)) || bytes.HasPrefix(token, []byte(`'''`)) {
				return nil, fmt.Errorf("multi-line strings are not supported in %s", strings.TrimSpace(statement.name))
			}
			if token[0] == '\'' {
				values = append(values, string(token[1:len(token)-1]))
			} else {
				var decoded string
				if err := json.Unmarshal(token, &decoded); err != nil {
					return nil, fmt.Errorf("unsupported string escape in %s", strings.TrimSpace(statement.name))
				}
				values = append(values, decoded)
			}
			index = end
		default:
			return nil, fmt.Errorf("%s must contain only strings", strings.TrimSpace(statement.name))
		}
	}
	return nil, fmt.Errorf("unterminated array in %s", strings.TrimSpace(statement.name))
}

// appendTOMLBlock appends a table block after existing content, separated by a
// blank line and ending with the file's newline convention.
func appendTOMLBlock(raw []byte, newline, block string) []byte {
	if len(raw) == 0 {
		return []byte(block + newline)
	}
	finalNewline := raw[len(raw)-1] == '\n' || raw[len(raw)-1] == '\r'
	separator := newline
	if finalNewline {
		separator = ""
	}
	if len(bytes.TrimSpace(raw)) > 0 {
		separator += newline
	}
	result := append(append([]byte(nil), raw...), []byte(separator+block)...)
	if finalNewline {
		result = append(result, []byte(newline)...)
	}
	return result
}

// insertTOMLLine adds a line at the end of a table section, before trailing
// blank lines.
func insertTOMLLine(section []byte, newline, line string) []byte {
	insertAt := len(section)
	for insertAt > 0 && (section[insertAt-1] == '\n' || section[insertAt-1] == '\r') {
		insertAt--
	}
	prefix := newline
	if insertAt == 0 || section[insertAt-1] == '\n' || section[insertAt-1] == '\r' {
		prefix = ""
	}
	return splice(section, insertAt, insertAt, []byte(prefix+line))
}
