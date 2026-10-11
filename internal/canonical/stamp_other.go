//go:build !linux && !darwin && !windows

package canonical

import (
	"io/fs"
	"os"
)

// addPlatformStamp has no portable change time or file identity here; size,
// modification time and the racy window still apply.
func addPlatformStamp(*fileStamp, *os.Root, string, fs.FileInfo) error { return nil }
