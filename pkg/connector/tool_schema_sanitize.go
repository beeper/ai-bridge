package connector

import "strings"

// Based on OpenClaw's tool schema cleaning to keep providers happy.
var unsupportedSchemaKeywords = map[string]struct{}{
	"patternProperties":    {},
	"additionalProperties": {},
	"$schema":              {},
	"$id":                  {},
	"$ref":                 {},
	"$defs":                {},
	"definitions":          {},
	"examples":             {},
	"minLength":            {},
	"maxLength":            {},
	"minimum":              {},
	"maximum":              {},
	"multipleOf":           {},
	"pattern":              {},
	"format":               {},
	"minItems":             {},
	"maxItems":             {},
	"uniqueItems":          {},
	"minProperties":        {},
	"maxProperties":        {},
}

type schemaDefs map[string]any

func sanitizeToolSchema(schema map[string]any) map[string]any {
	cleaned := cleanSchemaForProvider(schema)
	cleanedMap, ok := cleaned.(map[string]any)
	if !ok || cleanedMap == nil {
		return schema
	}

	// Ensure top-level object type when properties/required are present.
	if _, hasType := cleanedMap["type"]; !hasType {
		if _, hasProps := cleanedMap["properties"]; hasProps || cleanedMap["required"] != nil {
			cleanedMap["type"] = "object"
		}
	}

	return cleanedMap
}

func cleanSchemaForProvider(schema any) any {
	if schema == nil {
		return schema
	}
	if arr, ok := schema.([]any); ok {
		out := make([]any, 0, len(arr))
		for _, item := range arr {
			out = append(out, cleanSchemaForProvider(item))
		}
		return out
	}
	obj, ok := schema.(map[string]any)
	if !ok {
		return schema
	}
	defs := extendSchemaDefs(nil, obj)
	return cleanSchemaWithDefs(obj, defs, nil)
}

func extendSchemaDefs(defs schemaDefs, schema map[string]any) schemaDefs {
	next := defs
	if rawDefs, ok := schema["$defs"].(map[string]any); ok {
		if next == nil {
			next = make(schemaDefs)
		}
		for k, v := range rawDefs {
			next[k] = v
		}
	}
	if rawDefs, ok := schema["definitions"].(map[string]any); ok {
		if next == nil {
			next = make(schemaDefs)
		}
		for k, v := range rawDefs {
			next[k] = v
		}
	}
	return next
}

func decodeJsonPointerSegment(segment string) string {
	return strings.ReplaceAll(strings.ReplaceAll(segment, "~1", "/"), "~0", "~")
}

func tryResolveLocalRef(ref string, defs schemaDefs) any {
	if defs == nil {
		return nil
	}
	switch {
	case strings.HasPrefix(ref, "#/$defs/"):
		name := decodeJsonPointerSegment(strings.TrimPrefix(ref, "#/$defs/"))
		return defs[name]
	case strings.HasPrefix(ref, "#/definitions/"):
		name := decodeJsonPointerSegment(strings.TrimPrefix(ref, "#/definitions/"))
		return defs[name]
	default:
		return nil
	}
}

func tryFlattenLiteralAnyOf(variants []any) map[string]any {
	if len(variants) == 0 {
		return nil
	}
	var commonType string
	values := make([]any, 0, len(variants))
	for _, variant := range variants {
		obj, ok := variant.(map[string]any)
		if !ok {
			return nil
		}
		var literal any
		if v, ok := obj["const"]; ok {
			literal = v
		} else if enumVals, ok := obj["enum"].([]any); ok && len(enumVals) == 1 {
			literal = enumVals[0]
		} else {
			return nil
		}
		typ, ok := obj["type"].(string)
		if !ok || typ == "" {
			return nil
		}
		if commonType == "" {
			commonType = typ
		} else if commonType != typ {
			return nil
		}
		values = append(values, literal)
	}
	if commonType == "" {
		return nil
	}
	return map[string]any{
		"type": commonType,
		"enum": values,
	}
}

func isNullSchema(variant any) bool {
	obj, ok := variant.(map[string]any)
	if !ok {
		return false
	}
	if v, ok := obj["const"]; ok && v == nil {
		return true
	}
	if enumVals, ok := obj["enum"].([]any); ok && len(enumVals) == 1 && enumVals[0] == nil {
		return true
	}
	switch typ := obj["type"].(type) {
	case string:
		return typ == "null"
	case []any:
		if len(typ) == 1 {
			if s, ok := typ[0].(string); ok && s == "null" {
				return true
			}
		}
	case []string:
		return len(typ) == 1 && typ[0] == "null"
	}
	return false
}

func stripNullVariants(variants []any) ([]any, bool) {
	if len(variants) == 0 {
		return variants, false
	}
	nonNull := make([]any, 0, len(variants))
	for _, variant := range variants {
		if !isNullSchema(variant) {
			nonNull = append(nonNull, variant)
		}
	}
	return nonNull, len(nonNull) != len(variants)
}

func copySchemaMeta(src map[string]any, dst map[string]any) {
	for _, key := range []string{"description", "title", "default"} {
		if value, ok := src[key]; ok {
			dst[key] = value
		}
	}
}

func cleanSchemaWithDefs(schema map[string]any, defs schemaDefs, refStack map[string]struct{}) any {
	nextDefs := extendSchemaDefs(defs, schema)

	if ref, ok := schema["$ref"].(string); ok && ref != "" {
		if refStack != nil {
			if _, seen := refStack[ref]; seen {
				return map[string]any{}
			}
		}
		if resolved := tryResolveLocalRef(ref, nextDefs); resolved != nil {
			nextStack := make(map[string]struct{}, len(refStack)+1)
			for k := range refStack {
				nextStack[k] = struct{}{}
			}
			nextStack[ref] = struct{}{}
			cleaned := cleanSchemaForProviderWithDefs(resolved, nextDefs, nextStack)
			if obj, ok := cleaned.(map[string]any); ok {
				result := make(map[string]any, len(obj)+3)
				for k, v := range obj {
					result[k] = v
				}
				copySchemaMeta(schema, result)
				return result
			}
			return cleaned
		}
		result := map[string]any{}
		copySchemaMeta(schema, result)
		return result
	}

	hasAnyOf := false
	hasOneOf := false
	if _, ok := schema["anyOf"].([]any); ok {
		hasAnyOf = true
	}
	if _, ok := schema["oneOf"].([]any); ok {
		hasOneOf = true
	}

	var cleanedAnyOf []any
	var cleanedOneOf []any
	if hasAnyOf {
		raw := schema["anyOf"].([]any)
		cleanedAnyOf = make([]any, 0, len(raw))
		for _, variant := range raw {
			cleanedAnyOf = append(cleanedAnyOf, cleanSchemaForProviderWithDefs(variant, nextDefs, refStack))
		}
	}
	if hasOneOf {
		raw := schema["oneOf"].([]any)
		cleanedOneOf = make([]any, 0, len(raw))
		for _, variant := range raw {
			cleanedOneOf = append(cleanedOneOf, cleanSchemaForProviderWithDefs(variant, nextDefs, refStack))
		}
	}

	if hasAnyOf {
		nonNull, stripped := stripNullVariants(cleanedAnyOf)
		if stripped {
			cleanedAnyOf = nonNull
		}
		if flattened := tryFlattenLiteralAnyOf(nonNull); flattened != nil {
			copySchemaMeta(schema, flattened)
			return flattened
		}
		if stripped && len(nonNull) == 1 {
			if lone, ok := nonNull[0].(map[string]any); ok {
				result := make(map[string]any, len(lone)+3)
				for k, v := range lone {
					result[k] = v
				}
				copySchemaMeta(schema, result)
				return result
			}
			return nonNull[0]
		}
	}

	if hasOneOf {
		nonNull, stripped := stripNullVariants(cleanedOneOf)
		if stripped {
			cleanedOneOf = nonNull
		}
		if flattened := tryFlattenLiteralAnyOf(nonNull); flattened != nil {
			copySchemaMeta(schema, flattened)
			return flattened
		}
		if stripped && len(nonNull) == 1 {
			if lone, ok := nonNull[0].(map[string]any); ok {
				result := make(map[string]any, len(lone)+3)
				for k, v := range lone {
					result[k] = v
				}
				copySchemaMeta(schema, result)
				return result
			}
			return nonNull[0]
		}
	}

	cleaned := make(map[string]any, len(schema))
	for key, value := range schema {
		if _, blocked := unsupportedSchemaKeywords[key]; blocked {
			continue
		}

		if key == "const" {
			cleaned["enum"] = []any{value}
			continue
		}

		if key == "type" && (hasAnyOf || hasOneOf) {
			continue
		}
		if key == "type" {
			if arr, ok := value.([]any); ok {
				types := make([]string, 0, len(arr))
				for _, entry := range arr {
					s, ok := entry.(string)
					if !ok {
						types = nil
						break
					}
					if s != "null" {
						types = append(types, s)
					}
				}
				if types != nil {
					if len(types) == 1 {
						cleaned["type"] = types[0]
					} else if len(types) > 1 {
						cleaned["type"] = types
					}
					continue
				}
			}
		}

		switch key {
		case "properties":
			if props, ok := value.(map[string]any); ok {
				nextProps := make(map[string]any, len(props))
				for k, v := range props {
					nextProps[k] = cleanSchemaForProviderWithDefs(v, nextDefs, refStack)
				}
				cleaned[key] = nextProps
			} else {
				cleaned[key] = value
			}
		case "items":
			switch items := value.(type) {
			case []any:
				nextItems := make([]any, 0, len(items))
				for _, entry := range items {
					nextItems = append(nextItems, cleanSchemaForProviderWithDefs(entry, nextDefs, refStack))
				}
				cleaned[key] = nextItems
			case map[string]any:
				cleaned[key] = cleanSchemaForProviderWithDefs(items, nextDefs, refStack)
			default:
				cleaned[key] = value
			}
		case "anyOf":
			if arr, ok := value.([]any); ok {
				if cleanedAnyOf != nil {
					cleaned[key] = cleanedAnyOf
				} else {
					nextItems := make([]any, 0, len(arr))
					for _, entry := range arr {
						nextItems = append(nextItems, cleanSchemaForProviderWithDefs(entry, nextDefs, refStack))
					}
					cleaned[key] = nextItems
				}
			}
		case "oneOf":
			if arr, ok := value.([]any); ok {
				if cleanedOneOf != nil {
					cleaned[key] = cleanedOneOf
				} else {
					nextItems := make([]any, 0, len(arr))
					for _, entry := range arr {
						nextItems = append(nextItems, cleanSchemaForProviderWithDefs(entry, nextDefs, refStack))
					}
					cleaned[key] = nextItems
				}
			}
		case "allOf":
			if arr, ok := value.([]any); ok {
				nextItems := make([]any, 0, len(arr))
				for _, entry := range arr {
					nextItems = append(nextItems, cleanSchemaForProviderWithDefs(entry, nextDefs, refStack))
				}
				cleaned[key] = nextItems
			}
		default:
			cleaned[key] = value
		}
	}

	return cleaned
}

func cleanSchemaForProviderWithDefs(schema any, defs schemaDefs, refStack map[string]struct{}) any {
	if schema == nil {
		return schema
	}
	if arr, ok := schema.([]any); ok {
		out := make([]any, 0, len(arr))
		for _, item := range arr {
			out = append(out, cleanSchemaForProviderWithDefs(item, defs, refStack))
		}
		return out
	}
	if obj, ok := schema.(map[string]any); ok {
		return cleanSchemaWithDefs(obj, defs, refStack)
	}
	return schema
}
