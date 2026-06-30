package parser

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../testdata/petstore.yaml")
	require.NoError(t, err)

	result, err := Parse(data)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.NotEmpty(t, result.Operations)

	opIDs := make(map[string]bool, len(result.Operations))
	for _, op := range result.Operations {
		opIDs[op.Spec.OperationID] = true
	}

	require.NotNil(t, result.RawSpec)
	require.NotNil(t, result.RawSpec.Components)
	assert.Contains(t, result.RawSpec.Components.Schemas, "Pet")
	assert.Contains(t, result.RawSpec.Components.Schemas, "CreatePetRequest")
	assert.Contains(t, result.RawSpec.Components.Schemas, "Order")
	assert.Contains(t, result.RawSpec.Components.Schemas, "CreateOrderRequest")

	assert.True(t, opIDs["listPets"], "listPets should be present")
	assert.True(t, opIDs["showPetById"], "showPetById should be present")
	assert.True(t, opIDs["createPet"], "createPet should be present")
	assert.True(t, opIDs["updatePet"], "updatePet should be present")
	assert.True(t, opIDs["deletePet"], "deletePet should be present")
	assert.True(t, opIDs["listOrders"], "listOrders should be present")
	assert.True(t, opIDs["createOrder"], "createOrder should be present")
	assert.True(t, opIDs["addPetTag"], "addPetTag should be present")
	assert.True(t, opIDs["healthCheck"], "healthCheck should be in IR")
}

func TestParseExtensions(t *testing.T) {
	t.Parallel()

	data, err := os.ReadFile("../../testdata/petstore.yaml")
	require.NoError(t, err)

	result, err := Parse(data)
	require.NoError(t, err)

	ext, ok := result.Extensions["healthCheck"]
	require.True(t, ok, "healthCheck should have extensions")
	assert.True(t, ext.Exclude, "healthCheck should be excluded")

	_, ok = result.Extensions["addPetTag"]
	assert.False(t, ok, "addPetTag should not have parsed extensions in code mode")
}

func TestParseInvalidExtensionType(t *testing.T) {
	t.Parallel()

	spec := []byte(`
openapi: "3.0.3"
info:
  title: Invalid ext
  version: "1.0.0"
paths:
  /health:
    get:
      operationId: healthCheck
      x-mcp-exclude:
        bad: true
      responses:
        "200":
          description: OK
`)

	_, err := Parse(spec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "x-mcp-exclude")
}

func TestParseInvalidSpec(t *testing.T) {
	t.Parallel()

	_, err := Parse([]byte("not valid yaml: ["))
	require.Error(t, err)
}

func TestParseMinimalSpec(t *testing.T) {
	t.Parallel()

	spec := []byte(`
openapi: "3.0.3"
info:
  title: Minimal
  version: "1.0.0"
paths:
  /health:
    get:
      operationId: healthCheck
      responses:
        "200":
          description: OK
`)

	result, err := Parse(spec)
	require.NoError(t, err)
	assert.Len(t, result.Operations, 1)
	assert.Equal(t, "healthCheck", result.Operations[0].Spec.OperationID)
	assert.Empty(t, result.Extensions)
}

func TestParseOpenAPI31Minimal(t *testing.T) {
	t.Parallel()

	spec := []byte(`
openapi: "3.1.0"
info:
  title: Minimal31
  version: "1.0.0"
paths:
  /health:
    get:
      operationId: healthCheck
      responses:
        "200":
          description: OK
`)

	result, err := Parse(spec)
	require.NoError(t, err)
	assert.Len(t, result.Operations, 1)
	assert.Equal(t, "healthCheck", result.Operations[0].Spec.OperationID)
}

func TestParseOpenAPI31OneOfNull(t *testing.T) {
	t.Parallel()

	spec := []byte(`
openapi: "3.1.0"
info:
  title: OneOf31
  version: "1.0.0"
paths:
  /thing:
    get:
      operationId: getThing
      responses:
        "200":
          description: OK
          content:
            application/json:
              schema:
                type: object
                properties:
                  maybe:
                    oneOf:
                      - type: string
                      - type: "null"
`)

	result, err := Parse(spec)
	require.NoError(t, err)
	assert.Len(t, result.Operations, 1)
	assert.Equal(t, "getThing", result.Operations[0].Spec.OperationID)
}

func TestParseOpenAPI31NullableTypeArrayUnsupported(t *testing.T) {
	t.Parallel()

	spec := []byte(`
openapi: "3.1.0"
info:
  title: UnsupportedNullable31
  version: "1.0.0"
paths:
  /thing:
    get:
      operationId: getThing
      responses:
        "200":
          description: OK
          content:
            application/json:
              schema:
                type: object
                properties:
                  maybe:
                    type: ["string", "null"]
`)

	_, err := Parse(spec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "openapi 3.1 schema compatibility")
	assert.Contains(t, err.Error(), "`type` array including \"null\"")
	assert.Contains(t, err.Error(), "oneOf")
	assert.Contains(t, err.Error(), "$.paths./thing.get")
}
