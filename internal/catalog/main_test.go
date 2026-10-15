package catalog

import (
	"testing"

	"github.com/vantt/skillhub-mcp/internal/testenv"
)

// TestMain keeps Git in these tests on the repositories they create: `git
// bisect run` and Git hooks export GIT_DIR, which Git prefers over the
// working directory.
func TestMain(m *testing.M) {
	testenv.RunOutsideGitRepository(m)
}

// TestTestsRunOutsideTheCallersRepository guards the isolation in TestMain:
// no test or subprocess may find the repository that runs the tests.
func TestTestsRunOutsideTheCallersRepository(t *testing.T) {
	testenv.RequireNoGitRepository(t)
}
