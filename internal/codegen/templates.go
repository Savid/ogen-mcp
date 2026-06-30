package codegen

import "embed"

//go:embed templates/*.go.tmpl
var templateFS embed.FS
