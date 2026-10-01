//go:build windows

package source

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsOpenNoFollowRegularFile(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(sub, "test.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	f, err := openNoFollow(root, "sub/test.txt")
	if err != nil {
		t.Fatalf("openNoFollow failed: %v", err)
	}
	defer f.Close()

	buf := make([]byte, 5)
	n, err := f.Read(buf)
	if err != nil || n != 5 || string(buf) != "hello" {
		t.Fatalf("read failed: %d, %s, %v", n, string(buf), err)
	}
}

func TestWindowsOpenNoFollowRejectsEscape(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := openNoFollow(sub, "../other.txt"); !errors.Is(err, ErrInvalidLocator) {
		t.Fatalf("expected ErrInvalidLocator for escape, got %v", err)
	}
}

func TestWindowsOpenNoFollowRejectsSymlinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "target.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		// Symlink creation on Windows may require elevated privileges; skip if not permitted
		t.Skip("skipping symlink test on Windows without symlink privileges")
	}

	_, err := openNoFollow(root, "link.txt")
	if err == nil {
		t.Fatal("expected error opening symlink on Windows")
	}
	if !errors.Is(err, ErrUnsafeFile) && !errors.Is(err, ErrInvalidLocator) {
		t.Fatalf("expected ErrUnsafeFile or ErrInvalidLocator, got: %v", err)
	}
}
