package canonical

import (
	"fmt"
	"os"
	"time"
)

// FileIdentity is the cheap, comparable identity of one regular file: its size,
// mode, modification and change times and its volume and file index. Writing the
// file, replacing it, renaming another file onto its path or restoring its
// metadata changes at least one of them, except a write that keeps the
// modification time unchanged, which Settled covers.
type FileIdentity struct {
	stamp fileStamp
}

// StatFile returns the identity of a regular file below root without reading its
// contents. A symbolic link or a file of another kind is refused.
func StatFile(root *os.Root, relative string) (FileIdentity, error) {
	info, err := root.Lstat(relative)
	if err != nil {
		return FileIdentity{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return FileIdentity{}, fmt.Errorf("%s is not a regular file", relative)
	}
	stamp := fileStamp{size: info.Size(), mode: info.Mode(), modTime: info.ModTime().UnixNano()}
	if err := addPlatformStamp(&stamp, root, relative, info); err != nil {
		return FileIdentity{}, err
	}
	return FileIdentity{stamp: stamp}, nil
}

// Settled reports whether an unchanged identity proves that the contents seen at
// the given time are still current: the file was last modified clearly before
// that time, so any later write must have moved its modification or change time.
func (identity FileIdentity) Settled(observedAt time.Time) bool {
	return identity.stamp.modTime < observedAt.UnixNano()-int64(racyWindow)
}
