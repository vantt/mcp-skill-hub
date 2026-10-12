package app

import "testing"

func TestSourceCountsUseSingularAndPluralWords(t *testing.T) {
	cases := []struct {
		count int
		want  string
	}{
		{1, "1 source is ready to distill"},
		{2, "2 sources are ready to distill"},
	}
	for _, tc := range cases {
		got := termCount(tc.count, "source", "sources") + " " + pluralWord(tc.count, "is", "are") + " ready to distill"
		if got != tc.want {
			t.Fatalf("count %d: got %q, want %q", tc.count, got, tc.want)
		}
	}
}
