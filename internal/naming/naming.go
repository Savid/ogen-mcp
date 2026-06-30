// Package naming provides string conversion utilities for mapping OpenAPI
// operation identifiers to Go and MCP naming conventions.
package naming

import (
	"strconv"
	"strings"
	"unicode"
)

// commonAcronyms are uppercase abbreviations that should be preserved as
// single units during case conversion.
var commonAcronyms = map[string]bool{
	"API":   true,
	"HTTP":  true,
	"HTTPS": true,
	"ID":    true,
	"JSON":  true,
	"REST":  true,
	"URL":   true,
	"XML":   true,
	"YAML":  true,
	"UUID":  true,
	"HTML":  true,
	"CSS":   true,
	"SQL":   true,
	"SSH":   true,
	"TCP":   true,
	"UDP":   true,
	"IP":    true,
	"DNS":   true,
	"TLS":   true,
	"JWT":   true,
	"OAuth": true,
	"URI":   true,
	"EOF":   true,
	"GID":   true,
	"UID":   true,
	"ACL":   true,
	"TTL":   true,
	"RPC":   true,
	"GRPC":  true,
}

// ToSnakeCase converts a camelCase or PascalCase string to snake_case.
// It handles acronyms (e.g., "HTTPMethod" → "http_method") and consecutive
// uppercase runs.
func ToSnakeCase(s string) string {
	if s == "" {
		return ""
	}

	// Already snake_case or lowercase.
	if s == strings.ToLower(s) {
		return s
	}

	runes := []rune(s)
	var buf strings.Builder
	buf.Grow(len(s) + 4) //nolint:mnd // room for underscores

	for i, r := range runes {
		if i == 0 {
			buf.WriteRune(unicode.ToLower(r))
			continue
		}

		prev := runes[i-1]

		if unicode.IsUpper(r) {
			// Insert underscore when:
			// - previous char is lowercase or digit (e.g., "listP" → "list_p")
			// - previous char is upper AND next is lower (e.g., "HTTPMethod" → "HTTP_Method")
			prevLowerOrDigit := unicode.IsLower(prev) || unicode.IsDigit(prev)
			nextLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])

			if prevLowerOrDigit || (nextLower && unicode.IsUpper(prev)) {
				buf.WriteByte('_')
			}

			buf.WriteRune(unicode.ToLower(r))
		} else {
			buf.WriteRune(r)
		}
	}

	return buf.String()
}

// ToPascalCase converts a snake_case string to PascalCase. It recognizes
// common acronyms and uppercases them fully (e.g., "pet_id" → "PetID").
func ToPascalCase(s string) string {
	if s == "" {
		return ""
	}

	parts := strings.Split(s, "_")
	var buf strings.Builder
	buf.Grow(len(s))

	for _, part := range parts {
		if part == "" {
			continue
		}

		upper := strings.ToUpper(part)
		if commonAcronyms[upper] {
			buf.WriteString(upper)
			continue
		}

		runes := []rune(part)
		buf.WriteRune(unicode.ToUpper(runes[0]))
		buf.WriteString(string(runes[1:]))
	}

	return buf.String()
}

// OperationDomain derives a domain/action name from an OpenAPI operation.
// It prefers the operationID (converted to snake_case). When operationID is
// empty, it falls back to a combination of HTTP method and path segments.
func OperationDomain(operationID, httpMethod, path string) string {
	if operationID != "" {
		return ToSnakeCase(operationID)
	}

	// Fallback: method + path segments.
	// e.g., GET /pets/{petId}/toys → get_pets_pet_id_toys
	segments := strings.Split(strings.Trim(path, "/"), "/")
	parts := make([]string, 0, len(segments)+1)
	parts = append(parts, strings.ToLower(httpMethod))

	for _, seg := range segments {
		if seg == "" {
			continue
		}

		// Strip path parameter braces: {petId} → petId
		seg = strings.TrimPrefix(seg, "{")
		seg = strings.TrimSuffix(seg, "}")

		parts = append(parts, ToSnakeCase(seg))
	}

	return strings.Join(parts, "_")
}

// DeduplicateNames appends numeric suffixes (_2, _3, …) to resolve
// collisions in a list of names. The first occurrence keeps its original
// name; subsequent duplicates get incrementing suffixes.
func DeduplicateNames(names []string) []string {
	if len(names) == 0 {
		return names
	}

	seen := make(map[string]int, len(names))
	result := make([]string, len(names))

	for i, name := range names {
		count := seen[name]
		seen[name] = count + 1

		if count == 0 {
			result[i] = name
		} else {
			candidate := name + "_" + itoa(count+1)
			// Ensure the suffixed name is also unique.
			for seen[candidate] > 0 {
				count++
				candidate = name + "_" + itoa(count+1)
			}

			seen[candidate] = 1
			result[i] = candidate
		}
	}

	return result
}

// itoa converts a small integer to its string representation.
func itoa(n int) string {
	return strconv.Itoa(n)
}
