package codegen

import (
	"go/parser"
	"go/token"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/savid/ogen-mcp/internal/mapper"
	specparser "github.com/savid/ogen-mcp/internal/parser"
)

const (
	testEngineInterface = "Engine"
	testSearchToolName  = "search"
	testExecuteToolName = "execute"
	testResourcePrefix  = "openapi"
)

func parsePetstoreMapped(t *testing.T) *mapper.MappedAPI {
	t.Helper()

	data, err := os.ReadFile("../../testdata/petstore.yaml")
	require.NoError(t, err)

	result, err := specparser.Parse(data)
	require.NoError(t, err)

	mapped, err := mapper.Map(result, mapper.MapOptions{PackageName: "mcpserver"})
	require.NoError(t, err)

	return mapped
}

func TestGeneratePetstore(t *testing.T) {
	t.Parallel()

	mapped := parsePetstoreMapped(t)

	got, err := Generate(mapped)
	require.NoError(t, err)

	fset := token.NewFileSet()
	_, parseErr := parser.ParseFile(fset, "generated.go", got, parser.AllErrors)
	require.NoError(t, parseErr, "generated code is not valid Go:\n%s", string(got))

	output := string(got)
	assert.Contains(t, output, "type Engine interface")
	assert.Contains(t, output, "RunSearch")
	assert.Contains(t, output, "RunExecute")
	assert.Contains(t, output, "type JSEngine struct")
	assert.Contains(t, output, "codeInputSchema")
	assert.Contains(t, output, "openapi://schemas")
	assert.Contains(t, output, "openapi://examples")
	assert.Contains(t, output, `"://operations"`)
	assert.Contains(t, output, "handleOperationsList")
	assert.Contains(t, output, "api.request")

	// Phase A helpers.
	assert.Contains(t, output, "type HTTPTransport struct")
	assert.Contains(t, output, "func NewHTTPTransport")
	assert.Contains(t, output, "func WithEngineHook")
	assert.Contains(t, output, "func WithTransportHook")
	assert.Contains(t, output, "func BuildExecuteCode")
	assert.Contains(t, output, "func ParseExecuteResult")
	assert.Contains(t, output, "type ExecuteResponse struct")

	// Body handling: a single body field with an encoding modifier
	// (json default, text, base64), normalized to bytes before the
	// transport; binary responses come back base64-encoded.
	assert.Contains(t, output, "func normalizeBody")
	assert.Contains(t, output, `field "encoding" must be "json", "text" or "base64"`)
	assert.Contains(t, output, `field "encoding" requires "body"`)
	assert.Contains(t, output, "base64.StdEncoding.DecodeString")
	assert.Contains(t, output, "base64.StdEncoding.EncodeToString(respBody)")
	assert.Contains(t, output, "func WithTextBody")
	assert.Contains(t, output, "func WithBinaryBody")

	// User-code JS errors must surface as IsError tool results, not Go
	// errors, so MCP consumers can forward them in a tool_result block.
	// See https://github.com/savid/ogen-mcp/issues/2.
	assert.Contains(t, output, "func userCodeErrorResult(err error) *mcp.CallToolResult")
	assert.Contains(t, output, "IsError: true")
	assert.NotContains(t, output, `"running search code: %w"`)
	assert.NotContains(t, output, `"running execute code: %w"`)
	assert.Contains(t, output, "return userCodeErrorResult(err), nil")
}

func TestGenerateNoResources(t *testing.T) {
	t.Parallel()

	api := &mapper.MappedAPI{
		PackageName:             "nores",
		EngineInterface:         testEngineInterface,
		SearchToolName:          testSearchToolName,
		ExecuteToolName:         testExecuteToolName,
		ResourceURIPrefix:       testResourcePrefix,
		Runtime:                 "js",
		ExamplesMode:            "none",
		ExecuteMaxRequests:      10,
		ExecuteTimeout:          0,
		ExecuteMaxResponseBytes: 1024,
		ExecuteMaxOutputBytes:   2048,
		ExecuteMaxRedirects:     3,
		SearchDescription:       testSearchToolName,
		ExecuteDescription:      testExecuteToolName,
		SpecJSON:                `{"operations":[]}`,
		OperationsJSON:          "",
		Operations:              nil,
		Schemas:                 nil,
		Examples:                nil,
	}

	got, err := Generate(api)
	require.NoError(t, err)

	fset := token.NewFileSet()
	_, parseErr := parser.ParseFile(fset, "generated.go", got, parser.AllErrors)
	require.NoError(t, parseErr, "generated code is not valid Go:\n%s", string(got))

	output := string(got)
	assert.Contains(t, output, "var schemaContents = map[string]string{}")
	assert.Contains(t, output, "var exampleContents = map[string]exampleRecord{}")
	// When OperationsJSON is empty, the runtime guard prevents registration.
	assert.Contains(t, output, "embeddedOperationsJSON = ``")
}

func TestGenerateOperationsResource(t *testing.T) {
	t.Parallel()

	mapped := parsePetstoreMapped(t)

	got, err := Generate(mapped)
	require.NoError(t, err)

	fset := token.NewFileSet()
	_, parseErr := parser.ParseFile(fset, "generated.go", got, parser.AllErrors)
	require.NoError(t, parseErr, "generated code is not valid Go:\n%s", string(got))

	output := string(got)
	assert.Contains(t, output, `"://operations"`)
	assert.Contains(t, output, "handleOperationsList")
	assert.Contains(t, output, "embeddedOperationsJSON")
}

func TestGenerateOperationsResourceAbsentWhenEmpty(t *testing.T) {
	t.Parallel()

	api := &mapper.MappedAPI{
		PackageName:             "noops",
		EngineInterface:         testEngineInterface,
		SearchToolName:          testSearchToolName,
		ExecuteToolName:         testExecuteToolName,
		ResourceURIPrefix:       testResourcePrefix,
		Runtime:                 "js",
		ExamplesMode:            "none",
		ExecuteMaxRequests:      10,
		ExecuteTimeout:          0,
		ExecuteMaxResponseBytes: 1024,
		ExecuteMaxOutputBytes:   2048,
		ExecuteMaxRedirects:     3,
		SearchDescription:       testSearchToolName,
		ExecuteDescription:      testExecuteToolName,
		SpecJSON:                `{"operations":[]}`,
		OperationsJSON:          "",
	}

	got, err := Generate(api)
	require.NoError(t, err)

	fset := token.NewFileSet()
	_, parseErr := parser.ParseFile(fset, "generated.go", got, parser.AllErrors)
	require.NoError(t, parseErr, "generated code is not valid Go:\n%s", string(got))

	output := string(got)
	// When OperationsJSON is empty, the runtime guard prevents registration.
	assert.Contains(t, output, "embeddedOperationsJSON = ``")
}

func TestGenerateEscapedStrings(t *testing.T) {
	t.Parallel()

	api := &mapper.MappedAPI{
		PackageName:             "escapes",
		EngineInterface:         testEngineInterface,
		SearchToolName:          testSearchToolName,
		ExecuteToolName:         testExecuteToolName,
		ResourceURIPrefix:       testResourcePrefix,
		Runtime:                 "js",
		ExamplesMode:            "inline",
		ExecuteMaxRequests:      10,
		ExecuteTimeout:          0,
		ExecuteMaxResponseBytes: 1024,
		ExecuteMaxOutputBytes:   2048,
		ExecuteMaxRedirects:     3,
		SearchDescription:       "desc with \"quotes\" and\nnewlines",
		ExecuteDescription:      "another\n\"desc\"",
		SpecJSON:                `{"name":"demo"}`,
		OperationsJSON:          `[{"operationId":"test"}]`,
	}

	got, err := Generate(api)
	require.NoError(t, err)

	fset := token.NewFileSet()
	_, parseErr := parser.ParseFile(fset, "generated.go", got, parser.AllErrors)
	require.NoError(t, parseErr, "generated code is not valid Go:\n%s", string(got))
}
