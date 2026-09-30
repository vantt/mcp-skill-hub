//go:build darwin

package source

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func openNoFollow(rootPath, relative string) (*os.File, error) {
	if !safeResourcePath(relative) {
		return nil, ErrInvalidLocator
	}
	current := rootPath
	for _, component := range strings.Split(relative, "/") {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, ErrInvalidLocator
		}
	}
	file, err := os.OpenFile(current, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		if errors.Is(err, syscall.ELOOP) {
			return nil, ErrInvalidLocator
		}
		return nil, err
	}
	fi, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if !fi.Mode().IsRegular() {
		file.Close()
		return nil, ErrInvalidLocator
	}
	return file, nil
}
