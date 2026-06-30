// Package parser wraps ogen to parse an OpenAPI spec and extract IR
// operations along with x-mcp-* extensions.
package parser

import (
	"fmt"
	"strings"

	"github.com/go-faster/yaml"
	"github.com/ogen-go/ogen"
	"github.com/ogen-go/ogen/gen"
	"github.com/ogen-go/ogen/gen/ir"
	"go.uber.org/zap"
)

// MCPExtensions holds x-mcp-* extension values for a single operation.
type MCPExtensions struct {
	// Exclude marks an operation as excluded from code generation.
	Exclude bool
}

// Result holds the parsed output from an OpenAPI spec.
type Result struct {
	// Operations are the IR operations produced by ogen's generator.
	Operations []*ir.Operation
	// Extensions maps an operation key (operationID or "METHOD /path")
	// to its x-mcp-* extensions extracted from the raw spec.
	Extensions map[string]MCPExtensions
	// RawSpec is the parsed ogen spec, retained so downstream consumers
	// can access components and spec metadata.
	RawSpec *ogen.Spec
}

// Parse parses an OpenAPI spec from raw bytes and returns IR operations
// with associated MCP extensions.
func Parse(specBytes []byte) (*Result, error) {
	if compatErr := validateOpenAPI31Compatibility(specBytes); compatErr != nil {
		return nil, compatErr
	}

	spec, err := ogen.Parse(specBytes)
	if err != nil {
		return nil, fmt.Errorf("parsing openapi spec: %w", err)
	}

	extensions, err := extractExtensions(spec)
	if err != nil {
		return nil, err
	}

	g, err := gen.NewGenerator(spec, gen.Options{
		Generator: gen.GenerateOptions{
			IgnoreNotImplemented: []string{"all"},
		},
		Logger: zap.NewNop(),
	})
	if err != nil {
		return nil, fmt.Errorf("creating ogen generator: %w", err)
	}

	return &Result{
		Operations: g.Operations(),
		Extensions: extensions,
		RawSpec:    spec,
	}, nil
}

// operationKey returns the canonical key for an operation, preferring
// operationID and falling back to "METHOD /path".
func operationKey(operationID, method, path string) string {
	if operationID != "" {
		return operationID
	}

	return strings.ToUpper(method) + " " + path
}

// extractExtensions walks the raw spec paths and extracts x-mcp-*
// extensions from each operation.
func extractExtensions(spec *ogen.Spec) (map[string]MCPExtensions, error) {
	result := make(map[string]MCPExtensions, len(spec.Paths))

	for path, item := range spec.Paths {
		if item == nil {
			continue
		}

		methods := []struct {
			name string
			op   *ogen.Operation
		}{
			{"GET", item.Get},
			{"HEAD", item.Head},
			{"POST", item.Post},
			{"PUT", item.Put},
			{"PATCH", item.Patch},
			{"DELETE", item.Delete},
			{"OPTIONS", item.Options},
			{"TRACE", item.Trace},
		}

		for _, m := range methods {
			if m.op == nil {
				continue
			}

			ext, parseErr := parseMCPExtensions(m.op.Common.Extensions)
			if parseErr != nil {
				key := operationKey(m.op.OperationID, m.name, path)
				return nil, fmt.Errorf("parsing extensions for %s: %w", key, parseErr)
			}

			if ext.Exclude {
				key := operationKey(m.op.OperationID, m.name, path)
				result[key] = ext
			}
		}
	}

	return result, nil
}

// parseMCPExtensions extracts x-mcp-* values from a raw extensions map.
func parseMCPExtensions(extensions map[string]yaml.Node) (MCPExtensions, error) {
	var ext MCPExtensions

	if node, ok := extensions["x-mcp-exclude"]; ok {
		if err := node.Decode(&ext.Exclude); err != nil {
			return MCPExtensions{}, fmt.Errorf("x-mcp-exclude must be boolean: %w", err)
		}
	}

	return ext, nil
}
