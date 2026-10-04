package skillruntime

import (
	"reflect"
	"strings"
	"testing"
)

func TestIsThirdParty(t *testing.T) {
	tests := []struct {
		p    Provenance
		want bool
	}{
		{Provenance{}, false},
		{Provenance{OriginKind: "local"}, false},
		{Provenance{OriginKind: "github"}, true},
		{Provenance{OriginKind: "git"}, true},
		{Provenance{OriginKind: "local", SourceID: "upstream"}, true},
	}
	for _, tt := range tests {
		if got := IsThirdParty(tt.p); got != tt.want {
			t.Errorf("IsThirdParty(%+v) = %v, want %v", tt.p, got, tt.want)
		}
	}
}

func TestContentDigest(t *testing.T) {
	a := []ResourceDigest{{Path: "references/b.md", Digest: "sha256:2"}, {Path: "SKILL.md", Digest: "sha256:1"}}
	b := []ResourceDigest{{Path: "SKILL.md", Digest: "sha256:1"}, {Path: "references/b.md", Digest: "sha256:2"}}
	spec := Spec{Setup: Setup{Check: "true"}}
	digest := ContentDigest(a, spec, true)
	if !strings.HasPrefix(digest, "sha256:") || len(digest) != len("sha256:")+64 {
		t.Fatalf("digest %q has wrong shape", digest)
	}
	if digest != ContentDigest(b, spec, true) {
		t.Fatal("digest depends on input order")
	}
	withMeta := append(append([]ResourceDigest{}, a...), ResourceDigest{Path: "skill.meta.yaml", Digest: "sha256:meta"})
	if digest != ContentDigest(withMeta, spec, true) {
		t.Fatal("skill.meta.yaml must not affect the digest")
	}
	if digest == ContentDigest(a, spec, false) {
		t.Fatal("digest ignores the runtime block")
	}
	if digest == ContentDigest(a, Spec{Setup: Setup{Check: "false"}}, true) {
		t.Fatal("digest ignores the runtime commands")
	}
	changed := []ResourceDigest{{Path: "references/b.md", Digest: "sha256:2"}, {Path: "SKILL.md", Digest: "sha256:changed"}}
	if digest == ContentDigest(changed, spec, true) {
		t.Fatal("digest ignores a file change")
	}
	added := append(append([]ResourceDigest{}, a...), ResourceDigest{Path: "notes.txt", Digest: "sha256:3"})
	if digest == ContentDigest(added, spec, true) {
		t.Fatal("digest ignores an added file")
	}
}

func TestEvaluate(t *testing.T) {
	github := Provenance{OriginKind: "github"}
	files := []ResourceDigest{{Path: "SKILL.md", Digest: "sha256:abc"}}
	current := ContentDigest(files, Spec{}, false)
	other := "sha256:" + strings.Repeat("0", 64)
	tests := []struct {
		name     string
		p        Provenance
		reviewed string
		trusted  bool
		reasons  []string
	}{
		{name: "local skill passes unreviewed", p: Provenance{OriginKind: "local"}, trusted: true},
		{name: "unreviewed third-party needs review", p: github, reasons: []string{ReasonContentReviewRequired}},
		{name: "registered source needs review", p: Provenance{SourceID: "src"}, reasons: []string{ReasonContentReviewRequired}},
		{name: "matching approval passes", p: github, reviewed: current, trusted: true},
		{name: "approval of other content is stale", p: github, reviewed: other, reasons: []string{ReasonContentReviewRequired, ReasonContentReviewStale}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Evaluate(tt.p, current, tt.reviewed)
			if got.Trusted != tt.trusted || got.RequiresReview == tt.trusted {
				t.Fatalf("verdict = %+v, want trusted %v", got, tt.trusted)
			}
			if !reflect.DeepEqual(got.ReasonCodes, tt.reasons) {
				t.Fatalf("reasons = %v, want %v", got.ReasonCodes, tt.reasons)
			}
			if got.ContentDigest != current {
				t.Fatal("verdict must carry the evaluated digest")
			}
		})
	}
}
