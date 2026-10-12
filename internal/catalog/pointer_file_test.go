package catalog

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPointerFileFailuresAreReturnedNotSwallowed(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	closed, err := os.CreateTemp(directory, "closed-*")
	if err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := fillPointerFile(closed, []byte("{}\n"), BuildOptions{}); err == nil {
		t.Fatal("a pointer file that cannot be restricted or written must fail")
	}
	readOnly, err := os.Open(filepath.Join(directory, filepath.Base(closed.Name())))
	if err != nil {
		t.Fatal(err)
	}
	defer readOnly.Close()
	if err := fillPointerFile(readOnly, []byte("{}\n"), BuildOptions{}); err == nil {
		t.Fatal("a failed write of the pointer contents must be returned")
	}
	writable, err := os.CreateTemp(directory, "writable-*")
	if err != nil {
		t.Fatal(err)
	}
	defer writable.Close()
	injected := errors.New("injected sync failure")
	fault := func(point FaultPoint) error {
		if point == FaultPointerTempSync {
			return injected
		}
		return nil
	}
	if err := fillPointerFile(writable, []byte("{}\n"), BuildOptions{Fault: fault}); !errors.Is(err, injected) {
		t.Fatalf("fault before sync = %v", err)
	}
}
