# ogen-mcp

`ogen-mcp` generates a **Code Mode MCP server scaffold** from an OpenAPI spec.

Instead of one MCP tool per endpoint, generated code exposes two fixed tools:

- `search` with `{ "code": "..." }` for read-only discovery against `spec`
- `execute` with `{ "code": "..." }` for request execution through `api.request(...)`

This keeps tool surface and prompt footprint stable as your API grows.

## Install

```bash
go install github.com/savid/ogen-mcp/cmd/ogen-mcp@latest
```

Or build from source:

```bash
git clone https://github.com/savid/ogen-mcp.git
cd ogen-mcp
go build ./cmd/ogen-mcp/
```

Requires **Go 1.26+**.

## Quick Start

```bash
ogen-mcp -i openapi.yaml -o mcpserver/mcp_gen.go --package mcpserver
```

## OpenAPI 3.1 Support

`ogen-mcp` supports OpenAPI 3.1 specs, with one current compatibility guard:

- Supported: normal 3.1 schemas, including nullable modeled as `oneOf` with `{ type: "null" }`
- Unsupported (fails fast): schema `type` arrays that include `"null"` (for example `type: ["string", "null"]`)

When this unsupported pattern is detected, `ogen-mcp` returns a targeted error with the schema path and asks you to rewrite using `oneOf`.

This guard exists because upstream `ogen` still tracks this case separately:
- https://github.com/ogen-go/ogen/issues/1617
- https://github.com/ogen-go/ogen/pull/1619

Generated code includes:

- `Engine` interface (`RunSearch`, `RunExecute`)
- `JSEngine` implementation (Goja-based)
- `RegisterTools()` for `search` and `execute`
- `RegisterResources()` for schemas and examples
  - `openapi://schemas`
  - `openapi://schemas/{name}`
  - `openapi://examples`
  - `openapi://examples/{operationId}`
- `HTTPTransport` — built-in `net/http` transport with functional options (`WithHTTPClient`, `WithRequestHook`)
- `WithEngineHook` / `WithTransportHook` — observability decorators for audit logging, metrics, etc.
- `BuildExecuteCode` / `ParseExecuteResult` — Go client helpers for programmatic MCP usage

## Host Wiring (Go)

```go
package main

import (
	"context"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"yourmod/mcpserver"
)

func main() {
	transport := mcpserver.NewHTTPTransport("https://api.example.com",
		mcpserver.WithRequestHook(func(ctx context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+token)
			return nil
		}),
	)

	engine, err := mcpserver.NewJSEngine(transport, nil)
	if err != nil {
		panic(err)
	}

	srv := mcp.NewServer(&mcp.Implementation{Name: "my-api", Version: "1.0.0"}, nil)
	mcpserver.RegisterTools(srv, engine)
	mcpserver.RegisterResources(srv)
}
```

You can also implement the `APITransport` interface directly for full control over request execution.

## Hooks

Wrap the engine or transport with observability decorators for audit logging, metrics, or tracing:

```go
// Log every search/execute invocation
engine = mcpserver.WithEngineHook(engine, func(
	ctx context.Context, tool, code string,
	result *mcp.CallToolResult, err error, duration time.Duration,
) {
	log.Printf("tool=%s duration=%s err=%v", tool, duration, err)
})

// Log every outgoing API request
transport = mcpserver.WithTransportHook(transport, func(
	ctx context.Context, req mcpserver.APIRequest,
	resp *mcpserver.APIResponse, err error, duration time.Duration,
) {
	log.Printf("method=%s path=%s status=%d duration=%s",
		req.Method, req.Path, resp.StatusCode, duration)
})
```

## Client Helpers

For Go MCP clients that call the generated server programmatically, use `BuildExecuteCode` and `ParseExecuteResult` to construct and parse execute tool calls:

```go
code := mcpserver.BuildExecuteCode("GET", "/pets",
	mcpserver.WithQuery(map[string]any{"limit": 10}),
)
// code == `return api.request({"method":"GET","path":"/pets","query":{"limit":10}});`

result, err := client.CallTool(ctx, &mcp.CallToolRequest{...}, code)
resp, err := mcpserver.ParseExecuteResult(result)
// resp.StatusCode, resp.Headers, resp.Body
```

## Tool Model

### `search`

Runtime globals:

- `spec`: compact OpenAPI projection (info, operations, schema resource URIs)

Example:

```js
const matches = spec.operations.filter(op => op.tags.includes("pets"));
return matches.map(op => ({ id: op.operationId, method: op.method, path: op.path }));
```

### `execute`

Runtime globals:

- `api.request({ method, path, pathParams?, query?, headers?, body? })`

Example (multiple API requests in one code block):

```js
const orders = api.request({ method: "GET", path: "/orders", query: { limit: 3 } });
const pets = api.request({ method: "GET", path: "/pets", query: { limit: 3 } });
return { orders, pets };
```

## Limits and Guardrails

Generated engine enforces limits for `execute`:

- max requests per run
- timeout per run
- max response body bytes captured
- max output bytes returned
- max redirects per request

Configure at generation time with CLI flags, and at runtime through `EngineOptions`.

## CLI Reference

```text
ogen-mcp [flags]

Flags:
  -i, --input string                     path to OpenAPI spec (required)
  -o, --output string                    output file path (default: stdout)
      --package string                   Go package name (default "mcpserver")
      --engine-interface string          generated engine interface name (default "Engine")
      --search-tool string               MCP search tool name (default "search")
      --execute-tool string              MCP execute tool name (default "execute")
      --resource-uri-prefix string       URI scheme prefix for generated resources (default "openapi")
      --runtime string                   runtime to generate (currently only: js) (default "js")
      --examples string                  example mode: resources|inline|none (default "resources")
      --no-resources                     skip schema/example resource generation
      --execute-max-requests int         max API requests per execute code run (default 10)
      --execute-timeout duration         max wall time per tool code run (default 5s)
      --execute-max-response-bytes int   max bytes captured per API response body (default 262144)
      --execute-max-output-bytes int     max total execute output bytes (default 524288)
      --execute-max-redirects int        max redirects allowed per API request (default 3)
  -h, --help                             help for ogen-mcp
```

## License

GPLv3 — see [LICENSE](LICENSE).
