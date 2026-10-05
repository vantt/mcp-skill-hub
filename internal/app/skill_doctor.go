package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	resolverpkg "github.com/vantt/mcp-skill-hub/internal/resolver"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	"github.com/vantt/mcp-skill-hub/internal/telemetry"
)

// ErrDoctorSkillUnavailable means the skill is unknown, not active, or its
// files cannot be served, so there is no snapshot to check against.
var ErrDoctorSkillUnavailable = errors.New("skill is not an active, servable skill")

// DoctorNoRuntime is the note reported for a skill without a runtime block.
const DoctorNoRuntime = "no runtime requirements declared"

// doctorCheckOutputTail bounds how much setup check output is shown. The
// output is returned to the caller only and never stored.
const doctorCheckOutputTail = 2 << 10

// doctorFingerprint is the single source of the doctor cache key. The doctor
// writes and every reader (activation preflight, resolver setup annotation)
// looks up results with the distributed manifest version of the skill, so a
// change to any skill file or to the runtime block invalidates the result.
func doctorFingerprint(manifestVersion string, spec skillruntime.Spec) string {
	return skillruntime.RuntimeFingerprint(manifestVersion, spec)
}

// SkillDoctorService runs the full, executing machine check for one skill on
// explicit user request. It is never part of the MCP flow.
type SkillDoctorService struct {
	Telemetry TelemetrySink

	// Test seams; zero values use the host implementations.
	snapshots SnapshotService
	options   skillruntime.DoctorOptions
}

// SkillDoctorResult reports one doctor run. Checks never contain environment
// values; CheckOutput is the tail of the setup check output, shown to the
// caller once and never written to the cache.
type SkillDoctorResult struct {
	Result
	SkillID          string               `json:"skill_id"`
	State            string               `json:"state"`
	Note             string               `json:"note,omitempty"`
	ManifestVersion  string               `json:"manifest_version,omitempty"`
	WorkingDirectory string               `json:"working_directory,omitempty"`
	ReasonCodes      []string             `json:"reason_codes,omitempty"`
	Checks           []skillruntime.Check `json:"checks"`
	Basis            string               `json:"basis,omitempty"`
	CheckedAt        string               `json:"checked_at,omitempty"`
	SetupCommand     string               `json:"setup_command,omitempty"`
	CheckOutput      string               `json:"check_output,omitempty"`
	CachePath        string               `json:"cache_path,omitempty"`
}

// Run checks the skill's declared runtime requirements in the caller's
// terminal environment (basis "terminal"). The setup check command runs in the
// skill's state directory with SKILLHUB_SKILL_DIR and SKILLHUB_STATE_DIR set,
// and only when the skill's content is trusted. Results of trusted skills are
// cached under runtime/cache/doctor keyed by machine and runtime fingerprint so
// activation and resolution can report them as hints without executing
// anything.
func (service SkillDoctorService) Run(ctx context.Context, workspacePath, skillID string) (SkillDoctorResult, error) {
	startedAt := time.Now()
	if !snapshotSkillIDPattern.MatchString(skillID) {
		return SkillDoctorResult{}, NewInvalidRequestError(fmt.Sprintf("invalid skill id %q", skillID), "Pass a skill ID from `skillhub skill list`.")
	}
	local, details, err := service.snapshots.ensure(ctx, workspacePath, skillID)
	if err != nil {
		return SkillDoctorResult{}, err
	}
	if local.Status == LocalStatusUnavailable {
		return SkillDoctorResult{}, fmt.Errorf("%w: %s (%v)", ErrDoctorSkillUnavailable, skillID, local.ReasonCodes)
	}
	result := SkillDoctorResult{
		SkillID:         skillID,
		ManifestVersion: local.ManifestVersion,
		ReasonCodes:     append([]string(nil), local.ReasonCodes...),
		Checks:          []skillruntime.Check{},
	}
	if !details.HasSpec {
		result.State = skillruntime.StateReady
		result.Note = DoctorNoRuntime
		result.Result = NewResult(StatusReady, "Skill "+skillID+" declares no runtime requirements.")
		return result, nil
	}

	var workDir string
	extraEnv := map[string]string{}
	if local.Preflight != nil {
		workDir = local.Preflight.StateDirectory
		for key, value := range local.Preflight.Env {
			extraEnv[key] = value
		}
	}
	result.WorkingDirectory = workDir
	// Stored secrets count toward env presence and are added to the check's
	// environment. They stay inside this process: checks record presence only
	// and the check output is redacted before it is shown.
	stored, err := loadSkillConfigEnv(details.Root, skillID)
	if err != nil {
		return SkillDoctorResult{}, NewInvalidRequestError("the stored environment file for "+skillID+" is unusable: "+err.Error(), "Fix or remove it, or re-create entries with `skillhub skill env set "+skillID+" <KEY>`.")
	}
	for key, value := range stored {
		extraEnv[key] = value
	}
	options := service.options
	baseLookup := options.LookupEnv
	if baseLookup == nil {
		baseLookup = os.LookupEnv
	}
	options.LookupEnv = func(key string) (string, bool) {
		if _, ok := stored[key]; ok {
			return "", true
		}
		return baseLookup(key)
	}
	run := skillruntime.RunDoctor(ctx, details.Spec, workDir, extraEnv, details.Trusted, options)
	run.CheckOutput = redactValues(run.CheckOutput, stored)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return SkillDoctorResult{}, ctxErr
	}
	var cachePath string
	// A result for content that awaits review is not cached: its skipped check
	// would otherwise outlive the approval as a stale hint.
	if details.Trusted {
		fingerprint := doctorFingerprint(local.ManifestVersion, details.Spec)
		if err := skillruntime.WriteCache(details.Root, skillID, fingerprint, run); err != nil {
			service.recordDoctorTelemetry(ctx, details.Root, skillID, "failed", doctorReasonCodes(result.ReasonCodes, nil), startedAt)
			return SkillDoctorResult{}, fmt.Errorf("write doctor result: %w", err)
		}
		if cachePath, err = skillruntime.CachePath(details.Root, skillID, fingerprint); err != nil {
			return SkillDoctorResult{}, err
		}
	}
	service.recordDoctorTelemetry(ctx, details.Root, skillID, run.State, doctorReasonCodes(result.ReasonCodes, run.Checks), startedAt)

	result.State = run.State
	result.Basis = run.Basis
	result.Checks = append(result.Checks, run.Checks...)
	result.CheckedAt = run.CheckedAt.UTC().Format(time.RFC3339)
	result.CheckOutput = tailText(run.CheckOutput, doctorCheckOutputTail)
	result.CachePath = cachePath
	switch run.State {
	case skillruntime.StateReady:
		result.Result = NewResult(StatusReady, "Skill "+skillID+" is ready on this machine.")
	case skillruntime.StateUnsupportedPlatform:
		result.Result = NewResult(StatusActionRequired, "Skill "+skillID+" does not support this platform.")
	default:
		result.Result = NewResult(StatusActionRequired, "Skill "+skillID+" needs setup on this machine.")
		// Unreviewed third-party commands are never offered; the review
		// command is suggested instead.
		if details.Trusted && details.Spec.Setup.Command != "" {
			result.SetupCommand = details.Spec.Setup.Command
			result.SuggestedActions = append(result.SuggestedActions, Action{Label: "Run the declared setup in the working directory (review it first)", Command: details.Spec.Setup.Command, RequiresConfirmation: true})
		}
		for _, check := range run.Checks {
			if check.Kind == skillruntime.KindEnv && check.Status != skillruntime.StatusPass {
				result.SuggestedActions = append(result.SuggestedActions, Action{Label: "Store " + check.Name + " for this skill (the value is read without echo)", Command: "skillhub skill env set " + skillID + " " + check.Name})
			}
		}
		if !details.Trusted {
			result.SuggestedActions = append(result.SuggestedActions, Action{Label: "Review the skill's content before running anything from it", Command: "skillhub skill review " + skillID})
		}
	}
	return result, nil
}

// recordDoctorTelemetry records a content-free summary of one doctor run.
func (service SkillDoctorService) recordDoctorTelemetry(ctx context.Context, root, skillID, status string, codes []string, startedAt time.Time) {
	if service.Telemetry == nil {
		return
	}
	recordCurationTelemetry(ctx, service.Telemetry, root, curationTelemetryEvent(telemetry.EventSkillDoctorChecked, map[string]any{
		"skill_id":     skillID,
		"status":       status,
		"reason_codes": codes,
		"duration_ms":  time.Since(startedAt).Milliseconds(),
	}))
}

// doctorReasonCodes lists review reason codes and the kind and status of every
// check that did not pass, sorted and unique. Check names and details are
// omitted because names may identify private environment variables.
func doctorReasonCodes(reviewReasons []string, checks []skillruntime.Check) []string {
	seen := map[string]bool{}
	codes := []string{}
	add := func(code string) {
		if code != "" && !seen[code] {
			seen[code] = true
			codes = append(codes, code)
		}
	}
	for _, reason := range reviewReasons {
		add(reason)
	}
	for _, check := range checks {
		if check.Status != skillruntime.StatusPass {
			add(check.Kind + "_" + check.Status)
		}
	}
	sort.Strings(codes)
	return codes
}

// cachedDoctorResult reads the cached terminal doctor result for a skill
// if the skill is trusted and declares a runtime spec. An untrusted skill
// has no cached result by design.
func cachedDoctorResult(root string, entry DistributedSkill, contentJSON []byte) (*skillruntime.Result, error) {
	trust, err := evaluateSkillTrust(entry.SkillID, contentJSON, entry.digests)
	if err != nil {
		return nil, err
	}
	if !trust.Verdict.Trusted || !trust.HasSpec {
		return nil, nil
	}
	result, ok, readErr := skillruntime.ReadCache(root, entry.SkillID, doctorFingerprint(entry.Version, trust.Spec))
	if readErr != nil || !ok {
		return nil, readErr
	}
	return &result, nil
}

// setupAnnotation computes the post-ranking setup hint for a recommended
// skill from its manifest, the platform check, and the cached terminal doctor
// result. The hub verifies only trust and platform; a doctor-derived state is a
// hint labelled with its basis. It returns nil for trusted skills without a
// runtime block. Any failure to read the manifest or cache leaves the state
// unknown rather than hiding the skill.
func setupAnnotation(root string, entry DistributedSkill, contentJSON []byte, probe runtimeProbe) (*resolverpkg.SetupStatus, error) {
	trust, err := evaluateSkillTrust(entry.SkillID, contentJSON, entry.digests)
	if err != nil {
		return nil, err
	}
	if !trust.Verdict.Trusted {
		return &resolverpkg.SetupStatus{State: skillruntime.StateReviewRequired, ReasonCodes: append([]string(nil), trust.Verdict.ReasonCodes...)}, nil
	}
	if !trust.HasSpec {
		return nil, nil
	}
	platform := probe.platform(trust.Spec)
	var cached *skillruntime.Result
	// An unreadable doctor cache only means the state stays unknown.
	if result, readErr := cachedDoctorResult(root, entry, contentJSON); readErr == nil && result != nil {
		cached = result
	}
	annotation := &resolverpkg.SetupStatus{State: skillruntime.SetupState(false, true, platform, cached)}
	if annotation.State == skillruntime.StateUnsupportedPlatform {
		annotation.ReasonCodes = appendUnique(annotation.ReasonCodes, "platform_unsupported")
	}
	if cached != nil {
		annotation.Basis = skillruntime.BasisTerminal
		annotation.CheckedAt = cached.CheckedAt.UTC().Format(time.RFC3339)
		if cached.State == skillruntime.StateSetupRequired {
			annotation.ReasonCodes = appendUnique(annotation.ReasonCodes, "doctor_setup_required")
		}
	} else if annotation.State == skillruntime.StateUnknown {
		annotation.ReasonCodes = appendUnique(annotation.ReasonCodes, "doctor_not_run")
	}
	return annotation, nil
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

// tailText returns at most limit bytes from the end of text, starting at a
// UTF-8 boundary.
func tailText(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	start := len(text) - limit
	for start < len(text) && text[start]&0xC0 == 0x80 {
		start++
	}
	return text[start:]
}
