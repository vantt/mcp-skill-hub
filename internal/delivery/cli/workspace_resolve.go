package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	workspaceFix         = "Pass `--workspace <path>`, set the SKILLHUB_WORKSPACE environment variable, or run `skillhub init <path> --yes`."
	maxConnectionFileLen = 1024 * 1024 // 1 MB bounded read for untrusted connection files
)

// WorkspaceResolutionError encapsulates a user-facing workspace resolution failure.
type WorkspaceResolutionError struct {
	Path string
	Why  string
	Fix  string
}

func (e *WorkspaceResolutionError) Error() string {
	return e.Why
}

// writeWorkspaceResolutionError renders a workspace resolution error using the standard error format.
func writeWorkspaceResolutionError(stdout, stderr io.Writer, jsonOutput bool, err *WorkspaceResolutionError) int {
	return writeInvalidRequest(stdout, stderr, jsonOutput, err.Why, err.Fix)
}

// resolveWorkspace implements the resolution order:
//  1. --workspace flag
//  2. SKILLHUB_WORKSPACE environment variable
//  3. Connection files (.mcp.json / .codex/config.toml / .gemini/settings.json) in cwd or nearest parent
//  4. Upward discovery of .skillhub/schema-version
//
// Home directory is never scanned.
func resolveWorkspace(flagValue string) (string, *WorkspaceResolutionError) {
	// 1. Explicit flag
	flagValue = strings.TrimSpace(flagValue)
	if flagValue != "" {
		return validateExplicitWorkspace(flagValue)
	}

	// 2. SKILLHUB_WORKSPACE environment variable
	envValue := strings.TrimSpace(os.Getenv(workspaceEnvVar))
	if envValue != "" {
		return validateExplicitWorkspace(envValue)
	}

	// 3. Search cwd and upward for connection file or .skillhub/schema-version
	cwd, err := os.Getwd()
	if err != nil {
		return "", &WorkspaceResolutionError{
			Why: "The current directory cannot be determined.",
			Fix: workspaceFix,
		}
	}

	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		homeDir = filepath.Clean(homeDir)
	}

	// 3a. Connection file in cwd or nearest parent (project scope only)
	for dir := cwd; ; {
		if isHomeDir(dir, homeDir) {
			break
		}
		if ws, ok := findWorkspaceInConnectionFiles(dir); ok {
			return ws, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// 4. Upward discovery of .skillhub/schema-version (never scanning home)
	for dir := cwd; ; {
		if isHomeDir(dir, homeDir) {
			break
		}
		schemaPath := filepath.Join(dir, ".skillhub", "schema-version")
		if _, err := os.Stat(schemaPath); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	return "", &WorkspaceResolutionError{
		Why: "No Skill Hub workspace was found from the current directory.",
		Fix: workspaceFix,
	}
}

// validateExplicitWorkspace verifies that an explicitly provided workspace path exists and is initialized.
func validateExplicitWorkspace(rawPath string) (string, *WorkspaceResolutionError) {
	abs, err := filepath.Abs(rawPath)
	if err != nil {
		return "", &WorkspaceResolutionError{
			Path: rawPath,
			Why:  fmt.Sprintf("The workspace path cannot be resolved: %v", err),
			Fix:  workspaceFix,
		}
	}
	info, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", &WorkspaceResolutionError{
				Path: rawPath,
				Why:  fmt.Sprintf("No Skill Hub workspace at %s.", rawPath),
				Fix:  workspaceFix,
			}
		}
		return "", &WorkspaceResolutionError{
			Path: rawPath,
			Why:  fmt.Sprintf("Cannot access workspace at %s: %v", rawPath, err),
			Fix:  workspaceFix,
		}
	}
	if !info.IsDir() {
		return "", &WorkspaceResolutionError{
			Path: rawPath,
			Why:  fmt.Sprintf("No Skill Hub workspace at %s.", rawPath),
			Fix:  workspaceFix,
		}
	}
	schemaFile := filepath.Join(abs, ".skillhub", "schema-version")
	if _, err := os.Stat(schemaFile); err != nil {
		if _, skillhubErr := os.Stat(filepath.Join(abs, ".skillhub")); skillhubErr == nil {
			return abs, nil
		}
		return "", &WorkspaceResolutionError{
			Path: rawPath,
			Why:  fmt.Sprintf("No Skill Hub workspace was found at %s.", rawPath),
			Fix:  workspaceFix,
		}
	}
	return abs, nil
}

// isHomeDir checks if dir matches the user's home directory (clean and symlink-resolved).
func isHomeDir(dir, homeDir string) bool {
	if homeDir == "" {
		return false
	}
	cleanDir := filepath.Clean(dir)
	if cleanDir == homeDir {
		return true
	}
	evalDir, errDir := filepath.EvalSymlinks(cleanDir)
	evalHome, errHome := filepath.EvalSymlinks(homeDir)
	if errDir == nil && errHome == nil && evalDir == evalHome {
		return true
	}
	return false
}

// findWorkspaceInConnectionFiles checks for .mcp.json, .codex/config.toml, .gemini/settings.json in dir.
func findWorkspaceInConnectionFiles(dir string) (string, bool) {
	// Order: Claude Code (.mcp.json), Codex (.codex/config.toml), Gemini (.gemini/settings.json)
	if ws, ok := readConnectionFile(dir, ".mcp.json", extractWorkspaceFromJSON); ok {
		return ws, true
	}
	if ws, ok := readConnectionFile(dir, filepath.Join(".codex", "config.toml"), extractWorkspaceFromCodexTOML); ok {
		return ws, true
	}
	if ws, ok := readConnectionFile(dir, filepath.Join(".gemini", "settings.json"), extractWorkspaceFromJSON); ok {
		return ws, true
	}
	return "", false
}

// readConnectionFile safely inspects a candidate connection file:
// - bounded read (<= 1MB)
// - does not follow symlinks outside the project dir
// - verifies the extracted workspace is an absolute existing dir containing .skillhub/schema-version
func readConnectionFile(dir, relativePath string, extract func([]byte) (string, bool)) (string, bool) {
	filePath := filepath.Join(dir, relativePath)

	lstat, err := os.Lstat(filePath)
	if err != nil {
		return "", false
	}

	// Symlink check: ensure symlink doesn't escape project directory
	if lstat.Mode()&os.ModeSymlink != 0 {
		evalTarget, err := filepath.EvalSymlinks(filePath)
		if err != nil {
			return "", false
		}
		rel, err := filepath.Rel(dir, evalTarget)
		if err != nil || strings.HasPrefix(rel, "..") || rel == ".." || filepath.IsAbs(rel) {
			return "", false
		}
	}

	if lstat.Size() > maxConnectionFileLen {
		return "", false
	}

	file, err := os.Open(filePath)
	if err != nil {
		return "", false
	}
	defer file.Close()

	contents, err := io.ReadAll(io.LimitReader(file, maxConnectionFileLen+1))
	if err != nil || int64(len(contents)) > maxConnectionFileLen {
		return "", false
	}

	candidate, ok := extract(contents)
	if !ok {
		return "", false
	}

	candidate = strings.TrimSpace(candidate)
	if candidate == "" || !filepath.IsAbs(candidate) {
		return "", false
	}

	info, err := os.Stat(candidate)
	if err != nil || !info.IsDir() {
		return "", false
	}

	if _, err := os.Stat(filepath.Join(candidate, ".skillhub", "schema-version")); err != nil {
		return "", false
	}

	return candidate, true
}

// extractWorkspaceFromJSON parses .mcp.json or .gemini/settings.json looking for skillhub entry.
func extractWorkspaceFromJSON(contents []byte) (string, bool) {
	var root struct {
		MCPServers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
			CWD     string   `json:"cwd"`
		} `json:"mcpServers"`
	}
	if err := json.Unmarshal(contents, &root); err != nil {
		return "", false
	}
	server, found := root.MCPServers["skillhub"]
	if !found {
		return "", false
	}
	// Look for --workspace in args
	for i := range server.Args {
		if server.Args[i] == "--workspace" && i+1 < len(server.Args) {
			return strings.TrimSpace(server.Args[i+1]), true
		}
	}
	if server.CWD != "" {
		return strings.TrimSpace(server.CWD), true
	}
	return "", false
}

// extractWorkspaceFromCodexTOML parses .codex/config.toml looking for [mcp_servers.skillhub] entry.
func extractWorkspaceFromCodexTOML(contents []byte) (string, bool) {
	lines := bytes.Split(contents, []byte("\n"))
	inSkillhubTable := false

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(string(lines[i]))
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			table := strings.Trim(line, "[]")
			table = strings.TrimSpace(table)
			if table == "mcp_servers.skillhub" || table == "mcp_servers.\"skillhub\"" || table == "mcp_servers.'skillhub'" {
				inSkillhubTable = true
			} else {
				inSkillhubTable = false
			}
			continue
		}
		if !inSkillhubTable {
			continue
		}

		// Look for args = [ ... "--workspace", "<path>" ... ]
		if strings.HasPrefix(line, "args") && strings.Contains(line, "=") {
			// May be on one line or multiline
			rawArgs := line
			for !strings.Contains(rawArgs, "]") && i+1 < len(lines) {
				i++
				rawArgs += " " + strings.TrimSpace(string(lines[i]))
			}
			if ws := extractWorkspaceFromTOMLArgs(rawArgs); ws != "" {
				return ws, true
			}
		}

		// Fallback: look for cwd = "<path>"
		if strings.HasPrefix(line, "cwd") && strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				val := strings.TrimSpace(parts[1])
				val = strings.Trim(val, `"'`)
				if val != "" {
					return val, true
				}
			}
		}
	}
	return "", false
}

// extractWorkspaceFromTOMLArgs extracts the value after "--workspace" in a TOML array string.
func extractWorkspaceFromTOMLArgs(raw string) string {
	// Tokenize string literals in the array
	var tokens []string
	var current strings.Builder
	inQuote := false
	quoteChar := byte(0)
	escaped := false

	for i := range raw {
		b := raw[i]
		if inQuote {
			if escaped {
				current.WriteByte(b)
				escaped = false
				continue
			}
			if b == '\\' {
				escaped = true
				continue
			}
			if b == quoteChar {
				tokens = append(tokens, current.String())
				current.Reset()
				inQuote = false
				continue
			}
			current.WriteByte(b)
		} else {
			if b == '"' || b == '\'' {
				inQuote = true
				quoteChar = b
			}
		}
	}

	for i := range tokens {
		if tokens[i] == "--workspace" && i+1 < len(tokens) {
			return tokens[i+1]
		}
	}
	return ""
}
