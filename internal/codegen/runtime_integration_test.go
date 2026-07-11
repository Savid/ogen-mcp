package codegen_test

import (
	"context"
	"encoding/base64"
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

// TestGeneratedEngineRuntimeContract compiles the generated JS engine
// into a throwaway binary and exercises its runtime behavior:
//
//   - Per the MCP tool-error contract (and issue #2), goja parse errors,
//     thrown exceptions, runtime TypeErrors, and body validation errors
//     must come back as CallToolResult{IsError: true} with a nil Go
//     error — not as Go errors that would terminate the MCP session.
//   - Body handling end-to-end against a real HTTP server: JSON default
//     (including null), text and base64 encodings, Content-Type
//     defaulting and override, and base64-encoded binary responses.
func TestGeneratedEngineRuntimeContract(t *testing.T) {
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

	assertBodyContract(t, report.Body)
}

// assertBodyContract checks the body-handling cases the probe ran against
// its echo server: request bytes and Content-Type per encoding, plus the
// response-side plain-text and base64-binary paths.
func assertBodyContract(t *testing.T, cases []probeCase) {
	t.Helper()

	// Expected request bytes and Content-Type per body case, as observed
	// by the probe's echo server. Absent bodies must echo empty bytes and
	// no defaulted Content-Type.
	wantEcho := map[string]struct {
		bodyB64     string
		contentType string
	}{
		"json_object":           {b64(`{"a":1}`), "application/json"},
		"json_null":             {b64(`null`), "application/json"},
		"text_body":             {b64("héllo"), "text/plain; charset=utf-8"},
		"base64_body":           {"AAEC/w==", "application/octet-stream"},
		"content_type_override": {b64("x"), "application/xml"},
		"no_body":               {"", ""},
	}

	envelopes := make(map[string]executeEnvelope, len(cases))
	for _, c := range cases {
		t.Run("body_"+c.Name, func(t *testing.T) {
			t.Parallel()
			require.Empty(t, c.GoError)
			require.False(t, c.IsError, "tool result text: %s", c.Text)
		})

		var env executeEnvelope
		require.NoError(t, json.Unmarshal([]byte(c.Text), &env),
			"case %q result: %s", c.Name, c.Text)
		envelopes[c.Name] = env
	}

	for name, want := range wantEcho {
		t.Run("body_echo_"+name, func(t *testing.T) {
			t.Parallel()
			env, ok := envelopes[name]
			require.True(t, ok, "probe did not report case %q", name)

			var echoed echoPayload
			require.NoError(t, json.Unmarshal(env.Body, &echoed))
			assert.Equal(t, want.bodyB64, echoed.BodyB64)
			assert.Equal(t, want.contentType, echoed.ContentType)
		})
	}

	t.Run("body_binary_response_base64", func(t *testing.T) {
		t.Parallel()
		env, ok := envelopes["binary_response"]
		require.True(t, ok)
		assert.Equal(t, "base64", env.Encoding)
		assert.JSONEq(t, `"`+b64(string([]byte{0xff, 0xfe, 0x00, 0x01}))+`"`, string(env.Body))
	})

	t.Run("body_text_response_plain", func(t *testing.T) {
		t.Parallel()
		env, ok := envelopes["text_response"]
		require.True(t, ok)
		assert.Empty(t, env.Encoding)
		assert.JSONEq(t, `"plain text, not json"`, string(env.Body))
	})
}

func b64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// executeEnvelope mirrors the api.request result shape in the tool text.
type executeEnvelope struct {
	StatusCode int             `json:"statusCode"`
	Body       json.RawMessage `json:"body"`
	Encoding   string          `json:"encoding"`
}

// echoPayload mirrors the probe echo server's response body.
type echoPayload struct {
	ContentType string `json:"contentType"`
	BodyB64     string `json:"bodyB64"`
}

// probeReport mirrors the JSON emitted by the probe program.
type probeReport struct {
	Cases []probeCase `json:"cases"`
	Body  []probeCase `json:"body"`
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
//   - runs body-handling scripts against a second JSEngine backed by an
//     HTTPTransport pointed at a local echo server,
//   - emits a JSON report on stdout describing each case's outcome.
//
// The outer test unmarshals the report and asserts the tool-error and
// body-handling contracts.
const probeProgram = `package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
	Body  []probeCase ` + "`json:\"body\"`" + `
	Happy probeCase   ` + "`json:\"happy\"`" + `
}

// newEchoServer serves endpoints for body-handling cases: /echo reports
// the received body (base64) and Content-Type, /binary returns bytes
// that are neither JSON nor valid UTF-8, /text returns plain text.
func newEchoServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"contentType": r.Header.Get("Content-Type"),
			"bodyB64":     base64.StdEncoding.EncodeToString(b),
		})
	})
	mux.HandleFunc("/binary", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write([]byte{0xff, 0xfe, 0x00, 0x01})
	})
	mux.HandleFunc("/text", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("plain text, not json"))
	})
	return httptest.NewServer(mux)
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

	srv := newEchoServer()
	defer srv.Close()

	httpEngine, err := NewJSEngine(NewHTTPTransport(srv.URL), nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "http engine init: %v\n", err)
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
			record("body_bad_encoding",
				"return api.request({ method: \"POST\", path: \"/echo\", body: \"x\", encoding: \"gzip\" });",
				"field \"encoding\" must be", httpEngine.RunExecute),
			record("body_encoding_without_body",
				"return api.request({ method: \"GET\", path: \"/echo\", encoding: \"text\" });",
				"field \"encoding\" requires \"body\"", httpEngine.RunExecute),
			record("body_bad_base64",
				"return api.request({ method: \"PUT\", path: \"/echo\", body: \"%%%\", encoding: \"base64\" });",
				"decoding base64 body", httpEngine.RunExecute),
			record("body_non_string_text",
				"return api.request({ method: \"PUT\", path: \"/echo\", body: { a: 1 }, encoding: \"text\" });",
				"field \"body\" must be a string", httpEngine.RunExecute),
		},
		Body: []probeCase{
			record("json_object",
				"return api.request({ method: \"POST\", path: \"/echo\", body: { a: 1 } });",
				"", httpEngine.RunExecute),
			record("json_null",
				"return api.request({ method: \"POST\", path: \"/echo\", body: null });",
				"", httpEngine.RunExecute),
			record("text_body",
				"return api.request({ method: \"PUT\", path: \"/echo\", body: \"héllo\", encoding: \"text\" });",
				"", httpEngine.RunExecute),
			record("base64_body",
				"return api.request({ method: \"PUT\", path: \"/echo\", body: \"AAEC/w==\", encoding: \"base64\" });",
				"", httpEngine.RunExecute),
			record("content_type_override",
				"return api.request({ method: \"PUT\", path: \"/echo\", body: \"x\", encoding: \"text\", headers: { \"content-type\": \"application/xml\" } });",
				"", httpEngine.RunExecute),
			record("no_body",
				"return api.request({ method: \"GET\", path: \"/echo\" });",
				"", httpEngine.RunExecute),
			record("binary_response",
				"return api.request({ method: \"GET\", path: \"/binary\" });",
				"", httpEngine.RunExecute),
			record("text_response",
				"return api.request({ method: \"GET\", path: \"/text\" });",
				"", httpEngine.RunExecute),
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
