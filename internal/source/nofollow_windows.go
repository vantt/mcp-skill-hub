//go:build windows

package source

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func openNoFollow(rootPath, relative string) (*os.File, error) {
	if !safeResourcePath(relative) {
		return nil, ErrInvalidLocator
	}

	rootPathU16, err := windows.UTF16PtrFromString(rootPath)
	if err != nil {
		return nil, err
	}
	rootHandle, err := windows.CreateFile(
		rootPathU16,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(rootHandle)

	var rootInfo windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(rootHandle, &rootInfo); err != nil {
		return nil, err
	}
	if rootInfo.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return nil, fmt.Errorf("%w: root is a reparse point", ErrInvalidLocator)
	}

	rootFinalPath, err := getFinalPath(rootHandle)
	if err != nil {
		return nil, err
	}

	components := strings.Split(relative, "/")
	currentPath := rootPath
	for index, comp := range components {
		currentPath = filepath.Join(currentPath, comp)
		isLast := (index == len(components)-1)

		pathU16, err := windows.UTF16PtrFromString(currentPath)
		if err != nil {
			return nil, err
		}

		flags := uint32(windows.FILE_FLAG_OPEN_REPARSE_POINT)
		if !isLast {
			flags |= windows.FILE_FLAG_BACKUP_SEMANTICS
		}

		h, err := windows.CreateFile(
			pathU16,
			windows.GENERIC_READ,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
			nil,
			windows.OPEN_EXISTING,
			flags,
			0,
		)
		if err != nil {
			return nil, err
		}

		var fi windows.ByHandleFileInformation
		if err := windows.GetFileInformationByHandle(h, &fi); err != nil {
			windows.CloseHandle(h)
			return nil, err
		}

		if fi.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			windows.CloseHandle(h)
			return nil, fmt.Errorf("%w: unsafe source reparse point: %s", ErrUnsafeFile, comp)
		}
		if fi.FileAttributes&windows.FILE_ATTRIBUTE_DEVICE != 0 {
			windows.CloseHandle(h)
			return nil, fmt.Errorf("%w: unsupported source object: %s", ErrUnsafeFile, comp)
		}

		if !isLast {
			if fi.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY == 0 {
				windows.CloseHandle(h)
				return nil, ErrInvalidLocator
			}
		} else {
			if fi.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0 {
				windows.CloseHandle(h)
				return nil, ErrInvalidLocator
			}
		}

		finalPath, err := getFinalPath(h)
		if err != nil {
			windows.CloseHandle(h)
			return nil, err
		}

		if !isInsidePath(rootFinalPath, finalPath) {
			windows.CloseHandle(h)
			return nil, fmt.Errorf("%w: path escape detected: %s", ErrInvalidLocator, comp)
		}

		if !isLast {
			windows.CloseHandle(h)
		} else {
			return os.NewFile(uintptr(h), relative), nil
		}
	}

	return nil, ErrInvalidLocator
}

func getFinalPath(h windows.Handle) (string, error) {
	var buf [windows.MAX_LONG_PATH]uint16
	n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
	if err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf[:n]), nil
}

func isInsidePath(root, target string) bool {
	cleanRoot := strings.TrimSuffix(strings.ToLower(filepath.Clean(root)), `\`) + `\`
	cleanTarget := strings.ToLower(filepath.Clean(target))
	return strings.HasPrefix(cleanTarget, cleanRoot)
}
