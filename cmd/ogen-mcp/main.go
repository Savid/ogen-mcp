// Package main provides the ogen-mcp CLI that generates MCP Code Mode
// boilerplate from an OpenAPI spec.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/savid/ogen-mcp/internal/codegen"
	"github.com/savid/ogen-mcp/internal/mapper"
	"github.com/savid/ogen-mcp/internal/parser"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	var opts struct {
		input                   string
		output                  string
		pkg                     string
		engineInterface         string
		searchTool              string
		executeTool             string
		resourceURIPrefix       string
		runtime                 string
		examples                string
		noResources             bool
		executeMaxRequests      int
		executeTimeout          time.Duration
		executeMaxResponseBytes int
		executeMaxOutputBytes   int
		executeMaxRedirects     int
	}

	cmd := &cobra.Command{
		Use:   "ogen-mcp",
		Short: "Generate MCP Code Mode boilerplate from an OpenAPI spec",
		Long: `ogen-mcp generates a fixed two-tool MCP surface (search + execute)
from an OpenAPI specification. Generated code uses a JavaScript runtime for
Code Mode with guardrails and optional schema/example resources.`,
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return run(logger, opts.input, opts.output, mapper.MapOptions{
				PackageName:             opts.pkg,
				EngineInterface:         opts.engineInterface,
				SearchToolName:          opts.searchTool,
				ExecuteToolName:         opts.executeTool,
				ResourceURIPrefix:       opts.resourceURIPrefix,
				Runtime:                 opts.runtime,
				ExamplesMode:            opts.examples,
				NoResources:             opts.noResources,
				ExecuteMaxRequests:      opts.executeMaxRequests,
				ExecuteTimeout:          opts.executeTimeout,
				ExecuteMaxResponseBytes: opts.executeMaxResponseBytes,
				ExecuteMaxOutputBytes:   opts.executeMaxOutputBytes,
				ExecuteMaxRedirects:     opts.executeMaxRedirects,
			})
		},
	}

	flags := cmd.Flags()
	flags.StringVarP(&opts.input, "input", "i", "", "path to OpenAPI spec (required)")
	flags.StringVarP(&opts.output, "output", "o", "", "output file path (default: stdout)")
	flags.StringVar(&opts.pkg, "package", "mcpserver", "Go package name")
	flags.StringVar(&opts.engineInterface, "engine-interface", "Engine", "generated engine interface name")
	flags.StringVar(&opts.searchTool, "search-tool", "search", "MCP search tool name")
	flags.StringVar(&opts.executeTool, "execute-tool", "execute", "MCP execute tool name")
	flags.StringVar(&opts.resourceURIPrefix, "resource-uri-prefix", "openapi", "URI scheme prefix for generated resources")
	flags.StringVar(&opts.runtime, "runtime", "js", "runtime to generate (currently only: js)")
	flags.StringVar(&opts.examples, "examples", "resources", "example mode: resources|inline|none")
	flags.BoolVar(&opts.noResources, "no-resources", false, "skip schema/example resource generation")
	flags.IntVar(&opts.executeMaxRequests, "execute-max-requests", 10, "max API requests per execute code run")
	flags.DurationVar(&opts.executeTimeout, "execute-timeout", 5*time.Second, "max wall time per tool code run")
	flags.IntVar(&opts.executeMaxResponseBytes, "execute-max-response-bytes", 256*1024, "max bytes captured per API response body")
	flags.IntVar(&opts.executeMaxOutputBytes, "execute-max-output-bytes", 512*1024, "max total execute output bytes")
	flags.IntVar(&opts.executeMaxRedirects, "execute-max-redirects", 3, "max redirects allowed per API request")

	_ = cmd.MarkFlagRequired("input")

	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func run(logger *slog.Logger, input, output string, mapOpts mapper.MapOptions) error {
	logger.Info("reading spec", slog.String("path", input))

	specBytes, err := os.ReadFile(input) //nolint:gosec // user-provided input path is intentional
	if err != nil {
		return fmt.Errorf("reading spec file: %w", err)
	}

	logger.Info("parsing spec")

	result, err := parser.Parse(specBytes)
	if err != nil {
		return fmt.Errorf("parsing spec: %w", err)
	}

	logger.Info("mapping operations",
		slog.Int("operations", len(result.Operations)),
		slog.Int("extensions", len(result.Extensions)),
	)

	mapped, err := mapper.Map(result, mapOpts)
	if err != nil {
		return fmt.Errorf("mapping operations: %w", err)
	}

	logger.Info("generating code",
		slog.Int("operations", len(mapped.Operations)),
		slog.Int("schemas", len(mapped.Schemas)),
		slog.Int("examples", len(mapped.Examples)),
	)

	src, err := codegen.Generate(mapped)
	if err != nil {
		return fmt.Errorf("generating code: %w", err)
	}

	if output == "" {
		_, err = os.Stdout.Write(src)

		return err //nolint:wrapcheck // stdout write error needs no wrapping
	}

	if writeErr := os.WriteFile(output, src, 0o600); writeErr != nil { //nolint:gosec // user-provided output path is intentional
		return fmt.Errorf("writing output file: %w", writeErr)
	}

	logger.Info("wrote output", slog.String("path", output))

	return nil
}
