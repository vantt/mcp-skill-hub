package termui

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Wrap splits text into lines of at most width runes at spaces.
// Unbreakable tokens:
//   - a single word longer than width (URL, path, digest) stays whole on its own line;
//   - a span enclosed in backticks (`skillhub skill edit x --trigger "y"`) is never split,
//     so commands quoted inside prose (for example in FIX text) stay copy-pasteable.
//
// Leading/trailing spaces are trimmed per line.
func Wrap(text string, width int) []string {
	if text == "" {
		return nil
	}
	if width <= 0 {
		return []string{text}
	}

	paragraphs := strings.Split(text, "\n")
	var allLines []string

	for _, para := range paragraphs {
		para = strings.TrimSpace(para)
		if para == "" {
			allLines = append(allLines, "")
			continue
		}

		tokens := tokenize(para)
		if len(tokens) == 0 {
			allLines = append(allLines, "")
			continue
		}

		var currentLine string
		currentLineLen := 0

		for _, token := range tokens {
			tokenLen := utf8.RuneCountInString(token)
			if currentLine == "" {
				currentLine = token
				currentLineLen = tokenLen
			} else if currentLineLen+1+tokenLen <= width {
				currentLine += " " + token
				currentLineLen += 1 + tokenLen
			} else {
				allLines = append(allLines, currentLine)
				currentLine = token
				currentLineLen = tokenLen
			}
		}
		if currentLine != "" {
			allLines = append(allLines, currentLine)
		}
	}

	return allLines
}

func tokenize(text string) []string {
	var tokens []string
	runes := []rune(text)
	n := len(runes)
	i := 0
	for i < n {
		for i < n && unicode.IsSpace(runes[i]) {
			i++
		}
		if i >= n {
			break
		}
		start := i
		if runes[i] == '`' {
			i++
			for i < n && runes[i] != '`' {
				i++
			}
			if i < n && runes[i] == '`' {
				i++
			}
			for i < n && !unicode.IsSpace(runes[i]) && runes[i] != '`' {
				i++
			}
			tokens = append(tokens, string(runes[start:i]))
		} else {
			for i < n && !unicode.IsSpace(runes[i]) {
				if runes[i] == '`' {
					break
				}
				i++
			}
			tokens = append(tokens, string(runes[start:i]))
		}
	}
	return tokens
}

// Bytes renders a size with base 1024 and one decimal: 0 B, 512 B, 1.0 KB, 1.4 KB, 183.2 KB, 4.0 MB, 1.0 GB.
func Bytes(n int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	if n < kb {
		return fmt.Sprintf("%d B", n)
	}
	if n < mb {
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	}
	if n < gb {
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	}
	return fmt.Sprintf("%.1f GB", float64(n)/float64(gb))
}

// Plural renders "1 skill" / "3 skills" style counts.
func Plural(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, plural)
}
