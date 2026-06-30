// Package codegen renders Go source files from a MappedAPI using
// embedded templates and go/format.
package codegen

import (
	"bytes"
	"fmt"
	"go/format"
	"strconv"
	"strings"
	"text/template"

	"github.com/savid/ogen-mcp/internal/mapper"
)

// templateOrder defines the rendering order for template fragments.
var templateOrder = []string{
	"all.go.tmpl",
}

// Generate renders the MappedAPI into a single formatted Go source file.
func Generate(api *mapper.MappedAPI) ([]byte, error) {
	funcMap := template.FuncMap{
		"goQuote":   strconv.Quote,
		"stringLit": stringLit,
		"title":     title,
	}

	tmpl, err := template.New("").Funcs(funcMap).ParseFS(templateFS, "templates/*.go.tmpl")
	if err != nil {
		return nil, fmt.Errorf("parsing templates: %w", err)
	}

	var buf bytes.Buffer

	for _, name := range templateOrder {
		if execErr := tmpl.ExecuteTemplate(&buf, name, api); execErr != nil {
			return nil, fmt.Errorf("executing template %s: %w", name, execErr)
		}
	}

	src, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("formatting generated code: %w (raw output:\n%s)", err, buf.String())
	}

	return src, nil
}

func title(s string) string {
	if s == "" {
		return ""
	}

	runes := []rune(s)
	runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]

	return string(runes)
}

// stringLit escapes a string for embedding as a Go string literal.
// It uses raw string syntax when possible.
func stringLit(s string) string {
	if !strings.Contains(s, "`") {
		return "`" + s + "`"
	}

	return strconv.Quote(s)
}
