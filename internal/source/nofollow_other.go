//go:build !linux

package source

import (
	"os"
	"path/filepath"
	"strings"
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
	return os.Open(current)
}
