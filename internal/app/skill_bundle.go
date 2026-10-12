package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	"github.com/vantt/mcp-skill-hub/internal/systemskills"
)

// SkillBundle is the deterministic archive of one approved, active skill
// version, addressed by the manifest digest that its skill URI pins. When
// NotModified is set the caller already holds this version and Archive is
// empty.
type SkillBundle struct {
	SkillID     string
	Digest      string // lowercase hex manifest digest
	NotModified bool
	Archive     []byte
}

// ErrContentReviewRequired reports a skill whose content a human has not
// approved; none of it may be served.
var ErrContentReviewRequired = errors.New("content_review_required")

// ContentReviewRequiredError carries the review reasons of a blocked skill.
type ContentReviewRequiredError struct {
	SkillID     string
	ReasonCodes []string
}

func (e *ContentReviewRequiredError) Error() string {
	return "content_review_required: skill " + e.SkillID + " awaits content review"
}

func (e *ContentReviewRequiredError) Unwrap() error { return ErrContentReviewRequired }

// SkillBundle builds the bundle of the current version of an active skill.
// It applies the same trust rule as the local snapshot: a third-party skill
// whose content is not approved returns ContentReviewRequiredError and no
// bytes. When the current digest is one of held, it returns NotModified
// without reading any skill file. The catalog generation is opened once and
// every file is read in the same pass.
func (DistributionService) SkillBundle(ctx context.Context, workspacePath, skillID string, held []string) (SkillBundle, error) {
	if skillID == systemskills.CuratorSkillID || !snapshotSkillIDPattern.MatchString(skillID) {
		return SkillBundle{}, skill.ErrNotFound
	}
	var trust skillTrust
	var digest string
	notModified := false
	_, source, err := loadDistributedSkillSource(ctx, workspacePath, skillID, func(entry DistributedSkill, contentJSON []byte) (bool, error) {
		var evalErr error
		trust, evalErr = evaluateSkillTrust(skillID, contentJSON, entry.digests)
		if evalErr != nil {
			return false, evalErr
		}
		digest = strings.TrimPrefix(entry.Version, "sha256:")
		notModified = slices.Contains(held, digest)
		return trust.Verdict.Trusted && !notModified, nil
	})
	if err != nil {
		return SkillBundle{}, err
	}
	if !trust.Verdict.Trusted {
		return SkillBundle{}, &ContentReviewRequiredError{SkillID: skillID, ReasonCodes: append([]string(nil), trust.Verdict.ReasonCodes...)}
	}
	if notModified {
		return SkillBundle{SkillID: skillID, Digest: digest, NotModified: true}, nil
	}
	detail, err := (SkillService{}).ReadSkillCore(ctx, workspacePath, skillID)
	if err != nil {
		return SkillBundle{}, fmt.Errorf("read skill %s detail: %w", skillID, err)
	}
	// SKILL.md travels once, as a file of the bundle.
	detail.Content = ""
	encodedDetail, err := json.Marshal(detail)
	if err != nil {
		return SkillBundle{}, fmt.Errorf("encode skill %s detail: %w", skillID, err)
	}
	metadata := skillruntime.BundleMetadata{
		Schema:          skillruntime.BundleSchema,
		SkillID:         skillID,
		ManifestVersion: source.Skill.Version,
		DepsKey:         trust.DepsKey,
		Detail:          encodedDetail,
		Files:           make([]skillruntime.BundleFile, 0, len(source.Files)),
	}
	if trust.HasSpec {
		spec := trust.Spec
		metadata.Runtime = &spec
	}
	contents := make(map[string][]byte, len(source.Files))
	for _, file := range source.Files {
		if file.Path == skillruntime.BundleMetadataName || file.Path == snapshotMarkerName {
			return SkillBundle{}, fmt.Errorf("skill %s uses the reserved file name %s", skillID, file.Path)
		}
		if err := validateSnapshotRelativePath(file.Path); err != nil {
			return SkillBundle{}, err
		}
		metadata.Files = append(metadata.Files, skillruntime.BundleFile{Path: file.Path, Digest: file.Digest, Size: file.Size, Kind: file.Kind})
		contents[file.Path] = file.Bytes
	}
	computed, err := skillruntime.ManifestDigest(metadata.Files)
	if err != nil {
		return SkillBundle{}, err
	}
	if computed != digest {
		return SkillBundle{}, fmt.Errorf("%w: skill %s files hash to %s, not the pinned %s", skill.ErrResourceDigestMismatch, skillID, computed, digest)
	}
	var archive bytes.Buffer
	if err := skillruntime.WriteBundle(&archive, metadata, contents); err != nil {
		return SkillBundle{}, err
	}
	return SkillBundle{SkillID: skillID, Digest: digest, Archive: archive.Bytes()}, nil
}

// RemoteStatus reports the local status of an active skill for a caller on
// another machine, without exporting a snapshot and without any path on this
// machine: ready, review_required with its reasons, or unavailable.
func (SnapshotService) RemoteStatus(ctx context.Context, workspacePath, skillID string) (LocalSkill, error) {
	if skillID == systemskills.CuratorSkillID || !snapshotSkillIDPattern.MatchString(skillID) {
		return unavailableLocalSkill(LocalReasonNotServable), nil
	}
	var trust skillTrust
	_, source, err := loadDistributedSkillManifest(ctx, workspacePath, skillID, func(entry DistributedSkill, contentJSON []byte) (bool, error) {
		var evalErr error
		trust, evalErr = evaluateSkillTrust(skillID, contentJSON, entry.digests)
		return false, evalErr
	})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return LocalSkill{}, ctxErr
		}
		switch {
		case errors.Is(err, ErrResourceContentUnavailable), errors.Is(err, skill.ErrResourceContentUnavailable), errors.Is(err, skill.ErrResourceDigestMismatch):
			return unavailableLocalSkill(LocalReasonResourceContentUnavailable), nil
		case errors.Is(err, skill.ErrNotFound), errors.Is(err, catalog.ErrSkillNotServable), errors.Is(err, skill.ErrSnapshotExpired):
			return unavailableLocalSkill(LocalReasonNotServable), nil
		}
		return LocalSkill{}, err
	}
	if !trust.Verdict.Trusted {
		return LocalSkill{
			ManifestVersion: source.Skill.Version,
			Status:          LocalStatusReviewRequired,
			ReasonCodes:     append([]string(nil), trust.Verdict.ReasonCodes...),
			ReviewCommand:   "skillhub skill review " + skillID,
		}, nil
	}
	return LocalSkill{ManifestVersion: source.Skill.Version, Status: LocalStatusReady}, nil
}

// ClientLocalSkill describes a skill version extracted from a bundle on a
// client machine, in the same shape skill_get returns for a local hub: the
// read-only skill folder, a writable state directory seeded with the
// dependency manifests, the per-skill config directory, the variables to
// export, and the preflight plan with the platform checked on this machine.
// stateRoot holds the state and config directories (runtime/envs and
// runtime/config below it); nothing is ever executed.
func ClientLocalSkill(stateRoot, skillDirectory string, metadata skillruntime.BundleMetadata) (LocalSkill, error) {
	skillID := metadata.SkillID
	if !snapshotSkillIDPattern.MatchString(skillID) {
		return LocalSkill{}, fmt.Errorf("invalid skill id %q", skillID)
	}
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		return LocalSkill{}, fmt.Errorf("prepare the state directory: %w", err)
	}
	local := LocalSkill{
		Path:            skillDirectory,
		ManifestVersion: metadata.ManifestVersion,
		Status:          LocalStatusReady,
		Resources:       make([]LocalResource, 0, len(metadata.Files)),
	}
	var dependencyFiles []distributedSkillFile
	for _, file := range metadata.Files {
		if err := validateSnapshotRelativePath(file.Path); err != nil {
			return LocalSkill{}, err
		}
		localPath := filepath.Join(skillDirectory, filepath.FromSlash(file.Path))
		local.Resources = append(local.Resources, LocalResource{Path: file.Path, LocalPath: localPath, Kind: file.Kind})
		if skillruntime.IsDependencyManifest(file.Path) {
			contents, err := os.ReadFile(localPath)
			if err != nil {
				return LocalSkill{}, fmt.Errorf("read dependency manifest %s: %w", file.Path, err)
			}
			dependencyFiles = append(dependencyFiles, distributedSkillFile{Path: file.Path, Kind: file.Kind, Digest: file.Digest, Size: file.Size, Bytes: contents, Loaded: true})
		}
	}
	stateDirectory, err := ensureStateDir(stateRoot, skillID, metadata.DepsKey, dependencyFiles)
	if err != nil {
		return LocalSkill{}, err
	}
	configDirectory, err := ensureSnapshotDirectory(stateRoot, skillruntime.ConfigDirRoot+"/"+skillID, true)
	if err != nil {
		return LocalSkill{}, fmt.Errorf("prepare config directory: %w", err)
	}
	local.StateDirectory = stateDirectory
	local.Env = skillExportEnv(skillDirectory, stateDirectory, configDirectory)
	if metadata.Runtime != nil {
		local.Preflight = SnapshotService{}.preflight(stateRoot, skillID, stateDirectory, local.Env, metadata.ManifestVersion, *metadata.Runtime)
	}
	return local, nil
}
