package invite

import (
	"bytes"
	"errors"
	"testing"
)

func TestAlphabetHas32DistinctSymbols(t *testing.T) {
	seen := map[rune]bool{}
	for _, r := range alphabet {
		seen[r] = true
	}
	if len(alphabet) != 32 || len(seen) != 32 {
		t.Fatalf("alphabet has %d symbols, %d distinct; want 32", len(alphabet), len(seen))
	}
}

func TestParseCodeNormalises(t *testing.T) {
	cases := map[string]string{
		"K7QF9M2X4TBD":     "K7QF9M2X4TBD",
		"k7qf 9m2x-4tbd":   "K7QF9M2X4TBD",
		" K7QF-9M2X-4TBD ": "K7QF9M2X4TBD",
		"O0oo-0000-0000":   "000000000000",
		"I1il-LLLL-1111":   "111111111111",
	}
	for in, want := range cases {
		got, err := ParseCode(in)
		if err != nil || string(got) != want {
			t.Errorf("ParseCode(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestParseCodeRejects(t *testing.T) {
	for _, in := range []string{
		"", "K7QF-9M2X-4TB", "K7QF-9M2X-4TBDD", "K7QF-9M2X-4TB!", "K7QF-9M2X-4TBU",
		"K7QF-9M2X-4TBé", "K7QF_9M2X_4TBD", "K7QF\t9M2X4TBD",
	} {
		if _, err := ParseCode(in); !errors.Is(err, ErrInvalid) {
			t.Errorf("ParseCode(%q) error = %v; want ErrInvalid", in, err)
		}
	}
}

func TestNewCodeIsDeterministicForAFixedReader(t *testing.T) {
	stream := func() *bytes.Reader {
		b := make([]byte, 24)
		for i := range b {
			b[i] = byte(i * 7)
		}
		return bytes.NewReader(b)
	}
	r := stream()
	first, err := NewCode(r)
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewCode(r)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("two codes from one stream are equal: %s", first)
	}
	again, _ := NewCode(stream())
	if again != first {
		t.Fatalf("same bytes gave %s and %s", again, first)
	}
	if _, err := NewCode(bytes.NewReader([]byte{1, 2})); err == nil {
		t.Fatal("a short reader must fail")
	}
}

func TestFormatRoundTrip(t *testing.T) {
	code, err := NewCode(bytes.NewReader(bytes.Repeat([]byte{0x1f, 0x03, 0x10}, 4)))
	if err != nil {
		t.Fatal(err)
	}
	grouped := code.Format()
	if len(grouped) != 14 || grouped[4] != '-' || grouped[9] != '-' || code.String() != grouped {
		t.Fatalf("unexpected grouped form %q", grouped)
	}
	back, err := ParseCode(grouped)
	if err != nil || back != code || back.Hash() != code.Hash() {
		t.Fatalf("round trip = %q, %v; want %q", back, err, code)
	}
}
