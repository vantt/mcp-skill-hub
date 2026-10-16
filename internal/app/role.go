package app

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/vantt/skillhub-mcp/internal/hostintegration"
	"github.com/vantt/skillhub-mcp/internal/shellquote"
	"github.com/vantt/skillhub-mcp/internal/userconfig"
)

// The roles a folder can have for Skill Hub.
const (
	RoleHub          = "hub"
	RoleClientLocal  = "client-local"
	RoleClientRemote = "client-remote"
	RoleBoth         = "both"
	RoleNone         = "none"
)

// HubRef names the hub folder that contains the current directory.
type HubRef struct {
	Root string `json:"root"`
}

// ClientEntry is the skillhub entry of one agent host in one scope.
type ClientEntry struct {
	Host hostintegration.Host `json:"host"`
	// Global is true for an entry in the user scope.
	Global bool `json:"global"`
	// Root is the project folder, or the home folder for the user scope.
	Root string `json:"root"`
	// Path is the host config file that holds the entry.
	Path string `json:"path"`
	// Workspace is the local hub folder the entry serves; empty for a remote hub.
	Workspace string `json:"workspace,omitempty"`
	HubURL    string `json:"hub_url,omitempty"`
	TokenFile string `json:"token_file,omitempty"`
	// Self marks the hub's own connection: an entry in the hub folder that
	// points at that hub.
	Self bool `json:"self,omitempty"`
}

// Remote reports whether the entry starts the proxy for a hub on another machine.
func (entry ClientEntry) Remote() bool { return entry.HubURL != "" }

// HostRole holds the entries of one agent host. Effective is the entry the
// host uses: the project entry when there is one, else the user entry.
type HostRole struct {
	Host      hostintegration.Host `json:"host"`
	Project   *ClientEntry         `json:"project,omitempty"`
	User      *ClientEntry         `json:"user,omitempty"`
	Effective *ClientEntry         `json:"effective,omitempty"`
}

// RoleReport is what DetectRole found for a directory.
type RoleReport struct {
	Cwd  string  `json:"cwd"`
	Home string  `json:"home,omitempty"`
	Hub  *HubRef `json:"hub,omitempty"`
	// ProjectRoot is the nearest folder, from Cwd upward, that holds a
	// skillhub entry of any host; empty when there is none.
	ProjectRoot  string     `json:"project_root,omitempty"`
	Hosts        []HostRole `json:"hosts"`
	StoredHub    string     `json:"stored_hub,omitempty"`
	PreviousHubs []string   `json:"previous_hubs,omitempty"`
	// Workspace is the hub folder the person chose explicitly (--workspace or
	// SKILLHUB_WORKSPACE); empty when none was chosen. WithWorkspace sets it.
	Workspace string `json:"workspace,omitempty"`
}

// WithWorkspace narrows the report to an explicitly chosen hub folder: the
// health checks look at that hub, and of the local connections only the
// entries that point at it are kept, so `doctor --fix --workspace <hub>`
// never touches a connection that belongs to another hub. Entries for remote
// hubs stay; they are reported, never rewritten.
func (report RoleReport) WithWorkspace(root string) RoleReport {
	narrowed := report
	narrowed.Workspace = filepath.Clean(root)
	narrowed.Hosts = make([]HostRole, 0, len(report.Hosts))
	keep := func(entry *ClientEntry) *ClientEntry {
		if entry == nil || entry.Remote() || sameDirectory(entry.Workspace, narrowed.Workspace) {
			return entry
		}
		return nil
	}
	for _, host := range report.Hosts {
		role := HostRole{Host: host.Host, Project: keep(host.Project), User: keep(host.User)}
		role.Effective = role.Project
		if role.Effective == nil {
			role.Effective = role.User
		}
		narrowed.Hosts = append(narrowed.Hosts, role)
	}
	return narrowed
}

// HealthRole is the role status and doctor report: Kind, except that an
// explicitly chosen hub makes a folder without any role a hub.
func (report RoleReport) HealthRole() string {
	kind := report.Kind()
	if kind == RoleNone && report.Workspace != "" {
		return RoleHub
	}
	return kind
}

// HealthHub is the hub folder whose health status and doctor check: the hub
// chosen explicitly, else the hub that contains the current directory, else
// the hub on this machine that a project entry, then a user entry, points at.
// It is empty when no hub on this machine applies.
func (report RoleReport) HealthHub() string {
	if report.Workspace != "" {
		return report.Workspace
	}
	if report.Hub != nil {
		return report.Hub.Root
	}
	for _, global := range []bool{false, true} {
		for _, host := range report.Hosts {
			entry := host.Project
			if global {
				if host.Project != nil {
					continue
				}
				entry = host.User
			}
			if entry != nil && !entry.Remote() && IsHubRoot(entry.Workspace) {
				return entry.Workspace
			}
		}
	}
	return ""
}

// hubCommand adds the explicitly chosen hub to a skillhub command, so a
// printed command acts on the same hub when it is run from the same folder.
func (report RoleReport) hubCommand(command string) string {
	if report.Workspace == "" {
		return command
	}
	return command + " --workspace " + shellquote.Arg(report.Workspace)
}

// DetectRole finds the hub and the agent connections that apply to cwd. It
// walks from cwd up to the filesystem root and stops when it reaches home,
// which is never scanned: the nearest folder with .skillhub/schema-version is
// the hub, and the nearest folder with a skillhub entry holds the project
// entries. User-scope entries come from the hosts' user config files in home.
// It only reads files.
func DetectRole(cwd, home string, cfg userconfig.Config) RoleReport {
	report := RoleReport{Cwd: filepath.Clean(cwd), StoredHub: cfg.Hub, PreviousHubs: append([]string(nil), cfg.PreviousHubs...)}
	if home != "" {
		report.Home = filepath.Clean(home)
	}
	adapters := hostintegration.SupportedAdapters()
	project := map[hostintegration.Host]*ClientEntry{}
	for dir := report.Cwd; ; {
		if report.Home != "" && sameDirectory(dir, report.Home) {
			break
		}
		if report.Hub == nil {
			if info, err := os.Stat(filepath.Join(dir, ".skillhub", "schema-version")); err == nil && info.Mode().IsRegular() {
				report.Hub = &HubRef{Root: dir}
			}
		}
		if report.ProjectRoot == "" {
			for _, adapter := range adapters {
				if entry, ok := readClientEntry(adapter.Host, dir, adapter.ConfigRelativePath, false); ok {
					project[adapter.Host] = entry
				}
			}
			if len(project) > 0 {
				report.ProjectRoot = dir
			}
		}
		if report.Hub != nil && report.ProjectRoot != "" {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	for _, adapter := range adapters {
		role := HostRole{Host: adapter.Host, Project: project[adapter.Host]}
		if role.Project != nil && report.Hub != nil && !role.Project.Remote() &&
			sameDirectory(role.Project.Root, report.Hub.Root) && role.Project.Workspace != "" && sameDirectory(role.Project.Workspace, report.Hub.Root) {
			role.Project.Self = true
		}
		if report.Home != "" {
			if entry, ok := readClientEntry(adapter.Host, report.Home, adapter.UserConfigRelativePath, true); ok {
				role.User = entry
			}
		}
		role.Effective = role.Project
		if role.Effective == nil {
			role.Effective = role.User
		}
		report.Hosts = append(report.Hosts, role)
	}
	return report
}

// Kind classifies the report: hub, client-local, client-remote, both or
// none. The hub's own connection does not make the hub a client.
func (report RoleReport) Kind() string {
	local, remote := false, false
	for _, host := range report.Hosts {
		if host.Effective == nil || host.Effective.Self {
			continue
		}
		if host.Effective.Remote() {
			remote = true
		} else {
			local = true
		}
	}
	switch {
	case report.Hub != nil && (local || remote):
		return RoleBoth
	case report.Hub != nil:
		return RoleHub
	case local:
		return RoleClientLocal
	case remote:
		return RoleClientRemote
	}
	return RoleNone
}

// EffectiveEntries returns the effective entry of every host that has one,
// in host order.
func (report RoleReport) EffectiveEntries() []ClientEntry {
	var entries []ClientEntry
	for _, host := range report.Hosts {
		if host.Effective != nil {
			entries = append(entries, *host.Effective)
		}
	}
	return entries
}

// Entries returns every project and user entry, project entries first.
func (report RoleReport) Entries() []ClientEntry {
	var entries []ClientEntry
	for _, host := range report.Hosts {
		if host.Project != nil {
			entries = append(entries, *host.Project)
		}
	}
	for _, host := range report.Hosts {
		if host.User != nil {
			entries = append(entries, *host.User)
		}
	}
	return entries
}

// DetectRoleHere runs DetectRole for the current directory, the user's home
// and the stored user configuration. An unreadable user configuration counts
// as empty, so a damaged file never hides the hub of the current directory.
func DetectRoleHere() RoleReport {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	cfg, err := userconfig.Default().Load()
	if err != nil {
		cfg = userconfig.Config{}
	}
	return DetectRole(cwd, home, cfg)
}

// NewHubCommand is the command that creates a hub in ~/skill-hub, written for
// the shell of this platform.
func NewHubCommand() string {
	return shellquote.Chain(shellquote.MakeDir("~/skill-hub"), "cd ~/skill-hub", "skillhub init")
}

// MovedHubFix tells how to repair a connection whose hub folder (missing) is
// gone: when the stored hub itself is gone, init must record the hub's new
// location first and doctor --fix then re-points each project; when missing
// is a hub this machine recorded before and the stored hub exists, doctor
// --fix re-points the project; a folder never recorded is connected again.
func MovedHubFix(report RoleReport, missing string) string {
	storedExists := IsHubRoot(report.StoredHub)
	recorded := slices.ContainsFunc(report.PreviousHubs, func(path string) bool { return missing != "" && filepath.Clean(path) == filepath.Clean(missing) })
	switch {
	case report.StoredHub != "" && (!storedExists || filepath.Clean(report.StoredHub) == filepath.Clean(missing)):
		return "If you moved the hub, run `" + shellquote.Chain("cd <new-location>", "skillhub init") + "` there, then `skillhub doctor --fix` in each project that uses it."
	case recorded:
		return "The hub now lives at " + report.StoredHub + "; run `skillhub doctor --fix` to point this project at it."
	case storedExists:
		return "Run `skillhub connect " + shellquote.Arg(report.StoredHub) + "` to use the hub stored on this machine."
	}
	return "To create a hub, run `" + NewHubCommand() + "`, then `skillhub connect -g`."
}

// IsHubRoot reports whether dir holds a hub marker.
func IsHubRoot(dir string) bool {
	if dir == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, ".skillhub", "schema-version"))
	return err == nil && info.Mode().IsRegular()
}

// readClientEntry reads the skillhub entry of one host config file. A project
// file that is a link pointing outside its folder is ignored, so a cloned
// project cannot point detection at another file of this machine.
func readClientEntry(host hostintegration.Host, root, relative string, global bool) (*ClientEntry, bool) {
	path := filepath.Join(root, filepath.FromSlash(relative))
	info, err := os.Lstat(path)
	if err != nil {
		return nil, false
	}
	if info.Mode()&os.ModeSymlink != 0 && !global {
		target, err := filepath.EvalSymlinks(path)
		if err != nil {
			return nil, false
		}
		realRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			return nil, false
		}
		relativeTarget, err := filepath.Rel(realRoot, target)
		if err != nil || relativeTarget == ".." || strings.HasPrefix(relativeTarget, ".."+string(filepath.Separator)) || filepath.IsAbs(relativeTarget) {
			return nil, false
		}
	}
	raw, err := readSmallFile(path)
	if err != nil {
		return nil, false
	}
	registration := hostintegration.ReadRegistration(host, raw)
	if !registration.Present {
		return nil, false
	}
	entry := &ClientEntry{Host: host, Global: global, Root: root, Path: path, HubURL: registration.HubURL, TokenFile: registration.TokenFile}
	if workspace := strings.TrimSpace(registration.Workspace); workspace != "" && filepath.IsAbs(workspace) {
		entry.Workspace = filepath.Clean(workspace)
	}
	return entry, true
}

// sameDirectory compares two paths after cleaning and resolving links, case
// insensitively on Windows and macOS file systems that usually are.
func sameDirectory(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	a, b = filepath.Clean(a), filepath.Clean(b)
	if resolved, err := filepath.EvalSymlinks(a); err == nil {
		a = resolved
	}
	if resolved, err := filepath.EvalSymlinks(b); err == nil {
		b = resolved
	}
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" {
		return strings.EqualFold(a, b)
	}
	return a == b
}
