//go:build unix

package migration

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestDetectVersionRejectsFifoMarkerWithoutBlocking(t *testing.T) {
	root := legacyWorkspace(t)
	if err := syscall.Mkfifo(filepath.Join(root, ".skillhub", "schema-version"), 0o644); err != nil {
		t.Skipf("fifo unavailable: %v", err)
	}
	done := make(chan error, 1)
	go func() { _, err := DetectVersion(root); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("fifo marker accepted")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("DetectVersion blocked on fifo marker")
	}
}
