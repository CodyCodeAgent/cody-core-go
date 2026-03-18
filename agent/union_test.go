package agent

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"

	"github.com/codycode/cody-core-go/output"
	"github.com/codycode/cody-core-go/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type VulnFound struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
}

type CodeSafe struct {
	Summary string `json:"summary"`
}

type ExtraType struct {
	Info string `json:"info"`
}

func TestOneOf2_Match(t *testing.T) {
	u := NewOneOf2A[VulnFound, CodeSafe](VulnFound{Type: "SQLi", Severity: "high"})

	aCalled := false
	bCalled := false
	u.Match(
		func(v VulnFound) {
			aCalled = true
			assert.Equal(t, "SQLi", v.Type)
		},
		func(s CodeSafe) {
			bCalled = true
		},
	)
	assert.True(t, aCalled)
	assert.False(t, bCalled)
}

func TestOneOf2_MatchB(t *testing.T) {
	u := NewOneOf2B[VulnFound, CodeSafe](CodeSafe{Summary: "all clear"})

	bCalled := false
	u.Match(
		func(v VulnFound) { t.Error("should not be called") },
		func(s CodeSafe) {
			bCalled = true
			assert.Equal(t, "all clear", s.Summary)
		},
	)
	assert.True(t, bCalled)
}

func TestOneOf2_Value(t *testing.T) {
	u := NewOneOf2A[VulnFound, CodeSafe](VulnFound{Type: "XSS"})
	v, ok := u.Value().(VulnFound)
	assert.True(t, ok)
	assert.Equal(t, "XSS", v.Type)
}

func TestOneOf3_Match(t *testing.T) {
	u := NewOneOf3C[VulnFound, CodeSafe, ExtraType](ExtraType{Info: "extra"})

	cCalled := false
	u.Match(
		func(v VulnFound) { t.Error("should not be called") },
		func(s CodeSafe) { t.Error("should not be called") },
		func(e ExtraType) {
			cCalled = true
			assert.Equal(t, "extra", e.Info)
		},
	)
	assert.True(t, cCalled)
}

func TestBuildOneOf2OutputTools(t *testing.T) {
	tools, infos, err := buildOneOf2OutputTools[VulnFound, CodeSafe]()
	require.NoError(t, err)
	assert.Len(t, tools, 2)
	assert.Len(t, infos, 2)

	assert.Equal(t, output.DefaultOutputToolName+"_VulnFound", infos[0].toolName)
	assert.Equal(t, output.DefaultOutputToolName+"_CodeSafe", infos[1].toolName)
	assert.Equal(t, 0, infos[0].typeIndex)
	assert.Equal(t, 1, infos[1].typeIndex)
}

func TestBuildOneOf3OutputTools(t *testing.T) {
	tools, infos, err := buildOneOf3OutputTools[VulnFound, CodeSafe, ExtraType]()
	require.NoError(t, err)
	assert.Len(t, tools, 3)
	assert.Len(t, infos, 3)

	assert.Equal(t, output.DefaultOutputToolName+"_VulnFound", infos[0].toolName)
	assert.Equal(t, output.DefaultOutputToolName+"_CodeSafe", infos[1].toolName)
	assert.Equal(t, output.DefaultOutputToolName+"_ExtraType", infos[2].toolName)
}

func TestTypeName(t *testing.T) {
	assert.Equal(t, "VulnFound", typeName[VulnFound]())
	assert.Equal(t, "CodeSafe", typeName[CodeSafe]())
	assert.Equal(t, "int", typeName[int]())
	assert.Equal(t, "string", typeName[string]())
}

// -- Integration tests: OneOf2/OneOf3 agents --

func TestNewOneOf2_ReturnsVariantA(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{
					Name:      output.DefaultOutputToolName + "_VulnFound",
					Arguments: `{"type":"SQLi","severity":"high"}`,
				},
			}},
		},
	)

	a := NewOneOf2[NoDeps, VulnFound, CodeSafe](tm)

	result, err := a.Run(context.Background(), "Scan code", NoDeps{})
	require.NoError(t, err)

	aCalled := false
	result.Output.Match(
		func(v VulnFound) {
			aCalled = true
			assert.Equal(t, "SQLi", v.Type)
			assert.Equal(t, "high", v.Severity)
		},
		func(_ CodeSafe) {
			t.Error("should not be variant B")
		},
	)
	assert.True(t, aCalled)
}

func TestNewOneOf2_ReturnsVariantB(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{
					Name:      output.DefaultOutputToolName + "_CodeSafe",
					Arguments: `{"summary":"No vulnerabilities found"}`,
				},
			}},
		},
	)

	a := NewOneOf2[NoDeps, VulnFound, CodeSafe](tm)

	result, err := a.Run(context.Background(), "Scan code", NoDeps{})
	require.NoError(t, err)

	bCalled := false
	result.Output.Match(
		func(_ VulnFound) {
			t.Error("should not be variant A")
		},
		func(s CodeSafe) {
			bCalled = true
			assert.Equal(t, "No vulnerabilities found", s.Summary)
		},
	)
	assert.True(t, bCalled)
}

func TestNewOneOf2_RegistersBothOutputTools(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{
					Name:      output.DefaultOutputToolName + "_VulnFound",
					Arguments: `{"type":"X","severity":"low"}`,
				},
			}},
		},
	)

	a := NewOneOf2[NoDeps, VulnFound, CodeSafe](tm)
	_, err := a.Run(context.Background(), "Scan", NoDeps{})
	require.NoError(t, err)

	// Verify both output tools were sent to the model
	call := tm.AllCalls()[0]
	toolNames := make(map[string]bool)
	for _, ti := range call.Tools {
		toolNames[ti.Name] = true
	}
	assert.True(t, toolNames[output.DefaultOutputToolName+"_VulnFound"], "VulnFound output tool should be registered")
	assert.True(t, toolNames[output.DefaultOutputToolName+"_CodeSafe"], "CodeSafe output tool should be registered")
}

func TestNewOneOf2_ParseErrorTriggersRetry(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{
					Name:      output.DefaultOutputToolName + "_VulnFound",
					Arguments: `{bad json}`,
				},
			}},
		},
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_2", Type: "function",
				Function: schema.FunctionCall{
					Name:      output.DefaultOutputToolName + "_VulnFound",
					Arguments: `{"type":"OK","severity":"low"}`,
				},
			}},
		},
	)

	a := NewOneOf2[NoDeps, VulnFound, CodeSafe](tm,
		WithMaxResultRetries[NoDeps, OneOf2[VulnFound, CodeSafe]](2),
	)

	result, err := a.Run(context.Background(), "Scan", NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, 2, tm.CallCount())

	v, ok := result.Output.Value().(VulnFound)
	require.True(t, ok)
	assert.Equal(t, "OK", v.Type)
}

func TestNewOneOf3_ReturnsVariantC(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{
					Name:      output.DefaultOutputToolName + "_ExtraType",
					Arguments: `{"info":"extra data"}`,
				},
			}},
		},
	)

	a := NewOneOf3[NoDeps, VulnFound, CodeSafe, ExtraType](tm)

	result, err := a.Run(context.Background(), "Analyze", NoDeps{})
	require.NoError(t, err)

	cCalled := false
	result.Output.Match(
		func(_ VulnFound) { t.Error("should not be A") },
		func(_ CodeSafe) { t.Error("should not be B") },
		func(e ExtraType) {
			cCalled = true
			assert.Equal(t, "extra data", e.Info)
		},
	)
	assert.True(t, cCalled)
}

func TestNewOneOf2_WithTools(t *testing.T) {
	// Test that union agents work with regular tools too
	type EmptyArgs struct{}

	tm := testutil.NewTestModel(
		// First: call a regular tool
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{Name: "scan", Arguments: `{}`},
			}},
		},
		// Then: return via union output tool
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_2", Type: "function",
				Function: schema.FunctionCall{
					Name:      output.DefaultOutputToolName + "_CodeSafe",
					Arguments: `{"summary":"clean"}`,
				},
			}},
		},
	)

	a := NewOneOf2[NoDeps, VulnFound, CodeSafe](tm,
		WithToolFunc[NoDeps, OneOf2[VulnFound, CodeSafe], EmptyArgs](
			"scan", "Run scan",
			func(_ *RunContext[NoDeps], _ EmptyArgs) (string, error) {
				return "no issues", nil
			},
		),
	)

	result, err := a.Run(context.Background(), "Check code", NoDeps{})
	require.NoError(t, err)

	bCalled := false
	result.Output.Match(
		func(_ VulnFound) { t.Error("should not be A") },
		func(s CodeSafe) {
			bCalled = true
			assert.Equal(t, "clean", s.Summary)
		},
	)
	assert.True(t, bCalled)
	assert.Equal(t, 2, tm.CallCount())
}
