package skill

import (
	"reflect"
	"testing"
)

func TestSuggestRouting(t *testing.T) {
	t.Parallel()
	scaffold := "---\nname: x\n---\n# X\n\n## When to use\n\n- after a production deploy\n- when the release is tagged\n\n## When not to use\n\n- staging deploys\n\n## Examples\n\n1. write the deploy notes for today's release\n\n## Steps\n\n- not a routing list\n"
	tests := []struct {
		name        string
		description string
		skillMD     string
		want        RoutingInput
	}{
		{
			name:        "description sentences",
			description: "Cut a release. Use when you cut a release, bump the version or tag it. Not for publishing a package to npm.",
			want: RoutingInput{
				Triggers: []string{"you cut a release", "bump the version", "tag it"},
				NotFor:   []string{"publishing a package to npm"},
				MinScope: "single_step",
			},
		},
		{
			name:        "do not use wins over use",
			description: "Do not use for staging deploys. Use for production deploys.",
			want:        RoutingInput{Triggers: []string{"production deploys"}, NotFor: []string{"staging deploys"}, MinScope: "single_step"},
		},
		{
			name:        "and never splits a phrase",
			description: "Use when you lint and format a diff.",
			want:        RoutingInput{Triggers: []string{"you lint and format a diff"}, MinScope: "single_step"},
		},
		{
			name:        "sections of the scaffold",
			description: "Summarize a deploy for the team channel.",
			skillMD:     scaffold,
			want: RoutingInput{
				Triggers: []string{"after a production deploy", "when the release is tagged"},
				NotFor:   []string{"staging deploys"},
				Examples: []string{"write the deploy notes for today's release"},
				MinScope: "single_step",
			},
		},
		{
			name:        "duplicates collapse ignoring case",
			description: "Use when Deploying.",
			skillMD:     "## When to use\n- deploying\n",
			want:        RoutingInput{Triggers: []string{"Deploying"}, MinScope: "single_step"},
		},
		{
			name:        "nothing to suggest",
			description: "A plain description.",
			want:        RoutingInput{MinScope: "single_step"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := SuggestRouting(test.description, test.skillMD); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("SuggestRouting() = %+v, want %+v", got, test.want)
			}
		})
	}
}
