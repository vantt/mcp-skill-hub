package skillruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// doctorCacheDir is relative to the workspace root.
const doctorCacheDir = "runtime/cache/doctor"

var (
	skillIDPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	fingerprintPattern = regexp.MustCompile(`^[0-9a-f]{16,}$`)
)

// MachineID identifies this machine for doctor results: the first 12 hex
// characters of sha256(hostname + GOOS + GOARCH).
func MachineID() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown-host"
	}
	sum := sha256.Sum256([]byte(hostname + runtime.GOOS + runtime.GOARCH))
	return hex.EncodeToString(sum[:])[:12]
}

// RuntimeFingerprint keys doctor results: sha256 over the caller-supplied
// manifest version and the spec fingerprint, so any change to either
// invalidates cached results.
func RuntimeFingerprint(manifestVersion string, spec Spec) string {
	sum := sha256.Sum256([]byte(manifestVersion + "\n" + spec.Fingerprint()))
	return hex.EncodeToString(sum[:])
}

// CachePath returns <root>/runtime/cache/doctor/<machine-id>/<skill-id>@<fingerprint[:16]>.json.
func CachePath(root, skillID, fingerprint string) (string, error) {
	relative, err := cacheRelativeDir(skillID, fingerprint)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, filepath.FromSlash(relative), cacheFileName(skillID, fingerprint)), nil
}

func cacheRelativeDir(skillID, fingerprint string) (string, error) {
	if !skillIDPattern.MatchString(skillID) {
		return "", fmt.Errorf("invalid skill id %q", skillID)
	}
	if !fingerprintPattern.MatchString(fingerprint) {
		return "", fmt.Errorf("invalid runtime fingerprint")
	}
	return doctorCacheDir + "/" + MachineID(), nil
}

func cacheFileName(skillID, fingerprint string) string {
	return skillID + "@" + fingerprint[:16] + ".json"
}

// WriteCache stores a doctor result atomically (temp file and rename) with
// mode 0600 under 0700 directories, refusing symlinked path components.
func WriteCache(root, skillID, fingerprint string, result Result) error {
	relative, err := cacheRelativeDir(skillID, fingerprint)
	if err != nil {
		return err
	}
	if err := ensurePrivateDirectory(root, relative, true); err != nil {
		return err
	}
	dir := filepath.Join(root, filepath.FromSlash(relative))
	target := filepath.Join(dir, cacheFileName(skillID, fingerprint))
	if info, err := os.Lstat(target); err == nil && info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("unsafe doctor cache file: %s", target)
	}
	encoded, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("encode doctor result: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".doctor-*.tmp")
	if err != nil {
		return fmt.Errorf("create doctor cache file: %w", err)
	}
	tempName := temp.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tempName)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		_ = temp.Close()
		return fmt.Errorf("set doctor cache mode: %w", err)
	}
	if _, err := temp.Write(append(encoded, '\n')); err != nil {
		_ = temp.Close()
		return fmt.Errorf("write doctor cache file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		_ = temp.Close()
		return fmt.Errorf("sync doctor cache file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close doctor cache file: %w", err)
	}
	if err := os.Rename(tempName, target); err != nil {
		return fmt.Errorf("commit doctor cache file: %w", err)
	}
	committed = true
	return nil
}

// ReadCache loads a cached doctor result. It reports false without error when
// no result is cached, and refuses symlinked path components.
func ReadCache(root, skillID, fingerprint string) (Result, bool, error) {
	relative, err := cacheRelativeDir(skillID, fingerprint)
	if err != nil {
		return Result{}, false, err
	}
	if err := ensurePrivateDirectory(root, relative, false); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Result{}, false, nil
		}
		return Result{}, false, err
	}
	target := filepath.Join(root, filepath.FromSlash(relative), cacheFileName(skillID, fingerprint))
	info, err := os.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return Result{}, false, nil
	}
	if err != nil {
		return Result{}, false, err
	}
	if !info.Mode().IsRegular() {
		return Result{}, false, fmt.Errorf("unsafe doctor cache file: %s", target)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return Result{}, false, err
	}
	var result Result
	if err := json.Unmarshal(data, &result); err != nil {
		return Result{}, false, fmt.Errorf("decode doctor cache file: %w", err)
	}
	switch result.State {
	case StateReady, StateSetupRequired, StateUnsupportedPlatform:
	default:
		return Result{}, false, fmt.Errorf("doctor cache file has unknown state %q", result.State)
	}
	return result, true, nil
}

// ensurePrivateDirectory walks relative under root, rejecting symlinks and
// non-directories. With create it makes missing components with mode 0700;
// without it a missing component returns an fs.ErrNotExist error.
func ensurePrivateDirectory(root, relative string, create bool) error {
	current := root
	for _, component := range strings.Split(relative, "/") {
		if component == "" || component == "." || component == ".." {
			return fmt.Errorf("invalid runtime directory %s", relative)
		}
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, fs.ErrNotExist) && create {
			if err := os.Mkdir(current, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
				return err
			}
			info, err = os.Lstat(current)
		}
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("unsafe runtime directory: %s", current)
		}
	}
	return nil
}

// StateReviewRequired is the setup state of a third-party skill whose content
// a human has not approved.
const StateReviewRequired = "review_required"

// SetupState summarizes readiness for resolver annotations without running
// anything: review_required for an unapproved third-party skill, "" for a
// trusted skill without a runtime block, unsupported_platform when the
// platform check fails, the cached terminal doctor state as a hint when one
// exists, and unknown otherwise. Binaries and environment are not probed by
// the hub; see PlatformChecks.
func SetupState(untrusted bool, hasSpec bool, platform []Check, cached *Result) string {
	if untrusted {
		return StateReviewRequired
	}
	if !hasSpec {
		return ""
	}
	for _, check := range platform {
		if check.Kind == KindPlatform && check.Status == StatusFail {
			return StateUnsupportedPlatform
		}
	}
	if cached != nil {
		return cached.State
	}
	return StateUnknown
}
