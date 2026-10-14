package invite

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	usedSuffix   = ".used"
	hashFileName = 2 * sha256.Size
)

// FileStore keeps one empty file per pending invite, named hex(hash), whose
// modification time is the expiry. Claim stats the invite and then creates a
// <hex>.used marker exclusively, which lets exactly one caller win across
// threads and processes on every system. Several FileStores on one folder
// cooperate without any lock file.
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

// prune removes expired invite files and the used markers that are old enough
// that the invite they guard would have expired anyway. Failures are ignored:
// a leftover file is only clutter because Claim checks expiry itself.
func (s *FileStore) prune() {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return
	}
	now := s.now()
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		invite := len(name) == hashFileName
		marker := len(name) == hashFileName+len(usedSuffix) && strings.HasSuffix(name, usedSuffix)
		if !invite && !marker {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if (invite && !info.ModTime().After(now)) || (marker && now.Sub(info.ModTime()) > 2*Life) {
			_ = os.Remove(filepath.Join(s.dir, name))
		}
	}
}

// Claim takes the invite if it exists and is unexpired at now. The claim is
// the exclusive creation of <hex>.used, which succeeds for exactly one caller
// on every system, Windows included; the marker is left in place until it is
// old enough that the invite would have expired anyway, so a slow second
// caller cannot create it again. A caller that loses, or finds the invite
// gone, gets "not present", not an error. Removing the invite file itself is
// housekeeping: the marker is what makes the invite single use.
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
	marker, err := os.OpenFile(path+usedSuffix, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return false, nil
	}
	_ = marker.Close()
	_ = os.Remove(path)
	return true, nil
}
