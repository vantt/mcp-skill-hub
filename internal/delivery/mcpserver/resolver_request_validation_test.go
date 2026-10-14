package mcpserver

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	contractschemas "github.com/vantt/skillhub-mcp/schemas"
)

// committedRequestDocument returns the committed resolver request schema as a
// generic JSON document, so the tests below walk every keyword it carries
// instead of the subset the Schema struct models.
func committedRequestDocument(t *testing.T) map[string]any {
	t.Helper()
	committed, err := contractschemas.ResolverRequest()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(committed)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatal(err)
	}
	return document
}

// requestStep is one move from a request value to a nested value: an object
// property, or the first element of an array.
type requestStep struct {
	property string
	element  bool
}

func (step requestStep) String() string {
	if step.element {
		return "[]"
	}
	return step.property
}

// requestBound is one validator-only constraint of the committed schema, with
// the route from the request root to the value it constrains.
type requestBound struct {
	keyword string
	route   []requestStep
	schema  map[string]any
}

func (bound requestBound) name() string {
	parts := make([]string, len(bound.route))
	for index, step := range bound.route {
		parts[index] = step.String()
	}
	return bound.keyword + " at " + strings.Join(parts, ".")
}

// requestSchemaWalker resolves the local references of the committed schema.
type requestSchemaWalker struct {
	definitions map[string]any
}

func (walker requestSchemaWalker) resolve(node map[string]any) map[string]any {
	reference, _ := node["$ref"].(string)
	if reference == "" {
		return node
	}
	name, found := strings.CutPrefix(reference, "#/$defs/")
	target, _ := walker.definitions[name].(map[string]any)
	if !found || target == nil {
		panic(fmt.Sprintf("unsupported reference %q in the committed request schema", reference))
	}
	return target
}

// bounds lists every maxLength, minLength, maxItems, uniqueItems and format of
// the schema reachable from node, one entry per route.
func (walker requestSchemaWalker) bounds(node map[string]any, route []requestStep) []requestBound {
	node = walker.resolve(node)
	var found []requestBound
	for _, keyword := range []string{"maxLength", "minLength", "maxItems", "uniqueItems", "format"} {
		if _, present := node[keyword]; present {
			found = append(found, requestBound{keyword: keyword, route: route, schema: node})
		}
	}
	if properties, _ := node["properties"].(map[string]any); properties != nil {
		for name, child := range properties {
			childRoute := append(append([]requestStep{}, route...), requestStep{property: name})
			found = append(found, walker.bounds(child.(map[string]any), childRoute)...)
		}
	}
	if items, _ := node["items"].(map[string]any); items != nil {
		childRoute := append(append([]requestStep{}, route...), requestStep{element: true})
		found = append(found, walker.bounds(items, childRoute)...)
	}
	for _, unsupported := range []string{"oneOf", "allOf", "anyOf", "prefixItems", "patternProperties", "if", "not"} {
		if _, present := node[unsupported]; present {
			panic(fmt.Sprintf("the committed request schema uses %q, which the bound walker does not follow", unsupported))
		}
	}
	return found
}

// minimalValue builds a smallest value that satisfies node. seed makes string
// elements distinct so a uniqueItems array stays valid.
func (walker requestSchemaWalker) minimalValue(node map[string]any, seed int) any {
	node = walker.resolve(node)
	if constant, present := node["const"]; present {
		return constant
	}
	if choices, _ := node["enum"].([]any); len(choices) > 0 {
		return choices[0]
	}
	switch node["type"] {
	case "object":
		object := map[string]any{}
		required, _ := node["required"].([]any)
		properties, _ := node["properties"].(map[string]any)
		for _, name := range required {
			object[name.(string)] = walker.minimalValue(properties[name.(string)].(map[string]any), seed)
		}
		return object
	case "array":
		return []any{}
	case "string":
		if node["format"] == "date-time" {
			return "2026-01-02T03:04:05Z"
		}
		return fmt.Sprintf("v%d", seed)
	case "integer", "number":
		if minimum, ok := node["minimum"].(float64); ok {
			return minimum
		}
		return float64(1)
	case "boolean":
		return false
	}
	panic(fmt.Sprintf("no minimal value for schema %v", node))
}

// requestWith builds a request that satisfies every constraint except that the
// value at route is replaced by value.
func (walker requestSchemaWalker) requestWith(root map[string]any, route []requestStep, value any) map[string]any {
	request := walker.minimalValue(root, 0).(map[string]any)
	if len(route) == 0 {
		panic("a bound at the request root has no property to replace")
	}
	walker.place(walker.resolve(root), request, route, value)
	return request
}

func (walker requestSchemaWalker) place(node map[string]any, holder any, route []requestStep, value any) any {
	node = walker.resolve(node)
	if len(route) == 0 {
		return value
	}
	step := route[0]
	if step.element {
		items := node["items"].(map[string]any)
		return []any{walker.place(items, walker.minimalValue(items, 0), route[1:], value)}
	}
	object := holder.(map[string]any)
	child := node["properties"].(map[string]any)[step.property].(map[string]any)
	current, present := object[step.property]
	if !present {
		current = walker.minimalValue(child, 0)
	}
	object[step.property] = walker.place(child, current, route[1:], value)
	return object
}

// violation returns a value for the schema that breaks exactly the given bound.
func (walker requestSchemaWalker) violation(bound requestBound) (any, bool) {
	limit, _ := bound.schema[bound.keyword].(float64)
	switch bound.keyword {
	case "maxLength":
		return strings.Repeat("x", int(limit)+1), true
	case "minLength":
		return strings.Repeat("x", int(limit)-1), true
	case "maxItems":
		items := make([]any, int(limit)+1)
		for index := range items {
			items[index] = walker.minimalValue(bound.schema["items"].(map[string]any), index)
		}
		return items, true
	case "uniqueItems":
		element := walker.minimalValue(bound.schema["items"].(map[string]any), 0)
		return []any{element, element}, true
	}
	return nil, false
}

func resolveSession(t *testing.T) *mcp.ClientSession {
	t.Helper()
	_, server, err := New(newMCPWorkspace(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	return newClientSession(t, server)
}

// TestResolveRequestBoundsEnforced proves that every length, count and
// uniqueness bound of the committed request schema still rejects a call, even
// though discovery advertises a request schema without those keywords.
func TestResolveRequestBoundsEnforced(t *testing.T) {
	t.Parallel()
	document := committedRequestDocument(t)
	definitions, _ := document["$defs"].(map[string]any)
	walker := requestSchemaWalker{definitions: definitions}
	bounds := walker.bounds(document, nil)
	if len(bounds) < 10 {
		t.Fatalf("walked only %d bounds from the committed request schema", len(bounds))
	}
	session := resolveSession(t)

	valid := walker.minimalValue(document, 0).(map[string]any)
	valid["task"] = map[string]any{"description": "review the change"}
	if response, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "skill_resolve", Arguments: valid}); err != nil || response.IsError {
		t.Fatalf("minimal valid request rejected: %#v, %v", response, err)
	}

	for _, bound := range bounds {
		t.Run(bound.name(), func(t *testing.T) {
			broken, enforceable := walker.violation(bound)
			if !enforceable {
				t.Skipf("%s is an annotation in the validator, so no call can break it", bound.keyword)
			}
			request := walker.requestWith(document, bound.route, broken)
			response, err := session.CallTool(t.Context(), &mcp.CallToolParams{Name: "skill_resolve", Arguments: request})
			if err != nil {
				t.Fatalf("protocol error instead of a tool error: %v", err)
			}
			if !response.IsError {
				t.Fatalf("accepted a request that breaks %s: %v", bound.name(), request)
			}
			text := ""
			if len(response.Content) > 0 {
				if content, ok := response.Content[0].(*mcp.TextContent); ok {
					text = content.Text
				}
			}
			if !strings.Contains(text, `validating "arguments"`) {
				t.Fatalf("error does not use the argument-validation wording: %q", text)
			}
		})
	}
}
