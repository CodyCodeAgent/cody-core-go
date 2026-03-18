package output

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/cloudwego/eino/schema"
)

// IsPrimitive returns true if T is a non-struct primitive type (int, bool, float64, etc.)
// or a slice type, which requires wrapping in {"result": ...} for the output tool.
func IsPrimitive[T any]() bool {
	var zero T
	t := reflect.TypeOf(zero)
	if t == nil {
		return false
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Kind() != reflect.Struct
}

// IsString returns true if T is the string type.
func IsString[T any]() bool {
	var zero T
	t := reflect.TypeOf(zero)
	if t == nil {
		return false
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Kind() == reflect.String
}

// BuildParamsOneOf generates a ParamsOneOf for the given type T.
// For struct types, it generates the schema directly from the struct fields.
// For primitive/slice types, it wraps the type in a {"result": <schema>} object.
func BuildParamsOneOf[T any]() (*schema.ParamsOneOf, error) {
	var zero T
	t := reflect.TypeOf(zero)
	if t == nil {
		return nil, fmt.Errorf("cannot generate schema for nil type")
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() == reflect.Struct {
		return buildStructParamsOneOf(t)
	}
	return buildPrimitiveParamsOneOf(t)
}

// buildStructParamsOneOf generates ParamsOneOf from a struct type.
func buildStructParamsOneOf(t reflect.Type) (*schema.ParamsOneOf, error) {
	params := make(map[string]*schema.ParameterInfo)
	required := collectStructFields(t, params)
	_ = required
	return schema.NewParamsOneOfByParams(params), nil
}

// collectStructFields recursively collects fields from a struct type.
func collectStructFields(t reflect.Type, params map[string]*schema.ParameterInfo) []string {
	var required []string
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}

		// Handle embedded structs
		if field.Anonymous {
			ft := field.Type
			for ft.Kind() == reflect.Ptr {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				embeddedRequired := collectStructFields(ft, params)
				required = append(required, embeddedRequired...)
				continue
			}
		}

		jsonTag := field.Tag.Get("json")
		if jsonTag == "-" {
			continue
		}
		name := field.Name
		omitempty := false
		if jsonTag != "" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" {
				name = parts[0]
			}
			for _, p := range parts[1:] {
				if p == "omitempty" {
					omitempty = true
				}
			}
		}

		paramInfo := fieldToParameterInfo(field)

		// Determine if required
		reqTag := field.Tag.Get("required")
		jsTag := field.Tag.Get("jsonschema")
		isRequired := !omitempty
		if reqTag == "true" {
			isRequired = true
		} else if reqTag == "false" {
			isRequired = false
		}
		if strings.Contains(jsTag, "required") {
			isRequired = true
		}
		paramInfo.Required = isRequired
		if isRequired {
			required = append(required, name)
		}

		params[name] = paramInfo
	}
	return required
}

// fieldToParameterInfo converts a struct field to a ParameterInfo.
func fieldToParameterInfo(field reflect.StructField) *schema.ParameterInfo {
	ft := field.Type
	for ft.Kind() == reflect.Ptr {
		ft = ft.Elem()
	}

	info := typeToParameterInfo(ft)

	// Apply struct tags
	if desc := field.Tag.Get("description"); desc != "" {
		info.Desc = desc
	}
	// Also check jsonschema tag for description
	if jsTag := field.Tag.Get("jsonschema"); jsTag != "" {
		for _, part := range strings.Split(jsTag, ",") {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "description=") {
				info.Desc = strings.TrimPrefix(part, "description=")
			}
			if strings.HasPrefix(part, "enum=") {
				info.Enum = append(info.Enum, strings.TrimPrefix(part, "enum="))
			}
		}
	}
	if enumTag := field.Tag.Get("enum"); enumTag != "" {
		info.Enum = strings.Split(enumTag, ",")
	}

	return info
}

// typeToParameterInfo converts a reflect.Type to a ParameterInfo.
func typeToParameterInfo(t reflect.Type) *schema.ParameterInfo {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.String:
		return &schema.ParameterInfo{Type: schema.String}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return &schema.ParameterInfo{Type: schema.Integer}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &schema.ParameterInfo{Type: schema.Integer}
	case reflect.Float32, reflect.Float64:
		return &schema.ParameterInfo{Type: schema.Number}
	case reflect.Bool:
		return &schema.ParameterInfo{Type: schema.Boolean}
	case reflect.Slice, reflect.Array:
		elemInfo := typeToParameterInfo(t.Elem())
		return &schema.ParameterInfo{
			Type:     schema.Array,
			ElemInfo: elemInfo,
		}
	case reflect.Struct:
		subParams := make(map[string]*schema.ParameterInfo)
		collectStructFields(t, subParams)
		return &schema.ParameterInfo{
			Type:      schema.Object,
			SubParams: subParams,
		}
	case reflect.Map:
		return &schema.ParameterInfo{Type: schema.Object}
	default:
		return &schema.ParameterInfo{Type: schema.String}
	}
}

// buildPrimitiveParamsOneOf wraps a primitive type in {"result": <schema>}.
func buildPrimitiveParamsOneOf(t reflect.Type) (*schema.ParamsOneOf, error) {
	info := typeToParameterInfo(t)
	info.Required = true
	info.Desc = "The result value"
	params := map[string]*schema.ParameterInfo{
		"result": info,
	}
	return schema.NewParamsOneOfByParams(params), nil
}

// ParseStructuredOutput parses JSON data into type T.
// For struct types, it directly unmarshals the JSON.
// For primitive types, it extracts the "result" field first.
func ParseStructuredOutput[T any](data []byte) (T, error) {
	var zero T

	// Strip markdown fences
	data = stripMarkdownFences(data)

	if IsPrimitive[T]() {
		var wrapper struct {
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(data, &wrapper); err != nil {
			return zero, fmt.Errorf("parse output wrapper: %w", err)
		}
		var result T
		if err := json.Unmarshal(wrapper.Result, &result); err != nil {
			return zero, fmt.Errorf("parse output result: %w", err)
		}
		return result, nil
	}

	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		return zero, fmt.Errorf("parse output: %w", err)
	}
	return result, nil
}

// stripMarkdownFences removes markdown code block markers from JSON data.
func stripMarkdownFences(data []byte) []byte {
	s := strings.TrimSpace(string(data))
	if strings.HasPrefix(s, "```") {
		// Remove opening fence (```json or ```)
		idx := strings.Index(s, "\n")
		if idx >= 0 {
			s = s[idx+1:]
		}
		// Remove closing fence
		if strings.HasSuffix(s, "```") {
			s = s[:len(s)-3]
		}
		s = strings.TrimSpace(s)
	}
	return []byte(s)
}
