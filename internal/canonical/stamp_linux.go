package canonical

import (
	"io/fs"
	"os"
	"syscall"
)

// addPlatformStamp adds the status change time and file identity, which every
// content write, metadata restore and replacement of the path changes.
func addPlatformStamp(stamp *fileStamp, _ *os.Root, _ string, info fs.FileInfo) error {
	if status, ok := info.Sys().(*syscall.Stat_t); ok {
		stamp.changeTime = status.Ctim.Nano()
		stamp.device = uint64(status.Dev) //nolint:unconvert // Dev is narrower than uint64 on some Linux architectures.
		stamp.inode = uint64(status.Ino)  //nolint:unconvert // Ino is narrower than uint64 on some Linux architectures.
	}
	return nil
}
