package hostintegration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type tomlStatement struct {
	kind       byte // 'h' for table header, 'a' for assignment
	name       string
	start, end int
}

func upsertCodexTOML(raw []byte, binary, workspace string) ([]byte, error) {
	newline := detectNewline(raw)
	finalNewline := len(raw) > 0 && (raw[len(raw)-1] == '\n' || raw[len(raw)-1] == '\r')
	statements, err := scanTOMLStatements(raw)
	if err != nil {
		return nil, fmt.Errorf("cannot safely update Codex TOML: %w", err)
	}

	sectionStart, sectionEnd := -1, len(raw)
	for index, statement := range statements {
		if statement.kind != 'h' || normalizeTOMLTable(statement.name) != "mcp_servers.skillhub" {
			continue
		}
		if sectionStart >= 0 {
			return nil, fmt.Errorf("duplicate TOML table mcp_servers.skillhub")
		}
		sectionStart = statement.start
		for _, later := range statements[index+1:] {
			if later.kind == 'h' {
				sectionEnd = later.start
				break
			}
		}
	}
	const target = "mcp_servers.skillhub"
	currentTable := ""
	for _, statement := range statements {
		if statement.kind == 'h' {
			currentTable = normalizeTOMLTable(statement.name)
			continue
		}
		full := normalizeTOMLTable(statement.name)
		if currentTable != "" {
			full = currentTable + "." + full
		}
		inTargetTable := currentTable == target || strings.HasPrefix(currentTable, target+".")
		if full == target || (!inTargetTable && strings.HasPrefix(full, target+".")) {
			return nil, fmt.Errorf("ambiguous TOML definition of mcp_servers.skillhub outside a [mcp_servers.skillhub] table; edit it manually")
		}
	}

	managed := []string{
		"command = " + tomlString(binary),
		"args = [" + strings.Join([]string{tomlString("mcp"), tomlString("serve"), tomlString("--workspace"), tomlString(workspace)}, ", ") + "]",
		"cwd = " + tomlString(workspace),
		"enabled = true",
	}
	if sectionStart < 0 {
		block := "[mcp_servers.skillhub]" + newline + strings.Join(managed, newline)
		if len(raw) == 0 {
			return []byte(block + newline), nil
		}
		separator := newline
		if finalNewline {
			separator = ""
		}
		if len(bytes.TrimSpace(raw)) > 0 {
			separator += newline
		}
		result := append(append([]byte(nil), raw...), []byte(separator+block)...)
		if finalNewline {
			result = append(result, []byte(newline)...)
		}
		return result, nil
	}

	section := append([]byte(nil), raw[sectionStart:sectionEnd]...)
	for _, line := range managed {
		key := line[:strings.IndexByte(line, ' ')]
		updated, count, replaceErr := replaceTOMLKey(section, key, line)
		if replaceErr != nil {
			return nil, fmt.Errorf("cannot safely update %s in mcp_servers.skillhub: %w", key, replaceErr)
		}
		if count > 1 {
			return nil, fmt.Errorf("duplicate TOML key %s in mcp_servers.skillhub", key)
		}
		section = updated
		if count == 0 {
			insertAt := len(section)
			for insertAt > 0 && (section[insertAt-1] == '\n' || section[insertAt-1] == '\r') {
				insertAt--
			}
			prefix := newline
			if insertAt == 0 || section[insertAt-1] == '\n' || section[insertAt-1] == '\r' {
				prefix = ""
			}
			section = splice(section, insertAt, insertAt, []byte(prefix+line))
		}
	}
	return splice(raw, sectionStart, sectionEnd, section), nil
}

func normalizeTOMLTable(name string) string {
	parts := strings.Split(name, ".")
	for index := range parts {
		parts[index] = strings.Trim(strings.TrimSpace(parts[index]), `"'`)
	}
	return strings.Join(parts, ".")
}

func replaceTOMLKey(section []byte, key, replacement string) ([]byte, int, error) {
	statements, err := scanTOMLStatements(section)
	if err != nil {
		return nil, 0, err
	}
	var matches []tomlStatement
	for _, statement := range statements {
		if statement.kind == 'a' && normalizeTOMLTable(statement.name) == key {
			matches = append(matches, statement)
		}
	}
	if len(matches) != 1 {
		return section, len(matches), nil
	}
	match := matches[0]
	lineEnding := ""
	if bytes.HasSuffix(section[match.start:match.end], []byte("\r\n")) {
		lineEnding = "\r\n"
	} else if bytes.HasSuffix(section[match.start:match.end], []byte("\n")) {
		lineEnding = "\n"
	}
	return splice(section, match.start, match.end, []byte(replacement+lineEnding)), 1, nil
}

// scanTOMLStatements recognizes complete TOML assignments rather than physical
// lines. This is intentionally conservative: ambiguous or unterminated values
// are rejected instead of risking a partial replacement.
func scanTOMLStatements(raw []byte) ([]tomlStatement, error) {
	var result []tomlStatement
	for offset := 0; offset < len(raw); {
		lineStart := offset
		offset = skipTOMLHorizontal(raw, offset)
		if offset >= len(raw) {
			break
		}
		if raw[offset] == '\r' || raw[offset] == '\n' || raw[offset] == '#' {
			offset = tomlNextLine(raw, offset)
			continue
		}
		if raw[offset] == '[' {
			end, name, err := scanTOMLHeader(raw, offset)
			if err != nil {
				return nil, err
			}
			result = append(result, tomlStatement{kind: 'h', name: name, start: lineStart, end: end})
			offset = end
			continue
		}
		equals, err := findTOMLEquals(raw, offset)
		if err != nil {
			return nil, err
		}
		if equals < 0 {
			return nil, fmt.Errorf("unrecognized statement at byte %d", lineStart)
		}
		name := strings.TrimSpace(string(raw[offset:equals]))
		if name == "" {
			return nil, fmt.Errorf("empty key at byte %d", lineStart)
		}
		end, err := scanTOMLValue(raw, equals+1)
		if err != nil {
			return nil, fmt.Errorf("key %s: %w", name, err)
		}
		result = append(result, tomlStatement{kind: 'a', name: name, start: lineStart, end: end})
		offset = end
	}
	return result, nil
}

func scanTOMLHeader(raw []byte, start int) (int, string, error) {
	arrayTable := start+1 < len(raw) && raw[start+1] == '['
	closing := byte(']')
	for index := start + 1; index < len(raw); index++ {
		if raw[index] != closing {
			continue
		}
		endBracket := index
		if arrayTable {
			if index+1 >= len(raw) || raw[index+1] != ']' {
				continue
			}
			endBracket++
		}
		end, err := finishTOMLStatement(raw, endBracket+1)
		if err != nil {
			return 0, "", err
		}
		nameStart := start + 1
		nameEnd := index
		if arrayTable {
			nameStart++
		}
		return end, strings.TrimSpace(string(raw[nameStart:nameEnd])), nil
	}
	return 0, "", fmt.Errorf("unterminated table header at byte %d", start)
}

func findTOMLEquals(raw []byte, start int) (int, error) {
	quote := byte(0)
	for index := start; index < len(raw); index++ {
		value := raw[index]
		if quote != 0 {
			if value == quote {
				quote = 0
			} else if quote == '"' && value == '\\' {
				index++
			}
			continue
		}
		switch value {
		case '\'', '"':
			quote = value
		case '=':
			return index, nil
		case '\r', '\n', '#':
			return -1, nil
		}
	}
	if quote != 0 {
		return -1, fmt.Errorf("unterminated quoted key")
	}
	return -1, nil
}

func scanTOMLValue(raw []byte, start int) (int, error) {
	start = skipTOMLHorizontal(raw, start)
	if start >= len(raw) || raw[start] == '\r' || raw[start] == '\n' || raw[start] == '#' {
		return 0, fmt.Errorf("missing value")
	}
	switch raw[start] {
	case '"', '\'':
		end, err := scanTOMLString(raw, start)
		if err != nil {
			return 0, err
		}
		return finishTOMLStatement(raw, end)
	case '[', '{':
		end, err := scanTOMLContainer(raw, start)
		if err != nil {
			return 0, err
		}
		return finishTOMLStatement(raw, end)
	default:
		index := start
		for index < len(raw) && raw[index] != '\r' && raw[index] != '\n' && raw[index] != '#' {
			index++
		}
		if len(bytes.TrimSpace(raw[start:index])) == 0 {
			return 0, fmt.Errorf("missing value")
		}
		return finishTOMLStatement(raw, index)
	}
}

func scanTOMLString(raw []byte, start int) (int, error) {
	quote := raw[start]
	triple := start+2 < len(raw) && raw[start+1] == quote && raw[start+2] == quote
	index := start + 1
	if triple {
		index = start + 3
	}
	for index < len(raw) {
		if !triple && (raw[index] == '\r' || raw[index] == '\n') {
			return 0, fmt.Errorf("unterminated string")
		}
		if quote == '"' && raw[index] == '\\' {
			index += 2
			continue
		}
		if raw[index] != quote {
			index++
			continue
		}
		if triple {
			if index+2 < len(raw) && raw[index+1] == quote && raw[index+2] == quote {
				return index + 3, nil
			}
			index++
			continue
		}
		return index + 1, nil
	}
	return 0, fmt.Errorf("unterminated string")
}

func scanTOMLContainer(raw []byte, start int) (int, error) {
	stack := []byte{matchingTOMLDelimiter(raw[start])}
	for index := start + 1; index < len(raw); {
		switch raw[index] {
		case '"', '\'':
			end, err := scanTOMLString(raw, index)
			if err != nil {
				return 0, err
			}
			index = end
		case '#':
			index = tomlNextLine(raw, index)
		case '[', '{':
			stack = append(stack, matchingTOMLDelimiter(raw[index]))
			index++
		case ']', '}':
			if len(stack) == 0 || raw[index] != stack[len(stack)-1] {
				return 0, fmt.Errorf("mismatched container delimiter")
			}
			stack = stack[:len(stack)-1]
			index++
			if len(stack) == 0 {
				return index, nil
			}
		default:
			index++
		}
	}
	return 0, fmt.Errorf("unterminated container")
}

func matchingTOMLDelimiter(open byte) byte {
	if open == '[' {
		return ']'
	}
	return '}'
}

func finishTOMLStatement(raw []byte, index int) (int, error) {
	index = skipTOMLHorizontal(raw, index)
	if index < len(raw) && raw[index] == '#' {
		index = tomlNextLine(raw, index)
		return index, nil
	}
	if index == len(raw) {
		return index, nil
	}
	if raw[index] != '\r' && raw[index] != '\n' {
		return 0, fmt.Errorf("unexpected content after value")
	}
	return tomlNextLine(raw, index), nil
}

func skipTOMLHorizontal(raw []byte, index int) int {
	for index < len(raw) && (raw[index] == ' ' || raw[index] == '\t') {
		index++
	}
	return index
}

func tomlNextLine(raw []byte, index int) int {
	for index < len(raw) && raw[index] != '\n' {
		index++
	}
	if index < len(raw) {
		index++
	}
	return index
}

func tomlString(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
