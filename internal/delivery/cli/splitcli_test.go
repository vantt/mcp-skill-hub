package cli

import (
	"runtime"
	"strings"
)

// splitCLI splits a printed command line the way the shell of this platform
// would for the subset shellquote emits: single-quoted words, with a quote
// escaped as '\” in POSIX sh and doubled in PowerShell, where a backslash is
// an ordinary path character. strings.Fields would keep the quotes and hand
// them to the CLI.
func splitCLI(line string) []string {
	windows := runtime.GOOS == "windows"
	var words []string
	var cur strings.Builder
	inWord, inQuote := false, false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case inQuote:
			if c == '\'' && windows && i+1 < len(line) && line[i+1] == '\'' {
				cur.WriteByte(c)
				i++
			} else if c == '\'' {
				inQuote = false
			} else {
				cur.WriteByte(c)
			}
		case c == '\'':
			inQuote, inWord = true, true
		case c == '\\' && !windows && i+1 < len(line):
			i++
			cur.WriteByte(line[i])
			inWord = true
		case c == ' ' || c == '\t' || c == '\n':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteByte(c)
			inWord = true
		}
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words
}
