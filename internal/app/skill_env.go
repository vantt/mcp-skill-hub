package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

const (
	configEnvFileName = "env"
	configEnvMaxBytes = 1 << 20
	configEnvMaxValue = 32 << 10
	// configEnvReservedPrefix keeps user keys from replacing the variables the
	// hub itself exports.
	configEnvReservedPrefix = "SKILLHUB_"
)

// safeEnvValue lists the characters that need no quoting when the file is
// sourced by a POSIX shell.
var safeEnvValue = regexp.MustCompile(`^[A-Za-z0-9_./:@%+=,-]+$`)

// SkillEnvResult reports one env command. It carries key names only; values
// are never part of any result.
type SkillEnvResult struct {
	Result
	SkillID         string   `json:"skill_id"`
	ConfigDirectory string   `json:"config_directory,omitempty"`
	Keys            []string `json:"keys"`
}

// SkillEnvService manages the per-skill secret environment file
// runtime/config/<id>/env. The file is private to the user (0600 in a 0700
// directory) and lives outside Git. Values enter through Set only and are
// never returned, logged, cached, or recorded in telemetry.
type SkillEnvService struct{}

// Set stores or replaces one variable, preserving the others.
func (SkillEnvService) Set(ctx context.Context, workspacePath, skillID, key, value string) (SkillEnvResult, error) {
	if err := validateEnvKey(key); err != nil {
		return SkillEnvResult{}, err
	}
	if err := validateEnvValue(value); err != nil {
		return SkillEnvResult{}, err
	}
	// Setting a variable for a skill that does not exist is almost always a typo.
	if _, err := (SkillService{}).ReadSkill(ctx, workspacePath, skillID); err != nil {
		return SkillEnvResult{}, err
	}
	_, directory, err := openConfigDir(workspacePath, skillID, true)
	if err != nil {
		return SkillEnvResult{}, err
	}
	entries, err := readConfigEnvEntries(directory)
	if err != nil {
		return SkillEnvResult{}, err
	}
	entries[key] = value
	if err := writeConfigEnvEntries(directory, entries); err != nil {
		return SkillEnvResult{}, err
	}
	return envResult(skillID, directory, entries, fmt.Sprintf("Stored %s for skill %s.", key, skillID)), nil
}

// Unset removes one variable; removing an absent key succeeds.
func (SkillEnvService) Unset(_ context.Context, workspacePath, skillID, key string) (SkillEnvResult, error) {
	if err := validateEnvKey(key); err != nil {
		return SkillEnvResult{}, err
	}
	_, directory, err := openConfigDir(workspacePath, skillID, false)
	if err != nil {
		return SkillEnvResult{}, err
	}
	entries, err := readConfigEnvEntries(directory)
	if err != nil {
		return SkillEnvResult{}, err
	}
	if _, present := entries[key]; present {
		delete(entries, key)
		if err := writeConfigEnvEntries(directory, entries); err != nil {
			return SkillEnvResult{}, err
		}
	}
	return envResult(skillID, directory, entries, fmt.Sprintf("Removed %s from skill %s.", key, skillID)), nil
}

// List returns the stored key names, never values.
func (SkillEnvService) List(_ context.Context, workspacePath, skillID string) (SkillEnvResult, error) {
	_, directory, err := openConfigDir(workspacePath, skillID, false)
	if err != nil {
		return SkillEnvResult{}, err
	}
	entries, err := readConfigEnvEntries(directory)
	if err != nil {
		return SkillEnvResult{}, err
	}
	return envResult(skillID, directory, entries, fmt.Sprintf("Skill %s has %d stored variable(s).", skillID, len(entries))), nil
}

func envResult(skillID, directory string, entries map[string]string, summary string) SkillEnvResult {
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return SkillEnvResult{Result: NewResult(StatusReady, summary), SkillID: skillID, ConfigDirectory: directory, Keys: keys}
}

func validateEnvKey(key string) error {
	if !skillruntime.ValidEnvName(key) {
		return NewInvalidRequestError("invalid variable name", "Use letters, digits, and underscores, starting with a letter or underscore.")
	}
	if strings.HasPrefix(strings.ToUpper(key), configEnvReservedPrefix) {
		return NewInvalidRequestError("variable names starting with "+configEnvReservedPrefix+" are reserved", "Choose another name.")
	}
	return nil
}

func validateEnvValue(value string) error {
	switch {
	case value == "":
		return NewInvalidRequestError("the value is empty", "Provide a value, or use `skillhub skill env unset` to remove the variable.")
	case len(value) > configEnvMaxValue:
		return NewInvalidRequestError("the value is too large", "Store values of at most 32 KiB.")
	case !utf8.ValidString(value) || strings.ContainsAny(value, "\r\n\x00"):
		return NewInvalidRequestError("the value must be a single line of text without control characters", "Store a single-line value.")
	}
	return nil
}

// openConfigDir resolves the skill's config directory under the workspace,
// refusing symlinked components. Without create, a missing directory returns a
// path that readConfigEnvEntries treats as empty.
func openConfigDir(workspacePath, skillID string, create bool) (string, string, error) {
	if !snapshotSkillIDPattern.MatchString(skillID) {
		return "", "", NewInvalidRequestError(fmt.Sprintf("invalid skill id %q", skillID), "Pass a skill ID from `skillhub skill list`.")
	}
	root, err := workspace.Discover(workspacePath)
	if err != nil {
		return "", "", err
	}
	directory, err := ensureSnapshotDirectory(root, skillruntime.ConfigDirRoot+"/"+skillID, create)
	if errors.Is(err, fs.ErrNotExist) && !create {
		return root, filepath.Join(root, filepath.FromSlash(skillruntime.ConfigDirRoot), skillID), nil
	}
	if err != nil {
		return "", "", fmt.Errorf("open config directory: %w", err)
	}
	return root, directory, nil
}

// readConfigEnvEntries parses the env file. A missing file or directory is an
// empty set; a symlink, non-regular file, or malformed line is an error that
// names the line number, never its content.
func readConfigEnvEntries(directory string) (map[string]string, error) {
	entries := map[string]string{}
	path := filepath.Join(directory, configEnvFileName)
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return entries, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config env: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("unsafe config env file: %s must be a regular file", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read config env: %w", err)
	}
	defer file.Close()
	contents, err := io.ReadAll(io.LimitReader(file, configEnvMaxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read config env: %w", err)
	}
	if len(contents) > configEnvMaxBytes {
		return nil, fmt.Errorf("config env file %s is larger than %d bytes", path, configEnvMaxBytes)
	}
	for number, line := range strings.Split(string(contents), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		key, raw, found := strings.Cut(line, "=")
		value, ok := parseEnvValue(raw)
		if !found || !ok || !skillruntime.ValidEnvName(key) {
			return nil, fmt.Errorf("config env file %s has an invalid entry on line %d", path, number+1)
		}
		entries[key] = value
	}
	return entries, nil
}

// formatEnvValue quotes a value for POSIX shell sourcing when it contains
// anything beyond a conservative safe set.
func formatEnvValue(value string) string {
	if safeEnvValue.MatchString(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// parseEnvValue reverses formatEnvValue and also accepts a raw value.
func parseEnvValue(raw string) (string, bool) {
	if !strings.HasPrefix(raw, "'") {
		return raw, !strings.ContainsAny(raw, "'\x00")
	}
	var value strings.Builder
	rest := raw[1:]
	for {
		end := strings.IndexByte(rest, '\'')
		if end < 0 {
			return "", false
		}
		value.WriteString(rest[:end])
		rest = rest[end+1:]
		if rest == "" {
			return value.String(), true
		}
		if !strings.HasPrefix(rest, `\''`) {
			return "", false
		}
		value.WriteByte('\'')
		rest = rest[3:]
	}
}

// writeConfigEnvEntries atomically replaces the env file with mode 0600.
func writeConfigEnvEntries(directory string, entries map[string]string) error {
	keys := make([]string, 0, len(entries))
	for key := range entries {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var contents strings.Builder
	for _, key := range keys {
		contents.WriteString(key + "=" + formatEnvValue(entries[key]) + "\n")
	}
	temp, err := os.CreateTemp(directory, ".env-*")
	if err != nil {
		return fmt.Errorf("write config env: %w", err)
	}
	tempPath := temp.Name()
	cleanup := func() { _ = os.Remove(tempPath) }
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		cleanup()
		return fmt.Errorf("write config env: %w", err)
	}
	if _, err := temp.WriteString(contents.String()); err != nil {
		_ = temp.Close()
		cleanup()
		return fmt.Errorf("write config env: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		cleanup()
		return fmt.Errorf("write config env: %w", err)
	}
	if err := temp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("write config env: %w", err)
	}
	if err := os.Rename(tempPath, filepath.Join(directory, configEnvFileName)); err != nil {
		cleanup()
		return fmt.Errorf("write config env: %w", err)
	}
	return nil
}

// loadSkillConfigEnv reads a skill's stored variables for the doctor. The
// values are returned to the caller's process only.
func loadSkillConfigEnv(root, skillID string) (map[string]string, error) {
	directory, err := ensureSnapshotDirectory(root, skillruntime.ConfigDirRoot+"/"+skillID, false)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open config directory: %w", err)
	}
	return readConfigEnvEntries(directory)
}

// redactValues replaces every stored value in text with a placeholder so check
// output cannot echo a secret back to the terminal.
func redactValues(text string, values map[string]string) string {
	for _, value := range values {
		if value != "" {
			text = strings.ReplaceAll(text, value, "***")
		}
	}
	return text
}
