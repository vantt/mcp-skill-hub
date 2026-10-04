package web

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/vantt/mcp-skill-hub/internal/app"
)

type errorEnvelopeSchema struct {
	Properties struct {
		Code struct {
			Enum []string `json:"enum"`
		} `json:"code"`
	} `json:"properties"`
}

func TestErrorStatusCoversSchemaCodes(t *testing.T) {
	data, err := os.ReadFile("../../../schemas/error-envelope.schema.json")
	if err != nil {
		t.Fatalf("failed to read schema: %v", err)
	}

	var schema errorEnvelopeSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("failed to parse schema: %v", err)
	}

	enum := schema.Properties.Code.Enum
	if len(enum) != 30 {
		t.Fatalf("expected 30 error codes in schema, got %d", len(enum))
	}

	for _, code := range enum {
		status, ok := statusByCode[app.ErrorCode(code)]
		if !ok {
			t.Errorf("missing status mapping for schema code: %q", code)
		} else if status < 400 || status > 599 {
			t.Errorf("invalid HTTP status %d for schema code: %q", status, code)
		}
	}
}
