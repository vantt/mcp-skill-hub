// Package hostintegration inspects and remediates supported agent-host integrations.
// It deliberately separates read-only inspection/planning from confirmed mutation.
package hostintegration

import (
	"context"
	"errors"
	"fmt"
)

// Host identifies a supported stock agent host.
type Host string

const (
	HostClaude Host = "claude-code"
	HostCodex  Host = "codex-cli"
	HostGemini Host = "gemini-cli"
)

// IntegrationLevel describes the strongest coordination a host adapter can install.
type IntegrationLevel string

const (
	// LevelNativeSkillBestEffort installs a native bundled skill, while activation
	// coordination remains an instruction-only, best-effort contract.
	LevelNativeSkillBestEffort IntegrationLevel = "native-skill-instruction-coordination"
	// LevelInstructionOnly is reserved for adapters without a native skill path.
	LevelInstructionOnly IntegrationLevel = "instruction-only"
)

// ChangeKind names what a change writes. Plans order changes by changeRank:
// MCP, permissions, shared skill files, native skill files, shared skill
// removals, then bootstrap instructions.
type ChangeKind string

const (
	ChangeMCP ChangeKind = "mcp-registration"
	// ChangeHostPermissions is a separate host settings file that allows the
	// runtime directories. Hosts whose MCP config file also carries the
	// allowance (Codex, Gemini CLI) fold it into ChangeMCP instead.
	ChangeHostPermissions ChangeKind = "host-permissions"
	// ChangeNativeSkill is a skill file in a host's own skills folder: the
	// Claude Code wrapper, or an old full copy that is removed.
	ChangeNativeSkill ChangeKind = "native-skill"
	// ChangeSharedSkill is a file of a system skill in the shared Agent Skills
	// folder <root>/.agents/skills/<id>/. It belongs to no host: Codex and
	// Gemini CLI read it directly, and the Claude Code wrapper points at it.
	ChangeSharedSkill ChangeKind = "shared-skill"
	ChangeBootstrap   ChangeKind = "bootstrap-instructions"
)

// SharedSkillsDirRelativePath is the shared Agent Skills folder, relative to
// the scope root, that holds the real files of every system skill.
const SharedSkillsDirRelativePath = ".agents/skills"

// Scope selects where a host integration is written relative to Request.Root.
type Scope string

const (
	// ScopeProject writes into a project directory (Root is that directory).
	ScopeProject Scope = "project"
	// ScopeUser writes user-level host files (Root is the user's home directory).
	ScopeUser Scope = "user"
)

// Adapter describes the stable filesystem contract for a supported host.
// The User* paths are relative to the home directory. Sources for the user
// scope layout (checked against the vendors' current documentation):
//   - Claude Code: user MCP servers live in ~/.claude.json under mcpServers
//     (code.claude.com/docs/en/mcp); user skills in ~/.claude/skills and user
//     instructions in ~/.claude/CLAUDE.md.
//   - Codex: user config ~/.codex/config.toml, global instructions
//     ~/.codex/AGENTS.md, user skills $HOME/.agents/skills
//     (developers.openai.com/codex).
//   - Gemini CLI: ~/.gemini/settings.json and ~/.gemini/GEMINI.md; skills in
//     .agents/skills from v0.28.0 (geminicli.com/docs/cli/creating-skills).
//
// Skill folders are the same relative path in both scopes.
type Adapter struct {
	Host                        Host
	Level                       IntegrationLevel
	ConfigRelativePath          string
	InstructionFileName         string
	UserConfigRelativePath      string
	UserInstructionRelativePath string
	NativeSkill                 bool
	// SkillsDirRelativePath is the folder the host reads skills from.
	SkillsDirRelativePath string
	// Wrapper marks a host that does not read the shared Agent Skills folder
	// and gets a generated wrapper SKILL.md pointing at it.
	Wrapper bool
	// LegacySkillsDirRelativePath is a skills folder that older releases
	// wrote a full curator copy to; it is only ever cleaned up.
	LegacySkillsDirRelativePath string
	// PermissionsRelativePath and UserPermissionsRelativePath name a settings
	// file separate from the MCP config; empty when the config file is shared.
	PermissionsRelativePath string
	// SharedPermissionsRelativePath is a project file that also satisfies the
	// check when it already holds the entries; it is never written.
	SharedPermissionsRelativePath string
	UserPermissionsRelativePath   string
}

// permissionsPath returns the separate permissions settings path for a scope.
func (a Adapter) permissionsPath(scope Scope) string {
	if scope == ScopeUser {
		return a.UserPermissionsRelativePath
	}
	return a.PermissionsRelativePath
}

// relativePaths returns the config and instruction paths for a scope.
func (a Adapter) relativePaths(scope Scope) (config, instructions string) {
	if scope == ScopeUser {
		return a.UserConfigRelativePath, a.UserInstructionRelativePath
	}
	return a.ConfigRelativePath, a.InstructionFileName
}

// Request defines an integration target. Workspace, Root, and Binary must
// identify absolute paths. Workspace is what the MCP server serves; Root is the
// directory that receives host files (a project directory, or the home
// directory for ScopeUser) and defaults to Workspace. Hosts may be empty to
// select every supported host.
//
// Hub and Remove select the other two kinds of plan. With Hub set the scope
// is pointed at a hub on another machine: the host starts the stdio proxy,
// Workspace stays empty and Root must be set. With Remove set every Skill Hub
// surface in the scope is removed (the entry, its allowed directories and
// permission rules, the instruction block and an unchanged curator skill);
// Workspace stays empty and Root must be set.
type Request struct {
	Workspace string
	Root      string
	Scope     Scope
	Binary    string
	Hosts     []Host
	Hub       *RemoteHub
	Remove    bool
	// KeepInstructions leaves the instruction files (CLAUDE.md, AGENTS.md,
	// GEMINI.md) alone on a removal. A hub commits them as its own content,
	// so removing the connection must not delete or edit them.
	KeepInstructions bool
}

// RemoteHub is a hub on another machine. URL is its base URL without a path;
// TokenFile, when set, is the absolute path of the local copy of the hub's
// token file. Only that path is written into host config files, never the
// token itself.
type RemoteHub struct {
	URL       string
	TokenFile string
}

// FileState is a read-only view of one managed integration surface.
//
// Conflict stops the whole plan. Blocked is set only for system skill files:
// Skill Hub leaves that one file unchanged and plans every other change.
type FileState struct {
	// Host is set on the entries of PlanResult.Blocked; it is empty for a
	// shared skill file.
	Host           Host
	Kind           ChangeKind
	Path           string
	Exists         bool
	Current        bool
	PreimageDigest string
	Conflict       string
	Blocked        string
}

// HostInspection reports the capability and current state for one host.
type HostInspection struct {
	Host  Host
	Level IntegrationLevel
	Files []FileState
}

// Inspection is safe to produce without user confirmation and performs no writes.
// Shared holds the files of the shared Agent Skills folder, which belong to no host.
type Inspection struct {
	Workspace string
	Root      string
	Binary    string
	Hosts     []HostInspection
	Shared    []FileState
}

// Change is an immutable, preimage-pinned write proposal. Desired content is
// retained for compatibility; Preview is the bounded, sanitized description
// applications should render before confirmation.
type Change struct {
	Host           Host
	Kind           ChangeKind
	Path           string
	PreimageDigest string
	Desired        []byte
	Mode           uint32
	Preview        string
	// Remove deletes the file instead of writing Desired: what is left holds
	// no content other than what Skill Hub wrote.
	Remove bool
}

// PlanResult is dependency ordered and can be passed directly to Apply.
// Changes of the shared kind carry an empty Host. Blocked lists the system
// skill files the plan leaves unchanged, each with its reason in Blocked;
// Notices are sentences a caller shows next to the result.
type PlanResult struct {
	Workspace string
	Root      string
	Scope     Scope
	Binary    string
	Changes   []Change
	Hub       *RemoteHub
	Remove    bool
	Blocked   []FileState
	Notices   []Notice
}

// Notice is a sentence about a plan that is not a change, for example what
// an agent still sees after a change of another agent.
type Notice struct {
	Code    string
	Summary string
}

// ApplyOptions makes mutation authorization explicit at the application boundary.
type ApplyOptions struct {
	Confirmed bool
}

// ApplyResult reports actual writes. Already-current changes are not counted.
type ApplyResult struct {
	Changed []Change
}

var (
	ErrConfirmationRequired = errors.New("host integration application requires explicit confirmation")
	ErrStalePlan            = errors.New("host integration plan preimage no longer matches")
	ErrConflict             = errors.New("host integration conflict")
	ErrSymlink              = errors.New("host integration path contains a symbolic link")
)

// Inspect performs a read-only inspection.
func Inspect(ctx context.Context, request Request) (Inspection, error) {
	return inspect(ctx, request)
}

// Plan builds a read-only, dependency-ordered remediation plan.
func Plan(ctx context.Context, request Request) (PlanResult, error) {
	return buildPlan(ctx, request)
}

// Apply atomically applies a preimage-pinned plan after explicit confirmation.
func Apply(ctx context.Context, plan PlanResult, options ApplyOptions) (ApplyResult, error) {
	return apply(ctx, plan, options)
}

// ConflictError associates a conflict with its managed path.
type ConflictError struct {
	Path   string
	Reason string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%v at %s: %s", ErrConflict, e.Path, e.Reason)
}

func (e *ConflictError) Unwrap() error { return ErrConflict }

// Finding describes a host integration issue reported to doctor or status.
type Finding struct {
	ID      string `json:"id"`
	Summary string `json:"summary"`
	Note    string `json:"note,omitempty"`
	Fixable bool   `json:"fixable"`
}

const (
	FindingHostIntegrationMissing = "host_integration_missing"
	FindingHostIntegrationBlocked = "host_integration_blocked"
	SummaryHostIntegrationMissing = "host integration missing"
	NoteRunDoctorFix              = "run skillhub doctor --fix after a pull that untracked those files"
)

// MissingFinding returns a fixable Finding if the plan contains missing host
// files, else a Finding naming the first system skill file the plan leaves
// unchanged, else nil.
func MissingFinding(plan PlanResult) *Finding {
	for _, change := range plan.Changes {
		if change.PreimageDigest == missingDigest {
			return &Finding{
				ID:      FindingHostIntegrationMissing,
				Summary: SummaryHostIntegrationMissing,
				Note:    NoteRunDoctorFix,
				Fixable: true,
			}
		}
	}
	if len(plan.Blocked) > 0 {
		summary := plan.Blocked[0].Path + " was not written by Skill Hub; review it or delete it, then run skillhub doctor --fix"
		if more := len(plan.Blocked) - 1; more > 0 {
			summary += fmt.Sprintf(" (and %d more)", more)
		}
		return &Finding{ID: FindingHostIntegrationBlocked, Summary: summary}
	}
	return nil
}

// CheckMissing inspects the workspace and returns a Finding if host integration files are missing.
func CheckMissing(ctx context.Context, request Request) (*Finding, error) {
	plan, err := Plan(ctx, request)
	if err != nil {
		return nil, err
	}
	return MissingFinding(plan), nil
}
