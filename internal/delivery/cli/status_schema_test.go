package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStatusJSONValidatesAgainstVersionedSchema(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "workspace")
	var stdout, stderr bytes.Buffer
	if code := Run([]string{"init", root, "--yes"}, &stdout, &stderr); code != 0 {
		t.Fatalf("init exit = %d: %s", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := Run([]string{"status", "--workspace", root, "--json"}, &stdout, &stderr); code != 0 {
		t.Fatalf("status exit = %d: %s", code, stderr.String())
	}
	validateJSONDocument(t, filepath.Join("..", "..", "..", "schemas", "curation-home-v1.schema.json"), stdout.Bytes())
}

func validateJSONDocument(t *testing.T, schemaPath string, document []byte) {
	t.Helper()
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	var schema map[string]any
	if err := json.Unmarshal(schemaBytes, &schema); err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	if id, _ := schema["$id"].(string); !strings.HasSuffix(id, "curation-home-v1.schema.json") {
		t.Fatalf("status schema is not explicitly versioned: %q", id)
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(document))
	if err := decoder.Decode(&value); err != nil {
		t.Fatalf("parse status JSON: %v", err)
	}
	if err := validateSchemaValue(schema, schema, value, "$", true); err != nil {
		t.Fatal(err)
	}
}

func validateSchemaValue(root, schema map[string]any, value any, path string, enforceOneOf bool) error {
	if reference, ok := schema["$ref"].(string); ok {
		const prefix = "#/$defs/"
		if !strings.HasPrefix(reference, prefix) {
			return fmt.Errorf("%s: unsupported schema reference %q", path, reference)
		}
		definitions, ok := root["$defs"].(map[string]any)
		if !ok {
			return fmt.Errorf("%s: schema definitions are missing", path)
		}
		resolved, ok := definitions[strings.TrimPrefix(reference, prefix)].(map[string]any)
		if !ok {
			return fmt.Errorf("%s: unresolved schema reference %q", path, reference)
		}
		return validateSchemaValue(root, resolved, value, path, true)
	}
	if choices, ok := schema["oneOf"].([]any); ok && enforceOneOf {
		matches := 0
		for _, choice := range choices {
			candidate, ok := choice.(map[string]any)
			if ok && validateSchemaValue(root, candidate, value, path, false) == nil {
				matches++
			}
		}
		if matches != 1 {
			return fmt.Errorf("%s: matched %d oneOf branches", path, matches)
		}
		return nil
	}
	if constant, ok := schema["const"]; ok && !reflect.DeepEqual(value, constant) {
		return fmt.Errorf("%s: value %#v does not equal const %#v", path, value, constant)
	}
	if choices, ok := schema["enum"].([]any); ok {
		matched := false
		for _, choice := range choices {
			matched = matched || reflect.DeepEqual(value, choice)
		}
		if !matched {
			return fmt.Errorf("%s: value %#v is outside enum", path, value)
		}
	}
	typeName, _ := schema["type"].(string)
	switch typeName {
	case "null":
		if value != nil {
			return fmt.Errorf("%s: expected null", path)
		}
	case "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s: expected string", path)
		}
		if minimum, ok := schema["minLength"].(float64); ok && len(text) < int(minimum) {
			return fmt.Errorf("%s: string is shorter than minLength", path)
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s: expected boolean", path)
		}
	case "integer":
		number, ok := value.(float64)
		if !ok || math.Trunc(number) != number {
			return fmt.Errorf("%s: expected integer", path)
		}
	case "array":
		values, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s: expected array", path)
		}
		if minimum, ok := schema["minItems"].(float64); ok && len(values) < int(minimum) {
			return fmt.Errorf("%s: array is shorter than minItems", path)
		}
		if maximum, ok := schema["maxItems"].(float64); ok && len(values) > int(maximum) {
			return fmt.Errorf("%s: array is longer than maxItems", path)
		}
		if itemSchema, ok := schema["items"].(map[string]any); ok {
			for index, item := range values {
				if err := validateSchemaValue(root, itemSchema, item, fmt.Sprintf("%s[%d]", path, index), true); err != nil {
					return err
				}
			}
		}
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s: expected object", path)
		}
		properties, _ := schema["properties"].(map[string]any)
		if required, ok := schema["required"].([]any); ok {
			for _, field := range required {
				name, _ := field.(string)
				if _, exists := object[name]; !exists {
					return fmt.Errorf("%s: required property %q is missing", path, name)
				}
			}
		}
		for name, child := range object {
			childSchema, exists := properties[name].(map[string]any)
			if !exists {
				if additional, ok := schema["additionalProperties"].(bool); ok && !additional {
					return fmt.Errorf("%s: additional property %q is not allowed", path, name)
				}
				continue
			}
			if err := validateSchemaValue(root, childSchema, child, path+"."+name, true); err != nil {
				return err
			}
		}
	}
	return nil
}
