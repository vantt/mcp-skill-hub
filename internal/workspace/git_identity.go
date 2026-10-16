package workspace

import (
	"context"
	"os/exec"
)

// Commands that set the identity Git records on commits, for a person to copy.
const (
	SetGitNameCommand  = `git config --global user.name "<your name>"`
	SetGitEmailCommand = `git config --global user.email "<your email>"`
)

// GitIdentityMissing reports whether Git, run in dir, cannot name the author
// or the committer of a new commit, so every commit would fail. It asks Git
// itself, so an identity from the environment, from the repository's own
// config or one Git can work out from the machine counts as set. It is false
// when Git is not installed: other checks report that.
func GitIdentityMissing(ctx context.Context, dir string) bool {
	if _, err := exec.LookPath("git"); err != nil {
		return false
	}
	for _, variable := range []string{"GIT_AUTHOR_IDENT", "GIT_COMMITTER_IDENT"} {
		if err := gitCommand(ctx, dir, nil, "var", variable).Run(); err != nil {
			return true
		}
	}
	return false
}
