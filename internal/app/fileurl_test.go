package app

import (
	"path/filepath"
	"strings"
)

// localFileURL returns a file:// URL for a local path. On Windows a drive path
// such as C:\x\y must become file:///C:/x/y; file://C:/x/y would parse C: as
// the URL host and the clone would fail.
func localFileURL(path string) string {
	slash := filepath.ToSlash(path)
	if !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	return "file://" + slash
}
