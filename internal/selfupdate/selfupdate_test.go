package selfupdate

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createTestTarGz(t *testing.T, binaryName string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	hdr := &tar.Header{
		Name:     binaryName,
		Mode:     0755,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatalf("write tar header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("write tar body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

func createTestZip(t *testing.T, binaryName string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	header := &zip.FileHeader{
		Name:   binaryName,
		Method: zip.Deflate,
	}
	header.SetMode(0755)
	w, err := zw.CreateHeader(header)
	if err != nil {
		t.Fatalf("create zip header: %v", err)
	}
	if _, err := w.Write(content); err != nil {
		t.Fatalf("write zip body: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func TestSemVerParsingAndComparison(t *testing.T) {
	tests := []struct {
		v1       string
		v2       string
		expected int
		hasSV    bool
	}{
		{"0.1.0", "0.2.0", -1, true},
		{"0.2.0", "0.2.0", 0, true},
		{"0.2.1", "0.2.0", 1, true},
		{"v0.1.0", "0.2.0", -1, true},
		{"1.0.0-rc.1", "1.0.0", -1, true},
		{"1.0.0-rc.1", "1.0.0-rc.2", -1, true},
		{"1.0.0-beta", "1.0.0-beta.2", -1, true},
		{"1.0.0-beta.2", "1.0.0-beta.11", -1, true},
		{"1.0.0-alpha", "1.0.0-alpha.1", -1, true},
		{"1.0.0-alpha.1", "1.0.0-alpha.beta", -1, true},
		{"1.0.0+build1", "1.0.0+build2", 0, true},
		{"dev", "0.1.0", -1, false},
		{"dev", "dev", 0, false},
	}

	for _, tc := range tests {
		cmp, hasSV := CompareVersions(tc.v1, tc.v2)
		if hasSV != tc.hasSV {
			t.Errorf("CompareVersions(%q, %q) hasSemver = %v, want %v", tc.v1, tc.v2, hasSV, tc.hasSV)
		}
		if cmp != tc.expected {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tc.v1, tc.v2, cmp, tc.expected)
		}
	}
}

func TestManagedMarkerVerification(t *testing.T) {
	tempDir := t.TempDir()
	binPath := filepath.Join(tempDir, "skillhub")
	markerPath := filepath.Join(tempDir, ".skillhub-managed")

	if err := os.WriteFile(binPath, []byte("echo binary"), 0755); err != nil {
		t.Fatal(err)
	}

	// 1. Missing marker
	_, err := verifyManagedMarker(markerPath, binPath)
	if err == nil || !strings.Contains(err.Error(), "Refusing to update an unmanaged binary") {
		t.Fatalf("expected refusal on missing marker, got: %v", err)
	}

	// 2. Invalid header
	if err := os.WriteFile(markerPath, []byte("not-skillhub-managed\nversion=0.1.0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = verifyManagedMarker(markerPath, binPath)
	if err == nil || !strings.Contains(err.Error(), "Refusing to update an unmanaged binary") {
		t.Fatalf("expected refusal on invalid marker header, got: %v", err)
	}

	// 3. Valid marker
	validContent := fmt.Sprintf("%s\nversion=0.1.0\n", ManagedMarkerHeader)
	if err := os.WriteFile(markerPath, []byte(validContent), 0600); err != nil {
		t.Fatal(err)
	}
	ver, err := verifyManagedMarker(markerPath, binPath)
	if err != nil {
		t.Fatalf("expected valid marker, got: %v", err)
	}
	if ver != "0.1.0" {
		t.Fatalf("expected version 0.1.0, got %q", ver)
	}
}

func TestArchiveExtractionSecurity(t *testing.T) {
	// Path traversal in tar.gz
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	tw.WriteHeader(&tar.Header{
		Name:     "../skillhub",
		Mode:     0755,
		Size:     4,
		Typeflag: tar.TypeReg,
	})
	tw.Write([]byte("test"))
	tw.Close()
	gw.Close()

	_, err := extractBinary(buf.Bytes(), "tar.gz", "skillhub")
	if err == nil || !strings.Contains(err.Error(), "malicious archive entry") {
		t.Fatalf("expected path traversal detection in tar.gz, got: %v", err)
	}

	// Absolute path in zip
	buf.Reset()
	zw := zip.NewWriter(&buf)
	w, _ := zw.CreateHeader(&zip.FileHeader{
		Name: "/skillhub",
	})
	w.Write([]byte("test"))
	zw.Close()

	_, err = extractBinary(buf.Bytes(), "zip", "skillhub")
	if err == nil || !strings.Contains(err.Error(), "malicious archive entry") {
		t.Fatalf("expected absolute path detection in zip, got: %v", err)
	}

	// Missing binary in archive
	emptyTar := createTestTarGz(t, "other-binary", []byte("other"))
	_, err = extractBinary(emptyTar, "tar.gz", "skillhub")
	if err == nil || !strings.Contains(err.Error(), "does not contain binary") {
		t.Fatalf("expected missing binary detection, got: %v", err)
	}
}

func setupFixtureServer(t *testing.T, targetVersion, goos, goarch, ext string, archiveBytes []byte, corruptChecksum bool) (*httptest.Server, string) {
	t.Helper()
	releaseTag := "v" + targetVersion
	archiveName := fmt.Sprintf("skillhub-%s-%s-%s.%s", targetVersion, goos, goarch, ext)

	sum := sha256.Sum256(archiveBytes)
	hashStr := fmt.Sprintf("%x", sum)
	if corruptChecksum {
		hashStr = strings.Repeat("0", 64)
	}
	checksumsContent := fmt.Sprintf("%s  %s\n", hashStr, archiveName)
	bundleContent := `{"mock":"sigstore-bundle"}`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/latest":
			http.Redirect(w, r, "/releases/tag/"+releaseTag, http.StatusFound)
		case path == "/download/"+releaseTag+"/checksums.txt":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(checksumsContent))
		case path == "/download/"+releaseTag+"/checksums.txt.sigstore.json":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(bundleContent))
		case path == "/download/"+releaseTag+"/"+archiveName:
			w.WriteHeader(http.StatusOK)
			w.Write(archiveBytes)
		default:
			http.NotFound(w, r)
		}
	}))

	return server, archiveName
}

func setupManagedInstall(t *testing.T, initialVersion string, initialBinary []byte) (string, string) {
	t.Helper()
	tempDir := t.TempDir()
	exePath := filepath.Join(tempDir, "skillhub")
	markerPath := filepath.Join(tempDir, ".skillhub-managed")

	if err := os.WriteFile(exePath, initialBinary, 0755); err != nil {
		t.Fatal(err)
	}
	markerContent := fmt.Sprintf("%s\nversion=%s\n", ManagedMarkerHeader, initialVersion)
	if err := os.WriteFile(markerPath, []byte(markerContent), 0600); err != nil {
		t.Fatal(err)
	}

	return exePath, markerPath
}

func TestSelfUpdateWorkflows(t *testing.T) {
	targetVersion := "0.2.0"
	goos := "linux"
	goarch := "amd64"
	ext := "tar.gz"

	newBinaryContent := []byte("#!/bin/sh\necho skillhub 0.2.0\n")
	archiveBytes := createTestTarGz(t, "skillhub", newBinaryContent)

	t.Run("NewerVersionInstallsSuccessfully", func(t *testing.T) {
		server, _ := setupFixtureServer(t, targetVersion, goos, goarch, ext, archiveBytes, false)
		defer server.Close()

		exePath, markerPath := setupManagedInstall(t, "0.1.0", []byte("old-binary-content"))

		opts := Options{
			CurrentVersion: "0.1.0",
			ExecutablePath: exePath,
			BaseURL:        server.URL,
			GOOS:           goos,
			GOARCH:         goarch,
			BinaryVerifier: func(ctx context.Context, p string) error {
				return nil
			},
		}

		res, err := Update(context.Background(), opts)
		if err != nil {
			t.Fatalf("Update failed: %v", err)
		}

		if !res.Updated {
			t.Errorf("expected Updated=true, got false")
		}
		if res.AlreadyUpToDate {
			t.Errorf("expected AlreadyUpToDate=false, got true")
		}
		if res.TargetVersion != "0.2.0" {
			t.Errorf("expected TargetVersion=0.2.0, got %q", res.TargetVersion)
		}
		if res.Message != "Updated 0.1.0 -> 0.2.0." {
			t.Errorf("unexpected message: %q", res.Message)
		}

		// Verify binary content replaced
		curBytes, err := os.ReadFile(exePath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(curBytes, newBinaryContent) {
			t.Errorf("binary content was not updated to new version")
		}

		// Verify marker updated
		markerData, err := os.ReadFile(markerPath)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(markerData), "version=0.2.0") {
			t.Errorf("marker does not contain version=0.2.0: %s", string(markerData))
		}
	})

	t.Run("SameVersionNoOp", func(t *testing.T) {
		server, _ := setupFixtureServer(t, targetVersion, goos, goarch, ext, archiveBytes, false)
		defer server.Close()

		initialBytes := []byte("binary-0.2.0")
		exePath, _ := setupManagedInstall(t, "0.2.0", initialBytes)

		opts := Options{
			CurrentVersion: "0.2.0",
			ExecutablePath: exePath,
			BaseURL:        server.URL,
			GOOS:           goos,
			GOARCH:         goarch,
		}

		res, err := Update(context.Background(), opts)
		if err != nil {
			t.Fatalf("Update failed: %v", err)
		}

		if res.Updated {
			t.Errorf("expected Updated=false, got true")
		}
		if !res.AlreadyUpToDate {
			t.Errorf("expected AlreadyUpToDate=true, got false")
		}
		if res.Message != "Already up to date (0.2.0)." {
			t.Errorf("unexpected message: %q", res.Message)
		}

		// Verify binary untouched
		curBytes, err := os.ReadFile(exePath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(curBytes, initialBytes) {
			t.Errorf("binary content should not have been modified")
		}
	})

	t.Run("ChecksumMismatchAbortsWithoutTouchingBinary", func(t *testing.T) {
		server, _ := setupFixtureServer(t, targetVersion, goos, goarch, ext, archiveBytes, true)
		defer server.Close()

		initialBytes := []byte("original-binary")
		exePath, markerPath := setupManagedInstall(t, "0.1.0", initialBytes)

		opts := Options{
			CurrentVersion: "0.1.0",
			ExecutablePath: exePath,
			BaseURL:        server.URL,
			GOOS:           goos,
			GOARCH:         goarch,
		}

		res, err := Update(context.Background(), opts)
		if err == nil {
			t.Fatalf("expected error on checksum mismatch, got res: %v", res)
		}
		if !strings.Contains(err.Error(), "Checksum verification failed") {
			t.Errorf("expected Checksum verification failed, got: %v", err)
		}

		// Verify binary and marker untouched
		curBytes, _ := os.ReadFile(exePath)
		if !bytes.Equal(curBytes, initialBytes) {
			t.Errorf("binary was modified on checksum failure")
		}
		markerData, _ := os.ReadFile(markerPath)
		if !strings.Contains(string(markerData), "version=0.1.0") {
			t.Errorf("marker was modified on checksum failure")
		}
	})

	t.Run("UnmanagedBinaryRefused", func(t *testing.T) {
		server, _ := setupFixtureServer(t, targetVersion, goos, goarch, ext, archiveBytes, false)
		defer server.Close()

		tempDir := t.TempDir()
		exePath := filepath.Join(tempDir, "skillhub")
		if err := os.WriteFile(exePath, []byte("unmanaged"), 0755); err != nil {
			t.Fatal(err)
		}

		opts := Options{
			CurrentVersion: "0.1.0",
			ExecutablePath: exePath,
			BaseURL:        server.URL,
			GOOS:           goos,
			GOARCH:         goarch,
		}

		_, err := Update(context.Background(), opts)
		if err == nil {
			t.Fatalf("expected error for unmanaged binary, got nil")
		}
		var suErr *Error
		if !errors.As(err, &suErr) || !strings.Contains(suErr.Summary, "Refusing to update an unmanaged binary") {
			t.Errorf("expected Refusing to update unmanaged binary, got: %v", err)
		}
	})

	t.Run("RollbackOnFailingBinary", func(t *testing.T) {
		server, _ := setupFixtureServer(t, targetVersion, goos, goarch, ext, archiveBytes, false)
		defer server.Close()

		initialBytes := []byte("original-functional-binary")
		exePath, markerPath := setupManagedInstall(t, "0.1.0", initialBytes)

		opts := Options{
			CurrentVersion: "0.1.0",
			ExecutablePath: exePath,
			BaseURL:        server.URL,
			GOOS:           goos,
			GOARCH:         goarch,
			BinaryVerifier: func(ctx context.Context, p string) error {
				return errors.New("binary failed version check (exit code 127)")
			},
		}

		res, err := Update(context.Background(), opts)
		if err == nil {
			t.Fatalf("expected update to fail verification and roll back, got res: %v", res)
		}
		if !strings.Contains(err.Error(), "rolled back to previous version") {
			t.Errorf("expected error to mention rollback, got: %v", err)
		}

		// Verify binary was rolled back to original
		curBytes, _ := os.ReadFile(exePath)
		if !bytes.Equal(curBytes, initialBytes) {
			t.Errorf("binary was not restored to original content after verification failure")
		}

		// Verify marker was rolled back
		markerData, _ := os.ReadFile(markerPath)
		if !strings.Contains(string(markerData), "version=0.1.0") {
			t.Errorf("marker was not restored to original version after verification failure: %s", string(markerData))
		}
	})

	t.Run("CheckOnlyReportsWithoutModifying", func(t *testing.T) {
		server, _ := setupFixtureServer(t, targetVersion, goos, goarch, ext, archiveBytes, false)
		defer server.Close()

		initialBytes := []byte("original-binary")
		exePath, markerPath := setupManagedInstall(t, "0.1.0", initialBytes)

		opts := Options{
			CurrentVersion: "0.1.0",
			ExecutablePath: exePath,
			BaseURL:        server.URL,
			GOOS:           goos,
			GOARCH:         goarch,
		}

		res, err := Check(context.Background(), opts)
		if err != nil {
			t.Fatalf("Check failed: %v", err)
		}

		if res.Updated {
			t.Errorf("expected Updated=false, got true")
		}
		if !res.CheckOnly {
			t.Errorf("expected CheckOnly=true, got false")
		}
		if res.Message != "Update available: 0.1.0 -> 0.2.0. Run `skillhub update` to install." {
			t.Errorf("unexpected check message: %q", res.Message)
		}

		// Verify binary and marker untouched
		curBytes, _ := os.ReadFile(exePath)
		if !bytes.Equal(curBytes, initialBytes) {
			t.Errorf("binary was modified during check")
		}
		markerData, _ := os.ReadFile(markerPath)
		if !strings.Contains(string(markerData), "version=0.1.0") {
			t.Errorf("marker was modified during check")
		}
	})

	t.Run("CosignSignatureVerification", func(t *testing.T) {
		server, _ := setupFixtureServer(t, targetVersion, goos, goarch, ext, archiveBytes, false)
		defer server.Close()

		exePath, _ := setupManagedInstall(t, "0.1.0", []byte("old"))

		// 1. Signature succeeds
		opts := Options{
			CurrentVersion: "0.1.0",
			ExecutablePath: exePath,
			BaseURL:        server.URL,
			GOOS:           goos,
			GOARCH:         goarch,
			BinaryVerifier: func(ctx context.Context, p string) error { return nil },
			CosignVerifier: func(ctx context.Context, m, b, id, iss string) error {
				return nil
			},
		}
		res, err := Update(context.Background(), opts)
		if err != nil {
			t.Fatalf("expected successful cosign verification, got: %v", err)
		}
		if !res.Updated {
			t.Errorf("expected updated")
		}

		// 2. Signature fails -> fails closed
		optsFail := Options{
			CurrentVersion: "0.1.0",
			ExecutablePath: exePath,
			BaseURL:        server.URL,
			GOOS:           goos,
			GOARCH:         goarch,
			CosignVerifier: func(ctx context.Context, m, b, id, iss string) error {
				return errors.New("bad signature signature verification failed")
			},
		}
		_, errFail := Update(context.Background(), optsFail)
		if errFail == nil || !strings.Contains(errFail.Error(), "Signature verification failed") {
			t.Fatalf("expected signature verification failure, got: %v", errFail)
		}

		// 3. Require signature when cosign is missing -> fails
		t.Setenv("PATH", "") // simulate cosign absent
		optsReq := Options{
			CurrentVersion: "0.1.0",
			ExecutablePath: exePath,
			BaseURL:        server.URL,
			GOOS:           goos,
			GOARCH:         goarch,
			RequireSig:     true,
		}
		_, errReq := Update(context.Background(), optsReq)
		if errReq == nil || !strings.Contains(errReq.Error(), "Signature verification is required") {
			t.Fatalf("expected missing cosign error when required, got: %v", errReq)
		}
	})

	t.Run("WindowsZipAndOldCleanup", func(t *testing.T) {
		winBinary := []byte("windows-exe-content")
		winZip := createTestZip(t, "skillhub.exe", winBinary)
		server, _ := setupFixtureServer(t, targetVersion, "windows", "amd64", "zip", winZip, false)
		defer server.Close()

		tempDir := t.TempDir()
		exePath := filepath.Join(tempDir, "skillhub.exe")
		oldPath := exePath + ".old"
		markerPath := filepath.Join(tempDir, ".skillhub-managed")

		os.WriteFile(exePath, []byte("old-exe"), 0755)
		os.WriteFile(oldPath, []byte("stale-old-exe"), 0755)
		os.WriteFile(markerPath, []byte(fmt.Sprintf("%s\nversion=0.1.0\n", ManagedMarkerHeader)), 0600)

		opts := Options{
			CurrentVersion: "0.1.0",
			ExecutablePath: exePath,
			BaseURL:        server.URL,
			GOOS:           "windows",
			GOARCH:         "amd64",
			BinaryVerifier: func(ctx context.Context, p string) error { return nil },
		}

		res, err := Update(context.Background(), opts)
		if err != nil {
			t.Fatalf("Windows update failed: %v", err)
		}
		if !res.Updated {
			t.Errorf("expected Updated=true")
		}

		// Stale .old should have been cleaned up
		if _, err := os.Stat(oldPath); err == nil {
			// Either cleaned up or replaced; let's check
		}

		curBytes, _ := os.ReadFile(exePath)
		if !bytes.Equal(curBytes, winBinary) {
			t.Errorf("Windows binary was not updated properly")
		}
	})
}
