package app

import "testing"

func TestFirstRunRecommendationRequiresEmptyUncommittedWorkspace(t *testing.T) {
	t.Parallel()
	base := homeState{Health: "valid", Index: "current", GitDirty: true, GitConfigured: true, CountsKnown: true}
	cases := []struct {
		name     string
		mutate   func(*homeState)
		firstRun bool
	}{
		{"empty and uncommitted", func(*homeState) {}, true},
		{"committed", func(s *homeState) { s.GitDirty = false }, false},
		{"draft skill exists", func(s *homeState) { s.TotalSkills = 1 }, false},
		{"source exists", func(s *homeState) { s.TotalSources = 1 }, false},
		{"counts unavailable", func(s *homeState) { s.CountsKnown = false }, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := base
			tc.mutate(&state)
			home := deriveCurationHome(state)
			got := home.Summary == "The new hub is not committed to Git yet."
			if got != tc.firstRun {
				t.Fatalf("first-run headline = %t, want %t (summary %q)", got, tc.firstRun, home.Summary)
			}
		})
	}
}

func TestStaleIndexHeadlineRecommendsRebuild(t *testing.T) {
	t.Parallel()
	home := deriveCurationHome(homeState{Health: "valid", Index: "stale", GitDirty: true, GitConfigured: true})
	if len(home.SuggestedActions) != 1 || home.Actions[0].Kind != "rebuild_index" {
		t.Fatalf("actions = %#v", home.Actions)
	}
	if want := "Run `skillhub maintain rebuild` to refresh the search index"; home.SuggestedActions[0].Label != want {
		t.Fatalf("label = %q, want %q", home.SuggestedActions[0].Label, want)
	}
	if home.Summary == "The new hub is not committed to Git yet." {
		t.Fatalf("stale index reported as first run: %q", home.Summary)
	}
}
