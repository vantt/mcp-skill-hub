// Package testenv holds the process isolation that the TestMain of every
// package whose tests run Git shares. Only test files import it.
package testenv

import (
	"fmt"
	"os"
	"path/filepath"
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

// UnsetGitRepository removes GitRepositoryVariables from the process
// environment, so Git in tests and in their subprocesses finds its repository
// from the working directory. Call it from TestMain before m.Run.
func UnsetGitRepository() error {
	for _, key := range GitRepositoryVariables {
		if err := os.Unsetenv(key); err != nil {
			return fmt.Errorf("unset %s: %w", key, err)
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

// SetGitIdentity points Git's global configuration at a new file in dir that
// holds a test identity, and turns the system configuration off. Hubs made by
// tests commit, and init refuses to run without an identity, so the tests of a
// package that runs init call it from TestMain; a test about a missing
// identity sets its own GIT_CONFIG_GLOBAL.
func SetGitIdentity(dir string) error {
	config := filepath.Join(dir, "gitconfig")
	if err := os.WriteFile(config, []byte("[user]\n\tname = Skill Hub Tests\n\temail = tests@example.invalid\n"), 0o600); err != nil {
		return fmt.Errorf("write the test Git config: %w", err)
	}
	for key, value := range map[string]string{"GIT_CONFIG_GLOBAL": config, "GIT_CONFIG_NOSYSTEM": "1"} {
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set %s: %w", key, err)
		}
	}
	return nil
}
