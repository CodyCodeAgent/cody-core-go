package agent

import (
	"context"
	"fmt"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/codycode/cody-core-go/testutil"
)

// -- Test types --

type TestDeps struct {
	APIKey string
	UserID string
}

type TestOutput struct {
	City        string  `json:"city"`
	Temperature float64 `json:"temperature"`
	Condition   string  `json:"condition"`
}

type GetWeatherArgs struct {
	City string `json:"city"`
}

// -- Tests: Pure text Agent --

func TestAgent_Run_TextOutput(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{Text: "Hello, world!"},
	)

	a := New[NoDeps, string](tm,
		WithSystemPrompt[NoDeps, string]("You are a helpful assistant."),
	)

	result, err := a.Run(context.Background(), "Hi!", NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, "Hello, world!", result.Output)
	assert.Equal(t, 1, tm.CallCount())

	// Verify system message was sent
	call := tm.AllCalls()[0]
	assert.Equal(t, schema.System, call.Messages[0].Role)
	assert.Equal(t, "You are a helpful assistant.", call.Messages[0].Content)

	// Verify user message was sent
	assert.Equal(t, schema.User, call.Messages[1].Role)
	assert.Equal(t, "Hi!", call.Messages[1].Content)

	// No output tool registered for string type
	assert.Empty(t, call.Tools)
}

// -- Tests: Structured output --

func TestAgent_Run_StructuredOutput(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID:   "call_1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "final_result",
					Arguments: `{"city":"Beijing","temperature":22.5,"condition":"sunny"}`,
				},
			}},
		},
	)

	a := New[NoDeps, TestOutput](tm,
		WithSystemPrompt[NoDeps, TestOutput]("Weather assistant."),
	)

	result, err := a.Run(context.Background(), "Weather?", NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, "Beijing", result.Output.City)
	assert.Equal(t, 22.5, result.Output.Temperature)
	assert.Equal(t, "sunny", result.Output.Condition)

	// Verify output tool was registered
	call := tm.AllCalls()[0]
	hasOutputTool := false
	for _, ti := range call.Tools {
		if ti.Name == "final_result" {
			hasOutputTool = true
		}
	}
	assert.True(t, hasOutputTool, "final_result tool should be registered")
}

// -- Tests: Tool call + structured output --

func TestAgent_Run_ToolCallThenOutput(t *testing.T) {
	tm := testutil.NewTestModel(
		// First: model calls get_weather tool
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID:   "call_1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "get_weather",
					Arguments: `{"city":"Beijing"}`,
				},
			}},
		},
		// Second: model returns final_result
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID:   "call_2",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "final_result",
					Arguments: `{"city":"Beijing","temperature":22.5,"condition":"sunny"}`,
				},
			}},
		},
	)

	toolCalled := false

	a := New[TestDeps, TestOutput](tm,
		WithSystemPrompt[TestDeps, TestOutput]("Weather assistant."),
		WithToolFunc[TestDeps, TestOutput, GetWeatherArgs](
			"get_weather", "Get weather for a city",
			func(ctx *RunContext[TestDeps], args GetWeatherArgs) (string, error) {
				toolCalled = true
				assert.Equal(t, "test-key", ctx.Deps.APIKey)
				assert.Equal(t, "Beijing", args.City)
				return `{"temp":22.5,"condition":"sunny"}`, nil
			},
		),
	)

	result, err := a.Run(context.Background(), "Beijing weather", TestDeps{APIKey: "test-key", UserID: "u1"})
	require.NoError(t, err)
	assert.True(t, toolCalled)
	assert.Equal(t, "Beijing", result.Output.City)
	assert.Equal(t, 2, tm.CallCount())
}

// -- Tests: Multiple tool calls in one turn --

func TestAgent_Run_MultipleToolCalls(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{
				{ID: "call_1", Type: "function", Function: schema.FunctionCall{Name: "tool_a", Arguments: `{}`}},
				{ID: "call_2", Type: "function", Function: schema.FunctionCall{Name: "tool_b", Arguments: `{}`}},
			},
		},
		testutil.TestResponse{Text: "done"},
	)

	aCalled, bCalled := false, false
	type EmptyArgs struct{}

	a := New[NoDeps, string](tm,
		WithToolFunc[NoDeps, string, EmptyArgs]("tool_a", "Tool A", func(_ *RunContext[NoDeps], _ EmptyArgs) (string, error) {
			aCalled = true
			return "a_result", nil
		}),
		WithToolFunc[NoDeps, string, EmptyArgs]("tool_b", "Tool B", func(_ *RunContext[NoDeps], _ EmptyArgs) (string, error) {
			bCalled = true
			return "b_result", nil
		}),
	)

	result, err := a.Run(context.Background(), "Do both", NoDeps{})
	require.NoError(t, err)
	assert.True(t, aCalled)
	assert.True(t, bCalled)
	assert.Equal(t, "done", result.Output)
}

// -- Tests: Int output --

func TestAgent_Run_IntOutput(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID:   "call_1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "final_result",
					Arguments: `{"result":42}`,
				},
			}},
		},
	)

	a := New[NoDeps, int](tm)
	result, err := a.Run(context.Background(), "What is the answer?", NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, 42, result.Output)
}

// -- Tests: Bool output --

func TestAgent_Run_BoolOutput(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID:   "call_1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "final_result",
					Arguments: `{"result":true}`,
				},
			}},
		},
	)

	a := New[NoDeps, bool](tm)
	result, err := a.Run(context.Background(), "Is it safe?", NoDeps{})
	require.NoError(t, err)
	assert.True(t, result.Output)
}

// -- Tests: []string output --

func TestAgent_Run_StringSliceOutput(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID:   "call_1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "final_result",
					Arguments: `{"result":["go","rust","python"]}`,
				},
			}},
		},
	)

	a := New[NoDeps, []string](tm)
	result, err := a.Run(context.Background(), "List languages", NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, []string{"go", "rust", "python"}, result.Output)
}

// -- Tests: Output validation + retry --

func TestAgent_Run_OutputValidatorRetry(t *testing.T) {
	tm := testutil.NewTestModel(
		// First response: invalid temperature
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID:   "call_1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "final_result",
					Arguments: `{"city":"Beijing","temperature":-999,"condition":"sunny"}`,
				},
			}},
		},
		// Second response: corrected
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID:   "call_2",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "final_result",
					Arguments: `{"city":"Beijing","temperature":22.5,"condition":"sunny"}`,
				},
			}},
		},
	)

	a := New[NoDeps, TestOutput](tm,
		WithOutputValidator[NoDeps, TestOutput](func(_ context.Context, o TestOutput) (TestOutput, error) {
			if o.Temperature < -100 {
				return o, NewModelRetry("temperature is unreasonably low")
			}
			return o, nil
		}),
		WithMaxResultRetries[NoDeps, TestOutput](2),
	)

	result, err := a.Run(context.Background(), "Weather?", NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, 22.5, result.Output.Temperature)
	assert.Equal(t, 2, tm.CallCount())
}

// -- Tests: Tool returns ModelRetry --

func TestAgent_Run_ToolModelRetry(t *testing.T) {
	type EmptyArgs struct{}
	callCount := 0

	tm := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{Name: "my_tool", Arguments: `{}`},
			}},
		},
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_2", Type: "function",
				Function: schema.FunctionCall{Name: "my_tool", Arguments: `{}`},
			}},
		},
		testutil.TestResponse{Text: "done"},
	)

	a := New[NoDeps, string](tm,
		WithToolFunc[NoDeps, string, EmptyArgs]("my_tool", "My tool", func(_ *RunContext[NoDeps], _ EmptyArgs) (string, error) {
			callCount++
			if callCount == 1 {
				return "", NewModelRetry("try again with different approach")
			}
			return "success", nil
		}),
		WithMaxRetries[NoDeps, string](3),
	)

	result, err := a.Run(context.Background(), "Do it", NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, "done", result.Output)
	assert.Equal(t, 2, callCount)
}

// -- Tests: Max retries exceeded --

func TestAgent_Run_MaxResultRetriesExceeded(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{Name: "final_result", Arguments: `{"city":"X","temperature":-999,"condition":"bad"}`},
			}},
		},
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_2", Type: "function",
				Function: schema.FunctionCall{Name: "final_result", Arguments: `{"city":"X","temperature":-999,"condition":"bad"}`},
			}},
		},
	)

	a := New[NoDeps, TestOutput](tm,
		WithOutputValidator[NoDeps, TestOutput](func(_ context.Context, o TestOutput) (TestOutput, error) {
			return o, NewModelRetry("always fail")
		}),
		WithMaxResultRetries[NoDeps, TestOutput](1),
	)

	_, err := a.Run(context.Background(), "Weather?", NoDeps{})
	require.Error(t, err)
	assert.IsType(t, &ResultRetriesExceededError{}, err)
}

// -- Tests: Text response when struct expected (retry) --

func TestAgent_Run_TextResponseForStructTriggersRetry(t *testing.T) {
	tm := testutil.NewTestModel(
		// First: model returns text instead of tool call
		testutil.TestResponse{Text: "Let me think..."},
		// Second: model correctly returns output tool
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{Name: "final_result", Arguments: `{"city":"Beijing","temperature":20,"condition":"cloudy"}`},
			}},
		},
	)

	a := New[NoDeps, TestOutput](tm,
		WithMaxResultRetries[NoDeps, TestOutput](2),
	)

	result, err := a.Run(context.Background(), "Weather?", NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, "Beijing", result.Output.City)
	assert.Equal(t, 2, tm.CallCount())
}

// -- Tests: Dynamic system prompt --

func TestAgent_Run_DynamicSystemPrompt(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{Text: "OK"},
	)

	a := New[TestDeps, string](tm,
		WithSystemPrompt[TestDeps, string]("Static prompt."),
		WithDynamicSystemPrompt[TestDeps, string](func(ctx *RunContext[TestDeps]) (string, error) {
			return fmt.Sprintf("User: %s", ctx.Deps.UserID), nil
		}),
	)

	_, err := a.Run(context.Background(), "Hi", TestDeps{UserID: "user_42"})
	require.NoError(t, err)

	call := tm.AllCalls()[0]
	assert.Equal(t, 3, len(call.Messages)) // static system + dynamic system + user
	assert.Equal(t, schema.System, call.Messages[0].Role)
	assert.Equal(t, "Static prompt.", call.Messages[0].Content)
	assert.Equal(t, schema.System, call.Messages[1].Role)
	assert.Equal(t, "User: user_42", call.Messages[1].Content)
}

// -- Tests: Message history --

func TestAgent_Run_WithHistory(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{Text: "You are Alice."},
	)

	a := New[NoDeps, string](tm,
		WithSystemPrompt[NoDeps, string]("Assistant."),
	)

	history := []*schema.Message{
		{Role: schema.User, Content: "My name is Alice."},
		{Role: schema.Assistant, Content: "Nice to meet you, Alice!"},
	}

	result, err := a.Run(context.Background(), "What's my name?", NoDeps{}, WithHistory(history))
	require.NoError(t, err)
	assert.Equal(t, "You are Alice.", result.Output)

	// Check message order: system, history..., user prompt
	call := tm.AllCalls()[0]
	assert.Equal(t, schema.System, call.Messages[0].Role)
	assert.Equal(t, schema.User, call.Messages[1].Role)
	assert.Equal(t, "My name is Alice.", call.Messages[1].Content)
	assert.Equal(t, schema.Assistant, call.Messages[2].Role)
	assert.Equal(t, schema.User, call.Messages[3].Role)
	assert.Equal(t, "What's my name?", call.Messages[3].Content)

	// Check NewMessages vs AllMessages
	assert.Equal(t, 4, len(result.AllMessages()))  // history(2) + user(1) + assistant(1)
	assert.Equal(t, 2, len(result.NewMessages()))   // user(1) + assistant(1)
}

// -- Tests: Usage limits --

func TestAgent_Run_UsageLimitExceeded(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{
			Text: "First",
			Usage: &schema.TokenUsage{
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
			},
		},
		testutil.TestResponse{Text: "Second"},
	)

	type EmptyArgs struct{}
	a := New[NoDeps, string](tm,
		WithToolFunc[NoDeps, string, EmptyArgs]("dummy", "Dummy", func(_ *RunContext[NoDeps], _ EmptyArgs) (string, error) {
			return "ok", nil
		}),
	)

	// The first response returns text for a string agent, so it completes.
	// To test usage limits, we need a scenario where the loop continues.
	// Let's use a struct output agent that gets text first (retry), then exceeds limit.

	tmStruct := testutil.NewTestModel(
		testutil.TestResponse{
			Text: "thinking...",
			Usage: &schema.TokenUsage{
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
			},
		},
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{Name: "final_result", Arguments: `{"city":"X","temperature":1,"condition":"ok"}`},
			}},
		},
	)

	_ = a // use the string agent above for a different test
	aStruct := New[NoDeps, TestOutput](tmStruct,
		WithMaxResultRetries[NoDeps, TestOutput](5),
	)

	// Set request limit to 1 — should fail on second request
	_, err := aStruct.Run(context.Background(), "Weather?", NoDeps{},
		WithUsageLimits(UsageLimits{RequestLimit: 1}),
	)
	require.Error(t, err)
	assert.IsType(t, &UsageLimitExceededError{}, err)
}

// -- Tests: Output tool with regular tool (early strategy) --

func TestAgent_Run_OutputToolWithRegularTool_EarlyStrategy(t *testing.T) {
	regularToolCalled := false
	type EmptyArgs struct{}

	tm := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{
				{ID: "call_1", Type: "function", Function: schema.FunctionCall{Name: "final_result", Arguments: `{"city":"X","temperature":1,"condition":"ok"}`}},
				{ID: "call_2", Type: "function", Function: schema.FunctionCall{Name: "search", Arguments: `{}`}},
			},
		},
	)

	a := New[NoDeps, TestOutput](tm,
		WithToolFunc[NoDeps, TestOutput, EmptyArgs]("search", "Search", func(_ *RunContext[NoDeps], _ EmptyArgs) (string, error) {
			regularToolCalled = true
			return "found", nil
		}),
	)

	result, err := a.Run(context.Background(), "Weather?", NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, "X", result.Output.City)
	assert.False(t, regularToolCalled, "regular tool should not be called in early strategy")
}

// -- Tests: WithModel --

func TestAgent_WithModel(t *testing.T) {
	tm1 := testutil.NewTestModel(testutil.TestResponse{Text: "from tm1"})
	tm2 := testutil.NewTestModel(testutil.TestResponse{Text: "from tm2"})

	a := New[NoDeps, string](tm1)
	a2 := a.WithModel(tm2)

	r1, err := a.Run(context.Background(), "Hi", NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, "from tm1", r1.Output)

	r2, err := a2.Run(context.Background(), "Hi", NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, "from tm2", r2.Output)
}

// -- Tests: Unknown tool call --

func TestAgent_Run_UnknownToolCall(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{Name: "nonexistent", Arguments: `{}`},
			}},
		},
		testutil.TestResponse{Text: "ok"},
	)

	a := New[NoDeps, string](tm)
	result, err := a.Run(context.Background(), "Do it", NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Output)

	// Check that the error feedback was sent for the unknown tool
	call := tm.AllCalls()[1]
	hasToolError := false
	for _, msg := range call.Messages {
		if msg.Role == schema.Tool && msg.ToolCallID == "call_1" {
			assert.Contains(t, msg.Content, "unknown tool")
			hasToolError = true
		}
	}
	assert.True(t, hasToolError)
}

// -- Tests: Tool panic recovery --

func TestAgent_Run_ToolPanicRecovery(t *testing.T) {
	type EmptyArgs struct{}

	tm := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{Name: "panicky", Arguments: `{}`},
			}},
		},
		testutil.TestResponse{Text: "recovered"},
	)

	a := New[NoDeps, string](tm,
		WithToolFunc[NoDeps, string, EmptyArgs]("panicky", "Panicky tool", func(_ *RunContext[NoDeps], _ EmptyArgs) (string, error) {
			panic("something went wrong")
		}),
	)

	result, err := a.Run(context.Background(), "Do it", NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, "recovered", result.Output)
}

// -- Tests: Concurrent runs --

func TestAgent_ConcurrentRuns(t *testing.T) {
	// Create a FunctionModel that echoes the input
	fm := testutil.NewFunctionModel(func(msgs []*schema.Message, _ []*schema.ToolInfo) (*schema.Message, error) {
		// Find the last user message
		for i := len(msgs) - 1; i >= 0; i-- {
			if msgs[i].Role == schema.User {
				return &schema.Message{Role: schema.Assistant, Content: "echo: " + msgs[i].Content}, nil
			}
		}
		return &schema.Message{Role: schema.Assistant, Content: "no input"}, nil
	})

	a := New[NoDeps, string](fm)

	done := make(chan struct{}, 10)
	for i := 0; i < 10; i++ {
		go func(n int) {
			defer func() { done <- struct{}{} }()
			result, err := a.Run(context.Background(), fmt.Sprintf("msg_%d", n), NoDeps{})
			assert.NoError(t, err)
			assert.Contains(t, result.Output, "echo:")
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
