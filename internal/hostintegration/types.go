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

// ChangeKind is ordered by dependency: MCP, native skill, then bootstrap instructions.
type ChangeKind string

const (
	ChangeMCP         ChangeKind = "mcp-registration"
	ChangeNativeSkill ChangeKind = "native-skill"
	ChangeBootstrap   ChangeKind = "bootstrap-instructions"
)

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
//   - Gemini CLI: ~/.gemini/settings.json, ~/.gemini/GEMINI.md and
//     ~/.gemini/skills (geminicli.com/docs/cli/skills).
type Adapter struct {
	Host                        Host
	Level                       IntegrationLevel
	ConfigRelativePath          string
	SkillRelativePath           string
	InstructionFileName         string
	UserConfigRelativePath      string
	UserSkillRelativePath       string
	UserInstructionRelativePath string
	NativeSkill                 bool
}

// relativePaths returns the config, skill, and instruction paths for a scope.
func (a Adapter) relativePaths(scope Scope) (config, skill, instructions string) {
	if scope == ScopeUser {
		return a.UserConfigRelativePath, a.UserSkillRelativePath, a.UserInstructionRelativePath
	}
	return a.ConfigRelativePath, a.SkillRelativePath, a.InstructionFileName
}

// Request defines an integration target. Workspace, Root, and Binary must
// identify absolute paths. Workspace is what the MCP server serves; Root is the
// directory that receives host files (a project directory, or the home
// directory for ScopeUser) and defaults to Workspace. Hosts may be empty to
// select every supported host.
type Request struct {
	Workspace string
	Root      string
	Scope     Scope
	Binary    string
	Hosts     []Host
}

// FileState is a read-only view of one managed integration surface.
type FileState struct {
	Kind           ChangeKind
	Path           string
	Exists         bool
	Current        bool
	PreimageDigest string
	Conflict       string
}

// HostInspection reports the capability and current state for one host.
type HostInspection struct {
	Host  Host
	Level IntegrationLevel
	Files []FileState
}

// Inspection is safe to produce without user confirmation and performs no writes.
type Inspection struct {
	Workspace string
	Root      string
	Binary    string
	Hosts     []HostInspection
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
}

// PlanResult is dependency ordered and can be passed directly to Apply.
type PlanResult struct {
	Workspace string
	Root      string
	Scope     Scope
	Binary    string
	Changes   []Change
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
