//go:build !windows

package catalog

import "os"

func atomicReplace(source, target string) error { return os.Rename(source, target) }
