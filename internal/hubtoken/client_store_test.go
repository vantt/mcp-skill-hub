package hubtoken

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestClientStorePath(t *testing.T) {
	dir := filepath.Join("cfg", "skillhub")
	cases := []struct{ name, url, want string }{
		{"IPv4 with port", "http://100.64.1.2:7420", "100.64.1.2-7420"},
		{"IPv6 loopback", "http://[::1]:7420", "__1-7420"},
		{"IPv6 full", "http://[fd7a:115c::1]:7420", "fd7a_115c__1-7420"},
		{"hostname", "http://Hub.Example.com:9000", "hub.example.com-9000"},
		{"http default port", "http://hub.example.com", "hub.example.com-80"},
		{"https default port", "https://hub.example.com/", "hub.example.com-443"},
		{"odd characters", "http://my_hub!x.test:7420", "myhubx.test-7420"},
		{"unparsable", "://", "hub"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := filepath.Join(dir, "hub-tokens", tc.want)
			if got := ClientStorePath(dir, tc.url); got != want {
				t.Fatalf("ClientStorePath(%q) = %q, want %q", tc.url, got, want)
			}
		})
	}
	if ClientStorePath(dir, "http://[::1]:7420") == ClientStorePath(dir, "http://1:7420") {
		t.Fatal("::1 and 1 share a file")
	}
}

func TestSaveCreatesPrivateFolderAndFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hub-tokens", "hub-7420")
	token := strings.Repeat("ab", 32)
	if err := Save(path, token); err != nil {
		t.Fatal(err)
	}
	got, err := Read(path)
	if err != nil || got != token {
		t.Fatalf("Read = %q, %v", got, err)
	}
	if runtime.GOOS != "windows" {
		for target, want := range map[string]os.FileMode{path: 0o600, filepath.Dir(path): 0o700} {
			info, err := os.Stat(target)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != want {
				t.Fatalf("%s mode = %04o, want %04o", target, info.Mode().Perm(), want)
			}
		}
	}
}

func TestSaveOverwritesAndRejectsMalformedToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	first, second := strings.Repeat("a", 40), strings.Repeat("b", 40)
	if err := Save(path, first); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, second); err != nil {
		t.Fatal(err)
	}
	if got, _ := Read(path); got != second {
		t.Fatalf("token after overwrite = %q", got)
	}
	for _, bad := range []string{"", "short", strings.Repeat("a", 31), strings.Repeat("a", 40) + "\n", strings.Repeat("é", 40)} {
		if err := Save(path, bad); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("Save(%q) = %v, want ErrInvalidToken", bad, err)
		}
	}
	if got, _ := Read(path); got != second {
		t.Fatal("a refused token changed the file")
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("leftover files: %v", entries)
	}
}

func TestCheckWritable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hub-tokens", "hub-7420")
	if err := CheckWritable(path); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 0 {
		t.Fatalf("probe left files: %v", entries)
	}
	if err := CheckWritable(dir); err == nil {
		t.Fatal("a folder was accepted as the token file")
	}
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckWritable(filepath.Join(blocker, "sub", "token")); err == nil {
		t.Fatal("an unusable folder was accepted")
	}
}
