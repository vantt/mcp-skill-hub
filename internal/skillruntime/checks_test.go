package skillruntime

import (
	"errors"
	"reflect"
	"testing"
)

const secretSentinel = "s3cr3t-sentinel-value"

func fakeEnv(values map[string]string) LookupEnv {
	return func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}
}

func fakeLook(found ...string) LookPath {
	return func(file string) (string, error) {
		for _, name := range found {
			if name == file {
				return "/usr/bin/" + file, nil
			}
		}
		return "", errors.New("not found")
	}
}

func TestPlatformChecks(t *testing.T) {
	spec := Spec{Requires: Requires{
		Bins:      []Bin{{Name: "git"}},
		Env:       []string{"API_TOKEN"},
		Platforms: []string{"linux", "darwin"},
	}}
	got := PlatformChecks(spec, "linux")
	want := []Check{{Kind: KindPlatform, Name: "linux", Status: StatusPass}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("checks = %+v, want only the platform check %+v", got, want)
	}
	failing := PlatformChecks(Spec{Requires: Requires{Platforms: []string{"darwin"}}}, "windows")
	if len(failing) != 1 || failing[0].Status != StatusFail || failing[0].Detail != "requires darwin" {
		t.Fatalf("checks = %+v", failing)
	}
	if got := PlatformChecks(Spec{}, "windows"); len(got) != 0 {
		t.Fatalf("undeclared platform produced checks: %+v", got)
	}
	if got := PlatformChecks(Spec{Requires: Requires{Bins: []Bin{{Name: "git"}}, Env: []string{"X"}}}, "linux"); len(got) != 0 {
		t.Fatalf("bins and env must not be checked by the hub: %+v", got)
	}
}

func TestVersionConstraint(t *testing.T) {
	tests := []struct {
		constraint string
		have       string
		want       bool
	}{
		{">= 3.9", "3.10.2", true},
		{">=3.9", "3.8.18", false},
		{">18", "18.0.0", false},
		{">18", "18.0.1", true},
		{"<=1.2", "1.2.0", true},
		{"<2", "2.0", false},
		{"=1.2.3", "1.2.3", true},
		{"=1.2", "1.2.1", false},
		{"18", "20.1.0", true},
		{"18", "16.4", false},
	}
	for _, tt := range tests {
		c, err := parseConstraint(tt.constraint)
		if err != nil {
			t.Fatalf("parseConstraint(%q): %v", tt.constraint, err)
		}
		_, have, ok := extractVersion(tt.have)
		if !ok {
			t.Fatalf("extractVersion(%q) failed", tt.have)
		}
		if got := c.satisfiedBy(have); got != tt.want {
			t.Errorf("%q satisfied by %q = %v, want %v", tt.constraint, tt.have, got, tt.want)
		}
	}
	for _, bad := range []string{"", "latest", ">=", "1.2.3.4", "~1.2"} {
		if _, err := parseConstraint(bad); err == nil {
			t.Errorf("parseConstraint(%q) accepted", bad)
		}
	}
}

func TestExtractVersion(t *testing.T) {
	tests := map[string]string{
		"Python 3.12.1":                    "3.12.1",
		"v20.11.0\n":                       "20.11.0",
		"git version 2.43.0 (Apple Git-1)": "2.43.0",
		"jq-1.7":                           "1.7",
	}
	for output, want := range tests {
		got, _, ok := extractVersion(output)
		if !ok || got != want {
			t.Errorf("extractVersion(%q) = %q, %v; want %q", output, got, ok, want)
		}
	}
	if _, _, ok := extractVersion("no digits here"); ok {
		t.Error("extractVersion accepted output without a version")
	}
}
