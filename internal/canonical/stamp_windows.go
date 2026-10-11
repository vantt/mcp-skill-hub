package canonical

import (
	"io/fs"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// fileBasicInfo mirrors FILE_BASIC_INFO, the FileBasicInfo information class.
type fileBasicInfo struct {
	CreationTime   int64
	LastAccessTime int64
	LastWriteTime  int64
	ChangeTime     int64
	FileAttributes uint32
	_              uint32
}

// addPlatformStamp adds the NTFS change time and the volume and file index, which
// every content write, metadata restore and replacement of the path changes. A
// directory listing does not report them, so the file is opened, but not read.
func addPlatformStamp(stamp *fileStamp, root *os.Root, relative string, _ fs.FileInfo) error {
	file, err := root.Open(relative)
	if err != nil {
		return err
	}
	defer file.Close()
	handle := windows.Handle(file.Fd())
	var basic fileBasicInfo
	if err := windows.GetFileInformationByHandleEx(handle, windows.FileBasicInfo, (*byte)(unsafe.Pointer(&basic)), uint32(unsafe.Sizeof(basic))); err != nil {
		return err
	}
	var identity windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &identity); err != nil {
		return err
	}
	stamp.changeTime = basic.ChangeTime
	stamp.device = uint64(identity.VolumeSerialNumber)
	stamp.inode = uint64(identity.FileIndexHigh)<<32 | uint64(identity.FileIndexLow)
	return nil
}
