package hubtoken

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnsureCreatesPrivateTokenOnce(t *testing.T) {
	path := DefaultPath(t.TempDir())
	created, err := Ensure(path)
	if err != nil || !created {
		t.Fatalf("Ensure = %v, %v; want created", created, err)
	}
	first, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2*tokenBytes {
		t.Fatalf("token length = %d, want %d", len(first), 2*tokenBytes)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("token file mode = %04o, want 0600", info.Mode().Perm())
		}
	}
	created, err = Ensure(path)
	if err != nil || created {
		t.Fatalf("second Ensure = %v, %v; want existing token kept", created, err)
	}
	second, err := Read(path)
	if err != nil || second != first {
		t.Fatalf("second Ensure changed the token (err %v)", err)
	}
}

func TestRotateReplacesToken(t *testing.T) {
	path := DefaultPath(t.TempDir())
	if _, err := Ensure(path); err != nil {
		t.Fatal(err)
	}
	before, _ := Read(path)
	if err := Rotate(path); err != nil {
		t.Fatal(err)
	}
	after, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if after == before {
		t.Fatal("Rotate kept the old token")
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("Rotate left temporary files: %v", entries)
	}
}

func TestReadRejectsUnsafeFiles(t *testing.T) {
	directory := t.TempDir()
	short := filepath.Join(directory, "short")
	if err := os.WriteFile(short, []byte("abc\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(short); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("short token error = %v, want ErrInvalidToken", err)
	}
	spaced := filepath.Join(directory, "spaced")
	if err := os.WriteFile(spaced, []byte("0123456789abcdef 0123456789abcdef0123456789\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(spaced); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("token with a space error = %v, want ErrInvalidToken", err)
	}
	if _, err := Read(filepath.Join(directory, "missing")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing file error = %v, want not exist", err)
	}
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits and symlinks are not checked on Windows")
	}
	open := filepath.Join(directory, "open")
	if err := os.WriteFile(open, []byte("0123456789abcdef0123456789abcdef0123456789\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(open, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(open); !errors.Is(err, ErrInsecureMode) {
		t.Fatalf("world-readable token error = %v, want ErrInsecureMode", err)
	}
	if _, err := Ensure(open); !errors.Is(err, ErrInsecureMode) {
		t.Fatalf("Ensure over a world-readable token = %v, want ErrInsecureMode", err)
	}
	link := filepath.Join(directory, "link")
	if err := os.Symlink(short, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(link); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("symlinked token error = %v, want ErrInvalidToken", err)
	}
}

func TestSourceFollowsRotation(t *testing.T) {
	path := DefaultPath(t.TempDir())
	if _, err := Ensure(path); err != nil {
		t.Fatal(err)
	}
	source := NewSource(path)
	old, _ := Read(path)
	if !source.Valid(old) {
		t.Fatal("current token rejected")
	}
	if source.Valid("") || source.Valid(old+"x") || source.Valid(old[:len(old)-1]) {
		t.Fatal("a wrong token was accepted")
	}
	if err := Rotate(path); err != nil {
		t.Fatal(err)
	}
	fresh, _ := Read(path)
	if source.Valid(old) {
		t.Fatal("old token still accepted after rotation")
	}
	if !source.Valid(fresh) {
		t.Fatal("rotated token rejected")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if source.Valid(fresh) {
		t.Fatal("token accepted after the token file was removed")
	}
	if err := source.Check(); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Check after removal = %v, want not exist", err)
	}
}

func TestClientPathIsOneFilePerHub(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ url, want string }{
		{"http://192.168.1.20:7420", "~/.skillhub-token-192.168.1.20-7420"},
		{"http://hub.tailnet.ts.net:7420/", "~/.skillhub-token-hub.tailnet.ts.net-7420"},
		{"http://[fd7a:115c::1]:7420", "~/.skillhub-token-fd7a115c1-7420"},
		{"https://hub.example", "~/.skillhub-token-hub.example-443"},
		{"http://host_name:7420", "~/.skillhub-token-hostname-7420"},
	} {
		if got := ClientPath(test.url); got != test.want {
			t.Errorf("ClientPath(%q) = %q, want %q", test.url, got, test.want)
		}
	}
}
