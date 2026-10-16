package hostintegration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/skillruntime"
)

// planMode carries what a plan connects a scope to, so planning and the
// re-check in Apply compute the same content.
type planMode struct {
	workspace string
	binary    string
	hub       *RemoteHub
	remove    bool
}

func (mode planMode) local() bool { return mode.hub == nil && !mode.remove }

func (plan PlanResult) mode() planMode {
	return planMode{workspace: plan.Workspace, binary: plan.Binary, hub: plan.Hub, remove: plan.Remove}
}

// ProxyArgs returns the arguments of the host entry that starts the stdio
// proxy for a remote hub. Only the token file path is passed, never the token.
func ProxyArgs(hub RemoteHub) []string {
	args := []string{"mcp", "proxy", "--hub", hub.URL}
	if hub.TokenFile != "" {
		args = append(args, "--token-file", hub.TokenFile)
	}
	return args
}

// RemoteAccessDir returns the directory on this machine that holds what the
// proxy fetches from a hub: the cached skill folders and their writable state
// and config directories. Hosts must be allowed to use it, as they are for a
// local workspace's runtime directories.
func RemoteAccessDir(hubURL string) (string, error) {
	state, err := skillruntime.ClientStateDir()
	if err != nil {
		return "", fmt.Errorf("find this machine's skill cache directory: %w", err)
	}
	id, err := skillruntime.HubID(hubURL)
	if err != nil {
		return "", err
	}
	return filepath.Join(skillruntime.RemoteCacheDir(state), id), nil
}

func validateRemoteHub(hub RemoteHub) error {
	parsed, err := url.Parse(hub.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("hub URL %q must be an http or https URL with a host", hub.URL)
	}
	if hub.TokenFile != "" && !filepath.IsAbs(hub.TokenFile) {
		return fmt.Errorf("token file path must be absolute")
	}
	return nil
}

// registrationAccessDirs lists the allowed directories that connect added for
// an existing entry.
func registrationAccessDirs(registration Registration) []string {
	switch {
	case registration.Remote():
		dir, err := RemoteAccessDir(registration.HubURL)
		if err != nil {
			return nil
		}
		return []string{dir}
	case registration.Workspace != "" && filepath.IsAbs(registration.Workspace):
		return RuntimeAccessDirs(filepath.Clean(registration.Workspace))
	}
	return nil
}

func targetAccessDirs(mode planMode) ([]string, error) {
	switch {
	case mode.remove:
		return nil, nil
	case mode.hub != nil:
		dir, err := RemoteAccessDir(mode.hub.URL)
		if err != nil {
			return nil, err
		}
		return []string{dir}, nil
	}
	return RuntimeAccessDirs(mode.workspace), nil
}

// staleAccessDirs lists the directories added for the entry being replaced or
// removed that the new connection does not need. A local connection over a
// local entry keeps every directory, exactly as before remote hubs existed.
func staleAccessDirs(previous Registration, mode planMode) []string {
	if mode.local() && !previous.Remote() {
		return nil
	}
	target, _ := targetAccessDirs(mode)
	return missingEntries(target, registrationAccessDirs(previous))
}

// hubIDDirPattern matches the cache directory name skillruntime.HubID gives a
// hub: a host label and ten hex digits of the URL hash.
var hubIDDirPattern = regexp.MustCompile(`^[a-z0-9.-]*-[0-9a-f]{10}$`)

// isSkillHubAccessDir recognizes an allowed directory connect adds even when
// the entry that named it is gone: a remote hub's cache directory
// (.../skillhub/remote/<hub-id>), or a runtime directory of a directory that
// is a Skill Hub workspace.
func isSkillHubAccessDir(dir string) bool {
	clean := filepath.Clean(dir)
	parent := filepath.Dir(clean)
	if hubIDDirPattern.MatchString(filepath.Base(clean)) && filepath.Base(parent) == "remote" && filepath.Base(filepath.Dir(parent)) == "skillhub" {
		return true
	}
	for _, suffix := range [][]string{{"runtime", "cache", "skills"}, {"runtime", "envs"}, {"runtime", "config"}} {
		root := clean
		matched := true
		for index := len(suffix) - 1; index >= 0; index-- {
			if filepath.Base(root) != suffix[index] {
				matched = false
				break
			}
			root = filepath.Dir(root)
		}
		if matched {
			if info, err := os.Stat(filepath.Join(root, ".skillhub", "schema-version")); err == nil && info.Mode().IsRegular() {
				return true
			}
		}
	}
	return false
}

// removalDirs adds to the directories of the removed entry every directory
// in existing that is recognizably Skill Hub's, so a removal also cleans up
// after an entry that was deleted by hand or a cache directory that moved.
func removalDirs(stale, existing []string) []string {
	dirs := append([]string(nil), stale...)
	for _, dir := range existing {
		if isSkillHubAccessDir(dir) && len(missingEntries(dirs, []string{dir})) > 0 {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

// scopeRegistration reads the skillhub entry of a host's config file in a scope.
func scopeRegistration(adapter Adapter, scope Scope, root string) (Registration, error) {
	configRel, _ := adapter.relativePaths(scope)
	raw, _, _, err := readManagedFile(filepath.Join(root, filepath.FromSlash(configRel)), root)
	if err != nil {
		return Registration{}, err
	}
	return ReadRegistration(adapter.Host, raw), nil
}

// desiredConfig computes a host config file for any kind of plan. The second
// result reports that the file is left with nothing but Skill Hub content and
// is deleted instead.
func desiredConfig(host Host, raw []byte, mode planMode) ([]byte, bool, error) {
	previous := ReadRegistration(host, raw)
	stale := staleAccessDirs(previous, mode)
	switch {
	case mode.remove:
		return removedConfig(host, raw, stale)
	case mode.hub != nil:
		desired, err := remoteConfig(host, raw, mode, stale)
		return desired, false, err
	}
	base, err := stripHostAllowedDirs(host, raw, stale, false)
	if err != nil {
		return nil, false, err
	}
	supportsToggle := HostSupportsServerToggle(host)
	var desired []byte
	switch host {
	case HostClaude:
		desired, err = desiredClaudeCLICurationConfig(base, mode.binary, mode.workspace)
	case HostGemini:
		desired, err = desiredGeminiConfig(base, mode.binary, mode.workspace, supportsToggle)
	case HostCodex:
		desired, err = desiredCodexConfig(base, mode.binary, mode.workspace, supportsToggle)
	default:
		err = fmt.Errorf("unsupported host %q", host)
	}
	return desired, false, err
}

type geminiRemoteServer struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	Trust   bool     `json:"trust"`
}

// remoteConfig points the skillhub entry at the stdio proxy. A remote hub has
// no curation server: curation stays on the hub machine.
func remoteConfig(host Host, raw []byte, mode planMode, stale []string) ([]byte, error) {
	target, err := targetAccessDirs(mode)
	if err != nil {
		return nil, err
	}
	args := ProxyArgs(*mode.hub)
	switch host {
	case HostClaude:
		encoded, err := json.Marshal(claudeMCPServer{Type: "stdio", Command: mode.binary, Args: args})
		if err != nil {
			return nil, fmt.Errorf("encode Claude remote MCP registration: %w", err)
		}
		updated, err := upsertJSONPath(raw, []string{"mcpServers", "skillhub"}, encoded)
		if err != nil {
			return nil, err
		}
		return removeJSONPath(updated, []string{"mcpServers", "skillhub-curation"})
	case HostGemini:
		encoded, err := json.Marshal(geminiRemoteServer{Command: mode.binary, Args: args})
		if err != nil {
			return nil, fmt.Errorf("encode Gemini remote MCP registration: %w", err)
		}
		updated, err := upsertJSONPath(raw, []string{"mcpServers", "skillhub"}, encoded)
		if err == nil {
			// A local entry ran in the workspace; the proxy needs no directory.
			updated, err = removeJSONPath(updated, []string{"mcpServers", "skillhub", "cwd"})
		}
		if err == nil {
			updated, err = removeJSONPath(updated, []string{"mcpServers", "skillhub-curation"})
		}
		if err == nil {
			updated, err = stripJSONStringArray(updated, geminiAllowedDirsPath, stale, true)
		}
		if err != nil {
			return nil, err
		}
		return ensureJSONStringArray(updated, geminiAllowedDirsPath, target)
	case HostCodex:
		updated, err := upsertCodexTOMLManaged(raw, "mcp_servers.skillhub", []string{
			"command = " + tomlString(mode.binary),
			"args = " + tomlStringArray(args),
			"enabled = true",
		}, []string{"cwd"})
		if err == nil {
			updated, err = removeCodexTOMLTableTidy(updated, "mcp_servers.skillhub-curation")
		}
		if err == nil {
			updated, err = stripCodexWritableRoots(updated, stale, false)
		}
		if err != nil {
			return nil, err
		}
		return ensureCodexWritableRoots(updated, target)
	}
	return nil, fmt.Errorf("unsupported host %q", host)
}

// removedConfig removes the Skill Hub entries and the directories added for
// them. Containers left empty are removed too, and a file left empty is
// deleted, so a scope returns to what it was before it was connected.
func removedConfig(host Host, raw []byte, stale []string) ([]byte, bool, error) {
	var updated []byte
	var err error
	switch host {
	case HostClaude, HostGemini:
		updated = raw
		if host == HostGemini {
			existing, _ := jsonStringArrayAt(updated, geminiAllowedDirsPath)
			updated, err = stripJSONStringArray(updated, geminiAllowedDirsPath, removalDirs(stale, existing), true)
			if err == nil {
				updated, err = pruneEmptyJSON(updated, geminiAllowedDirsPath)
			}
			if err == nil {
				updated, err = pruneEmptyJSON(updated, geminiAllowedDirsPath[:1])
			}
		}
		for _, server := range []string{"skillhub-curation", "skillhub"} {
			if err == nil {
				updated, err = removeJSONPath(updated, []string{"mcpServers", server})
			}
		}
		if err == nil {
			updated, err = pruneEmptyJSON(updated, []string{"mcpServers"})
		}
		if err != nil {
			return nil, false, err
		}
		return updated, emptyJSONDocument(updated), nil
	case HostCodex:
		existing, _ := codexWritableRoots(raw)
		updated, err = stripCodexWritableRoots(raw, removalDirs(stale, existing), true)
		for _, table := range []string{"mcp_servers.skillhub-curation", "mcp_servers.skillhub"} {
			if err == nil {
				updated, err = removeCodexTOMLTableTidy(updated, table)
			}
		}
		if err != nil {
			return nil, false, err
		}
		return updated, len(bytes.TrimSpace(updated)) == 0, nil
	}
	return nil, false, fmt.Errorf("unsupported host %q", host)
}

// stripHostAllowedDirs removes directories from the allowance a host keeps in
// its MCP config file. Claude Code keeps it in a separate settings file.
func stripHostAllowedDirs(host Host, raw []byte, dirs []string, prune bool) ([]byte, error) {
	if len(dirs) == 0 {
		return raw, nil
	}
	switch host {
	case HostGemini:
		return stripJSONStringArray(raw, geminiAllowedDirsPath, dirs, true)
	case HostCodex:
		return stripCodexWritableRoots(raw, dirs, prune)
	}
	return raw, nil
}

// prepareConfigFor reads a host config file and plans it for any kind of plan.
func prepareConfigFor(host Host, path, root string, mode planMode) (preparedFile, error) {
	if mode.local() {
		return prepareConfig(host, path, root, mode)
	}
	raw, fileMode, exists, err := readManagedFile(path, root)
	if err != nil {
		return preparedFile{}, err
	}
	desired, remove, err := desiredConfig(host, raw, mode)
	if err != nil {
		return newPreparedFile(host, ChangeMCP, path, raw, raw, fileMode, exists, err.Error()), nil
	}
	file := newPreparedFile(host, ChangeMCP, path, raw, desired, fileMode, exists, "")
	previous := ReadRegistration(host, raw)
	if note := accessDirsPreview(host, raw, desired, staleAccessDirs(previous, mode)); note != "" {
		file.preview = boundPreview(file.preview + "\n" + note)
	}
	if remove {
		markRemoval(&file, exists)
	}
	return file, nil
}

// accessDirsPreview describes the directory entries a remote or removal plan
// adds to and removes from a host's config file.
func accessDirsPreview(host Host, raw, desired []byte, stale []string) string {
	read := func(content []byte) []string {
		var values []string
		switch host {
		case HostGemini:
			values, _ = jsonStringArrayAt(content, geminiAllowedDirsPath)
		case HostCodex:
			values, _ = codexWritableRoots(content)
		}
		return values
	}
	before, after := read(raw), read(desired)
	var notes []string
	if added := missingEntries(before, after); len(added) > 0 {
		notes = append(notes, "allowed directories to add: "+strings.Join(added, ", "))
	}
	if removed := missingEntries(after, before); len(removed) > 0 && len(stale) > 0 {
		notes = append(notes, "allowed directories to remove: "+strings.Join(removed, ", "))
	}
	return strings.Join(notes, "\n")
}

// markRemoval turns a planned file into a deletion: what remains of it holds
// nothing but Skill Hub content.
func markRemoval(file *preparedFile, exists bool) {
	file.remove = true
	file.desired = nil
	file.state.Current = !exists && file.state.Conflict == ""
	if exists {
		file.preview = boundPreview(file.preview + "\nthe file holds nothing else and will be deleted")
	}
}

// prepareNonLocalPermissions plans the Claude Code settings file for a remote
// hub (runtime directory of the hub's cache plus the CLI permission rules) or
// for a removal (both taken out again).
func prepareNonLocalPermissions(adapter Adapter, scope Scope, path, root string, mode planMode, previous Registration) (preparedFile, error) {
	host := adapter.Host
	raw, fileMode, exists, err := readManagedFile(path, root)
	if err != nil {
		return preparedFile{}, err
	}
	desired, remove, err := desiredNonLocalPermissions(adapter, settingsFile{scope, path}, root, raw, mode, previous)
	if err != nil {
		return newPreparedFile(host, ChangeHostPermissions, path, raw, raw, fileMode, exists, err.Error()), nil
	}
	file := newPreparedFile(host, ChangeHostPermissions, path, raw, desired, fileMode, exists, "")
	file.preview = claudePermissionsChangePreview(raw, desired)
	if remove {
		markRemoval(&file, exists)
	}
	return file, nil
}

// settingsFile names a Claude settings file and the scope it belongs to.
type settingsFile struct {
	scope Scope
	path  string
}

func desiredNonLocalPermissions(adapter Adapter, settings settingsFile, root string, raw []byte, mode planMode, previous Registration) ([]byte, bool, error) {
	stale := staleAccessDirs(previous, mode)
	if mode.remove {
		existing, _ := jsonStringArrayAt(raw, claudeAllowedDirsPath)
		stale = removalDirs(stale, existing)
	}
	updated, err := stripJSONStringArray(raw, claudeAllowedDirsPath, stale, true)
	if err != nil {
		return nil, false, err
	}
	if mode.remove {
		owned, err := managedRulesToRemove(settings.scope, settings.path)
		if err != nil {
			return nil, false, err
		}
		for _, rule := range claudeCLIRules {
			updated, err = stripJSONStringArray(updated, []string{"permissions", rule.key}, owned[rule.key], false)
			if err != nil {
				return nil, false, err
			}
		}
		// Prune in the reverse order connect adds the keys, so each removal
		// undoes one addition exactly.
		for _, key := range []string{"deny", "ask", "additionalDirectories", "allow"} {
			updated, err = pruneEmptyJSON(updated, []string{"permissions", key})
			if err != nil {
				return nil, false, err
			}
		}
		updated, err = pruneEmptyJSON(updated, []string{"permissions"})
		if err != nil {
			return nil, false, err
		}
		return updated, emptyJSONDocument(updated), nil
	}
	target, err := targetAccessDirs(mode)
	if err != nil {
		return nil, false, err
	}
	if updated, err = withoutRetiredClaudeCLIRules(updated, settings); err != nil {
		return nil, false, err
	}
	withDirs, err := ensureJSONStringArray(updated, claudeAllowedDirsPath, target)
	if err != nil {
		return nil, false, err
	}
	if settings.scope == ScopeProject && adapter.SharedPermissionsRelativePath != "" {
		sharedPath := filepath.Join(root, filepath.FromSlash(adapter.SharedPermissionsRelativePath))
		shared, _, sharedExists, readErr := readManagedFile(sharedPath, root)
		if readErr != nil {
			return nil, false, readErr
		}
		if sharedExists {
			sharedWithDirs, dirErr := ensureJSONStringArray(shared, claudeAllowedDirsPath, target)
			if dirErr == nil && bytes.Equal(sharedWithDirs, shared) {
				withDirs = updated
			}
		}
	}
	desired, err := desiredClaudeCLIRules(withDirs)
	return desired, false, err
}

// claudePermissionsChangePreview lists the managed permission entries a plan
// adds and removes.
func claudePermissionsChangePreview(raw, desired []byte) string {
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

// prepareInstructionFile plans the removal of an obsolete Skill Hub instruction
// block. It deletes the file when nothing else is left in it, because a file
// that held only the block is one an older connect created.
func prepareInstructionFile(path, root string) (preparedFile, error) {
	raw, fileMode, exists, err := readManagedFile(path, root)
	if err != nil {
		return preparedFile{}, err
	}
	desired, conflict := removeBootstrap(raw)
	if conflict != "" {
		return newPreparedFile("", ChangeBootstrap, path, raw, raw, fileMode, exists, conflict), nil
	}
	file := newPreparedFile("", ChangeBootstrap, path, raw, desired, fileMode, exists, "")
	if exists && !bytes.Equal(raw, desired) && len(bytes.TrimSpace(desired)) == 0 {
		markRemoval(&file, exists)
	}
	return file, nil
}

// removeBootstrap takes out the obsolete instruction block together with the
// blank line an older connect put before it. Content without a block comes
// back unchanged.
func removeBootstrap(raw []byte) ([]byte, string) {
	start, end, found, conflict := bootstrapSpan(raw)
	if conflict != "" {
		return nil, conflict
	}
	if !found {
		return raw, ""
	}
	before := bytes.TrimRight(raw[:start], "\r\n")
	after := bytes.TrimLeft(raw[end:], "\r\n")
	newline := detectNewline(raw)
	switch {
	case len(before) == 0:
		return append([]byte(nil), after...), ""
	case len(after) == 0:
		result := append([]byte(nil), before...)
		if end < len(raw) {
			result = append(result, newline...)
		}
		return result, ""
	}
	result := append(append([]byte(nil), before...), []byte(newline+newline)...)
	return append(result, after...), ""
}

// stripJSONStringArray removes values from the array of strings at path.
// Directories compare by cleaned path. The file is returned unchanged when no
// value is present.
func stripJSONStringArray(raw []byte, path []string, values []string, paths bool) ([]byte, error) {
	if len(values) == 0 {
		return raw, nil
	}
	existing, err := jsonStringArrayAt(raw, path)
	if err != nil {
		return nil, err
	}
	kept := removeEntries(existing, values, paths)
	if len(kept) == len(existing) {
		return raw, nil
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(kept); err != nil {
		return nil, fmt.Errorf("encode %s: %w", strings.Join(path, "."), err)
	}
	return upsertJSONPath(raw, path, bytes.TrimSpace(encoded.Bytes()))
}

func removeEntries(existing, values []string, paths bool) []string {
	kept := make([]string, 0, len(existing))
	for _, entry := range existing {
		drop := slices.ContainsFunc(values, func(value string) bool {
			if paths {
				return filepath.Clean(value) == filepath.Clean(entry)
			}
			return value == entry
		})
		if !drop {
			kept = append(kept, entry)
		}
	}
	return kept
}

// pruneEmptyJSON removes the member at path when its value is an empty object
// or array.
func pruneEmptyJSON(raw []byte, path []string) ([]byte, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return raw, nil
	}
	var node map[string]json.RawMessage
	if err := json.Unmarshal(raw, &node); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	for index, key := range path {
		value, ok := node[key]
		if !ok {
			return raw, nil
		}
		if index == len(path)-1 {
			compact := &bytes.Buffer{}
			if json.Compact(compact, value) != nil {
				return raw, nil
			}
			if compact.String() != "{}" && compact.String() != "[]" {
				return raw, nil
			}
			return removeJSONPath(raw, path)
		}
		node = nil
		if json.Unmarshal(value, &node) != nil || node == nil {
			return raw, nil
		}
	}
	return raw, nil
}

func emptyJSONDocument(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return true
	}
	compact := &bytes.Buffer{}
	return json.Compact(compact, trimmed) == nil && compact.String() == "{}"
}

// stripCodexWritableRoots removes directories from
// [sandbox_workspace_write].writable_roots. With prune, a list left empty is
// removed, and so is the table when nothing else is in it.
func stripCodexWritableRoots(raw []byte, dirs []string, prune bool) ([]byte, error) {
	if len(dirs) == 0 {
		return raw, nil
	}
	statements, err := scanTOMLStatements(raw)
	if err != nil {
		return nil, fmt.Errorf("cannot safely update Codex TOML: %w", err)
	}
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
	if sectionStart < 0 {
		return raw, nil
	}
	section := append([]byte(nil), raw[sectionStart:sectionEnd]...)
	sectionStatements, err := scanTOMLStatements(section)
	if err != nil {
		return nil, fmt.Errorf("cannot safely update Codex TOML: %w", err)
	}
	var match *tomlStatement
	assignments := 0
	for index, statement := range sectionStatements {
		if statement.kind != 'a' {
			continue
		}
		assignments++
		if normalizeTOMLTable(statement.name) == "writable_roots" {
			if match != nil {
				return nil, fmt.Errorf("duplicate TOML key writable_roots in %s", codexWritableRootsTable)
			}
			match = &sectionStatements[index]
		}
	}
	if match == nil {
		return raw, nil
	}
	existing, err := parseTOMLStringArray(section, *match)
	if err != nil {
		return nil, fmt.Errorf("cannot safely update writable_roots: %w", err)
	}
	kept := removeEntries(existing, dirs, true)
	if len(kept) == len(existing) {
		return raw, nil
	}
	if len(kept) > 0 || !prune {
		updated, _, err := replaceTOMLKey(section, "writable_roots", "writable_roots = "+tomlStringArray(kept))
		if err != nil {
			return nil, fmt.Errorf("cannot safely update writable_roots: %w", err)
		}
		return splice(raw, sectionStart, sectionEnd, updated), nil
	}
	if assignments == 1 {
		return removeCodexTOMLTableTidy(raw, codexWritableRootsTable)
	}
	return splice(raw, sectionStart, sectionEnd, splice(section, match.start, match.end, nil)), nil
}

// removeCodexTOMLTableTidy removes a table like removeCodexTOMLTable and, when
// the table was the last one, also the blank line connect put before it.
func removeCodexTOMLTableTidy(raw []byte, table string) ([]byte, error) {
	statements, err := scanTOMLStatements(raw)
	if err != nil {
		return nil, fmt.Errorf("cannot safely update Codex TOML: %w", err)
	}
	target := normalizeTOMLTable(table)
	last := false
	for index, statement := range statements {
		if statement.kind != 'h' || normalizeTOMLTable(statement.name) != target {
			continue
		}
		last = !slices.ContainsFunc(statements[index+1:], func(later tomlStatement) bool { return later.kind == 'h' })
	}
	updated, err := removeCodexTOMLTable(raw, table)
	if err != nil || bytes.Equal(updated, raw) || !last {
		return updated, err
	}
	trimmed := bytes.TrimRight(updated, "\r\n")
	if len(trimmed) == 0 {
		return []byte{}, nil
	}
	return append(append([]byte(nil), trimmed...), detectNewline(raw)...), nil
}

// upsertCodexTOMLManaged sets the managed key lines of a TOML table, adding
// the table when it is missing, and removes the keys in drop.
func upsertCodexTOMLManaged(raw []byte, table string, managed, drop []string) ([]byte, error) {
	updated, err := upsertCodexTOMLLines(raw, table, managed)
	if err != nil || len(drop) == 0 {
		return updated, err
	}
	statements, err := scanTOMLStatements(updated)
	if err != nil {
		return nil, fmt.Errorf("cannot safely update Codex TOML: %w", err)
	}
	target := normalizeTOMLTable(table)
	sectionStart, sectionEnd := -1, len(updated)
	for index, statement := range statements {
		if statement.kind != 'h' || normalizeTOMLTable(statement.name) != target {
			continue
		}
		sectionStart = statement.start
		for _, later := range statements[index+1:] {
			if later.kind == 'h' {
				sectionEnd = later.start
				break
			}
		}
		break
	}
	if sectionStart < 0 {
		return updated, nil
	}
	section := append([]byte(nil), updated[sectionStart:sectionEnd]...)
	for _, key := range drop {
		sectionStatements, err := scanTOMLStatements(section)
		if err != nil {
			return nil, fmt.Errorf("cannot safely update Codex TOML: %w", err)
		}
		for index := len(sectionStatements) - 1; index >= 0; index-- {
			statement := sectionStatements[index]
			if statement.kind == 'a' && normalizeTOMLTable(statement.name) == key {
				section = splice(section, statement.start, statement.end, nil)
			}
		}
	}
	return splice(updated, sectionStart, sectionEnd, section), nil
}

// readConfigRegistration reads the registration of a host in the scope of a
// plan, for the re-check in Apply.
func readConfigRegistration(plan PlanResult, host Host) (Registration, error) {
	index := slices.IndexFunc(adapters, func(adapter Adapter) bool { return adapter.Host == host })
	if index < 0 {
		return Registration{}, fmt.Errorf("unsupported host %q", host)
	}
	root := plan.Root
	if root == "" {
		root = plan.Workspace
	}
	return scopeRegistration(adapters[index], plan.Scope, root)
}

// isRemoval reports whether a change deletes its file.
func isRemoval(change Change) bool { return change.Remove }

// removeEmptyParents deletes the directories above a removed file that are
// left empty, up to the scope root, so a removal leaves no empty host
// directory behind. A directory that still holds anything stays, because
// removing a non-empty directory fails.
func removeEmptyParents(root *os.Root, relative string) {
	for directory := filepath.Dir(relative); directory != "." && directory != string(filepath.Separator); directory = filepath.Dir(directory) {
		if root.Remove(directory) != nil {
			return
		}
	}
}
