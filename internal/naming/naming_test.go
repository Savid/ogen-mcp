package naming

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testListPets    = "list_pets"
	testShowPetByID = "show_pet_by_id"
	testMethodGET   = "GET"
	testFoo         = "foo"
)

func TestToSnakeCase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"listPets", testListPets},
		{"showPetById", testShowPetByID},
		{"createPet", "create_pet"},
		{"HTTPMethod", "http_method"},
		{"getHTTPSURL", "get_httpsurl"},
		{"simpleXML", "simple_xml"},
		{"already_snake", "already_snake"},
		{"lowercase", "lowercase"},
		{"A", "a"},
		{"AB", "ab"},
		{"ABC", "abc"},
		{"GetAPIKey", "get_api_key"},
		{"OAuth2Token", "o_auth2_token"},
		{"listV2Pets", "list_v2_pets"},
		{"JSONResponse", "json_response"},
		{"getIPAddress", "get_ip_address"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ToSnakeCase(tt.input))
		})
	}
}

func TestToPascalCase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{testListPets, "ListPets"},
		{testShowPetByID, "ShowPetByID"},
		{"create_pet", "CreatePet"},
		{"http_method", "HTTPMethod"},
		{"get_api_key", "GetAPIKey"},
		{"simple", "Simple"},
		{"a", "A"},
		{"pet_id", "PetID"},
		{"json_response", "JSONResponse"},
		{"get_url", "GetURL"},
		{"__leading_underscores", "LeadingUnderscores"},
		{"trailing__double", "TrailingDouble"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, ToPascalCase(tt.input))
		})
	}
}

func TestOperationDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		operationID string
		httpMethod  string
		path        string
		want        string
	}{
		{
			name:        "with operationID",
			operationID: "listPets",
			httpMethod:  testMethodGET,
			path:        "/pets",
			want:        testListPets,
		},
		{
			name:        "with PascalCase operationID",
			operationID: "ShowPetById",
			httpMethod:  testMethodGET,
			path:        "/pets/{petId}",
			want:        testShowPetByID,
		},
		{
			name:        "fallback to method+path",
			operationID: "",
			httpMethod:  testMethodGET,
			path:        "/pets",
			want:        "get_pets",
		},
		{
			name:        "fallback with path params",
			operationID: "",
			httpMethod:  testMethodGET,
			path:        "/pets/{petId}/toys",
			want:        "get_pets_pet_id_toys",
		},
		{
			name:        "fallback with nested path",
			operationID: "",
			httpMethod:  "POST",
			path:        "/users/{userId}/orders",
			want:        "post_users_user_id_orders",
		},
		{
			name:        "fallback trailing slash",
			operationID: "",
			httpMethod:  "DELETE",
			path:        "/pets/{petId}/",
			want:        "delete_pets_pet_id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := OperationDomain(tt.operationID, tt.httpMethod, tt.path)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDeduplicateNames(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input []string
		want  []string
	}{
		{
			name:  "nil",
			input: nil,
			want:  nil,
		},
		{
			name:  "empty",
			input: []string{},
			want:  []string{},
		},
		{
			name:  "no duplicates",
			input: []string{"a", "b", "c"},
			want:  []string{"a", "b", "c"},
		},
		{
			name:  "single duplicate",
			input: []string{testListPets, testListPets},
			want:  []string{testListPets, "list_pets_2"},
		},
		{
			name:  "triple duplicate",
			input: []string{testFoo, testFoo, testFoo},
			want:  []string{testFoo, "foo_2", "foo_3"},
		},
		{
			name:  "mixed",
			input: []string{"a", "b", "a", "c", "b", "a"},
			want:  []string{"a", "b", "a_2", "c", "b_2", "a_3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := DeduplicateNames(tt.input)
			require.Equal(t, tt.want, got)
		})
	}
}
