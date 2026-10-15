package skillruntime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

// A skill bundle is a gzip-compressed tar archive of one approved skill
// version: a metadata entry first, then every distributed file of the skill in
// path order. The archive is deterministic, so the same skill version always
// produces the same bytes.
const (
	// BundleMediaType is the content type of a skill bundle.
	BundleMediaType = "application/vnd.skillhub.bundle+gzip"
	// BundleMetadataName is the first archive entry, and the file that marks a
	// complete skill version in the client cache. No skill file may use it.
	BundleMetadataName = ".skillhub-bundle.json"
	// BundleSchema is the version of the metadata document.
	BundleSchema = 1
	// BundleMaxFiles and BundleMaxBytes repeat the distribution limits of the
	// catalog, so a client never accepts more than a hub may serve.
	BundleMaxFiles = 512
	BundleMaxBytes = 16 << 20
	// BundleMaxMetadataBytes bounds the metadata entry.
	BundleMaxMetadataBytes = 2 << 20
	// bundleHeaderAllowance covers the tar headers (including long-name
	// records) and padding of every entry when bounding the decompressed size.
	bundleHeaderAllowance = 4 << 10
)

// BundleMaxArchiveBytes bounds the decompressed archive, and therefore also
// the compressed body a client reads.
const BundleMaxArchiveBytes = BundleMaxBytes + BundleMaxMetadataBytes + (BundleMaxFiles+2)*bundleHeaderAllowance

// Errors reported while reading a bundle.
var (
	// ErrBundleDigestMismatch reports an archive whose files do not hash to
	// the digest it was requested by.
	ErrBundleDigestMismatch = errors.New("skill bundle does not match its digest")
	// ErrBundleInvalid reports an archive that is malformed or breaks a limit.
	ErrBundleInvalid = errors.New("invalid skill bundle")
	// ErrBundleLimit is joined to ErrBundleInvalid when the archive breaks a
	// size or count limit, so a receiver can tell too large from malformed.
	ErrBundleLimit = errors.New("skill bundle over a limit")
)

// BundleFile is one distributed file of a skill version. Path is relative to
// the skill folder with forward slashes; Digest is "sha256:<hex>".
type BundleFile struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
	Kind   string `json:"kind,omitempty"`
	// Executable carries the file's execute bit in a submission; a hub bundle
	// leaves it out.
	Executable bool `json:"executable,omitempty"`
}

// BundleSubmission is what a machine adds to a bundle it sends to a hub for
// review: the version it started from, a label for the machine, and the files
// its folder filter left out. A bundle the hub serves has none of it.
type BundleSubmission struct {
	// BaseVersion is the content digest of the checkout the files came from;
	// it is empty for a skill the hub is not known to have.
	BaseVersion string `json:"base_version,omitempty"`
	// ClientLabel names the submitting machine. It is a label for people only.
	ClientLabel string           `json:"client_label,omitempty"`
	Excluded    []BundleExcluded `json:"excluded,omitempty"`
}

// BundleExcluded names a file the sending machine's filter left out, and why.
type BundleExcluded struct {
	Path   string `json:"path"`
	Reason string `json:"reason,omitempty"`
}

// BundleMetadata describes the skill version inside a bundle. Files is the
// complete file list the manifest digest covers. Detail is the hub's
// read model of the skill without its SKILL.md text (which is a file of the
// bundle), kept opaque here so this package stays transport-neutral.
type BundleMetadata struct {
	Schema          int               `json:"schema"`
	SkillID         string            `json:"skill_id"`
	ManifestVersion string            `json:"manifest_version"`
	Files           []BundleFile      `json:"files"`
	DepsKey         string            `json:"deps_key"`
	Runtime         *Spec             `json:"runtime,omitempty"`
	Detail          json.RawMessage   `json:"detail,omitempty"`
	Submission      *BundleSubmission `json:"submission,omitempty"`
}

// ManifestDigest returns the lowercase hex digest that a skill URI pins: the
// SHA-256 of the JSON list of {path, digest, size} sorted by path.
func ManifestDigest(files []BundleFile) (string, error) {
	type entry struct {
		Path   string `json:"path"`
		Digest string `json:"digest"`
		Size   int64  `json:"size"`
	}
	entries := make([]entry, 0, len(files))
	for _, file := range files {
		entries = append(entries, entry{Path: file.Path, Digest: file.Digest, Size: file.Size})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	encoded, err := json.Marshal(entries)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// WriteBundle writes the deterministic archive of a skill version. contents
// holds the bytes of every file listed in metadata.Files.
func WriteBundle(w io.Writer, metadata BundleMetadata, contents map[string][]byte) error {
	if err := checkBundleMetadata(metadata); err != nil {
		return err
	}
	files := append([]BundleFile(nil), metadata.Files...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	metadata.Files = files
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode bundle metadata: %w", err)
	}
	if len(encoded) > BundleMaxMetadataBytes {
		return fmt.Errorf("%w: metadata is larger than %d bytes", ErrBundleInvalid, BundleMaxMetadataBytes)
	}
	compressor, err := gzip.NewWriterLevel(w, gzip.DefaultCompression)
	if err != nil {
		return err
	}
	archive := tar.NewWriter(compressor)
	write := func(name string, data []byte) error {
		header := &tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: 0o644, Size: int64(len(data)), ModTime: time.Unix(0, 0).UTC(), Format: tar.FormatPAX}
		if err := archive.WriteHeader(header); err != nil {
			return fmt.Errorf("write bundle entry %s: %w", name, err)
		}
		if _, err := archive.Write(data); err != nil {
			return fmt.Errorf("write bundle entry %s: %w", name, err)
		}
		return nil
	}
	if err := write(BundleMetadataName, encoded); err != nil {
		return err
	}
	for _, file := range files {
		data, ok := contents[file.Path]
		if !ok || int64(len(data)) != file.Size {
			return fmt.Errorf("%w: contents of %s are missing or have the wrong size", ErrBundleInvalid, file.Path)
		}
		if err := write(file.Path, data); err != nil {
			return err
		}
	}
	if err := archive.Close(); err != nil {
		return err
	}
	return compressor.Close()
}

var (
	bundleSkillIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	bundleDigestPattern  = regexp.MustCompile(`^[0-9a-f]{64}$`)
	fileDigestPattern    = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	hubIDPattern         = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{0,80}$`)
)

// ValidBundleDigest reports whether digest is a lowercase hex manifest digest.
func ValidBundleDigest(digest string) bool { return bundleDigestPattern.MatchString(digest) }

// ValidBundlePath accepts only clean, relative, slash-separated paths that stay
// inside the skill folder, the same rule the hub applies to its own snapshots.
func ValidBundlePath(relative string) bool {
	if relative == "" || strings.HasPrefix(relative, "/") || strings.Contains(relative, "\\") || strings.ContainsRune(relative, 0) || path.Clean(relative) != relative || filepath.IsAbs(filepath.FromSlash(relative)) {
		return false
	}
	for _, part := range strings.Split(relative, "/") {
		if part == "" || part == "." || part == ".." || strings.Contains(part, ":") {
			return false
		}
	}
	return true
}

func checkBundleMetadata(metadata BundleMetadata) error {
	if metadata.Schema != BundleSchema {
		return fmt.Errorf("%w: unsupported metadata schema %d", ErrBundleInvalid, metadata.Schema)
	}
	if !bundleSkillIDPattern.MatchString(metadata.SkillID) {
		return fmt.Errorf("%w: invalid skill id", ErrBundleInvalid)
	}
	if !ValidBundleDigest(strings.TrimPrefix(metadata.ManifestVersion, "sha256:")) || !strings.HasPrefix(metadata.ManifestVersion, "sha256:") {
		return fmt.Errorf("%w: invalid manifest version", ErrBundleInvalid)
	}
	if !depsKeyPattern.MatchString(metadata.DepsKey) {
		return fmt.Errorf("%w: invalid dependency key", ErrBundleInvalid)
	}
	if len(metadata.Files) > BundleMaxFiles {
		return fmt.Errorf("%w: %d files; supported range is 1..%d: %w", ErrBundleInvalid, len(metadata.Files), BundleMaxFiles, ErrBundleLimit)
	}
	if len(metadata.Files) == 0 {
		return fmt.Errorf("%w: %d files; supported range is 1..%d", ErrBundleInvalid, len(metadata.Files), BundleMaxFiles)
	}
	seen := make(map[string]bool, len(metadata.Files))
	var total int64
	for _, file := range metadata.Files {
		if !ValidBundlePath(file.Path) || file.Path == BundleMetadataName {
			return fmt.Errorf("%w: unsafe file path %q", ErrBundleInvalid, file.Path)
		}
		if seen[file.Path] {
			return fmt.Errorf("%w: duplicate file %s", ErrBundleInvalid, file.Path)
		}
		seen[file.Path] = true
		if !fileDigestPattern.MatchString(file.Digest) || file.Size < 0 {
			return fmt.Errorf("%w: invalid digest or size for %s", ErrBundleInvalid, file.Path)
		}
		total += file.Size
	}
	if total > BundleMaxBytes {
		return fmt.Errorf("%w: files total %d bytes; the limit is %d: %w", ErrBundleInvalid, total, BundleMaxBytes, ErrBundleLimit)
	}
	if !seen["SKILL.md"] {
		return fmt.Errorf("%w: no top-level SKILL.md", ErrBundleInvalid)
	}
	return nil
}

// limitedReader returns ErrBundleInvalid once more than remaining bytes are read.
type limitedReader struct {
	reader    io.Reader
	remaining int64
}

func (limited *limitedReader) Read(buffer []byte) (int, error) {
	if limited.remaining <= 0 {
		var probe [1]byte
		if n, _ := limited.reader.Read(probe[:]); n > 0 {
			return 0, fmt.Errorf("%w: the archive is larger than %d bytes: %w", ErrBundleInvalid, BundleMaxArchiveBytes, ErrBundleLimit)
		}
		return 0, io.EOF
	}
	if int64(len(buffer)) > limited.remaining {
		buffer = buffer[:limited.remaining]
	}
	n, err := limited.reader.Read(buffer)
	limited.remaining -= int64(n)
	return n, err
}

// extractBundle reads an archive into the empty directory staging and returns
// its metadata. It accepts the archive only when the metadata hashes to
// digest and every file matches its listed size and digest; anything else in
// the archive is an error. On error staging may hold partial files.
func extractBundle(archive io.Reader, digest, staging string) (BundleMetadata, error) {
	return extractBundleFile(archive, digest, staging, nil)
}

// ExtractBundle reads a bundle a machine sends to a hub into the empty
// directory staging and returns its metadata, with the same limits and path
// checks as a bundle the hub serves. There is no digest to pin: the archive is
// accepted when its metadata hashes to its own file list and every file
// matches its listed size and digest. The metadata entry is written into
// staging as well; on error staging may hold partial files.
func ExtractBundle(archive io.Reader, staging string) (BundleMetadata, error) {
	return extractBundleFile(archive, "", staging, nil)
}

// ExtractBundleAdmitting is ExtractBundle that asks admit about the verified
// metadata before any file is written. When admit fails, its error is returned
// and staging is left empty, so a bundle the receiver would refuse anyway
// costs no disk.
func ExtractBundleAdmitting(archive io.Reader, staging string, admit func(BundleMetadata) error) (BundleMetadata, error) {
	return extractBundleFile(archive, "", staging, admit)
}

// extractBundleFile is the shared reader; an empty digest accepts the digest
// the file list computes.
func extractBundleFile(archive io.Reader, digest, staging string, admit func(BundleMetadata) error) (BundleMetadata, error) {
	compressed := &limitedReader{reader: archive, remaining: BundleMaxArchiveBytes}
	decompressor, err := gzip.NewReader(compressed)
	if err != nil {
		return BundleMetadata{}, fmt.Errorf("%w: %w", ErrBundleInvalid, err)
	}
	defer decompressor.Close()
	reader := tar.NewReader(&limitedReader{reader: decompressor, remaining: BundleMaxArchiveBytes})
	header, err := reader.Next()
	if err != nil {
		return BundleMetadata{}, fmt.Errorf("%w: no metadata entry: %w", ErrBundleInvalid, err)
	}
	if header.Name != BundleMetadataName || header.Typeflag != tar.TypeReg || header.Size > BundleMaxMetadataBytes {
		return BundleMetadata{}, fmt.Errorf("%w: the first entry is not the metadata", ErrBundleInvalid)
	}
	encoded, err := io.ReadAll(reader)
	if err != nil {
		return BundleMetadata{}, fmt.Errorf("%w: read metadata: %w", ErrBundleInvalid, err)
	}
	var metadata BundleMetadata
	if err := json.Unmarshal(encoded, &metadata); err != nil {
		return BundleMetadata{}, fmt.Errorf("%w: decode metadata: %w", ErrBundleInvalid, err)
	}
	if err := checkBundleMetadata(metadata); err != nil {
		return BundleMetadata{}, err
	}
	computed, err := ManifestDigest(metadata.Files)
	if err != nil {
		return BundleMetadata{}, err
	}
	if digest == "" {
		digest = computed
	}
	if computed != digest || metadata.ManifestVersion != "sha256:"+digest {
		return BundleMetadata{}, fmt.Errorf("%w: the file list hashes to %s, not %s", ErrBundleDigestMismatch, computed, digest)
	}
	if admit != nil {
		if err := admit(metadata); err != nil {
			return BundleMetadata{}, err
		}
	}
	expected := make(map[string]BundleFile, len(metadata.Files))
	for _, file := range metadata.Files {
		expected[file.Path] = file
	}
	handle, err := os.OpenRoot(staging)
	if err != nil {
		return BundleMetadata{}, err
	}
	defer handle.Close()
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return BundleMetadata{}, fmt.Errorf("%w: %w", ErrBundleInvalid, err)
		}
		file, listed := expected[header.Name]
		if !listed || header.Typeflag != tar.TypeReg || header.Size != file.Size {
			return BundleMetadata{}, fmt.Errorf("%w: unexpected archive entry %q", ErrBundleInvalid, header.Name)
		}
		delete(expected, header.Name)
		if err := writeBundleFile(handle, file, reader); err != nil {
			return BundleMetadata{}, err
		}
	}
	if len(expected) > 0 {
		return BundleMetadata{}, fmt.Errorf("%w: %d listed files are missing from the archive", ErrBundleInvalid, len(expected))
	}
	if err := writeReadOnlyFile(handle, BundleMetadataName, encoded); err != nil {
		return BundleMetadata{}, err
	}
	return metadata, nil
}

func writeBundleFile(handle *os.Root, file BundleFile, contents io.Reader) error {
	relative := filepath.FromSlash(file.Path)
	if parent := filepath.Dir(relative); parent != "." {
		if err := handle.MkdirAll(parent, 0o755); err != nil {
			return fmt.Errorf("create directory for %s: %w", file.Path, err)
		}
	}
	output, err := handle.OpenFile(relative, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create %s: %w", file.Path, err)
	}
	sum := sha256.New()
	source := &archiveSource{reader: io.LimitReader(contents, file.Size+1)}
	written, copyErr := io.Copy(io.MultiWriter(output, sum), source)
	if closeErr := output.Close(); copyErr == nil {
		copyErr = closeErr
	}
	if source.err != nil {
		// The archive ended or broke inside this file, for example because
		// the download was cut off: the bundle is invalid, not the disk.
		return fmt.Errorf("%w: read %s: %w", ErrBundleInvalid, file.Path, source.err)
	}
	if copyErr != nil {
		return fmt.Errorf("write %s: %w", file.Path, copyErr)
	}
	if written != file.Size || "sha256:"+hex.EncodeToString(sum.Sum(nil)) != file.Digest {
		return fmt.Errorf("%w: %s does not match its listed digest", ErrBundleDigestMismatch, file.Path)
	}
	return protectFile(handle, relative)
}

// archiveSource remembers the first error reading the archive returned, so a
// broken archive is told apart from a failure to write the file.
type archiveSource struct {
	reader io.Reader
	err    error
}

func (source *archiveSource) Read(buffer []byte) (int, error) {
	n, err := source.reader.Read(buffer)
	if err != nil && !errors.Is(err, io.EOF) && source.err == nil {
		source.err = err
	}
	return n, err
}

func writeReadOnlyFile(handle *os.Root, relative string, data []byte) error {
	output, err := handle.OpenFile(relative, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := output.Write(data)
	if closeErr := output.Close(); writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		return writeErr
	}
	return protectFile(handle, relative)
}

// protectFile makes a cached file read-only. Mode bits are best effort on
// Windows; integrity rests on the digest check at extraction.
func protectFile(handle *os.Root, relative string) error {
	if err := handle.Chmod(relative, 0o444); err != nil && runtime.GOOS != "windows" {
		return fmt.Errorf("protect %s: %w", relative, err)
	}
	return nil
}

// ClientStateDir returns the directory where this machine keeps what it
// fetched from remote hubs: $XDG_STATE_HOME/skillhub when set, otherwise
// %LocalAppData%\skillhub on Windows, ~/Library/Application Support/skillhub
// on macOS, and ~/.local/state/skillhub elsewhere.
func ClientStateDir() (string, error) {
	if base := os.Getenv("XDG_STATE_HOME"); base != "" && filepath.IsAbs(base) {
		return filepath.Join(base, "skillhub"), nil
	}
	switch runtime.GOOS {
	case "windows":
		base, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(base, "skillhub"), nil
	case "darwin":
		base, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(base, "skillhub"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".local", "state", "skillhub"), nil
}

// RemoteCacheDir returns the root of the remote skill cache inside a client
// state directory.
func RemoteCacheDir(stateDir string) string { return filepath.Join(stateDir, "remote") }

// HubID derives the cache directory name of a hub from its base URL: the host
// and port made safe for every file system, plus a short hash of the URL so
// two hubs never share a directory.
func HubID(hubURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(hubURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", fmt.Errorf("invalid hub URL %q", hubURL)
	}
	normalized := strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host) + strings.TrimRight(parsed.EscapedPath(), "/")
	var label strings.Builder
	for _, character := range strings.ToLower(parsed.Host) {
		if (character >= 'a' && character <= 'z') || (character >= '0' && character <= '9') || character == '.' || character == '-' {
			label.WriteRune(character)
		} else {
			label.WriteByte('-')
		}
	}
	name := strings.Trim(label.String(), ".-")
	if len(name) > 48 {
		name = strings.Trim(name[:48], ".-")
	}
	sum := sha256.Sum256([]byte(normalized))
	if name == "" {
		return "hub-" + hex.EncodeToString(sum[:])[:10], nil
	}
	return name + "-" + hex.EncodeToString(sum[:])[:10], nil
}

const (
	cacheStagingName   = ".staging"
	cacheHubFileName   = ".hub.json"
	cacheStateName     = ".state"
	cacheStagingMaxAge = time.Hour
)

// RemoteCache stores skill versions fetched from remote hubs under
// <root>/<hub-id>/<skill-id>/<digest>/, one read-only directory per version.
// A directory exists only once it is complete, because it is built in
// <root>/<hub-id>/.staging and renamed into place.
type RemoteCache struct {
	Root string
}

// CachedSkill is one complete skill version in the cache.
type CachedSkill struct {
	Directory string
	Digest    string
	Metadata  BundleMetadata
	LastUsed  time.Time
}

// Size is the total size of the skill files.
func (cached CachedSkill) Size() int64 {
	var total int64
	for _, file := range cached.Metadata.Files {
		total += file.Size
	}
	return total
}

// installPlaceAttempts bounds the retries of the final rename.
const installPlaceAttempts = 5

// installHook runs after a version is fully written to staging and before it
// is renamed into place. Tests use it to simulate a crash at that moment.
var installHook func(staging string)

// HubDir returns the cache directory of a hub.
func (cache RemoteCache) HubDir(hubID string) string { return filepath.Join(cache.Root, hubID) }

// StateRoot returns the directory that holds the writable state directories
// and per-skill config directories of a hub's skills. It survives cache
// clearing, so installed dependencies and stored secrets are kept.
func (cache RemoteCache) StateRoot(hubID string) string {
	return filepath.Join(cache.HubDir(hubID), cacheStateName)
}

func (cache RemoteCache) versionDir(hubID, skillID, digest string) (string, error) {
	if cache.Root == "" || !hubIDPattern.MatchString(hubID) || !bundleSkillIDPattern.MatchString(skillID) || !ValidBundleDigest(digest) {
		return "", fmt.Errorf("invalid cache key %s/%s/%s", hubID, skillID, digest)
	}
	return filepath.Join(cache.Root, hubID, skillID, digest), nil
}

// Lookup returns a complete cached version. It checks the directory, the
// metadata, and that every listed file is present with its listed size; it
// does not re-hash the files, which were verified when they were extracted
// and are read-only. A hit refreshes the last-use time used by garbage
// collection.
func (cache RemoteCache) Lookup(hubID, skillID, digest string) (CachedSkill, bool) {
	directory, err := cache.versionDir(hubID, skillID, digest)
	if err != nil {
		return CachedSkill{}, false
	}
	cached, err := readCachedSkill(directory, skillID, digest)
	if err != nil {
		return CachedSkill{}, false
	}
	now := time.Now()
	if os.Chtimes(directory, now, now) == nil {
		cached.LastUsed = now
	}
	return cached, true
}

func readCachedSkill(directory, skillID, digest string) (CachedSkill, error) {
	info, err := os.Lstat(directory)
	if err != nil {
		return CachedSkill{}, err
	}
	if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
		return CachedSkill{}, fmt.Errorf("cached skill %s is not a directory", directory)
	}
	handle, err := os.OpenRoot(directory)
	if err != nil {
		return CachedSkill{}, err
	}
	defer handle.Close()
	metaInfo, err := handle.Lstat(BundleMetadataName)
	if err != nil || !metaInfo.Mode().IsRegular() || metaInfo.Size() > BundleMaxMetadataBytes {
		return CachedSkill{}, fmt.Errorf("cached skill %s has no metadata", directory)
	}
	encoded, err := handle.ReadFile(BundleMetadataName)
	if err != nil {
		return CachedSkill{}, err
	}
	var metadata BundleMetadata
	if err := json.Unmarshal(encoded, &metadata); err != nil {
		return CachedSkill{}, err
	}
	if err := checkBundleMetadata(metadata); err != nil {
		return CachedSkill{}, err
	}
	if metadata.SkillID != skillID || metadata.ManifestVersion != "sha256:"+digest {
		return CachedSkill{}, fmt.Errorf("cached skill %s holds another version", directory)
	}
	for _, file := range metadata.Files {
		fileInfo, err := lstatInside(handle, file.Path)
		if err != nil || fileInfo.Size() != file.Size {
			return CachedSkill{}, fmt.Errorf("cached skill %s is incomplete", directory)
		}
	}
	return CachedSkill{Directory: directory, Digest: digest, Metadata: metadata, LastUsed: info.ModTime()}, nil
}

// lstatInside checks every component of a slash-separated path inside handle,
// refusing symlinks, and returns the final regular file's info.
func lstatInside(handle *os.Root, relative string) (fs.FileInfo, error) {
	parts := strings.Split(relative, "/")
	var last fs.FileInfo
	for index := range parts {
		info, err := handle.Lstat(filepath.FromSlash(strings.Join(parts[:index+1], "/")))
		if err != nil {
			return nil, err
		}
		final := index == len(parts)-1
		if info.Mode()&fs.ModeSymlink != 0 || (final && !info.Mode().IsRegular()) || (!final && !info.IsDir()) {
			return nil, fmt.Errorf("unexpected file type at %s", relative)
		}
		last = info
	}
	return last, nil
}

// Install verifies archive against digest and places the version in the
// cache atomically: the files are written to a staging directory and renamed
// into place only when every file matched, so a failure or a crash never
// leaves a partial version. A version already present is reused.
func (cache RemoteCache) Install(hubID, skillID, digest string, archive io.Reader) (CachedSkill, error) {
	target, err := cache.versionDir(hubID, skillID, digest)
	if err != nil {
		return CachedSkill{}, err
	}
	if cached, ok := cache.Lookup(hubID, skillID, digest); ok {
		return cached, nil
	}
	stagingRoot := filepath.Join(cache.Root, hubID, cacheStagingName)
	if err := os.MkdirAll(stagingRoot, 0o700); err != nil {
		return CachedSkill{}, fmt.Errorf("create the cache staging directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return CachedSkill{}, fmt.Errorf("create the cache directory: %w", err)
	}
	staging, err := os.MkdirTemp(stagingRoot, "fetch-")
	if err != nil {
		return CachedSkill{}, fmt.Errorf("create a staging directory: %w", err)
	}
	placed := false
	defer func() {
		if !placed {
			removeTree(staging)
		}
	}()
	metadata, err := extractBundle(archive, digest, staging)
	if err != nil {
		return CachedSkill{}, err
	}
	if metadata.SkillID != skillID {
		return CachedSkill{}, fmt.Errorf("%w: the archive holds skill %s, not %s", ErrBundleInvalid, metadata.SkillID, skillID)
	}
	if installHook != nil {
		installHook(staging)
	}
	// The rename fails when a non-empty target exists (POSIX may replace an
	// empty one, which is never a valid version), and the existing copy is
	// checked instead. A failure with no target is retried briefly: another
	// process may have removed the empty skill directory meanwhile, or, on
	// Windows, a virus scanner may still hold a new file open.
	var renameErr error
	for attempt := 0; attempt < installPlaceAttempts; attempt++ {
		renameErr = os.Rename(staging, target)
		if renameErr == nil {
			placed = true
			return cache.mustLookup(hubID, skillID, digest)
		}
		if _, statErr := os.Lstat(target); errors.Is(statErr, fs.ErrNotExist) {
			if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
				return CachedSkill{}, fmt.Errorf("create the cache directory: %w", err)
			}
			time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
			continue
		}
		if cached, ok := cache.Lookup(hubID, skillID, digest); ok {
			// Another proxy placed the same version first.
			return cached, nil
		}
		// An invalid leftover (for example edited by hand) is moved aside.
		trash := filepath.Join(stagingRoot, "trash-"+randomSuffix())
		if err := os.Rename(target, trash); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return CachedSkill{}, fmt.Errorf("move an invalid cached skill aside: %w", err)
		}
		removeTree(trash)
	}
	return CachedSkill{}, fmt.Errorf("place the cached skill %s: %w", target, renameErr)
}

func (cache RemoteCache) mustLookup(hubID, skillID, digest string) (CachedSkill, error) {
	cached, ok := cache.Lookup(hubID, skillID, digest)
	if !ok {
		return CachedSkill{}, fmt.Errorf("the cached skill %s/%s disappeared after it was placed", skillID, digest)
	}
	return cached, nil
}

// Versions lists the complete cached versions of a skill, most recently used
// first.
func (cache RemoteCache) Versions(hubID, skillID string) []CachedSkill {
	if !hubIDPattern.MatchString(hubID) || !bundleSkillIDPattern.MatchString(skillID) {
		return nil
	}
	directory := filepath.Join(cache.Root, hubID, skillID)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil
	}
	var versions []CachedSkill
	for _, entry := range entries {
		if !entry.IsDir() || !ValidBundleDigest(entry.Name()) {
			continue
		}
		if cached, err := readCachedSkill(filepath.Join(directory, entry.Name()), skillID, entry.Name()); err == nil {
			versions = append(versions, cached)
		}
	}
	sort.SliceStable(versions, func(i, j int) bool { return versions[i].LastUsed.After(versions[j].LastUsed) })
	return versions
}

// Latest returns the most recently used cached version of a skill.
func (cache RemoteCache) Latest(hubID, skillID string) (CachedSkill, bool) {
	versions := cache.Versions(hubID, skillID)
	if len(versions) == 0 {
		return CachedSkill{}, false
	}
	return versions[0], true
}

// Forget removes every cached version of one skill of a hub, for a skill the
// hub no longer serves. The skill's writable state and config directories are
// kept, as Clear keeps them.
func (cache RemoteCache) Forget(hubID, skillID string) error {
	if cache.Root == "" || !hubIDPattern.MatchString(hubID) || !bundleSkillIDPattern.MatchString(skillID) {
		return fmt.Errorf("invalid cache key %s/%s", hubID, skillID)
	}
	directory := filepath.Join(cache.HubDir(hubID), skillID)
	removeTree(directory)
	if _, err := os.Lstat(directory); !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("could not remove %s", directory)
	}
	return nil
}

// GCPolicy bounds the cache of one hub.
type GCPolicy struct {
	// KeepVersions is how many versions of each skill are kept.
	KeepVersions int
	// MaxAge removes a version not used for this long.
	MaxAge time.Duration
	// MaxBytes caps the total size of the hub's cached skills; the least
	// recently used versions are removed first.
	MaxBytes int64
}

// DefaultGCPolicy keeps three versions of each skill, drops versions unused
// for 30 days, and caps one hub's cache at 512 MiB.
var DefaultGCPolicy = GCPolicy{KeepVersions: 3, MaxAge: 30 * 24 * time.Hour, MaxBytes: 512 << 20}

// CollectGarbage applies policy to one hub's cache and returns how many
// versions it removed. The version keep (skill and digest) is never removed,
// so a skill just fetched stays available. Staging directories older than an
// hour, which only a crash leaves behind, are removed too.
func (cache RemoteCache) CollectGarbage(hubID string, policy GCPolicy, now time.Time, keepSkill, keepDigest string) (int, error) {
	if !hubIDPattern.MatchString(hubID) {
		return 0, fmt.Errorf("invalid hub id %q", hubID)
	}
	hubDir := cache.HubDir(hubID)
	removeOldStaging(filepath.Join(hubDir, cacheStagingName), now)
	skills, err := cache.hubSkills(hubID)
	if err != nil {
		return 0, err
	}
	removed := 0
	var survivors []CachedSkill
	for _, versions := range skills {
		for index, version := range versions {
			protected := version.Metadata.SkillID == keepSkill && version.Digest == keepDigest
			tooMany := policy.KeepVersions > 0 && index >= policy.KeepVersions
			tooOld := policy.MaxAge > 0 && now.Sub(version.LastUsed) > policy.MaxAge
			if !protected && (tooMany || tooOld) {
				removeTree(version.Directory)
				removed++
				continue
			}
			survivors = append(survivors, version)
		}
	}
	if policy.MaxBytes > 0 {
		var total int64
		for _, version := range survivors {
			total += version.Size()
		}
		sort.SliceStable(survivors, func(i, j int) bool { return survivors[i].LastUsed.Before(survivors[j].LastUsed) })
		for _, version := range survivors {
			if total <= policy.MaxBytes {
				break
			}
			if version.Metadata.SkillID == keepSkill && version.Digest == keepDigest {
				continue
			}
			removeTree(version.Directory)
			total -= version.Size()
			removed++
		}
	}
	removeEmptySkillDirs(hubDir)
	return removed, nil
}

// hubSkills lists the cached versions of every skill of a hub, each list most
// recently used first.
func (cache RemoteCache) hubSkills(hubID string) (map[string][]CachedSkill, error) {
	entries, err := os.ReadDir(cache.HubDir(hubID))
	if errors.Is(err, fs.ErrNotExist) {
		return map[string][]CachedSkill{}, nil
	}
	if err != nil {
		return nil, err
	}
	skills := make(map[string][]CachedSkill)
	for _, entry := range entries {
		if !entry.IsDir() || !bundleSkillIDPattern.MatchString(entry.Name()) {
			continue
		}
		if versions := cache.Versions(hubID, entry.Name()); len(versions) > 0 {
			skills[entry.Name()] = versions
		}
	}
	return skills, nil
}

func removeOldStaging(stagingRoot string, now time.Time) {
	entries, err := os.ReadDir(stagingRoot)
	if err != nil {
		return
	}
	for _, entry := range entries {
		info, err := entry.Info()
		if err == nil && now.Sub(info.ModTime()) > cacheStagingMaxAge {
			removeTree(filepath.Join(stagingRoot, entry.Name()))
		}
	}
}

func removeEmptySkillDirs(hubDir string) {
	entries, err := os.ReadDir(hubDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() && bundleSkillIDPattern.MatchString(entry.Name()) {
			// Remove fails on a directory that still holds versions.
			_ = os.Remove(filepath.Join(hubDir, entry.Name()))
		}
	}
}

// HubUsage summarizes one hub's cached skills.
type HubUsage struct {
	HubID    string
	HubURL   string
	Skills   int
	Versions int
	Bytes    int64
}

// Usage summarizes the cache of every hub, or of hubID alone when it is not
// empty. A hub with nothing cached is omitted.
func (cache RemoteCache) Usage(hubID string) ([]HubUsage, error) {
	hubIDs, err := cache.hubIDs(hubID)
	if err != nil {
		return nil, err
	}
	var usage []HubUsage
	for _, id := range hubIDs {
		skills, err := cache.hubSkills(id)
		if err != nil {
			return nil, err
		}
		item := HubUsage{HubID: id, HubURL: cache.readHubURL(id), Skills: len(skills)}
		for _, versions := range skills {
			item.Versions += len(versions)
			for _, version := range versions {
				item.Bytes += version.Size()
			}
		}
		if item.Versions > 0 {
			usage = append(usage, item)
		}
	}
	return usage, nil
}

// Clear removes every cached skill version of every hub, or of hubID alone,
// together with leftover staging directories. The writable state and config
// directories of the skills are kept.
func (cache RemoteCache) Clear(hubID string) ([]HubUsage, error) {
	usage, err := cache.Usage(hubID)
	if err != nil {
		return nil, err
	}
	hubIDs, err := cache.hubIDs(hubID)
	if err != nil {
		return nil, err
	}
	for _, id := range hubIDs {
		hubDir := cache.HubDir(id)
		entries, err := os.ReadDir(hubDir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() && (bundleSkillIDPattern.MatchString(entry.Name()) || entry.Name() == cacheStagingName) {
				removeTree(filepath.Join(hubDir, entry.Name()))
			}
		}
		for _, entry := range entries {
			name := filepath.Join(hubDir, entry.Name())
			if entry.IsDir() && bundleSkillIDPattern.MatchString(entry.Name()) {
				if _, statErr := os.Lstat(name); statErr == nil {
					return nil, fmt.Errorf("could not remove %s", name)
				}
			}
		}
	}
	return usage, nil
}

func (cache RemoteCache) hubIDs(only string) ([]string, error) {
	if only != "" {
		if !hubIDPattern.MatchString(only) {
			return nil, fmt.Errorf("invalid hub id %q", only)
		}
		return []string{only}, nil
	}
	entries, err := os.ReadDir(cache.Root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, entry := range entries {
		if entry.IsDir() && hubIDPattern.MatchString(entry.Name()) {
			ids = append(ids, entry.Name())
		}
	}
	return ids, nil
}

// RecordHubURL remembers the base URL of a hub next to its cache, so the
// cache commands can name the hub instead of its directory.
func (cache RemoteCache) RecordHubURL(hubID, hubURL string) error {
	if !hubIDPattern.MatchString(hubID) {
		return fmt.Errorf("invalid hub id %q", hubID)
	}
	hubDir := cache.HubDir(hubID)
	if err := os.MkdirAll(hubDir, 0o700); err != nil {
		return err
	}
	encoded, err := json.Marshal(struct {
		URL string `json:"url"`
	}{URL: hubURL})
	if err != nil {
		return err
	}
	file := filepath.Join(hubDir, cacheHubFileName)
	if existing, err := os.ReadFile(file); err == nil && bytes.Equal(existing, encoded) {
		return nil
	}
	return os.WriteFile(file, encoded, 0o600)
}

func (cache RemoteCache) readHubURL(hubID string) string {
	data, err := os.ReadFile(filepath.Join(cache.HubDir(hubID), cacheHubFileName))
	if err != nil || len(data) > 4096 {
		return ""
	}
	var record struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(data, &record) != nil {
		return ""
	}
	return record.URL
}

// removeTree deletes a cached version or staging tree. Entries are made
// writable first, because cached files are read-only and Windows refuses to
// delete read-only files. Symlinks are removed, never followed.
func removeTree(directory string) {
	_ = filepath.WalkDir(directory, func(current string, entry fs.DirEntry, err error) error {
		if err != nil || entry == nil {
			return nil
		}
		if entry.Type().IsRegular() {
			_ = os.Chmod(current, 0o600)
		} else if entry.IsDir() {
			_ = os.Chmod(current, 0o700)
		}
		return nil
	})
	_ = os.RemoveAll(directory)
}

func randomSuffix() string {
	var value [8]byte
	_, _ = rand.Read(value[:])
	return hex.EncodeToString(value[:])
}
