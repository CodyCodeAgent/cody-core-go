package output

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type BasicStruct struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
}

type StructWithDesc struct {
	Name   string `json:"name" description:"用户名"`
	Status string `json:"status" enum:"active,inactive"`
}

type StructWithOptional struct {
	Name  string `json:"name"`
	Extra string `json:"extra,omitempty"`
}

type NestedStruct struct {
	Addr Address `json:"addr"`
}

type Address struct {
	City   string `json:"city"`
	Street string `json:"street"`
}

type SliceStruct struct {
	Tags []string `json:"tags"`
}

type NestedSliceStruct struct {
	Items []BasicStruct `json:"items"`
}

type PointerStruct struct {
	Name *string `json:"name"`
}

type EmptyStruct struct{}

type EmbeddedBase struct {
	ID string `json:"id"`
}

type EmbeddedStruct struct {
	EmbeddedBase
	Extra string `json:"extra"`
}

func TestBuildParamsOneOf_BasicStruct(t *testing.T) {
	p, err := BuildParamsOneOf[BasicStruct]()
	require.NoError(t, err)
	require.NotNil(t, p)

	js, err := p.ToJSONSchema()
	require.NoError(t, err)
	require.NotNil(t, js)
}

func TestBuildParamsOneOf_StructWithDescription(t *testing.T) {
	p, err := BuildParamsOneOf[StructWithDesc]()
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestBuildParamsOneOf_StructWithOptional(t *testing.T) {
	p, err := BuildParamsOneOf[StructWithOptional]()
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestBuildParamsOneOf_NestedStruct(t *testing.T) {
	p, err := BuildParamsOneOf[NestedStruct]()
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestBuildParamsOneOf_SliceStruct(t *testing.T) {
	p, err := BuildParamsOneOf[SliceStruct]()
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestBuildParamsOneOf_NestedSliceStruct(t *testing.T) {
	p, err := BuildParamsOneOf[NestedSliceStruct]()
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestBuildParamsOneOf_PointerStruct(t *testing.T) {
	p, err := BuildParamsOneOf[PointerStruct]()
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestBuildParamsOneOf_EmptyStruct(t *testing.T) {
	p, err := BuildParamsOneOf[EmptyStruct]()
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestBuildParamsOneOf_EmbeddedStruct(t *testing.T) {
	p, err := BuildParamsOneOf[EmbeddedStruct]()
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestBuildParamsOneOf_Int(t *testing.T) {
	p, err := BuildParamsOneOf[int]()
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestBuildParamsOneOf_Float64(t *testing.T) {
	p, err := BuildParamsOneOf[float64]()
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestBuildParamsOneOf_Bool(t *testing.T) {
	p, err := BuildParamsOneOf[bool]()
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestBuildParamsOneOf_StringSlice(t *testing.T) {
	p, err := BuildParamsOneOf[[]string]()
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestBuildParamsOneOf_IntSlice(t *testing.T) {
	p, err := BuildParamsOneOf[[]int]()
	require.NoError(t, err)
	require.NotNil(t, p)
}

func TestIsPrimitive(t *testing.T) {
	assert.True(t, IsPrimitive[int]())
	assert.True(t, IsPrimitive[float64]())
	assert.True(t, IsPrimitive[bool]())
	assert.True(t, IsPrimitive[[]string]())
	assert.True(t, IsPrimitive[[]int]())
	assert.False(t, IsPrimitive[BasicStruct]())
	assert.False(t, IsPrimitive[EmptyStruct]())
}

func TestIsString(t *testing.T) {
	assert.True(t, IsString[string]())
	assert.False(t, IsString[int]())
	assert.False(t, IsString[BasicStruct]())
}

func TestParseStructuredOutput_Struct(t *testing.T) {
	data := []byte(`{"name":"test","score":42}`)
	result, err := ParseStructuredOutput[BasicStruct](data)
	require.NoError(t, err)
	assert.Equal(t, "test", result.Name)
	assert.Equal(t, 42, result.Score)
}

func TestParseStructuredOutput_Int(t *testing.T) {
	data := []byte(`{"result":42}`)
	result, err := ParseStructuredOutput[int](data)
	require.NoError(t, err)
	assert.Equal(t, 42, result)
}

func TestParseStructuredOutput_Bool(t *testing.T) {
	data := []byte(`{"result":true}`)
	result, err := ParseStructuredOutput[bool](data)
	require.NoError(t, err)
	assert.True(t, result)
}

func TestParseStructuredOutput_StringSlice(t *testing.T) {
	data := []byte(`{"result":["a","b","c"]}`)
	result, err := ParseStructuredOutput[[]string](data)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, result)
}

func TestParseStructuredOutput_InvalidJSON(t *testing.T) {
	data := []byte(`{invalid}`)
	_, err := ParseStructuredOutput[BasicStruct](data)
	assert.Error(t, err)
}

func TestParseStructuredOutput_MarkdownFence(t *testing.T) {
	data := []byte("```json\n{\"name\":\"test\",\"score\":5}\n```")
	result, err := ParseStructuredOutput[BasicStruct](data)
	require.NoError(t, err)
	assert.Equal(t, "test", result.Name)
	assert.Equal(t, 5, result.Score)
}

func TestParseStructuredOutput_ExtraFields(t *testing.T) {
	data := []byte(`{"name":"test","score":5,"extra":"ignore"}`)
	result, err := ParseStructuredOutput[BasicStruct](data)
	require.NoError(t, err)
	assert.Equal(t, "test", result.Name)
	assert.Equal(t, 5, result.Score)
}

func TestStripMarkdownFences(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"no fence", `{"key":"val"}`, `{"key":"val"}`},
		{"json fence", "```json\n{\"key\":\"val\"}\n```", `{"key":"val"}`},
		{"plain fence", "```\n{\"key\":\"val\"}\n```", `{"key":"val"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripMarkdownFences([]byte(tt.input))
			// Parse both to compare structurally
			var expected, got json.RawMessage
			require.NoError(t, json.Unmarshal([]byte(tt.expected), &expected))
			require.NoError(t, json.Unmarshal(result, &got))
			assert.Equal(t, string(expected), string(got))
		})
	}
}
