package hostintegration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
)

type jsonMember struct {
	key                  string
	valueStart, valueEnd int
}

func upsertJSONPath(raw []byte, path []string, encoded []byte) ([]byte, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("JSON path is empty")
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = []byte("{}")
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if root == nil {
		return nil, fmt.Errorf("JSON root must be an object")
	}
	if !json.Valid(encoded) {
		return nil, fmt.Errorf("managed JSON value is invalid")
	}
	updated, err := upsertJSONObject(raw, 0, len(raw), nil, path, encoded)
	if err != nil {
		return nil, err
	}
	var verification map[string]json.RawMessage
	if err := json.Unmarshal(updated, &verification); err != nil {
		return nil, fmt.Errorf("generated invalid JSON: %w", err)
	}
	return updated, nil
}

// upsertJSONObject sets path inside the object at raw[start:end]. prefix holds
// the keys already traversed and is used only for error messages. When the
// terminal value and the existing value are both objects, only the keys in
// encoded are set so unmanaged sibling keys survive.
func upsertJSONObject(raw []byte, start, end int, prefix, path []string, encoded []byte) ([]byte, error) {
	open := skipJSONSpace(raw, start, end)
	if open >= end || raw[open] != '{' {
		location := strings.Join(prefix, ".")
		if location == "" {
			location = "(root)"
		}
		return nil, fmt.Errorf("JSON path %q is not an object", location)
	}
	members, closeIndex, err := parseJSONObject(raw, open, end)
	if err != nil {
		return nil, err
	}
	var found *jsonMember
	for index := range members {
		if members[index].key != path[0] {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("duplicate JSON key %q", path[0])
		}
		found = &members[index]
	}
	if found != nil {
		traversed := append(append([]string(nil), prefix...), path[0])
		if len(path) == 1 {
			return mergeJSONValue(raw, found.valueStart, found.valueEnd, traversed, encoded)
		}
		return upsertJSONObject(raw, found.valueStart, found.valueEnd, traversed, path[1:], encoded)
	}

	subtree := encoded
	for index := len(path) - 1; index >= 1; index-- {
		key, _ := json.Marshal(path[index])
		subtree = append(append(append([]byte{'{'}, key...), ':'), append(subtree, '}')...)
	}
	key, _ := json.Marshal(path[0])
	property := append(append(key, ':'), subtree...)
	inner := raw[open+1 : closeIndex]
	newline := detectNewline(raw)
	if len(bytes.TrimSpace(inner)) == 0 {
		if bytes.Contains(inner, []byte("\n")) || bytes.Contains(inner, []byte("\r")) {
			closingIndent := indentationBefore(raw, closeIndex)
			property = append([]byte(newline+closingIndent+"  "), property...)
			property = append(property, []byte(newline+closingIndent)...)
		}
		return splice(raw, open+1, closeIndex, property), nil
	}
	insertAt := closeIndex
	for insertAt > open+1 && isJSONSpace(raw[insertAt-1]) {
		insertAt--
	}
	if bytes.Contains(inner, []byte("\n")) || bytes.Contains(inner, []byte("\r")) {
		closingIndent := indentationBefore(raw, closeIndex)
		property = append([]byte(","+newline+closingIndent+"  "), property...)
	} else {
		property = append([]byte(", "), property...)
	}
	return splice(raw, insertAt, insertAt, property), nil
}

// mergeJSONValue replaces the value at raw[start:end] with encoded. If both are
// objects, each encoded member is set individually instead, keeping members of
// the existing object that encoded does not mention.
func mergeJSONValue(raw []byte, start, end int, location []string, encoded []byte) ([]byte, error) {
	existing := skipJSONSpace(raw, start, end)
	desiredOpen := skipJSONSpace(encoded, 0, len(encoded))
	if existing >= end || raw[existing] != '{' || desiredOpen >= len(encoded) || encoded[desiredOpen] != '{' {
		return splice(raw, start, end, encoded), nil
	}
	desired, _, err := parseJSONObject(encoded, desiredOpen, len(encoded))
	if err != nil {
		return nil, err
	}
	result := raw
	for _, member := range desired {
		value := encoded[member.valueStart:member.valueEnd]
		updated, err := upsertJSONObject(result, start, end, location, []string{member.key}, value)
		if err != nil {
			return nil, err
		}
		end += len(updated) - len(result)
		result = updated
	}
	return result, nil
}

func parseJSONObject(raw []byte, open, end int) ([]jsonMember, int, error) {
	index := open + 1
	var members []jsonMember
	for {
		index = skipJSONSpace(raw, index, end)
		if index >= end {
			return nil, 0, fmt.Errorf("unterminated JSON object")
		}
		if raw[index] == '}' {
			return members, index, nil
		}
		keyStart := index
		keyEnd, err := scanJSONString(raw, keyStart, end)
		if err != nil {
			return nil, 0, err
		}
		var key string
		if err := json.Unmarshal(raw[keyStart:keyEnd], &key); err != nil {
			return nil, 0, fmt.Errorf("invalid JSON object key: %w", err)
		}
		index = skipJSONSpace(raw, keyEnd, end)
		if index >= end || raw[index] != ':' {
			return nil, 0, fmt.Errorf("missing colon after JSON key %q", key)
		}
		valueStart := skipJSONSpace(raw, index+1, end)
		valueEnd, err := scanJSONValue(raw, valueStart, end)
		if err != nil {
			return nil, 0, err
		}
		members = append(members, jsonMember{key: key, valueStart: valueStart, valueEnd: valueEnd})
		index = skipJSONSpace(raw, valueEnd, end)
		if index < end && raw[index] == ',' {
			index++
			continue
		}
		if index < end && raw[index] == '}' {
			return members, index, nil
		}
		return nil, 0, fmt.Errorf("invalid JSON object after key %q", key)
	}
}

func scanJSONValue(raw []byte, start, end int) (int, error) {
	if start >= end {
		return 0, fmt.Errorf("missing JSON value")
	}
	if raw[start] == '"' {
		return scanJSONString(raw, start, end)
	}
	if raw[start] == '{' || raw[start] == '[' {
		open, close := raw[start], byte('}')
		if open == '[' {
			close = ']'
		}
		stack := []byte{close}
		for index := start + 1; index < end; index++ {
			switch raw[index] {
			case '"':
				next, err := scanJSONString(raw, index, end)
				if err != nil {
					return 0, err
				}
				index = next - 1
			case '{':
				stack = append(stack, '}')
			case '[':
				stack = append(stack, ']')
			case '}', ']':
				if len(stack) == 0 || raw[index] != stack[len(stack)-1] {
					return 0, fmt.Errorf("mismatched JSON delimiter")
				}
				stack = stack[:len(stack)-1]
				if len(stack) == 0 {
					return index + 1, nil
				}
			}
		}
		return 0, fmt.Errorf("unterminated JSON value")
	}
	index := start
	for index < end && raw[index] != ',' && raw[index] != '}' && raw[index] != ']' && !isJSONSpace(raw[index]) {
		index++
	}
	if index == start {
		return 0, fmt.Errorf("missing JSON value")
	}
	return index, nil
}

func scanJSONString(raw []byte, start, end int) (int, error) {
	if start >= end || raw[start] != '"' {
		return 0, fmt.Errorf("JSON object key must be a string")
	}
	escaped := false
	for index := start + 1; index < end; index++ {
		if escaped {
			escaped = false
			continue
		}
		if raw[index] == '\\' {
			escaped = true
			continue
		}
		if raw[index] == '"' {
			return index + 1, nil
		}
	}
	return 0, fmt.Errorf("unterminated JSON string")
}

func skipJSONSpace(raw []byte, index, end int) int {
	for index < end && isJSONSpace(raw[index]) {
		index++
	}
	return index
}

func isJSONSpace(value byte) bool {
	return value == ' ' || value == '\t' || value == '\r' || value == '\n'
}

func splice(raw []byte, start, end int, replacement []byte) []byte {
	result := make([]byte, 0, len(raw)-(end-start)+len(replacement))
	result = append(result, raw[:start]...)
	result = append(result, replacement...)
	result = append(result, raw[end:]...)
	return result
}

func detectNewline(raw []byte) string {
	if bytes.Contains(raw, []byte("\r\n")) {
		return "\r\n"
	}
	return "\n"
}

func indentationBefore(raw []byte, index int) string {
	lineStart := bytes.LastIndexAny(raw[:index], "\r\n") + 1
	indent := raw[lineStart:index]
	for _, value := range string(indent) {
		if !unicode.IsSpace(value) || value == '\r' || value == '\n' {
			return ""
		}
	}
	return string(indent)
}
