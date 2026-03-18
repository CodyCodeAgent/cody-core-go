package agent

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/codycode/cody-core-go/testutil"
)

func TestRunStream_TextOutput(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{Text: "Hello, streaming!"},
	)

	a := New[NoDeps, string](tm,
		WithSystemPrompt[NoDeps, string]("You are helpful."),
	)

	sr, err := a.RunStream(context.Background(), "Hi!", NoDeps{})
	require.NoError(t, err)

	// Consume text stream
	var chunks []string
	for chunk := range sr.TextStream() {
		chunks = append(chunks, chunk)
	}
	assert.Equal(t, []string{"Hello, streaming!"}, chunks)

	// Get final result
	result, err := sr.Final()
	require.NoError(t, err)
	assert.Equal(t, "Hello, streaming!", result.Output)
	assert.Equal(t, 1, tm.CallCount())
}

func TestRunStream_StructuredOutput(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID:   "call_1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "final_result",
					Arguments: `{"city":"Tokyo","temperature":18.5,"condition":"rainy"}`,
				},
			}},
		},
	)

	a := New[NoDeps, TestOutput](tm)

	sr, err := a.RunStream(context.Background(), "Weather in Tokyo?", NoDeps{})
	require.NoError(t, err)

	// For structured output, text stream is empty (output comes via tool call)
	for range sr.TextStream() {
	}

	result, err := sr.Final()
	require.NoError(t, err)
	assert.Equal(t, "Tokyo", result.Output.City)
	assert.Equal(t, 18.5, result.Output.Temperature)
}

func TestRunStream_FinalWithoutTextStream(t *testing.T) {
	// Calling Final() without first calling TextStream() should still work
	tm := testutil.NewTestModel(
		testutil.TestResponse{Text: "Direct final"},
	)

	a := New[NoDeps, string](tm)

	sr, err := a.RunStream(context.Background(), "Hi!", NoDeps{})
	require.NoError(t, err)

	result, err := sr.Final()
	require.NoError(t, err)
	assert.Equal(t, "Direct final", result.Output)
}

func TestRunStream_InitErrors(t *testing.T) {
	// Agent with init errors should return error from RunStream
	a := &Agent[NoDeps, string]{
		initErrors: []error{assert.AnError},
	}

	_, err := a.RunStream(context.Background(), "Hi!", NoDeps{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent initialization errors")
}

func TestRunStream_TextStreamCalledOnce(t *testing.T) {
	// Multiple calls to TextStream should return the same channel (sync.Once)
	tm := testutil.NewTestModel(
		testutil.TestResponse{Text: "once"},
	)

	a := New[NoDeps, string](tm)
	sr, err := a.RunStream(context.Background(), "Hi!", NoDeps{})
	require.NoError(t, err)

	ch1 := sr.TextStream()
	ch2 := sr.TextStream()
	// Should be the same channel
	assert.Equal(t, ch1, ch2)

	// Drain and verify
	result, err := sr.Final()
	require.NoError(t, err)
	assert.Equal(t, "once", result.Output)
}

func TestRunStream_WithToolCalls(t *testing.T) {
	type EmptyArgs struct{}

	tm := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{Name: "greet", Arguments: `{}`},
			}},
		},
		testutil.TestResponse{Text: "Hello from tool!"},
	)

	toolCalled := false
	a := New[NoDeps, string](tm,
		WithToolFunc[NoDeps, string, EmptyArgs]("greet", "Greet", func(_ *RunContext[NoDeps], _ EmptyArgs) (string, error) {
			toolCalled = true
			return "greeted", nil
		}),
	)

	sr, err := a.RunStream(context.Background(), "Greet me", NoDeps{})
	require.NoError(t, err)

	result, err := sr.Final()
	require.NoError(t, err)
	assert.True(t, toolCalled)
	assert.Equal(t, "Hello from tool!", result.Output)
}

func TestRunStream_Close(t *testing.T) {
	// Close should not panic even without a real stream
	sr := &StreamResult[string]{}
	sr.Close() // should be no-op
}
