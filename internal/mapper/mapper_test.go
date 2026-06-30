package mapper

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/savid/ogen-mcp/internal/parser"
)

func parsePetstore(t *testing.T) *parser.Result {
	t.Helper()

	data, err := os.ReadFile("../../testdata/petstore.yaml")
	require.NoError(t, err)

	result, err := parser.Parse(data)
	require.NoError(t, err)

	return result
}

func TestMapPetstoreCodeMode(t *testing.T) {
	t.Parallel()

	result := parsePetstore(t)

	mapped, err := Map(result, MapOptions{})
	require.NoError(t, err)

	assert.Equal(t, "mcpserver", mapped.PackageName)
	assert.Equal(t, "Engine", mapped.EngineInterface)
	assert.Equal(t, "search", mapped.SearchToolName)
	assert.Equal(t, "execute", mapped.ExecuteToolName)
	assert.Equal(t, "openapi", mapped.ResourceURIPrefix)
	assert.Equal(t, "js", mapped.Runtime)
	assert.Equal(t, "resources", mapped.ExamplesMode)
	assert.Equal(t, 10, mapped.ExecuteMaxRequests)
	assert.Equal(t, 5*time.Second, mapped.ExecuteTimeout)

	require.Len(t, mapped.Operations, 8) // healthCheck excluded
	require.Len(t, mapped.Examples, 8)
	require.NotEmpty(t, mapped.Schemas)

	ids := make([]string, 0, len(mapped.Operations))
	for _, op := range mapped.Operations {
		ids = append(ids, op.ID)
	}

	assert.Contains(t, ids, "listPets")
	assert.Contains(t, ids, "showPetById")
	assert.Contains(t, ids, "createPet")
	assert.Contains(t, ids, "updatePet")
	assert.Contains(t, ids, "deletePet")
	assert.Contains(t, ids, "listOrders")
	assert.Contains(t, ids, "createOrder")
	assert.Contains(t, ids, "addPetTag")
	assert.NotContains(t, ids, "healthCheck")

	assert.True(t, json.Valid([]byte(mapped.SpecJSON)))
	assert.Contains(t, mapped.SearchDescription, "spec")
	assert.Contains(t, mapped.ExecuteDescription, "api.request")
}

func TestMapBodyExample(t *testing.T) {
	t.Parallel()

	result := parsePetstore(t)

	mapped, err := Map(result, MapOptions{})
	require.NoError(t, err)

	var createPet *MappedOperation
	for i := range mapped.Operations {
		if mapped.Operations[i].ID == "createPet" {
			createPet = &mapped.Operations[i]
			break
		}
	}
	require.NotNil(t, createPet)
	require.True(t, createPet.HasBody)

	body, ok := createPet.BodyExample.(map[string]any)
	require.True(t, ok)
	assert.Contains(t, body, "name")
}

func TestMapNoResources(t *testing.T) {
	t.Parallel()

	result := parsePetstore(t)

	mapped, err := Map(result, MapOptions{NoResources: true})
	require.NoError(t, err)

	assert.Empty(t, mapped.Schemas)
	assert.Empty(t, mapped.Examples)
	assert.NotEmpty(t, mapped.Operations)
}

func TestMapCustomOptions(t *testing.T) {
	t.Parallel()

	result := parsePetstore(t)

	mapped, err := Map(result, MapOptions{
		PackageName:             "myserver",
		EngineInterface:         "RuntimeEngine",
		SearchToolName:          "discover",
		ExecuteToolName:         "act",
		ResourceURIPrefix:       "petstore",
		ExamplesMode:            "inline",
		ExecuteMaxRequests:      25,
		ExecuteTimeout:          12 * time.Second,
		ExecuteMaxResponseBytes: 111,
		ExecuteMaxOutputBytes:   222,
		ExecuteMaxRedirects:     4,
	})
	require.NoError(t, err)

	assert.Equal(t, "myserver", mapped.PackageName)
	assert.Equal(t, "RuntimeEngine", mapped.EngineInterface)
	assert.Equal(t, "discover", mapped.SearchToolName)
	assert.Equal(t, "act", mapped.ExecuteToolName)
	assert.Equal(t, "petstore", mapped.ResourceURIPrefix)
	assert.Equal(t, "inline", mapped.ExamplesMode)
	assert.Equal(t, 25, mapped.ExecuteMaxRequests)
	assert.Equal(t, 12*time.Second, mapped.ExecuteTimeout)
	assert.Equal(t, 111, mapped.ExecuteMaxResponseBytes)
	assert.Equal(t, 222, mapped.ExecuteMaxOutputBytes)
	assert.Equal(t, 4, mapped.ExecuteMaxRedirects)
	assert.Contains(t, mapped.ExecuteDescription, "Operation starters")
}

type specJSONOp struct {
	OperationID string `json:"operationId"`
	Parameters  []struct {
		Name     string `json:"name"`
		In       string `json:"in"`
		Required bool   `json:"required"`
		Type     string `json:"type"`
		Example  any    `json:"example"`
	} `json:"parameters"`
	BodyExample any `json:"bodyExample"`
}

func specJSONParamNames(op specJSONOp) []string {
	names := make([]string, 0, len(op.Parameters))
	for _, p := range op.Parameters {
		names = append(names, p.Name)
	}

	return names
}

func findSpecJSONOp(t *testing.T, specJSON, operationID string) specJSONOp {
	t.Helper()

	var spec struct {
		Operations []specJSONOp `json:"operations"`
	}

	require.NoError(t, json.Unmarshal([]byte(specJSON), &spec))

	for _, op := range spec.Operations {
		if op.OperationID == operationID {
			return op
		}
	}

	t.Fatalf("operation %q not found in spec JSON", operationID)

	return specJSONOp{}
}

func TestBuildSearchSpecJSONIncludesParams(t *testing.T) {
	t.Parallel()

	result := parsePetstore(t)

	mapped, err := Map(result, MapOptions{})
	require.NoError(t, err)
	require.True(t, json.Valid([]byte(mapped.SpecJSON)))

	tests := []struct {
		name           string
		operationID    string
		wantParams     []string
		wantBodyExists bool
	}{
		{
			name:        "showPetById has path param",
			operationID: "showPetById",
			wantParams:  []string{"petId"},
		},
		{
			name:        "listPets has query params",
			operationID: "listPets",
			wantParams:  []string{"limit", "status"},
		},
		{
			name:           "createPet has body example",
			operationID:    "createPet",
			wantBodyExists: true,
		},
		{
			name:        "deletePet has path param only",
			operationID: "deletePet",
			wantParams:  []string{"petId"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			op := findSpecJSONOp(t, mapped.SpecJSON, tt.operationID)

			if len(tt.wantParams) > 0 {
				names := specJSONParamNames(op)
				for _, want := range tt.wantParams {
					assert.Contains(t, names, want)
				}
			}

			if tt.wantBodyExists {
				assert.NotNil(t, op.BodyExample, "operation %q should have bodyExample", tt.operationID)
			}
		})
	}
}

func TestBuildSearchSpecJSONParamDetails(t *testing.T) {
	t.Parallel()

	result := parsePetstore(t)

	mapped, err := Map(result, MapOptions{})
	require.NoError(t, err)

	op := findSpecJSONOp(t, mapped.SpecJSON, "showPetById")

	require.Len(t, op.Parameters, 1)
	assert.Equal(t, "petId", op.Parameters[0].Name)
	assert.Equal(t, "path", op.Parameters[0].In)
	assert.True(t, op.Parameters[0].Required)
	assert.Equal(t, "integer", op.Parameters[0].Type)
	assert.NotNil(t, op.Parameters[0].Example)
}

func TestBuildSearchSpecJSONBodyExample(t *testing.T) {
	t.Parallel()

	result := parsePetstore(t)

	mapped, err := Map(result, MapOptions{})
	require.NoError(t, err)

	// createPet should have a bodyExample.
	createPet := findSpecJSONOp(t, mapped.SpecJSON, "createPet")
	require.NotNil(t, createPet.BodyExample)

	body, ok := createPet.BodyExample.(map[string]any)
	require.True(t, ok)
	assert.Contains(t, body, "name")

	// listPets should not have a bodyExample.
	listPets := findSpecJSONOp(t, mapped.SpecJSON, "listPets")
	assert.Nil(t, listPets.BodyExample)
}

func TestMapInvalidOptions(t *testing.T) {
	t.Parallel()

	result := parsePetstore(t)

	_, err := Map(result, MapOptions{Runtime: "lua"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported runtime")

	_, err = Map(result, MapOptions{ExamplesMode: "all"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported examples mode")
}
