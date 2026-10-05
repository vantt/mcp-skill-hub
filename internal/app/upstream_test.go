package app

import (
	"testing"

	sourcepkg "github.com/vantt/mcp-skill-hub/internal/source"
)

func TestDeriveUpstreamStatus(t *testing.T) {
	validDigest := "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	modifiedDigest := "sha256:2222222222222222222222222222222222222222222222222222222222222222"

	baseOrigin := SkillOrigin{
		Kind:        "github",
		Repository:  "https://github.com/example/skills",
		Ref:         "main",
		Commit:      "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111",
		Path:        "skills/pdf",
		FilesDigest: validDigest,
	}

	baseRecord := &sourcepkg.Record{
		ID:      "example-skills",
		Adapter: "git",
		Locator: sourcepkg.Locator{
			Repository: "https://github.com/example/skills",
			Ref:        "main",
		},
	}

	baseState := &sourcepkg.UpstreamState{
		SkillID:        "pdf",
		SourceID:       "example-skills",
		Repository:     "https://github.com/example/skills",
		Ref:            "main",
		Path:           "skills/pdf",
		BaseCommit:     "aaaa1111aaaa1111aaaa1111aaaa1111aaaa1111",
		CheckedCommit:  "bbbb2222bbbb2222bbbb2222bbbb2222bbbb2222",
		Upstream:       "same",
		UpstreamDigest: validDigest,
	}

	cases := []struct {
		name           string
		origin         SkillOrigin
		sourceID       string
		sourceRec      *sourcepkg.Record
		state          *sourcepkg.UpstreamState
		currentLocal   string
		expectedStatus string
		expectedLocal  string
		expectedErr    string
	}{
		{
			name:           "untracked github without sourceID",
			origin:         baseOrigin,
			sourceID:       "",
			sourceRec:      baseRecord,
			state:          baseState,
			currentLocal:   validDigest,
			expectedStatus: "untracked",
			expectedLocal:  "clean",
		},
		{
			name: "untracked git without sourceID",
			origin: func() SkillOrigin {
				o := baseOrigin
				o.Kind = "git"
				return o
			}(),
			sourceID:       "",
			sourceRec:      baseRecord,
			state:          baseState,
			currentLocal:   modifiedDigest,
			expectedStatus: "untracked",
			expectedLocal:  "modified",
		},
		{
			name: "pinned 40 lowercase hex ref",
			origin: func() SkillOrigin {
				o := baseOrigin
				o.Ref = "1234567890abcdef1234567890abcdef12345678"
				return o
			}(),
			sourceID:       "example-skills",
			sourceRec:      baseRecord,
			state:          baseState,
			currentLocal:   validDigest,
			expectedStatus: "pinned",
			expectedLocal:  "clean",
		},
		{
			name:           "unavailable missing source record",
			origin:         baseOrigin,
			sourceID:       "example-skills",
			sourceRec:      nil,
			state:          baseState,
			currentLocal:   validDigest,
			expectedStatus: "unavailable",
			expectedLocal:  "clean",
			expectedErr:    "source_origin_mismatch",
		},
		{
			name:     "unavailable source record repo mismatch",
			origin:   baseOrigin,
			sourceID: "example-skills",
			sourceRec: &sourcepkg.Record{
				ID:      "example-skills",
				Adapter: "git",
				Locator: sourcepkg.Locator{
					Repository: "https://github.com/other/skills",
					Ref:        "main",
				},
			},
			state:          baseState,
			currentLocal:   validDigest,
			expectedStatus: "unavailable",
			expectedLocal:  "clean",
			expectedErr:    "source_origin_mismatch",
		},
		{
			name:     "unavailable source record ref mismatch",
			origin:   baseOrigin,
			sourceID: "example-skills",
			sourceRec: &sourcepkg.Record{
				ID:      "example-skills",
				Adapter: "git",
				Locator: sourcepkg.Locator{
					Repository: "https://github.com/example/skills",
					Ref:        "dev",
				},
			},
			state:          baseState,
			currentLocal:   validDigest,
			expectedStatus: "unavailable",
			expectedLocal:  "clean",
			expectedErr:    "source_origin_mismatch",
		},
		{
			name:           "unknown state is nil",
			origin:         baseOrigin,
			sourceID:       "example-skills",
			sourceRec:      baseRecord,
			state:          nil,
			currentLocal:   validDigest,
			expectedStatus: "unknown",
			expectedLocal:  "clean",
		},
		{
			name:     "unknown state base commit mismatch",
			origin:   baseOrigin,
			sourceID: "example-skills",
			sourceRec: baseRecord,
			state: func() *sourcepkg.UpstreamState {
				s := *baseState
				s.BaseCommit = "different-base-commit"
				return &s
			}(),
			currentLocal:   validDigest,
			expectedStatus: "unknown",
			expectedLocal:  "clean",
		},
		{
			name:     "unknown state repository mismatch",
			origin:   baseOrigin,
			sourceID: "example-skills",
			sourceRec: baseRecord,
			state: func() *sourcepkg.UpstreamState {
				s := *baseState
				s.Repository = "https://github.com/other/skills"
				return &s
			}(),
			currentLocal:   validDigest,
			expectedStatus: "unknown",
			expectedLocal:  "clean",
		},
		{
			name:     "unknown state ref mismatch",
			origin:   baseOrigin,
			sourceID: "example-skills",
			sourceRec: baseRecord,
			state: func() *sourcepkg.UpstreamState {
				s := *baseState
				s.Ref = "dev"
				return &s
			}(),
			currentLocal:   validDigest,
			expectedStatus: "unknown",
			expectedLocal:  "clean",
		},
		{
			name:     "unknown state path mismatch",
			origin:   baseOrigin,
			sourceID: "example-skills",
			sourceRec: baseRecord,
			state: func() *sourcepkg.UpstreamState {
				s := *baseState
				s.Path = "skills/other"
				return &s
			}(),
			currentLocal:   validDigest,
			expectedStatus: "unknown",
			expectedLocal:  "clean",
		},
		{
			name:     "state unavailable",
			origin:   baseOrigin,
			sourceID: "example-skills",
			sourceRec: baseRecord,
			state: func() *sourcepkg.UpstreamState {
				s := *baseState
				s.Upstream = "unavailable"
				s.LastError = "git remote unreachable"
				return &s
			}(),
			currentLocal:   validDigest,
			expectedStatus: "unavailable",
			expectedLocal:  "clean",
			expectedErr:    "git remote unreachable",
		},
		{
			name:     "state removed yields upstream_removed",
			origin:   baseOrigin,
			sourceID: "example-skills",
			sourceRec: baseRecord,
			state: func() *sourcepkg.UpstreamState {
				s := *baseState
				s.Upstream = "removed"
				return &s
			}(),
			currentLocal:   validDigest,
			expectedStatus: "upstream_removed",
			expectedLocal:  "clean",
		},
		{
			name:     "same + clean yields up_to_date",
			origin:   baseOrigin,
			sourceID: "example-skills",
			sourceRec: baseRecord,
			state: func() *sourcepkg.UpstreamState {
				s := *baseState
				s.Upstream = "same"
				return &s
			}(),
			currentLocal:   validDigest,
			expectedStatus: "up_to_date",
			expectedLocal:  "clean",
		},
		{
			name: "same + unknown local yields up_to_date",
			origin: func() SkillOrigin {
				o := baseOrigin
				o.FilesDigest = ""
				return o
			}(),
			sourceID:  "example-skills",
			sourceRec: baseRecord,
			state: func() *sourcepkg.UpstreamState {
				s := *baseState
				s.Upstream = "same"
				return &s
			}(),
			currentLocal:   validDigest,
			expectedStatus: "up_to_date",
			expectedLocal:  "unknown",
		},
		{
			name:     "same + modified local yields modified",
			origin:   baseOrigin,
			sourceID: "example-skills",
			sourceRec: baseRecord,
			state: func() *sourcepkg.UpstreamState {
				s := *baseState
				s.Upstream = "same"
				return &s
			}(),
			currentLocal:   modifiedDigest,
			expectedStatus: "modified",
			expectedLocal:  "modified",
		},
		{
			name:     "changed + clean yields update_available",
			origin:   baseOrigin,
			sourceID: "example-skills",
			sourceRec: baseRecord,
			state: func() *sourcepkg.UpstreamState {
				s := *baseState
				s.Upstream = "changed"
				return &s
			}(),
			currentLocal:   validDigest,
			expectedStatus: "update_available",
			expectedLocal:  "clean",
		},
		{
			name: "changed + unknown local yields update_available",
			origin: func() SkillOrigin {
				o := baseOrigin
				o.FilesDigest = ""
				return o
			}(),
			sourceID:  "example-skills",
			sourceRec: baseRecord,
			state: func() *sourcepkg.UpstreamState {
				s := *baseState
				s.Upstream = "changed"
				return &s
			}(),
			currentLocal:   validDigest,
			expectedStatus: "update_available",
			expectedLocal:  "unknown",
		},
		{
			name:     "changed + modified local yields diverged",
			origin:   baseOrigin,
			sourceID: "example-skills",
			sourceRec: baseRecord,
			state: func() *sourcepkg.UpstreamState {
				s := *baseState
				s.Upstream = "changed"
				return &s
			}(),
			currentLocal:   modifiedDigest,
			expectedStatus: "diverged",
			expectedLocal:  "modified",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, local, errStr := deriveUpstreamStatus(tc.origin, tc.sourceID, tc.sourceRec, tc.state, tc.currentLocal)
			if status != tc.expectedStatus {
				t.Fatalf("expected status %q, got %q", tc.expectedStatus, status)
			}
			if local != tc.expectedLocal {
				t.Fatalf("expected local %q, got %q", tc.expectedLocal, local)
			}
			if tc.expectedErr != "" && errStr != tc.expectedErr {
				t.Fatalf("expected errStr %q, got %q", tc.expectedErr, errStr)
			}
		})
	}
}
