//go:build unix

package workspace

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestReadCanonicalFileRejectsFifoWithoutBlocking(t *testing.T) {
	root := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(root, "fifo"), 0o644); err != nil {
		t.Skipf("fifo unavailable: %v", err)
	}
	done := make(chan error, 1)
	go func() { _, err := ReadCanonicalFile(root, "fifo"); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("fifo was read")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ReadCanonicalFile blocked on fifo")
	}
}
