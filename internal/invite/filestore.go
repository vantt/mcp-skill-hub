package invite

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const (
	claimedSuffix = ".claimed"
	hashFileName  = 2 * sha256.Size
)

// FileStore keeps one empty file per pending invite, named hex(hash), whose
// modification time is the expiry. No file is ever opened to claim it: Claim
// stats it and renames it, which is atomic, lets exactly one caller win across
// threads and processes, and works on Windows where removing an open file
// fails. Several FileStores on one folder cooperate without any lock file.
type FileStore struct {
	dir string
	now func() time.Time
}

// NewFileStore uses dir as the invite folder. It creates the folder with mode
// 0700 and refuses a symlink or, outside Windows, a folder that group or other
// users can access.
func NewFileStore(dir string) (*FileStore, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create the invite folder: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, fmt.Errorf("inspect the invite folder: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("the invite folder must be a real directory, not a symlink")
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		return nil, errors.New("the invite folder is accessible to other users")
	}
	return &FileStore{dir: dir, now: time.Now}, nil
}

func (s *FileStore) path(hash [32]byte) string {
	return filepath.Join(s.dir, hex.EncodeToString(hash[:]))
}

// Put writes the invite through a temp file so a half-written invite is never
// visible, then removes invites that have expired.
func (s *FileStore) Put(hash [32]byte, expires time.Time) error {
	tmp, err := os.CreateTemp(s.dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("write an invite: %w", err)
	}
	tmpName := tmp.Name()
	closeErr := tmp.Close()
	for _, step := range []func() error{
		func() error { return closeErr },
		func() error { return os.Chtimes(tmpName, expires, expires) },
		func() error { return os.Chmod(tmpName, 0o600) },
		func() error { return os.Rename(tmpName, s.path(hash)) },
	} {
		if err := step(); err != nil {
			_ = os.Remove(tmpName)
			return fmt.Errorf("write an invite: %w", err)
		}
	}
	s.prune()
	return nil
}

// prune removes expired invite files. Failures are ignored: a leftover file is
// only clutter because Claim checks expiry itself.
func (s *FileStore) prune() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	now := s.now()
	for _, e := range entries {
		if len(e.Name()) != hashFileName || e.IsDir() {
			continue
		}
		if info, err := e.Info(); err == nil && !info.ModTime().After(now) {
			_ = os.Remove(filepath.Join(s.dir, e.Name()))
		}
	}
}

// Claim takes the invite if it exists and is unexpired at now. A failed
// rename means another caller won or the file vanished: that is "not
// present", not an error.
func (s *FileStore) Claim(hash [32]byte, now time.Time) (bool, error) {
	path := s.path(hash)
	info, err := os.Stat(path)
	if err != nil {
		return false, nil
	}
	if !info.ModTime().After(now) {
		_ = os.Remove(path)
		return false, nil
	}
	claimed := path + claimedSuffix
	if err := os.Rename(path, claimed); err != nil {
		return false, nil
	}
	_ = os.Remove(claimed)
	return true, nil
}
