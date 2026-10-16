// Package testenv holds the process isolation that the TestMain of every
// package whose tests run Git shares. Only test files import it.
package testenv

import (
	"fmt"
	"os"
	"testing"
)

// GitRepositoryVariables names the variables that point Git at a repository
// other than the one around its working directory. `git bisect run` and Git
// hooks export GIT_DIR, and a test that runs `git init` or `skillhub init` in
// a temporary folder would then write into the repository running the tests.
var GitRepositoryVariables = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR", "GIT_PREFIX",
	"GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_NAMESPACE",
}

// backgroundMaintenanceOff turns off the maintenance Git may start in the
// background after a commit. A detached repack can still be writing into
// .git/objects/pack of a temporary repository when the test that made it
// ends, so removing the test's temporary folder fails.
var backgroundMaintenanceOff = [][2]string{{"gc.auto", "0"}, {"maintenance.auto", "false"}}

// UnsetGitRepository removes GitRepositoryVariables from the process
// environment, so Git in tests and in their subprocesses finds its repository
// from the working directory, and turns off Git's background maintenance for
// them. Call it from TestMain before m.Run.
func UnsetGitRepository() error {
	for _, key := range GitRepositoryVariables {
		if err := os.Unsetenv(key); err != nil {
			return fmt.Errorf("unset %s: %w", key, err)
		}
	}
	env := map[string]string{"GIT_CONFIG_COUNT": fmt.Sprint(len(backgroundMaintenanceOff))}
	for i, entry := range backgroundMaintenanceOff {
		env[fmt.Sprintf("GIT_CONFIG_KEY_%d", i)] = entry[0]
		env[fmt.Sprintf("GIT_CONFIG_VALUE_%d", i)] = entry[1]
	}
	for key, value := range env {
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set %s: %w", key, err)
		}
	}
	return nil
}

// RequireNoGitRepository fails the test when any of GitRepositoryVariables is
// still set, which means TestMain did not isolate the package's tests.
func RequireNoGitRepository(t testing.TB) {
	t.Helper()
	for _, key := range GitRepositoryVariables {
		if value, ok := os.LookupEnv(key); ok {
			t.Errorf("%s=%s is still set", key, value)
		}
	}
}

// RunOutsideGitRepository is a whole TestMain for a package that needs only
// the Git isolation: it unsets GitRepositoryVariables and runs the tests.
func RunOutsideGitRepository(m *testing.M) {
	if err := UnsetGitRepository(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
