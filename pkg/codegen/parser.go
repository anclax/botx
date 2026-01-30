package codegen

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Parser struct {
}

func NewParser() *Parser {
	return &Parser{}
}

func (p *Parser) Parse(input string) (*Doc, error) {
	var raw map[string]any
	if err := yaml.Unmarshal([]byte(input), &raw); err != nil {
		return nil, err
	}
	if err := normalizeDoc(raw); err != nil {
		return nil, err
	}
	data, err := yaml.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var doc Doc
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return &doc, nil
}

func (p *Parser) ParseFile(path string) (*Doc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return p.Parse(string(data))
}

func normalizeDoc(doc map[string]any) error {
	normalizeI18n(doc)
	normalizeComponents(doc)
	normalizePages(doc)
	normalizeAPI(doc)
	return nil
}

func normalizeI18n(doc map[string]any) {
	raw, ok := doc["i18n"].(map[string]any)
	if !ok {
		return
	}
	result := map[string]any{}
	if value, ok := raw["default"]; ok {
		if text, ok := value.(string); ok {
			result["default"] = text
		}
	}
	entries := map[string]map[string]string{}
	for key, value := range raw {
		if key == "default" {
			continue
		}
		flattenI18nEntries(entries, key, value)
	}
	if len(entries) != 0 {
		result["entries"] = entries
	}
	if len(result) == 0 {
		return
	}
	doc["i18n"] = result
}

func flattenI18nEntries(entries map[string]map[string]string, prefix string, value any) {
	child, ok := value.(map[string]any)
	if !ok {
		return
	}
	allStrings := len(child) != 0
	for _, v := range child {
		if _, ok := v.(string); !ok {
			allStrings = false
			break
		}
	}
	if allStrings {
		langMap := make(map[string]string, len(child))
		for lang, text := range child {
			if value, ok := text.(string); ok {
				langMap[lang] = value
			}
		}
		if len(langMap) != 0 {
			entries[prefix] = langMap
		}
		return
	}
	for key, next := range child {
		flattenI18nEntries(entries, prefix+"."+key, next)
	}
}

func normalizeComponents(doc map[string]any) {
	components, ok := doc["components"].(map[string]any)
	if !ok {
		return
	}
	schemas, ok := components["schemas"].(map[string]any)
	if !ok {
		return
	}
	for name, value := range schemas {
		if schema, ok := value.(map[string]any); ok {
			schemas[name] = normalizeSchemaValue(schema)
		}
	}
}

func normalizePages(doc map[string]any) {
	pages, ok := doc["pages"].(map[string]any)
	if !ok {
		return
	}
	for _, value := range pages {
		page, ok := value.(map[string]any)
		if !ok {
			continue
		}
		if params, ok := page["parameters"]; ok {
			page["parameters"] = normalizeParameters(params)
		}
		if state, ok := page["state"]; ok {
			page["state"] = normalizeSchemaValue(state)
		}
		if form, ok := page["form"].(map[string]any); ok {
			normalizeForm(form)
		}
	}
}

func normalizeAPI(doc map[string]any) {
	api, ok := doc["api"].(map[string]any)
	if !ok {
		return
	}
	for _, value := range api {
		item, ok := value.(map[string]any)
		if !ok {
			continue
		}
		args, ok := item["args"].([]any)
		if !ok {
			continue
		}
		for _, arg := range args {
			argMap, ok := arg.(map[string]any)
			if !ok {
				continue
			}
			if schema, ok := argMap["schema"]; ok {
				argMap["schema"] = normalizeSchemaRef(schema)
			}
		}
	}
}

func normalizeParameters(value any) any {
	switch v := value.(type) {
	case []any:
		for i := range v {
			v[i] = normalizeParameter(v[i])
		}
		return v
	case map[string]any:
		if hasParameterGroups(v) {
			return normalizeParameterGroups(v)
		}
		for key, item := range v {
			v[key] = normalizeParameter(item)
		}
		return v
	default:
		return value
	}
}

func normalizeParameter(value any) any {
	param, ok := value.(map[string]any)
	if !ok {
		return value
	}
	if schema, ok := param["schema"]; ok {
		param["schema"] = normalizeSchemaRef(schema)
	}
	return param
}

func hasParameterGroups(params map[string]any) bool {
	for _, value := range params {
		if _, ok := value.([]any); ok {
			return true
		}
	}
	return false
}

func normalizeParameterGroups(params map[string]any) []any {
	var result []any
	used := make(map[string]struct{})
	for _, key := range []string{"path", "query", "header", "cookie"} {
		value, ok := params[key].([]any)
		if !ok {
			continue
		}
		used[key] = struct{}{}
		for _, item := range value {
			result = append(result, normalizeParameter(item))
		}
	}
	for key, value := range params {
		if _, ok := used[key]; ok {
			continue
		}
		list, ok := value.([]any)
		if !ok {
			continue
		}
		for _, item := range list {
			result = append(result, normalizeParameter(item))
		}
	}
	return result
}

func normalizeForm(form map[string]any) {
	fields, ok := form["fields"].(map[string]any)
	if !ok {
		return
	}
	for _, value := range fields {
		field, ok := value.(map[string]any)
		if !ok {
			continue
		}
		normalizeFormField(field)
	}
}

func normalizeFormField(field map[string]any) {
	input, hasInput := field["input"]
	fieldType, hasFieldType := field["type"].(string)
	fieldTip, hasFieldTip := field["tip"]
	if hasInput {
		switch inputValue := input.(type) {
		case string:
			inputMap := map[string]any{}
			if hasFieldType {
				inputMap["type"] = fieldType
				delete(field, "type")
			} else {
				inputMap["type"] = inputValue
			}
			if hasFieldTip {
				inputMap["tip"] = fieldTip
				delete(field, "tip")
			}
			field["input"] = inputMap
		case map[string]any:
			if hasFieldType {
				if _, ok := inputValue["type"]; !ok {
					inputValue["type"] = fieldType
				}
				delete(field, "type")
			}
			if hasFieldTip {
				if _, ok := inputValue["tip"]; !ok {
					inputValue["tip"] = fieldTip
				}
				delete(field, "tip")
			}
			if schema, ok := inputValue["schema"]; ok {
				inputValue["schema"] = normalizeSchemaValue(schema)
			}
		}
		return
	}
	if !hasFieldType {
		return
	}
	inputMap := map[string]any{"type": fieldType}
	delete(field, "type")
	if hasFieldTip {
		inputMap["tip"] = fieldTip
		delete(field, "tip")
	}
	field["input"] = inputMap
}

func normalizeSchemaValue(value any) any {
	switch schema := value.(type) {
	case map[string]any:
		if inner, ok := schema["schema"]; ok {
			return normalizeSchemaValue(inner)
		}
		normalizeSchemaMap(schema)
		return schema
	case string:
		return map[string]any{
			"allOf": []any{normalizeSchemaRef(schema)},
		}
	default:
		return value
	}
}

func normalizeSchemaRef(value any) any {
	switch schema := value.(type) {
	case map[string]any:
		if inner, ok := schema["schema"]; ok {
			return normalizeSchemaRef(inner)
		}
		if refValue, ok := schema["ref"]; ok {
			return map[string]any{"ref": refValue}
		}
		if refValue, ok := schema["$ref"]; ok {
			return map[string]any{"ref": refValue}
		}
		if valueNode, ok := schema["value"]; ok {
			switch inner := valueNode.(type) {
			case map[string]any:
				normalizeSchemaMap(inner)
				return map[string]any{"value": inner}
			case string:
				return map[string]any{"ref": inner}
			default:
				return map[string]any{"value": valueNode}
			}
		}
		normalizeSchemaMap(schema)
		return map[string]any{"value": schema}
	case string:
		return map[string]any{"ref": schema}
	default:
		return value
	}
}

func normalizeSchemaMap(schema map[string]any) {
	switch value := schema["type"].(type) {
	case string:
		schema["type"] = []string{value}
	case []any:
		var types []string
		for _, item := range value {
			if text, ok := item.(string); ok {
				types = append(types, text)
			}
		}
		if len(types) != 0 {
			schema["type"] = types
		}
	}
	if items, ok := schema["items"]; ok {
		schema["items"] = normalizeSchemaRef(items)
	}
	if props, ok := schema["properties"].(map[string]any); ok {
		for key, value := range props {
			props[key] = normalizeSchemaRef(value)
		}
	}
	if addProps, ok := schema["additionalProperties"]; ok {
		schema["additionalProperties"] = map[string]any{"schema": normalizeSchemaRef(addProps)}
	}
	normalizeSchemaRefList(schema, "oneOf")
	normalizeSchemaRefList(schema, "anyOf")
	normalizeSchemaRefList(schema, "allOf")
	if notValue, ok := schema["not"]; ok {
		schema["not"] = normalizeSchemaRef(notValue)
	}
}

func normalizeSchemaRefList(schema map[string]any, key string) {
	value, ok := schema[key].([]any)
	if !ok {
		return
	}
	for i := range value {
		value[i] = normalizeSchemaRef(value[i])
	}
}
