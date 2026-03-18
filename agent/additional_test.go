package agent

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/CodyCodeAgent/cody-core-go/testutil"
)

// -- Tests: WithTool (wrapping an existing InvokableTool) --

type staticTool struct {
	name   string
	desc   string
	result string
}

func (s *staticTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: s.name,
		Desc: s.desc,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"input": {Type: schema.String, Desc: "Input value"},
		}),
	}, nil
}

func (s *staticTool) InvokableRun(_ context.Context, _ string, _ ...tool.Option) (string, error) {
	return s.result, nil
}

func TestAgent_WithTool(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{Name: "my_static_tool", Arguments: `{"input":"hello"}`},
			}},
		},
		testutil.TestResponse{Text: "done"},
	)

	st := &staticTool{name: "my_static_tool", desc: "A static tool", result: "static_result"}

	a := New[NoDeps, string](tm,
		WithTool[NoDeps, string](st),
	)

	result, err := a.Run(context.Background(), "Use the tool", NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, "done", result.Output)

	// Verify the tool was registered with the model
	call := tm.AllCalls()[0]
	found := false
	for _, ti := range call.Tools {
		if ti.Name == "my_static_tool" {
			found = true
			assert.Equal(t, "A static tool", ti.Desc)
		}
	}
	assert.True(t, found, "my_static_tool should be registered")
}

// -- Tests: RunWithHistory --

func TestAgent_RunWithHistory(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{Text: "You are Alice."},
	)

	a := New[NoDeps, string](tm,
		WithSystemPrompt[NoDeps, string]("Memory assistant."),
	)

	history := []*schema.Message{
		{Role: schema.User, Content: "My name is Alice."},
		{Role: schema.Assistant, Content: "Nice to meet you, Alice!"},
	}

	result, err := a.RunWithHistory(context.Background(), "What's my name?", NoDeps{}, history)
	require.NoError(t, err)
	assert.Equal(t, "You are Alice.", result.Output)

	// Verify history was included
	call := tm.AllCalls()[0]
	assert.Equal(t, schema.System, call.Messages[0].Role)
	assert.Equal(t, "My name is Alice.", call.Messages[1].Content)
	assert.Equal(t, "Nice to meet you, Alice!", call.Messages[2].Content)
	assert.Equal(t, "What's my name?", call.Messages[3].Content)
}

// -- Tests: WithMaxIterations --

func TestAgent_WithMaxIterations(t *testing.T) {
	type EmptyArgs struct{}

	// Create a model that always returns tool calls (never a final answer)
	fm := testutil.NewFunctionModel(func(msgs []*schema.Message, _ []*schema.ToolInfo) (*schema.Message, error) {
		return &schema.Message{
			Role: schema.Assistant,
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{Name: "loop_tool", Arguments: `{}`},
			}},
		}, nil
	})

	a := New[NoDeps, string](fm,
		WithMaxIterations[NoDeps, string](3),
		WithToolFunc[NoDeps, string, EmptyArgs]("loop_tool", "Loops", func(_ *RunContext[NoDeps], _ EmptyArgs) (string, error) {
			return "looping", nil
		}),
	)

	_, err := a.Run(context.Background(), "Loop forever", NoDeps{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "max iterations (3)")
}

func TestAgent_WithMaxIterations_Default(t *testing.T) {
	// Default maxIterations is 20
	a := New[NoDeps, string](testutil.NewTestModel(testutil.TestResponse{Text: "ok"}))
	assert.Equal(t, defaultMaxIterations, a.maxIterations)
}

// -- Tests: NewMessages returns empty slice not nil --

func TestResult_NewMessages_EmptySlice(t *testing.T) {
	r := &Result[string]{
		allMessages:     []*schema.Message{},
		newMessageStart: 0,
	}
	msgs := r.NewMessages()
	require.NotNil(t, msgs, "NewMessages should return empty slice, not nil")
	assert.Empty(t, msgs)
}

func TestResult_NewMessages_StartBeyondLength(t *testing.T) {
	r := &Result[string]{
		allMessages:     []*schema.Message{{Role: schema.User, Content: "hi"}},
		newMessageStart: 5,
	}
	msgs := r.NewMessages()
	require.NotNil(t, msgs, "NewMessages should return empty slice, not nil")
	assert.Empty(t, msgs)
}

// -- Tests: GetMetadata --

func TestGetMetadata(t *testing.T) {
	type MyDeps struct{}

	meta := map[string]any{"key": "value", "count": 42}
	rc := &RunContext[MyDeps]{
		Ctx:      context.Background(),
		Metadata: meta,
	}

	ctx := withRunContext[MyDeps](context.Background(), rc)

	got := GetMetadata[MyDeps](ctx)
	assert.Equal(t, "value", got["key"])
	assert.Equal(t, 42, got["count"])
}

func TestGetMetadata_NotFound(t *testing.T) {
	type MyDeps struct{}

	got := GetMetadata[MyDeps](context.Background())
	assert.Nil(t, got)
}

func TestGetMetadata_InTool(t *testing.T) {
	type EmptyArgs struct{}

	tm := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{Name: "meta_tool", Arguments: `{}`},
			}},
		},
		testutil.TestResponse{Text: "done"},
	)

	var receivedMeta map[string]any
	a := New[NoDeps, string](tm,
		WithToolFunc[NoDeps, string, EmptyArgs]("meta_tool", "Gets metadata", func(rc *RunContext[NoDeps], _ EmptyArgs) (string, error) {
			receivedMeta = rc.Metadata
			return "ok", nil
		}),
	)

	_, err := a.Run(context.Background(), "test", NoDeps{},
		WithRunMetadata(map[string]any{"trace_id": "abc123"}),
	)
	require.NoError(t, err)
	assert.Equal(t, "abc123", receivedMeta["trace_id"])
}

// -- Tests: Conversation.SendStream auto-updates history after Final() --

func TestConversation_SendStream_AutoUpdateHistory(t *testing.T) {
	callIdx := 0
	fm := testutil.NewFunctionModel(func(msgs []*schema.Message, _ []*schema.ToolInfo) (*schema.Message, error) {
		callIdx++
		switch callIdx {
		case 1:
			return &schema.Message{Role: schema.Assistant, Content: "First response"}, nil
		case 2:
			// Check that history from the streamed turn was preserved
			for _, m := range msgs {
				if m.Content == "First response" {
					return &schema.Message{Role: schema.Assistant, Content: "History preserved!"}, nil
				}
			}
			return &schema.Message{Role: schema.Assistant, Content: "History lost"}, nil
		default:
			return &schema.Message{Role: schema.Assistant, Content: "OK"}, nil
		}
	})

	a := New[NoDeps, string](fm)
	conv := NewConversation(a)

	// First turn: use SendStream
	sr, err := conv.SendStream(context.Background(), "Hello", NoDeps{})
	require.NoError(t, err)

	result, err := sr.Final()
	require.NoError(t, err)
	assert.Equal(t, "First response", result.Output)

	// History should be auto-updated after Final()
	assert.NotEmpty(t, conv.Messages(), "conversation history should be updated after Final()")

	// Second turn: use Send to verify history was carried
	r2, err := conv.Send(context.Background(), "Remember me?", NoDeps{})
	require.NoError(t, err)
	assert.Equal(t, "History preserved!", r2.Output)
}

// -- Tests: testutil assertion helpers --

func TestAssertToolCalled_Integration(t *testing.T) {
	type EmptyArgs struct{}

	tm := testutil.NewTestModel(
		testutil.TestResponse{
			ToolCalls: []schema.ToolCall{{
				ID: "call_1", Type: "function",
				Function: schema.FunctionCall{Name: "my_tool", Arguments: `{}`},
			}},
		},
		testutil.TestResponse{Text: "done"},
	)

	a := New[NoDeps, string](tm,
		WithToolFunc[NoDeps, string, EmptyArgs]("my_tool", "Tool", func(_ *RunContext[NoDeps], _ EmptyArgs) (string, error) {
			return "result", nil
		}),
	)

	_, err := a.Run(context.Background(), "Use tool", NoDeps{})
	require.NoError(t, err)

	testutil.AssertToolCalled(t, tm, "my_tool")
	testutil.AssertToolRegistered(t, tm, "my_tool")
	testutil.AssertNoSystemPrompt(t, tm)
	testutil.AssertUserPromptSent(t, tm, "Use tool")
}

// -- Tests: AllMessages --

func TestResult_AllMessages(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{Text: "hello"},
	)

	a := New[NoDeps, string](tm)
	result, err := a.Run(context.Background(), "Hi", NoDeps{})
	require.NoError(t, err)

	msgs := result.AllMessages()
	require.NotNil(t, msgs)
	assert.True(t, len(msgs) >= 2) // at least user + assistant
}

// -- Tests: Usage tracking --

func TestResult_Usage(t *testing.T) {
	tm := testutil.NewTestModel(
		testutil.TestResponse{
			Text: "response",
			Usage: &schema.TokenUsage{
				PromptTokens:     100,
				CompletionTokens: 50,
				TotalTokens:      150,
			},
		},
	)

	a := New[NoDeps, string](tm)
	result, err := a.Run(context.Background(), "Hi", NoDeps{})
	require.NoError(t, err)

	assert.Equal(t, 1, result.Usage.Requests)
	assert.Equal(t, 100, result.Usage.RequestTokens)
	assert.Equal(t, 50, result.Usage.ResponseTokens)
	assert.Equal(t, 150, result.Usage.TotalTokens)
}
