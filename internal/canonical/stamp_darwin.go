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
		stamp.changeTime = status.Ctimespec.Nano()
		stamp.device = uint64(uint32(status.Dev))
		stamp.inode = status.Ino
	}
	return nil
}
