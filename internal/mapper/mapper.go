package mapper

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/ogen-go/ogen"
	"github.com/ogen-go/ogen/gen/ir"
	"github.com/ogen-go/ogen/jsonschema"
	"github.com/ogen-go/ogen/openapi"

	"github.com/savid/ogen-mcp/internal/naming"
	"github.com/savid/ogen-mcp/internal/parser"
)

const (
	defaultExampleText = "example"
	exampleValueKey    = "value"
)

// Map converts parsed OpenAPI operations into a Code Mode mapping payload.
func Map(result *parser.Result, opts MapOptions) (*MappedAPI, error) {
	opts.setDefaults()

	if err := validateOptions(opts); err != nil {
		return nil, err
	}

	operations := make([]MappedOperation, 0, len(result.Operations))

	for _, op := range result.Operations {
		spec := op.Spec
		if spec == nil {
			continue
		}

		key := extensionKey(spec)
		if ext, ok := result.Extensions[key]; ok && ext.Exclude {
			continue
		}

		operations = append(operations, mapOperation(op))
	}

	sort.Slice(operations, func(i, j int) bool {
		if operations[i].ID == operations[j].ID {
			if operations[i].Method == operations[j].Method {
				return operations[i].Path < operations[j].Path
			}

			return operations[i].Method < operations[j].Method
		}

		return operations[i].ID < operations[j].ID
	})

	schemas, err := mapSchemas(result.RawSpec)
	if err != nil {
		return nil, fmt.Errorf("mapping schemas: %w", err)
	}

	examples := mapExamples(operations)

	specJSON, err := buildSearchSpecJSON(result.RawSpec, operations, schemas, opts.ResourceURIPrefix)
	if err != nil {
		return nil, fmt.Errorf("building search spec projection: %w", err)
	}

	operationsJSON, err := buildOperationsJSON(operations)
	if err != nil {
		return nil, fmt.Errorf("building operations JSON: %w", err)
	}

	searchDescription, executeDescription := buildToolDescriptions(opts, operations)

	if opts.NoResources {
		schemas = nil
		examples = nil
	}

	return &MappedAPI{
		PackageName:             opts.PackageName,
		EngineInterface:         opts.EngineInterface,
		SearchToolName:          opts.SearchToolName,
		ExecuteToolName:         opts.ExecuteToolName,
		ResourceURIPrefix:       opts.ResourceURIPrefix,
		Runtime:                 opts.Runtime,
		ExamplesMode:            opts.ExamplesMode,
		ExecuteMaxRequests:      opts.ExecuteMaxRequests,
		ExecuteTimeout:          opts.ExecuteTimeout,
		ExecuteMaxResponseBytes: opts.ExecuteMaxResponseBytes,
		ExecuteMaxOutputBytes:   opts.ExecuteMaxOutputBytes,
		ExecuteMaxRedirects:     opts.ExecuteMaxRedirects,
		SearchDescription:       searchDescription,
		ExecuteDescription:      executeDescription,
		SpecJSON:                specJSON,
		OperationsJSON:          operationsJSON,
		Operations:              operations,
		Schemas:                 schemas,
		Examples:                examples,
	}, nil
}

func validateOptions(opts MapOptions) error {
	if opts.Runtime != "js" {
		return fmt.Errorf("unsupported runtime %q: only \"js\" is supported", opts.Runtime)
	}

	switch opts.ExamplesMode {
	case examplesModeResources, examplesModeInline, examplesModeNone:
		return nil
	default:
		return fmt.Errorf("unsupported examples mode %q: use resources, inline, or none", opts.ExamplesMode)
	}
}

func mapOperation(op *ir.Operation) MappedOperation {
	spec := op.Spec
	method := strings.ToUpper(spec.HTTPMethod)
	id := spec.OperationID
	if id == "" {
		id = fallbackOperationID(method, pathString(spec.Path))
	}

	params := mapParams(op)

	mapped := MappedOperation{
		ID:           id,
		Method:       method,
		Path:         pathString(spec.Path),
		Summary:      spec.Summary,
		Description:  spec.Description,
		Tags:         append([]string(nil), spec.Tags...),
		HasBody:      op.Request != nil,
		BodyRequired: op.Request != nil && op.Request.Spec != nil && op.Request.Spec.Required,
		Params:       params,
	}

	if op.Request != nil {
		mapped.BodyExample = sampleFromType(op.Request.Type, 0)
	}

	return mapped
}

func mapParams(op *ir.Operation) []MappedParam {
	if len(op.Params) == 0 {
		return nil
	}

	params := make([]MappedParam, 0, len(op.Params))

	for _, p := range op.Params {
		params = append(params, MappedParam{
			Name:     p.Spec.Name,
			In:       string(p.Spec.In),
			Required: p.Spec.Required,
			Type:     schemaType(p.Spec.Schema),
			Example:  sampleFromType(p.Type, 0),
		})
	}

	sort.Slice(params, func(i, j int) bool {
		if params[i].In == params[j].In {
			return params[i].Name < params[j].Name
		}

		return params[i].In < params[j].In
	})

	return params
}

func schemaType(schema *jsonschema.Schema) string {
	if schema == nil {
		return "unknown"
	}

	if schema.Type == jsonschema.Empty {
		if len(schema.Enum) > 0 {
			return "enum"
		}

		return "unknown"
	}

	return schema.Type.String()
}

func sampleFromType(t *ir.Type, depth int) any {
	if t == nil || depth > 5 {
		return map[string]any{defaultExampleText: defaultExampleText}
	}

	if t.Kind == ir.KindPointer {
		return sampleFromType(t.PointerTo, depth+1)
	}

	if t.Kind == ir.KindAlias {
		return sampleFromType(t.AliasTo, depth+1)
	}

	if t.Kind == ir.KindStruct {
		return sampleFromStruct(t, depth+1)
	}

	if t.Kind == ir.KindPrimitive || t.Kind == ir.KindEnum || t.Kind == ir.KindConst {
		return sampleFromSchema(t.Schema, depth+1)
	}

	if t.Kind == ir.KindArray {
		return []any{sampleFromType(t.Item, depth+1)}
	}

	if t.Kind == ir.KindMap {
		return map[string]any{"key": sampleFromType(t.Item, depth+1)}
	}

	return map[string]any{exampleValueKey: defaultExampleText}
}

func sampleFromStruct(t *ir.Type, depth int) any {
	obj := make(map[string]any)

	for _, field := range t.Fields {
		if field == nil || field.Type == nil || field.Type.Kind == ir.KindPointer {
			continue
		}

		name := jsonFieldName(*field)
		if name == "" {
			continue
		}

		obj[name] = sampleFromType(field.Type, depth+1)
	}

	if len(obj) == 0 {
		return map[string]any{exampleValueKey: defaultExampleText}
	}

	return obj
}

func jsonFieldName(field ir.Field) string {
	tag := field.Tag.JSON
	if tag != "" {
		name := strings.Split(tag, ",")[0]
		if name != "" && name != "-" {
			return name
		}
	}

	if field.Spec != nil && field.Spec.Name != "" {
		return field.Spec.Name
	}

	return naming.ToSnakeCase(field.Name)
}

func sampleFromSchema(schema *jsonschema.Schema, depth int) any {
	if schema == nil || depth > 5 {
		return "example"
	}

	if len(schema.Examples) > 0 {
		var out any
		if err := json.Unmarshal(schema.Examples[0], &out); err == nil {
			return out
		}
	}

	if len(schema.Enum) > 0 {
		return schema.Enum[0]
	}

	switch schema.Type {
	case jsonschema.String:
		switch schema.Format {
		case "date-time":
			return "2026-01-01T00:00:00Z"
		case "date":
			return "2026-01-01"
		default:
			return "example"
		}
	case jsonschema.Integer:
		return 1
	case jsonschema.Number:
		return 1.0
	case jsonschema.Boolean:
		return true
	case jsonschema.Array:
		return []any{sampleFromSchema(schema.Item, depth+1)}
	case jsonschema.Object:
		obj := make(map[string]any)
		for _, prop := range schema.Properties {
			if !prop.Required {
				continue
			}

			obj[prop.Name] = sampleFromSchema(prop.Schema, depth+1)
		}

		if len(obj) == 0 {
			return map[string]any{exampleValueKey: defaultExampleText}
		}

		return obj
	case jsonschema.Empty, jsonschema.Null:
		return defaultExampleText
	default:
		return defaultExampleText
	}
}

func mapSchemas(spec *ogen.Spec) ([]MappedSchema, error) {
	if spec == nil || spec.Components == nil || len(spec.Components.Schemas) == 0 {
		return nil, nil
	}

	schemas := make([]MappedSchema, 0, len(spec.Components.Schemas))

	for name, schema := range spec.Components.Schemas {
		if schema == nil {
			continue
		}

		if node, ok := schema.Common.Extensions["x-mcp-exclude"]; ok {
			var exclude bool
			if err := node.Decode(&exclude); err != nil {
				return nil, fmt.Errorf("decoding x-mcp-exclude on schema %q: %w", name, err)
			}
			if exclude {
				continue
			}
		}

		data, err := json.Marshal(schema)
		if err != nil {
			return nil, fmt.Errorf("marshaling schema %q: %w", name, err)
		}

		schemas = append(schemas, MappedSchema{
			Name:        name,
			Description: schema.Description,
			JSONContent: string(data),
		})
	}

	sort.Slice(schemas, func(i, j int) bool {
		return schemas[i].Name < schemas[j].Name
	})

	return schemas, nil
}

func mapExamples(operations []MappedOperation) []MappedExample {
	if len(operations) == 0 {
		return nil
	}

	examples := make([]MappedExample, 0, len(operations))

	for _, op := range operations {
		examples = append(examples, MappedExample{
			OperationID: op.ID,
			Method:      op.Method,
			Path:        op.Path,
			Summary:     op.Summary,
			Code:        buildExecuteExample(op),
		})
	}

	return examples
}

func buildExecuteExample(op MappedOperation) string {
	request := map[string]any{
		"method": op.Method,
		"path":   op.Path,
	}

	pathParams := make(map[string]any)
	query := make(map[string]any)
	headers := make(map[string]any)

	for _, p := range op.Params {
		if !p.Required {
			continue
		}

		switch p.In {
		case "path":
			pathParams[p.Name] = p.Example
		case "query":
			query[p.Name] = p.Example
		case "header":
			headers[p.Name] = p.Example
		}
	}

	if len(pathParams) > 0 {
		request["pathParams"] = pathParams
	}

	if len(query) > 0 {
		request["query"] = query
	}

	if len(headers) > 0 {
		request["headers"] = headers
	}

	if op.HasBody {
		request["body"] = op.BodyExample
	}

	payload, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return "const response = api.request({ method: \"GET\", path: \"/\" });\nreturn response;"
	}

	return "const response = api.request(" + string(payload) + ");\nreturn response;"
}

func buildToolDescriptions(opts MapOptions, operations []MappedOperation) (string, string) {
	searchDescription := strings.TrimSpace(`Discover API capabilities by running JavaScript against a read-only 'spec' object.

Use this for endpoint discovery before execution.

Example:
const matches = spec.operations.filter(op => op.tags.includes("pets"));
return matches.map(op => ({id: op.operationId, method: op.method, path: op.path}));`)

	executeDescription := fmt.Sprintf(strings.TrimSpace(`Execute API requests by running JavaScript against the 'api' helper.

Use api.request({ method, path, pathParams?, query?, headers?, body? }) to perform one or more requests.

Limits: max_requests=%d, timeout=%s, max_response_bytes=%d, max_output_bytes=%d.

Example (multi-request):
const a = api.request({ method: "GET", path: "/orders", query: { limit: 3 } });
const b = api.request({ method: "GET", path: "/pets", query: { limit: 3 } });
return { orders: a, pets: b };`),
		opts.ExecuteMaxRequests,
		opts.ExecuteTimeout,
		opts.ExecuteMaxResponseBytes,
		opts.ExecuteMaxOutputBytes,
	)

	if opts.ExamplesMode == examplesModeInline && len(operations) > 0 {
		limit := min(3, len(operations))

		lines := make([]string, 0, limit)
		for i := range limit {
			op := operations[i]
			lines = append(lines, fmt.Sprintf("- %s %s (%s)", op.Method, op.Path, op.ID))
		}

		executeDescription += "\n\nOperation starters:\n" + strings.Join(lines, "\n")
	}

	return searchDescription, executeDescription
}

func buildSearchSpecJSON(
	raw *ogen.Spec,
	operations []MappedOperation,
	schemas []MappedSchema,
	prefix string,
) (string, error) {
	type paramProjection = MappedParam

	type opProjection struct {
		OperationID  string            `json:"operationId"`
		Method       string            `json:"method"`
		Path         string            `json:"path"`
		Summary      string            `json:"summary"`
		Description  string            `json:"description"`
		Tags         []string          `json:"tags,omitempty"`
		HasBody      bool              `json:"hasBody"`
		BodyRequired bool              `json:"bodyRequired"`
		Parameters   []paramProjection `json:"parameters"`
		BodyExample  any               `json:"bodyExample,omitempty"`
		ExampleURI   string            `json:"exampleUri"`
	}

	type schemaProjection struct {
		Name        string `json:"name"`
		Description string `json:"description,omitempty"`
		URI         string `json:"uri"`
	}

	projection := struct {
		OpenAPI    string             `json:"openapi"`
		Info       map[string]string  `json:"info"`
		Operations []opProjection     `json:"operations"`
		Schemas    []schemaProjection `json:"schemas"`
	}{
		Info: map[string]string{},
	}

	if raw != nil {
		projection.OpenAPI = raw.OpenAPI
		projection.Info["title"] = raw.Info.Title
		projection.Info["version"] = raw.Info.Version
	}

	projection.Operations = make([]opProjection, 0, len(operations))
	for _, op := range operations {
		// Always emit a non-nil slice so the field renders as `[]`
		// rather than `null` for parameterless operations. The model
		// querying spec.operations[i].parameters can then trust an
		// empty array to mean "no parameters expected" without having
		// to disambiguate from a missing field.
		params := make([]paramProjection, 0, len(op.Params))
		params = append(params, op.Params...)

		projection.Operations = append(projection.Operations, opProjection{
			OperationID:  op.ID,
			Method:       op.Method,
			Path:         op.Path,
			Summary:      op.Summary,
			Description:  op.Description,
			Tags:         op.Tags,
			HasBody:      op.HasBody,
			BodyRequired: op.BodyRequired,
			Parameters:   params,
			BodyExample:  op.BodyExample,
			ExampleURI:   prefix + "://examples/" + op.ID,
		})
	}

	projection.Schemas = make([]schemaProjection, 0, len(schemas))
	for _, schema := range schemas {
		projection.Schemas = append(projection.Schemas, schemaProjection{
			Name:        schema.Name,
			Description: schema.Description,
			URI:         prefix + "://schemas/" + schema.Name,
		})
	}

	data, err := json.Marshal(projection)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// buildOperationsJSON serializes the enriched operations list (with params
// and body examples) as JSON for the operations resource.
func buildOperationsJSON(operations []MappedOperation) (string, error) {
	type opEntry struct {
		OperationID  string        `json:"operationId"`
		Method       string        `json:"method"`
		Path         string        `json:"path"`
		Summary      string        `json:"summary,omitempty"`
		Description  string        `json:"description,omitempty"`
		Tags         []string      `json:"tags,omitempty"`
		HasBody      bool          `json:"hasBody"`
		BodyRequired bool          `json:"bodyRequired"`
		Parameters   []MappedParam `json:"parameters"`
		BodyExample  any           `json:"bodyExample,omitempty"`
	}

	entries := make([]opEntry, 0, len(operations))
	for _, op := range operations {
		// Always emit a non-nil slice so the JSON field is `[]` for
		// parameterless ops rather than `null` (or omitted).
		params := make([]MappedParam, 0, len(op.Params))
		params = append(params, op.Params...)

		entries = append(entries, opEntry{
			OperationID:  op.ID,
			Method:       op.Method,
			Path:         op.Path,
			Summary:      op.Summary,
			Description:  op.Description,
			Tags:         op.Tags,
			HasBody:      op.HasBody,
			BodyRequired: op.BodyRequired,
			Parameters:   params,
			BodyExample:  op.BodyExample,
		})
	}

	data, err := json.Marshal(entries)
	if err != nil {
		return "", fmt.Errorf("marshaling operations: %w", err)
	}

	return string(data), nil
}

func extensionKey(spec *openapi.Operation) string {
	if spec.OperationID != "" {
		return spec.OperationID
	}

	return strings.ToUpper(spec.HTTPMethod) + " " + pathString(spec.Path)
}

func fallbackOperationID(method, path string) string {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	parts := make([]string, 0, len(segments)+1)
	parts = append(parts, strings.ToLower(method))

	for _, seg := range segments {
		if seg == "" {
			continue
		}

		seg = strings.TrimPrefix(seg, "{")
		seg = strings.TrimSuffix(seg, "}")
		parts = append(parts, naming.ToSnakeCase(seg))
	}

	return strings.Join(parts, "_")
}

func pathString(path openapi.Path) string {
	return path.String()
}
