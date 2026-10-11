// Package shellquote quotes the arguments of commands that Skill Hub prints
// for the user to copy and run, so a pasted command receives each argument
// unchanged in the shell of this platform.
package shellquote

import (
	"runtime"
	"strings"
)

// Arg returns value as one argument for the shell of this platform: POSIX sh
// on Linux and macOS, PowerShell on Windows. A value made only of characters
// that no shell treats specially is returned as it is.
func Arg(value string) string { return ArgFor(runtime.GOOS, value) }

// ArgFor is Arg for the shell of the named GOOS.
func ArgFor(goos, value string) string {
	windows := goos == "windows"
	if value != "" && strings.IndexFunc(value, func(r rune) bool { return !plain(r, windows) }) < 0 {
		return value
	}
	if windows {
		// Inside a PowerShell single-quoted string only a single quote is
		// special, and PowerShell also reads the typographic single quotes as
		// one; each is escaped by doubling it.
		var quoted strings.Builder
		quoted.WriteByte('\'')
		for _, r := range value {
			if strings.ContainsRune("'‘’‚‛", r) {
				quoted.WriteRune(r)
			}
			quoted.WriteRune(r)
		}
		quoted.WriteByte('\'')
		return quoted.String()
	}
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

// plain reports whether r needs no quoting. A backslash separates path
// elements on Windows and is an ordinary character in PowerShell.
func plain(r rune, windows bool) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case strings.ContainsRune("/._-:+%", r):
		return true
	case r == '\\':
		return windows
	}
	return false
}

// Chain joins commands that run one after another in the shell of this
// platform: `a && b` in POSIX sh, `a; b` in PowerShell, because Windows
// PowerShell 5.1 has no `&&`.
func Chain(commands ...string) string { return ChainFor(runtime.GOOS, commands...) }

// ChainFor is Chain for the shell of the named GOOS.
func ChainFor(goos string, commands ...string) string {
	if goos == "windows" {
		return strings.Join(commands, "; ")
	}
	return strings.Join(commands, " && ")
}

// MakeDir returns the command that creates dir with its parents and succeeds
// when it already exists. dir is inserted as it is, so a caller can pass a
// ~ path for the shell to expand.
func MakeDir(dir string) string { return MakeDirFor(runtime.GOOS, dir) }

// MakeDirFor is MakeDir for the shell of the named GOOS.
func MakeDirFor(goos, dir string) string {
	if goos == "windows" {
		return "mkdir -Force " + dir
	}
	return "mkdir -p " + dir
}
