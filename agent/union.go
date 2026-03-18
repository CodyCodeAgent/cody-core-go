package agent

import (
	"fmt"
	"reflect"
	"regexp"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"

	"github.com/codycode/cody-core-go/output"
)

var outputToolNameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9\-_]`)

// UnionOutput is implemented by OneOf2 and OneOf3 to support union output types.
type UnionOutput interface {
	Value() any
}

// OneOf2 represents a union of two possible output types.
// The model chooses which type to return by calling the corresponding output tool.
type OneOf2[A, B any] struct {
	value any
}

// NewOneOf2A creates a OneOf2 containing a value of type A.
func NewOneOf2A[A, B any](v A) OneOf2[A, B] {
	return OneOf2[A, B]{value: v}
}

// NewOneOf2B creates a OneOf2 containing a value of type B.
func NewOneOf2B[A, B any](v B) OneOf2[A, B] {
	return OneOf2[A, B]{value: v}
}

// Value returns the underlying value. Use type switch to determine the actual type.
func (u OneOf2[A, B]) Value() any { return u.value }

// Match dispatches to the appropriate handler based on the actual type.
// This provides compile-time safety by requiring handlers for both types.
func (u OneOf2[A, B]) Match(onA func(A), onB func(B)) {
	switch v := u.value.(type) { //nolint:gocritic // case order matches type parameter order A, B
	case A:
		onA(v)
	case B:
		onB(v)
	}
}

// OneOf3 represents a union of three possible output types.
type OneOf3[A, B, C any] struct {
	value any
}

// NewOneOf3A creates a OneOf3 containing a value of type A.
func NewOneOf3A[A, B, C any](v A) OneOf3[A, B, C] {
	return OneOf3[A, B, C]{value: v}
}

// NewOneOf3B creates a OneOf3 containing a value of type B.
func NewOneOf3B[A, B, C any](v B) OneOf3[A, B, C] {
	return OneOf3[A, B, C]{value: v}
}

// NewOneOf3C creates a OneOf3 containing a value of type C.
func NewOneOf3C[A, B, C any](v C) OneOf3[A, B, C] {
	return OneOf3[A, B, C]{value: v}
}

// Value returns the underlying value.
func (u OneOf3[A, B, C]) Value() any { return u.value }

// Match dispatches to the appropriate handler based on the actual type.
func (u OneOf3[A, B, C]) Match(onA func(A), onB func(B), onC func(C)) {
	switch v := u.value.(type) { //nolint:gocritic // case order matches type parameter order A, B, C
	case A:
		onA(v)
	case B:
		onB(v)
	case C:
		onC(v)
	}
}

// typeName returns a sanitized name for a type, suitable for use in tool names.
func typeName[T any]() string {
	var zero T
	t := reflect.TypeOf(zero)
	if t == nil {
		return "Unknown"
	}
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	name := t.Name()
	if name == "" {
		name = t.String()
	}
	return outputToolNameSanitizer.ReplaceAllString(name, "")
}

// unionToolInfo holds the mapping from tool name to type index for union types.
type unionToolInfo struct {
	toolName  string
	typeIndex int
}

// buildOneOf2OutputTools generates output tools for a OneOf2 union type.
func buildOneOf2OutputTools[A, B any]() ([]tool.InvokableTool, []unionToolInfo, error) {
	nameA := fmt.Sprintf("%s_%s", output.DefaultOutputToolName, typeName[A]())
	nameB := fmt.Sprintf("%s_%s", output.DefaultOutputToolName, typeName[B]())

	paramsA, err := output.BuildParamsOneOf[A]()
	if err != nil {
		return nil, nil, fmt.Errorf("build schema for type A: %w", err)
	}
	paramsB, err := output.BuildParamsOneOf[B]()
	if err != nil {
		return nil, nil, fmt.Errorf("build schema for type B: %w", err)
	}

	tools := []tool.InvokableTool{
		output.GenerateOutputToolWithName[A](nameA, paramsA),
		output.GenerateOutputToolWithName[B](nameB, paramsB),
	}

	infos := []unionToolInfo{
		{toolName: nameA, typeIndex: 0},
		{toolName: nameB, typeIndex: 1},
	}

	return tools, infos, nil
}

// NewOneOf2 creates an Agent whose output is a OneOf2[A, B] union type.
// The model chooses which variant to return by calling the corresponding output tool.
func NewOneOf2[D, A, B any](chatModel model.BaseChatModel, opts ...Option[D, OneOf2[A, B]]) *Agent[D, OneOf2[A, B]] {
	a := New[D, OneOf2[A, B]](chatModel, opts...)

	tools, infos, err := buildOneOf2OutputTools[A, B]()
	if err != nil {
		a.initErrors = append(a.initErrors, fmt.Errorf("build union output tools: %w", err))
		return a
	}

	a.outputTools = make([]outputToolEntry, len(tools))
	for i, t := range tools {
		a.outputTools[i] = outputToolEntry{tool: t, name: infos[i].toolName}
	}

	// Build name-to-index lookup
	nameToIndex := make(map[string]int, len(infos))
	for _, info := range infos {
		nameToIndex[info.toolName] = info.typeIndex
	}

	a.outputParser = func(toolName string, argsJSON []byte) (OneOf2[A, B], error) {
		idx, ok := nameToIndex[toolName]
		if !ok {
			var zero OneOf2[A, B]
			return zero, fmt.Errorf("unknown union output tool %q", toolName)
		}
		switch idx {
		case 0:
			v, err := output.ParseStructuredOutput[A](argsJSON)
			if err != nil {
				var zero OneOf2[A, B]
				return zero, err
			}
			return NewOneOf2A[A, B](v), nil
		case 1:
			v, err := output.ParseStructuredOutput[B](argsJSON)
			if err != nil {
				var zero OneOf2[A, B]
				return zero, err
			}
			return NewOneOf2B[A, B](v), nil
		default:
			var zero OneOf2[A, B]
			return zero, fmt.Errorf("invalid type index %d for OneOf2", idx)
		}
	}

	return a
}

// NewOneOf3 creates an Agent whose output is a OneOf3[A, B, C] union type.
func NewOneOf3[D, A, B, C any](chatModel model.BaseChatModel, opts ...Option[D, OneOf3[A, B, C]]) *Agent[D, OneOf3[A, B, C]] {
	a := New[D, OneOf3[A, B, C]](chatModel, opts...)

	tools, infos, err := buildOneOf3OutputTools[A, B, C]()
	if err != nil {
		a.initErrors = append(a.initErrors, fmt.Errorf("build union output tools: %w", err))
		return a
	}

	a.outputTools = make([]outputToolEntry, len(tools))
	for i, t := range tools {
		a.outputTools[i] = outputToolEntry{tool: t, name: infos[i].toolName}
	}

	nameToIndex := make(map[string]int, len(infos))
	for _, info := range infos {
		nameToIndex[info.toolName] = info.typeIndex
	}

	a.outputParser = func(toolName string, argsJSON []byte) (OneOf3[A, B, C], error) {
		idx, ok := nameToIndex[toolName]
		if !ok {
			var zero OneOf3[A, B, C]
			return zero, fmt.Errorf("unknown union output tool %q", toolName)
		}
		switch idx {
		case 0:
			v, err := output.ParseStructuredOutput[A](argsJSON)
			if err != nil {
				var zero OneOf3[A, B, C]
				return zero, err
			}
			return NewOneOf3A[A, B, C](v), nil
		case 1:
			v, err := output.ParseStructuredOutput[B](argsJSON)
			if err != nil {
				var zero OneOf3[A, B, C]
				return zero, err
			}
			return NewOneOf3B[A, B, C](v), nil
		case 2:
			v, err := output.ParseStructuredOutput[C](argsJSON)
			if err != nil {
				var zero OneOf3[A, B, C]
				return zero, err
			}
			return NewOneOf3C[A, B, C](v), nil
		default:
			var zero OneOf3[A, B, C]
			return zero, fmt.Errorf("invalid type index %d for OneOf3", idx)
		}
	}

	return a
}

// buildOneOf3OutputTools generates output tools for a OneOf3 union type.
func buildOneOf3OutputTools[A, B, C any]() ([]tool.InvokableTool, []unionToolInfo, error) {
	nameA := fmt.Sprintf("%s_%s", output.DefaultOutputToolName, typeName[A]())
	nameB := fmt.Sprintf("%s_%s", output.DefaultOutputToolName, typeName[B]())
	nameC := fmt.Sprintf("%s_%s", output.DefaultOutputToolName, typeName[C]())

	paramsA, err := output.BuildParamsOneOf[A]()
	if err != nil {
		return nil, nil, fmt.Errorf("build schema for type A: %w", err)
	}
	paramsB, err := output.BuildParamsOneOf[B]()
	if err != nil {
		return nil, nil, fmt.Errorf("build schema for type B: %w", err)
	}
	paramsC, err := output.BuildParamsOneOf[C]()
	if err != nil {
		return nil, nil, fmt.Errorf("build schema for type C: %w", err)
	}

	tools := []tool.InvokableTool{
		output.GenerateOutputToolWithName[A](nameA, paramsA),
		output.GenerateOutputToolWithName[B](nameB, paramsB),
		output.GenerateOutputToolWithName[C](nameC, paramsC),
	}

	infos := []unionToolInfo{
		{toolName: nameA, typeIndex: 0},
		{toolName: nameB, typeIndex: 1},
		{toolName: nameC, typeIndex: 2},
	}

	return tools, infos, nil
}
