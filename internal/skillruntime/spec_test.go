package skillruntime

import (
	"reflect"
	"testing"
)

func TestParseSpec(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Spec
		has     bool
		wantErr bool
	}{
		{name: "no runtime block", input: `{"id":"demo"}`, has: false},
		{name: "null runtime block", input: `{"runtime":null}`, has: false},
		{name: "empty runtime block", input: `{"runtime":{}}`, has: true},
		{
			name:  "string and object bins",
			input: `{"runtime":{"requires":{"bins":["git",{"name":"node","version":">=18"}],"env":["API_TOKEN"],"platforms":["linux","darwin"]},"setup":{"command":"npm ci","check":"node -e 1"}}}`,
			want: Spec{
				Requires: Requires{
					Bins:      []Bin{{Name: "git"}, {Name: "node", Version: ">=18"}},
					Env:       []string{"API_TOKEN"},
					Platforms: []string{"linux", "darwin"},
				},
				Setup: Setup{Command: "npm ci", Check: "node -e 1"},
			},
			has: true,
		},
		{name: "bare version constraint", input: `{"runtime":{"requires":{"bins":[{"name":"python3","version":"3.11"}]}}}`, want: Spec{Requires: Requires{Bins: []Bin{{Name: "python3", Version: "3.11"}}}}, has: true},
		{name: "invalid json", input: `{`, wantErr: true},
		{name: "unknown runtime key", input: `{"runtime":{"install":"x"}}`, wantErr: true},
		{name: "unknown bin key", input: `{"runtime":{"requires":{"bins":[{"name":"git","path":"/x"}]}}}`, wantErr: true},
		{name: "bin name with path", input: `{"runtime":{"requires":{"bins":["/usr/bin/git"]}}}`, wantErr: true},
		{name: "duplicate bin", input: `{"runtime":{"requires":{"bins":["git",{"name":"git"}]}}}`, wantErr: true},
		{name: "bad version constraint", input: `{"runtime":{"requires":{"bins":[{"name":"git","version":"latest"}]}}}`, wantErr: true},
		{name: "bad env name", input: `{"runtime":{"requires":{"env":["A-B"]}}}`, wantErr: true},
		{name: "unknown platform", input: `{"runtime":{"requires":{"platforms":["plan9"]}}}`, wantErr: true},
		{name: "multiline command", input: `{"runtime":{"setup":{"check":"a\nb"}}}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, has, err := ParseSpec([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if has != tt.has {
				t.Fatalf("has = %v, want %v", has, tt.has)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("spec = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestSpecFingerprintIgnoresListOrder(t *testing.T) {
	a := Spec{Requires: Requires{Bins: []Bin{{Name: "git"}, {Name: "node", Version: ">=18"}}, Env: []string{"B", "A"}, Platforms: []string{"linux", "darwin"}}}
	b := Spec{Requires: Requires{Bins: []Bin{{Name: "node", Version: ">=18"}, {Name: "git"}}, Env: []string{"A", "B"}, Platforms: []string{"darwin", "linux"}}}
	if a.Fingerprint() != b.Fingerprint() {
		t.Fatal("reordered lists changed the fingerprint")
	}
	if len(a.Fingerprint()) != 64 {
		t.Fatalf("fingerprint %q is not 64 hex chars", a.Fingerprint())
	}
	c := a
	c.Setup.Check = "true"
	if a.Fingerprint() == c.Fingerprint() {
		t.Fatal("changed setup command kept the fingerprint")
	}
	if (Spec{}).Fingerprint() != (Spec{Requires: Requires{Env: []string{}}}).Fingerprint() {
		t.Fatal("nil and empty lists fingerprint differently")
	}
}
