package app

import "testing"

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
