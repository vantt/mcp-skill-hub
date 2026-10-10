package cli

import "strings"

// splitCLI splits a printed command line the way a POSIX shell would for the
// subset shellQuote emits: single-quoted words with '\” for embedded quotes.
// strings.Fields would keep the quotes around Windows paths (backslashes force
// quoting) and hand the literal quote characters to the CLI.
func splitCLI(line string) []string {
	var words []string
	var cur strings.Builder
	inWord, inQuote := false, false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case inQuote:
			if c == '\'' {
				inQuote = false
			} else {
				cur.WriteByte(c)
			}
		case c == '\'':
			inQuote, inWord = true, true
		case c == '\\' && i+1 < len(line):
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
