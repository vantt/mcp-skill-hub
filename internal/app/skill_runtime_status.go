package app

import (
	"context"
	"fmt"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
)

type SkillRuntimeStatus struct {
	Result
	SkillID       string                   `json:"skill_id"`
	Runtime       *skillruntime.Spec       `json:"runtime"`          // canonical block; null when absent
	Served        bool                     `json:"served"`           // active and servable in the current catalog
	Setup         *resolverpkg.SetupStatus `json:"setup,omitempty"`  // exactly what skill_resolve attaches for this skill
	Doctor        *SkillRuntimeDoctor      `json:"doctor,omitempty"` // cached terminal result, if any
	DoctorCommand string                   `json:"doctor_command"`   // "skillhub skill doctor <id>"
	EnvKeys       []string                 `json:"env_keys"`         // names only, sorted, never values
	EnvSetCommand string                   `json:"env_set_command"`  // "skillhub skill env set <id> <KEY>"
}

type SkillRuntimeDoctor struct {
	State     string               `json:"state"`
	Basis     string               `json:"basis"`      // "terminal"
	CheckedAt string               `json:"checked_at"` // RFC 3339 UTC
	Checks    []skillruntime.Check `json:"checks"`
}

// RuntimeStatus returns the static runtime status and cached verification state
// of a skill without mutating any workspace files, exporting snapshots, or
// running executable checks.
func (SkillService) RuntimeStatus(ctx context.Context, path, id string) (SkillRuntimeStatus, error) {
	if !snapshotSkillIDPattern.MatchString(id) {
		return SkillRuntimeStatus{}, NewInvalidRequestError(
			"invalid skill ID",
			"Choose a skill ID containing only lowercase alphanumeric characters and hyphens.",
		)
	}

	root, err := skill.ResolveWorkspace(path)
	if err != nil {
		return SkillRuntimeStatus{}, err
	}

	_, _, skillMetaBytes, err := locateSkillDir(root, id)
	if err != nil {
		return SkillRuntimeStatus{}, err
	}

	spec, hasSpec := reviewRuntimeSpec(skillMetaBytes)
	var runtimeSpec *skillruntime.Spec
	if hasSpec {
		runtimeSpec = &spec
	}

	doctorCommand := fmt.Sprintf("skillhub skill doctor %s", id)
	envSetCommand := fmt.Sprintf("skillhub skill env set %s <KEY>", id)

	var warnings []Warning
	keys := []string{}
	envResult, envErr := (SkillEnvService{}).List(ctx, root, id)
	if envErr != nil {
		warnings = append(warnings, Warning{
			Code:    "env_file_unusable",
			Summary: fmt.Sprintf("Fix or recreate it with skillhub skill env set %s <KEY>.", id),
		})
	} else if envResult.Keys != nil {
		keys = envResult.Keys
	}

	var served bool
	var setup *resolverpkg.SetupStatus
	var doctor *SkillRuntimeDoctor

	handle, err := catalog.OpenCurrentLocked(ctx, root)
	if err == nil {
		defer handle.Close()
		entry, distErr := buildDistributedSkill(ctx, root, handle, id)
		if distErr == nil {
			served = true
			setup = (ResolverService{}).setupStatus(ctx, root, handle, entry)
			var contentJSON string
			if scanErr := handle.DB.QueryRowContext(ctx, `SELECT content_json FROM canonical_entities WHERE id=?`, id).Scan(&contentJSON); scanErr == nil {
				if cached, readErr := cachedDoctorResult(root, entry, []byte(contentJSON)); readErr == nil && cached != nil {
					checks := cached.Checks
					if checks == nil {
						checks = []skillruntime.Check{}
					}
					doctor = &SkillRuntimeDoctor{
						State:     string(cached.State),
						Basis:     string(cached.Basis),
						CheckedAt: cached.CheckedAt.UTC().Format(time.RFC3339),
						Checks:    checks,
					}
				}
			}
		}
	}

	statusResult := NewResult(StatusOK, fmt.Sprintf("Runtime status for skill %s.", id))
	statusResult.Warnings = warnings

	return SkillRuntimeStatus{
		Result:        statusResult,
		SkillID:       id,
		Runtime:       runtimeSpec,
		Served:        served,
		Setup:         setup,
		Doctor:        doctor,
		DoctorCommand: doctorCommand,
		EnvKeys:       keys,
		EnvSetCommand: envSetCommand,
	}, nil
}
