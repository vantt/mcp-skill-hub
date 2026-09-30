// Package selfupdate provides in-place updates for managed Skill Hub installations.
package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/version"
)

const (
	// DefaultReleaseBaseURL is the standard GitHub releases base URL.
	DefaultReleaseBaseURL = "https://github.com/vantt/mcp-skill-hub/releases"

	// DefaultSigstoreIssuer is the GitHub Actions OIDC token issuer.
	DefaultSigstoreIssuer = "https://token.actions.githubusercontent.com"

	// DefaultSigstoreWorkflow is the release workflow identity prefix.
	DefaultSigstoreWorkflow = "https://github.com/vantt/mcp-skill-hub/.github/workflows/release.yml"

	// ManagedMarkerHeader is the expected first line of .skillhub-managed.
	ManagedMarkerHeader = "skillhub-managed-v1"

	// Bounded limits for downloads
	maxChecksumSize = 1024 * 1024 // 1 MB
	maxBundleSize   = 1024 * 1024 // 1 MB
	maxArchiveSize  = 134217728   // 128 MB
)

// Result describes the outcome of a self-update operation.
type Result struct {
	CurrentVersion  string `json:"current_version"`
	TargetVersion   string `json:"target_version"`
	Updated         bool   `json:"updated"`
	AlreadyUpToDate bool   `json:"already_up_to_date"`
	CheckOnly       bool   `json:"check_only"`
	Message         string `json:"message"`
}

// Error represents an actionable failure during selfupdate.
type Error struct {
	Summary string
	Why     string
	Fix     string
	Err     error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Summary, e.Err)
	}
	return e.Summary
}

// Options configure the self-update execution.
type Options struct {
	// CurrentVersion overrides the running version (defaults to version.Current().Version).
	CurrentVersion string

	// TargetVersion specifies an explicit target version (e.g. "0.2.0" or "v0.2.0").
	// If empty, the latest stable release is fetched.
	TargetVersion string

	// ExecutablePath overrides the path of the current binary (defaults to os.Executable()).
	ExecutablePath string

	// BaseURL overrides the releases base URL (env SKILLHUB_UPDATE_BASE_URL).
	BaseURL string

	// CheckOnly when true only checks for updates without downloading or replacing the binary.
	CheckOnly bool

	// RequireSig enforces cosign signature verification (also set by SKILLHUB_REQUIRE_SIGNATURE=1).
	RequireSig bool

	// HTTPClient overrides the HTTP client for network calls.
	HTTPClient *http.Client

	// Stdout receives non-error progress output.
	Stdout io.Writer

	// Stderr receives warnings.
	Stderr io.Writer

	// GOOS overrides runtime.GOOS (for tests).
	GOOS string

	// GOARCH overrides runtime.GOARCH (for tests).
	GOARCH string

	// CosignVerifier overrides signature verification execution (for tests).
	CosignVerifier func(ctx context.Context, manifestPath, bundlePath, identity, issuer string) error

	// BinaryVerifier overrides post-replacement binary verification (for tests).
	BinaryVerifier func(ctx context.Context, exePath string) error
}

// Check queries for updates without changing the installation.
func Check(ctx context.Context, opts Options) (*Result, error) {
	opts.CheckOnly = true
	return Run(ctx, opts)
}

// Update downloads and installs the update.
func Update(ctx context.Context, opts Options) (*Result, error) {
	opts.CheckOnly = false
	return Run(ctx, opts)
}

// Run executes the self-update or check process.
func Run(ctx context.Context, opts Options) (*Result, error) {
	// 1. Resolve executable and check managed marker
	exePath, err := resolveExecutablePath(opts.ExecutablePath)
	if err != nil {
		return nil, &Error{
			Summary: "Could not determine executable path.",
			Why:     err.Error(),
			Fix:     "Ensure skillhub is executed from a valid filesystem path.",
		}
	}

	binDir := filepath.Dir(exePath)
	markerPath := filepath.Join(binDir, ".skillhub-managed")
	markerVersion, err := verifyManagedMarker(markerPath, exePath)
	if err != nil {
		return nil, err
	}

	// Clean up any stale Windows .old file from previous update run
	cleanOldBinary(exePath)

	// 2. Resolve current version
	currentVersion := opts.CurrentVersion
	if currentVersion == "" {
		currentVersion = version.Current().Version
	}
	if (currentVersion == "" || currentVersion == "dev") && markerVersion != "" {
		currentVersion = markerVersion
	}
	if currentVersion == "" {
		currentVersion = "dev"
	}
	normalizedCurrent := strings.TrimPrefix(strings.TrimSpace(currentVersion), "v")

	// 3. Resolve target version & release tag
	baseURL := resolveBaseURL(opts.BaseURL)
	targetVersion, releaseTag, err := resolveTarget(ctx, opts, baseURL)
	if err != nil {
		return nil, err
	}
	normalizedTarget := strings.TrimPrefix(strings.TrimSpace(targetVersion), "v")

	// 4. Compare versions
	cmp, hasSemver := CompareVersions(normalizedCurrent, normalizedTarget)

	isExplicitTarget := strings.TrimSpace(opts.TargetVersion) != ""

	if isExplicitTarget {
		// When explicit target is given, target == current means already up to date
		if (hasSemver && cmp == 0) || normalizedCurrent == normalizedTarget {
			return &Result{
				CurrentVersion:  normalizedCurrent,
				TargetVersion:   normalizedTarget,
				Updated:         false,
				AlreadyUpToDate: true,
				CheckOnly:       opts.CheckOnly,
				Message:         fmt.Sprintf("Already up to date (%s).", normalizedCurrent),
			}, nil
		}
	} else {
		// When resolving latest, if current >= target, already up to date
		if (hasSemver && cmp >= 0) || (!hasSemver && normalizedCurrent == normalizedTarget) {
			return &Result{
				CurrentVersion:  normalizedCurrent,
				TargetVersion:   normalizedTarget,
				Updated:         false,
				AlreadyUpToDate: true,
				CheckOnly:       opts.CheckOnly,
				Message:         fmt.Sprintf("Already up to date (%s).", normalizedCurrent),
			}, nil
		}
	}

	// An update is available
	if opts.CheckOnly {
		return &Result{
			CurrentVersion:  normalizedCurrent,
			TargetVersion:   normalizedTarget,
			Updated:         false,
			AlreadyUpToDate: false,
			CheckOnly:       true,
			Message:         fmt.Sprintf("Update available: %s -> %s. Run `skillhub update` to install.", normalizedCurrent, normalizedTarget),
		}, nil
	}

	// 5. Check signature verification requirements
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}

	requireSig := opts.RequireSig || os.Getenv("SKILLHUB_REQUIRE_SIGNATURE") == "1"
	cosignPath := findCosignPath()
	hasCosign := cosignPath != "" || opts.CosignVerifier != nil || os.Getenv("SKILLHUB_FIXTURE_VERIFIER") != ""

	if requireSig && !hasCosign {
		return nil, &Error{
			Summary: "Signature verification is required but cosign is not installed.",
			Why:     "SKILLHUB_REQUIRE_SIGNATURE is set, but cosign was not found on PATH.",
			Fix:     "Install cosign from https://github.com/sigstore/cosign to continue, or unset SKILLHUB_REQUIRE_SIGNATURE.",
		}
	}

	// 6. Determine archive asset name
	goos := opts.GOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := opts.GOARCH
	if goarch == "" {
		goarch = runtime.GOARCH
	}

	ext := "tar.gz"
	binaryName := "skillhub"
	if goos == "windows" {
		ext = "zip"
		binaryName = "skillhub.exe"
	}
	archiveName := fmt.Sprintf("skillhub-%s-%s-%s.%s", normalizedTarget, goos, goarch, ext)

	// 7. Download checksums.txt
	checksumURL := fmt.Sprintf("%s/download/%s/checksums.txt", baseURL, releaseTag)
	checksumBytes, err := downloadBytes(ctx, client, checksumURL, maxChecksumSize)
	if err != nil {
		return nil, &Error{
			Summary: "Failed to download checksums manifest.",
			Why:     fmt.Sprintf("Could not retrieve %s: %v", checksumURL, err),
			Fix:     "Check your network connection and retry the update.",
			Err:     err,
		}
	}

	// 8. Signature verification if cosign available
	sigstoreWorkflow := os.Getenv("SKILLHUB_SIGSTORE_WORKFLOW")
	if sigstoreWorkflow == "" {
		sigstoreWorkflow = DefaultSigstoreWorkflow
	}
	sigstoreIssuer := os.Getenv("SKILLHUB_SIGSTORE_ISSUER")
	if sigstoreIssuer == "" {
		sigstoreIssuer = DefaultSigstoreIssuer
	}
	identity := fmt.Sprintf("%s@refs/tags/%s", sigstoreWorkflow, releaseTag)

	if hasCosign {
		bundleURL := fmt.Sprintf("%s/download/%s/checksums.txt.sigstore.json", baseURL, releaseTag)
		bundleBytes, bErr := downloadBytes(ctx, client, bundleURL, maxBundleSize)
		if bErr != nil {
			return nil, &Error{
				Summary: "Failed to download signature bundle.",
				Why:     fmt.Sprintf("Could not retrieve %s: %v", bundleURL, bErr),
				Fix:     "Ensure the release assets include checksums.txt.sigstore.json and retry.",
				Err:     bErr,
			}
		}

		if err := verifySignature(ctx, opts.CosignVerifier, cosignPath, checksumBytes, bundleBytes, identity, sigstoreIssuer); err != nil {
			return nil, &Error{
				Summary: "Signature verification failed.",
				Why:     fmt.Sprintf("Sigstore verification failed for checksums.txt: %v", err),
				Fix:     "Verify that cosign is functioning properly and release signatures are valid.",
				Err:     err,
			}
		}
		if opts.Stdout != nil {
			fmt.Fprintln(opts.Stdout, "Authenticated checksums.txt with Sigstore bundle")
		}
	} else {
		if opts.Stdout != nil {
			fmt.Fprintf(opts.Stdout, "Signature not checked (cosign not installed). To verify: cosign verify-blob --bundle checksums.txt.sigstore.json --certificate-identity %s --certificate-oidc-issuer %s checksums.txt\n", identity, sigstoreIssuer)
		}
	}

	// 9. Verify archive SHA-256 against checksums.txt
	expectedSHA, err := parseChecksums(checksumBytes, archiveName)
	if err != nil {
		return nil, &Error{
			Summary: "Checksum entry not found.",
			Why:     fmt.Sprintf("checksums.txt does not contain an entry for %s: %v", archiveName, err),
			Fix:     "Ensure the release includes an archive for this platform and retry.",
			Err:     err,
		}
	}

	archiveURL := fmt.Sprintf("%s/download/%s/%s", baseURL, releaseTag, archiveName)
	archiveBytes, err := downloadBytes(ctx, client, archiveURL, maxArchiveSize)
	if err != nil {
		return nil, &Error{
			Summary: "Failed to download release archive.",
			Why:     fmt.Sprintf("Could not retrieve %s: %v", archiveURL, err),
			Fix:     "Check your network connection and retry the update.",
			Err:     err,
		}
	}

	actualSHA := fmt.Sprintf("%x", sha256.Sum256(archiveBytes))
	if !strings.EqualFold(actualSHA, expectedSHA) {
		return nil, &Error{
			Summary: "Checksum verification failed.",
			Why:     fmt.Sprintf("SHA-256 verification failed for %s: expected %s, got %s", archiveName, expectedSHA, actualSHA),
			Fix:     "The downloaded artifact does not match published checksums. Check your connection or cache and retry.",
		}
	}
	if opts.Stdout != nil {
		fmt.Fprintf(opts.Stdout, "Verified SHA-256 for %s\n", archiveName)
	}

	// 10. Extract binary only (validate member name to prevent path traversal)
	newBinaryBytes, err := extractBinary(archiveBytes, ext, binaryName)
	if err != nil {
		return nil, &Error{
			Summary: "Failed to extract binary from archive.",
			Why:     err.Error(),
			Fix:     "The release archive format or contents are invalid. Report this issue.",
			Err:     err,
		}
	}

	// 11. Atomic replacement with rollback
	if err := replaceBinaryWithRollback(ctx, opts, exePath, markerPath, newBinaryBytes, normalizedTarget); err != nil {
		return nil, err
	}

	return &Result{
		CurrentVersion:  normalizedCurrent,
		TargetVersion:   normalizedTarget,
		Updated:         true,
		AlreadyUpToDate: false,
		CheckOnly:       false,
		Message:         fmt.Sprintf("Updated %s -> %s.", normalizedCurrent, normalizedTarget),
	}, nil
}

func resolveExecutablePath(override string) (string, error) {
	exePath := override
	if exePath == "" {
		exePath = os.Getenv("SKILLHUB_UPDATE_TEST_EXECUTABLE")
	}
	if exePath == "" {
		var err error
		exePath, err = os.Executable()
		if err != nil {
			return "", err
		}
	}
	if realPath, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = realPath
	}
	return filepath.Clean(exePath), nil
}

func verifyManagedMarker(markerPath, exePath string) (string, error) {
	fi, err := os.Lstat(markerPath)
	if err != nil {
		return "", &Error{
			Summary: "Refusing to update an unmanaged binary.",
			Why:     fmt.Sprintf("The binary at %s was not installed by the Skill Hub installer (missing .skillhub-managed marker).", exePath),
			Fix: "To reinstall or manage Skill Hub with the official installer:\n" +
				"  Linux/macOS: curl -fsSL https://github.com/vantt/mcp-skill-hub/releases/latest/download/install.sh | sh\n" +
				"  Windows:     powershell -ExecutionPolicy ByPass -c \"irm https://github.com/vantt/mcp-skill-hub/releases/latest/download/install.ps1 | iex\"",
		}
	}

	if fi.Mode()&os.ModeSymlink != 0 {
		return "", &Error{
			Summary: "Refusing to update an unmanaged binary.",
			Why:     fmt.Sprintf("The installation marker at %s is a symlink.", markerPath),
			Fix:     "Reinstall Skill Hub with the official installer.",
		}
	}

	data, err := os.ReadFile(markerPath)
	if err != nil {
		return "", &Error{
			Summary: "Could not read installation marker.",
			Why:     err.Error(),
			Fix:     "Check file permissions on .skillhub-managed.",
			Err:     err,
		}
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != ManagedMarkerHeader {
		return "", &Error{
			Summary: "Refusing to update an unmanaged binary.",
			Why:     fmt.Sprintf("The marker at %s does not contain a valid %q header.", markerPath, ManagedMarkerHeader),
			Fix: "To reinstall or manage Skill Hub with the official installer:\n" +
				"  Linux/macOS: curl -fsSL https://github.com/vantt/mcp-skill-hub/releases/latest/download/install.sh | sh\n" +
				"  Windows:     powershell -ExecutionPolicy ByPass -c \"irm https://github.com/vantt/mcp-skill-hub/releases/latest/download/install.ps1 | iex\"",
		}
	}

	markerVersion := ""
	for _, line := range lines[1:] {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "version=") {
			markerVersion = strings.TrimPrefix(line, "version=")
			break
		}
	}

	return markerVersion, nil
}

func cleanOldBinary(exePath string) {
	oldExe := exePath + ".old"
	if _, err := os.Stat(oldExe); err == nil {
		_ = os.Remove(oldExe)
	}
}

func resolveBaseURL(override string) string {
	baseURL := override
	if baseURL == "" {
		baseURL = os.Getenv("SKILLHUB_UPDATE_BASE_URL")
	}
	if baseURL == "" {
		baseURL = DefaultReleaseBaseURL
	}
	return strings.TrimRight(baseURL, "/")
}

func resolveTarget(ctx context.Context, opts Options, baseURL string) (string, string, error) {
	if strings.TrimSpace(opts.TargetVersion) != "" {
		raw := strings.TrimSpace(opts.TargetVersion)
		ver := strings.TrimPrefix(raw, "v")
		return ver, "v" + ver, nil
	}

	latestURL := baseURL + "/latest"
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestURL, nil)
	if err != nil {
		return "", "", &Error{
			Summary: "Could not create request for latest release.",
			Why:     err.Error(),
			Fix:     "Check the update configuration and retry.",
			Err:     err,
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", "", &Error{
			Summary: "Failed to resolve latest release.",
			Why:     fmt.Sprintf("Request to %s failed: %v", latestURL, err),
			Fix:     "Check your internet connection and retry.",
			Err:     err,
		}
	}
	defer resp.Body.Close()

	var rawTag string
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		loc := resp.Header.Get("Location")
		if loc == "" {
			return "", "", &Error{
				Summary: "Failed to resolve latest release.",
				Why:     "Server returned redirect with missing Location header.",
				Fix:     "Retry the update or specify --version <v>.",
			}
		}
		rawTag, err = extractTagFromURL(loc)
		if err != nil {
			return "", "", &Error{
				Summary: "Failed to parse release tag from redirect.",
				Why:     err.Error(),
				Fix:     "Specify an explicit target version via `skillhub update --version <v>`.",
				Err:     err,
			}
		}
	} else if resp.StatusCode == http.StatusOK {
		if resp.Request != nil && resp.Request.URL != nil {
			rawTag, err = extractTagFromURL(resp.Request.URL.String())
			if err != nil {
				return "", "", &Error{
					Summary: "Failed to parse release tag from URL.",
					Why:     err.Error(),
					Fix:     "Specify an explicit target version via `skillhub update --version <v>`.",
					Err:     err,
				}
			}
		} else {
			return "", "", &Error{
				Summary: "Failed to resolve latest release.",
				Why:     fmt.Sprintf("Unexpected status 200 without redirect when checking %s", latestURL),
				Fix:     "Specify an explicit target version via `skillhub update --version <v>`.",
			}
		}
	} else {
		return "", "", &Error{
			Summary: "Failed to resolve latest release.",
			Why:     fmt.Sprintf("HTTP status %d when checking %s", resp.StatusCode, latestURL),
			Fix:     "Check your network connection or specify --version <v>.",
		}
	}

	targetVer := strings.TrimPrefix(rawTag, "v")
	releaseTag := rawTag
	if !strings.HasPrefix(releaseTag, "v") {
		releaseTag = "v" + releaseTag
	}
	return targetVer, releaseTag, nil
}

func extractTagFromURL(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := range len(parts) - 1 {
		if parts[i] == "tag" {
			return parts[i+1], nil
		}
	}
	if len(parts) > 0 && parts[len(parts)-1] != "latest" {
		return parts[len(parts)-1], nil
	}
	return "", fmt.Errorf("could not determine release tag from URL path: %s", u.Path)
}

func findCosignPath() string {
	cosignPath, err := exec.LookPath("cosign")
	if err == nil {
		return cosignPath
	}
	if runtime.GOOS == "windows" {
		cosignPath, err = exec.LookPath("cosign.exe")
		if err == nil {
			return cosignPath
		}
	}
	return ""
}

func verifySignature(ctx context.Context, verifier func(ctx context.Context, m, b, id, iss string) error, cosignPath string, manifestBytes, bundleBytes []byte, identity, issuer string) error {
	tmpDir, err := os.MkdirTemp("", "skillhub-update-sig-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	manifestPath := filepath.Join(tmpDir, "checksums.txt")
	bundlePath := filepath.Join(tmpDir, "checksums.txt.sigstore.json")

	if err := os.WriteFile(manifestPath, manifestBytes, 0600); err != nil {
		return err
	}
	if err := os.WriteFile(bundlePath, bundleBytes, 0600); err != nil {
		return err
	}

	if verifier != nil {
		return verifier(ctx, manifestPath, bundlePath, identity, issuer)
	}

	if fixtureVerifier := os.Getenv("SKILLHUB_FIXTURE_VERIFIER"); fixtureVerifier != "" {
		cmd := exec.CommandContext(ctx, fixtureVerifier, manifestPath, bundlePath, identity, issuer)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
		}
		return nil
	}

	if cosignPath == "" {
		return errors.New("cosign binary not found")
	}

	cmd := exec.CommandContext(ctx, cosignPath, "verify-blob",
		"--bundle", bundlePath,
		"--certificate-identity", identity,
		"--certificate-oidc-issuer", issuer,
		manifestPath,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func downloadBytes(ctx context.Context, client *http.Client, urlStr string, maxBytes int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP status %d", resp.StatusCode)
	}

	lr := io.LimitReader(resp.Body, maxBytes+1)
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("downloaded data exceeded maximum allowed size (%d bytes)", maxBytes)
	}
	return data, nil
}

func parseChecksums(data []byte, targetFile string) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	var foundHash string
	count := 0

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		hash := fields[0]
		fileName := strings.TrimPrefix(fields[1], "*")
		fileName = filepath.Base(fileName)

		if fileName == targetFile {
			if len(hash) != 64 {
				return "", fmt.Errorf("invalid SHA-256 hash length in checksum entry: %s", hash)
			}
			foundHash = hash
			count++
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("reading checksums.txt: %w", err)
	}
	if count == 0 {
		return "", fmt.Errorf("no entry found for %s", targetFile)
	}
	if count > 1 {
		return "", fmt.Errorf("multiple entries found for %s", targetFile)
	}
	return foundHash, nil
}

func extractBinary(archiveBytes []byte, ext, binaryName string) ([]byte, error) {
	switch ext {
	case "tar.gz":
		return extractTarGz(archiveBytes, binaryName)
	case "zip":
		return extractZip(archiveBytes, binaryName)
	default:
		return nil, fmt.Errorf("unsupported archive format: %s", ext)
	}
}

func extractTarGz(archiveBytes []byte, binaryName string) ([]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(archiveBytes))
	if err != nil {
		return nil, fmt.Errorf("invalid gzip archive: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var binaryData []byte
	matchCount := 0

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error reading tar archive: %w", err)
		}

		cleanName := filepath.Clean(hdr.Name)
		if strings.HasPrefix(cleanName, "..") || filepath.IsAbs(cleanName) || strings.HasPrefix(hdr.Name, "/") || strings.HasPrefix(hdr.Name, "\\") {
			return nil, fmt.Errorf("malicious archive entry detected: %s", hdr.Name)
		}

		baseName := filepath.Base(cleanName)
		if baseName == binaryName && (cleanName == binaryName || cleanName == "./"+binaryName || cleanName == filepath.Clean("./"+binaryName)) {
			if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA {
				return nil, fmt.Errorf("archive entry %s is not a regular file", hdr.Name)
			}
			data, err := io.ReadAll(io.LimitReader(tr, maxArchiveSize))
			if err != nil {
				return nil, fmt.Errorf("error reading archive entry %s: %w", hdr.Name, err)
			}
			binaryData = data
			matchCount++
		}
	}

	if matchCount == 0 {
		return nil, fmt.Errorf("release archive does not contain binary %s", binaryName)
	}
	if matchCount > 1 {
		return nil, fmt.Errorf("release archive contains multiple entries for %s", binaryName)
	}

	return binaryData, nil
}

func extractZip(archiveBytes []byte, binaryName string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
	if err != nil {
		return nil, fmt.Errorf("invalid zip archive: %w", err)
	}

	var binaryData []byte
	matchCount := 0

	for _, f := range zr.File {
		cleanName := filepath.Clean(f.Name)
		if strings.HasPrefix(cleanName, "..") || filepath.IsAbs(cleanName) || strings.HasPrefix(f.Name, "/") || strings.HasPrefix(f.Name, "\\") {
			return nil, fmt.Errorf("malicious archive entry detected: %s", f.Name)
		}

		baseName := filepath.Base(cleanName)
		if baseName == binaryName && (cleanName == binaryName || cleanName == "./"+binaryName || cleanName == filepath.Clean("./"+binaryName)) {
			if f.FileInfo().IsDir() {
				return nil, fmt.Errorf("archive entry %s is a directory", f.Name)
			}
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("error opening archive entry %s: %w", f.Name, err)
			}
			data, err := io.ReadAll(io.LimitReader(rc, maxArchiveSize))
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("error reading archive entry %s: %w", f.Name, err)
			}
			binaryData = data
			matchCount++
		}
	}

	if matchCount == 0 {
		return nil, fmt.Errorf("release archive does not contain binary %s", binaryName)
	}
	if matchCount > 1 {
		return nil, fmt.Errorf("release archive contains multiple entries for %s", binaryName)
	}

	return binaryData, nil
}

func replaceBinaryWithRollback(ctx context.Context, opts Options, exePath, markerPath string, newBytes []byte, newVersion string) error {
	binDir := filepath.Dir(exePath)
	pid := os.Getpid()
	stagedExe := filepath.Join(binDir, fmt.Sprintf(".skillhub.new.%d", pid))
	backupExe := filepath.Join(binDir, fmt.Sprintf(".skillhub.bak.%d", pid))
	markerBackup := filepath.Join(binDir, fmt.Sprintf(".skillhub-managed.bak.%d", pid))
	stagedMarker := filepath.Join(binDir, fmt.Sprintf(".skillhub-managed.new.%d", pid))
	oldExe := exePath + ".old"

	defer func() {
		_ = os.Remove(stagedExe)
		_ = os.Remove(stagedMarker)
	}()

	// Stage new binary
	if err := os.WriteFile(stagedExe, newBytes, 0755); err != nil {
		return &Error{
			Summary: "Could not write staged binary.",
			Why:     err.Error(),
			Fix:     "Check write permissions in the installation directory.",
			Err:     err,
		}
	}

	// Stage new marker
	newMarker := fmt.Sprintf("%s\nversion=%s\n", ManagedMarkerHeader, newVersion)
	if err := os.WriteFile(stagedMarker, []byte(newMarker), 0600); err != nil {
		return &Error{
			Summary: "Could not write staged installation marker.",
			Why:     err.Error(),
			Fix:     "Check write permissions in the installation directory.",
			Err:     err,
		}
	}

	// Backup existing marker
	if err := copyFile(markerPath, markerBackup); err != nil {
		return &Error{
			Summary: "Could not back up existing installation marker.",
			Why:     err.Error(),
			Fix:     "Check write permissions in the installation directory.",
			Err:     err,
		}
	}

	isWindows := opts.GOOS == "windows" || (opts.GOOS == "" && runtime.GOOS == "windows")

	if isWindows {
		// On Windows: rename running exe to .old, then staged to exe
		_ = os.Remove(oldExe)
		if err := os.Rename(exePath, oldExe); err != nil {
			_ = os.Remove(markerBackup)
			return &Error{
				Summary: "Could not rename running binary to .old.",
				Why:     err.Error(),
				Fix:     "Ensure no conflicting processes are locking the binary.",
				Err:     err,
			}
		}
		if err := os.Rename(stagedExe, exePath); err != nil {
			// Immediate rollback
			_ = os.Rename(oldExe, exePath)
			_ = os.Remove(markerBackup)
			return &Error{
				Summary: "Could not replace binary with staged binary.",
				Why:     err.Error(),
				Fix:     "The original binary was restored. Retry the update.",
				Err:     err,
			}
		}
	} else {
		// On Unix: copy running exe to backupExe, then atomic rename stagedExe -> exePath
		if err := copyFile(exePath, backupExe); err != nil {
			_ = os.Remove(markerBackup)
			return &Error{
				Summary: "Could not back up running binary.",
				Why:     err.Error(),
				Fix:     "Check write permissions in the installation directory.",
				Err:     err,
			}
		}
		_ = os.Chmod(backupExe, 0755)

		if err := os.Rename(stagedExe, exePath); err != nil {
			_ = os.Remove(backupExe)
			_ = os.Remove(markerBackup)
			return &Error{
				Summary: "Could not atomically replace binary.",
				Why:     err.Error(),
				Fix:     "The original binary was not modified.",
				Err:     err,
			}
		}
	}

	// Replace marker
	if err := os.Rename(stagedMarker, markerPath); err != nil {
		// Roll back binary and marker
		if isWindows {
			_ = os.Remove(exePath)
			_ = os.Rename(oldExe, exePath)
		} else {
			_ = os.Rename(backupExe, exePath)
			_ = os.Remove(backupExe)
		}
		_ = os.Rename(markerBackup, markerPath)
		return &Error{
			Summary: "Could not update installation marker.",
			Why:     err.Error(),
			Fix:     "Installation was rolled back.",
			Err:     err,
		}
	}

	// Verify replacement
	var verifyErr error
	if opts.BinaryVerifier != nil {
		verifyErr = opts.BinaryVerifier(ctx, exePath)
	} else {
		cmd := exec.CommandContext(ctx, exePath, "version")
		out, err := cmd.CombinedOutput()
		if err != nil {
			verifyErr = fmt.Errorf("%v (output: %s)", err, strings.TrimSpace(string(out)))
		}
	}

	if verifyErr != nil {
		// ROLLBACK
		if isWindows {
			_ = os.Remove(exePath)
			_ = os.Rename(oldExe, exePath)
		} else {
			_ = os.Rename(backupExe, exePath)
		}
		_ = os.Rename(markerBackup, markerPath)

		return &Error{
			Summary: "Updated binary failed verification; rolled back to previous version.",
			Why:     fmt.Sprintf("Post-update verification failed: %v", verifyErr),
			Fix:     "The original installation has been restored. Retry the update or report this issue.",
			Err:     verifyErr,
		}
	}

	// Success: clean up backups
	_ = os.Remove(backupExe)
	_ = os.Remove(markerBackup)
	if isWindows {
		_ = os.Remove(oldExe)
	}

	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// SemVer represents a parsed Semantic Version (SemVer 2.0.0).
type SemVer struct {
	Major      uint64
	Minor      uint64
	Patch      uint64
	Prerelease string
	Build      string
}

// ParseSemVer parses a SemVer string, ignoring any leading 'v'.
func ParseSemVer(s string) (*SemVer, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if s == "" {
		return nil, errors.New("empty version string")
	}

	var build string
	if idx := strings.Index(s, "+"); idx != -1 {
		build = s[idx+1:]
		s = s[:idx]
	}

	var prerelease string
	if idx := strings.Index(s, "-"); idx != -1 {
		prerelease = s[idx+1:]
		s = s[:idx]
	}

	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("expected 3 numeric components, got %d in %q", len(parts), s)
	}

	major, err := parseComponent(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid major: %w", err)
	}
	minor, err := parseComponent(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid minor: %w", err)
	}
	patch, err := parseComponent(parts[2])
	if err != nil {
		return nil, fmt.Errorf("invalid patch: %w", err)
	}

	return &SemVer{
		Major:      major,
		Minor:      minor,
		Patch:      patch,
		Prerelease: prerelease,
		Build:      build,
	}, nil
}

func parseComponent(s string) (uint64, error) {
	if s == "" {
		return 0, errors.New("empty component")
	}
	if len(s) > 1 && s[0] == '0' {
		return 0, errors.New("leading zeros not permitted")
	}
	return strconv.ParseUint(s, 10, 64)
}

// Compare returns -1 if a < b, 0 if a == b, and 1 if a > b.
func (a *SemVer) Compare(b *SemVer) int {
	if a.Major != b.Major {
		if a.Major < b.Major {
			return -1
		}
		return 1
	}
	if a.Minor != b.Minor {
		if a.Minor < b.Minor {
			return -1
		}
		return 1
	}
	if a.Patch != b.Patch {
		if a.Patch < b.Patch {
			return -1
		}
		return 1
	}

	if a.Prerelease == "" && b.Prerelease != "" {
		return 1
	}
	if a.Prerelease != "" && b.Prerelease == "" {
		return -1
	}
	if a.Prerelease == "" && b.Prerelease == "" {
		return 0
	}

	return comparePrereleases(a.Prerelease, b.Prerelease)
}

func comparePrereleases(preA, preB string) int {
	partsA := strings.Split(preA, ".")
	partsB := strings.Split(preB, ".")
	minLen := len(partsA)
	if len(partsB) < minLen {
		minLen = len(partsB)
	}

	for i := range minLen {
		idA, idB := partsA[i], partsB[i]
		if idA == idB {
			continue
		}
		numA, isNumA := parseUint(idA)
		numB, isNumB := parseUint(idB)

		if isNumA && isNumB {
			if numA < numB {
				return -1
			}
			return 1
		}
		if isNumA && !isNumB {
			return -1
		}
		if !isNumA && isNumB {
			return 1
		}
		if idA < idB {
			return -1
		}
		return 1
	}

	if len(partsA) < len(partsB) {
		return -1
	}
	if len(partsA) > len(partsB) {
		return 1
	}
	return 0
}

func parseUint(s string) (uint64, bool) {
	if s == "" {
		return 0, false
	}
	n, err := strconv.ParseUint(s, 10, 64)
	return n, err == nil
}

// CompareVersions compares two version strings. Returns (-1, 0, 1) and true if both
// parsed as SemVer. Otherwise falls back to string/dev comparison and returns false.
func CompareVersions(current, target string) (int, bool) {
	svCur, errCur := ParseSemVer(current)
	svTar, errTar := ParseSemVer(target)
	if errCur == nil && errTar == nil {
		return svCur.Compare(svTar), true
	}
	if current == target {
		return 0, false
	}
	if errCur != nil && errTar == nil {
		// e.g. current is "dev", target is "0.2.0": current is older
		return -1, false
	}
	if errCur == nil && errTar != nil {
		return 1, false
	}
	if current < target {
		return -1, false
	}
	return 1, false
}
