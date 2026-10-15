package skillruntime

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// testBundle builds a bundle of files (path to contents) for skill id.
func testBundle(t *testing.T, id string, files map[string]string) (BundleMetadata, map[string][]byte, string) {
	t.Helper()
	metadata := BundleMetadata{Schema: BundleSchema, SkillID: id, DepsKey: "0123456789abcdef", Detail: json.RawMessage(`{"skill_id":"` + id + `"}`)}
	contents := map[string][]byte{}
	for path, text := range files {
		sum := sha256.Sum256([]byte(text))
		metadata.Files = append(metadata.Files, BundleFile{Path: path, Digest: "sha256:" + hex.EncodeToString(sum[:]), Size: int64(len(text))})
		contents[path] = []byte(text)
	}
	digest, err := ManifestDigest(metadata.Files)
	if err != nil {
		t.Fatal(err)
	}
	metadata.ManifestVersion = "sha256:" + digest
	return metadata, contents, digest
}

func writeTestBundle(t *testing.T, metadata BundleMetadata, contents map[string][]byte) []byte {
	t.Helper()
	var archive bytes.Buffer
	if err := WriteBundle(&archive, metadata, contents); err != nil {
		t.Fatal(err)
	}
	return archive.Bytes()
}

var sampleSkill = map[string]string{
	"SKILL.md":              "---\nname: sample\ndescription: Sample.\n---\n\n# Sample\n",
	"references/guide.md":   "# Guide\n",
	"scripts/check.sh":      "#!/bin/sh\necho ok\n",
	"deep/nested/data.json": "{}\n",
}

// directoryDigest hashes every file of a cached version except the metadata
// marker, so an extra or missing file changes the result.
func directoryDigest(t *testing.T, directory string) string {
	t.Helper()
	var files []BundleFile
	err := filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, _ := filepath.Rel(directory, path)
		relative = filepath.ToSlash(relative)
		if relative == BundleMetadataName {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		files = append(files, BundleFile{Path: relative, Digest: "sha256:" + hex.EncodeToString(sum[:]), Size: int64(len(data))})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	digest, err := ManifestDigest(files)
	if err != nil {
		t.Fatal(err)
	}
	return digest
}

func TestBundleIsDeterministicAndRoundTrips(t *testing.T) {
	metadata, contents, digest := testBundle(t, "sample", sampleSkill)
	first := writeTestBundle(t, metadata, contents)
	reordered := metadata
	reordered.Files = append([]BundleFile(nil), metadata.Files...)
	sort.Slice(reordered.Files, func(i, j int) bool { return reordered.Files[i].Path > reordered.Files[j].Path })
	if second := writeTestBundle(t, reordered, contents); !bytes.Equal(first, second) {
		t.Fatal("the same skill version produced different archives")
	}

	cache := RemoteCache{Root: t.TempDir()}
	cached, err := cache.Install("hub-1", "sample", digest, bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	if got := directoryDigest(t, cached.Directory); got != digest {
		t.Fatalf("extracted folder digest = %s, want %s", got, digest)
	}
	for path, text := range sampleSkill {
		data, err := os.ReadFile(filepath.Join(cached.Directory, filepath.FromSlash(path)))
		if err != nil || string(data) != text {
			t.Fatalf("%s = %q, %v", path, data, err)
		}
	}
	if string(cached.Metadata.Detail) != string(metadata.Detail) || cached.Metadata.DepsKey != metadata.DepsKey {
		t.Fatalf("metadata = %+v", cached.Metadata)
	}
	if again, ok := cache.Lookup("hub-1", "sample", digest); !ok || again.Directory != cached.Directory {
		t.Fatalf("lookup after install = %+v, %v", again, ok)
	}
	if _, ok := cache.Lookup("hub-1", "sample", strings.Repeat("0", 64)); ok {
		t.Fatal("lookup of a version never fetched hit")
	}
}

func TestInstallRejectsTamperedArchives(t *testing.T) {
	metadata, contents, digest := testBundle(t, "sample", sampleSkill)
	tamperedContents := map[string][]byte{}
	for path, data := range contents {
		tamperedContents[path] = append([]byte(nil), data...)
	}
	tamperedContents["scripts/check.sh"] = []byte("#!/bin/sh\nrm -rf /\n")[:len(contents["scripts/check.sh"])]

	// WriteBundle checks sizes only, so a file whose bytes changed but whose
	// size did not still makes an archive that a client must refuse.
	tamperedFile := writeTestBundle(t, metadata, tamperedContents)

	listed := metadata
	listed.Files = append([]BundleFile(nil), metadata.Files...)
	listed.Files[0].Size++
	listedContents := map[string][]byte{}
	for path, data := range contents {
		listedContents[path] = data
	}
	listedContents[listed.Files[0].Path] = append(append([]byte(nil), contents[listed.Files[0].Path]...), 'x')

	flipped := writeTestBundle(t, metadata, contents)
	flipped[len(flipped)/2] ^= 0xff

	cases := map[string]struct {
		archive []byte
		digest  string
		want    error
	}{
		"file bytes changed":       {tamperedFile, digest, ErrBundleDigestMismatch},
		"file list changed":        {writeTestBundle(t, listed, listedContents), digest, ErrBundleDigestMismatch},
		"requested another digest": {writeTestBundle(t, metadata, contents), strings.Repeat("a", 64), ErrBundleDigestMismatch},
		"corrupted in transit":     {flipped, digest, ErrBundleInvalid},
		"truncated":                {writeTestBundle(t, metadata, contents)[:200], digest, ErrBundleInvalid},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			cache := RemoteCache{Root: t.TempDir()}
			_, err := cache.Install("hub-1", "sample", test.digest, bytes.NewReader(test.archive))
			// A corrupted archive may fail either check, depending on
			// which byte changed.
			accepted := errors.Is(err, test.want) || (test.want == ErrBundleInvalid && errors.Is(err, ErrBundleDigestMismatch))
			if !accepted {
				t.Fatalf("install error = %v, want %v", err, test.want)
			}
			if _, ok := cache.Lookup("hub-1", "sample", test.digest); ok {
				t.Fatal("a rejected archive was cached")
			}
			assertNoSkillLeftovers(t, cache, "hub-1")
		})
	}
}

// An archive cut short at any point, as when the hub stops in the middle of a
// download, is refused as an invalid bundle and never reported as a failure
// to store the skill on this machine.
func TestInstallRefusesAnArchiveCutShortAnywhere(t *testing.T) {
	var large strings.Builder
	state := uint32(1)
	for large.Len() < 48<<10 {
		// A simple generator keeps the text from compressing away, so many
		// cuts land inside a file's contents.
		state = state*1664525 + 1013904223
		large.WriteByte(byte('a' + state>>24%26))
	}
	files := map[string]string{"SKILL.md": sampleSkill["SKILL.md"], "references/large.md": large.String()}
	metadata, contents, digest := testBundle(t, "sample", files)
	archive := writeTestBundle(t, metadata, contents)
	for cut := 1; cut < len(archive); cut += 101 {
		cache := RemoteCache{Root: t.TempDir()}
		cached, err := cache.Install("hub-1", "sample", digest, bytes.NewReader(archive[:cut]))
		if err == nil {
			// Only the archive's closing padding and checksum were lost; every
			// file was read in full and matched its digest.
			if got := directoryDigest(t, cached.Directory); got != digest {
				t.Fatalf("archive cut at %d of %d bytes was installed with digest %s, want %s", cut, len(archive), got, digest)
			}
			continue
		}
		if !errors.Is(err, ErrBundleInvalid) && !errors.Is(err, ErrBundleDigestMismatch) {
			t.Fatalf("archive cut at %d of %d bytes: error %v, want an invalid bundle", cut, len(archive), err)
		}
		assertNoSkillLeftovers(t, cache, "hub-1")
	}
}

// assertNoSkillLeftovers checks that no version directory and no staging
// directory remains for a hub.
func assertNoSkillLeftovers(t *testing.T, cache RemoteCache, hubID string) {
	t.Helper()
	staging, _ := os.ReadDir(filepath.Join(cache.HubDir(hubID), cacheStagingName))
	if len(staging) != 0 {
		t.Fatalf("staging directories left behind: %v", staging)
	}
	versions, _ := os.ReadDir(filepath.Join(cache.HubDir(hubID), "sample"))
	if len(versions) != 0 {
		t.Fatalf("version directories left behind: %v", versions)
	}
}

func TestInstallRejectsUnsafeArchiveEntries(t *testing.T) {
	metadata, contents, digest := testBundle(t, "sample", map[string]string{"SKILL.md": "# x\n"})
	build := func(extra func(*tar.Writer)) []byte {
		var buffer bytes.Buffer
		compressor := gzip.NewWriter(&buffer)
		archive := tar.NewWriter(compressor)
		encoded, _ := json.Marshal(metadata)
		_ = archive.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: BundleMetadataName, Mode: 0o644, Size: int64(len(encoded))})
		_, _ = archive.Write(encoded)
		_ = archive.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "SKILL.md", Mode: 0o644, Size: int64(len(contents["SKILL.md"]))})
		_, _ = archive.Write(contents["SKILL.md"])
		extra(archive)
		_ = archive.Close()
		_ = compressor.Close()
		return buffer.Bytes()
	}
	cases := map[string][]byte{
		"unlisted file": build(func(archive *tar.Writer) {
			_ = archive.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "../escape.sh", Mode: 0o644, Size: 1})
			_, _ = archive.Write([]byte("x"))
		}),
		"symlink": build(func(archive *tar.Writer) {
			_ = archive.WriteHeader(&tar.Header{Typeflag: tar.TypeSymlink, Name: "link", Linkname: "/etc/passwd"})
		}),
		"duplicate": build(func(archive *tar.Writer) {
			_ = archive.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: "SKILL.md", Mode: 0o644, Size: int64(len(contents["SKILL.md"]))})
			_, _ = archive.Write(contents["SKILL.md"])
		}),
	}
	for name, archive := range cases {
		t.Run(name, func(t *testing.T) {
			cache := RemoteCache{Root: t.TempDir()}
			if _, err := cache.Install("hub-1", "sample", digest, bytes.NewReader(archive)); !errors.Is(err, ErrBundleInvalid) {
				t.Fatalf("install error = %v, want ErrBundleInvalid", err)
			}
			if _, err := os.Lstat(filepath.Join(cache.Root, "escape.sh")); err == nil {
				t.Fatal("an entry escaped the cache")
			}
		})
	}

	unsafe := metadata
	unsafe.Files = append([]BundleFile(nil), metadata.Files...)
	unsafe.Files = append(unsafe.Files, BundleFile{Path: "../x", Digest: metadata.Files[0].Digest, Size: 1})
	if err := WriteBundle(io.Discard, unsafe, contents); !errors.Is(err, ErrBundleInvalid) {
		t.Fatalf("write with an unsafe path = %v", err)
	}
}

func TestInstallCrashLeavesNoPartialSkill(t *testing.T) {
	metadata, contents, digest := testBundle(t, "sample", sampleSkill)
	archive := writeTestBundle(t, metadata, contents)
	cache := RemoteCache{Root: t.TempDir()}

	// At the last moment before the version is placed, every file is
	// written. A crash there must leave nothing a lookup accepts. A real crash
	// skips the cleanup that a panic runs, so the hook keeps a copy of the
	// staging directory, standing in for what the crash leaves on disk.
	leftover := filepath.Join(cache.HubDir("hub-1"), cacheStagingName, "fetch-crashed")
	hookRan := false
	installHook = func(staging string) {
		hookRan = true
		if _, ok := cache.Lookup("hub-1", "sample", digest); ok {
			t.Error("a version is visible before it is placed")
		}
		if err := os.CopyFS(leftover, os.DirFS(staging)); err != nil {
			t.Errorf("keep the crash leftover: %v", err)
		}
		panic("simulated crash before the version is placed")
	}
	func() {
		defer func() { _ = recover() }()
		_, _ = cache.Install("hub-1", "sample", digest, bytes.NewReader(archive))
	}()
	installHook = nil

	if !hookRan {
		t.Fatal("the crash hook did not run")
	}
	if _, ok := cache.Lookup("hub-1", "sample", digest); ok {
		t.Fatal("a crashed install is visible as a cached skill")
	}
	if versions := cache.Versions("hub-1", "sample"); len(versions) != 0 {
		t.Fatalf("versions after a crash = %v", versions)
	}
	// The staging directory a crash leaves behind is only garbage: an hour
	// later collection removes it, and a new install succeeds.
	stagedAt := leftover
	if _, err := os.Stat(stagedAt); err != nil {
		t.Fatalf("the crash leftover is missing: %v", err)
	}
	if _, err := cache.CollectGarbage("hub-1", DefaultGCPolicy, time.Now().Add(2*time.Hour), "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stagedAt); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("staging directory after collection: %v", err)
	}
	if _, err := cache.Install("hub-1", "sample", digest, bytes.NewReader(archive)); err != nil {
		t.Fatalf("install after a crash: %v", err)
	}
}

func TestLookupRefusesIncompleteVersions(t *testing.T) {
	metadata, contents, digest := testBundle(t, "sample", sampleSkill)
	cache := RemoteCache{Root: t.TempDir()}
	cached, err := cache.Install("hub-1", "sample", digest, bytes.NewReader(writeTestBundle(t, metadata, contents)))
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(cached.Directory, "references", "guide.md")
	if err := os.Chmod(target, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if _, ok := cache.Lookup("hub-1", "sample", digest); ok {
		t.Fatal("a version with a missing file counted as cached")
	}
	// Installing again replaces the damaged copy.
	if _, err := cache.Install("hub-1", "sample", digest, bytes.NewReader(writeTestBundle(t, metadata, contents))); err != nil {
		t.Fatalf("reinstall over a damaged copy: %v", err)
	}
	if _, ok := cache.Lookup("hub-1", "sample", digest); !ok {
		t.Fatal("reinstalled version not found")
	}
}

func TestConcurrentInstallsOfOneVersion(t *testing.T) {
	metadata, contents, digest := testBundle(t, "sample", sampleSkill)
	archive := writeTestBundle(t, metadata, contents)
	cache := RemoteCache{Root: t.TempDir()}
	var group sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := cache.Install("hub-1", "sample", digest, bytes.NewReader(archive))
			errs <- err
		}()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent install: %v", err)
		}
	}
	if versions := cache.Versions("hub-1", "sample"); len(versions) != 1 {
		t.Fatalf("versions = %d, want 1", len(versions))
	}
	staging, _ := os.ReadDir(filepath.Join(cache.HubDir("hub-1"), cacheStagingName))
	if len(staging) != 0 {
		t.Fatalf("staging left behind: %v", staging)
	}
}

func TestCollectGarbageKeepsRecentVersionsWithinBudget(t *testing.T) {
	cache := RemoteCache{Root: t.TempDir()}
	now := time.Now()
	install := func(skillID, body string, lastUsed time.Time) string {
		metadata, contents, digest := testBundle(t, skillID, map[string]string{"SKILL.md": body})
		cached, err := cache.Install("hub-1", skillID, digest, bytes.NewReader(writeTestBundle(t, metadata, contents)))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(cached.Directory, lastUsed, lastUsed); err != nil {
			t.Fatal(err)
		}
		return digest
	}
	var alpha []string
	for index := range 5 {
		alpha = append(alpha, install("alpha", strings.Repeat("a", 100+index), now.Add(-time.Duration(5-index)*time.Hour)))
	}
	stale := install("beta", "old", now.Add(-60*24*time.Hour))
	big := install("gamma", strings.Repeat("g", 4000), now.Add(-10*time.Hour))

	removed, err := cache.CollectGarbage("hub-1", GCPolicy{KeepVersions: 3, MaxAge: 30 * 24 * time.Hour, MaxBytes: 3000}, now, "beta", stale)
	if err != nil {
		t.Fatal(err)
	}
	digests := func(skillID string) []string {
		var out []string
		for _, version := range cache.Versions("hub-1", skillID) {
			out = append(out, version.Digest)
		}
		return out
	}
	if got := digests("alpha"); len(got) != 3 || got[0] != alpha[4] || got[2] != alpha[2] {
		t.Fatalf("alpha versions = %v, want the three most recent", got)
	}
	if got := digests("beta"); len(got) != 1 || got[0] != stale {
		t.Fatalf("the protected version was removed: %v", got)
	}
	if got := digests("gamma"); len(got) != 0 {
		t.Fatalf("the size cap kept %v (digest %s)", got, big)
	}
	if removed != 3 {
		t.Fatalf("removed = %d, want 3 (two old alpha versions, gamma over budget)", removed)
	}
	if _, err := os.Stat(filepath.Join(cache.HubDir("hub-1"), "gamma")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("empty skill directory kept: %v", err)
	}
}

func TestForgetRemovesOneSkillAndKeepsItsState(t *testing.T) {
	cache := RemoteCache{Root: t.TempDir()}
	for _, skillID := range []string{"sample", "other"} {
		metadata, contents, digest := testBundle(t, skillID, sampleSkill)
		if _, err := cache.Install("hub-1", skillID, digest, bytes.NewReader(writeTestBundle(t, metadata, contents))); err != nil {
			t.Fatal(err)
		}
	}
	state := filepath.Join(cache.StateRoot("hub-1"), "runtime", "config", "sample")
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := cache.Forget("hub-1", "sample"); err != nil {
		t.Fatal(err)
	}
	if versions := cache.Versions("hub-1", "sample"); len(versions) != 0 {
		t.Fatalf("sample still has %d cached versions", len(versions))
	}
	if versions := cache.Versions("hub-1", "other"); len(versions) != 1 {
		t.Fatal("forgetting sample touched another skill")
	}
	if _, err := os.Stat(state); err != nil {
		t.Fatalf("the skill's state was removed: %v", err)
	}
	if err := cache.Forget("hub-1", "sample"); err != nil {
		t.Fatalf("forgetting a skill that is not cached: %v", err)
	}
	for _, skillID := range []string{".state", "../hub-2", ""} {
		if err := cache.Forget("hub-1", skillID); err == nil {
			t.Errorf("Forget accepted the skill id %q", skillID)
		}
	}
}

func TestClearRemovesOnlyFetchedSkills(t *testing.T) {
	cache := RemoteCache{Root: t.TempDir()}
	for _, hubID := range []string{"hub-1", "hub-2"} {
		metadata, contents, digest := testBundle(t, "sample", sampleSkill)
		if _, err := cache.Install(hubID, "sample", digest, bytes.NewReader(writeTestBundle(t, metadata, contents))); err != nil {
			t.Fatal(err)
		}
		if err := cache.RecordHubURL(hubID, "http://"+hubID+":7420"); err != nil {
			t.Fatal(err)
		}
	}
	state := filepath.Join(cache.StateRoot("hub-1"), "runtime", "config", "sample")
	if err := os.MkdirAll(state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(state, "env"), []byte("TOKEN=secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	usage, err := cache.Usage("")
	if err != nil || len(usage) != 2 || usage[0].HubURL != "http://hub-1:7420" || usage[0].Versions != 1 || usage[0].Bytes == 0 {
		t.Fatalf("usage = %+v, %v", usage, err)
	}
	cleared, err := cache.Clear("hub-1")
	if err != nil || len(cleared) != 1 || cleared[0].HubID != "hub-1" {
		t.Fatalf("clear = %+v, %v", cleared, err)
	}
	if versions := cache.Versions("hub-1", "sample"); len(versions) != 0 {
		t.Fatalf("hub-1 still caches %d versions", len(versions))
	}
	if versions := cache.Versions("hub-2", "sample"); len(versions) != 1 {
		t.Fatal("clearing hub-1 touched hub-2")
	}
	if _, err := os.Stat(filepath.Join(state, "env")); err != nil {
		t.Fatalf("stored secrets were removed: %v", err)
	}
	if _, err := cache.Clear(""); err != nil {
		t.Fatal(err)
	}
	if usage, _ := cache.Usage(""); len(usage) != 0 {
		t.Fatalf("usage after clearing everything = %+v", usage)
	}
}

func TestHubIDIsStableAndSafeOnEveryFileSystem(t *testing.T) {
	first, err := HubID("http://127.0.0.1:7420")
	if err != nil {
		t.Fatal(err)
	}
	for _, same := range []string{"http://127.0.0.1:7420/", "HTTP://127.0.0.1:7420"} {
		if got, _ := HubID(same); got != first {
			t.Fatalf("HubID(%q) = %s, want %s", same, got, first)
		}
	}
	other, _ := HubID("http://127.0.0.1:7421")
	ipv6, _ := HubID("http://[::1]:7420")
	if other == first || !hubIDPattern.MatchString(first) || !hubIDPattern.MatchString(ipv6) {
		t.Fatalf("hub ids %q %q %q", first, other, ipv6)
	}
	if strings.ContainsAny(first+ipv6, `:[]/\`) {
		t.Fatalf("hub id holds a character some file systems refuse: %q %q", first, ipv6)
	}
	if _, err := HubID("ftp://host"); err == nil {
		t.Fatal("a non-HTTP URL was accepted")
	}
}

func TestClientStateDirFollowsXDGStateHome(t *testing.T) {
	base := t.TempDir()
	t.Setenv("XDG_STATE_HOME", base)
	got, err := ClientStateDir()
	if err != nil || got != filepath.Join(base, "skillhub") {
		t.Fatalf("ClientStateDir = %q, %v", got, err)
	}
	if RemoteCacheDir(got) != filepath.Join(base, "skillhub", "remote") {
		t.Fatalf("RemoteCacheDir = %q", RemoteCacheDir(got))
	}
}
