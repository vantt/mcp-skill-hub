package skillruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"
)

// Reason codes reported when a third-party skill has not been approved.
const (
	ReasonContentReviewRequired = "content_review_required"
	ReasonContentReviewStale    = "content_review_stale"
)

// metaFileName is hub metadata that is never part of the reviewed content.
const metaFileName = "skill.meta.yaml"

// Provenance is the subset of manifest provenance that decides trust.
type Provenance struct {
	OriginKind string
	SourceID   string
}

// IsThirdParty reports whether the skill was imported from a remote origin or
// a registered source, as opposed to authored or added from a local folder.
func IsThirdParty(p Provenance) bool {
	return p.OriginKind == "github" || p.OriginKind == "git" || p.SourceID != ""
}

// ResourceDigest identifies one skill file by path (relative to the skill
// folder) and content digest.
type ResourceDigest struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

// ContentDigest binds a human approval to the whole skill content: sha256 over
// the canonical JSON {"files":[{path,digest}... sorted by path, without
// skill.meta.yaml], "runtime": spec-or-null}, prefixed "sha256:". It needs only
// catalog resource rows, never file bytes.
func ContentDigest(files []ResourceDigest, spec Spec, hasSpec bool) string {
	return "sha256:" + hashFilesAndRuntime("files", withoutMeta(files), spec, hasSpec)
}

// withoutMeta returns files sorted by path and digest, minus skill.meta.yaml.
func withoutMeta(files []ResourceDigest) []ResourceDigest {
	sorted := make([]ResourceDigest, 0, len(files))
	for _, file := range files {
		if file.Path != metaFileName {
			sorted = append(sorted, file)
		}
	}
	slices.SortFunc(sorted, func(a, b ResourceDigest) int {
		if c := strings.Compare(a.Path, b.Path); c != 0 {
			return c
		}
		return strings.Compare(a.Digest, b.Digest)
	})
	return sorted
}

// hashFilesAndRuntime hashes {<key>: files, "runtime": spec-or-null} as
// canonical JSON and returns the lowercase hex digest.
func hashFilesAndRuntime(key string, files []ResourceDigest, spec Spec, hasSpec bool) string {
	runtimeJSON := json.RawMessage("null")
	if hasSpec {
		runtimeJSON = spec.CanonicalJSON()
	}
	// Marshalling strings and pre-encoded JSON cannot fail.
	encoded, _ := json.Marshal(map[string]any{key: files, "runtime": runtimeJSON})
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

// Verdict is the trust decision for a skill's content.
type Verdict struct {
	Trusted        bool     `json:"trusted"`
	RequiresReview bool     `json:"requires_review"`
	ContentDigest  string   `json:"content_digest"`
	ReasonCodes    []string `json:"reason_codes,omitempty"`
}

// Evaluate decides whether a skill's content may be served and run. A skill
// that is not third-party is always trusted; a third-party skill passes only
// when reviewedDigest equals its current content digest. A review of different
// content adds the stale reason.
func Evaluate(p Provenance, contentDigest, reviewedDigest string) Verdict {
	verdict := Verdict{ContentDigest: contentDigest}
	if !IsThirdParty(p) || reviewedDigest == contentDigest {
		verdict.Trusted = true
		return verdict
	}
	verdict.RequiresReview = true
	verdict.ReasonCodes = []string{ReasonContentReviewRequired}
	if reviewedDigest != "" {
		verdict.ReasonCodes = append(verdict.ReasonCodes, ReasonContentReviewStale)
	}
	return verdict
}
