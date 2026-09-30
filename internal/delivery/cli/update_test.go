package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

func createCLITestTarGz(t *testing.T, binaryName string, content []byte) []byte {
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

func createCLITestArchive(t *testing.T, binaryName string, content []byte) []byte {
	t.Helper()
	if runtime.GOOS == "windows" {
		var buf bytes.Buffer
		zw := zip.NewWriter(&buf)
		w, err := zw.Create(binaryName)
		if err != nil {
			t.Fatalf("create zip entry: %v", err)
		}
		if _, err := w.Write(content); err != nil {
			t.Fatalf("write zip entry: %v", err)
		}
		if err := zw.Close(); err != nil {
			t.Fatalf("close zip: %v", err)
		}
		return buf.Bytes()
	}
	return createCLITestTarGz(t, binaryName, content)
}

func setupCLIFixtureServer(t *testing.T, targetVersion string, archiveBytes []byte) *httptest.Server {
	t.Helper()
	releaseTag := "v" + targetVersion
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	archiveName := fmt.Sprintf("skillhub-%s-%s-%s.%s", targetVersion, runtime.GOOS, runtime.GOARCH, ext)

	sum := sha256.Sum256(archiveBytes)
	hashStr := fmt.Sprintf("%x", sum)
	checksumsContent := fmt.Sprintf("%s  %s\n", hashStr, archiveName)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/latest":
			http.Redirect(w, r, "/releases/tag/"+releaseTag, http.StatusFound)
		case path == "/download/"+releaseTag+"/checksums.txt":
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(checksumsContent))
		case path == "/download/"+releaseTag+"/"+archiveName:
			w.WriteHeader(http.StatusOK)
			w.Write(archiveBytes)
		default:
			http.NotFound(w, r)
		}
	}))
}

func getCompiledTestBinary(t *testing.T, versionOutput string) []byte {
	t.Helper()
	if runtime.GOOS != "windows" {
		return []byte("#!/bin/sh\ncase \"$1\" in version) echo \"" + versionOutput + "\";; *) echo ok;; esac\n")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "main.go")
	code := fmt.Sprintf("package main\nimport (\n\t\"fmt\"\n\t\"os\"\n)\nfunc main() {\n\tif len(os.Args) > 1 && os.Args[1] == \"version\" {\n\t\tfmt.Println(%q)\n\t\tos.Exit(0)\n\t}\n\tos.Exit(0)\n}\n", versionOutput)
	if err := os.WriteFile(src, []byte(code), 0644); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "mock.exe")
	cmd := exec.Command("go", "build", "-o", exe, src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("compile test binary: %v\n%s", err, out)
	}
	data, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func setupCLIManagedInstall(t *testing.T, initialVersion string) string {
	t.Helper()
	tempDir := t.TempDir()
	binName := "skillhub"
	if runtime.GOOS == "windows" {
		binName = "skillhub.exe"
	}
	exePath := filepath.Join(tempDir, binName)
	markerPath := filepath.Join(tempDir, ".skillhub-managed")

	script := getCompiledTestBinary(t, initialVersion)
	if err := os.WriteFile(exePath, script, 0755); err != nil {
		t.Fatal(err)
	}
	markerContent := fmt.Sprintf("skillhub-managed-v1\nversion=%s\n", initialVersion)
	if err := os.WriteFile(markerPath, []byte(markerContent), 0600); err != nil {
		t.Fatal(err)
	}

	return exePath
}

func TestUpdateFlagValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		args    []string
		wantWhy string
	}{
		{
			name:    "unsupported flag",
			args:    []string{"update", "--invalid"},
			wantWhy: "is not supported for update",
		},
		{
			name:    "missing version value",
			args:    []string{"update", "--version"},
			wantWhy: "missing value for --version",
		},
		{
			name:    "empty version value",
			args:    []string{"update", "--version="},
			wantWhy: "target version cannot be empty",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(tc.args, &stdout, &stderr)
			if code != 2 {
				t.Errorf("Run(%v) exit code = %d, want 2", tc.args, code)
			}
			if !strings.Contains(stderr.String(), tc.wantWhy) {
				t.Errorf("stderr %q does not contain %q", stderr.String(), tc.wantWhy)
			}
		})
	}
}

func TestUpdateHelp(t *testing.T) {
	t.Parallel()

	t.Run("update --help", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"update", "--help"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if !strings.Contains(stdout.String(), "Usage: skillhub update") {
			t.Errorf("expected usage output, got: %s", stdout.String())
		}
	})

	t.Run("help update", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"help", "update"}, &stdout, &stderr)
		if code != 0 {
			t.Fatalf("exit code = %d, want 0", code)
		}
		if !strings.Contains(stdout.String(), "Usage: skillhub update") {
			t.Errorf("expected usage output, got: %s", stdout.String())
		}
	})
}

func TestUpdateCheckAvailable(t *testing.T) {
	newBinary := getCompiledTestBinary(t, "0.2.0")
	binName := "skillhub"
	if runtime.GOOS == "windows" {
		binName = "skillhub.exe"
	}
	archiveBytes := createCLITestArchive(t, binName, newBinary)
	server := setupCLIFixtureServer(t, "0.2.0", archiveBytes)
	defer server.Close()

	exePath := setupCLIManagedInstall(t, "0.1.0")

	t.Setenv("SKILLHUB_UPDATE_TEST_EXECUTABLE", exePath)
	t.Setenv("SKILLHUB_UPDATE_BASE_URL", server.URL)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"update", "--check"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(update --check) exit code = %d, stderr = %q", code, stderr.String())
	}

	want := "Update available: 0.1.0 -> 0.2.0. Run `skillhub update` to install."
	if !strings.Contains(stdout.String(), want) {
		t.Errorf("stdout %q does not contain %q", stdout.String(), want)
	}

	// Verify binary was untouched
	data, _ := os.ReadFile(exePath)
	if strings.Contains(string(data), "0.2.0") {
		t.Errorf("binary was modified during check")
	}
}

func TestUpdateCheckAlreadyUpToDate(t *testing.T) {
	newBinary := getCompiledTestBinary(t, "0.1.0")
	binName := "skillhub"
	if runtime.GOOS == "windows" {
		binName = "skillhub.exe"
	}
	archiveBytes := createCLITestArchive(t, binName, newBinary)
	server := setupCLIFixtureServer(t, "0.1.0", archiveBytes)
	defer server.Close()

	exePath := setupCLIManagedInstall(t, "0.1.0")

	t.Setenv("SKILLHUB_UPDATE_TEST_EXECUTABLE", exePath)
	t.Setenv("SKILLHUB_UPDATE_BASE_URL", server.URL)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"update", "--check"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(update --check) exit code = %d, stderr = %q", code, stderr.String())
	}

	want := "Already up to date (0.1.0)."
	if !strings.Contains(stdout.String(), want) {
		t.Errorf("stdout %q does not contain %q", stdout.String(), want)
	}
}

func TestUpdateCheckJSON(t *testing.T) {
	newBinary := getCompiledTestBinary(t, "0.2.0")
	binName := "skillhub"
	if runtime.GOOS == "windows" {
		binName = "skillhub.exe"
	}
	archiveBytes := createCLITestArchive(t, binName, newBinary)
	server := setupCLIFixtureServer(t, "0.2.0", archiveBytes)
	defer server.Close()

	exePath := setupCLIManagedInstall(t, "0.1.0")

	t.Setenv("SKILLHUB_UPDATE_TEST_EXECUTABLE", exePath)
	t.Setenv("SKILLHUB_UPDATE_BASE_URL", server.URL)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"update", "--check", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(update --check --json) exit code = %d, stderr = %q", code, stderr.String())
	}

	var res app.Result
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("invalid json: %v\noutput: %s", err, stdout.String())
	}

	if res.Status != app.StatusOK {
		t.Errorf("status = %q, want %q", res.Status, app.StatusOK)
	}
	if res.Details["check_only"] != true {
		t.Errorf("check_only = %v, want true", res.Details["check_only"])
	}
	if res.Details["already_up_to_date"] != false {
		t.Errorf("already_up_to_date = %v, want false", res.Details["already_up_to_date"])
	}
	if res.Details["current_version"] != "0.1.0" {
		t.Errorf("current_version = %v, want 0.1.0", res.Details["current_version"])
	}
	if res.Details["target_version"] != "0.2.0" {
		t.Errorf("target_version = %v, want 0.2.0", res.Details["target_version"])
	}
}

func TestUpdateApplySuccess(t *testing.T) {
	newBinary := getCompiledTestBinary(t, "0.2.0")
	binName := "skillhub"
	if runtime.GOOS == "windows" {
		binName = "skillhub.exe"
	}
	archiveBytes := createCLITestArchive(t, binName, newBinary)
	server := setupCLIFixtureServer(t, "0.2.0", archiveBytes)
	defer server.Close()

	exePath := setupCLIManagedInstall(t, "0.1.0")

	t.Setenv("SKILLHUB_UPDATE_TEST_EXECUTABLE", exePath)
	t.Setenv("SKILLHUB_UPDATE_BASE_URL", server.URL)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"update"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(update) exit code = %d, stderr = %q", code, stderr.String())
	}

	want := "Updated 0.1.0 -> 0.2.0."
	if !strings.Contains(stdout.String(), want) {
		t.Errorf("stdout %q does not contain %q", stdout.String(), want)
	}

	// Verify binary was updated
	data, _ := os.ReadFile(exePath)
	if !bytes.Equal(data, newBinary) {
		t.Errorf("binary content was not updated to new version")
	}

	// Verify marker was updated
	markerPath := filepath.Join(filepath.Dir(exePath), ".skillhub-managed")
	markerData, _ := os.ReadFile(markerPath)
	if !strings.Contains(string(markerData), "version=0.2.0") {
		t.Errorf("marker was not updated to version=0.2.0: %s", string(markerData))
	}
}

func TestUpdateApplyJSON(t *testing.T) {
	newBinary := getCompiledTestBinary(t, "0.2.0")
	binName := "skillhub"
	if runtime.GOOS == "windows" {
		binName = "skillhub.exe"
	}
	archiveBytes := createCLITestArchive(t, binName, newBinary)
	server := setupCLIFixtureServer(t, "0.2.0", archiveBytes)
	defer server.Close()

	exePath := setupCLIManagedInstall(t, "0.1.0")

	t.Setenv("SKILLHUB_UPDATE_TEST_EXECUTABLE", exePath)
	t.Setenv("SKILLHUB_UPDATE_BASE_URL", server.URL)

	var stdout, stderr bytes.Buffer
	code := Run([]string{"update", "--json"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("Run(update --json) exit code = %d, stderr = %q", code, stderr.String())
	}

	var res app.Result
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		t.Fatalf("invalid json: %v\noutput: %s", err, stdout.String())
	}

	if res.Status != app.StatusOK {
		t.Errorf("status = %q, want %q", res.Status, app.StatusOK)
	}
	if res.Details["updated"] != true {
		t.Errorf("updated = %v, want true", res.Details["updated"])
	}
	if res.Details["already_up_to_date"] != false {
		t.Errorf("already_up_to_date = %v, want false", res.Details["already_up_to_date"])
	}
	if res.Details["current_version"] != "0.1.0" {
		t.Errorf("current_version = %v, want 0.1.0", res.Details["current_version"])
	}
	if res.Details["target_version"] != "0.2.0" {
		t.Errorf("target_version = %v, want 0.2.0", res.Details["target_version"])
	}
}

func TestUpdateUnmanagedBinaryRefusal(t *testing.T) {
	tempDir := t.TempDir()
	exePath := filepath.Join(tempDir, "skillhub")
	if err := os.WriteFile(exePath, []byte("#!/bin/sh\necho unmanaged\n"), 0755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SKILLHUB_UPDATE_TEST_EXECUTABLE", exePath)

	t.Run("human output", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"update"}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("exit code = %d, want 1", code)
		}
		if !strings.Contains(stderr.String(), "ERROR: Refusing to update an unmanaged binary.") {
			t.Errorf("expected ERROR to refuse unmanaged binary, got: %s", stderr.String())
		}
		if !strings.Contains(stderr.String(), "WHY:") {
			t.Errorf("expected WHY in error output, got: %s", stderr.String())
		}
		if !strings.Contains(stderr.String(), "FIX:") {
			t.Errorf("expected FIX in error output, got: %s", stderr.String())
		}
	})

	t.Run("json output", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run([]string{"update", "--json"}, &stdout, &stderr)
		if code != 1 {
			t.Errorf("exit code = %d, want 1", code)
		}

		var res app.Result
		if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
			t.Fatalf("invalid json: %v\noutput: %s", err, stdout.String())
		}
		if res.Status != app.StatusError {
			t.Errorf("status = %q, want %q", res.Status, app.StatusError)
		}
		if res.Error == nil || !strings.Contains(res.Error.Render.Error, "Refusing to update an unmanaged binary") {
			t.Errorf("unexpected error payload: %#v", res.Error)
		}
	})
}
