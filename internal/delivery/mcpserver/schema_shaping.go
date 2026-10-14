package mcpserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/vantt/skillhub-mcp/internal/delivery/paging"
	"github.com/vantt/skillhub-mcp/internal/telemetry"
	contractschemas "github.com/vantt/skillhub-mcp/schemas"
)

// walkSchema calls visit on schema and then on every subschema reachable
// through properties, definitions, anyOf and items. A nil schema is skipped.
// visit returns false to leave the children of that schema unvisited; children
// are read after visit returns, so a visit may replace or add them.
func walkSchema(schema *jsonschema.Schema, visit func(*jsonschema.Schema) bool) {
	if schema == nil || !visit(schema) {
		return
	}
	for _, child := range schema.Properties {
		walkSchema(child, visit)
	}
	for _, child := range schema.Defs {
		walkSchema(child, visit)
	}
	for _, child := range schema.AnyOf {
		walkSchema(child, visit)
	}
	walkSchema(schema.Items, visit)
}

// replacePropertySchema swaps every property called name, at any depth, for a
// copy of replacement. It reports whether it replaced any.
func replacePropertySchema(schema *jsonschema.Schema, name string, replacement *jsonschema.Schema) bool {
	replaced := false
	walkSchema(schema, func(node *jsonschema.Schema) bool {
		if _, exists := node.Properties[name]; exists {
			node.Properties[name] = replacement.CloneSchemas()
			replaced = true
		}
		return true
	})
	return replaced
}

func closeObjectSchemas(schema *jsonschema.Schema) {
	walkSchema(schema, func(node *jsonschema.Schema) bool {
		if node.Type == "object" && node.AdditionalProperties == nil {
			node.AdditionalProperties = &jsonschema.Schema{Not: &jsonschema.Schema{}}
		}
		return true
	})
}

// normalizeMultiTypeSchemas preserves nullable semantics while avoiding the
// multi-value `type` form rejected by several function-calling clients.
func normalizeMultiTypeSchemas(schema *jsonschema.Schema) {
	walkSchema(schema, func(node *jsonschema.Schema) bool {
		if len(node.Types) > 0 {
			for _, schemaType := range node.Types {
				node.AnyOf = append(node.AnyOf, &jsonschema.Schema{Type: schemaType})
			}
			node.Types = nil
		}
		return true
	})
}

func shrinkOutputSchema(schema *jsonschema.Schema) {
	walkSchema(schema, func(node *jsonschema.Schema) bool {
		// An embedded $id starts a separate schema resource. Its local references
		// depend on that boundary and its definitions, so keep the resource intact.
		if node.ID != "" {
			return false
		}
		node.Description = ""
		node.Title = ""
		node.Schema = ""
		node.AdditionalProperties = nil
		return true
	})
}

func applyCommonConstraints(schema *jsonschema.Schema) {
	walkSchema(schema, func(node *jsonschema.Schema) bool {
		if schemaHasType(node, "array") && node.MaxItems == nil {
			node.MaxItems = intPointer(paging.MaximumLimit)
		}
		for name, property := range node.Properties {
			switch name {
			case "limit":
				minimum, maximum := float64(1), float64(paging.MaximumLimit)
				property.Minimum, property.Maximum = &minimum, &maximum
			case "schema_version":
				property.Enum = []any{SchemaVersion}
			case "decision":
				property.MaxLength = intPointer(64)
			case "content":
				property.MaxLength = intPointer(1 << 20)
			case "cursor", "uri", "locator", "reason", "rationale", "description", "note":
				property.MaxLength = intPointer(4096)
			default:
				if schemaHasType(property, "string") {
					property.MaxLength = intPointer(2048)
				}
			}
		}
		return true
	})
}

// dropEnumLengthBounds clears maxLength on enum properties: an enum already
// bounds the length.
func dropEnumLengthBounds(schema *jsonschema.Schema) {
	walkSchema(schema, func(node *jsonschema.Schema) bool {
		if len(node.Enum) > 0 {
			node.MaxLength = nil
		}
		return true
	})
}

func schemaHasType(schema *jsonschema.Schema, target string) bool {
	if schema.Type == target {
		return true
	}
	for _, schemaType := range schema.Types {
		if schemaType == target {
			return true
		}
	}
	return false
}

func intPointer(value int) *int { return &value }

// toolSchemaOptions carries what one tool needs beyond the schema derived from
// its Go types.
type toolSchemaOptions struct {
	// shapeInput refines the derived input schema after the common constraints.
	shapeInput func(*jsonschema.Schema)
	// committedResolver replaces the input schema with the committed resolver
	// request schema and embeds the committed response schema in the output.
	committedResolver bool
}

// toolOption adjusts how addTool shapes the schemas of one tool.
type toolOption func(*toolSchemaOptions)

// withInputSchema refines the derived input schema of a tool.
func withInputSchema(shape func(*jsonschema.Schema)) toolOption {
	return func(options *toolSchemaOptions) { options.shapeInput = shape }
}

// withCommittedResolverSchemas makes a tool advertise the committed resolver
// request and response schemas.
func withCommittedResolverSchemas() toolOption {
	return func(options *toolSchemaOptions) { options.committedResolver = true }
}

// withPropertyEnums limits string properties of the input schema to the listed
// values. Properties the schema does not have are ignored.
func withPropertyEnums(enums map[string][]any) toolOption {
	return withInputSchema(func(schema *jsonschema.Schema) {
		for name, values := range enums {
			if property := schema.Properties[name]; property != nil {
				property.Enum = values
			}
		}
	})
}

// deriveToolSchemas derives the input and output schema of a tool from its Go
// types and shapes them for clients: closed objects, bounded strings and
// arrays, no multi-type `type` lists, and a compact output schema.
func deriveToolSchemas[In, Out any](name string, options toolSchemaOptions) (input, output *jsonschema.Schema) {
	input, err := jsonschema.For[In](&jsonschema.ForOptions{})
	if err != nil {
		panic(fmt.Sprintf("derive schema for %s: %v", name, err))
	}
	closeObjectSchemas(input)
	applyCommonConstraints(input)
	if options.shapeInput != nil {
		options.shapeInput(input)
	}
	dropEnumLengthBounds(input)
	normalizeMultiTypeSchemas(input)
	output, err = jsonschema.For[toolOutcome[Out]](&jsonschema.ForOptions{})
	if err != nil {
		panic(fmt.Sprintf("derive output schema for %s: %v", name, err))
	}
	normalizeMultiTypeSchemas(output)
	shrinkOutputSchema(output)
	if options.committedResolver {
		committed, contractErr := committedResolverSchemas()
		if contractErr != nil {
			panic(fmt.Sprintf("load committed resolver schemas: %v", contractErr))
		}
		committedRequest, committedResponse := committed[0].CloneSchemas(), committed[1].CloneSchemas()
		if embedErr := embedResolverResponse(output, committedResponse); embedErr != nil {
			panic(fmt.Sprintf("resolver output schema for %s: %v", name, embedErr))
		}
		input = advertisedRequestSchema(committedRequest)
	}
	return input, output
}

// committedResolverSchemas decodes the committed resolver request and response
// schemas once per process. Callers clone what they keep, because tools shape
// and resolve their copies.
var committedResolverSchemas = sync.OnceValues(func() ([2]*jsonschema.Schema, error) {
	request, err := contractschemas.ResolverRequest()
	if err != nil {
		return [2]*jsonschema.Schema{}, err
	}
	response, err := contractschemas.ResolverResponse()
	return [2]*jsonschema.Schema{request, response}, err
})

// embedResolverResponse swaps the derived resolution property for the committed
// response schema. A committed schema without its own $id shares the output
// schema's root resource, so its $defs move to the root where its local
// references resolve; the result is resolved once so a dangling reference is
// reported here as an error instead of surfacing later from tool registration.
func embedResolverResponse(outputSchema, response *jsonschema.Schema) error {
	embedded := response.CloneSchemas()
	if embedded.ID == "" && len(embedded.Defs) > 0 {
		if outputSchema.Defs == nil {
			outputSchema.Defs = map[string]*jsonschema.Schema{}
		}
		for name, def := range embedded.Defs {
			if _, taken := outputSchema.Defs[name]; taken {
				return fmt.Errorf("committed response definition %q collides with a derived definition", name)
			}
			outputSchema.Defs[name] = def
		}
		embedded.Defs = nil
	}
	if !replacePropertySchema(outputSchema, "resolution", embedded) {
		return errors.New("no resolution property")
	}
	if _, err := outputSchema.Resolve(&jsonschema.ResolveOptions{}); err != nil {
		return fmt.Errorf("resolve embedded response schema: %w", err)
	}
	return nil
}

// curationSessionInputSchema bounds the enumerated and numeric fields of the
// curation session record.
func curationSessionInputSchema(schema *jsonschema.Schema) {
	for name, property := range schema.Properties {
		switch name {
		case "status":
			property.Enum = []any{"completed", "partial", "failed", "abandoned"}
		case "basis":
			property.Enum = []any{telemetry.CurationBasisHostReported, telemetry.CurationBasisControlledBenchmark}
		case "error_code":
			property.Enum = []any{"capability_unavailable", "internal_error", "operation_cancelled", "partial_failure", "recovery_required", "source_unavailable", "validation_failed"}
		case "turns_to_next_action", "unnecessary_confirmations", "prompts_per_batch", "batch_size", "routine_git_noise":
			minimum, maximum := float64(0), float64(10_000)
			property.Minimum, property.Maximum = &minimum, &maximum
		case "duration_ms":
			minimum, maximum := float64(0), float64(604_800_000)
			property.Minimum, property.Maximum = &minimum, &maximum
		}
	}
}

// advertisedRequestSchema projects the committed resolver request schema onto
// what a client needs to build a call: types, properties, required lists, enums,
// closed objects, references and descriptions. It drops the keywords only a
// validator reads (identifiers, length and count bounds, uniqueness, formats),
// because every call is validated against the full committed schema by
// validateResolverArguments. It edits and returns the schema it is given.
func advertisedRequestSchema(committed *jsonschema.Schema) *jsonschema.Schema {
	walkSchema(committed, func(node *jsonschema.Schema) bool {
		node.ID, node.Schema, node.Format = "", "", ""
		node.MaxLength, node.MinLength, node.MaxItems = nil, nil, nil
		node.UniqueItems = false
		return true
	})
	return committed
}

// committedRequestValidator resolves the full committed resolver request schema
// once per process.
var committedRequestValidator = sync.OnceValues(func() (*jsonschema.Resolved, error) {
	committed, err := committedResolverSchemas()
	if err != nil {
		return nil, err
	}
	return committed[0].CloneSchemas().Resolve(&jsonschema.ResolveOptions{})
})

// validateResolverArguments checks the raw arguments of a resolver tool call
// against the full committed request schema, including the bounds that
// discovery no longer advertises. Its error carries the wording the SDK uses for
// arguments that break the advertised schema.
func validateResolverArguments(raw json.RawMessage) error {
	validator, err := committedRequestValidator()
	if err != nil {
		return fmt.Errorf("load committed resolver request schema: %w", err)
	}
	arguments := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &arguments); err != nil {
			return fmt.Errorf("validating \"arguments\": unmarshaling arguments: %w", err)
		}
	}
	var instance any = arguments
	if err := validator.Validate(&instance); err != nil {
		return fmt.Errorf("validating \"arguments\": %v", err)
	}
	return nil
}
