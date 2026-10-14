package invite

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func hashOf(b byte) [32]byte { return [32]byte{b, 1, 2, 3} }

func newTestFileStore(t *testing.T) (*FileStore, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "invites")
	fs, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	return fs, dir
}

func TestFileStoreModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits are not enforced on Windows")
	}
	fs, dir := newTestFileStore(t)
	h := hashOf(1)
	if err := fs.Put(h, time.Now().Add(Life)); err != nil {
		t.Fatal(err)
	}
	di, _ := os.Stat(dir)
	fi, err := os.Stat(fs.path(h))
	if err != nil {
		t.Fatal(err)
	}
	if di.Mode().Perm() != 0o700 || fi.Mode().Perm() != 0o600 {
		t.Fatalf("modes = %04o folder, %04o file; want 0700, 0600", di.Mode().Perm(), fi.Mode().Perm())
	}
	if fi.Size() != 0 {
		t.Fatalf("invite file holds %d bytes, want none", fi.Size())
	}
}

func TestFileStoreClaimTwice(t *testing.T) {
	fs, dir := newTestFileStore(t)
	h := hashOf(2)
	if err := fs.Put(h, time.Now().Add(Life)); err != nil {
		t.Fatal(err)
	}
	if ok, err := fs.Claim(h, time.Now()); !ok || err != nil {
		t.Fatalf("first claim = %v, %v", ok, err)
	}
	if ok, err := fs.Claim(h, time.Now()); ok || err != nil {
		t.Fatalf("second claim = %v, %v", ok, err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || !strings.HasSuffix(entries[0].Name(), usedSuffix) {
		t.Fatalf("after a claim only the used marker should remain, got %d files", len(entries))
	}
}

func TestFileStoreExpiredIsRemovedAndNotClaimable(t *testing.T) {
	fs, _ := newTestFileStore(t)
	h := hashOf(3)
	if err := fs.Put(h, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if ok, err := fs.Claim(h, time.Now().Add(time.Hour)); ok || err != nil {
		t.Fatalf("expired claim = %v, %v", ok, err)
	}
	if _, err := os.Stat(fs.path(h)); !os.IsNotExist(err) {
		t.Fatalf("expired invite still on disk: %v", err)
	}
	if ok, _ := fs.Claim(hashOf(99), time.Now()); ok {
		t.Fatal("unknown invite claimed")
	}
}

func TestFileStorePutRemovesExpiredInvites(t *testing.T) {
	fs, dir := newTestFileStore(t)
	old := hashOf(4)
	if err := fs.Put(old, time.Now().Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	fs.now = func() time.Time { return time.Now().Add(time.Hour) }
	fresh := hashOf(5)
	if err := fs.Put(fresh, time.Now().Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fs.path(old)); !os.IsNotExist(err) {
		t.Fatalf("expired invite survived Put: %v", err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Fatalf("%d files after Put, want only the fresh invite", len(entries))
	}
}

func TestFileStoreConcurrentClaimHasOneWinner(t *testing.T) {
	fs, _ := newTestFileStore(t)
	const claimers = 20
	for round := 0; round < 50; round++ {
		h := hashOf(byte(round))
		if err := fs.Put(h, time.Now().Add(Life)); err != nil {
			t.Fatal(err)
		}
		var wins int
		var mu sync.Mutex
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < claimers; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				if ok, err := fs.Claim(h, time.Now()); err != nil {
					t.Error(err)
				} else if ok {
					mu.Lock()
					wins++
					mu.Unlock()
				}
			}()
		}
		close(start)
		wg.Wait()
		if wins != 1 {
			t.Fatalf("round %d: %d claimers won, want exactly 1", round, wins)
		}
	}
}

func TestFileStoreClaimedInviteStaysUsedUntilItWouldHaveExpired(t *testing.T) {
	fs, _ := newTestFileStore(t)
	h := hashOf(9)
	if err := fs.Put(h, time.Now().Add(Life)); err != nil {
		t.Fatal(err)
	}
	if ok, _ := fs.Claim(h, time.Now()); !ok {
		t.Fatal("first claim lost")
	}
	// Even if the invite file reappears (a slow writer, a restored backup), the used marker blocks a second claim.
	if err := fs.Put(h, time.Now().Add(Life)); err != nil {
		t.Fatal(err)
	}
	if ok, _ := fs.Claim(h, time.Now()); ok {
		t.Fatal("a used invite was claimed again")
	}
}

func TestFileStoreSharesInvitesAcrossInstances(t *testing.T) {
	first, dir := newTestFileStore(t)
	second, err := NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	h := hashOf(7)
	if err := first.Put(h, time.Now().Add(Life)); err != nil {
		t.Fatal(err)
	}
	if ok, _ := second.Claim(h, time.Now()); !ok {
		t.Fatal("second store did not see the first store's invite")
	}
	if ok, _ := first.Claim(h, time.Now()); ok {
		t.Fatal("invite claimed through both stores")
	}
}

func TestNewFileStoreRefusesSymlink(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "real")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := NewFileStore(link); err == nil {
		t.Fatal("a symlinked invite folder must be refused")
	}
}

func TestNewFileStoreRefusesOpenPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits are not enforced on Windows")
	}
	dir := filepath.Join(t.TempDir(), "open")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewFileStore(dir); err == nil {
		t.Fatal("a group or other readable folder must be refused")
	}
}

func TestServiceOverFileStore(t *testing.T) {
	fs, _ := newTestFileStore(t)
	svc := NewService(fs, nil, nil)
	code, _, err := svc.Issue()
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Redeem(code.String()); err != nil {
		t.Fatal(err)
	}
	if err := svc.Redeem(code.String()); err != ErrInvalid {
		t.Fatalf("reuse = %v, want ErrInvalid", err)
	}
}
