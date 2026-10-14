package hostintegration

import (
	"bytes"
	"fmt"
	"regexp"

	"github.com/vantt/skillhub-mcp/internal/systemskills"
)

// Older versions wrote a Skill Hub instruction block between these markers
// into CLAUDE.md, AGENTS.md and GEMINI.md. The protocol now reaches agents as
// MCP server instructions, so the block is obsolete: connect never writes it
// and removes any block of any marker version it finds.
var bootstrapMarkerPattern = regexp.MustCompile(`<!--[ \t]*skillhub:bootstrap:v([^:>[:space:]]+):(start|end)[ \t]*-->`)

func systemCuratorInstructions() string { return systemskills.CuratorBundle().Instructions }

// bootstrapSpan locates the single obsolete block, markers included. A file
// without markers reports found=false. Nested, duplicate, reversed, unmatched
// or version-mismatched markers report a conflict.
func bootstrapSpan(raw []byte) (start, end int, found bool, conflict string) {
	matches := bootstrapMarkerPattern.FindAllSubmatchIndex(raw, -1)
	if len(matches) == 0 {
		return 0, 0, false, ""
	}
	depth := 0
	blocks := 0
	open := -1
	version := ""
	blockStart, blockEnd := -1, -1
	for _, match := range matches {
		kind := string(raw[match[4]:match[5]])
		markerVersion := string(raw[match[2]:match[3]])
		switch kind {
		case "start":
			if depth != 0 {
				return 0, 0, false, "nested Skill Hub bootstrap blocks"
			}
			depth = 1
			open = match[0]
			version = markerVersion
		case "end":
			if depth == 0 {
				return 0, 0, false, "reversed or unmatched Skill Hub bootstrap end marker"
			}
			if markerVersion != version {
				return 0, 0, false, fmt.Sprintf("bootstrap marker version mismatch: v%s start and v%s end", version, markerVersion)
			}
			depth = 0
			blocks++
			if blocks > 1 {
				return 0, 0, false, "duplicate Skill Hub bootstrap blocks"
			}
			blockStart, blockEnd = open, match[1]
		}
	}
	if depth != 0 {
		return 0, 0, false, "unmatched Skill Hub bootstrap start marker"
	}
	if blocks != 1 || blockStart < 0 {
		return 0, 0, false, "invalid Skill Hub bootstrap markers"
	}
	return blockStart, blockEnd, true, ""
}

// IsBootstrapOnlyFile reports that raw holds an obsolete block and nothing
// else, which is the file an older connect created. init uses it to accept
// such a file in a fetched hub as Skill Hub's own.
func IsBootstrapOnlyFile(raw []byte) bool {
	if _, _, found, conflict := bootstrapSpan(raw); !found || conflict != "" {
		return false
	}
	desired, conflict := removeBootstrap(raw)
	return conflict == "" && len(bytes.TrimSpace(desired)) == 0
}
