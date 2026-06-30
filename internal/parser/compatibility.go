package parser

import (
	"fmt"
	"strings"

	"github.com/go-faster/yaml"
)

// validateOpenAPI31Compatibility checks for spec patterns that are valid in
// OpenAPI 3.1 but currently unsupported by ogen.
func validateOpenAPI31Compatibility(specBytes []byte) error {
	var root yaml.Node
	// Keep parser errors owned by ogen.Parse for consistency.
	_ = yaml.Unmarshal(specBytes, &root)

	doc := &root
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		doc = root.Content[0]
	}

	if doc == nil || doc.Kind != yaml.MappingNode {
		return nil
	}

	versionNode := mappingValue(doc, "openapi")
	if versionNode == nil {
		return nil
	}

	version := strings.TrimSpace(versionNode.Value)
	if !strings.HasPrefix(version, "3.1") {
		return nil
	}

	findings := make([]string, 0)
	findUnsupportedNullableTypeArrays(doc, "$", &findings)
	if len(findings) == 0 {
		return nil
	}

	location := findings[0]
	if len(findings) > 1 {
		location = fmt.Sprintf("%s (and %d more)", findings[0], len(findings)-1)
	}

	return fmt.Errorf(
		"openapi 3.1 schema compatibility: unsupported `type` array including \"null\" at %s; rewrite as `oneOf` with `{type: \"null\"}`",
		location,
	)
}

func findUnsupportedNullableTypeArrays(
	node *yaml.Node, path string, findings *[]string,
) {
	if node == nil {
		return
	}

	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			findUnsupportedNullableTypeArrays(child, path, findings)
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key := node.Content[i]
			val := node.Content[i+1]

			childPath := path + "." + key.Value
			if key.Value == "type" &&
				val.Kind == yaml.SequenceNode &&
				sequenceHasNull(val) {
				*findings = append(*findings, childPath)
			}

			findUnsupportedNullableTypeArrays(
				val, childPath, findings,
			)
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			findUnsupportedNullableTypeArrays(
				child,
				fmt.Sprintf("%s[%d]", path, i),
				findings,
			)
		}
	case yaml.ScalarNode, yaml.AliasNode:
		return
	default:
		return
	}
}

func sequenceHasNull(node *yaml.Node) bool {
	for _, item := range node.Content {
		if strings.TrimSpace(item.Value) == "null" {
			return true
		}
	}

	return false
}

func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}

	return nil
}
