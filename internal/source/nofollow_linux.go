//go:build linux

package source

import (
	"errors"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

func openNoFollow(rootPath, relative string) (*os.File, error) {
	if !safeResourcePath(relative) {
		return nil, ErrInvalidLocator
	}
	rootFD, err := unix.Open(rootPath, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	fd, err := unix.Openat2(rootFD, relative, &unix.OpenHow{
		Flags:   unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW,
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_SYMLINKS | unix.RESOLVE_NO_MAGICLINKS,
	})
	if errors.Is(err, unix.ENOSYS) || errors.Is(err, unix.EINVAL) {
		fd, err = openNoFollowFallback(rootFD, relative)
	}
	_ = unix.Close(rootFD)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), relative), nil
}

func openNoFollowFallback(rootFD int, relative string) (int, error) {
	current, err := unix.Dup(rootFD)
	if err != nil {
		return -1, err
	}
	parts := strings.Split(relative, "/")
	for index, part := range parts {
		flags := unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW
		if index < len(parts)-1 {
			flags |= unix.O_DIRECTORY
		}
		next, openErr := unix.Openat(current, part, flags, 0)
		_ = unix.Close(current)
		if openErr != nil {
			return -1, openErr
		}
		current = next
	}
	return current, nil
}
