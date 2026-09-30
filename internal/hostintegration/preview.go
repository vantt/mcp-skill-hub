package hostintegration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

const maxChangePreviewBytes = 4096

func changePreview(host Host, kind ChangeKind, raw, desired []byte, exists bool) string {
	var preview string
	switch kind {
	case ChangeMCP:
		preview = mcpChangePreview(host, raw, desired)
	case ChangeNativeSkill:
		preview = fmt.Sprintf("native skill exact file: %s -> %s", contentDescriptor(raw, exists), contentDescriptor(desired, true))
	case ChangeBootstrap:
		preview = bootstrapChangePreview(raw, desired)
	}
	return boundPreview(preview)
}

func mcpChangePreview(host Host, raw, desired []byte) string {
	fields := managedMCPFields(host)
	oldValues := extractManagedMCPValues(host, raw, fields)
	newValues := extractManagedMCPValues(host, desired, fields)
	var builder strings.Builder
	builder.WriteString("managed MCP fields only:")
	for _, field := range fields {
		builder.WriteString("\n")
		builder.WriteString(field)
		builder.WriteString(": ")
		builder.WriteString(managedValueDescriptor(oldValues[field]))
		builder.WriteString(" -> ")
		builder.WriteString(sanitizeManagedValue(newValues[field]))
	}
	return builder.String()
}

func managedMCPFields(host Host) []string {
	switch host {
	case HostClaude:
		return []string{"type", "command", "args"}
	case HostGemini:
		return []string{"command", "args", "cwd", "trust"}
	case HostCodex:
		return []string{"command", "args", "cwd", "enabled"}
	default:
		return nil
	}
}

func extractManagedMCPValues(host Host, raw []byte, fields []string) map[string][]byte {
	values := make(map[string][]byte, len(fields))
	if host == HostCodex {
		return extractCodexManagedValues(raw, fields)
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(raw, &root) != nil {
		return values
	}
	var servers map[string]json.RawMessage
	if json.Unmarshal(root["mcpServers"], &servers) != nil {
		return values
	}
	var registration map[string]json.RawMessage
	if json.Unmarshal(servers["skillhub"], &registration) != nil {
		return values
	}
	for _, field := range fields {
		if value, ok := registration[field]; ok {
			compact := &bytes.Buffer{}
			if json.Compact(compact, value) == nil {
				values[field] = append([]byte(nil), compact.Bytes()...)
			}
		}
	}
	return values
}

func extractCodexManagedValues(raw []byte, fields []string) map[string][]byte {
	values := make(map[string][]byte, len(fields))
	statements, err := scanTOMLStatements(raw)
	if err != nil {
		return values
	}
	inManagedTable := false
	allowed := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		allowed[field] = struct{}{}
	}
	for _, statement := range statements {
		if statement.kind == 'h' {
			inManagedTable = normalizeTOMLTable(statement.name) == "mcp_servers.skillhub"
			continue
		}
		name := normalizeTOMLTable(statement.name)
		if !inManagedTable {
			continue
		}
		if _, ok := allowed[name]; !ok {
			continue
		}
		line := raw[statement.start:statement.end]
		equals := bytes.IndexByte(line, '=')
		if equals < 0 {
			continue
		}
		// The descriptor hashes the complete owned value statement. This avoids
		// exposing credentials that a user may previously have placed there.
		values[name] = bytes.TrimSpace(line[equals+1:])
	}
	return values
}

func managedValueDescriptor(value []byte) string {
	if len(value) == 0 {
		return "<absent>"
	}
	sum := sha256.Sum256(value)
	return fmt.Sprintf("sha256:%s (%d bytes)", hex.EncodeToString(sum[:8]), len(value))
}

func sanitizeManagedValue(value []byte) string {
	if len(value) == 0 {
		return "<absent>"
	}
	const maxValueBytes = 512
	if len(value) > maxValueBytes {
		sum := sha256.Sum256(value)
		return fmt.Sprintf("sha256:%s (%d bytes)", hex.EncodeToString(sum[:8]), len(value))
	}
	// JSON and generated TOML values escape control characters. TOML containers
	// may contain comments, so normalize physical whitespace before rendering.
	return strings.Join(strings.Fields(string(value)), " ")
}

func bootstrapChangePreview(raw, desired []byte) string {
	before := managedBootstrapBlock(raw)
	after := managedBootstrapBlock(desired)
	return "managed bootstrap block only:\n--- before\n" + before + "\n+++ after\n" + after
}

func managedBootstrapBlock(content []byte) string {
	matches := bootstrapMarkerPattern.FindAllIndex(content, -1)
	if len(matches) != 2 {
		return "<absent>"
	}
	return string(content[matches[0][0]:matches[1][1]])
}

func contentDescriptor(content []byte, exists bool) string {
	if !exists {
		return "missing"
	}
	sum := sha256.Sum256(content)
	return fmt.Sprintf("sha256:%s (%d bytes)", hex.EncodeToString(sum[:]), len(content))
}

func boundPreview(value string) string {
	if len(value) <= maxChangePreviewBytes {
		return value
	}
	const suffix = "\n...[preview truncated]"
	limit := maxChangePreviewBytes - len(suffix)
	for limit > 0 && (value[limit]&0xc0) == 0x80 {
		limit--
	}
	return value[:limit] + suffix
}
