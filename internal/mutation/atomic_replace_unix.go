//go:build !windows

package mutation

import "os"

func atomicReplace(source, target string) error {
	return os.Rename(source, target)
}
