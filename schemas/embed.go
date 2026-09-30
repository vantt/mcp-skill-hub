// Package schemas exposes committed public JSON Schemas to runtime discovery.
package schemas

import (
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
)

//go:embed skill-resolve-request-v1.schema.json
var resolverRequestJSON []byte

//go:embed skill-resolve-response-v1.schema.json
var resolverResponseJSON []byte

//go:embed telemetry-event-v1.schema.json
var telemetryEventJSON []byte

//go:embed evaluation-case-v1.schema.json
var evaluationCaseJSON []byte

//go:embed evaluation-suite-v1.schema.json
var evaluationSuiteJSON []byte

//go:embed experiment-manifest-v1.schema.json
var experimentManifestJSON []byte

func ResolverRequest() (*jsonschema.Schema, error) {
	return decode("resolver request", resolverRequestJSON)
}

func ResolverResponse() (*jsonschema.Schema, error) {
	return decode("resolver response", resolverResponseJSON)
}

func TelemetryEvent() (*jsonschema.Schema, error) {
	return decode("telemetry event", telemetryEventJSON)
}

func EvaluationCase() (*jsonschema.Schema, error) {
	return decode("evaluation case", evaluationCaseJSON)
}

func EvaluationSuite() (*jsonschema.Schema, error) {
	return decode("evaluation suite", evaluationSuiteJSON)
}

func ExperimentManifest() (*jsonschema.Schema, error) {
	return decode("experiment manifest", experimentManifestJSON)
}

func decode(name string, data []byte) (*jsonschema.Schema, error) {
	var schema jsonschema.Schema
	if err := json.Unmarshal(data, &schema); err != nil {
		return nil, fmt.Errorf("decode committed %s schema: %w", name, err)
	}
	return &schema, nil
}
