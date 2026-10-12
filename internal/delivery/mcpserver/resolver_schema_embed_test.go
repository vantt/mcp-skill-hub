package mcpserver

import (
	"testing"

	"github.com/google/jsonschema-go/jsonschema"
	contractschemas "github.com/vantt/mcp-skill-hub/schemas"
)

type embedProbeOutput struct {
	Resolution map[string]any `json:"resolution"`
}

func probeOutputSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	schema, err := jsonschema.For[embedProbeOutput](&jsonschema.ForOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func TestEmbedResolverResponseKeepsCommittedSchemaWithOwnID(t *testing.T) {
	response, err := contractschemas.ResolverResponse()
	if err != nil {
		t.Fatal(err)
	}
	output := probeOutputSchema(t)
	if err := embedResolverResponse(output, response); err != nil {
		t.Fatalf("embed committed response: %v", err)
	}
	if output.Properties["resolution"].ID == "" {
		t.Fatal("committed response lost its own resource id")
	}
}

func TestEmbedResolverResponseHoistsDefsWhenSchemaHasNoID(t *testing.T) {
	response, err := contractschemas.ResolverResponse()
	if err != nil {
		t.Fatal(err)
	}
	response.ID = ""
	output := probeOutputSchema(t)
	if err := embedResolverResponse(output, response); err != nil {
		t.Fatalf("embed response without id: %v", err)
	}
	if output.Defs["setup"] == nil {
		t.Fatal("definitions were not moved to the output schema root")
	}
}

func TestEmbedResolverResponseReportsDanglingReference(t *testing.T) {
	response, err := contractschemas.ResolverResponse()
	if err != nil {
		t.Fatal(err)
	}
	response.Defs = nil
	if err := embedResolverResponse(probeOutputSchema(t), response); err == nil {
		t.Fatal("expected an error for a response schema whose definitions are missing")
	}
}
