package app

import (
	"strings"
	"testing"
)

func TestValidateClientLabelAcceptsWhatTheHubShowsUnchanged(t *testing.T) {
	t.Parallel()
	for _, label := range []string{"laptop", "Ann's laptop", "máy-tính", strings.Repeat("a", 64), strings.Repeat("é", 64)} {
		if err := ValidateClientLabel(label); err != nil {
			t.Errorf("ValidateClientLabel(%q) = %v", label, err)
		}
		if cleaned := cleanClientLabel(label); cleaned != label {
			t.Errorf("the hub shows %q as %q", label, cleaned)
		}
	}
	for _, label := range []string{"", "   ", "lap\ntop", "lap\ttop", "lap\x00top", "lap\u200btop", "lap\u2028top", strings.Repeat("a", 65)} {
		if err := ValidateClientLabel(label); err == nil {
			t.Errorf("ValidateClientLabel(%q) accepted it", label)
		}
	}
}

func TestMachineLabelDropsALocalNetworkSuffix(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ in, want string }{
		{"sjc22-bm211-fd82d2e3-5478-46f3-b781-843a1dd8779e-C66A5E8302EF.local", "sjc22-bm211-fd82d2e3-5478-46f3-b781-843a1dd8779e-C66A5E8302EF"},
		{"sjc22-bm211-fd82d2e3-5478-46f3-b781-843a1dd8779e-C66A5E8302EF", "sjc22-bm211-fd82d2e3-5478-46f3-b781-843a1dd8779e-C66A5E8302EF"},
		{"laptop.lan", "laptop"},
		{"laptop.home", "laptop"},
		{"laptop.localdomain", "laptop"},
		{"Laptop.LOCAL", "Laptop"},
		{"build.example.com", "build.example.com"},
		{"localhost", "localhost"},
		{"my.localdomain.example", "my.localdomain.example"},
		{"máy-tính.local", "máy-tính"},
		{".local", ".local"},
		{"  desk.lan  ", "desk"},
		{"", ""},
	} {
		if got := machineLabel(tc.in); got != tc.want {
			t.Errorf("machineLabel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
