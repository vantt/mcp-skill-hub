package hostintegration

// knownWrappers lists the digests ("sha256:<hex>") of Claude Code wrapper
// renders that a release wrote, keyed by system skill id. A wrapper on disk is
// Skill Hub's when its bytes equal the current render or one of these; any
// other content is the user's and is never overwritten.
//
// The current render of each wrapper is committed as
// testdata/wrappers/<id>.golden. After a release changes a wrapper (a new
// template version or a new canonical description), add the digest of the
// golden file that the release tag holds:
//
//	git show <tag>:internal/hostintegration/testdata/wrappers/<id>.golden | sha256sum
//
// TestKnownWrappersCoverReleasedRenders checks every release tag and fails
// when a shipped render is missing here.
var knownWrappers = map[string][]string{}
