// Package mapper transforms parsed OpenAPI operations into a Code Mode
// representation suitable for generation.
package mapper

import "time"

const (
	examplesModeResources = "resources"
	examplesModeInline    = "inline"
	examplesModeNone      = "none"
)

const (
	defaultPackageName      = "mcpserver"
	defaultEngineInterface  = "Engine"
	defaultSearchToolName   = "search"
	defaultExecuteToolName  = "execute"
	defaultResourcePrefix   = "openapi"
	defaultRuntime          = "js"
	defaultExamplesMode     = examplesModeResources
	defaultMaxRequests      = 10
	defaultMaxResponseBytes = 256 * 1024
	defaultMaxOutputBytes   = 512 * 1024
	defaultMaxRedirects     = 3
	defaultExecuteTimeout   = 5 * time.Second
)

// MapOptions configures mapping for generated Code Mode server code.
type MapOptions struct {
	PackageName       string
	EngineInterface   string
	SearchToolName    string
	ExecuteToolName   string
	ResourceURIPrefix string
	Runtime           string
	ExamplesMode      string
	NoResources       bool

	ExecuteMaxRequests      int
	ExecuteTimeout          time.Duration
	ExecuteMaxResponseBytes int
	ExecuteMaxOutputBytes   int
	ExecuteMaxRedirects     int
}

func (o *MapOptions) setDefaults() {
	if o.PackageName == "" {
		o.PackageName = defaultPackageName
	}

	if o.EngineInterface == "" {
		o.EngineInterface = defaultEngineInterface
	}

	if o.SearchToolName == "" {
		o.SearchToolName = defaultSearchToolName
	}

	if o.ExecuteToolName == "" {
		o.ExecuteToolName = defaultExecuteToolName
	}

	if o.ResourceURIPrefix == "" {
		o.ResourceURIPrefix = defaultResourcePrefix
	}

	if o.Runtime == "" {
		o.Runtime = defaultRuntime
	}

	if o.ExamplesMode == "" {
		o.ExamplesMode = defaultExamplesMode
	}

	if o.ExecuteMaxRequests <= 0 {
		o.ExecuteMaxRequests = defaultMaxRequests
	}

	if o.ExecuteTimeout <= 0 {
		o.ExecuteTimeout = defaultExecuteTimeout
	}

	if o.ExecuteMaxResponseBytes <= 0 {
		o.ExecuteMaxResponseBytes = defaultMaxResponseBytes
	}

	if o.ExecuteMaxOutputBytes <= 0 {
		o.ExecuteMaxOutputBytes = defaultMaxOutputBytes
	}

	if o.ExecuteMaxRedirects <= 0 {
		o.ExecuteMaxRedirects = defaultMaxRedirects
	}
}

// MappedAPI is the full Code Mode generation input.
type MappedAPI struct {
	PackageName       string
	EngineInterface   string
	SearchToolName    string
	ExecuteToolName   string
	ResourceURIPrefix string
	Runtime           string
	ExamplesMode      string

	ExecuteMaxRequests      int
	ExecuteTimeout          time.Duration
	ExecuteMaxResponseBytes int
	ExecuteMaxOutputBytes   int
	ExecuteMaxRedirects     int

	SearchDescription  string
	ExecuteDescription string
	SpecJSON           string
	OperationsJSON     string
	Operations         []MappedOperation
	Schemas            []MappedSchema
	Examples           []MappedExample
}

// MappedOperation is operation metadata exposed to search and examples.
type MappedOperation struct {
	ID           string
	Method       string
	Path         string
	Summary      string
	Description  string
	Tags         []string
	HasBody      bool
	BodyRequired bool
	Params       []MappedParam
	BodyExample  any
}

// MappedParam is a single operation parameter.
type MappedParam struct {
	Name     string `json:"name"`
	In       string `json:"in"`
	Required bool   `json:"required"`
	Type     string `json:"type"`
	Example  any    `json:"example,omitempty"`
}

// MappedSchema is a component schema resource entry.
type MappedSchema struct {
	Name        string
	Description string
	JSONContent string
}

// MappedExample is a generated executable snippet for a specific operation.
type MappedExample struct {
	OperationID string
	Method      string
	Path        string
	Summary     string
	Code        string
}
