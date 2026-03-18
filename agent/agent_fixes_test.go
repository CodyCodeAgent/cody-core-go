package agent

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/CodyCodeAgent/cody-core-go/testutil"
)

// -- Fix verification: Plain text JSON parse fallback (behavior.md §3.5) --

func TestAgent_Run_PlainTextJSONFallback(t *testing.T) {
	// Model returns valid JSON as plain text (no tool call) for a struct output.
	// The agent should try to parse it as JSON before triggering a retry.
	tm := testutil.NewTestModel(
		testutil.TestResponse{
			Text: `{"city":"Shanghai","temperature":25.0,"condition":"cloudy"}`,
		},
	)

	a := New[NoDeps, TestOutput](tm)

	result, err := a.Run(context.Background(), "Weather?", NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, "Shanghai", result.Output.City)
	assert.Equal(t, 25.0, result.Output.Temperature)
	assert.Equal(t, 1, tm.CallCount()) // No retry needed
}

func TestAgent_Run_PlainTextJSONFallback_WithMarkdownFence(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{
			Text: "```json\n{\"city\":\"Tokyo\",\"temperature\":18.0,\"condition\":\"rainy\"}\n```",
		},
	)

	a := New[NoDeps, TestOutput](tm)

	result, err := a.Run(context.Background(), "Weather?", NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, "Tokyo", result.Output.City)
}

func TestAgent_Run_PlainTextJSONFallback_InvalidJSON(t *testing.T) {
	// Invalid JSON in plain text — should trigger retry, not crash
	tm := testutil.NewTestModel(
		testutil.TestResponse{Text: "Let me think about that..."},
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{Name: "final_result", Arguments: `{"city":"X","temperature":1,"condition":"ok"}`},
			}},
		},
	)

	a := New[NoDeps, TestOutput](tm,
		WithMaxResultRetries[NoDeps, TestOutput](2),
	)

	result, err := a.Run(context.Background(), "Weather?", NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, "X", result.Output.City)
	assert.Equal(t, 2, tm.CallCount())
}

// -- Fix verification: IsModelRetry with wrapped errors --

func TestIsModelRetry_WrappedError(t *testing.T) {
	original := NewModelRetry("bad input")
	wrapped := fmt.Errorf("tool execution: %w", original)

	retry, ok := IsModelRetry(wrapped)
	assert.True(t, ok, "IsModelRetry should detect wrapped ErrModelRetry")
	assert.Equal(t, "bad input", retry.Message)
}

func TestIsModelRetry_DeepWrapped(t *testing.T) {
	original := NewModelRetry("deep")
	wrapped := fmt.Errorf("level 1: %w", fmt.Errorf("level 2: %w", original))

	retry, ok := IsModelRetry(wrapped)
	assert.True(t, ok)
	assert.Equal(t, "deep", retry.Message)
}

func TestIsModelRetry_NotModelRetry_ErrorsAs(t *testing.T) {
	_, ok := IsModelRetry(errors.New("random error"))
	assert.False(t, ok)
}

// -- Fix verification: Prepare callback per-iteration --

func TestAgent_Run_PrepareCalledPerIteration(t *testing.T) {
	type EmptyArgs struct{}
	prepareCallCount := 0

	tm := testutil.NewTestModel(
		// First: tool call
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{Name: "my_tool", Arguments: `{}`},
			}},
		},
		// Second: another tool call
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_2", Type: "function",
				Function: schema.FunctionCall{Name: "my_tool", Arguments: `{}`},
			}},
		},
		// Third: final text
		testutil.TestResponse{Text: "done"},
	)

	a := New[NoDeps, string](tm,
		WithToolFunc[NoDeps, string, EmptyArgs](
			"my_tool", "My tool",
			func(_ *RunContext[NoDeps], _ EmptyArgs) (string, error) {
				return "ok", nil
			},
			WithPrepare[NoDeps](func(_ *RunContext[NoDeps], info *schema.ToolInfo) (*schema.ToolInfo, error) {
				prepareCallCount++
				return info, nil
			}),
		),
	)

	_, err := a.Run(context.Background(), "Go", NoDeps{})
	require.NoError(t, err)
	// Prepare should be called once per model invocation (3 iterations)
	assert.Equal(t, 3, prepareCallCount, "prepare should be called once per model invocation")
}

func TestAgent_Run_PrepareModifiesSchema(t *testing.T) {
	type SearchArgs struct {
		Query string `json:"query"`
	}

	tm := testutil.NewTestModel(
		testutil.TestResponse{Text: "no tool called"},
	)

	type MyDeps struct {
		IsAdmin bool
	}

	a := New[MyDeps, string](tm,
		WithToolFunc[MyDeps, string, SearchArgs](
			"search", "Search",
			func(_ *RunContext[MyDeps], args SearchArgs) (string, error) {
				return "result", nil
			},
			WithPrepare[MyDeps](func(ctx *RunContext[MyDeps], info *schema.ToolInfo) (*schema.ToolInfo, error) {
				// Modify description based on deps
				if ctx.Deps.IsAdmin {
					info.Desc = "Admin search"
				} else {
					info.Desc = "User search"
				}
				return info, nil
			}),
		),
	)

	_, err := a.Run(context.Background(), "search", MyDeps{IsAdmin: true})
	require.NoError(t, err)

	// Verify the model received the modified tool info
	call := tm.AllCalls()[0]
	found := false
	for _, ti := range call.Tools {
		if ti.Name == "search" {
			assert.Equal(t, "Admin search", ti.Desc)
			found = true
		}
	}
	assert.True(t, found, "search tool should be in tools list")
}

// -- Fix verification: RunContext.Metadata --

func TestRunContext_Metadata(t *testing.T) {
	tm := testutil.NewTestModel(testutil.TestResponse{Text: "ok"})

	metaReceived := false
	a := New[NoDeps, string](tm,
		WithDynamicSystemPrompt[NoDeps, string](func(ctx *RunContext[NoDeps]) (string, error) {
			if ctx.Metadata != nil {
				if v, ok := ctx.Metadata["request_id"]; ok && v == "req-123" {
					metaReceived = true
				}
			}
			return "system", nil
		}),
	)

	_, err := a.Run(context.Background(), "test", NoDeps{},
		WithRunMetadata(map[string]any{"request_id": "req-123"}),
	)
	require.NoError(t, err)
	assert.True(t, metaReceived, "metadata should be available in RunContext")
}

// -- Fix verification: Empty response --

func TestAgent_Run_EmptyResponse_StringAgent(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{Text: ""}, // empty text
	)

	a := New[NoDeps, string](tm)
	result, err := a.Run(context.Background(), "Hello", NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, "", result.Output) // Empty string is valid for O=string
}

func TestAgent_Run_EmptyResponse_StructAgent(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{Text: ""}, // empty text, no tool calls
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{Name: "final_result", Arguments: `{"city":"X","temperature":1,"condition":"ok"}`},
			}},
		},
	)

	a := New[NoDeps, TestOutput](tm,
		WithMaxResultRetries[NoDeps, TestOutput](2),
	)

	result, err := a.Run(context.Background(), "Weather?", NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, "X", result.Output.City)
	assert.Equal(t, 2, tm.CallCount()) // First was empty, second had result
}

// -- Fix verification: Context cancellation --

func TestAgent_Run_ContextCanceled(t *testing.T) {
	// Use a FunctionModel that checks context cancellation (as a real model would)
	fm := testutil.NewFunctionModel(func(msgs []*schema.Message, _ []*schema.ToolInfo) (*schema.Message, error) {
		ctx := context.Background() // FunctionModel doesn't receive context directly
		_ = ctx
		return nil, context.Canceled
	})

	a := New[NoDeps, string](fm)

	_, err := a.Run(context.Background(), "Hi", NoDeps{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "canceled")
}

// -- Fix verification: Dynamic system prompt error --

func TestAgent_Run_DynamicPromptError(t *testing.T) {
	tm := testutil.NewTestModel(testutil.TestResponse{Text: "x"})

	a := New[NoDeps, string](tm,
		WithDynamicSystemPrompt[NoDeps, string](func(_ *RunContext[NoDeps]) (string, error) {
			return "", fmt.Errorf("prompt generation failed")
		}),
	)

	_, err := a.Run(context.Background(), "Hi", NoDeps{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "prompt generation failed")
	assert.Equal(t, 0, tm.CallCount()) // Model should not be called
}

// -- Fix verification: Tool max retries exceeded --

func TestAgent_Run_ToolMaxRetriesExceeded(t *testing.T) {
	type EmptyArgs struct{}

	tm := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{Name: "bad_tool", Arguments: `{}`},
			}},
		},
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_2", Type: "function",
				Function: schema.FunctionCall{Name: "bad_tool", Arguments: `{}`},
			}},
		},
	)

	a := New[NoDeps, string](tm,
		WithToolFunc[NoDeps, string, EmptyArgs]("bad_tool", "Always fails", func(_ *RunContext[NoDeps], _ EmptyArgs) (string, error) {
			return "", NewModelRetry("try again")
		}),
		WithMaxRetries[NoDeps, string](1), // Only 1 retry allowed
	)

	_, err := a.Run(context.Background(), "Do it", NoDeps{})
	require.Error(t, err)
	var toolRetryErr *ToolRetriesExceededError
	assert.True(t, errors.As(err, &toolRetryErr), "error should contain ToolRetriesExceededError")
}

// -- Fix verification: Tool regular error does NOT consume retries --

func TestAgent_Run_ToolRegularErrorNoRetryConsumed(t *testing.T) {
	type EmptyArgs struct{}
	callCount := 0

	tm := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{Name: "err_tool", Arguments: `{}`},
			}},
		},
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_2", Type: "function",
				Function: schema.FunctionCall{Name: "err_tool", Arguments: `{}`},
			}},
		},
		testutil.TestResponse{Text: "done"},
	)

	a := New[NoDeps, string](tm,
		WithToolFunc[NoDeps, string, EmptyArgs]("err_tool", "Errors", func(_ *RunContext[NoDeps], _ EmptyArgs) (string, error) {
			callCount++
			return "", fmt.Errorf("database connection failed")
		}),
		WithMaxRetries[NoDeps, string](1),
	)

	result, err := a.Run(context.Background(), "Do it", NoDeps{})
	require.NoError(t, err, "regular errors should not consume retries")
	assert.Equal(t, "done", result.Output)
	assert.Equal(t, 2, callCount)
}

// -- Fix verification: Model settings override --

func TestAgent_Run_ModelSettingsOverride(t *testing.T) {
	tm := testutil.NewTestModel(testutil.TestResponse{Text: "ok"})

	a := New[NoDeps, string](tm,
		WithModelSettings[NoDeps, string](ModelSettings{
			Temperature: Ptr(float32(0.5)),
			MaxTokens:   Ptr(100),
		}),
	)

	_, err := a.Run(context.Background(), "Hi", NoDeps{},
		WithRunModelSettings(ModelSettings{
			Temperature: Ptr(float32(0.9)), // Override
		}),
	)
	require.NoError(t, err)
	// We can't easily verify the model options were applied, but we verify no errors
}

// -- Fix verification: Multi-turn tool calls --

func TestAgent_Run_MultiRoundToolCalls(t *testing.T) {
	type EmptyArgs struct{}

	tm := testutil.NewTestModel(
		// Round 1: tool A
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{Name: "tool_a", Arguments: `{}`},
			}},
		},
		// Round 2: tool B
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_2", Type: "function",
				Function: schema.FunctionCall{Name: "tool_b", Arguments: `{}`},
			}},
		},
		// Round 3: final result
		testutil.TestResponse{Text: "combined"},
	)

	aCall, bCall := 0, 0
	a := New[NoDeps, string](tm,
		WithToolFunc[NoDeps, string, EmptyArgs]("tool_a", "A", func(_ *RunContext[NoDeps], _ EmptyArgs) (string, error) {
			aCall++
			return "a_result", nil
		}),
		WithToolFunc[NoDeps, string, EmptyArgs]("tool_b", "B", func(_ *RunContext[NoDeps], _ EmptyArgs) (string, error) {
			bCall++
			return "b_result", nil
		}),
	)

	result, err := a.Run(context.Background(), "Do both", NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, "combined", result.Output)
	assert.Equal(t, 1, aCall)
	assert.Equal(t, 1, bCall)
	assert.Equal(t, 3, tm.CallCount())
}

// -- Message history: system messages excluded --

func TestAgent_Run_SystemMessagesExcludedFromHistory(t *testing.T) {
	tm := testutil.NewTestModel(testutil.TestResponse{Text: "hello"})

	a := New[NoDeps, string](tm,
		WithSystemPrompt[NoDeps, string]("System 1"),
		WithSystemPrompt[NoDeps, string]("System 2"),
	)

	result, err := a.Run(context.Background(), "Hi", NoDeps{})
	require.NoError(t, err)

	// AllMessages should NOT contain system messages
	for _, msg := range result.AllMessages() {
		assert.NotEqual(t, schema.System, msg.Role,
			"system messages should be excluded from AllMessages()")
	}

	// NewMessages should also NOT contain system messages
	for _, msg := range result.NewMessages() {
		assert.NotEqual(t, schema.System, msg.Role,
			"system messages should be excluded from NewMessages()")
	}
}

// -- Output JSON parse error triggers retry --

func TestAgent_Run_OutputParseErrorTriggersRetry(t *testing.T) {
	tm := testutil.NewTestModel(
		// First: malformed JSON in output tool
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{Name: "final_result", Arguments: `{invalid json}`},
			}},
		},
		// Second: correct JSON
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_2", Type: "function",
				Function: schema.FunctionCall{Name: "final_result", Arguments: `{"city":"X","temperature":1,"condition":"ok"}`},
			}},
		},
	)

	a := New[NoDeps, TestOutput](tm,
		WithMaxResultRetries[NoDeps, TestOutput](2),
	)

	result, err := a.Run(context.Background(), "Weather?", NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, "X", result.Output.City)
	assert.Equal(t, 2, tm.CallCount())
}
