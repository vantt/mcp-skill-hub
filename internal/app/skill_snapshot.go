package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/catalog"
	"github.com/vantt/mcp-skill-hub/internal/skill"
	"github.com/vantt/mcp-skill-hub/internal/skillruntime"
	"github.com/vantt/mcp-skill-hub/internal/systemskills"
	"github.com/vantt/mcp-skill-hub/internal/workspace"
)

// Local snapshot statuses.
const (
	LocalStatusReady          = "ready"
	LocalStatusReviewRequired = "review_required"
	LocalStatusUnavailable    = "unavailable"
)

// Reason codes for an unavailable local snapshot. A skill that awaits content
// review uses the skillruntime review reason codes.
const (
	LocalReasonNotServable                = "not_servable"
	LocalReasonResourceContentUnavailable = "resource_content_unavailable"
	LocalReasonExportFailed               = "export_failed"
)

// SnapshotGCMinAge is how long a snapshot that no longer matches an active
// manifest is kept before garbage collection may remove it.
const SnapshotGCMinAge = 24 * time.Hour

// StateDirGCMinAge is how long a state directory that no active trusted skill
// references is kept untouched before garbage collection may remove it.
const StateDirGCMinAge = 7 * 24 * time.Hour

const (
	snapshotRootRelative   = "runtime/cache/skills"
	snapshotStagingName    = ".staging"
	snapshotMarkerName     = ".skillhub-snapshot.json"
	snapshotMarkerSchema   = 1
	stateStagingPrefix     = ".staging-"
	snapshotStagingMaxAge  = time.Hour
	snapshotGCInterval     = time.Hour
	snapshotMaxMarkerBytes = 1 << 20
	snapshotPlaceAttempts  = 3
)

var snapshotSkillIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// LocalSkill describes a read-only, digest-verified copy of an active skill
// folder exported for the host to use. It never points at the live workspace.
// A third-party skill whose content a human has not approved has status
// review_required and no path, resources, state directory, or preflight.
// Every trusted skill gets a writable StateDirectory and the Env variables to
// export when running its scripts or installing its dependencies, whether or
// not it declares a runtime block.
type LocalSkill struct {
	Path            string            `json:"path,omitempty"`
	StateDirectory  string            `json:"state_directory,omitempty"`
	Env             map[string]string `json:"env,omitempty"`
	ManifestVersion string            `json:"manifest_version,omitempty"`
	Status          string            `json:"status"`
	ReasonCodes     []string          `json:"reason_codes,omitempty"`
	ReviewCommand   string            `json:"review_command,omitempty"`
	Resources       []LocalResource   `json:"resources,omitempty"`
	Preflight       *LocalPreflight   `json:"preflight,omitempty"`
}

// LocalResource is one file present in the local snapshot.
type LocalResource struct {
	Path      string `json:"path"`
	LocalPath string `json:"local_path"`
	Kind      string `json:"kind"`
}

// LocalPreflight is the non-executing runtime plan for a skill with a runtime
// block. The hub never runs these commands in the MCP flow. WorkingDirectory
// is the writable state directory; Env lists the variables the host exports
// when it runs check, setup, or the skill's scripts. LiveChecks holds only
// the platform check: binaries and environment are verified by the agent's own
// `check` in its shell.
type LocalPreflight struct {
	State            string               `json:"state"`
	WorkingDirectory string               `json:"working_directory"`
	StateDirectory   string               `json:"state_directory"`
	Env              map[string]string    `json:"env"`
	Requires         LocalRequires        `json:"requires"`
	Check            string               `json:"check,omitempty"`
	Setup            string               `json:"setup,omitempty"`
	LiveChecks       []skillruntime.Check `json:"live_checks"`
	Doctor           *LocalDoctor         `json:"doctor,omitempty"`
	Instruction      string               `json:"instruction"`
}

// LocalRequires lists the declared host prerequisites.
type LocalRequires struct {
	Bins      []skillruntime.Bin `json:"bins,omitempty"`
	Env       []string           `json:"env,omitempty"`
	Platforms []string           `json:"platforms,omitempty"`
}

// LocalDoctor summarizes the cached result of `skillhub skill doctor`. Basis
// says whose environment produced it: the terminal that ran the doctor.
type LocalDoctor struct {
	State     string `json:"state"`
	Basis     string `json:"basis"`
	CheckedAt string `json:"checked_at"`
}

// SnapshotService exports trusted active skills into digest-keyed, read-only
// directories under runtime/cache/skills, provides writable state directories
// under runtime/envs, and reclaims outdated ones.
type SnapshotService struct {
	// Test seam; an empty value uses the host platform.
	goos string
	// memo lets a long-lived process skip re-hashing a snapshot it already
	// verified. A nil memo verifies in full on every call.
	memo *snapshotMemo
}

// NewSnapshotService returns a service that remembers verified snapshots for
// the life of the process. It is safe for concurrent use.
func NewSnapshotService() SnapshotService {
	return SnapshotService{memo: &snapshotMemo{entries: map[string]snapshotMemoEntry{}}}
}

// snapshotFileStat is the cheap identity of a snapshot file used to decide
// whether a previously verified file may have changed.
type snapshotFileStat struct {
	size    int64
	modTime time.Time
	mode    fs.FileMode
}

func newSnapshotFileStat(info fs.FileInfo) snapshotFileStat {
	return snapshotFileStat{size: info.Size(), modTime: info.ModTime(), mode: info.Mode()}
}

func (stat snapshotFileStat) equal(other snapshotFileStat) bool {
	return stat.size == other.size && stat.modTime.Equal(other.modTime) && stat.mode == other.mode
}

// snapshotMemoEntry records the stat of the marker and every listed file at
// the moment the snapshot directory was fully verified.
type snapshotMemoEntry struct {
	marker snapshotFileStat
	files  map[string]snapshotFileStat
}

// snapshotMemo remembers fully verified snapshot directories, keyed by the
// directory and the exact marker bytes. A hit still stats every listed file
// and falls back to a full re-hash when any stat differs; touching the
// directory for garbage collection changes neither the marker nor the files.
type snapshotMemo struct {
	mu      sync.Mutex
	entries map[string]snapshotMemoEntry
}

func snapshotMemoKey(directory string, expectedMarker []byte) string {
	return directory + "\x00" + string(expectedMarker)
}

// verify reports nil when directory is a valid snapshot for marker. A nil
// memo always performs the full verification.
func (memo *snapshotMemo) verify(directory string, expectedMarker []byte, marker snapshotMarker) error {
	if memo == nil {
		return verifySnapshot(directory, expectedMarker, marker)
	}
	key := snapshotMemoKey(directory, expectedMarker)
	memo.mu.Lock()
	entry, known := memo.entries[key]
	memo.mu.Unlock()
	if known && statsUnchanged(directory, marker, entry) {
		return nil
	}
	if err := verifySnapshot(directory, expectedMarker, marker); err != nil {
		memo.mu.Lock()
		delete(memo.entries, key)
		memo.mu.Unlock()
		return err
	}
	recorded, err := collectSnapshotStats(directory, marker)
	memo.mu.Lock()
	defer memo.mu.Unlock()
	if err != nil {
		// Without stats the next call simply verifies in full again.
		delete(memo.entries, key)
		return nil
	}
	memo.entries[key] = recorded
	return nil
}

// collectSnapshotStats stats the marker and listed files of a verified snapshot.
func collectSnapshotStats(directory string, marker snapshotMarker) (snapshotMemoEntry, error) {
	handle, err := os.OpenRoot(directory)
	if err != nil {
		return snapshotMemoEntry{}, err
	}
	defer handle.Close()
	markerInfo, err := lstatSnapshotRegular(handle, snapshotMarkerName)
	if err != nil {
		return snapshotMemoEntry{}, err
	}
	entry := snapshotMemoEntry{marker: newSnapshotFileStat(markerInfo), files: make(map[string]snapshotFileStat, len(marker.Files))}
	for _, file := range marker.Files {
		info, err := lstatSnapshotRegular(handle, filepath.FromSlash(file.Path))
		if err != nil {
			return snapshotMemoEntry{}, err
		}
		entry.files[file.Path] = newSnapshotFileStat(info)
	}
	return entry, nil
}

// statsUnchanged is the cheap check on a memo hit: the directory is still a
// real directory, and the marker and every listed file have the recorded stat
// without symlinked components.
func statsUnchanged(directory string, marker snapshotMarker, entry snapshotMemoEntry) bool {
	info, err := os.Lstat(directory)
	if err != nil || info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return false
	}
	handle, err := os.OpenRoot(directory)
	if err != nil {
		return false
	}
	defer handle.Close()
	markerInfo, err := lstatSnapshotRegular(handle, snapshotMarkerName)
	if err != nil || !entry.marker.equal(newSnapshotFileStat(markerInfo)) {
		return false
	}
	if len(entry.files) != len(marker.Files) {
		return false
	}
	for _, file := range marker.Files {
		recorded, ok := entry.files[file.Path]
		if !ok {
			return false
		}
		fileInfo, err := lstatSnapshotRegular(handle, filepath.FromSlash(file.Path))
		if err != nil || !recorded.equal(newSnapshotFileStat(fileInfo)) {
			return false
		}
	}
	return true
}

type snapshotMarker struct {
	Schema          int                  `json:"schema"`
	SkillID         string               `json:"skill_id"`
	ManifestVersion string               `json:"manifest_version"`
	Files           []snapshotMarkerFile `json:"files"`
}

type snapshotMarkerFile struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

// contentTrustDocument is the subset of a skill manifest that decides trust.
type contentTrustDocument struct {
	Provenance struct {
		SourceID string `json:"source_id" yaml:"source_id"`
		Origin   struct {
			Kind string `json:"kind" yaml:"kind"`
		} `json:"origin" yaml:"origin"`
	} `json:"provenance" yaml:"provenance"`
	Quality struct {
		ContentReviewedDigest string `json:"content_reviewed_digest" yaml:"content_reviewed_digest"`
	} `json:"quality" yaml:"quality"`
}

func (document contentTrustDocument) provenance() skillruntime.Provenance {
	return skillruntime.Provenance{OriginKind: document.Provenance.Origin.Kind, SourceID: document.Provenance.SourceID}
}

// verdict decides trust for files (path and digest of every distributed file).
func (document contentTrustDocument) verdict(files []skillruntime.ResourceDigest, spec skillruntime.Spec, hasSpec bool) skillruntime.Verdict {
	return skillruntime.Evaluate(document.provenance(), skillruntime.ContentDigest(files, spec, hasSpec), document.Quality.ContentReviewedDigest)
}

// skillTrust is the trust decision and runtime facts of one skill, derived from
// its manifest document and catalog resource rows without reading file bytes.
type skillTrust struct {
	Spec    skillruntime.Spec
	HasSpec bool
	Verdict skillruntime.Verdict
	DepsKey string
}

func evaluateSkillTrust(skillID string, contentJSON []byte, files []skillruntime.ResourceDigest) (skillTrust, error) {
	spec, hasSpec, err := skillruntime.ParseSpec(contentJSON)
	if err != nil {
		return skillTrust{}, fmt.Errorf("skill %s: %w", skillID, err)
	}
	var document contentTrustDocument
	if err := json.Unmarshal(contentJSON, &document); err != nil {
		return skillTrust{}, fmt.Errorf("decode skill %s manifest: %w", skillID, err)
	}
	return skillTrust{
		Spec: spec, HasSpec: hasSpec,
		Verdict: document.verdict(files, spec, hasSpec),
		DepsKey: skillruntime.DepsKey(files, spec, hasSpec),
	}, nil
}

func unavailableLocalSkill(reason string) LocalSkill {
	return LocalSkill{Status: LocalStatusUnavailable, ReasonCodes: []string{reason}}
}

// runtimeProbe performs the only host check the hub can verify for an agent:
// the platform. An empty goos uses the host platform; tests inject a fake.
type runtimeProbe struct {
	goos string
}

func (probe runtimeProbe) platform(spec skillruntime.Spec) []skillruntime.Check {
	goos := probe.goos
	if goos == "" {
		goos = runtime.GOOS
	}
	return skillruntime.PlatformChecks(spec, goos)
}

// snapshotRuntime carries the runtime facts ensure derived for a skill. Root is
// the workspace root that owns the snapshot, state, and doctor caches; it is
// empty when the skill was unavailable. Trusted is false for a third-party
// skill awaiting content review.
type snapshotRuntime struct {
	Root    string
	Spec    skillruntime.Spec
	HasSpec bool
	Trusted bool
}

// Ensure returns the local snapshot for an active, servable skill, exporting
// it when no valid snapshot exists. Skills that are not active or servable
// yield status unavailable without an error; third-party skills awaiting
// content review yield status review_required and export nothing; an error
// means the export itself failed.
func (service SnapshotService) Ensure(ctx context.Context, workspacePath, skillID string) (LocalSkill, error) {
	local, _, err := service.ensure(ctx, workspacePath, skillID)
	return local, err
}

func (service SnapshotService) ensure(ctx context.Context, workspacePath, skillID string) (LocalSkill, snapshotRuntime, error) {
	if skillID == systemskills.CuratorSkillID || !snapshotSkillIDPattern.MatchString(skillID) {
		return unavailableLocalSkill(LocalReasonNotServable), snapshotRuntime{}, nil
	}
	var trust skillTrust
	root, source, err := loadDistributedSkillSource(ctx, workspacePath, skillID, func(entry DistributedSkill, contentJSON []byte) (bool, error) {
		var evalErr error
		trust, evalErr = evaluateSkillTrust(skillID, contentJSON, entry.digests)
		return trust.Verdict.Trusted, evalErr
	})
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return LocalSkill{}, snapshotRuntime{}, ctxErr
		}
		switch {
		case errors.Is(err, ErrResourceContentUnavailable), errors.Is(err, skill.ErrResourceContentUnavailable), errors.Is(err, skill.ErrResourceDigestMismatch):
			return unavailableLocalSkill(LocalReasonResourceContentUnavailable), snapshotRuntime{}, nil
		case errors.Is(err, skill.ErrNotFound), errors.Is(err, catalog.ErrSkillNotServable), errors.Is(err, skill.ErrSnapshotExpired):
			return unavailableLocalSkill(LocalReasonNotServable), snapshotRuntime{}, nil
		}
		return LocalSkill{}, snapshotRuntime{}, err
	}
	facts := snapshotRuntime{Root: root, Spec: trust.Spec, HasSpec: trust.HasSpec, Trusted: trust.Verdict.Trusted}
	if !trust.Verdict.Trusted {
		return LocalSkill{
			ManifestVersion: source.Skill.Version,
			Status:          LocalStatusReviewRequired,
			ReasonCodes:     append([]string(nil), trust.Verdict.ReasonCodes...),
			ReviewCommand:   "skillhub skill review " + skillID,
		}, facts, nil
	}

	marker := snapshotMarker{Schema: snapshotMarkerSchema, SkillID: skillID, ManifestVersion: source.Skill.Version, Files: make([]snapshotMarkerFile, 0, len(source.Files))}
	var dependencyFiles []distributedSkillFile
	for _, file := range source.Files {
		if file.Path == snapshotMarkerName {
			return LocalSkill{}, snapshotRuntime{}, fmt.Errorf("skill %s uses the reserved file name %s", skillID, snapshotMarkerName)
		}
		if err := validateSnapshotRelativePath(file.Path); err != nil {
			return LocalSkill{}, snapshotRuntime{}, err
		}
		marker.Files = append(marker.Files, snapshotMarkerFile{Path: file.Path, Digest: file.Digest, Size: file.Size})
		if skillruntime.IsDependencyManifest(file.Path) {
			dependencyFiles = append(dependencyFiles, file)
		}
	}
	digest := strings.TrimPrefix(source.Skill.Version, "sha256:")
	if len(digest) < 16 || !isLowerHex(digest) {
		return LocalSkill{}, snapshotRuntime{}, fmt.Errorf("skill %s has an invalid manifest version", skillID)
	}
	directory, exported, err := materializeSnapshot(service.memo, root, skillID+"@"+digest[:16], marker, source.Files)
	if err != nil {
		return LocalSkill{}, snapshotRuntime{}, err
	}
	if exported {
		service.maybeCollectGarbage(ctx, root)
	}

	local := LocalSkill{
		Path:            directory,
		ManifestVersion: source.Skill.Version,
		Status:          LocalStatusReady,
		Resources:       make([]LocalResource, 0, len(source.Files)),
	}
	for _, file := range source.Files {
		local.Resources = append(local.Resources, LocalResource{
			Path:      file.Path,
			LocalPath: filepath.Join(directory, filepath.FromSlash(file.Path)),
			Kind:      file.Kind,
		})
	}
	stateDirectory, err := ensureStateDir(root, skillID, trust.DepsKey, dependencyFiles)
	if err != nil {
		return LocalSkill{}, snapshotRuntime{}, err
	}
	configDirectory, err := ensureSnapshotDirectory(root, skillruntime.ConfigDirRoot+"/"+skillID, true)
	if err != nil {
		return LocalSkill{}, snapshotRuntime{}, fmt.Errorf("prepare config directory: %w", err)
	}
	local.StateDirectory = stateDirectory
	local.Env = skillExportEnv(directory, stateDirectory, configDirectory)
	if trust.HasSpec {
		local.Preflight = service.preflight(root, skillID, stateDirectory, local.Env, source.Skill.Version, trust.Spec)
	}
	return local, facts, nil
}

// skillExportEnv lists the variables a host exports when it runs a trusted
// skill's check, setup, or scripts. The config directory may hold an `env`
// file of user-stored secrets; the hub never reads or returns its contents.
func skillExportEnv(skillDirectory, stateDirectory, configDirectory string) map[string]string {
	return map[string]string{
		skillruntime.EnvSkillDir:  skillDirectory,
		skillruntime.EnvStateDir:  stateDirectory,
		skillruntime.EnvConfigDir: configDirectory,
	}
}

func (service SnapshotService) preflight(root, skillID, stateDirectory string, env map[string]string, manifestVersion string, spec skillruntime.Spec) *LocalPreflight {
	live := runtimeProbe{goos: service.goos}.platform(spec)
	plan := &LocalPreflight{
		WorkingDirectory: stateDirectory,
		StateDirectory:   stateDirectory,
		Env:              maps.Clone(env),
		Requires: LocalRequires{
			Bins:      append([]skillruntime.Bin(nil), spec.Requires.Bins...),
			Env:       append([]string(nil), spec.Requires.Env...),
			Platforms: append([]string(nil), spec.Requires.Platforms...),
		},
		LiveChecks: live,
	}
	var cached *skillruntime.Result
	// An unreadable doctor cache only means the state stays unknown.
	if result, ok, err := skillruntime.ReadCache(root, skillID, doctorFingerprint(manifestVersion, spec)); err == nil && ok {
		cached = &result
		plan.Doctor = &LocalDoctor{State: result.State, Basis: skillruntime.BasisTerminal, CheckedAt: result.CheckedAt.UTC().Format(time.RFC3339)}
	}
	plan.State = skillruntime.SetupState(false, true, live, cached)
	const exportHint = "Export " + skillruntime.EnvSkillDir + ", " + skillruntime.EnvStateDir + ", and " + skillruntime.EnvConfigDir + " from `env` when you run check, setup, or the skill's scripts, using your shell's syntax (POSIX `export`, PowerShell `$env:`). If " + skillruntime.EnvConfigDir + "/env exists, load it too (POSIX: `set -a; . \"$SKILLHUB_CONFIG_DIR/env\"; set +a`) and never print its values. The hub verified only the platform; binaries and environment variables are verified by `check` in your own shell."
	switch {
	case spec.Setup.Check != "":
		plan.Check = spec.Setup.Check
		plan.Setup = spec.Setup.Command
		plan.Instruction = exportHint + " Run `check` in working_directory under your own permissions. If it fails, ask the user before running `setup`."
	case spec.Setup.Command != "":
		plan.Setup = spec.Setup.Command
		plan.Instruction = exportHint + " Confirm the listed requirements are met. If they are not, ask the user before running `setup` in working_directory."
	default:
		plan.Instruction = exportHint + " Confirm the listed requirements are met before using the skill's scripts."
	}
	return plan
}

// materializeSnapshot returns the absolute snapshot directory, reusing a
// valid existing snapshot or atomically placing a freshly built one. exported
// reports whether new files were written.
func materializeSnapshot(memo *snapshotMemo, root, name string, marker snapshotMarker, files []distributedSkillFile) (string, bool, error) {
	skillsRoot, err := ensureSnapshotDirectory(root, snapshotRootRelative, true)
	if err != nil {
		return "", false, err
	}
	expectedMarker, err := json.Marshal(marker)
	if err != nil {
		return "", false, err
	}
	target := filepath.Join(skillsRoot, name)
	if memo.verify(target, expectedMarker, marker) == nil {
		touchSnapshot(target)
		return target, false, nil
	}
	stagingRoot, err := ensureSnapshotDirectory(root, snapshotRootRelative+"/"+snapshotStagingName, true)
	if err != nil {
		return "", false, err
	}
	staging, err := buildSnapshotStaging(stagingRoot, expectedMarker, marker, files)
	if err != nil {
		return "", false, err
	}
	for range snapshotPlaceAttempts {
		renameErr := os.Rename(staging, target)
		if renameErr == nil {
			return target, true, nil
		}
		if _, statErr := os.Lstat(target); errors.Is(statErr, fs.ErrNotExist) {
			removeSnapshotTree(staging)
			return "", false, fmt.Errorf("place skill snapshot: %w", renameErr)
		}
		if memo.verify(target, expectedMarker, marker) == nil {
			// Another exporter won the race with an identical, valid snapshot.
			removeSnapshotTree(staging)
			touchSnapshot(target)
			return target, false, nil
		}
		trash := filepath.Join(stagingRoot, "trash-"+randomSnapshotSuffix())
		if err := os.Rename(target, trash); err != nil && !errors.Is(err, fs.ErrNotExist) {
			removeSnapshotTree(staging)
			return "", false, fmt.Errorf("move invalid skill snapshot aside: %w", err)
		}
		removeSnapshotTree(trash)
	}
	removeSnapshotTree(staging)
	return "", false, fmt.Errorf("place skill snapshot %s: concurrent replacement did not settle", name)
}

func buildSnapshotStaging(stagingRoot string, expectedMarker []byte, marker snapshotMarker, files []distributedSkillFile) (_ string, resultErr error) {
	staging := filepath.Join(stagingRoot, randomSnapshotSuffix())
	if err := os.Mkdir(staging, 0o755); err != nil {
		return "", fmt.Errorf("create skill snapshot staging directory: %w", err)
	}
	defer func() {
		if resultErr != nil {
			removeSnapshotTree(staging)
		}
	}()
	handle, err := os.OpenRoot(staging)
	if err != nil {
		return "", fmt.Errorf("open skill snapshot staging directory: %w", err)
	}
	defer handle.Close()
	// The catalog records no file modes, so every file is read-only and
	// non-executable; agents run scripts through their interpreter.
	for _, file := range files {
		if err := validateSnapshotRelativePath(file.Path); err != nil {
			return "", err
		}
		relative := filepath.FromSlash(file.Path)
		if parent := filepath.Dir(relative); parent != "." {
			if err := handle.MkdirAll(parent, 0o755); err != nil {
				return "", fmt.Errorf("create skill snapshot directory for %s: %w", file.Path, err)
			}
		}
		if err := writeSnapshotFile(handle, relative, file.Bytes, 0o444); err != nil {
			return "", err
		}
	}
	if err := writeSnapshotFile(handle, snapshotMarkerName, expectedMarker, 0o444); err != nil {
		return "", err
	}
	return staging, nil
}

func writeSnapshotFile(handle *os.Root, relative string, contents []byte, mode os.FileMode) error {
	file, err := handle.OpenFile(relative, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create skill snapshot file %s: %w", relative, err)
	}
	_, writeErr := file.Write(contents)
	if closeErr := file.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		return fmt.Errorf("write skill snapshot file %s: %w", relative, writeErr)
	}
	// Mode bits are best effort on platforms without POSIX permissions;
	// integrity relies on digest verification.
	if err := handle.Chmod(relative, mode); err != nil && runtime.GOOS != "windows" {
		return fmt.Errorf("protect skill snapshot file %s: %w", relative, err)
	}
	return nil
}

// verifySnapshot checks that directory is a real directory whose marker equals
// the expected marker, whose listed files match size and digest without any
// symlinked component. Extra files are allowed.
func verifySnapshot(directory string, expectedMarker []byte, marker snapshotMarker) error {
	info, err := os.Lstat(directory)
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("skill snapshot %s is not a directory", directory)
	}
	handle, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}
	defer handle.Close()
	stored, err := readSnapshotRegularFile(handle, snapshotMarkerName, snapshotMaxMarkerBytes)
	if err != nil {
		return err
	}
	if !bytes.Equal(stored, expectedMarker) {
		return errors.New("skill snapshot marker does not match the manifest")
	}
	for _, file := range marker.Files {
		contents, err := readSnapshotRegularFile(handle, filepath.FromSlash(file.Path), file.Size)
		if err != nil {
			return err
		}
		if int64(len(contents)) != file.Size {
			return fmt.Errorf("skill snapshot file %s size changed", file.Path)
		}
		sum := sha256.Sum256(contents)
		if "sha256:"+hex.EncodeToString(sum[:]) != file.Digest {
			return fmt.Errorf("skill snapshot file %s digest changed", file.Path)
		}
	}
	return nil
}

// readSnapshotRegularFile reads a file inside the snapshot after checking
// every path component with lstat, refusing symlinks and non-regular files.
// It reads at most limit+1 bytes so a grown file is detected cheaply.
func readSnapshotRegularFile(handle *os.Root, relative string, limit int64) ([]byte, error) {
	if _, err := lstatSnapshotRegular(handle, relative); err != nil {
		return nil, err
	}
	file, err := handle.Open(relative)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, limit+1))
}

// lstatSnapshotRegular lstats every component of relative inside the snapshot,
// refusing symlinks and non-regular files, and returns the final file's info.
func lstatSnapshotRegular(handle *os.Root, relative string) (fs.FileInfo, error) {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	var last fs.FileInfo
	for index := range parts {
		component := filepath.FromSlash(strings.Join(parts[:index+1], "/"))
		info, err := handle.Lstat(component)
		if err != nil {
			return nil, err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil, fmt.Errorf("skill snapshot path %s contains a symlink", relative)
		}
		final := index == len(parts)-1
		if (final && !info.Mode().IsRegular()) || (!final && !info.IsDir()) {
			return nil, fmt.Errorf("skill snapshot path %s has an unexpected file type", relative)
		}
		last = info
	}
	return last, nil
}

// validateSnapshotRelativePath accepts only clean, relative, slash-separated
// paths that stay inside the snapshot directory.
func validateSnapshotRelativePath(relative string) error {
	if relative == "" || strings.HasPrefix(relative, "/") || strings.Contains(relative, "\\") || strings.ContainsRune(relative, 0) || path.Clean(relative) != relative || filepath.IsAbs(filepath.FromSlash(relative)) {
		return fmt.Errorf("unsafe skill resource path %q", relative)
	}
	for _, part := range strings.Split(relative, "/") {
		if part == "" || part == "." || part == ".." || strings.Contains(part, ":") {
			return fmt.Errorf("unsafe skill resource path %q", relative)
		}
	}
	return nil
}

// ensureSnapshotDirectory walks relative under root, refusing symlinked or
// non-directory components. With create, missing components are made with
// mode 0700; without it, a missing component returns an fs.ErrNotExist error.
func ensureSnapshotDirectory(root, relative string, create bool) (string, error) {
	current := root
	for _, component := range strings.Split(relative, "/") {
		if component == "" || component == "." || component == ".." {
			return "", fmt.Errorf("invalid snapshot directory %s", relative)
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) && create {
			if err := os.Mkdir(current, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
				return "", err
			}
			info, err = os.Lstat(current)
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
			return "", fmt.Errorf("unsafe snapshot directory: %s", current)
		}
	}
	return current, nil
}

func touchSnapshot(directory string) {
	now := time.Now()
	// A failed touch only shortens the garbage-collection grace period of a
	// snapshot that is outdated anyway; current snapshots are never collected.
	_ = os.Chtimes(directory, now, now)
}

// removeSnapshotTree deletes a snapshot, state, or staging tree. Files and
// directories are made writable first so removal also works where read-only
// entries (for example module caches) cannot be deleted. Symlinks are removed,
// never followed.
func removeSnapshotTree(directory string) {
	_ = filepath.WalkDir(directory, func(current string, entry fs.DirEntry, err error) error {
		if err == nil && entry.Type().IsRegular() {
			_ = os.Chmod(current, 0o644)
		}
		if entry != nil && entry.IsDir() {
			_ = os.Chmod(current, 0o700)
		}
		return nil
	})
	_ = os.RemoveAll(directory)
}

func randomSnapshotSuffix() string {
	var value [12]byte
	// crypto/rand.Read never returns an error on supported platforms.
	_, _ = rand.Read(value[:])
	return hex.EncodeToString(value[:])
}

var snapshotGCRuns = struct {
	sync.Mutex
	last map[string]time.Time
}{last: map[string]time.Time{}}

func recordSnapshotGC(root string, now time.Time) bool {
	snapshotGCRuns.Lock()
	defer snapshotGCRuns.Unlock()
	if last, ok := snapshotGCRuns.last[root]; ok && now.Sub(last) < snapshotGCInterval {
		return false
	}
	snapshotGCRuns.last[root] = now
	return true
}

// maybeCollectGarbage runs best-effort garbage collection at most once per
// interval per workspace in this process.
func (service SnapshotService) maybeCollectGarbage(ctx context.Context, root string) {
	if !recordSnapshotGC(root, time.Now()) {
		return
	}
	// Collection is best effort; a failure leaves reclaimable files behind.
	_ = service.collectGarbage(ctx, root, SnapshotGCMinAge)
}

// CollectGarbage removes snapshot directories that do not match a current
// active skill's manifest version and are older than minAge, and staging
// entries older than one hour. Current snapshots are always kept.
func (service SnapshotService) CollectGarbage(ctx context.Context, workspacePath string, minAge time.Duration) error {
	root, err := workspace.Discover(workspacePath)
	if err != nil {
		return err
	}
	recordSnapshotGC(root, time.Now())
	return service.collectGarbage(ctx, root, minAge)
}

func (SnapshotService) collectGarbage(ctx context.Context, root string, minAge time.Duration) error {
	skillsRoot, err := ensureSnapshotDirectory(root, snapshotRootRelative, false)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	hasSnapshots := err == nil
	envsRoot, err := ensureSnapshotDirectory(root, skillruntime.StateDirRoot, false)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	hasStates := err == nil
	if !hasSnapshots && !hasStates {
		return nil
	}
	now := time.Now()
	if hasSnapshots {
		if stagingRoot, err := ensureSnapshotDirectory(root, snapshotRootRelative+"/"+snapshotStagingName, false); err == nil {
			removeOlderEntries(stagingRoot, now, snapshotStagingMaxAge, nil)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	active, err := activeSkillNames(ctx, root)
	if err != nil {
		return err
	}
	if hasSnapshots {
		removeOlderEntries(skillsRoot, now, minAge, func(name string) bool {
			return name == snapshotStagingName || active.snapshots[name]
		})
	}
	if hasStates {
		removeOlderEntries(envsRoot, now, snapshotStagingMaxAge, func(name string) bool {
			return !strings.HasPrefix(name, stateStagingPrefix)
		})
		removeOlderEntries(envsRoot, now, StateDirGCMinAge, func(name string) bool {
			return strings.HasPrefix(name, stateStagingPrefix) || active.states[name]
		})
	}
	return nil
}

func removeOlderEntries(directory string, now time.Time, minAge time.Duration, keep func(string) bool) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if keep != nil && keep(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil || now.Sub(info.ModTime()) < minAge {
			continue
		}
		removeSnapshotTree(filepath.Join(directory, entry.Name()))
	}
}

// activeNames lists the on-disk names that active skills still use.
type activeNames struct {
	// snapshots holds "<id>@<manifest-digest16>" for every active skill.
	snapshots map[string]bool
	// states holds "<id>@<deps16>" for every active skill whose content is
	// trusted, the only skills that get a state directory.
	states map[string]bool
}

// activeSkillNames computes the names in use from catalog rows and manifest
// documents of the current generation, without reading skill files.
func activeSkillNames(ctx context.Context, root string) (names activeNames, resultErr error) {
	_, handle, err := openDistributionGeneration(ctx, root)
	if err != nil {
		return names, err
	}
	defer closeDistributionHandle(handle, &resultErr)
	rows, err := handle.DB.QueryContext(ctx, `SELECT resources.skill_id,resources.path,resources.digest,resources.size_bytes FROM resources JOIN skills ON skills.id=resources.skill_id WHERE skills.status='active' ORDER BY resources.skill_id,resources.path`)
	if err != nil {
		return names, err
	}
	bySkill := map[string][]manifestDigestEntry{}
	for rows.Next() {
		var id, internal, digest string
		var size int64
		if err := rows.Scan(&id, &internal, &digest, &size); err != nil {
			_ = rows.Close()
			return names, err
		}
		if relative, ok := distributedRelativePath(internal, id); ok {
			bySkill[id] = append(bySkill[id], manifestDigestEntry{Path: relative, Digest: digest, Size: size})
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return names, err
	}
	if err := rows.Close(); err != nil {
		return names, err
	}
	manifests := map[string]string{}
	documents, err := handle.DB.QueryContext(ctx, `SELECT canonical_entities.id,canonical_entities.content_json FROM canonical_entities JOIN skills ON skills.id=canonical_entities.id WHERE skills.status='active'`)
	if err != nil {
		return names, err
	}
	for documents.Next() {
		var id, contentJSON string
		if err := documents.Scan(&id, &contentJSON); err != nil {
			_ = documents.Close()
			return names, err
		}
		manifests[id] = contentJSON
	}
	if err := documents.Err(); err != nil {
		_ = documents.Close()
		return names, err
	}
	if err := documents.Close(); err != nil {
		return names, err
	}
	names = activeNames{snapshots: make(map[string]bool, len(bySkill)), states: make(map[string]bool, len(bySkill))}
	for id, entries := range bySkill {
		sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
		digest, err := hashDistributedManifest(entries)
		if err != nil {
			return names, err
		}
		names.snapshots[id+"@"+digest[:16]] = true
		files := make([]skillruntime.ResourceDigest, len(entries))
		for index, entry := range entries {
			files[index] = skillruntime.ResourceDigest{Path: entry.Path, Digest: entry.Digest}
		}
		// A skill whose manifest cannot be evaluated has no state directory to keep.
		if trust, err := evaluateSkillTrust(id, []byte(manifests[id]), files); err == nil && trust.Verdict.Trusted {
			if name, err := skillruntime.StateDirName(id, trust.DepsKey); err == nil {
				names.states[name] = true
			}
		}
	}
	return names, nil
}

// ensureStateDir returns the writable state directory for a skill's dependency
// set, creating it on first use with a copy of the dependency manifest files so
// installers can run there and rewrite lock files. An existing directory is
// reused untouched apart from refreshing its modification time; its files are
// never overwritten.
func ensureStateDir(root, skillID, depsKey string, dependencyFiles []distributedSkillFile) (string, error) {
	name, err := skillruntime.StateDirName(skillID, depsKey)
	if err != nil {
		return "", err
	}
	envsRoot, err := ensureSnapshotDirectory(root, skillruntime.StateDirRoot, true)
	if err != nil {
		return "", err
	}
	target := filepath.Join(envsRoot, name)
	reuse := func() (string, bool, error) {
		info, err := os.Lstat(target)
		if errors.Is(err, fs.ErrNotExist) {
			return "", false, nil
		}
		if err != nil {
			return "", false, err
		}
		if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
			return "", false, fmt.Errorf("unsafe state directory: %s", target)
		}
		touchSnapshot(target)
		return target, true, nil
	}
	if directory, found, err := reuse(); err != nil || found {
		return directory, err
	}
	staging, err := os.MkdirTemp(envsRoot, stateStagingPrefix)
	if err != nil {
		return "", fmt.Errorf("create state directory staging: %w", err)
	}
	if err := writeStateDependencies(staging, dependencyFiles); err != nil {
		removeSnapshotTree(staging)
		return "", err
	}
	if err := os.Rename(staging, target); err != nil {
		removeSnapshotTree(staging)
		// Another process may have created the directory first.
		if directory, found, reuseErr := reuse(); reuseErr == nil && found {
			return directory, nil
		}
		return "", fmt.Errorf("place state directory: %w", err)
	}
	return target, nil
}

func writeStateDependencies(staging string, files []distributedSkillFile) error {
	handle, err := os.OpenRoot(staging)
	if err != nil {
		return fmt.Errorf("open state directory staging: %w", err)
	}
	defer handle.Close()
	for _, file := range files {
		if err := validateSnapshotRelativePath(file.Path); err != nil {
			return err
		}
		relative := filepath.FromSlash(file.Path)
		if parent := filepath.Dir(relative); parent != "." {
			if err := handle.MkdirAll(parent, 0o700); err != nil {
				return fmt.Errorf("create state directory for %s: %w", file.Path, err)
			}
		}
		if err := writeSnapshotFile(handle, relative, file.Bytes, 0o644); err != nil {
			return err
		}
	}
	return nil
}
