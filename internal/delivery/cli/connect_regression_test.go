package cli

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/vantt/mcp-skill-hub/internal/systemskills"
)

// updateGoldenEnv regenerates the golden files of this package when set to 1.
const updateGoldenEnv = "SKILLHUB_UPDATE_GOLDEN"

// testdataDir is resolved once, before any test changes the working directory.
var testdataDir = func() string {
	directory, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return filepath.Join(directory, "testdata")
}()

// pathNormalizer replaces the temporary directories and the test binary in
// command output and written files with stable placeholders.
type pathNormalizer [][2]string

func (n pathNormalizer) apply(text string) string {
	for _, pair := range n {
		text = strings.ReplaceAll(text, pair[0], pair[1])
	}
	if filepath.Separator == '\\' {
		// The goldens spell the paths below a placeholder with forward
		// slashes; Windows writes them with (JSON-escaped) backslashes.
		text = placeholderPathTail.ReplaceAllStringFunc(text, func(path string) string {
			return strings.ReplaceAll(strings.ReplaceAll(path, `\\`, "/"), `\`, "/")
		})
	}
	return text
}

// placeholderPathTail matches a placeholder followed by the path below it.
var placeholderPathTail = regexp.MustCompile(`<(?:BINARY|WORKSPACE|PROJECT|HOME)>(?:\\{1,2}[^\\"'\s,\]]+)+`)

func newPathNormalizer(t *testing.T, pairs ...[2]string) pathNormalizer {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	all := append([][2]string{{filepath.Clean(binary), "<BINARY>"}}, pairs...)
	// Host files are JSON or TOML, where a Windows path appears with each
	// backslash escaped.
	for _, pair := range all {
		if escaped := strings.ReplaceAll(pair[0], `\`, `\\`); escaped != pair[0] {
			all = append(all, [2]string{escaped, pair[1]})
		}
	}
	// Longer paths first, so a path never loses its prefix to a shorter one.
	sort.SliceStable(all, func(i, j int) bool { return len(all[i][0]) > len(all[j][0]) })
	return pathNormalizer(all)
}

// snapshotTree renders every regular file under root, sorted by path, as one
// text block. Directories without files do not appear.
func snapshotTree(t *testing.T, root string, normalize pathNormalizer) string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	var out strings.Builder
	for _, path := range paths {
		relative, _ := filepath.Rel(root, path)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		out.WriteString("=== " + filepath.ToSlash(relative) + "\n")
		if string(data) == systemskills.CuratorBundle().Instructions {
			// The curator text changes with every curator release; what is
			// pinned here is that the bundled copy is installed unchanged.
			out.WriteString("<the bundled system-curator SKILL.md, unchanged>\n")
			continue
		}
		out.WriteString(normalize.apply(string(data)))
		if len(data) > 0 && data[len(data)-1] != '\n' {
			out.WriteString("\n[no final newline]\n")
		}
	}
	return out.String()
}

// fixedLengthDir creates a directory whose absolute path always has the same
// length, so wrapped command output does not depend on the temporary
// directory's random name. The directory is padded from a short root: the
// default temporary directory is about 90 characters long on macOS, and
// t.TempDir adds the test name.
func fixedLengthDir(t *testing.T, length int) string {
	t.Helper()
	base, err := os.MkdirTemp(shortTempRoot(), "sh")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { removeTestDir(t, base) })
	if base, err = filepath.EvalSymlinks(base); err != nil {
		t.Fatal(err)
	}
	pad := length - len(base) - 1
	if pad < 1 {
		t.Fatalf("temporary directory %s is longer than %d characters", base, length)
	}
	directory := filepath.Join(base, strings.Repeat("d", pad))
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	return directory
}

// shortTempRoot returns a short directory for fixedLengthDir: /tmp where it
// exists, otherwise the system temporary directory.
func shortTempRoot() string {
	if runtime.GOOS != "windows" {
		if info, err := os.Stat("/tmp"); err == nil && info.IsDir() {
			return "/tmp"
		}
	}
	return os.TempDir()
}

// removeTestDir removes a directory created outside t.TempDir. Like
// t.TempDir it retries for a while, because on Windows a file that another
// handle still holds open cannot be deleted until that handle closes.
func removeTestDir(t *testing.T, directory string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		err := os.RemoveAll(directory)
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("remove test directory: %v", err)
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join(testdataDir, filepath.FromSlash(name))
	if os.Getenv(updateGoldenEnv) == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s (regenerate with %s=1): %v", path, updateGoldenEnv, err)
	}
	if got != string(want) {
		t.Fatalf("%s differs from the golden file.\ngot:\n%s\nwant:\n%s", path, got, want)
	}
}

// The one-machine setup must not change: these goldens were generated by the
// connect command before remote hubs existed, and pin both what it prints and
// every file it writes, for a project and for the user scope.
func TestConnectWorkspaceOutputIsPinned(t *testing.T) {
	home := fixedLengthDir(t, 90)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	workspace := filepath.Join(fixedLengthDir(t, 100), "workspace")
	if code, _, stderr := runInitCLI(t, workspace); code != 0 {
		t.Fatalf("init exit = %d: %s", code, stderr)
	}

	t.Run("project", func(t *testing.T) {
		project := fixedLengthDir(t, 80)
		// Unrelated host configuration that connect must keep.
		files := map[string]string{
			".mcp.json":                   "{\n  \"mcpServers\": {\n    \"other\": {\n      \"command\": \"other-server\"\n    }\n  }\n}\n",
			"CLAUDE.md":                   "# Project notes\n\nKeep tests fast.\n",
			".codex/config.toml":          "model = \"gpt-5\"\n\n[mcp_servers.other]\ncommand = \"other-server\"\n",
			".gemini/settings.json":       "{\"theme\": \"dark\"}\n",
			".claude/settings.local.json": "{\n  \"permissions\": {\n    \"allow\": [\"Bash(ls:*)\"]\n  }\n}\n",
		}
		for relative, content := range files {
			path := filepath.Join(project, filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		normalize := newPathNormalizer(t, [2]string{workspace, "<WORKSPACE>"}, [2]string{project, "<PROJECT>"}, [2]string{home, "<HOME>"})
		var transcript strings.Builder
		for _, args := range [][]string{
			{"connect", workspace, "--dry-run"},
			{"connect", workspace},
			{"connect", workspace},
		} {
			// connect runs in the project folder: `cd <PROJECT> && skillhub connect ...`.
			code, stdout, stderr := runConnectCLI(t, project, args[1:]...)
			transcript.WriteString("$ " + normalize.apply(strings.Join(args, " ")) + "\n")
			transcript.WriteString(normalize.apply(stdout))
			transcript.WriteString(normalize.apply(stderr))
			transcript.WriteString("[exit " + strconv.Itoa(code) + "]\n")
		}
		checkGolden(t, "connect-workspace/project-output.golden", transcript.String())
		checkGolden(t, "connect-workspace/project-files.golden", snapshotTree(t, project, normalize))
	})

	t.Run("global", func(t *testing.T) {
		normalize := newPathNormalizer(t, [2]string{workspace, "<WORKSPACE>"}, [2]string{home, "<HOME>"})
		var transcript strings.Builder
		for _, args := range [][]string{
			{"connect", "-g", workspace, "--host", "claude,codex,gemini", "--dry-run"},
			{"connect", "-g", workspace},
		} {
			code, stdout, stderr := runCLI(t, args...)
			transcript.WriteString("$ " + normalize.apply(strings.Join(args, " ")) + "\n")
			transcript.WriteString(normalize.apply(stdout))
			transcript.WriteString(normalize.apply(stderr))
			transcript.WriteString("[exit " + strconv.Itoa(code) + "]\n")
		}
		checkGolden(t, "connect-workspace/global-output.golden", transcript.String())
		checkGolden(t, "connect-workspace/global-files.golden", snapshotTree(t, home, normalize))
	})

	t.Run("doctor for a local entry", func(t *testing.T) {
		project := fixedLengthDir(t, 80)
		if code, _, stderr := runConnectCLI(t, project, workspace, "--host", "claude"); code != 0 {
			t.Fatal(stderr)
		}
		t.Chdir(project)
		normalize := newPathNormalizer(t, [2]string{workspace, "<WORKSPACE>"}, [2]string{project, "<PROJECT>"}, [2]string{home, "<HOME>"})
		var transcript strings.Builder
		for _, args := range [][]string{
			{"doctor", "--workspace", workspace},
			{"doctor"},
		} {
			code, stdout, stderr := runCLI(t, args...)
			transcript.WriteString("$ " + normalize.apply(strings.Join(args, " ")) + "\n")
			transcript.WriteString(normalize.apply(stdout))
			transcript.WriteString(normalize.apply(stderr))
			transcript.WriteString("[exit " + strconv.Itoa(code) + "]\n")
		}
		checkGolden(t, "connect-workspace/doctor-output.golden", transcript.String())
	})
}

// The connect summary names each host file the same way on every platform,
// although the item it reads names the file with the platform's separators.
func TestConnectNamesHostFilesTheSameWayOnEveryPlatform(t *testing.T) {
	root := t.TempDir()
	for _, test := range []struct {
		kind, relative, want string
	}{
		{"mcp", ".codex/config.toml", ".codex/config.toml"},
		{"mcp", ".gemini/settings.json", ".gemini/settings.json"},
		{"mcp", ".mcp.json", ".mcp.json"},
		{"mcp", ".claude.json", ".claude.json"},
		{"host-permissions", ".claude/settings.local.json", ".claude/settings.local.json"},
		{"bootstrap", ".codex/AGENTS.md", "AGENTS.md"},
	} {
		summary := "codex-cli (native) " + test.kind + ": " + filepath.Join(root, filepath.FromSlash(test.relative))
		if got := formatChangeKind(test.kind, summary); got != test.want {
			t.Errorf("formatChangeKind(%q, %q) = %q, want %q", test.kind, summary, got, test.want)
		}
	}
}
