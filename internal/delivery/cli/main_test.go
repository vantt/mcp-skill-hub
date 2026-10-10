package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Isolate host configuration before parallel tests or subprocesses run.
func TestMain(m *testing.M) {
	root, err := os.MkdirTemp("", "skillhub-cli-tests-")
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
			_ = os.RemoveAll(root)
			os.Exit(1)
		}
	}
	code := m.Run()
	if err := os.RemoveAll(root); err != nil {
		fmt.Fprintln(os.Stderr, err)
		code = 1
	}
	os.Exit(code)
}
