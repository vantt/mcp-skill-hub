package systemskills

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
	"testing"
)

// releasedBundleDirs maps the folder a release embeds each system skill from to
// the skill id.
var releasedBundleDirs = map[string]string{
	"internal/systemskills/curator/":     CuratorSkillID,
	"internal/systemskills/distill-lab/": DistillSkillID,
}

// TestKnownDigestsCoverReleasedBundles checks that every system skill file a
// release tag shipped is recognized by KnownFileDigest, so an unedited copy
// written by any released binary is upgraded instead of reported as the user's.
// The curator table holds every committed version, but only tagged ones are
// enforced here; distill-lab files enter the table only as they ship in a tag.
//
// It needs Git and the release tags. Without them it skips, unless
// SKILLHUB_REQUIRE_TAGS=1, which CI sets on its full-history checkouts.
func TestKnownDigestsCoverReleasedBundles(t *testing.T) {
	t.Parallel()

	require := os.Getenv("SKILLHUB_REQUIRE_TAGS") == "1"
	unavailable := func(format string, args ...any) {
		t.Helper()
		if require {
			t.Fatalf(format, args...)
		}
		t.Skipf(format, args...)
	}

	if _, err := exec.LookPath("git"); err != nil {
		unavailable("git is not available: %v", err)
	}
	tagList, err := gitOutput("tag", "-l", "v*")
	if err != nil {
		unavailable("list release tags: %v", err)
	}
	tags := strings.Fields(tagList)
	if len(tags) == 0 {
		unavailable("this checkout has no release tags")
	}

	for _, tag := range tags {
		checked := 0
		for dir, id := range releasedBundleDirs {
			// --full-tree: paths are relative to the repository root, not
			// to this package folder where the test runs.
			listing, err := gitOutput("ls-tree", "--full-tree", "-r", "--name-only", tag, "--", dir)
			if err != nil {
				t.Fatalf("list %s in %s: %v", dir, tag, err)
			}
			for _, file := range strings.Fields(listing) {
				rel := strings.TrimPrefix(file, dir)
				if embedSkips(rel) {
					continue
				}
				content, err := gitOutput("show", tag+":"+file)
				if err != nil {
					t.Fatalf("read %s in %s: %v", file, tag, err)
				}
				checked++
				if !KnownFileDigest(id, rel, FileDigest([]byte(content))) {
					t.Errorf("%s shipped %s (%s %s) with a digest missing from known-digests.go", tag, file, id, rel)
				}
			}
		}
		// Every release embeds the curator, so an empty listing means the
		// lookup itself is broken and the check would pass vacuously.
		if checked == 0 {
			t.Errorf("%s: no system skill file found under %v", tag, releasedBundleDirs)
		}
	}
}

// embedSkips reports whether a directory go:embed leaves out a file at this
// slash-separated path: any element starting with "." or "_".
func embedSkips(rel string) bool {
	for _, element := range strings.Split(path.Clean(rel), "/") {
		if strings.HasPrefix(element, ".") || strings.HasPrefix(element, "_") {
			return true
		}
	}
	return false
}

func gitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.Output()
	if exit, ok := err.(*exec.ExitError); ok {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(exit.Stderr)))
	}
	return string(out), err
}
