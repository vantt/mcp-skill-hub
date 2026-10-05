package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

// SkillRuntimeDoctor carries cached terminal doctor results.
type SkillRuntimeDoctor struct {
	State     string               `json:"state"`
	Basis     string               `json:"basis"`
	CheckedAt string               `json:"checked_at"`
	Checks    []skillruntime.Check `json:"checks"`
}

// SkillRuntimeStatus is the public read model for WebUI runtime parity.
type SkillRuntimeStatus struct {
	Result
	SkillID       string                   `json:"skill_id"`
	Runtime       *skillruntime.Spec       `json:"runtime"`
	Served        bool                     `json:"served"`
	Setup         *resolverpkg.SetupStatus `json:"setup,omitempty"`
	Doctor        *SkillRuntimeDoctor      `json:"doctor,omitempty"`
	DoctorCommand string                   `json:"doctor_command"`
	EnvKeys       []string                 `json:"env_keys"`
	EnvSetCommand string                   `json:"env_set_command"`
}

// RuntimeStatus derives the runtime and trust state for one skill without modifying runtime files.
func (SkillService) RuntimeStatus(ctx context.Context, path, id string) (SkillRuntimeStatus, error) {
	if !snapshotSkillIDPattern.MatchString(id) {
		return SkillRuntimeStatus{}, NewInvalidRequestError(fmt.Sprintf("invalid skill ID %q", id), "Skill ID must match "+snapshotSkillIDPattern.String())
	}

	root, err := workspace.Discover(path)
	if err != nil {
		return SkillRuntimeStatus{}, err
	}

	_, _, skillMetaBytes, err := locateSkillDir(root, id)
	if err != nil {
		return SkillRuntimeStatus{}, err
	}

	status := SkillRuntimeStatus{
		Result:        NewResult(StatusOK, fmt.Sprintf("Runtime status for skill %s.", id)),
		SkillID:       id,
		DoctorCommand: "skillhub skill doctor " + id,
		EnvSetCommand: "skillhub skill env set " + id + " <KEY>",
		EnvKeys:       []string{},
	}

	spec, hasSpec := reviewRuntimeSpec(skillMetaBytes)
	if hasSpec {
		status.Runtime = &spec
	}

	// Read env keys (names only, non-failing on unusable or absent file)
	envFile := filepath.Join(root, "runtime", "config", id, "env")
	if info, statErr := os.Lstat(envFile); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			status.Warnings = append(status.Warnings, Warning{
				Code:    "env_file_unusable",
				Summary: fmt.Sprintf("Fix or recreate it with skillhub skill env set %s <KEY>.", id),
			})
		} else {
			envRes, envErr := (SkillEnvService{}).List(ctx, root, id)
			if envErr != nil {
				status.Warnings = append(status.Warnings, Warning{
					Code:    "env_file_unusable",
					Summary: fmt.Sprintf("Fix or recreate it with skillhub skill env set %s <KEY>.", id),
				})
			} else {
				status.EnvKeys = envRes.Keys
			}
		}
	}

	// Served, Setup, and Doctor are only filled when active and servable
	handle, err := catalog.OpenCurrent(ctx, root)
	if err == nil {
		defer handle.Close()
		entry, distErr := buildDistributedSkill(ctx, root, handle, id)
		if distErr == nil {
			status.Served = true
			status.Setup = (ResolverService{}).setupStatus(ctx, root, handle, entry)

			var contentJSON string
			if queryErr := handle.DB.QueryRowContext(ctx, `SELECT content_json FROM canonical_entities WHERE id=?`, id).Scan(&contentJSON); queryErr == nil {
				if cached, _ := cachedDoctorResult(root, entry, []byte(contentJSON)); cached != nil {
					status.Doctor = &SkillRuntimeDoctor{
						State:     cached.State,
						Basis:     cached.Basis,
						CheckedAt: cached.CheckedAt.UTC().Format(time.RFC3339),
						Checks:    cached.Checks,
					}
				}
			}
		}
	}

	return status, nil
}
