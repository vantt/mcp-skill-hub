package web

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// Isolate host configuration before parallel tests or subprocesses run.
func TestMain(m *testing.M) {
	// Preserve HOME/XDG-derived Go paths so subprocess builds do not populate
	// the isolated home with read-only module cache files.
	args := []string{"env", "-json"}
	for _, key := range []string{"GOCACHE", "GOMODCACHE", "GOPATH"} {
		if os.Getenv(key) == "" {
			args = append(args, key)
		}
	}
	if len(args) > 2 {
		output, err := exec.Command("go", args...).Output()
		var paths map[string]string
		if err == nil {
			err = json.Unmarshal(output, &paths)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "resolve Go cache paths before HOME isolation: %v\n", err)
			os.Exit(1)
		}
		for _, key := range args[2:] {
			if err := os.Setenv(key, paths[key]); err != nil {
				fmt.Fprintf(os.Stderr, "pin %s before HOME isolation: %v\n", key, err)
				os.Exit(1)
			}
		}
	}
	root, err := os.MkdirTemp("", "skillhub-web-tests-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, key := range []string{
		"HOME", "USERPROFILE", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME",
		"XDG_STATE_HOME", "XDG_RUNTIME_DIR", "XDG_CONFIG_DIRS", "XDG_DATA_DIRS",
	} {
		directory := filepath.Join(root, key)
		err = os.Mkdir(directory, 0o700)
		if err == nil {
			err = os.Setenv(key, directory)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "isolate %s: %v\n", key, err)
			if cleanupErr := os.RemoveAll(root); cleanupErr != nil {
				fmt.Fprintf(os.Stderr, "clean up isolated test files in %s before running tests: %v\n", root, cleanupErr)
			}
			os.Exit(1)
		}
	}
	code := m.Run()
	if err := os.RemoveAll(root); err != nil {
		fmt.Fprintf(os.Stderr, "clean up isolated test files in %s (test exit code %d): %v\n", root, code, err)
		if code == 0 {
			code = 1
		}
	}
	os.Exit(code)
}
