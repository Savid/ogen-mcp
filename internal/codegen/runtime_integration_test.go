package codegen_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/savid/ogen-mcp/internal/codegen"
	"github.com/savid/ogen-mcp/internal/mapper"
	specparser "github.com/savid/ogen-mcp/internal/parser"
)

// TestGeneratedEngineSurfacesJSErrorsAsIsError compiles the generated JS
// engine into a throwaway binary and exercises it against scripts that
// fail in user code. Per the MCP tool-error contract (and issue #2),
// goja parse errors, thrown exceptions, and runtime TypeErrors must
// come back as CallToolResult{IsError: true} with a nil Go error — not
// as Go errors that would terminate the MCP session.
func TestGeneratedEngineSurfacesJSErrorsAsIsError(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("skipping runtime integration test in short mode")
	}

	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available in PATH")
	}

	dir := t.TempDir()
	writeGeneratedFixture(t, dir)

	bin := filepath.Join(dir, "probe")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	ctx := context.Background()

	// #nosec G204 -- fixed "go mod tidy" run inside t.TempDir().
	tidy := exec.CommandContext(ctx, "go", "mod", "tidy")
	tidy.Dir = dir
	tidy.Env = append(os.Environ(), "GOFLAGS=")
	if out, err := tidy.CombinedOutput(); err != nil {
		// Likely offline or module proxy unreachable; the behavior
		// we're testing is well-defined without network access, so
		// skip rather than flake.
		t.Skipf("go mod tidy failed (no network?): %v\n%s", err, out)
	}

	// #nosec G204 -- bin is a path inside t.TempDir() that we just wrote
	// ourselves; there's no external taint.
	build := exec.CommandContext(ctx, "go", "build", "-o", bin, ".")
	build.Dir = dir
	build.Env = append(os.Environ(), "GOFLAGS=")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building probe binary: %v\n%s", err, out)
	}

	// #nosec G204 -- bin is the binary we just built in t.TempDir().
	out, err := exec.CommandContext(ctx, bin).CombinedOutput()
	require.NoErrorf(t, err, "probe exited non-zero: %s", out)

	var report probeReport
	require.NoError(t, json.Unmarshal(out, &report), "probe output: %s", out)

	for _, c := range report.Cases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			assert.Empty(t, c.GoError,
				"case %q: user-code failures must not surface as Go errors", c.Name)
			assert.True(t, c.IsError,
				"case %q: expected CallToolResult.IsError=true", c.Name)
			assert.Contains(t, strings.ToLower(c.Text), strings.ToLower(c.WantSubstr),
				"case %q: tool result text should contain %q, got %q",
				c.Name, c.WantSubstr, c.Text)
		})
	}

	t.Run("happy_path", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, report.Happy.GoError)
		assert.False(t, report.Happy.IsError)
		assert.Equal(t, `"ok"`, report.Happy.Text)
	})
}

// probeReport mirrors the JSON emitted by the probe program.
type probeReport struct {
	Cases []probeCase `json:"cases"`
	Happy probeCase   `json:"happy"`
}

type probeCase struct {
	Name       string `json:"name"`
	GoError    string `json:"goError"`
	IsError    bool   `json:"isError"`
	Text       string `json:"text"`
	WantSubstr string `json:"wantSubstr"`
}

func writeGeneratedFixture(t *testing.T, dir string) {
	t.Helper()

	data, err := os.ReadFile("../../testdata/petstore.yaml")
	require.NoError(t, err)

	parsed, err := specparser.Parse(data)
	require.NoError(t, err)

	mapped, err := mapper.Map(parsed, mapper.MapOptions{PackageName: "main"})
	require.NoError(t, err)

	gen, err := codegen.Generate(mapped)
	require.NoError(t, err)

	// #nosec G304 G703 -- dir comes from t.TempDir(); gen is bytes we just
	// produced from the codegen template. No external taint.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "generated.go"), gen, 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "probe.go"), []byte(probeProgram), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte(probeGoMod), 0o600))
}

// probeGoMod is minimal on purpose: `go mod tidy` below resolves
// goja + mcp-go-sdk to whatever versions the module proxy serves. This
// test is about the generated error-handling contract, not dep pinning.
const probeGoMod = `module probe

go 1.26
`

// probeProgram is a standalone Go program that:
//   - instantiates the generated JSEngine with a transport stub,
//   - exercises RunSearch / RunExecute with deliberately broken JS,
//   - emits a JSON report on stdout describing each case's outcome.
//
// The outer test unmarshals the report and asserts the tool-error
// contract: IsError=true, Go error empty, text contains a diagnostic.
const probeProgram = `package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type nopTransport struct{}

func (nopTransport) DoAPIRequest(ctx context.Context, req APIRequest) (*APIResponse, error) {
	return &APIResponse{StatusCode: 200, Headers: map[string]string{}, Body: nil}, nil
}

type probeCase struct {
	Name       string ` + "`json:\"name\"`" + `
	GoError    string ` + "`json:\"goError\"`" + `
	IsError    bool   ` + "`json:\"isError\"`" + `
	Text       string ` + "`json:\"text\"`" + `
	WantSubstr string ` + "`json:\"wantSubstr\"`" + `
}

type report struct {
	Cases []probeCase ` + "`json:\"cases\"`" + `
	Happy probeCase   ` + "`json:\"happy\"`" + `
}

type runner func(ctx context.Context, code string) (*mcp.CallToolResult, error)

func record(name, code, want string, run runner) probeCase {
	res, err := run(context.Background(), code)
	c := probeCase{Name: name, WantSubstr: want}
	if err != nil {
		c.GoError = err.Error()
		return c
	}
	if res == nil {
		c.GoError = "nil result"
		return c
	}
	c.IsError = res.IsError
	if len(res.Content) > 0 {
		if tc, ok := res.Content[0].(*mcp.TextContent); ok {
			c.Text = tc.Text
		}
	}
	return c
}

func main() {
	engine, err := NewJSEngine(nopTransport{}, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "engine init: %v\n", err)
		os.Exit(1)
	}

	r := report{
		Cases: []probeCase{
			record("search_syntax_error",
				"return this is not valid javascript ][;",
				"SyntaxError", engine.RunSearch),
			record("search_runtime_type_error",
				"return undefined.foo;",
				"TypeError", engine.RunSearch),
			record("search_thrown_exception",
				"throw new Error('custom boom');",
				"boom", engine.RunSearch),
			record("execute_syntax_error",
				"return function( ",
				"SyntaxError", engine.RunExecute),
			record("execute_runtime_type_error",
				"return null.x;",
				"TypeError", engine.RunExecute),
			record("execute_thrown_exception",
				"throw new Error('execute boom');",
				"boom", engine.RunExecute),
		},
		Happy: record("happy", "return 'ok';", "", engine.RunSearch),
	}

	enc := json.NewEncoder(os.Stdout)
	if err := enc.Encode(r); err != nil {
		fmt.Fprintf(os.Stderr, "encoding report: %v\n", err)
		os.Exit(1)
	}
}
`
